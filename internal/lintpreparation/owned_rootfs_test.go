package lintpreparation

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func installedFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	target := t.TempDir()
	if err := os.Mkdir(filepath.Join(target, "node_modules"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range files {
		p := filepath.Join(target, "node_modules", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return target
}

func TestOwnedImportExact(t *testing.T) {
	files := map[string]string{"pkg/index.js": "one", "pkg/empty": "", "@scope/pkg/lib/a.js": "two"}
	target := installedFixture(t, files)
	if err := os.MkdirAll(filepath.Join(target, "node_modules", "empty", "nested"), 0700); err != nil {
		t.Fatal(err)
	}
	base := Tree{"keep": {Kind: "file", Mode: 0444, Data: []byte("base")}, "owned/node_modules": {Kind: "dir", Mode: 0555}}
	b := seedBundle(t)
	l, m := []byte("l"), []byte("m")
	unchanged := seedSnapshot(base, b, l, m)
	want, err := seedOwnedRootfs(context.Background(), base, b, l, m)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"owned/node_modules", "owned/node_modules/pkg", "owned/node_modules/@scope", "owned/node_modules/@scope/pkg", "owned/node_modules/@scope/pkg/lib"} {
		want[p] = Node{Kind: "dir", Mode: 0555}
	}
	for p, data := range files {
		want["owned/node_modules/"+p] = Node{Kind: "file", Mode: 0444, Data: []byte(data)}
	}
	got, err := buildOwnedRootfs(context.Background(), base, b, target, l, m)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("import mismatch: %v %#v", err, got)
	}
	if _, err := Inventory(got); err != nil {
		t.Fatal(err)
	}
	delete(got, "keep")
	unchanged(t)
	empty, err := buildOwnedRootfs(context.Background(), Tree{}, b, installedFixture(t, nil), l, m)
	seed, seedErr := seedOwnedRootfs(context.Background(), Tree{}, b, l, m)
	if err != nil || seedErr != nil || !reflect.DeepEqual(empty, seed) {
		t.Fatal("empty installation emitted directories", err, seedErr)
	}
	for p, data := range files {
		actual, err := os.ReadFile(filepath.Join(target, "node_modules", filepath.FromSlash(p)))
		if err != nil || string(actual) != data {
			t.Fatal("source changed", err)
		}
	}
}

func TestOwnedImportRefusals(t *testing.T) {
	b := seedBundle(t)
	l, m := []byte("l"), []byte("m")
	for _, kind := range []string{"missing", "file", "missing-target", "relative", "symlink-file", "symlink-dir", "symlink-root", "symlink-target"} {
		t.Run(kind, func(t *testing.T) {
			target := installedFixture(t, map[string]string{"pkg/a": "a"})
			modules := filepath.Join(target, "node_modules")
			switch kind {
			case "missing":
				target = t.TempDir()
			case "file":
				target = t.TempDir()
				if err := os.WriteFile(filepath.Join(target, "node_modules"), []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-target":
				target = filepath.Join(target, "absent")
			case "relative":
				target = "relative-target"
			default:
				link, dest := filepath.Join(modules, "alias"), filepath.Join(modules, "pkg", "a")
				if kind == "symlink-dir" {
					dest = filepath.Join(modules, "pkg")
				}
				if kind == "symlink-root" {
					link = filepath.Join(t.TempDir(), "node_modules")
					dest = modules
					target = filepath.Dir(link)
				}
				if kind == "symlink-target" {
					link = filepath.Join(t.TempDir(), "target")
					dest = target
					target = link
				}
				if err := os.Symlink(dest, link); err != nil {
					t.Fatalf("required symlink fixture unavailable: %v", err)
				}
			}
			base := Tree{}
			unchanged := seedSnapshot(base, b, l, m)
			got, err := buildOwnedRootfs(context.Background(), base, b, target, l, m)
			if err == nil || got != nil {
				t.Fatalf("accepted invalid installed shape: %v %v", got, err)
			}
			reason := "rootfs-entry-type"
			switch kind {
			case "missing", "file", "symlink-root":
				reason = "rootfs-modules"
			case "missing-target", "relative", "symlink-target":
				reason = "rootfs-target"
			}
			assertRootfsReason(t, err, reason)
			unchanged(t)
		})
	}
	for _, p := range []string{"owned", "owned/node_modules", "owned/node_modules/pkg", "owned/node_modules/pkg/a", "owned/node_modules/pkg/a/child"} {
		t.Run(p, func(t *testing.T) {
			target := installedFixture(t, map[string]string{"pkg/a": "a"})
			base := Tree{p: {Kind: "file", Mode: 0444, Data: []byte("old")}}
			unchanged := seedSnapshot(base, b, l, m)
			got, err := buildOwnedRootfs(context.Background(), base, b, target, l, m)
			if err == nil || got != nil {
				t.Fatal("collision/ancestor/descendant accepted")
			}
			reason := "rootfs-parent-type"
			switch p {
			case "owned":
				reason = "rootfs-seed-parent"
			case "owned/node_modules/pkg/a":
				reason = "rootfs-entry-collision"
			case "owned/node_modules/pkg/a/child":
				reason = "rootfs-file-ancestor"
			}
			assertRootfsReason(t, err, reason)
			unchanged(t)
			source, err := os.ReadFile(filepath.Join(target, "node_modules", "pkg", "a"))
			if err != nil || string(source) != "a" {
				t.Fatal("source changed", err)
			}
		})
	}
}

func TestOwnedImportBudgetsAndGuards(t *testing.T) {
	target := installedFixture(t, map[string]string{"a": "abc", "b": "de"})
	if err := os.Mkdir(filepath.Join(target, "node_modules", "empty"), 0700); err != nil {
		t.Fatal(err)
	}
	base := Tree{}
	b := seedBundle(t)
	l, m := []byte("l"), []byte("m")
	unchanged := seedSnapshot(base, b, l, m)
	// Root + two files + one empty directory: deterministic visited accounting.
	limits := installedLimits{2, 4, 12, 5, 1024}
	for _, tc := range []struct {
		name   string
		limits installedLimits
		ok     bool
	}{
		{"exact", limits, true},
		{"file-one-over", installedLimits{1, 4, 12, 5, 1024}, false},
		{"bytes-one-over", installedLimits{2, 4, 12, 4, 1024}, false},
		{"visited-one-over", installedLimits{2, 3, 12, 5, 1024}, false},
		{"tree-files-one-over", installedLimits{2, 4, 11, 5, 1024}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := tc.limits
			got, err := buildOwnedRootfsWith(context.Background(), base, b, target, l, m, tc.limits)
			if tc.ok {
				if err != nil || got == nil {
					t.Fatalf("exact rejected: %v", err)
				}
			} else if err == nil || got != nil {
				t.Fatalf("budget accepted: %v %v", got, err)
			}
			if tc.limits != before {
				t.Fatal("caller limits changed")
			}
			unchanged(t)
		})
	}
	got, err := seedOwnedRootfs(context.Background(), base, b, l, m)
	if err != nil {
		t.Fatal(err)
	}
	var metadata int64
	for p, n := range got {
		metadata += int64(len(p) + len(n.Target))
	}
	for _, p := range []string{"owned/node_modules", "owned/node_modules/a", "owned/node_modules/b"} {
		metadata += int64(len(p))
	}
	for _, delta := range []int64{0, -1} {
		lim := limits
		lim.treeBytes = metadata + delta
		out, err := buildOwnedRootfsWith(context.Background(), base, b, target, l, m, lim)
		if delta == 0 {
			if err != nil || out == nil {
				t.Fatalf("metadata exact: %v", err)
			}
		} else if err == nil || out != nil {
			t.Error("global metadata limit ignored")
		}
	}
	// Payload-dominant global cap is distinct from the installed aggregate.
	large := bytes.Repeat([]byte{'m'}, 512)
	for _, cap := range []int64{523, 522} {
		lim := limits
		lim.treeBytes = cap
		out, err := buildOwnedRootfsWith(context.Background(), base, b, target, l, large, lim)
		if cap == 523 {
			if err != nil || out == nil {
				t.Fatalf("global payload exact: %v", err)
			}
		} else if err == nil || out != nil {
			t.Error("global payload limit ignored")
		}
	}
	probe := &seedCancelContext{Context: context.Background(), after: 100000}
	if _, err := seedOwnedRootfs(probe, base, b, l, m); err != nil {
		t.Fatal(err)
	}
	for _, after := range []int{1, probe.checks + 8} {
		ctx := &seedCancelContext{Context: context.Background(), after: after}
		out, err := buildOwnedRootfs(ctx, base, b, target, l, m)
		if out != nil || !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation: %v %v", out, err)
		}
		unchanged(t)
	}
	for p, want := range map[string]string{"a": "abc", "b": "de"} {
		data, err := os.ReadFile(filepath.Join(target, "node_modules", p))
		if err != nil || string(data) != want {
			t.Fatal("source changed after rejection", err)
		}
	}
	if !bytes.Equal(large, bytes.Repeat([]byte{'m'}, 512)) {
		t.Fatal("module changed")
	}
	for _, bundle := range []*NodeBundle{nil, {}} {
		out, err := buildOwnedRootfs(context.Background(), base, bundle, "", l, m)
		if out != nil || err == nil {
			t.Fatal("seal guard ignored")
		}
	}
	unchanged(t)
}

