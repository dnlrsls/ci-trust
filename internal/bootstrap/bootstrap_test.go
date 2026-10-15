package bootstrap_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/bootstrap"
	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/evidence"
	"github.com/dnlrsls/ci-trust/internal/workflows"
)

func fixture() discovery.Snapshot {
	return discovery.Snapshot{Documents: []discovery.Document{
		{Path: "package.json", Content: []byte(`{"scripts":{"test":"node --test"}}`)},
		{Path: ".ci-trust.json", Content: []byte(`{"version":1,"platforms":["linux","windows","macos"]}`)},
	}}
}
func TestAnalyzeConditionalDraft(t *testing.T) {
	s := fixture()
	before, _ := json.Marshal(s)
	p, err := bootstrap.Analyze(s, bootstrap.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Recommendations) != 1 {
		t.Fatalf("supported empty CI requires one conditional draft; got %d", len(p.Recommendations))
	}
	if !p.DryRun || p.Version != bootstrap.Version || len(evidence.Validate(p.SourceReport)) != 0 {
		t.Fatal("invalid protocol/report")
	}
	r := p.Recommendations[0]
	if r.Path != ".github/workflows/ci-trust.yml" || len(r.Preconditions) < 3 || r.Conflicts == nil || r.Limitations == nil {
		t.Fatal("missing conditional contract")
	}
	for _, v := range []string{"npm run test", "timeout-minutes: 10", "contents: read", "persist-credentials: false", "actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803"} {
		if !strings.Contains(r.Content, v) {
			t.Fatalf("missing %s", v)
		}
	}
	for _, v := range []string{"setup-go", "setup-node", "npm install", "npm ci", "go-version:"} {
		if strings.Contains(r.Content, v) {
			t.Fatalf("invented setup: %s", v)
		}
	}
	// References must resolve to the authoritative report's unlinked finding and evidence.
	var report struct {
		Findings []struct {
			ID   string
			Rule string   `json:"rule_id"`
			Refs []string `json:"evidence_refs"`
		}
		Evidence []struct{ ID string }
	}
	if err := json.Unmarshal(p.SourceReport, &report); err != nil {
		t.Fatal(err)
	}
	fm := map[string]bool{}
	em := map[string]bool{}
	for _, f := range report.Findings {
		if f.Rule == "CIT_COMMAND_UNLINKED" {
			fm[f.ID] = true
			for _, ref := range f.Refs {
				em[ref] = true
			}
		}
	}
	for _, id := range r.FindingRefs {
		if !fm[id] {
			t.Fatalf("wrong source finding %s", id)
		}
	}
	if len(r.FindingRefs) == 0 || len(r.EvidenceRefs) == 0 {
		t.Fatal("missing traceability")
	}
	all := map[string]bool{}
	for _, e := range report.Evidence {
		all[e.ID] = true
	}
	matched := false
	for _, id := range r.EvidenceRefs {
		if !all[id] {
			t.Fatalf("dangling evidence %s", id)
		}
		matched = matched || em[id]
	}
	if !matched {
		t.Fatal("unlinked source evidence absent")
	}
	again, err := bootstrap.Analyze(s, bootstrap.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, again) {
		t.Fatal("nondeterministic")
	}
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("snapshot mutated")
	}
	golden(t, "conditional.yml", r.Content)
	golden(t, "conditional.diff", r.Diff)
	// Reanalysis, not execution: all three literal runners and exact npm links.
	s.Documents = append(s.Documents, discovery.Document{Path: r.Path, Content: []byte(r.Content)})
	graph := workflows.Analyze(s, discovery.Analyze(s, discovery.Options{}), workflows.Options{})
	if len(graph.Workflows) != 1 || len(graph.Workflows[0].Jobs) != 3 {
		t.Fatalf("unexpected graph %#v", graph)
	}
	runners := map[string]bool{}
	for _, j := range graph.Workflows[0].Jobs {
		runners[j.Runner.Raw] = true
		linked := false
		for _, st := range j.Steps {
			if st.Run == "npm run test" && len(st.Links) == 1 && st.Links[0].Command == "node --test" {
				linked = true
			}
		}
		if !linked {
			t.Fatal("generated YAML lost exact command/root linkage")
		}
	}
	for _, v := range []string{"ubuntu-latest", "windows-latest", "macos-latest"} {
		if !runners[v] {
			t.Fatalf("missing runner %s", v)
		}
	}
	second, err := bootstrap.Analyze(s, bootstrap.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Recommendations) != 0 {
		t.Fatal("duplicate equivalent draft")
	}
}
func golden(t *testing.T, name, got string) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != got {
		t.Fatalf("golden %s differs", name)
	}
}

