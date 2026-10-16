package lintpreparation

import (
	"archive/tar"
	"bytes"
	"strings"
	"testing"
	"time"
)

// Rehash every mutated layer/config/archive, so failures cannot be stale hashes.
func inspectCandidate(t *testing.T, layer []byte, change func(map[string]any), mutate func(Tree)) ([]byte, ImageIdentities) {
	t.Helper()
	c := map[string]any{"architecture": "amd64", "os": "linux", "config": map[string]any{"User": "65532:65532", "WorkingDir": "/work", "Env": []any{}, "Cmd": []any{}, "Entrypoint": []any{}}, "rootfs": map[string]any{"type": "layers", "diff_ids": []any{Digest(layer)}}, "history": []any{}}
	if change != nil {
		change(c)
	}
	config, e := Canonical(c)
	if e != nil {
		t.Fatal(e)
	}
	name := strings.TrimPrefix(Digest(config), "sha256:") + ".json"
	manifest, _ := Canonical([]any{map[string]any{"Config": name, "RepoTags": []any{}, "Layers": []any{"layer.tar"}}})
	outer := Tree{name: {Kind: "file", Mode: 0444, Data: config}, "manifest.json": {Kind: "file", Mode: 0444, Data: manifest}, "layer.tar": {Kind: "file", Mode: 0444, Data: layer}}
	if mutate != nil {
		mutate(outer)
	}
	b := imageTar(outer)
	return b, ImageIdentities{Digest(config), Digest(layer), Digest(b)}
}

func TestInspectArchive(t *testing.T) {
	tree := imageFixture()
	long := strings.Repeat("a", 110) + "é"
	tree[long] = Node{Kind: "file", Mode: 0444, Data: []byte("long")}
	tree["unicode"] = Node{Kind: "link", Mode: 0777, Target: "/" + long}
	normalized := Tree{}
	if e := Overlay(normalized, imageTar(Tree{"sgid": {Kind: "dir", Mode: 02775}})); e != nil {
		t.Fatal(e)
	}
	tree["sgid"] = normalized["sgid"]
	inv, _ := Inventory(tree)
	b, ids, e := PackImage(tree)
	if e != nil {
		t.Fatal(e)
	}
	if e := InspectArchive(b, inv, ids); e != nil {
		t.Fatal(e)
	}
	reject := func(t *testing.T, b []byte, inv []any, id ImageIdentities) {
		t.Helper()
		if InspectArchive(b, inv, id) == nil {
			t.Fatal("accepted invalid candidate")
		}
	}
	for _, field := range []string{"image", "layer", "archive"} {
		t.Run(field, func(t *testing.T) {
			bad := ids
			switch field {
			case "image":
				bad.ImageID = Digest(nil)
			case "layer":
				bad.LayerDiffID = Digest(nil)
			case "archive":
				bad.ArchiveSHA256 = Digest(nil)
			}
			reject(t, b, inv, bad)
		})
	}
	t.Run("inventory", func(t *testing.T) { reject(t, b, []any{}, ids) })
	t.Run("malformed", func(t *testing.T) { reject(t, b[:513], inv, ids) })
	for _, rule := range []string{"architecture", "os", "history", "extra", "User", "WorkingDir", "Env", "Cmd", "Entrypoint", "null", "rootfs"} {
		t.Run(rule, func(t *testing.T) {
			data, id := inspectCandidate(t, imageTar(tree), func(c map[string]any) {
				switch rule {
				case "architecture", "os", "history", "extra", "rootfs":
					c[rule] = "wrong"
				case "null":
					c["config"].(map[string]any)["Env"] = nil
				default:
					c["config"].(map[string]any)[rule] = "wrong"
				}
			}, nil)
			reject(t, data, inv, id)
		})
	}
	for _, rule := range []string{"extra", "missing", "tags", "schema", "duplicate-json", "config-name", "outer-mode"} {
		t.Run(rule, func(t *testing.T) {
			data, id := inspectCandidate(t, imageTar(tree), nil, func(o Tree) {
				switch rule {
				case "extra":
					o["extra"] = Node{Kind: "file", Mode: 0444}
				case "missing":
					delete(o, "layer.tar")
				case "outer-mode":
					n := o["layer.tar"]
					n.Mode = 0644
					o["layer.tar"] = n
				case "config-name":
					for n, v := range o {
						if n != "manifest.json" && n != "layer.tar" {
							delete(o, n)
							o["wrong.json"] = v
							break
						}
					}
				default:
					n := o["manifest.json"]
					switch rule {
					case "tags":
						n.Data = bytes.Replace(n.Data, []byte(`"RepoTags":[]`), []byte(`"RepoTags":["tag"]`), 1)
					case "schema":
						n.Data = []byte(`[]`)
					case "duplicate-json":
						n.Data = []byte(`[{"Config":"x","Config":"y"}]`)
					}
					o["manifest.json"] = n
				}
			})
			reject(t, data, inv, id)
		})
	}
	canonical, _ := Inventory(Tree{"config": {Kind: "file", Mode: 0444, Data: []byte("x")}})
	for _, rule := range []string{"writable", "suid", "sgid", "uid", "gid", "time", "hardlink", "global", "capability", "size", "duplicate"} {
		t.Run(rule, func(t *testing.T) {
			var buf bytes.Buffer
			w := tar.NewWriter(&buf)
			h := &tar.Header{Name: "config", Typeflag: tar.TypeReg, Mode: 0444, Size: 1}
			switch rule {
			case "writable":
				h.Mode = 0644
			case "suid":
				h.Mode = 04444
			case "sgid":
				h.Mode = 02444
			case "uid":
				h.Uid = 1
			case "gid":
				h.Gid = 1
			case "time":
				h.ModTime = time.Unix(1, 0)
			case "hardlink":
				h.Typeflag = tar.TypeLink
				h.Linkname = "bin"
				h.Size = 0
			case "global":
				*h = tar.Header{Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": "x"}}
			case "capability":
				h.PAXRecords = map[string]string{"SCHILY.xattr.security.capability": "x"}
			case "size":
				h.Size = MaxBytes + 1
			}
			if e := w.WriteHeader(h); e != nil {
				t.Fatal(e)
			}
			if h.Size == 1 {
				if _, e := w.Write([]byte("x")); e != nil {
					t.Fatal(e)
				}
			}
			if rule == "duplicate" {
				if e := w.WriteHeader(h); e != nil {
					t.Fatal(e)
				}
				w.Write([]byte("x"))
			}
			if rule != "size" {
				if e := w.Close(); e != nil {
					t.Fatal(e)
				}
			}
			raw, e := tar.NewReader(bytes.NewReader(buf.Bytes())).Next()
			if e != nil {
				t.Fatal(e)
			}
			if raw.Mode != h.Mode || raw.Uid != h.Uid || raw.Gid != h.Gid || raw.Typeflag != h.Typeflag || raw.Linkname != h.Linkname || raw.Size != h.Size {
				t.Fatalf("fixture lost metadata: %#v", raw)
			}
			if rule == "time" && raw.ModTime.Unix() != 1 {
				t.Fatal("lost time")
			}
			if rule == "capability" && raw.PAXRecords["SCHILY.xattr.security.capability"] != "x" {
				t.Fatal("lost capability")
			}
			data, id := inspectCandidate(t, buf.Bytes(), nil, nil)
			reject(t, data, canonical, id)
		})
	}
	t.Run("outer-duplicate", func(t *testing.T) {
		var buf bytes.Buffer
		w := tar.NewWriter(&buf)
		for range 2 {
			w.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0444, Size: 2})
			w.Write([]byte("[]"))
		}
		w.Close()
		id := ids
		id.ArchiveSHA256 = Digest(buf.Bytes())
		reject(t, buf.Bytes(), inv, id)
	})
}