func seedBundle(t *testing.T) *NodeBundle {
	t.Helper()
	node := nodeHeader("bin/node", tar.TypeReg)
	node.Size = 2
	license := nodeHeader("LICENSE", tar.TypeReg)
	license.Size = 3
	b, err := decodeSyntheticNode(t, node, license)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func seedSnapshot(base Tree, b *NodeBundle, launcher, module []byte) func(*testing.T) {
	copyTree := Tree{}
	for k, v := range base {
		v.Data = bytes.Clone(v.Data)
		copyTree[k] = v
	}
	var copyBundle *NodeBundle
	if b != nil {
		copyBundle = &NodeBundle{sealed: b.sealed, entries: append([]nodeBundleEntry(nil), b.entries...)}
		for i := range copyBundle.entries {
			copyBundle.entries[i].data = bytes.Clone(copyBundle.entries[i].data)
		}
	}
	l, m := bytes.Clone(launcher), bytes.Clone(module)
	return func(t *testing.T) {
		t.Helper()
		if !reflect.DeepEqual(base, copyTree) || !reflect.DeepEqual(b, copyBundle) || !bytes.Equal(l, launcher) || !bytes.Equal(m, module) {
			t.Fatal("input mutated")
		}
	}
}

func assertRootfsReason(t *testing.T, err error, reason string) {
	t.Helper()
	wrapped := InStage("rootfs", func() error { return err })
	want := "owned lint preparation failed; stage=rootfs; category=validation; reason=" + reason + "; no runtime admission or ESLint execution"
	if got := Diagnostic(wrapped); got != want {
		t.Fatalf("diagnostic = %q; want %q", got, want)
	}
}

func TestOwnedSeedExact(t *testing.T) {
	for _, work := range []Node{{Kind: "dir", Mode: 0555}, {Kind: "file", Mode: 0444, Data: []byte("old")}, {Kind: "link", Mode: 0777, Target: "lib"}} {
		t.Run(work.Kind, func(t *testing.T) {
			base := Tree{"lib": {Kind: "dir", Mode: 0555}, "lib/keep": {Kind: "file", Mode: 0444, Data: []byte("base")}, "work": work, "owned": {Kind: "dir", Mode: 0555}, "owned/node": {Kind: "dir", Mode: 0555}}
			b, l, m := seedBundle(t), []byte("launcher"), []byte("module")
			unchanged := seedSnapshot(base, b, l, m)
			got, err := seedOwnedRootfs(context.Background(), base, b, l, m)
			if err != nil {
				t.Fatal(err)
			}
			want := Tree{"lib": base["lib"], "lib/keep": base["lib/keep"]}
			for _, p := range []string{"owned", "owned/node", "owned/node/bin", "ci-trust", "work"} {
				want[p] = Node{Kind: "dir", Mode: 0555}
			}
			for p, n := range map[string]Node{
				"owned/node/bin/node":     {Kind: "file", Mode: 0555, Data: []byte("xx")},
				"owned/node/LICENSE":      {Kind: "file", Mode: 0444, Data: []byte("xxx")},
				"ci-trust/owned-eslint":   {Kind: "file", Mode: 0555, Data: l},
				"owned/eslint.config.mjs": {Kind: "file", Mode: 0444, Data: m},
			} {
				want[p] = n
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("seed mismatch: %#v", got)
			}
			if _, err := Inventory(got); err != nil {
				t.Fatal(err)
			}
			delete(got, "lib/keep") // output map must not alias the caller's map.
			unchanged(t)
		})
	}
}

func TestOwnedSeedGuards(t *testing.T) {
	cases := []struct {
		name   string
		change func(*NodeBundle) *NodeBundle
	}{
		{"nil", func(b *NodeBundle) *NodeBundle { return nil }},
		{"unsealed", func(b *NodeBundle) *NodeBundle { b.sealed = false; return b }},
		{"empty", func(b *NodeBundle) *NodeBundle { b.entries = nil; return b }},
	}
	for _, required := range []string{"bin/node", "LICENSE"} {
		for _, kind := range []byte{0, tar.TypeDir, tar.TypeSymlink} {
			name, kind := required, kind
			cases = append(cases, struct {
				name   string
				change func(*NodeBundle) *NodeBundle
			}{name + "/" + string(rune(kind)), func(b *NodeBundle) *NodeBundle {
				for i, e := range b.entries {
					if e.name == nodeBundlePrefix+"/"+name {
						if kind == 0 {
							b.entries = append(b.entries[:i], b.entries[i+1:]...)
						} else {
							b.entries[i].kind = kind
						}
						break
					}
				}
				return b
			}})
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := tc.change(seedBundle(t))
			base := Tree{}
			l, m := []byte("l"), []byte("m")
			unchanged := seedSnapshot(base, b, l, m)
			got, err := seedOwnedRootfs(context.Background(), base, b, l, m)
			if err == nil || got != nil {
				t.Fatalf("guard returned partial/success: %v %v", got, err)
			}
			reason := "rootfs-bundle"
			if strings.HasPrefix(tc.name, "bin/node/") {
				reason = "rootfs-node"
			} else if strings.HasPrefix(tc.name, "LICENSE/") {
				reason = "rootfs-license"
			}
			assertRootfsReason(t, err, reason)
			unchanged(t)
		})
	}
}

