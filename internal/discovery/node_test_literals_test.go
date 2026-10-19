package discovery_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/evidence"
)

type nodeTestCase struct{ command, normalized, kind string }

func TestStaticNodeTest(t *testing.T) {
	cases := []nodeTestCase{
		{"node --test test/*.test.mjs", "node --test test/*.test.mjs", "test"},
		{"node --test --test-concurrency=1 test/*.test.mjs", "node --test --test-concurrency=1 test/*.test.mjs", "test"},
		{" node\t--test  --test-concurrency=2147483647 ./test/a*b.test.mjs ", "node --test --test-concurrency=2147483647 ./test/a*b.test.mjs", "test"},
		{"node --test", "node --test", "test"},
		{"node --test test/a.test.mjs", "node --test test/a.test.mjs", "test"},
		{"go test ./...", "go test ./...", "test"},
		{"tsc --noEmit", "tsc --noEmit", "build"},
	}
	rejected := []string{
		"node --test --test-concurrency=0 test/*.test.mjs", "node --test --test-concurrency=-1 test/*.test.mjs",
		"node --test --test-concurrency=2147483648 test/*.test.mjs", "node --test --test-concurrency=999999999999999999999 test/*.test.mjs",
		"node --test --test-concurrency=1.5 test/*.test.mjs", "node --test --test-concurrency=+1 test/*.test.mjs",
		"node --test --test-concurrency=1 --test-concurrency=2 test/*.test.mjs", "node --test --test-concurrency 1 test/*.test.mjs",
		"node --test --watch test/*.test.mjs", "NODE_ENV=test node --test test/*.test.mjs",
		"node --test test/**.mjs", "node --test test/*/*.mjs", "node --test */a.mjs",
		"node --test ../test/*.mjs", "node --test ./../test/*.mjs", "node --test test/../*.mjs",
		"node --test /test/*.mjs", "node --test C:/test/*.mjs", "node --test test\\*.mjs",
		"node --test 'test/*.mjs'", "node --test test/?.mjs", "node --test test/[a].mjs",
		"node --test $(echo test/*.mjs)", "node --test test/*.mjs;echo", "node --test test/*.mjs | cat",
		"node\u00a0--test test/*.mjs", "node --test\ntest/*.mjs", "node --test\x00test/*.mjs",
		"node esbuild.config.mjs", "node esbuild.config.mjs --watch", "npx lockfile-lint",
		"jest test/*.mjs", "go test test/*.go", "node --test test/*.mjs && node esbuild.config.mjs",
	}
	for _, command := range rejected {
		cases = append(cases, nodeTestCase{command, "", ""})
	}
	assertNodeTestInventory(t, cases)
}

func TestStaticNodeTestLegacy(t *testing.T) {
	// These safe spellings were accepted by the original literal-token/leaf rules.
	// Keep their exact command spelling, not a cleaned filesystem path.
	cases := []nodeTestCase{}
	for _, p := range []string{".", "./", "test/", "test//child.test.mjs", "test/./child.test.mjs", "./-test.mjs"} {
		command := "node --test " + p
		cases = append(cases, nodeTestCase{command, command, "test"})
	}
	for _, p := range []string{"../", "./../", "test/../child.test.mjs", "test//../child.test.mjs", "./test/../../child.test.mjs", "/test/", "//test/", "C:/test/", "test\\child.test.mjs", "$(pwd)/", "test/;echo", "'test/'"} {
		cases = append(cases, nodeTestCase{"node --test " + p, "", ""})
	}
	assertNodeTestInventory(t, cases)
}

func assertNodeTestInventory(t *testing.T, cases []nodeTestCase) {
	t.Helper()
	for _, tt := range cases {
		t.Run(tt.command, func(t *testing.T) {
			// A neutral definition and a reference prove classification is not script-name based.
			scripts := map[string]string{"check": tt.command, "test": "npm run check"}
			content, err := json.Marshal(map[string]any{"scripts": scripts})
			if err != nil {
				t.Fatal(err)
			}
			before := bytes.Clone(content)
			snapshot := discovery.Snapshot{Documents: []discovery.Document{{Path: "plugin/package.json", Content: content}}}
			options := discovery.DefaultOptions()
			originalOptions := options
			got := discovery.Analyze(snapshot, options)
			rootID := evidence.StableID("root", "plugin", "npm")
			rootSource := discovery.Source{Path: "plugin/package.json"}
			want := discovery.Result{
				Roots:      []discovery.Root{{ID: rootID, Path: "plugin", Stack: "npm", Source: rootSource}},
				Tools:      []discovery.Tool{{ID: evidence.StableID("tool", rootID, "npm", rootSource.Path, ""), Root: rootID, Name: "npm", Source: rootSource, State: "CONFIGURED"}},
				Candidates: []discovery.Candidate{}, Diagnostics: []discovery.Diagnostic{}, Limitations: []string{},
			}
			for _, name := range []string{"check", "test"} {
				src := discovery.Source{Path: rootSource.Path, Symbol: name}
				chain := []discovery.Source{src}
				if name == "test" {
					chain = append(chain, discovery.Source{Path: rootSource.Path, Symbol: "check"})
				}
				candidate := discovery.Candidate{ID: evidence.StableID("candidate", rootID, name), Root: rootID, Kind: "unknown", Source: src, Chain: chain, State: "UNKNOWN", Origin: "SCRIPT"}
				if tt.kind != "" {
					candidate.ID = evidence.StableID("candidate", rootID, name, tt.normalized, "")
					candidate.Kind, candidate.Command, candidate.State = tt.kind, tt.normalized, "CONFIGURED"
					tool := strings.Fields(tt.normalized)[0]
					want.Tools = append(want.Tools, discovery.Tool{ID: evidence.StableID("tool", rootID, tool, src.Path, src.Symbol), Root: rootID, Name: tool, Source: src, State: "CONFIGURED"})
				} else {
					want.Diagnostics = append(want.Diagnostics, discovery.Diagnostic{Code: "unsupported_script", Path: rootSource.Path, Message: "Static discovery could not resolve this input.", State: "UNKNOWN"})
				}
				want.Candidates = append(want.Candidates, candidate)
			}
			// Compare complete observable inventories without depending on ID sort order.
			sort.Slice(want.Tools, func(i, j int) bool { return want.Tools[i].ID < want.Tools[j].ID })
			sort.Slice(want.Candidates, func(i, j int) bool { return want.Candidates[i].ID < want.Candidates[j].ID })
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("inventory mismatch\ngot: %#v\nwant: %#v", got, want)
			}
			if !bytes.Equal(content, before) || !reflect.DeepEqual(options, originalOptions) || len(snapshot.Documents) != 1 {
				t.Fatal("input or counters mutated")
			}
			if again := discovery.Analyze(snapshot, options); !reflect.DeepEqual(again, got) {
				t.Fatal("analysis is not repeatable")
			}
		})
	}
}