func TestAnalyzeLimitsAndOptions(t *testing.T) {
	cases := []struct {
		name    string
		s       discovery.Snapshot
		o       bootstrap.Options
		wantErr bool
	}{
		{name: "no sources"},
		{name: "convention", s: discovery.Snapshot{Documents: []discovery.Document{{Path: "go.mod", Content: []byte("module example.test/m\n\ngo 1.24\n")}, fixture().Documents[1]}}},
		{name: "unsupported", s: discovery.Snapshot{Documents: []discovery.Document{{Path: "package.json", Content: []byte(`{"scripts":{"test":"echo fine"}}`)}, fixture().Documents[1]}}},
		{name: "unresolved", s: discovery.Snapshot{Documents: []discovery.Document{{Path: "package.json", Content: []byte(`{"scripts":{"test":"npm run missing"}}`)}, fixture().Documents[1]}}},
		{name: "no declaration", s: discovery.Snapshot{Documents: fixture().Documents[:1]}},
		{name: "invalid declaration", s: discovery.Snapshot{Documents: []discovery.Document{fixture().Documents[0], {Path: ".ci-trust.json", Content: []byte(`{"Version":1,"platforms":["linux"]}`)}}}},
		{name: "unsafe collection", s: discovery.Snapshot{Documents: fixture().Documents, Diagnostics: []discovery.Diagnostic{{Code: "SYMLINK_SKIPPED", Path: ".github"}}}},
		{name: "partial", s: discovery.Snapshot{Documents: fixture().Documents, Limitations: []string{"omitted"}}},
		{name: "bad option", s: fixture(), o: bootstrap.Options{Platforms: []string{"Solaris"}}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, err := bootstrap.Analyze(tc.s, tc.o)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error %v", err)
			}
			if err == nil && (len(p.Recommendations) != 0 || len(p.Limitations) == 0 || p.Recommendations == nil) {
				t.Fatal("unsupported input must remain limited")
			}
		})
	}
	p, err := bootstrap.Analyze(discovery.Snapshot{Documents: fixture().Documents[:1]}, bootstrap.Options{Platforms: []string{"windows", "windows"}})
	if err != nil || len(p.Recommendations) != 1 {
		t.Fatalf("explicit override: %v %#v", err, p)
	}
	if !strings.Contains(p.Recommendations[0].Content, "windows-latest") || strings.Contains(p.Recommendations[0].Content, "ubuntu-latest") {
		t.Fatal("override platforms changed")
	}
}

func TestConfiguredGoGap(t *testing.T) {
	s := discovery.Snapshot{Documents: []discovery.Document{fixture().Documents[1], {Path: "go.mod", Content: []byte("module example.test/m\n\ngo 1.24\n")}, {Path: ".github/workflows/go.yml", Content: []byte("on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: go test ./...\n")}}}
	p, err := bootstrap.Analyze(s, bootstrap.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Recommendations) != 1 {
		t.Fatalf("configured Go gap: %#v", p.Limitations)
	}
	r := p.Recommendations[0]
	if !strings.Contains(r.Content, "go test ./...") || strings.Contains(r.Content, "ubuntu-latest") || strings.Contains(r.Content, "1.24") {
		t.Fatal("Go draft must use actual command and only gap platforms, without copied runtime")
	}
	if len(r.FindingRefs) != 2 {
		t.Fatalf("expected two platform gap finding refs, got %v", r.FindingRefs)
	}
}

func TestConflictAndEquivalentExisting(t *testing.T) {
	s := fixture()
	s.Documents = append(s.Documents, discovery.Document{Path: ".github/workflows/ci-trust.yml", Content: []byte("name: Existing\non: push\njobs:\n  other:\n    runs-on: ubuntu-latest\n    steps:\n      - run: echo unchanged\n")})
	before, _ := json.Marshal(s)
	p, err := bootstrap.Analyze(s, bootstrap.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Recommendations) != 1 || len(p.Recommendations[0].Conflicts) != 1 || !strings.Contains(p.Recommendations[0].Diff, "-name: Existing") {
		t.Fatal("occupied destination must conflict")
	}
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("existing workflow mutated")
	}
	s = fixture()
	s.Documents[1].Content = []byte(`{"version":1,"platforms":["linux"]}`)
	s.Documents = append(s.Documents, discovery.Document{Path: ".github/workflows/existing.yml", Content: []byte("on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm run test\n")})
	p, err = bootstrap.Analyze(s, bootstrap.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Recommendations) != 0 {
		t.Fatal("already linked definition must not duplicate")
	}
}