func TestOwnedSeedBannedBasenameDirectories(t *testing.T) {
	// These two directory names and modes were observed in the authenticated
	// pinned base; the child payload is an inert synthetic fixture, not image code.
	base := Tree{
		"usr":                               {Kind: "dir", Mode: 0555},
		"usr/share":                         {Kind: "dir", Mode: 0555},
		"usr/share/gcc":                     {Kind: "dir", Mode: 0555},
		"usr/share/gcc/python":              {Kind: "dir", Mode: 0555},
		"usr/share/gcc/python/metadata.txt": {Kind: "file", Mode: 0444, Data: []byte("fixture metadata")},
	}
	// The rule is type-aware, not an exception for two hard-coded image paths.
	for _, name := range strings.Fields("sh bash dash npm npx gcc cc python python3") {
		base["usr/share/"+name] = Node{Kind: "dir", Mode: 0555}
	}
	b, l, m := seedBundle(t), []byte("launcher"), []byte("module")
	unchanged := seedSnapshot(base, b, l, m)
	got, err := seedOwnedRootfs(context.Background(), base, b, l, m)
	if err != nil || got == nil {
		t.Fatalf("normalized directory mistaken for forbidden executable: %v", err)
	}
	for name, want := range base {
		if !reflect.DeepEqual(got[name], want) {
			t.Fatalf("base entry changed: %s: %#v", name, got[name])
		}
	}
	if _, err := Inventory(got); err != nil {
		t.Fatalf("allowed directory seed is not a valid image tree: %v", err)
	}
	unchanged(t)
}

