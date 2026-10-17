package lintimage_test

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/lintimage"
)

type item struct {
	name, target string
	kind         byte
	mode         int64
	data         []byte
}

func rootFixture(t *testing.T, nodes []item) ([]byte, []byte, lintimage.Identities) {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	records := []map[string]any{}
	for _, n := range nodes {
		h := &tar.Header{Name: n.name, Linkname: n.target, Typeflag: n.kind, Mode: n.mode, ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}
		if n.kind == tar.TypeReg {
			h.Size = int64(len(n.data))
		}
		if e := w.WriteHeader(h); e != nil {
			t.Fatal(e)
		}
		if n.kind == tar.TypeReg {
			if _, e := w.Write(n.data); e != nil {
				t.Fatal(e)
			}
		}
		r := map[string]any{"path": n.name, "mode": "0o555", "uid": 0, "gid": 0}
		switch n.kind {
		case tar.TypeDir:
			r["type"] = "dir"
		case tar.TypeReg:
			r["type"] = "file"
			r["mode"] = "0o444"
			r["size"] = len(n.data)
			r["sha256"] = digest(n.data)
		case tar.TypeSymlink:
			r["type"] = "link"
			r["mode"] = "0o777"
			r["target"] = n.target
		default:
			r["type"] = "special"
		}
		records = append(records, r)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	layer := b.Bytes()
	a, ids := fixture(t, configBytes(layer), layer, nil, nil, false)
	inv, _ := json.Marshal(records)
	return a, inv, ids
}
func roots() []item {
	return []item{{name: "work", kind: tar.TypeDir, mode: 0555}, {name: "work/file", kind: tar.TypeReg, mode: 0444, data: []byte("ok\n")}, {name: "work/link", kind: tar.TypeSymlink, mode: 0777, target: "file"}}
}
func TestPhysicalPAXProfile(t *testing.T) {
	a, inv, _ := rootFixture(t, []item{{name: "short", kind: tar.TypeReg, mode: 0444, data: []byte("ok\n")}})
	tr := tar.NewReader(bytes.NewReader(a))
	var layer []byte
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil {
			t.Fatal(e)
		}
		if h.Name == "layer.tar" {
			layer, _ = io.ReadAll(tr)
		}
	}
	long := strings.Repeat("a", 16384)
	inv = bytes.ReplaceAll(inv, []byte("short"), []byte(long))
	body := "path=" + long + "\n"
	n := len(body) + 2
	for next := len(body) + len(fmt.Sprint(n)) + 1; next != n; next = len(body) + len(fmt.Sprint(n)) + 1 {
		n = next
	}
	payload := []byte(fmt.Sprintf("%d %s", n, body))
	for _, name := range []string{"././@PaxHeader", "././@unowned"} {
		h := append([]byte{}, layer[:512]...)
		clear(h[:100])
		copy(h, name)
		copy(h[100:108], "0000000\x00")
		copy(h[124:136], fmt.Sprintf("%011o\x00", len(payload)))
		h[156] = 'x'
		copy(h[329:345], "0000000\x000000000\x00")
		copy(h[148:156], "        ")
		sum := 0
		for _, v := range h {
			sum += int(v)
		}
		copy(h[148:156], fmt.Sprintf("%06o\x00 ", sum))
		raw := append(h, payload...)
		raw = append(raw, make([]byte, (512-len(payload)%512)%512)...)
		raw = append(raw, layer...)
		archive, ids := fixture(t, configBytes(raw), raw, nil, nil, false)
		_, err := lintimage.InspectInventory(bytes.NewReader(archive), bytes.NewReader(inv), ids)
		if (err == nil) != (name == "././@PaxHeader") {
			t.Errorf("PAX helper %s: %v", name, err)
		}
	}
}