func TestInspectContainerBudgets(t *testing.T) {
	if inspectLayerBudget(MaxBytes, MaxBytes, MaxFiles) != inspectLayerLimit {
		t.Fatal("budget drift")
	}
	if inspectLayerLimit <= MaxBytes || inspectOuterLimit <= inspectLayerLimit {
		t.Fatal("container overhead missing")
	}
	// Tiny arithmetic fixture includes long Unicode PAX, payload and link padding.
	tree := Tree{strings.Repeat("é", 101): {Kind: "file", Mode: 0444, Data: []byte("x")}, "link": {Kind: "link", Mode: 0777, Target: strings.Repeat("é", 101)}}
	var meta int64
	for n, v := range tree {
		meta += int64(len(n) + len(v.Target))
	}
	if int64(len(imageTar(tree))) > inspectLayerBudget(1, meta, 2) {
		t.Fatal("PAX budget too small")
	}
	// Only a header, never claimed-size body allocation.
	for _, name := range []string{"manifest.json", "layer.tar"} {
		var buf bytes.Buffer
		size := inspectJSONLimit + 1
		if name == "layer.tar" {
			size = inspectLayerLimit + 1
		}
		imageHeader(&buf, name, "", '0', 0444, size)
		buf.Write(make([]byte, 512))
		visited := false
		err := inspectHeaders(buf.Bytes(), true, func(*tar.Header, *tar.Reader) error { visited = true; return nil })
		if err == nil || visited {
			t.Fatal("oversized outer body reached reader")
		}
	}
}