func TestOwnedSeedBansAndConflicts(t *testing.T) {
	cases := map[string]Tree{}
	reasons := map[string]string{}
	for _, ban := range strings.Fields("sh bash dash npm npx gcc cc python python3") {
		for _, kind := range []string{"file", "link"} {
			cases[ban+kind] = Tree{"usr": {Kind: "dir", Mode: 0555}, "usr/" + ban: {Kind: kind, Mode: 0555}}
			reasons[ban+kind] = "rootfs-base-banned"
		}
		cases[ban+"-executable"] = Tree{"usr": {Kind: "dir", Mode: 0555}, "usr/" + ban: {Kind: "file", Mode: 0555, Data: []byte("inert executable fixture")}}
		reasons[ban+"-executable"] = "rootfs-base-banned"
	}
	for _, p := range []string{"owned/node/bin/node", "owned/node/LICENSE", "ci-trust/owned-eslint", "owned/eslint.config.mjs"} {
		for _, kind := range []string{"file", "dir", "link"} {
			cases[p+kind] = Tree{p: {Kind: kind, Mode: 0555}}
			reasons[p+kind] = "rootfs-seed-collision"
		}
		cases[p+"/descendant"] = Tree{p + "/child": {Kind: "file", Mode: 0444, Data: []byte("child")}}
		reasons[p+"/descendant"] = "rootfs-seed-descendant"
	}
	for _, p := range []string{"owned", "owned/node", "owned/node/bin", "ci-trust"} {
		for _, kind := range []string{"file", "link"} {
			cases[p+kind] = Tree{p: {Kind: kind, Mode: 0555}}
			reasons[p+kind] = "rootfs-seed-parent"
		}
	}
	for name, base := range cases {
		base["keep"] = Node{Kind: "dir", Mode: 0555}
		for p, n := range base {
			if n.Kind == "link" {
				n.Mode, n.Target = 0777, "/keep"
			}
			if n.Kind == "file" && !strings.HasSuffix(name, "-executable") {
				n.Mode = 0444
			}
			base[p] = n
		}
		t.Run(name, func(t *testing.T) {
			b := seedBundle(t)
			l, m := []byte("l"), []byte("m")
			unchanged := seedSnapshot(base, b, l, m)
			got, err := seedOwnedRootfs(context.Background(), base, b, l, m)
			if err == nil || got != nil {
				t.Fatalf("conflict accepted: %v %v", got, err)
			}
			assertRootfsReason(t, err, reasons[name])
			unchanged(t)
		})
	}
}

