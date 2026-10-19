package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

func TestReviewSummaryGroupedLossless(t *testing.T) {
	data, err := os.ReadFile("../evidence/testdata/contracts/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	var empty, stderr bytes.Buffer
	if code := Run([]string{"report"}, bytes.NewReader(data), &empty, &stderr, Services{}); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	explanation := "Unknown\x1b\n\u009b\u202e"
	findings := []any{}
	for i, subject := range []string{"a", "b", "c", "d"} {
		text := explanation
		if i == 2 {
			text = "Other"
		}
		rule := "rule"
		if i == 3 {
			rule = "another-rule"
		}
		findings = append(findings, map[string]any{
			"id": "finding:" + subject, "rule_id": rule,
			"scope": map[string]any{"kind": "REPOSITORY", "subject": subject},
			"evidence_refs": []string{"ev:configured"}, "explanation": text,
			"certainty": "UNKNOWN", "certainty_rule": "Unresolved",
			"limitations": []string{"Offline only"},
		})
	}
	report["findings"] = findings
	data, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	before := append([]byte(nil), data...)
	canonical, ds := evidence.Encode(data)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	var out bytes.Buffer
	stderr.Reset()
	if code := Run([]string{"report"}, bytes.NewReader(data), &out, &stderr, Services{}); code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%q", code, stderr.String())
	}
	summary := "Review summary (record counts, not confirmed defects):\n" +
		"  another-rule: 1 record(s) — Unknown\\u001b\\u000a\\u009b\\u202e\n" +
		"  rule: 1 record(s) — Other\n" +
		"  rule: 2 record(s) — Unknown\\u001b\\u000a\\u009b\\u202e\n"
	// Independently reconstruct the old complete body from the canonical records.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(canonical, &raw); err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	body.WriteString("CI Trust report (configuration is not execution or enforcement)\n")
	for _, key := range []string{"schema_version", "snapshot"} {
		body.WriteString(key + ": " + string(raw[key]) + "\n")
	}
	for _, key := range []string{"inventory", "capabilities", "workflows", "evidence", "findings", "diagnostics", "limitations"} {
		body.WriteString(key + ":\n")
		var values []json.RawMessage
		if err := json.Unmarshal(raw[key], &values); err != nil {
			t.Fatal(err)
		}
		if len(values) == 0 {
			body.WriteString("  (none)\n")
		}
		for _, value := range values {
			body.WriteString("  " + strings.NewReplacer("\u009b", "\\u009b", "\u202e", "\\u202e").Replace(string(value)) + "\n")
		}
	}
	if want := summary + body.String(); out.String() != want {
		t.Fatalf("stdout=%q want=%q", out.String(), want)
	}
	if strings.ContainsAny(out.String(), "\x1b\u009b\u202e") {
		t.Fatal("terminal control leaked")
	}
	out.Reset()
	if code := Run([]string{"report", "--format", "json"}, bytes.NewReader(data), &out, &stderr, Services{}); code != 0 || stderr.Len() != 0 || !bytes.Equal(out.Bytes(), canonical) {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
	if !bytes.Equal(data, before) {
		t.Fatal("input mutated")
	}
	// Existing exact empty rendering remains the reference, including every section.
	report["findings"] = []any{}
	data, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if code := Run([]string{"report"}, bytes.NewReader(data), &out, &stderr, Services{}); code != 0 || stderr.Len() != 0 || out.String() != empty.String() {
		t.Fatalf("empty exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}
