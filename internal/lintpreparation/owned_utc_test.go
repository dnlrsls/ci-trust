package lintpreparation

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func ownedUTCFixture(data []byte) Tree {
	// Names/kinds/modes match authenticated base metadata; bytes are synthetic.
	return Tree{
		"etc":                          {Kind: "dir", Mode: 0555},
		"usr":                          {Kind: "dir", Mode: 0555},
		"usr/share":                    {Kind: "dir", Mode: 0555},
		"usr/share/zoneinfo":           {Kind: "dir", Mode: 0555},
		"usr/share/zoneinfo/Etc":       {Kind: "dir", Mode: 0555},
		"usr/share/zoneinfo/Etc/UTC":   {Kind: "file", Mode: 0444, Data: data},
		"usr/share/zoneinfo/localtime": {Kind: "link", Mode: 0777, Target: "/etc/localtime"},
	}
}

func TestOwnedUTCConfiguration(t *testing.T) {
	base := ownedUTCFixture([]byte("TZif synthetic UTC fixture"))
	bundle, launcher, module := seedBundle(t), []byte("l"), []byte("m")
	unchanged := seedSnapshot(base, bundle, launcher, module)
	target := installedFixture(t, nil)
	got, err := buildOwnedRootfs(context.Background(), base, bundle, target, launcher, module)
	if err != nil || got == nil {
		t.Fatalf("standard timezone target missing: %v", err)
	}
	want := Node{Kind: "file", Mode: 0444, Data: base["usr/share/zoneinfo/Etc/UTC"].Data}
	if !reflect.DeepEqual(got["etc/localtime"], want) {
		t.Fatalf("owned timezone differs from readonly base UTC: %#v", got["etc/localtime"])
	}
	resolved, err := Resolve(got, "usr/share/zoneinfo/localtime")
	if err != nil || !reflect.DeepEqual(resolved, want) {
		t.Fatalf("standard timezone link unresolved: %v %#v", err, resolved)
	}
	for name, node := range base {
		if !reflect.DeepEqual(got[name], node) {
			t.Fatalf("existing base entry overwritten: %s", name)
		}
	}
	if _, err := Inventory(got); err != nil {
		t.Fatal("invalid final tree", err)
	}
	if _, err := os.Stat(filepath.Join(target, "etc", "localtime")); !os.IsNotExist(err) {
		t.Fatal("timezone was materialized on the host", err)
	}
	unchanged(t)
}

func TestOwnedUTCPreservesExistingAndUnrelatedLinks(t *testing.T) {
	for _, kind := range []string{"existing", "no-alias", "unrelated", "wrong-target"} {
		t.Run(kind, func(t *testing.T) {
			base := ownedUTCFixture([]byte("UTC fixture"))
			existing := Node{Kind: "file", Mode: 0444, Data: []byte("existing timezone fixture")}
			switch kind {
			case "existing":
				base["etc/localtime"] = existing
			case "no-alias":
				delete(base, "usr/share/zoneinfo/localtime")
			case "unrelated":
				base["other-link"] = Node{Kind: "link", Mode: 0777, Target: "/missing"}
			case "wrong-target":
				base["usr/share/zoneinfo/localtime"] = Node{Kind: "link", Mode: 0777, Target: "/missing"}
			}
			b, l, m := seedBundle(t), []byte("l"), []byte("m")
			unchanged := seedSnapshot(base, b, l, m)
			got, err := buildOwnedRootfs(context.Background(), base, b, installedFixture(t, nil), l, m)
			if kind == "unrelated" || kind == "wrong-target" {
				if err == nil || got != nil {
					t.Fatal("arbitrary dangling link accepted")
				}
				assertRootfsReason(t, err, "rootfs-missing")
			} else {
				if err != nil || got == nil {
					t.Fatal("valid configuration rejected", err)
				}
				if kind == "existing" && !reflect.DeepEqual(got["etc/localtime"], existing) {
					t.Fatal("existing configuration overwritten")
				}
				if _, ok := got["etc/localtime"]; kind == "no-alias" && ok {
					t.Fatal("unrequested timezone created")
				}
			}
			unchanged(t)
		})
	}
}

func TestOwnedUTCSourceAndNamespaceRefusals(t *testing.T) {
	for _, kind := range []string{"missing", "directory", "link", "executable", "parent-file", "descendant"} {
		t.Run(kind, func(t *testing.T) {
			base := ownedUTCFixture([]byte("UTC fixture"))
			reason := "rootfs-timezone"
			switch kind {
			case "missing":
				delete(base, "usr/share/zoneinfo/Etc/UTC")
			case "directory":
				base["usr/share/zoneinfo/Etc/UTC"] = Node{Kind: "dir", Mode: 0555}
			case "link":
				base["usr/share/zoneinfo/Etc/UTC"] = Node{Kind: "link", Mode: 0777, Target: "/elsewhere"}
			case "executable":
				base["usr/share/zoneinfo/Etc/UTC"] = Node{Kind: "file", Mode: 0555, Data: []byte("inert executable fixture")}
			case "parent-file":
				base["etc"] = Node{Kind: "file", Mode: 0444}
				reason = "rootfs-seed-parent"
			case "descendant":
				base["etc/localtime/child"] = Node{Kind: "file", Mode: 0444}
				reason = "rootfs-seed-descendant"
			}
			b, l, m := seedBundle(t), []byte("l"), []byte("m")
			unchanged := seedSnapshot(base, b, l, m)
			got, err := seedOwnedRootfs(context.Background(), base, b, l, m)
			if err == nil || got != nil {
				t.Fatal("unsafe timezone source/namespace accepted")
			}
			assertRootfsReason(t, err, reason)
			unchanged(t)
		})
	}
}

func TestOwnedUTCBudgets(t *testing.T) {
	for _, size := range []int{1, 1024} {
		base := ownedUTCFixture(bytes.Repeat([]byte("u"), size))
		b, l, m := seedBundle(t), []byte("l"), []byte("m")
		unchanged := seedSnapshot(base, b, l, m)
		out, err := seedOwnedRootfs(context.Background(), base, b, l, m)
		if err != nil {
			t.Fatal(err)
		}
		var payload, metadata int64
		for name, node := range out {
			payload += int64(len(node.Data))
			metadata += int64(len(name) + len(node.Target))
		}
		bound := max(payload, metadata)
		for _, tc := range []struct {
			files int
			bytes int64
			ok    bool
		}{
			{len(out), bound, true}, {len(out) - 1, bound, false}, {len(out), bound - 1, false},
		} {
			got, err := seedOwnedRootfsWith(context.Background(), base, b, l, m, tc.files, tc.bytes)
			if tc.ok {
				if err != nil || got == nil {
					t.Fatalf("exact budget rejected: %v", err)
				}
				if _, ok := got["etc/localtime"]; !ok {
					t.Fatal("timezone missing at exact limit")
				}
			} else {
				if err == nil || got != nil {
					t.Fatal("timezone escaped shared budgets")
				}
				assertRootfsReason(t, err, "rootfs-limit")
			}
			unchanged(t)
		}
	}
}