// Deterministic cancellation after some assembly work, without timing or goroutines.
type seedCancelContext struct {
	context.Context
	checks, after int
}

func (c *seedCancelContext) Err() error {
	c.checks++
	if c.checks >= c.after {
		return context.Canceled
	}
	return nil
}

func TestOwnedSeedLimitsAndCancellation(t *testing.T) {
	base := Tree{"keep": {Kind: "file", Mode: 0444, Data: []byte("k")}}
	b := seedBundle(t)
	l, m := []byte("l"), []byte("m")
	unchanged := seedSnapshot(base, b, l, m)
	// Empty seed has 9 entries, 7 payload bytes with this synthetic bundle.
	// The retained base adds one entry and one byte; metadata is separately bounded.
	got, err := seedOwnedRootfs(context.Background(), base, b, l, m)
	if err != nil {
		t.Fatal(err)
	}
	var payload, metadata int64
	for p, n := range got {
		payload += int64(len(n.Data))
		metadata += int64(len(p) + len(n.Target))
	}
	limit := max(payload, metadata)
	for _, tc := range []struct {
		name  string
		files int
		bytes int64
		ctx   context.Context
		ok    bool
	}{
		{"exact", len(got), limit, context.Background(), true},
		{"file-one-over", len(got) - 1, limit, context.Background(), false},
		{"metadata-one-over", len(got), metadata - 1, context.Background(), false},
		{"canceled", len(got), limit, &seedCancelContext{Context: context.Background(), after: 1}, false},
		{"canceled-during", len(got), limit, &seedCancelContext{Context: context.Background(), after: len(b.entries) + 5}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := seedOwnedRootfsWith(tc.ctx, base, b, l, m, tc.files, tc.bytes)
			if tc.ok {
				if err != nil || out == nil {
					t.Fatalf("exact rejected: %v", err)
				}
			} else if err == nil || out != nil {
				t.Fatalf("limit/cancel returned partial/success: %v %v", out, err)
			}
			if strings.HasPrefix(tc.name, "canceled") && !errors.Is(err, context.Canceled) {
				t.Fatalf("wrong cancellation error: %v", err)
			}
			unchanged(t)
		})
	}
	// Tiny payload-dominant exact and one-over limits, not a large allocation probe.
	large := bytes.Repeat([]byte{'l'}, 512)
	out, err := seedOwnedRootfsWith(context.Background(), base, b, large, m, 100, 519)
	if err != nil || out == nil {
		t.Fatalf("payload exact: %v", err)
	}
	out, err = seedOwnedRootfsWith(context.Background(), base, b, large, m, 100, 518)
	if err == nil || out != nil {
		t.Fatal("payload one-over accepted")
	}
	if !bytes.Equal(large, bytes.Repeat([]byte{'l'}, 512)) {
		t.Fatal("launcher changed")
	}
	unchanged(t)
}