// These are the settled independent B1-B4 examples, not captured analyzer output.
func TestBootstrapSettledBlockers(t *testing.T) {
	decl := fixture().Documents[1]
	workflow := func(steps, defaults string) discovery.Document {
		return discovery.Document{Path: ".github/workflows/existing.yml", Content: []byte("on: push\n" + defaults + "jobs:\n  check:\n    runs-on: ubuntu-latest\n    steps:\n" + steps)}
	}
	cases := []struct {
		name, limitation string
		docs             []discovery.Document
	}{
		{"B1-test-gap-not-lint", "TEST", []discovery.Document{decl,
			{Path: "package.json", Content: []byte(`{"scripts":{"lint":"eslint ."}}`)},
			workflow("      - run: go test -race ./...\n      - run: npm run lint\n", "")}},
		{"B2-inherited-context", "execution context", []discovery.Document{decl,
			{Path: "pkg/go.mod", Content: []byte("module example.test/x\n")},
			workflow("      - run: go test ./...\n", "defaults:\n  run:\n    shell: bash\n    working-directory: pkg\n")}},
		{"B3-literal-expression-root", "expression", []discovery.Document{decl,
			{Path: "${{ github.workspace }}/package.json", Content: fixture().Documents[0].Content}}},
		{"B4-wrapper-chain", "chain", []discovery.Document{decl,
			{Path: "package.json", Content: []byte(`{"scripts":{"check":"npm run test && npm run lint","test":"node --test","lint":"eslint ."}}`)},
			workflow("      - run: npm run test\n      - run: npm run lint\n", "")}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := discovery.Snapshot{Documents: tc.docs}
			before, _ := json.Marshal(s)
			t.Cleanup(func() {
				after, _ := json.Marshal(s)
				if !bytes.Equal(before, after) {
					t.Error("snapshot bytes/records changed")
				}
			})
			p, err := bootstrap.Analyze(s, bootstrap.Options{})
			if err != nil {
				t.Fatal(err)
			}
			if ds := evidence.Validate(p.SourceReport); len(ds) != 0 {
				t.Fatalf("source report invalid: %v", ds)
			}
			if len(p.Recommendations) != 0 {
				t.Fatalf("must limit unsupported command/context/provenance, got draft: %s", p.Recommendations[0].Content)
			}
			if !strings.Contains(strings.Join(p.Limitations, "\n"), tc.limitation) {
				t.Fatalf("missing explicit %q limitation: %v", tc.limitation, p.Limitations)
			}
			again, err := bootstrap.Analyze(s, bootstrap.Options{})
			if err != nil || !reflect.DeepEqual(p, again) {
				t.Fatal("limited plan not deterministic")
			}
		})
	}
}

// Expression rejection must not discard an ordinary literal manifest directory.
func TestBootstrapLiteralDirectoryStillSupported(t *testing.T) {
	s := discovery.Snapshot{Documents: []discovery.Document{fixture().Documents[1],
		{Path: "packages/check/package.json", Content: fixture().Documents[0].Content}}}
	p, err := bootstrap.Analyze(s, bootstrap.Options{})
	if err != nil || len(p.Recommendations) != 1 {
		t.Fatalf("literal root regressed: %v %+v", err, p.Limitations)
	}
	if !strings.Contains(p.Recommendations[0].Content, `working-directory: "packages/check"`) {
		t.Fatal("literal manifest root changed")
	}
}

func TestScanOwnedImmutabilityAndTwoPass(t *testing.T) {
	root := t.TempDir()
	s := fixture()
	write := func(root string, docs []discovery.Document) {
		t.Helper()
		for _, d := range docs {
			p := filepath.Join(root, filepath.FromSlash(d.Path))
			if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, d.Content, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	write(root, s.Documents)
	if err := os.WriteFile(filepath.Join(root, "counter.txt"), []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	collect := func(root string) map[string]string {
		t.Helper()
		out := map[string]string{}
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
			if e != nil {
				return e
			}
			if !d.IsDir() {
				b, e := os.ReadFile(p)
				if e != nil {
					return e
				}
				rel, _ := filepath.Rel(root, p)
				out[rel] = string(b)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	before := collect(root)
	p, err := bootstrap.Scan(root, bootstrap.Options{})
	if err != nil || len(p.Recommendations) != 1 {
		t.Fatalf("scan: %v", err)
	}
	if _, err := bootstrap.Scan(root, bootstrap.Options{Platforms: []string{"invalid"}}); err == nil {
		t.Fatal("invalid option accepted")
	}
	if !reflect.DeepEqual(before, collect(root)) {
		t.Fatal("bytes/filelist/counter changed")
	}
	isolated := t.TempDir()
	write(isolated, s.Documents)
	r := p.Recommendations[0]
	write(isolated, []discovery.Document{{Path: r.Path, Content: []byte(r.Content)}})
	second, err := bootstrap.Scan(isolated, bootstrap.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Recommendations) != 0 {
		t.Fatal("isolated second pass duplicates")
	}
	if !reflect.DeepEqual(before, collect(root)) {
		t.Fatal("original source changed by second pass")
	}
}
