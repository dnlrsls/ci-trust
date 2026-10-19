package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode"
	"unicode/utf16"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

var updateAdversarial = flag.Bool("update", false, "capture semantically checked serialization candidates (independent acceptance pending)")

const captureSource = "624a6ffd6e9ad100fd7f3f3ee57f5cbda336be65"

type publicExpectation struct {
	CaseID    string   `json:"case_id"`
	Sources   []string `json:"sources"`
	Required  []string `json:"required_rule_ids"`
	Forbidden []string `json:"forbidden_rule_ids"`
	Public    struct {
		Exit   int    `json:"exit"`
		Stderr string `json:"stderr"`
	} `json:"public_audit"`
}
type object = map[string]any

func fixtureBytes(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			result[filepath.ToSlash(rel)+"/"] = ""
			return nil
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("nonregular owned input %s", rel)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		result[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func unchangedFixture(t *testing.T, root string, before map[string]string) {
	t.Helper()
	if !reflect.DeepEqual(before, fixtureBytes(t, root)) {
		t.Fatal("owned input bytes/filelist changed")
	}
}
func objects(r object, key string) []object {
	var out []object
	for _, v := range r[key].([]any) {
		out = append(out, v.(map[string]any))
	}
	return out
}
func text(v any) string { return fmt.Sprint(v) }

// This gate precedes ALL candidate writes. Membership comes from independently
// authored manifests; additional assertions protect their scoped meaning.
func assertAdversarialSemantics(t *testing.T, root string, want publicExpectation, out []byte) object {
	t.Helper()
	if ds := evidence.Validate(out); len(ds) != 0 {
		t.Fatalf("schema/references: %v", ds)
	}
	canonical, ds := evidence.Encode(out)
	if len(ds) != 0 || !bytes.Equal(canonical, out) || !bytes.HasSuffix(out, []byte("\n")) {
		t.Fatal("not exactly one canonical JSON document plus newline")
	}
	var r object
	if err := json.Unmarshal(out, &r); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(out, []byte(root)) || bytes.Contains(out, []byte(filepath.ToSlash(root))) {
		t.Fatal("absolute source path leak")
	}
	for _, p := range want.Sources {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err != nil {
			t.Fatal(err)
		}
	}
	// Recursively validate every source anchor against the owned input, including
	// line/column bounds; schema validation alone cannot establish source existence.
	var anchors func(any)
	anchors = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			if p, ok := v["path"].(string); ok {
				b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(p)))
				if err != nil {
					t.Fatalf("source %s: %v", p, err)
				}
				lines := bytes.Split(b, []byte("\n"))
				line := int(v["line"].(float64))
				col := int(v["column"].(float64))
				if line < 1 || line > len(lines) || col < 1 || col > len(lines[line-1])+1 {
					t.Fatalf("invalid source %s:%d:%d", p, line, col)
				}
			}
			for _, x := range v {
				anchors(x)
			}
		case []any:
			for _, x := range v {
				anchors(x)
			}
		}
	}
	anchors(r)
	evs := map[string]object{}
	for _, e := range objects(r, "evidence") {
		evs[text(e["id"])] = e
		state := text(e["state"])
		if state != "CONFIGURED" && state != "UNKNOWN" {
			t.Fatalf("YAML fabricated execution state %s", state)
		}
		if e["execution"] != nil {
			t.Fatal("offline execution record")
		}
		p := e["provenance"].(map[string]any)
		if p["kind"] != "CONFIGURATION" && p["kind"] != "DIAGNOSTIC" {
			t.Fatal("nonstatic provenance")
		}
		if len(e["limitations"].([]any)) == 0 {
			t.Fatal("evidence lacks limitations")
		}
	}
	rules := map[string][]object{}
	for _, f := range objects(r, "findings") {
		rules[text(f["rule_id"])] = append(rules[text(f["rule_id"])], f)
		if len(f["limitations"].([]any)) == 0 {
			t.Fatal("finding lacks limitations")
		}
		for _, ref := range f["evidence_refs"].([]any) {
			if evs[text(ref)] == nil {
				t.Fatal("unresolved finding evidence")
			}
		}
	}
	for _, id := range want.Required {
		if len(rules[id]) == 0 {
			t.Fatalf("independent required rule missing: %s", id)
		}
	}
	for _, id := range want.Forbidden {
		if len(rules[id]) != 0 {
			t.Fatalf("independent forbidden rule present: %s", id)
		}
	}
	present := 0
	for _, c := range objects(r, "capabilities") {
		if c["dimension"] != "CONFIGURATION" && c["status"] != "UNKNOWN" {
			t.Fatal("execution/efficacy/enforcement not UNKNOWN")
		}
		if c["dimension"] == "CONFIGURATION" && c["status"] == "PRESENT" {
			present++
		}
	}
	switch want.CaseID {
	case "a-misleading-e2e", "omitted-job", "malformed-workflow":
		if present != 0 {
			t.Fatal("echo/name/malformed input attributed validation")
		}
	case "correct-config", "b-skipped-validation", "c-dynamic-matrix", "d-non-blocking", "e-incomplete-platform", "f-unlinked-command":
		if present == 0 {
			t.Fatal("missing configured generic TEST intent")
		}
	}
	if want.CaseID == "omitted-job" && len(rules["CIT_STATIC_OMISSION"]) != 0 {
		t.Fatal("echo-only workflow is not event omission")
	}
	if want.CaseID == "b-skipped-validation" {
		for _, f := range rules["CIT_STATIC_OMISSION"] {
			s := text(f["explanation"])
			if !strings.Contains(s, "pull_request") || strings.Contains(s, "event push") {
				t.Fatalf("omission not scoped to pull_request: %s", s)
			}
		}
	}
	if want.CaseID == "b-skipped-validation" || want.CaseID == "d-non-blocking" {
		rule, field := "CIT_STATIC_OMISSION", "if:"
		if want.CaseID == "d-non-blocking" {
			rule, field = "CIT_NON_BLOCKING", "continue-on-error:"
		}
		scopes := map[string]bool{}
		for _, finding := range rules[rule] {
			scope := finding["scope"].(map[string]any)
			scopes[text(scope["kind"])] = true
			decisive := false
			for _, ref := range finding["evidence_refs"].([]any) {
				for _, source := range evs[text(ref)]["provenance"].(map[string]any)["sources"].([]any) {
					anchor := source.(map[string]any)
					if anchor["path"] == ".github/workflows/ci.yml" && int(anchor["line"].(float64)) == 4 {
						input, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "ci.yml"))
						if err != nil {
							t.Fatal(err)
						}
						decisive = strings.Contains(strings.Split(string(input), "\n")[3], field)
					}
				}
			}
			if !decisive {
				t.Fatalf("%s %s lacks decisive %s provenance on workflow line 4", rule, scope["kind"], field)
			}
		}
		if !scopes["JOB"] || want.CaseID == "b-skipped-validation" && !scopes["STEP"] {
			t.Fatalf("missing job or inherited step observation for %s: %v", rule, scopes)
		}
	}
	if want.CaseID == "e-incomplete-platform" {
		gaps := rules["CIT_PLATFORM_CONFIGURED_GAP"]
		if len(gaps) != 2 {
			t.Fatalf("want exactly two platform gaps, got %d", len(gaps))
		}
		seen := map[string]bool{}
		for _, f := range gaps {
			explanation := text(f["explanation"])
			for _, p := range []string{"windows", "macos"} {
				if strings.Contains(explanation, "Declared "+p+" ") {
					seen[p] = true
				}
			}
			paths := map[string]bool{}
			for _, ref := range f["evidence_refs"].([]any) {
				e := evs[text(ref)]
				for _, s := range e["provenance"].(map[string]any)["sources"].([]any) {
					paths[text(s.(map[string]any)["path"])] = true
				}
			}
			if !paths[".ci-trust.json"] || !paths[".github/workflows/ci.yml"] {
				t.Fatal("gap lacks declaration AND workflow evidence")
			}
		}
		if !seen["windows"] || !seen["macos"] {
			t.Fatal("gaps are not exactly Windows/macOS")
		}
	}
	if want.CaseID == "f-unlinked-command" {
		for _, f := range rules["CIT_COMMAND_UNLINKED"] {
			found := false
			for _, ref := range f["evidence_refs"].([]any) {
				for _, s := range evs[text(ref)]["provenance"].(map[string]any)["sources"].([]any) {
					loc := s.(map[string]any)
					if loc["path"] == "package.json" {
						b, _ := os.ReadFile(filepath.Join(root, "package.json"))
						line := int(loc["line"].(float64))
						if strings.Contains(strings.Split(string(b), "\n")[line-1], "lint") {
							found = true
						}
					}
				}
			}
			if !found {
				t.Fatal("unlinked finding does not anchor lint")
			}
		}
	}
	return r
}

