package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode"
	"unicode/utf16"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

// reviewSummaryExpected retains every canonical raw record, independently of the renderer.
func reviewSummaryExpected(t *testing.T, canonical []byte) string {
	t.Helper()
	var r map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &r); err != nil {
		t.Fatal(err)
	}
	var findings []struct {
		Rule        string `json:"rule_id"`
		Explanation string `json:"explanation"`
	}
	if err := json.Unmarshal(r["findings"], &findings); err != nil {
		t.Fatal(err)
	}
	type group struct {
		rule, explanation string
	}
	counts := map[group]int{}
	for _, f := range findings {
		counts[group{f.Rule, f.Explanation}]++
	}
	groups := []group{}
	for g := range counts {
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].rule != groups[j].rule {
			return groups[i].rule < groups[j].rule
		}
		return groups[i].explanation < groups[j].explanation
	})
	var b strings.Builder
	if len(groups) > 0 {
		b.WriteString("Review summary (record counts, not confirmed defects):\n")
		for _, g := range groups {
			fmt.Fprintf(&b, "  %s: %d record(s) — %s\n", reviewSummaryEscape(g.rule), counts[g], reviewSummaryEscape(g.explanation))
		}
	}
	b.WriteString("CI Trust report (configuration is not execution or enforcement)\n")
	for _, key := range []string{"schema_version", "snapshot"} {
		fmt.Fprintf(&b, "%s: %s\n", key, r[key])
	}
	for _, key := range []string{"inventory", "capabilities", "workflows", "evidence", "findings", "diagnostics", "limitations"} {
		fmt.Fprintf(&b, "%s:\n", key)
		var values []json.RawMessage
		if err := json.Unmarshal(r[key], &values); err != nil {
			t.Fatal(err)
		}
		if len(values) == 0 {
			b.WriteString("  (none)\n")
		}
		for _, v := range values {
			fmt.Fprintf(&b, "  %s\n", reviewSummaryEscape(string(v)))
		}
	}
	return b.String()
}

func reviewSummaryEscape(text string) string {
	var b strings.Builder
	for _, r := range text {
		if !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) {
			b.WriteRune(r)
			continue
		}
		if r <= 0xffff {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			hi, lo := utf16.EncodeRune(r)
			fmt.Fprintf(&b, "\\u%04x\\u%04x", hi, lo)
		}
	}
	return b.String()
}

func TestPublicReviewSummaryReportAndAudit(t *testing.T) {
	binary := buildPublicBinary(t)
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".github"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, ".github", "workflows"), 0700); err != nil {
		t.Fatal(err)
	}
	workflow := "on: [push]\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm test\n"
	if err := os.WriteFile(filepath.Join(root, ".github", "workflows", "ci.yml"), []byte(workflow), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sentinel"), []byte("unchanged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := fixtureBytes(t, root)
	code, canonical, stderr := invokePublic(t, binary, nil, "audit", root, "--format", "json")
	wantStderr := "npm-definition-unresolved: Static analysis could not resolve this input; no execution or enforcement is inferred.\n" +
		"DECLARATION_UNKNOWN: Static analysis could not resolve this input; no execution or enforcement is inferred.\n"
	if code != 0 || string(stderr) != wantStderr {
		t.Fatalf("audit exit=%d stdout=%q stderr=%q", code, canonical, stderr)
	}
	if ds := evidence.Validate(canonical); len(ds) != 0 {
		t.Fatal(ds)
	}
	code, human, stderr := invokePublic(t, binary, nil, "audit", root)
	if code != 0 || string(stderr) != wantStderr || string(human) != reviewSummaryExpected(t, canonical) {
		t.Fatalf("audit exit=%d stdout=%q stderr=%q", code, human, stderr)
	}
	var r map[string]any
	if err := json.Unmarshal(canonical, &r); err != nil {
		t.Fatal(err)
	}
	fs := r["findings"].([]any)
	if len(fs) == 0 {
		t.Fatal("fixture must produce findings")
	}
	// Equal explanations across scopes count records, not presumed root causes.
	original := fs[0].(map[string]any)
	fs = nil
	for i, explanation := range []string{"Same uncertainty", "Same uncertainty", "Different uncertainty"} {
		f := map[string]any{}
		for k, v := range original {
			f[k] = v
		}
		f["id"] = fmt.Sprintf("finding:review%d", i)
		f["scope"] = map[string]any{"kind": "REPOSITORY", "subject": fmt.Sprintf("scope%d", i)}
		f["explanation"] = explanation
		fs = append(fs, f)
	}
	r["findings"] = fs
	for _, empty := range []bool{false, true} {
		if empty {
			r["findings"] = []any{}
		}
		input, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		expected, ds := evidence.Encode(input)
		if len(ds) != 0 {
			t.Fatal(ds)
		}
		path := filepath.Join(root, "report.json")
		if err := os.WriteFile(path, input, 0600); err != nil {
			t.Fatal(err)
		}
		reportBefore := fixtureBytes(t, root)
		for _, format := range []string{"human", "json"} {
			code, out, stderr := invokePublic(t, binary, nil, "report", "--input", path, "--format", format)
			want := expected
			if format == "human" {
				want = []byte(reviewSummaryExpected(t, expected))
			}
			if code != 0 || string(stderr) != wantStderr || !bytes.Equal(out, want) {
				t.Fatalf("report %s empty=%v exit=%d stdout=%q stderr=%q want=%q", format, empty, code, out, stderr, want)
			}
			if format == "human" && !empty {
				for i := 0; i < 3; i++ {
					if !bytes.Contains(out, []byte(fmt.Sprintf("finding:review%d", i))) {
						t.Fatal("raw finding lost")
					}
				}
				if !bytes.Contains(out, []byte("2 record(s) — Same uncertainty\n")) || !bytes.Contains(out, []byte("1 record(s) — Different uncertainty\n")) {
					t.Fatal("incorrect exact-explanation groups")
				}
			}
		}
		unchangedFixture(t, root, reportBefore)
	}
	// The original audit inputs remain byte-identical; report files are owned additions.
	after := fixtureBytes(t, root)
	for path, want := range before {
		if after[path] != want {
			t.Fatalf("audit source changed: %s", path)
		}
	}
}
