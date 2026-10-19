package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/bootstrap"
	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/evidence"
	"github.com/dnlrsls/ci-trust/internal/workflows"
)

func ownedBootstrapFixture(t *testing.T) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "conditional source ñ")
	files := map[string]string{
		".ci-trust.json":      "{\"version\":1,\"platforms\":[\"linux\",\"windows\",\"macos\"]}\n",
		"package.json":        "{\"scripts\":{\"test\":\"node --test\"}}\n",
		"counter":             "0\n",
		"test/never.test.cjs": "require('node:fs').writeFileSync(require('node:path').join(__dirname,'..','counter'),'executed');\n",
	}
	for name, data := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func publicBootstrapPlan(t *testing.T, out []byte) bootstrap.Plan {
	t.Helper()
	var p bootstrap.Plan
	if err := json.Unmarshal(out, &p); err != nil {
		t.Fatalf("one plan JSON: %v, %q", err, out)
	}
	if p.Version != bootstrap.Version || !p.DryRun {
		t.Fatalf("not a dry-run proposal: %+v", p)
	}
	if ds := evidence.Validate(p.SourceReport); len(ds) != 0 {
		t.Fatalf("source report: %v", ds)
	}
	if !bytes.HasSuffix(out, []byte("\n")) || bytes.Count(out, []byte("\n")) != 1 {
		t.Fatal("expected exactly one JSON line plus newline")
	}
	if len(p.Limitations) == 0 {
		t.Fatal("missing plan uncertainty")
	}
	return p
}