// referenceHuman implements the published docs/cli.md layout using only the
// already validated data. It is a formatting reference, never a semantic oracle.
func referenceHuman(t *testing.T, report object) []byte {
	t.Helper()
	value := func(v any) string {
		b, err := json.Marshal(v) // sorted object keys, compact JSON
		if err != nil {
			t.Fatal(err)
		}
		var safe strings.Builder
		for _, r := range string(b) {
			if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
				if r > 0xffff {
					hi, lo := utf16.EncodeRune(r)
					fmt.Fprintf(&safe, "\\u%04x\\u%04x", hi, lo)
				} else {
					fmt.Fprintf(&safe, "\\u%04x", r)
				}
			} else {
				safe.WriteRune(r)
			}
		}
		return safe.String()
	}
	var out strings.Builder
	out.WriteString("CI Trust report (configuration is not execution or enforcement)\n")
	fmt.Fprintf(&out, "schema_version: %s\nsnapshot: %s\n", value(report["schema_version"]), value(report["snapshot"]))
	for _, section := range []string{"inventory", "capabilities", "workflows", "evidence", "findings", "diagnostics", "limitations"} {
		fmt.Fprintf(&out, "%s:\n", section)
		records := report[section].([]any)
		if len(records) == 0 {
			out.WriteString("  (none)\n")
		}
		for _, record := range records {
			fmt.Fprintf(&out, "  %s\n", value(record))
		}
	}
	return []byte(out.String())
}

