package audit

import (
	"bytes"
	"encoding/json"
	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/evidence"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUnknownAndLiteralPaths(t *testing.T) {
	cases := []struct {
		name, workflow string
		rule           string
		gaps           int
	}{
		{"literal all", strings.Replace(linuxWorkflow, "[ubuntu-latest]", "[ubuntu-latest, windows-latest, macos-latest]", 1), "", 0},
		{"dynamic runner", strings.Replace(linuxWorkflow, "${{ matrix.os }}", "${{ vars.RUNNER }}", 1), "CIT_DYNAMIC_PATH", 0},
		{"dynamic matrix", strings.Replace(linuxWorkflow, "matrix:\n        os: [ubuntu-latest]", "matrix: ${{ fromJSON(vars.MATRIX) }}", 1), "CIT_DYNAMIC_PATH", 0},
		{"dynamic condition", strings.Replace(linuxWorkflow, "runs-on:", "if: ${{ inputs.enabled }}\n    runs-on:", 1), "CIT_DYNAMIC_PATH", 0},
		{"known omission", strings.Replace(linuxWorkflow, "runs-on:", "if: false\n    runs-on:", 1), "CIT_STATIC_OMISSION", 0},
		{"nonblocking", strings.Replace(linuxWorkflow, "runs-on:", "continue-on-error: true\n    runs-on:", 1), "CIT_NON_BLOCKING", 2},
		{"name and echo", strings.Replace(linuxWorkflow, "go test ./...", "echo E2E", 1), "CIT_DYNAMIC_PATH", 0},
		{"wrapper", strings.Replace(linuxWorkflow, "go test ./...", "env GOOS=windows go test ./...", 1), "CIT_DYNAMIC_PATH", 0},
		{"cross build", strings.Replace(linuxWorkflow, "go test ./...", "go build ./...", 1), "", 3},
		{"malformed", "on: [\n", "", 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			r := report(t, snapshot(map[string]string{".ci-trust.json": validDeclaration, ".github/workflows/ci.yml": tt.workflow}), Options{})
			if n := len(findings(r, "CIT_PLATFORM_CONFIGURED_GAP")); n != tt.gaps {
				t.Fatalf("gaps=%d want %d", n, tt.gaps)
			}
			if tt.rule != "" && len(findings(r, tt.rule)) == 0 {
				t.Fatalf("missing %s", tt.rule)
			}
			if len(findings(r, "CIT_REQUIRED_CHECKS_UNKNOWN")) != 1 {
				t.Fatal("missing offline policy")
			}
			for _, v := range r["evidence"].([]any) {
				e := v.(map[string]any)
				if e["execution"] != nil || e["state"] == "EXECUTED" || e["state"] == "SKIPPED" || e["state"] == "NOT_EXECUTED" {
					t.Fatalf("fabricated execution %v", e)
				}
			}
		})
	}
}
func TestStrictDeclarationsAndOverride(t *testing.T) {
	for _, text := range []string{"", `{}`, `{"version":2,"platforms":["linux"]}`, `{"version":1,"platforms":["LINUX"]}`, `{"version":1,"platforms":["linux","linux"]}`, `{"version":1,"version":1,"platforms":["linux"]}`, `{"version":1,"platforms":["linux"],"extra":1}`, `{"version":1,"platforms":[]}`, validDeclaration + `{}`, `null`} {
		t.Run(text, func(t *testing.T) {
			s := snapshot(map[string]string{".ci-trust.json": text, ".github/workflows/ci.yml": linuxWorkflow})
			before, _ := json.Marshal(s)
			r := report(t, s, Options{})
			if len(findings(r, "CIT_DECLARATION_UNKNOWN")) != 1 || len(findings(r, "CIT_PLATFORM_CONFIGURED_GAP")) != 0 {
				t.Fatal("invented declaration")
			}
			r = report(t, s, Options{Platforms: []string{"windows"}})
			if len(findings(r, "CIT_DECLARATION_UNKNOWN")) != 0 || len(findings(r, "CIT_PLATFORM_CONFIGURED_GAP")) != 1 {
				t.Fatal("override failed")
			}
			for _, v := range r["evidence"].([]any) {
				e := v.(map[string]any)
				if e["id"] == evidence.StableID("evidence", "declaration") {
					p := e["provenance"].(map[string]any)
					if p["kind"] != "DIAGNOSTIC" || len(p["sources"].([]any)) != 0 || e["state"] != "UNKNOWN" {
						t.Fatal("fake explicit-input file provenance")
					}
				}
			}
			after, _ := json.Marshal(s)
			if !bytes.Equal(before, after) {
				t.Fatal("mutated snapshot")
			}
		})
	}
}
func TestNpmExactLinkAndProvenance(t *testing.T) {
	s := snapshot(map[string]string{"package.json": "{\n  \"scripts\": {\n    \"lint\": \"eslint .\",\n    \"test\": \"node --test\"\n  }\n}\n", ".github/workflows/ci.yml": strings.Replace(linuxWorkflow, "go test ./...", "npm test", 1)})
	r := report(t, s, Options{})
	if len(findings(r, "CIT_COMMAND_UNLINKED")) != 1 {
		t.Fatal("lint must be unlinked, test linked")
	}
	found := false
	for _, v := range r["evidence"].([]any) {
		e := v.(map[string]any)
		p := e["provenance"].(map[string]any)
		for _, v := range p["sources"].([]any) {
			x := v.(map[string]any)
			if x["path"] == "package.json" && x["line"] == float64(4) && x["column"] == float64(5) {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("exact scripts.test token missing")
	}
}
func TestExactOfflineStatesAndNoNameCoverage(t *testing.T) {
	r := report(t, snapshot(map[string]string{".github/workflows/ci.yml": strings.Replace(linuxWorkflow, "go test ./...", "echo E2E", 1)}), Options{})
	for _, v := range r["capabilities"].([]any) {
		c := v.(map[string]any)
		if c["dimension"] == "CONFIGURATION" && c["status"] == "PRESENT" {
			t.Fatal("echo/name attributed validation")
		}
		if c["dimension"] != "CONFIGURATION" && c["status"] != "UNKNOWN" {
			t.Fatal("offline dimension promoted")
		}
	}
	for _, v := range r["findings"].([]any) {
		f := v.(map[string]any)
		if len(f["limitations"].([]any)) == 0 || f["scope"] == nil || len(f["evidence_refs"].([]any)) == 0 {
			t.Fatal("unbounded finding")
		}
	}
	found := false
	for _, v := range r["evidence"].([]any) {
		e := v.(map[string]any)
		if e["id"] == evidence.StableID("evidence", "offline-policy") {
			found = true
			if e["state"] != "UNKNOWN" || e["scope"].(map[string]any)["kind"] != "REPOSITORY" || e["provenance"].(map[string]any)["kind"] != "DIAGNOSTIC" {
				t.Fatal("policy state/scope/provenance mismatch")
			}
		}
	}
	if !found {
		t.Fatal("missing exact offline evidence ID")
	}
}

func TestReaderOmissionsPreventGaps(t *testing.T) {
	s := snapshot(map[string]string{".ci-trust.json": validDeclaration, ".github/workflows/ci.yml": linuxWorkflow})
	s.Diagnostics = []discovery.Diagnostic{{Code: "FILE_LIMIT", Path: ".github/workflows/other.yml"}}
	r := report(t, s, Options{})
	if len(findings(r, "CIT_PLATFORM_CONFIGURED_GAP")) != 0 {
		t.Fatal("reader omission closed scope")
	}
}

func TestEmptyAndAPIRejections(t *testing.T) {
	for _, s := range []discovery.Snapshot{{}, snapshot(map[string]string{"README.md": "unsupported stack"})} {
		report(t, s, Options{})
	}
	cases := []discovery.Snapshot{snapshot(map[string]string{"../package.json": "{}"}), snapshot(map[string]string{"C:/package.json": "{}"}), {Documents: []discovery.Document{{Path: "package.json"}, {Path: "package.json"}}}, snapshot(map[string]string{"package.json": strings.Repeat("x", (1<<20)+1)}), {Documents: make([]discovery.Document, 4097)}}
	for i, s := range cases {
		before, _ := json.Marshal(s)
		b, e := Analyze(s, Options{})
		if e == nil || b != nil {
			t.Fatalf("case %d accepted", i)
		}
		after, _ := json.Marshal(s)
		if !bytes.Equal(before, after) {
			t.Fatal("rejection mutated snapshot")
		}
	}
	if b, e := Scan("unavailable-root", Options{Platforms: []string{"bad"}}); e == nil || e.Error() != "unsupported platform option" || b != nil {
		t.Fatal("options not validated first")
	}
	if b, e := Scan(filepath.Join(t.TempDir(), "missing"), Options{}); e == nil || b != nil {
		t.Fatal("missing root accepted")
	}
}
func TestScanReadOnlyAndDeterministic(t *testing.T) {
	root := t.TempDir()
	inputs := map[string]string{".ci-trust.json": validDeclaration, "package.json": `{"scripts":{"test":"node --test","lint":"eslint ."}}`, "counter": "0", "do-not-run.sh": "echo changed > counter\n", ".github/workflows/ci.yml": strings.Replace(linuxWorkflow, "go test ./...", "npm test", 1)}
	for p, s := range inputs {
		f := filepath.Join(root, filepath.FromSlash(p))
		if e := os.MkdirAll(filepath.Dir(f), 0755); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(f, []byte(s), 0644); e != nil {
			t.Fatal(e)
		}
	}
	a, e := Scan(root, Options{})
	if e != nil {
		t.Fatal(e)
	}
	b, e := Scan(root, Options{})
	if e != nil || !bytes.Equal(a, b) {
		t.Fatal("scan not deterministic")
	}
	if ds := evidence.Validate(a); len(ds) > 0 {
		t.Fatal(ds)
	}
	_, _ = Scan(root, Options{Platforms: []string{"invalid"}})
	for p, want := range inputs {
		got, e := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
		if e != nil || string(got) != want {
			t.Fatalf("changed input %s", p)
		}
	}
	if bytes.Contains(a, []byte(root)) {
		t.Fatal("host prefix leaked")
	}
}
func TestSnapshotIdentityAndRetainedSecrets(t *testing.T) {
	s := snapshot(map[string]string{".github/workflows/ci.yml": linuxWorkflow, "package.json": `{"scripts":{"test":"node --test"},"token":"ghp_ownedFixtureSecret123"}`})
	a, e := Analyze(s, Options{})
	if e != nil {
		t.Fatal(e)
	}
	if bytes.Contains(a, []byte("ghp_ownedFixtureSecret123")) {
		t.Fatal("secret leak")
	}
	s.Documents[0], s.Documents[1] = s.Documents[1], s.Documents[0]
	b, e := Analyze(s, Options{})
	if e != nil || !bytes.Equal(a, b) {
		t.Fatal("snapshot order changed report")
	}
	s.Documents[0].Content = append(append([]byte(nil), s.Documents[0].Content...), '\n')
	c, e := Analyze(s, Options{})
	if e != nil || bytes.Equal(b, c) {
		t.Fatal("identity ignored owned bytes")
	}
}
