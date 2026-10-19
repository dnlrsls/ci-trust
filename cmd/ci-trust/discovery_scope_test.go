package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

func TestDiscoveryRepositoryBoundary(t *testing.T) {
	binary := buildPublicBinary(t)
	for _, directory := range []bool{false, true} {
		name := "file"
		if directory {
			name = "directory"
		}
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string]string{
				".git":                          "gitdir: missing\n",
				"odd/checkout/package.json":     `{"scripts":{"test":"touch sentinel"}}`,
				"packages/regular/package.json": `{"scripts":{"test":"go test ./..."}}`,
			}
			if directory {
				files["odd/checkout/.git/config"] = "inert"
			} else {
				files["odd/checkout/.git"] = "gitdir: missing\ntouch sentinel\n"
			}
			for p, content := range files {
				target := filepath.Join(root, filepath.FromSlash(p))
				if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := fixtureBytes(t, root)
			code, out, stderr := invokePublic(t, binary, nil, "audit", root, "--format", "json")
			wantErr := "DISCOVERY_UNRESOLVED: Static analysis could not resolve this input; no execution or enforcement is inferred.\nsnapshot-omissions: Static analysis could not resolve this input; no execution or enforcement is inferred.\nDECLARATION_UNKNOWN: Static analysis could not resolve this input; no execution or enforcement is inferred.\n"
			if code != 0 || string(stderr) != wantErr {
				t.Fatalf("exit=%d stdout=%s stderr=%q", code, out, stderr)
			}
			canonical, ds := evidence.Encode(out)
			if len(ds) != 0 || !bytes.Equal(canonical, out) {
				t.Fatalf("noncanonical report: %v\n%s", ds, out)
			}
			var report object
			if err := json.Unmarshal(out, &report); err != nil {
				t.Fatal(err)
			}
			// Check every serialized source, not merely command names shared by roots.
			var paths []string
			var visit func(any)
			visit = func(v any) {
				switch x := v.(type) {
				case map[string]any:
					if p, ok := x["path"].(string); ok {
						paths = append(paths, p)
					}
					for _, value := range x {
						visit(value)
					}
				case []any:
					for _, value := range x {
						visit(value)
					}
				}
			}
			visit(report)
			regular := false
			for _, p := range paths {
				if strings.HasPrefix(p, "odd/checkout/") {
					t.Fatalf("descendant source retained: %s\n%s", p, out)
				}
				regular = regular || p == "packages/regular/package.json"
			}
			if !regular || !bytes.Contains(out, []byte(`"code":"snapshot-omissions"`)) {
				t.Fatalf("missing package or omission: %s", out)
			}
			// Repeated audit must preserve complete canonical bytes and counters.
			c2, o2, e2 := invokePublic(t, binary, nil, "audit", root, "--format", "json")
			assertPublic(t, c2, o2, e2, 0, string(out), wantErr)
			unchangedFixture(t, root, before)
		})
	}
}

func TestDiscoveryRepositoryBoundaryBootstrap(t *testing.T) {
	binary := buildPublicBinary(t)
	for _, tc := range []struct {
		name, marker string
		proposals    int
	}{
		{"outside-destination", "checkout/.git", 1},
		{"destination-excluded", ".github/.git", 0},
		{"destination-itself-excluded", ".github/workflows/ci-trust.yml/.git", 0},
		{"ordinary-omission-still-blocked", "node_modules/marker", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := ownedBootstrapFixture(t)
			p := filepath.Join(root, filepath.FromSlash(tc.marker))
			if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("inert\n"), 0600); err != nil {
				t.Fatal(err)
			}
			before := fixtureBytes(t, root)
			code, out, stderr := invokePublic(t, binary, nil, "bootstrap", root, "--dry-run", "--format", "json")
			wantErr := "snapshot-omissions: Static analysis could not resolve this input; no execution or enforcement is inferred.\nDISCOVERY_UNRESOLVED: Static analysis could not resolve this input; no execution or enforcement is inferred.\n"
			if code != 0 || string(stderr) != wantErr {
				t.Fatalf("exit=%d stdout=%s stderr=%q", code, out, stderr)
			}
			plan := publicBootstrapPlan(t, out)
			if len(plan.Recommendations) != tc.proposals {
				t.Fatalf("proposals=%d want=%d; %s", len(plan.Recommendations), tc.proposals, out)
			}
			if tc.proposals > 0 && plan.Recommendations[0].Path != ".github/workflows/ci-trust.yml" {
				t.Fatal("wrong root destination")
			}
			c2, o2, e2 := invokePublic(t, binary, nil, "bootstrap", root, "--dry-run", "--format", "json")
			assertPublic(t, c2, o2, e2, 0, string(out), wantErr)
			unchangedFixture(t, root, before)
		})
	}
}