func TestPublicAdversarial(t *testing.T) {
	if testing.Short() {
		t.Skip("owned public binary integration")
	}
	binary := buildPublicBinary(t)
	corpus := filepath.Join("..", "..", "internal", "audittest", "testdata")
	entries, err := os.ReadDir(corpus)
	if err != nil {
		t.Fatal(err)
	}
	// Capture is a second phase: no case may write a candidate if ANY independent
	// semantic/public contract gate fails in the corpus.
	type capture struct {
		path string
		data []byte
	}
	var captures []capture
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			base := filepath.Join(corpus, entry.Name())
			raw, err := os.ReadFile(filepath.Join(base, "expected.json"))
			if err != nil {
				t.Fatal(err)
			}
			var want publicExpectation
			if err = json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			if want.CaseID != entry.Name() {
				t.Fatal("case identity mismatch")
			}
			root := filepath.Join(t.TempDir(), "owned source with spaces ñ 世界")
			source := fixtureBytes(t, filepath.Join(base, "repo"))
			for p, b := range source {
				dst := filepath.Join(root, filepath.FromSlash(p))
				if strings.HasSuffix(p, "/") {
					err = os.MkdirAll(dst, 0700)
				} else {
					err = os.MkdirAll(filepath.Dir(dst), 0700)
					if err == nil {
						err = os.WriteFile(dst, []byte(b), 0600)
					}
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, ok := source["counter"]; !ok {
				if err = os.WriteFile(filepath.Join(root, "counter"), []byte("0\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := fixtureBytes(t, root)
			code, out, stderr := invokePublic(t, binary, nil, "audit", root, "--format", "json")
			unchangedFixture(t, root, before)
			if code != want.Public.Exit {
				t.Fatalf("audit exit=%d stdout=%q stderr=%q", code, out, stderr)
			}
			r := assertAdversarialSemantics(t, root, want, out)
			diagnosticStderr := ""
			for _, d := range objects(r, "diagnostics") {
				diagnosticStderr += text(d["code"]) + ": " + text(d["message"]) + "\n"
			}
			if string(stderr) != want.Public.Stderr {
				t.Fatalf("independent stderr requirement unmet: got %q want %q; report diagnostics=%q", stderr, want.Public.Stderr, diagnosticStderr)
			}
			if string(stderr) != diagnosticStderr {
				t.Fatalf("L13 diagnostic binding: got %q want %q", stderr, diagnosticStderr)
			}
			c, o, e := invokePublic(t, binary, nil, "audit", root, "--format", "json")
			assertPublic(t, c, o, e, code, string(out), string(stderr))
			unchangedFixture(t, root, before)
			hc, human, he := invokePublic(t, binary, nil, "audit", root, "--format", "human")
			assertPublic(t, hc, human, he, 0, reviewSummaryExpected(t, out), string(stderr))
			if !bytes.HasSuffix(human, referenceHuman(t, r)) {
				t.Fatal("summary changed the original raw human body")
			}
			unchangedFixture(t, root, before)
			for _, args := range [][]string{{"audit", root, "--format", "xml"}, {"audit", root, "--unexpected"}} {
				c, o, e = invokePublic(t, binary, nil, args...)
				assertPublic(t, c, o, e, 2, "", "ci-trust: invalid arguments\n"+publicHelp)
				unchangedFixture(t, root, before)
			}
			// A separate saved report and unrelated cwd inputs are not source dependencies.
			saved := filepath.Join(t.TempDir(), "saved report ñ.json")
			if err = os.WriteFile(saved, out, 0600); err != nil {
				t.Fatal(err)
			}
			// Detach only the owned temporary copy after every audit/rejection check.
			// The original source path must be absent throughout report rendering.
			detached := root + " detached"
			if err = os.Rename(root, detached); err != nil {
				t.Fatal(err)
			}
			if _, err = os.Stat(root); !os.IsNotExist(err) {
				t.Fatalf("source path must be absent: %v", err)
			}
			unchangedFixture(t, detached, before)
			for _, format := range []string{"json", "human"} {
				expected := out
				if format == "human" {
					expected = human
				}
				for _, args := range [][]string{{"report", "--input", saved, "--format", format}, {"report", "--input", "-", "--format", format}} {
					c, o, e = invokePublic(t, binary, out, args...)
					assertPublic(t, c, o, e, 0, string(expected), string(stderr))
					unchangedFixture(t, detached, before)
					if _, err = os.Stat(root); !os.IsNotExist(err) {
						t.Fatalf("report recreated source path: %v", err)
					}
				}
			}
			savedBytes, err := os.ReadFile(saved)
			if err != nil || !bytes.Equal(savedBytes, out) {
				t.Fatal("saved report changed")
			}
			for name, data := range map[string][]byte{"audit.json": out, "audit.human": referenceHuman(t, r), "stderr.txt": stderr} {
				captures = append(captures, capture{filepath.Join(base, name), data})
			}
		})
	}
	if t.Failed() {
		return
	}
	for _, c := range captures {
		if *updateAdversarial {
			if err := os.WriteFile(c.path, c.data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(c.path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(want, c.data) {
			t.Fatalf("whole-byte candidate mismatch %s", c.path)
		}
	}
	if *updateAdversarial {
		metadata := []byte("Serialization capture source: " + captureSource + "\nIndependent acceptance: PENDING\nLocal public harness only; not workflow execution or remote-portable proof.\n")
		if err := os.WriteFile(filepath.Join(corpus, "capture.txt"), metadata, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