func TestPublicBootstrapS10(t *testing.T) {
	binary := buildPublicBinary(t)
	root := ownedBootstrapFixture(t)
	before := fixtureBytes(t, root)
	var acceptedJSON []byte
	var acceptedPlan bootstrap.Plan
	t.Run("conditional-json", func(t *testing.T) {
		args := []string{"bootstrap", root, "--dry-run", "--format", "json"}
		code, out, errOut := invokePublic(t, binary, nil, args...)
		unchangedFixture(t, root, before)
		if code != 0 || len(errOut) != 0 {
			t.Fatalf("exit=%d stdout=%q stderr=%q; want valid conditional plan/empty stderr", code, out, errOut)
		}
		p := publicBootstrapPlan(t, out)
		acceptedJSON, acceptedPlan = append([]byte{}, out...), p
		if len(p.Recommendations) != 1 {
			t.Fatalf("empty supported fixture needs one minimal workflow: %+v", p)
		}
		r := p.Recommendations[0]
		if r.ID == "" || r.Path != ".github/workflows/ci-trust.yml" || r.Content == "" || r.Diff == "" || len(r.Preconditions) == 0 || len(r.Limitations) == 0 || len(r.FindingRefs) == 0 || len(r.EvidenceRefs) == 0 {
			t.Fatalf("incomplete source-backed proposal: %+v", r)
		}
		if !strings.Contains(r.Content, "node --test") && !strings.Contains(r.Content, "npm test") && !strings.Contains(r.Content, "npm run test") {
			t.Fatal("no supported declared invocation")
		}
		limits := strings.ToLower(strings.Join(append(append([]string{}, r.Preconditions...), r.Limitations...), " "))
		if !strings.Contains(limits, "dependenc") || (!strings.Contains(limits, "runtime") && !strings.Contains(limits, "toolchain")) {
			t.Fatal("unknown runtime/dependencies not explicit")
		}
		var report object
		if err := json.Unmarshal(p.SourceReport, &report); err != nil {
			t.Fatal(err)
		}
		findings, evs := map[string]object{}, map[string]object{}
		for _, f := range objects(report, "findings") {
			findings[text(f["id"])] = f
		}
		for _, e := range objects(report, "evidence") {
			evs[text(e["id"])] = e
		}
		for _, id := range r.FindingRefs {
			if findings[id] == nil {
				t.Fatalf("invented finding %s", id)
			}
		}
		paths := map[string]bool{}
		for _, id := range r.EvidenceRefs {
			e := evs[id]
			if e == nil {
				t.Fatalf("invented evidence %s", id)
			}
			for _, s := range e["provenance"].(map[string]any)["sources"].([]any) {
				paths[text(s.(map[string]any)["path"])] = true
			}
		}
		if !paths["package.json"] || !paths[".ci-trust.json"] {
			t.Fatalf("missing decisive command/declaration sources: %v", paths)
		}
		code2, out2, err2 := invokePublic(t, binary, nil, args...)
		assertPublic(t, code2, out2, err2, 0, string(out), "")
		unchangedFixture(t, root, before)
	})
	t.Run("conditional-human", func(t *testing.T) {
		args := []string{"bootstrap", root, "--dry-run"}
		code, out, errOut := invokePublic(t, binary, nil, args...)
		unchangedFixture(t, root, before)
		if code != 0 || len(errOut) != 0 {
			t.Fatalf("exit=%d stdout=%q stderr=%q", code, out, errOut)
		}
		if len(acceptedJSON) == 0 {
			t.Fatal("JSON contract must pass before human binding")
		}
		var report object
		if err := json.Unmarshal(acceptedPlan.SourceReport, &report); err != nil {
			t.Fatal(err)
		}
		want := "CI Trust bootstrap plan (dry-run only; no apply)\nConditional draft: not ready, executed, or enforced; review all preconditions and conflicts.\n" + string(acceptedJSON) + "Source uncertainty (configuration is not execution or enforcement):\n" + reviewSummaryExpected(t, acceptedPlan.SourceReport)
		assertPublic(t, code, out, errOut, 0, want, "")
		if !bytes.HasSuffix(out, referenceHuman(t, report)) {
			t.Fatal("summary changed the original source report body")
		}
		for _, token := range []string{"dry", ".github/workflows/ci-trust.yml", "precondition", "limitation"} {
			if !strings.Contains(strings.ToLower(string(out)), token) {
				t.Fatalf("human proposal omits %q: %q", token, out)
			}
		}
		code2, out2, err2 := invokePublic(t, binary, nil, args...)
		assertPublic(t, code2, out2, err2, 0, string(out), "")
		unchangedFixture(t, root, before)
	})
	t.Run("isolated-second-pass-and-yaml", func(t *testing.T) {
		if len(acceptedPlan.Recommendations) != 1 {
			t.Fatal("positive semantic gate must pass before isolated application")
		}
		second := ownedBootstrapFixture(t)
		r := acceptedPlan.Recommendations[0]
		// Only this owned temporary copy receives proposed data, never the audited input.
		p := filepath.Join(second, filepath.FromSlash(r.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(r.Content), 0600); err != nil {
			t.Fatal(err)
		}
		snapshot := discovery.ReadSnapshot(second, discovery.Options{})
		graph := workflows.Analyze(snapshot, discovery.Analyze(snapshot, discovery.Options{}), workflows.Options{})
		if len(graph.Workflows) != 1 || len(graph.Workflows[0].Jobs) == 0 {
			t.Fatal("proposed YAML is not understood by existing analyzer")
		}
		secondBefore := fixtureBytes(t, second)
		code, out, errOut := invokePublic(t, binary, nil, "bootstrap", second, "--dry-run", "--format", "json")
		if code != 0 {
			t.Fatalf("second pass exit=%d stdout=%q stderr=%q", code, out, errOut)
		}
		plan := publicBootstrapPlan(t, out)
		if len(plan.Recommendations) != 0 {
			t.Fatalf("duplicate equivalent proposal: %+v", plan.Recommendations)
		}
		var report object
		if err := json.Unmarshal(plan.SourceReport, &report); err != nil {
			t.Fatal(err)
		}
		var expectedErr strings.Builder
		for _, d := range objects(report, "diagnostics") {
			expectedErr.WriteString(text(d["code"]) + ": " + text(d["message"]) + "\n")
		}
		if string(errOut) != expectedErr.String() {
			t.Fatalf("second-pass stderr=%q want=%q", errOut, expectedErr.String())
		}
		for _, f := range objects(report, "findings") {
			if f["rule_id"] == "CIT_PLATFORM_CONFIGURED_GAP" || f["rule_id"] == "CIT_COMMAND_UNLINKED" {
				t.Fatalf("generated YAML failed declared command/platform matching: %v", f)
			}
		}
		code2, out2, err2 := invokePublic(t, binary, nil, "bootstrap", second, "--dry-run", "--format", "json")
		assertPublic(t, code2, out2, err2, 0, string(out), expectedErr.String())
		unchangedFixture(t, second, secondBefore)
		unchangedFixture(t, root, before)
	})
	t.Run("reject-without-effects", func(t *testing.T) {
		for _, args := range [][]string{{"bootstrap", root}, {"bootstrap", root, "--apply"}, {"bootstrap", "--dry-run"}, {"bootstrap", root, "--dry-run", "--format", "xml"}, {"bootstrap", root, "--dry-run", "--platform", "unknown"}} {
			code, out, errOut := invokePublic(t, binary, nil, args...)
			assertPublic(t, code, out, errOut, 2, "", "ci-trust: invalid arguments\n"+publicHelp)
			unchangedFixture(t, root, before)
		}
	})
}
