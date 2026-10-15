package evidence

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCanonicalEfficacyReferencePermutation(t *testing.T) {
	r, _, c := efficacyReport()
	forward := encoded(t, r)
	a, ds := Canonicalize(forward)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	c["evidence_refs"] = []any{"RESTORED", "FAULT", "CLEAN"}
	reverse := encoded(t, r)
	saved := bytes.Clone(reverse)
	if ds := ValidateSemantics(reverse); len(ds) > 0 {
		t.Fatalf("reversed reference set rejected: %v", ds)
	}
	b, ds := Canonicalize(reverse)
	if len(ds) > 0 || !bytes.Equal(a, b) {
		t.Fatalf("reference permutation changed canonical bytes: %v", ds)
	}
	if !bytes.Equal(saved, reverse) {
		t.Fatal("input mutated")
	}
}
func TestCanonicalEquivalentSourceNumbersAndOrdering(t *testing.T) {
	var expected []byte
	for _, number := range []string{"1", "1.0", "1e0"} {
		r := report()
		entry := r["inventory"].([]any)[0].(map[string]any)
		s := entry["source"].(map[string]any)
		s["line"] = json.Number(number)
		s["column"] = json.Number("9007199254740993")
		r["workflows"] = []any{map[string]any{"id": "w", "source": source(), "events": []any{"push", "pull_request"}, "jobs": []any{map[string]any{"id": "z", "needs": []any{"a"}, "evidence_refs": []any{}}, map[string]any{"id": "a", "needs": []any{}, "evidence_refs": []any{}}}}}
		input := encoded(t, r)
		saved := bytes.Clone(input)
		out, ds := Canonicalize(input)
		if len(ds) > 0 {
			t.Fatal(ds)
		}
		if expected == nil {
			expected = out
		} else if !bytes.Equal(expected, out) {
			t.Fatalf("equivalent source number %s changed bytes", number)
		}
		if !bytes.Contains(out, []byte(`"column":9007199254740993`)) || !bytes.Contains(out, []byte(`"events":["pull_request","push"]`)) || !bytes.Equal(saved, input) {
			t.Fatal("precision, ordering or preservation failure")
		}
	}
}
func TestCanonicalReferenceSets(t *testing.T) {
	r := report()
	a := observation("UNKNOWN", "DIAGNOSTIC")
	a["id"] = "a"
	b := observation("UNKNOWN", "DIAGNOSTIC")
	b["id"] = "b"
	r["evidence"] = []any{b, a}
	r["findings"] = []any{map[string]any{"id": "f", "certainty": "UNKNOWN", "evidence_refs": []any{"b", "a"}}}
	data, _ := json.Marshal(r)
	out, ds := Canonicalize(data)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	if !bytes.Contains(out, []byte(`"evidence_refs":["a","b"]`)) {
		t.Fatal("reference set not sorted")
	}
}
func TestCanonicalUnknownField(t *testing.T) {
	r := report()
	r["unexpected"] = true
	b, _ := json.Marshal(r)
	if out, ds := Canonicalize(b); len(ds) == 0 || out != nil {
		t.Fatal("unknown field accepted")
	}
}
func TestCanonicalBoundary(t *testing.T) {
	input := []byte(`{"diagnostics":[],"findings":[],"evidence":[],"workflows":[],"capabilities":[],"inventory":[],"snapshot":{"id":"s","repository_id":null,"commit_sha":null},"schema_version":"1","limitations":[]}`)
	saved := bytes.Clone(input)
	out, ds := Canonicalize(input)
	if len(ds) > 0 || !bytes.HasSuffix(out, []byte("\n")) {
		t.Fatalf("canonical newline missing; diagnostics=%v", ds)
	}
	again, ds := Canonicalize(out)
	if len(ds) > 0 || !bytes.Equal(out, again) || !bytes.Equal(input, saved) {
		t.Fatal("canonicalization not stable or mutated input")
	}
	if StableID("check", "ab", "c") == StableID("check", "a", "bc") {
		t.Fatal("ambiguous stable ID")
	}
	if out, ds := Canonicalize([]byte(`{"evidence":false}`)); len(ds) == 0 || out != nil {
		t.Fatal("invalid input accepted")
	}
}
