package lintpreparation

import (
	"archive/tar"
	"bytes"
	"errors"
	"reflect"
	"strconv"
	"testing"
)

func layer(t *testing.T, hs ...*tar.Header) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, h := range hs {
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Size > 0 {
			if _, err := w.Write(bytes.Repeat([]byte{'x'}, int(h.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func reason(t *testing.T, err error, want string) {
	t.Helper()
	var v *ValidationFailure
	if !errors.As(err, &v) || v.Reason != want {
		t.Fatalf("error=%v, want reason %s", err, want)
	}
}
func TestRootfsPrivileges(t *testing.T) {
	kinds := []byte{tar.TypeDir, tar.TypeReg, tar.TypeSymlink, tar.TypeLink, tar.TypeChar, tar.TypeBlock, tar.TypeFifo}
	for _, name := range []string{"var/local", ".", "./"} {
		for _, kind := range kinds {
			if name != "var/local" && kind != tar.TypeDir {
				continue
			}
			for _, mode := range []int64{0775, 02775, 04775, 06775} {
				for _, capability := range []string{"", "capability"} {
					h := &tar.Header{Name: name, Typeflag: kind, Mode: mode, Linkname: "target", PAXRecords: map[string]string{"SCHILY.xattr.security.capability": capability}}
					tree := Tree{"target": {Kind: "file", Mode: 0444, Data: []byte("existing")}}
					before := Tree{"target": tree["target"]}
					err := Overlay(tree, layer(t, h))
					rejected := mode&04000 != 0 || (mode&02000 != 0 && kind != tar.TypeDir) || capability != ""
					if rejected {
						reason(t, err, "layer-privileged")
						if !reflect.DeepEqual(tree, before) {
							t.Fatal("rejected entry mutated tree")
						}
					} else if kind == tar.TypeDir {
						if err != nil {
							t.Fatal(err)
						}
						if name == "var/local" {
							if tree[name].Mode != 0555 {
								t.Fatal(tree)
							}
						} else if !reflect.DeepEqual(tree, before) {
							t.Fatal("root marker mutated tree")
						}
					}
				}
			}
		}
	}
}

// Global PAX is outside the supported raw-tar subset, including harmless
// metadata: never silently drop inherited capabilities or other overrides.
func TestRootfsGlobalPAX(t *testing.T) {
	for _, records := range []map[string]string{
		{"comment": "ordinary metadata"},
		{"SCHILY.xattr.security.capability": "privileged-global"},
	} {
		tree := Tree{"existing": {Kind: "file", Mode: 0444, Data: []byte("keep")}}
		before := Tree{"existing": tree["existing"]}
		data := layer(t, &tar.Header{Name: "global", Typeflag: tar.TypeXGlobalHeader, PAXRecords: records}, &tar.Header{Name: "new", Typeflag: tar.TypeDir})
		err := Overlay(tree, data)
		if !errors.Is(err, tar.ErrHeader) || ErrorCategory(err) != "archive" {
			t.Fatalf("expected safe archive error, got %v", err)
		}
		if !reflect.DeepEqual(tree, before) {
			t.Fatal("global PAX rejection changed tree")
		}
	}
}

func TestRootfsFileCount(t *testing.T) {
	// Preseed empty nodes; import only one entry, not a quadratic large tar.
	tree := make(Tree, MaxFiles)
	for i := range MaxFiles {
		tree[strconv.Itoa(i)] = Node{Kind: "dir", Mode: 0555}
	}
	reason(t, Overlay(tree, layer(t, &tar.Header{Name: "new", Typeflag: tar.TypeDir})), "rootfs-limit")
	if len(tree) != MaxFiles {
		t.Fatal("rejected entry changed tree size")
	}
	for i := range MaxFiles {
		n, ok := tree[strconv.Itoa(i)]
		if !ok || n.Kind != "dir" || n.Mode != 0555 || len(n.Data) != 0 || n.Target != "" {
			t.Fatal("rejected entry changed existing node")
		}
	}
}

func TestRootfsPaths(t *testing.T) {
	for _, name := range []string{"", "/a", "../a", "a//b", "a/./b", "a\\b"} {
		_, err := SafePath(name)
		reason(t, err, "path-unsafe")
	}
	if got, err := SafePath("./a/b///"); err != nil || got != "a/b" {
		t.Fatal(got, err)
	}
	_, err := SafePath("bad\nname")
	reason(t, err, "path-invalid")
	for _, tc := range [][3]string{{"usr/bin/tool", "../lib/tool", "usr/lib/tool"}, {"bin/tool", "/usr/bin/tool", "usr/bin/tool"}} {
		got, err := LinkPath(tc[0], tc[1])
		if err != nil || got != tc[2] {
			t.Fatal(got, err)
		}
	}
	for _, target := range []string{"", "../../outside", "bad\\target", "bad\ntarget"} {
		if _, err := LinkPath("a", target); err == nil {
			t.Fatal(target)
		}
	}
}
func TestRootfsLayerRules(t *testing.T) {
	file := func(name string) *tar.Header {
		return &tar.Header{Name: name, Mode: 0755, Typeflag: tar.TypeReg, Size: 1}
	}
	for _, tc := range []struct {
		hs   []*tar.Header
		want string
	}{
		{[]*tar.Header{file("a"), file("a")}, "layer-duplicate-or-limit"},
		{[]*tar.Header{file("a/.wh.b")}, "layer-whiteout"},
		{[]*tar.Header{file("a"), file("a/b")}, "layer-ancestor"},
		{[]*tar.Header{{Name: "device", Typeflag: tar.TypeChar}}, "layer-special-entry"},
		{[]*tar.Header{file("a"), {Name: "a", Typeflag: tar.TypeDir}}, "layer-duplicate-or-limit"},
		{[]*tar.Header{{Name: "hard", Typeflag: tar.TypeLink, Linkname: "dir"}}, "layer-hardlink"},
	} {
		tree := Tree{"dir": {Kind: "dir", Mode: 0555}}
		reason(t, Overlay(tree, layer(t, tc.hs...)), tc.want)
	}
	tree := Tree{"a": {Kind: "dir", Mode: 0555}}
	reason(t, Overlay(tree, layer(t, file("a"))), "layer-type-replacement")
	if tree["a"].Kind != "dir" {
		t.Fatal(tree)
	}
	tree = Tree{}
	err := Overlay(tree, layer(t, file("file"), &tar.Header{Name: "sym", Typeflag: tar.TypeSymlink, Linkname: "/file"}, &tar.Header{Name: "hard", Typeflag: tar.TypeLink, Linkname: "sym"}))
	if err != nil {
		t.Fatal(err)
	}
	n, err := Resolve(tree, "sym")
	if err != nil || n.Mode != 0555 || string(n.Data) != "x" || !reflect.DeepEqual(n, tree["hard"]) {
		t.Fatal(n, err)
	}
	tree["cycle"] = Node{Kind: "link", Target: "cycle"}
	_, err = Resolve(tree, "cycle")
	reason(t, err, "link-cycle")
	_, err = Resolve(tree, "missing")
	reason(t, err, "rootfs-missing")
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	if err := w.WriteHeader(&tar.Header{Name: "huge", Size: MaxBytes + 1, Mode: 0444}); err != nil {
		t.Fatal(err)
	}
	retained := Tree{"existing": {Kind: "file", Mode: 0444, Data: []byte("keep")}}
	before := Tree{"existing": retained["existing"]}
	reason(t, Overlay(retained, b.Bytes()), "layer-expanded-limit")
	if !reflect.DeepEqual(retained, before) {
		t.Fatal("oversized entry changed existing tree")
	}
	for _, data := range [][]byte{nil, make([]byte, 511), layer(t, file("x"))[:513]} {
		if err := Overlay(Tree{}, data); ErrorCategory(err) != "archive" {
			t.Fatalf("expected archive error, got %v", err)
		}
	}
}
