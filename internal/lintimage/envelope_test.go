package lintimage_test

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/lintimage"
)

func digest(b []byte) string { s := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(s[:]) }
func layerBytes() []byte     { var b bytes.Buffer; w := tar.NewWriter(&b); _ = w.Close(); return b.Bytes() }
func configBytes(layer []byte) []byte {
	b, _ := json.Marshal(map[string]any{"architecture": "amd64", "os": "linux", "config": map[string]any{"User": "65532:65532", "WorkingDir": "/work", "Env": []any{}, "Entrypoint": []any{}, "Cmd": []any{}}, "rootfs": map[string]any{"type": "layers", "diff_ids": []string{digest(layer)}}, "history": []any{}})
	return b
}
func fixture(t *testing.T, config, layer []byte, change func(*tar.Header), manifest []byte, extra bool) ([]byte, lintimage.Identities) {
	t.Helper()
	ids := lintimage.Identities{ImageID: digest(config), LayerDiffID: digest(layer)}
	if manifest == nil {
		manifest, _ = json.Marshal([]any{map[string]any{"Config": ids.ImageID[7:] + ".json", "RepoTags": []any{}, "Layers": []string{"layer.tar"}}})
	}
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	entries := []struct {
		name string
		data []byte
	}{{ids.ImageID[7:] + ".json", config}, {"layer.tar", layer}, {"manifest.json", manifest}}
	if extra {
		entries = append(entries, struct {
			name string
			data []byte
		}{"unexpected", nil})
	}
	for _, e := range entries {
		h := &tar.Header{Name: e.name, Mode: 0444, Size: int64(len(e.data)), Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}
		if change != nil {
			change(h)
		}
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	ids.ArchiveSHA256 = digest(b.Bytes())
	return b.Bytes(), ids
}
func TestEnvelopeIdentities(t *testing.T) {
	layer := layerBytes()
	archive, ids := fixture(t, configBytes(layer), layer, nil, nil, false)
	report, err := lintimage.InspectEnvelope(bytes.NewReader(archive), ids)
	if err != nil || report.Verdict != "ARCHIVE_ENVELOPE_ONLY_NOT_ADMITTED" || report.RuntimeAdmitted || report.Identities != ids {
		t.Fatalf("valid envelope: %+v %v", report, err)
	}
	for _, field := range []string{"archive", "image", "layer"} {
		t.Run(field, func(t *testing.T) {
			bad := ids
			switch field {
			case "archive":
				bad.ArchiveSHA256 = digest(nil)
			case "image":
				bad.ImageID = digest(nil)
			case "layer":
				bad.LayerDiffID = digest(nil)
			}
			if _, e := lintimage.InspectEnvelope(bytes.NewReader(archive), bad); e == nil {
				t.Fatal("identity mismatch accepted")
			}
		})
	}
}
func TestEnvelopeRejectsHiddenExtensions(t *testing.T) {
	layer := layerBytes()
	config := configBytes(layer)
	a, ids := fixture(t, config, layer, nil, nil, false)
	offset := 512 + (len(config)+511)/512*512 + 512 + len(layer)
	for _, kind := range []byte{'L', 'x'} {
		payload := []byte("manifest.json\x00")
		if kind == 'x' {
			payload = nil
		}
		header := append([]byte{}, a[:512]...)
		clear(header[:100])
		copy(header, "././@extension")
		copy(header[124:136], fmt.Sprintf("%011o\x00", len(payload)))
		header[156] = kind
		copy(header[148:156], "        ")
		checksum := 0
		for _, c := range header {
			checksum += int(c)
		}
		copy(header[148:156], fmt.Sprintf("%06o\x00 ", checksum))
		for _, position := range []int{offset, len(a) - 1024} {
			bad := append([]byte{}, a[:position]...)
			bad = append(bad, header...)
			bad = append(bad, payload...)
			bad = append(bad, make([]byte, (512-len(payload)%512)%512)...)
			bad = append(bad, a[position:]...)
			ids.ArchiveSHA256 = digest(bad)
			if _, err := lintimage.InspectEnvelope(bytes.NewReader(bad), ids); err == nil {
				t.Errorf("hidden extension %c at %d accepted", kind, position)
			}
			if position == len(a)-1024 {
				withoutTerminal := bad[:len(bad)-1024]
				ids.ArchiveSHA256 = digest(withoutTerminal)
				report, err := lintimage.InspectEnvelope(bytes.NewReader(withoutTerminal), ids)
				if err != lintimage.ErrInvalid || report != (lintimage.Report{}) {
					t.Errorf("hidden extension %c without terminal zeros: report=%+v err=%v", kind, report, err)
				}
			}
		}
	}
}

func TestEnvelopeMetadataBound(t *testing.T) {
	layer := layerBytes()
	config := configBytes(layer)
	padded := append(config, bytes.Repeat([]byte(" "), 4096-len(config))...)
	for _, n := range []int{4096, 4097} {
		c := append([]byte{}, padded...)
		if n == 4097 {
			c = append(c, ' ')
		}
		a, ids := fixture(t, c, layer, nil, nil, false)
		_, err := lintimage.InspectEnvelope(bytes.NewReader(a), ids)
		if (err == nil) != (n == 4096) {
			t.Fatalf("metadata boundary %d: %v", n, err)
		}
	}
}

func TestEnvelopeRejectsUnsafeOrAmbiguousInput(t *testing.T) {
	layer := layerBytes()
	config := configBytes(layer)
	for _, tc := range []struct {
		name     string
		config   []byte
		change   func(*tar.Header)
		manifest []byte
		extra    bool
	}{
		{name: "extra", extra: true},
		{name: "uid", change: func(h *tar.Header) { h.Uid = 1 }},
		{name: "writable", change: func(h *tar.Header) { h.Mode = 0644 }},
		{name: "time", change: func(h *tar.Header) { h.ModTime = time.Unix(1, 0) }},
		{name: "path", change: func(h *tar.Header) {
			if h.Name == "manifest.json" {
				h.Name = "../manifest.json"
			}
		}},
		{name: "duplicate", change: func(h *tar.Header) { h.Name = "manifest.json" }},
		{name: "duplicate-json", config: append([]byte(`{"os":"linux",`), config[1:]...)},
		{name: "unsafe-default", config: bytes.Replace(config, []byte(`"Entrypoint":[]`), []byte(`"Entrypoint":["/bin/sh"]`), 1)},
		{name: "wrong-platform", config: bytes.Replace(config, []byte(`"amd64"`), []byte(`"arm64"`), 1)},
		{name: "manifest-reference", manifest: []byte(`[{"Config":"other.json","Layers":["layer.tar"],"RepoTags":[]}]`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := config
			if tc.config != nil {
				c = tc.config
			}
			a, ids := fixture(t, c, layer, tc.change, tc.manifest, tc.extra)
			if _, e := lintimage.InspectEnvelope(bytes.NewReader(a), ids); e == nil {
				t.Fatal("unsafe envelope accepted")
			}
		})
	}
	a, ids := fixture(t, config, layer, nil, nil, false)
	for _, bad := range [][]byte{a[:len(a)-1], append(append([]byte{}, a...), make([]byte, 512)...)} {
		if len(bad)%512 == 0 {
			bad[len(bad)-1] = 1
		}
		ids.ArchiveSHA256 = digest(bad)
		if _, e := lintimage.InspectEnvelope(bytes.NewReader(bad), ids); e == nil {
			t.Fatal("invalid framing accepted")
		}
	}
}
