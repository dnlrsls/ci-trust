package lintpreparation

import (
	"archive/tar"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func materializeFixture(t *testing.T, links bool) *NodeBundle {
	t.Helper()
	file := nodeHeader("bin/node", tar.TypeReg)
	file.Size, file.Mode = 3, 06711
	license := nodeHeader("LICENSE", tar.TypeReg)
	license.Size, license.Mode = 2, 0601
	hard := nodeHeader("node-copy", tar.TypeLink)
	hard.Linkname = file.Name
	headers := []*tar.Header{hard, file, license}
	if links {
		alias := nodeHeader("alias", tar.TypeSymlink)
		alias.Linkname = "lib"
		extra := nodeHeader("alias/extra", tar.TypeReg)
		extra.Size = 1
		npm := nodeHeader("bin/npm", tar.TypeSymlink)
		npm.Linkname = "../lib/extra"
		dangling := nodeHeader("dangling", tar.TypeSymlink)
		dangling.Linkname = "missing/leaf"
		headers = append(headers, alias, extra, npm, dangling)
	}
	b, err := decodeSyntheticNode(t, headers...)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func materializeEmpty(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("destination changed: %v %v", entries, err)
	}
}
func materializeError(t *testing.T, root string, err error, category string) {
	t.Helper()
	if root != "" || err == nil || Diagnostic(err) != Diagnostic(NewPreparationFailure("node-extract", category, "")) {
		t.Fatalf("unsafe result: %q %v", root, err)
	}
}

func TestMaterializeNodeBundle(t *testing.T) {
	t.Run("nil and zero cannot create destination", func(t *testing.T) {
		for _, b := range []*NodeBundle{nil, {}} {
			parent := t.TempDir()
			root, err := MaterializeNodeBundle(context.Background(), b, filepath.Join(parent, "absent"))
			materializeError(t, root, err, "validation")
			materializeEmpty(t, parent)
		}
	})
	t.Run("ordinary bytes copies and requested modes", func(t *testing.T) {
		dest := t.TempDir()
		b := materializeFixture(t, false)
		before := map[string]string{}
		for _, e := range b.entries {
			before[e.name] = Digest(e.data)
		}
		root, err := MaterializeNodeBundle(context.Background(), b, dest)
		if err != nil || root != filepath.Join(dest, nodeBundlePrefix) {
			t.Fatalf("%q %v", root, err)
		}
		for name, want := range map[string]string{"bin/node": "xxx", "node-copy": "xxx", "LICENSE": "xx"} {
			data, e := os.ReadFile(filepath.Join(root, name))
			if e != nil || string(data) != want {
				t.Fatalf("%s: %q %v", name, data, e)
			}
		}
		if err := os.WriteFile(filepath.Join(root, "node-copy"), []byte("changed"), 0600); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(root, "bin/node"))
		if err != nil || string(data) != "xxx" {
			t.Fatalf("hardlink instead of copy: %q %v", data, err)
		}
		for _, e := range b.entries {
			if Digest(e.data) != before[e.name] {
				t.Fatal("bundle mutated")
			}
		}
		if runtime.GOOS != "windows" {
			for name, mode := range map[string]os.FileMode{".": 0700, "bin": 0700, "bin/node": 0700, "LICENSE": 0600} {
				st, e := os.Stat(filepath.Join(root, name))
				if e != nil || st.Mode().Perm() != mode || st.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
					t.Fatalf("mode %s: %v %v", name, st, e)
				}
			}
		} else {
			t.Log("POSIX owner-mode proof inapplicable on Windows; no ACL parity claimed")
		}
	})
	t.Run("links last physical aliases and dangling targets", func(t *testing.T) {
		dest := t.TempDir()
		root, err := MaterializeNodeBundle(context.Background(), materializeFixture(t, true), dest)
		if err != nil {
			t.Fatalf("filesystem symlink proof failed or unavailable: %v", err)
		}
		for name, target := range map[string]string{"alias": "lib", "bin/npm": "../lib/extra", "dangling": "missing/leaf"} {
			got, e := os.Readlink(filepath.Join(root, name))
			if e != nil || got != filepath.FromSlash(target) {
				t.Fatalf("link %s: %q %v", name, got, e)
			}
		}
		for _, name := range []string{"lib/extra", "alias/extra", "bin/npm"} {
			data, e := os.ReadFile(filepath.Join(root, name))
			if e != nil || string(data) != "x" {
				t.Fatalf("physical %s: %q %v", name, data, e)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "dangling")); !os.IsNotExist(err) {
			t.Fatalf("dangling became live: %v", err)
		}
	})
	t.Run("nonempty destination preserved", func(t *testing.T) {
		dest := t.TempDir()
		counter := filepath.Join(dest, "counter")
		if err := os.WriteFile(counter, []byte("0\n"), 0600); err != nil {
			t.Fatal(err)
		}
		root, err := MaterializeNodeBundle(context.Background(), materializeFixture(t, false), dest)
		materializeError(t, root, err, "validation")
		data, e := os.ReadFile(counter)
		entries, re := os.ReadDir(dest)
		if e != nil || re != nil || string(data) != "0\n" || len(entries) != 1 {
			t.Fatal("preexisting data/counter changed")
		}
	})
	t.Run("root symlink rejected without sibling changes", func(t *testing.T) {
		base := t.TempDir()
		sibling := filepath.Join(base, "sibling")
		alias := filepath.Join(base, "alias")
		if err := os.Mkdir(sibling, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(sibling, alias); err != nil {
			t.Fatalf("filesystem symlink proof unavailable: %v", err)
		}
		root, err := MaterializeNodeBundle(context.Background(), materializeFixture(t, false), alias)
		materializeError(t, root, err, "validation")
		materializeEmpty(t, sibling)
		if _, err := os.Readlink(alias); err != nil {
			t.Fatal("input symlink changed")
		}
	})
	t.Run("cancelled before effects", func(t *testing.T) {
		dest := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		root, err := MaterializeNodeBundle(ctx, materializeFixture(t, false), dest)
		materializeError(t, root, err, "network")
		materializeEmpty(t, dest)
	})
	t.Run("cancellation after file creation cleans only owned child", func(t *testing.T) {
		dest := t.TempDir()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		watch := &materializeCancelContext{Context: ctx, cancel: cancel, file: filepath.Join(dest, nodeBundlePrefix, "LICENSE")}
		root, err := MaterializeNodeBundle(watch, materializeFixture(t, false), dest)
		materializeError(t, root, err, "network")
		materializeEmpty(t, dest)
		if !watch.observed {
			t.Fatal("no post-effect cancellation exercised")
		}
	})
}

// A real Context cancels when an observable fixture file has been written.
// No production filesystem hooks or timing/race probes are needed.
type materializeCancelContext struct {
	context.Context
	cancel   context.CancelFunc
	file     string
	observed bool
}

func (c *materializeCancelContext) Err() error {
	if st, err := os.Stat(c.file); err == nil && st.Size() > 0 {
		c.observed = true
		c.cancel()
	}
	return c.Context.Err()
}
