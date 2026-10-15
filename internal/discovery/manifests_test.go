package discovery

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

func TestAnalyzeExactBoundary(t *testing.T) {
	rootID := evidence.StableID("root", ".", "npm")
	src := Source{Path: "package.json", Symbol: "test"}
	rootSource := Source{Path: "package.json"}
	input := Snapshot{Documents: []Document{{Path: "package.json", Content: []byte(`{"scripts":{"test":"npm run lint","lint":"eslint ."}}`)}}}
	before := Snapshot{Documents: []Document{{Path: "package.json", Content: append([]byte(nil), input.Documents[0].Content...)}}}
	lint := Source{Path: "package.json", Symbol: "lint"}
	want := Result{
		Roots: []Root{{ID: rootID, Path: ".", Stack: "npm", Source: rootSource}},
		Tools: []Tool{
			{ID: evidence.StableID("tool", rootID, "npm", "package.json", ""), Root: rootID, Name: "npm", Source: rootSource, State: "CONFIGURED"},
			{ID: evidence.StableID("tool", rootID, "eslint", "package.json", "test"), Root: rootID, Name: "eslint", Source: src, State: "CONFIGURED"},
			{ID: evidence.StableID("tool", rootID, "eslint", "package.json", "lint"), Root: rootID, Name: "eslint", Source: lint, State: "CONFIGURED"},
		},
		Candidates: []Candidate{
			{ID: evidence.StableID("candidate", rootID, "test", "eslint .", ""), Root: rootID, Kind: "lint", Command: "eslint .", Source: src, Chain: []Source{src, lint}, State: "CONFIGURED", Origin: "SCRIPT"},
			{ID: evidence.StableID("candidate", rootID, "lint", "eslint .", ""), Root: rootID, Kind: "lint", Command: "eslint .", Source: lint, Chain: []Source{lint}, State: "CONFIGURED", Origin: "SCRIPT"},
		}, Diagnostics: []Diagnostic{}, Limitations: []string{},
	}
	sort.Slice(want.Tools, func(i, j int) bool { return want.Tools[i].ID < want.Tools[j].ID })
	sort.Slice(want.Candidates, func(i, j int) bool { return want.Candidates[i].ID < want.Candidates[j].ID })
	if got := Analyze(input, Options{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
	if !reflect.DeepEqual(input, before) {
		t.Fatal("input mutated")
	}
}

func TestAnalyzeEmpty(t *testing.T) {
	got := Analyze(Snapshot{}, Options{})
	want := Result{Roots: []Root{}, Tools: []Tool{}, Candidates: []Candidate{}, Diagnostics: []Diagnostic{}, Limitations: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}
func TestAnalyzeReferences(t *testing.T) {
	s := Snapshot{Documents: []Document{{Path: "web/package.json", Content: []byte(`{"scripts":{"check":"npm run unit && npm run build","unit":"go test ./...","build":"tsc --noEmit"}}`)}}}
	before := string(s.Documents[0].Content)
	got := Analyze(s, Options{})
	if len(got.Roots) != 1 || len(got.Candidates) != 4 {
		t.Fatalf("inventory: %#v", got)
	}
	for _, c := range got.Candidates {
		if c.State != "CONFIGURED" || c.Origin != "SCRIPT" || c.Root != got.Roots[0].ID || len(c.Chain) == 0 || c.Chain[0] != c.Source || strings.Contains(c.Command, "&&") || c.Kind == "composite" {
			t.Fatalf("candidate: %#v", c)
		}
	}
	if string(s.Documents[0].Content) != before {
		t.Fatal("mutated input")
	}
}
func TestAnalyzeUnknown(t *testing.T) {
	for _, script := range []string{"npm run missing", "npm run check", "npm run $NAME", "go test ./... | sh", "go test ./... && curl remote", "node evil.js", "X=1 go test ./...", "go test $(touch sentinel)", "go test > sentinel", "go test --made-up", "sh -c evil", "npm test -- --watch", "eslint .;evil", "go test /host/private"} {
		t.Run(script, func(t *testing.T) {
			got := Analyze(Snapshot{Documents: []Document{{Path: "package.json", Content: []byte(`{"scripts":{"check":"` + script + `"}}`)}}}, Options{})
			if len(got.Candidates) != 1 || got.Candidates[0].State != "UNKNOWN" || got.Candidates[0].Command != "" || len(got.Diagnostics) == 0 {
				t.Fatalf("unsafe result %#v", got)
			}
			for _, d := range got.Diagnostics {
				if strings.Contains(d.Message, script) {
					t.Fatal("script leaked")
				}
			}
		})
	}
}
func TestAnalyzeStrictJSON(t *testing.T) {
	for _, text := range []string{`{"scripts":{"test":"jest","test":"eslint ."}}`, `{"scripts":{},"scripts":{}}`, `{"ignored":{"x":1,"x":2}}`, `{} {}`, `[]`, `null`, `{"scripts":[]}`, `{"scripts":{"test":12}}`, `{"scripts":{"test":"jest"},}`} {
		t.Run(text, func(t *testing.T) {
			r := Analyze(Snapshot{Documents: []Document{{Path: "package.json", Content: []byte(text)}}}, Options{})
			if len(r.Candidates) != 0 || len(r.Diagnostics) == 0 {
				t.Fatalf("not rejected: %#v", r)
			}
		})
	}
	r := Analyze(Snapshot{Documents: []Document{{Path: "package.json", Content: []byte(`{"ignored":{"anything":[1,true,null]},"scripts":{"test":"node --test"}}`)}}}, Options{})
	if len(r.Candidates) != 1 || r.Candidates[0].Command != "node --test" || r.Candidates[0].Source.Symbol != "test" {
		t.Fatalf("irrelevant fields: %#v", r)
	}
}
func TestAnalyzeIndependentRootsAndOrdering(t *testing.T) {
	docs := []Document{{"go.mod", []byte("module example.org/repo\n\ngo 1.26\n")}, {"package.json", []byte(`{"scripts":{"test":"npm run unit","unit":"eslint ."}}`)}, {"nested/package.json", []byte(`{"scripts":{"test":"npm run unit"}}`)}, {"nested/go.mod", []byte("module nested\n")}, {"eslint.config.js", []byte("throw new Error('must not evaluate')")}, {"package-lock.json", []byte("not parsed")}, {"orphan/.golangci.yaml", []byte("hostile")}, {"nested/yarn.lock", nil}, {"Cargo.toml", nil}}
	s := Snapshot{Documents: docs}
	r := Analyze(s, Options{})
	if len(r.Roots) != 4 || len(r.Candidates) != 7 || len(r.Diagnostics) != 4 || len(r.Limitations) != 2 {
		t.Fatalf("inventory %#v", r)
	}
	for _, c := range r.Candidates {
		if c.Origin == "CONVENTION" && (c.State != "UNKNOWN" || len(c.Chain) != 1) {
			t.Fatalf("convention %#v", c)
		}
	}
	reversed := append([]Document(nil), docs...)
	for i, j := 0, len(reversed)-1; i < j; i, j = i+1, j-1 {
		reversed[i], reversed[j] = reversed[j], reversed[i]
	}
	if other := Analyze(Snapshot{Documents: reversed}, Options{}); !reflect.DeepEqual(r, other) {
		t.Fatalf("order changed: %#v", other)
	}
	if !reflect.DeepEqual(s.Documents, docs) {
		t.Fatal("input changed")
	}
}
func TestAnalyzeBudgets(t *testing.T) {
	s := Snapshot{Documents: []Document{{"package.json", []byte(`{"scripts":{"entry":"npm run mid && npm run leaf","mid":"npm run leaf","leaf":"jest"}}`)}}}
	for _, tc := range []struct {
		name, code string
		options    Options
	}{{"edges", "reference_limit", Options{MaxReferences: 1}}, {"depth", "script_depth_limit", Options{MaxScriptDepth: 1}}, {"file", "file_limit", Options{MaxFileBytes: 1}}, {"total", "total_limit", Options{MaxTotalBytes: 1}}, {"negative", "invalid_options", Options{MaxReferences: -1}}, {"edge cap", "invalid_options", Options{MaxReferences: 4097}}, {"depth cap", "invalid_options", Options{MaxScriptDepth: 65}}, {"byte cap", "invalid_options", Options{MaxFileBytes: 17 << 20}}} {
		t.Run(tc.name, func(t *testing.T) {
			r := Analyze(s, tc.options)
			found := false
			for _, d := range r.Diagnostics {
				found = found || d.Code == tc.code
			}
			if !found {
				t.Fatalf("missing %s: %#v", tc.code, r)
			}
		})
	}
}
func TestAnalyzeUnsafePathsAndCarry(t *testing.T) {
	for _, p := range []string{"/host/package.json", "../package.json", "C:/host/package.json", "x\\package.json", "a/../package.json"} {
		r := Analyze(Snapshot{Documents: []Document{{p, []byte(`{"scripts":{"test":"jest"}}`)}}, Diagnostics: []Diagnostic{{Code: "read_failure", Path: p, Message: "secret host payload"}}, Limitations: []string{"/host/secret"}}, Options{})
		if len(r.Roots) != 0 || len(r.Diagnostics) != 2 || len(r.Limitations) != 1 {
			t.Fatalf("unsafe %#v", r)
		}
		for _, d := range r.Diagnostics {
			if d.Path != "" || strings.Contains(d.Message, "secret") {
				t.Fatalf("leak %#v", d)
			}
		}
	}
}
func TestAnalyzeGoMalformedAndDuplicateDocuments(t *testing.T) {
	for _, text := range []string{"", "module", "module $(evil)", "module a\nmodule b", "console.log('evil')"} {
		r := Analyze(Snapshot{Documents: []Document{{"go.mod", []byte(text)}}}, Options{})
		if len(r.Candidates) != 0 || len(r.Diagnostics) != 1 || len(r.Limitations) != 1 {
			t.Fatalf("malformed %#v", r)
		}
	}
	r := Analyze(Snapshot{Documents: []Document{{"package.json", []byte(`{}`)}, {"package.json", []byte(`{}`)}, {"package.json", []byte(`{}`)}}}, Options{})
	if len(r.Roots) != 0 {
		t.Fatalf("ambiguous duplicates %#v", r)
	}
}
func TestAnalyzeReferenceDiagnostics(t *testing.T) {
	for _, tc := range []struct{ body, code string }{
		{`{"scripts":{"test":"npm test"}}`, "cyclic_reference"},
		{`{"scripts":{"a":"npm run b","b":"npm run a"}}`, "cyclic_reference"},
		{`{"scripts":{"test":"npm run absent"}}`, "missing_reference"},
		{`{"scripts":{"test":"npm run"}}`, "unsupported_reference"},
		{`{"scripts":{"test":"jest && npm run absent"}}`, "missing_reference"},
	} {
		r := Analyze(Snapshot{Documents: []Document{{"package.json", []byte(tc.body)}}}, Options{})
		for _, c := range r.Candidates {
			if c.Command != "" || c.State != "UNKNOWN" {
				t.Fatalf("partial resolution %#v", r)
			}
		}
		for _, d := range r.Diagnostics {
			if d.Code != tc.code || d.State != "UNKNOWN" || d.Path != "package.json" {
				t.Fatalf("diagnostic %#v", d)
			}
		}
		if len(r.Diagnostics) == 0 {
			t.Fatal("missing diagnostic")
		}
	}
}
func TestAnalyzeMarkerBounds(t *testing.T) {
	r := Analyze(Snapshot{Documents: []Document{{"eslint.config.js", []byte("hostile payload")}, {"package.json", []byte(`{}`)}}}, Options{MaxFileBytes: 2})
	if len(r.Tools) != 1 || r.Tools[0].Name != "npm" || len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != "file_limit" {
		t.Fatalf("unbounded marker %#v", r)
	}
	for _, tc := range []struct {
		docs    []Document
		options Options
		code    string
	}{
		{[]Document{{"a/b/package.json", []byte(`{}`)}}, Options{MaxDepth: 1}, "depth_limit"},
		{[]Document{{"a/package.json", []byte(`{}`)}, {"b/package.json", []byte(`{}`)}}, Options{MaxEntries: 1}, "entry_limit"},
	} {
		r := Analyze(Snapshot{Documents: tc.docs}, tc.options)
		if len(r.Diagnostics) != 1 || r.Diagnostics[0].Code != tc.code {
			t.Fatalf("bound %#v", r)
		}
	}
}

func TestAnalyzeLiteralLeaves(t *testing.T) {
	for _, cmd := range []string{"go test ./...", "go build ./...", "eslint .", "tsc --noEmit", "vitest run", "jest --runInBand", "node --test"} {
		r := Analyze(Snapshot{Documents: []Document{{"package.json", []byte(`{"scripts":{"test":"` + cmd + `"}}`)}}}, Options{})
		if len(r.Candidates) != 1 || r.Candidates[0].Command != cmd || r.Candidates[0].State != "CONFIGURED" {
			t.Fatalf("literal %s: %#v", cmd, r)
		}
	}
}
