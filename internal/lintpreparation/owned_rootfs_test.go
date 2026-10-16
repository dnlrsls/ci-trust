package lintpreparation

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

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
			unchanged(t)
		})
	}
}

func TestOwnedSeedBansAndConflicts(t *testing.T) {
	cases := map[string]Tree{}
	for _, ban := range strings.Fields("sh bash dash npm npx gcc cc python python3") {
		for _, kind := range []string{"file", "dir", "link"} {
			cases[ban+kind] = Tree{"usr": {Kind: "dir", Mode: 0555}, "usr/" + ban: {Kind: kind, Mode: 0555}}
		}
	}
	for _, p := range []string{"owned/node/bin/node", "owned/node/LICENSE", "ci-trust/owned-eslint", "owned/eslint.config.mjs"} {
		for _, kind := range []string{"file", "dir", "link"} {
			cases[p+kind] = Tree{p: {Kind: kind, Mode: 0555}}
		}
		cases[p+"/descendant"] = Tree{p + "/child": {Kind: "file", Mode: 0444, Data: []byte("child")}}
	}
	for _, p := range []string{"owned", "owned/node", "owned/node/bin", "ci-trust"} {
		for _, kind := range []string{"file", "link"} {
			cases[p+kind] = Tree{p: {Kind: kind, Mode: 0555}}
		}
	}
	for name, base := range cases {
		base["keep"] = Node{Kind: "dir", Mode: 0555}
		for p, n := range base {
			if n.Kind == "link" {
				n.Mode, n.Target = 0777, "/keep"
			}
			if n.Kind == "file" {
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
