package discovery

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seamWrite(t *testing.T, root, name, content string) {
	t.Helper()
	file := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func seamBytes(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestScanMixedRootsRemainStatic(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"backend/go.mod":            "module example.test/backend\n\ngo 1.26\n",
		"frontend/package.json":     `{"scripts":{"lint":"eslint .","unit":"node --test","test":"npm run unit","cycle":"npm run cycle","hostile":"node -e \"require('fs').writeFileSync('sentinel','bad')\""}}`,
		"frontend/eslint.config.js": `require('fs').writeFileSync('sentinel','bad'); throw Error('must-not-run');`,
		"counter":                   "0\n",
	}
	for name, content := range files {
		seamWrite(t, root, name, content)
	}
	got := Scan(root, Options{})
	roots := map[string]Root{}
	for _, item := range got.Roots {
		roots[item.Path] = item
	}
	if len(roots) != 2 || roots["backend"].ID == "" || roots["frontend"].ID == "" || roots["backend"].ID == roots["frontend"].ID {
		t.Fatalf("independent manifest roots missing: %+v", got.Roots)
	}
	if len(got.Candidates) != 7 {
		t.Fatalf("want two Go conventions, three configured and two unresolved entries: %+v", got.Candidates)
	}
	seen := map[string]bool{}
	conventions := map[string]bool{}
	scripts := map[string]bool{}
	for _, candidate := range got.Candidates {
		if candidate.ID == "" || seen[candidate.ID] {
			t.Fatal("candidate IDs must be stable and distinct")
		}
		seen[candidate.ID] = true
		if candidate.Root == roots["backend"].ID {
			if candidate.Origin != "CONVENTION" || candidate.State != "UNKNOWN" || candidate.Source.Path != "backend/go.mod" {
				t.Fatalf("Go convention promoted into configured invocation: %+v", candidate)
			}
			if candidate.Command != "go build ./..." && candidate.Command != "go test ./..." {
				t.Fatalf("invented Go command: %+v", candidate)
			}
			conventions[candidate.Command] = true
		} else if candidate.Root == roots["frontend"].ID {
			symbol := candidate.Source.Symbol
			if scripts[symbol] {
				t.Fatalf("duplicate script entry: %s", symbol)
			}
			scripts[symbol] = true
			if symbol == "cycle" || symbol == "hostile" {
				if candidate.State != "UNKNOWN" || candidate.Command != "" || candidate.Kind != "unknown" || candidate.Origin != "SCRIPT" || candidate.Source.Path != "frontend/package.json" {
					t.Fatalf("unresolved script promoted into a command: %+v", candidate)
				}
				continue
			}
			if symbol != "lint" && symbol != "unit" && symbol != "test" {
				t.Fatalf("invented script entry: %+v", candidate)
			}
			if candidate.Origin != "SCRIPT" || candidate.State != "CONFIGURED" || candidate.Source.Path != "frontend/package.json" {
				t.Fatalf("incorrect script provenance/state: %+v", candidate)
			}
			if candidate.Command != "eslint ." && candidate.Command != "node --test" {
				t.Fatalf("unsupported script became a candidate: %+v", candidate)
			}
			if candidate.Source.Symbol == "test" {
				if len(candidate.Chain) != 2 || candidate.Chain[0].Symbol != "test" || candidate.Chain[1].Symbol != "unit" {
					t.Fatalf("literal reference chain lost: %+v", candidate.Chain)
				}
			}
		} else {
			t.Fatalf("candidate lost root ownership: %+v", candidate)
		}
	}
	if len(conventions) != 2 || len(scripts) != 5 || len(got.Diagnostics) < 2 {
		t.Fatalf("missing configured/conventional entries or explicit uncertainty: %+v", got)
	}
	for _, diagnostic := range got.Diagnostics {
		if diagnostic.Code == "" || diagnostic.State != "UNKNOWN" || strings.Contains(diagnostic.Message, "must-not-run") {
			t.Fatalf("unsafe or unclassified diagnostic: %+v", diagnostic)
		}
	}
	serialized := seamBytes(t, got)
	if bytes.Contains(serialized, []byte(root)) || bytes.Contains(serialized, []byte("writeFileSync")) {
		t.Fatal("inventory leaked host path or rejected executable payload")
	}
	if repeated := seamBytes(t, Scan(root, Options{})); !bytes.Equal(serialized, repeated) {
		t.Fatal("repeated scan changed the complete inventory")
	}
	for name, content := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || string(data) != content {
			t.Fatalf("scan changed input %s: %v", name, err)
		}
	}
	for _, name := range []string{"sentinel", "frontend/sentinel", "backend/sentinel"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); !os.IsNotExist(err) {
			t.Fatalf("scan executed hostile input or sentinel status unavailable: %v", err)
		}
	}
}

func TestScanCarriesReadLimitsAndMissingRoot(t *testing.T) {
	root := t.TempDir()
	seamWrite(t, root, "go.mod", "module example.test/oversized\n")
	for _, scenario := range []struct {
		name string
		path string
		opts Options
	}{
		{"bounded file", root, Options{MaxFileBytes: 8}},
		{"missing root", filepath.Join(root, "missing"), Options{}},
		{"invalid limit", root, Options{MaxEntries: -1}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			got := Scan(scenario.path, scenario.opts)
			if len(got.Candidates) != 0 || len(got.Diagnostics) == 0 {
				t.Fatalf("read rejection not preserved in inventory: %+v", got)
			}
			for _, diagnostic := range got.Diagnostics {
				if diagnostic.Code == "" || diagnostic.State != "UNKNOWN" {
					t.Fatalf("read failure lost uncertainty: %+v", diagnostic)
				}
			}
			if bytes.Contains(seamBytes(t, got), []byte(root)) {
				t.Fatal("read failure leaked host path")
			}
		})
	}
}