func TestInventoryExactMetadata(t *testing.T) {
	a, inv, ids := rootFixture(t, roots())
	r, e := lintimage.InspectInventory(bytes.NewReader(a), bytes.NewReader(inv), ids)
	if e != nil || r.Verdict != "INVENTORY_CONSISTENT_NOT_ADMITTED" || r.RuntimeAdmitted || r.InventoryEntries != 3 || r.FileBytes != 3 {
		t.Fatalf("valid inventory: %+v %v", r, e)
	}
	for _, bad := range [][]byte{bytes.Replace(inv, []byte(`"size":3`), []byte(`"size":4`), 1), bytes.Replace(inv, []byte(`"uid":0`), []byte(`"uid":1`), 1), bytes.Replace(inv, []byte(`"target":"file"`), []byte(`"target":"other"`), 1), []byte(`[]`), append(inv, []byte(` []`)...)} {
		report, err := lintimage.InspectInventory(bytes.NewReader(a), bytes.NewReader(bad), ids)
		if err != lintimage.ErrInvalid || report != (lintimage.Report{}) {
			t.Fatal("inventory mismatch accepted")
		}
	}
}
func TestLinkTraversalControls(t *testing.T) {
	for _, target := range []string{"file", "/work/file", "../work/file", ".", "/"} {
		nodes := roots()
		nodes[2].target = target
		a, inv, ids := rootFixture(t, nodes)
		if _, err := lintimage.InspectInventory(bytes.NewReader(a), bytes.NewReader(inv), ids); err != nil {
			t.Fatalf("valid link %q: %v", target, err)
		}
	}
	for _, count := range []int{128, 129} {
		nodes := roots()[:2]
		for i := 0; i < count; i++ {
			target := fmt.Sprintf("l%03d", i+1)
			if i == count-1 {
				target = "file"
			}
			nodes = append(nodes, item{name: fmt.Sprintf("work/l%03d", i), kind: tar.TypeSymlink, mode: 0777, target: target})
		}
		a, inv, ids := rootFixture(t, nodes)
		_, err := lintimage.InspectInventory(bytes.NewReader(a), bytes.NewReader(inv), ids)
		if (err == nil) != (count == 128) {
			t.Fatalf("link rewrite boundary %d: %v", count, err)
		}
	}
}

func TestInventoryBounds(t *testing.T) {
	for _, n := range []int{50000, 50001} {
		nodes := make([]item, n)
		for i := range nodes {
			nodes[i] = item{name: fmt.Sprintf("d%05d", i), kind: tar.TypeDir, mode: 0555}
		}
		a, inv, ids := rootFixture(t, nodes)
		_, err := lintimage.InspectInventory(bytes.NewReader(a), bytes.NewReader(inv), ids)
		if (err == nil) != (n == 50000) {
			t.Fatalf("entry boundary %d: %v", n, err)
		}
	}
	a, inv, ids := rootFixture(t, roots())
	for _, n := range []int{16 << 20, (16 << 20) + 1} {
		padded := append(append([]byte{}, inv...), bytes.Repeat([]byte(" "), n-len(inv))...)
		_, err := lintimage.InspectInventory(bytes.NewReader(a), bytes.NewReader(padded), ids)
		if (err == nil) != (n == 16<<20) {
			t.Fatalf("inventory byte boundary %d: %v", n, err)
		}
	}
}

func TestFilesystemNamespaceAndTypes(t *testing.T) {
	for _, tc := range []struct {
		name  string
		alter func([]item) []item
	}{
		{"escape", func(n []item) []item { n[1].name = "../outside"; return n }},
		{"duplicate", func(n []item) []item { n[2].name = n[1].name; return n }},
		{"writable", func(n []item) []item { n[1].mode = 0644; return n }},
		{"dangling", func(n []item) []item { n[2].target = "missing"; return n }},
		{"link-escape", func(n []item) []item { n[2].target = "../../outside"; return n }},
		{"cycle", func(n []item) []item { n[2].target = "link"; return n }},
		{"trailing-slash", func(n []item) []item { n[2].target = "file/"; return n }},
		{"dot-through-file", func(n []item) []item { n[2].target = "file/."; return n }},
		{"parent-through-file", func(n []item) []item { n[2].target = "file/../file"; return n }},
		{"hidden-cycle", func(n []item) []item { n[2].target = "link/../file"; return n }},
		{"hidden-missing", func(n []item) []item { n[2].target = "missing/../file"; return n }},
		{"link-parent", func(n []item) []item { return append(n, item{name: "work/link/child", kind: tar.TypeReg, mode: 0444}) }},
		{"missing-parent", func(n []item) []item { n[1].name = "absent/file"; return n }},
		{"special", func(n []item) []item { n[1].kind = tar.TypeFifo; n[1].data = nil; return n }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, inv, ids := rootFixture(t, tc.alter(roots()))
			if _, err := lintimage.InspectInventory(bytes.NewReader(a), bytes.NewReader(inv), ids); err != lintimage.ErrInvalid {
				t.Fatalf("unsafe filesystem: %v", err)
			}
		})
	}
}
