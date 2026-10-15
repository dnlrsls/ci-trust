package ownedimage

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// This oracle deliberately does not call Pack, archive, layerEntries, or digest.
func literalTar(t *testing.T, names []string, bodies [][]byte, modes []int64, ids []int) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for i, n := range names {
		kind := byte(tar.TypeReg)
		if n[len(n)-1] == '/' {
			kind = tar.TypeDir
		}
		h := &tar.Header{Name: n, Mode: modes[i], Uid: ids[i], Gid: ids[i], Typeflag: kind, Size: int64(len(bodies[i])), ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}
		if e := w.WriteHeader(h); e != nil {
			t.Fatal(e)
		}
		if _, e := w.Write(bodies[i]); e != nil {
			t.Fatal(e)
		}
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	return b.Bytes()
}
func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func oracle(t *testing.T, b []byte, configChange, layerChange string) []byte {
	t.Helper()
	names := []string{"ci-trust/", "ci-trust/owned-smoke", "ci-trust/root-marker", "tmp/"}
	bodies := [][]byte{nil, b, []byte("ci-trust owned-smoke-v1 root marker\n"), nil}
	modes := []int64{0755, 0555, 0600, 01777}
	ids := []int{0, 0, 65534, 0}
	switch layerChange {
	case "content":
		bodies[1] = []byte("wrong")
	case "owner":
		ids[2] = 0
	case "mode":
		modes[3] = 0777
	case "traversal":
		names[1] = "../owned-smoke"
	case "duplicate":
		names[1] = names[2]
	case "extra":
		names = append(names, "extra")
		bodies = append(bodies, nil)
		modes = append(modes, 0644)
		ids = append(ids, 0)
	}
	l := literalTar(t, names, bodies, modes, ids)
	config := fmt.Sprintf(`{"architecture":"amd64","os":"linux","config":{},"rootfs":{"type":"layers","diff_ids":["sha256:%s"]}}`, hash(l))
	switch configChange {
	case "alias":
		config = fmt.Sprintf(`{"architecture":"amd64","Architecture":"amd64","os":"linux","config":{},"rootfs":{"type":"layers","diff_ids":["sha256:%s"]}}`, hash(l))
	case "nested-alias":
		config = fmt.Sprintf(`{"architecture":"amd64","os":"linux","config":{},"rootfs":{"type":"layers","Type":"layers","diff_ids":["sha256:%s"]}}`, hash(l))
	case "unknown":
		config = fmt.Sprintf(`{"architecture":"amd64","os":"linux","unexpected":0,"config":{},"rootfs":{"type":"layers","diff_ids":["sha256:%s"]}}`, hash(l))
	case "oversized":
		config += string(bytes.Repeat([]byte{' '}, 16384))
	case "env":
		config = fmt.Sprintf(`{"architecture":"amd64","os":"linux","config":{"Env":[]},"rootfs":{"type":"layers","diff_ids":["sha256:%s"]}}`, hash(l))
	case "duplicate":
		config = `{"architecture":"amd64","architecture":"amd64","os":"linux","config":{},"rootfs":{"type":"layers","diff_ids":["sha256:` + hash(l) + `"]}}`
	case "null":
		config = fmt.Sprintf(`{"architecture":"amd64","os":"linux","config":null,"rootfs":{"type":"layers","diff_ids":["sha256:%s"]}}`, hash(l))
	case "diff":
		config = `{"architecture":"amd64","os":"linux","config":{},"rootfs":{"type":"layers","diff_ids":["sha256:bad"]}}`
	}
	cn := hash([]byte(config)) + ".json"
	manifest := fmt.Sprintf(`[{"Config":%q,"RepoTags":[],"Layers":["layer.tar"]}]`, cn)
	if configChange == "manifest-alias" {
		manifest = fmt.Sprintf(`[{"Config":%q,"config":%q,"RepoTags":[],"Layers":["layer.tar"]}]`, cn, cn)
	}
	if configChange == "manifest-unknown" {
		manifest = fmt.Sprintf(`[{"Config":%q,"Unexpected":0,"RepoTags":[],"Layers":["layer.tar"]}]`, cn)
	}
	return literalTar(t, []string{"manifest.json", cn, "layer.tar"}, [][]byte{[]byte(manifest), []byte(config), l}, []int64{0644, 0644, 0644}, []int{0, 0, 0})
}
func TestIndependentOracle(t *testing.T) {
	p, m := fixture(t)
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	a := oracle(t, b, "", "")
	meta, e := Inspect(a, p, m)
	if e != nil {
		t.Fatal(e)
	}
	if meta.ImageID == "" || meta.ArchiveSHA256 != "sha256:"+hash(a) || meta.BinarySHA256 != "sha256:"+hash(b) || meta.MarkerSHA256 != "sha256:"+hash([]byte("ci-trust owned-smoke-v1 root marker\n")) {
		t.Fatal(meta)
	}
	packed, e := Pack(p, m)
	if e != nil || !bytes.Equal(a, packed) {
		t.Fatal("producer differs from literal oracle", e)
	}
	for _, v := range []struct{ c, l string }{{"alias", ""}, {"nested-alias", ""}, {"unknown", ""}, {"manifest-alias", ""}, {"manifest-unknown", ""}, {"oversized", ""}, {"env", ""}, {"duplicate", ""}, {"null", ""}, {"diff", ""}, {"", "content"}, {"", "owner"}, {"", "mode"}, {"", "traversal"}, {"", "duplicate"}, {"", "extra"}} {
		t.Run(v.c+v.l, func(t *testing.T) {
			if _, e := Inspect(oracle(t, b, v.c, v.l), p, m); e == nil {
				t.Fatal("accepted tamper")
			}
		})
	}
	for _, v := range [][]byte{a[:len(a)-512], append(append([]byte{}, a...), make([]byte, 512)...), append(append([]byte{}, a...), 1)} {
		if _, e := Inspect(v, p, m); e == nil {
			t.Fatal("accepted malformed trailer")
		}
	}
}
func TestInputRejection(t *testing.T) {
	for _, name := range []string{"marker", "elf", "dynamic", "oversize", "directory", "missing", "foreign-abi", "extended-sections"} {
		t.Run(name, func(t *testing.T) {
			p, m := fixture(t)
			before, _ := os.ReadFile(p)
			switch name {
			case "foreign-abi":
				before[7] = 9
				if e := os.WriteFile(p, before, 0600); e != nil {
					t.Fatal(e)
				}
			case "extended-sections":
				before[60] = 255
				before[61] = 255
				if e := os.WriteFile(p, before, 0600); e != nil {
					t.Fatal(e)
				}
			case "marker":
				if e := os.WriteFile(m, []byte("wrong"), 0600); e != nil {
					t.Fatal(e)
				}
			case "elf":
				if e := os.WriteFile(p, []byte("not ELF"), 0600); e != nil {
					t.Fatal(e)
				}
			case "dynamic":
				before[64] = 3
				if e := os.WriteFile(p, before, 0600); e != nil {
					t.Fatal(e)
				}
			case "oversize":
				f, e := os.OpenFile(p, os.O_WRONLY, 0)
				if e != nil {
					t.Fatal(e)
				}
				e = f.Truncate(MaxBinary + 1)
				f.Close()
				if e != nil {
					t.Fatal(e)
				}
			case "directory":
				p = filepath.Dir(p)
			case "missing":
				p += "missing"
			}
			if _, e := Pack(p, m); e == nil {
				t.Fatal("accepted invalid input")
			}
		})
	}
}
func TestRawArchivePolicy(t *testing.T) {
	p, m := fixture(t)
	a, e := Pack(p, m)
	if e != nil {
		t.Fatal(e)
	}
	tr := tar.NewReader(bytes.NewReader(a))
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		if h.Name == "manifest.json" {
			b, _ := io.ReadAll(tr)
			if !bytes.Contains(b, []byte(`"Layers":["layer.tar"]`)) {
				t.Fatal(string(b))
			}
		}
	}
}
