package evidence

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func emptyReport() map[string]any {
	return map[string]any{
		"schema_version": "1",
		"snapshot": map[string]any{
			"id": "snapshot:example", "repository_id": nil, "commit_sha": nil,
		},
		"inventory": []any{}, "capabilities": []any{}, "workflows": []any{},
		"evidence": []any{}, "findings": []any{}, "diagnostics": []any{},
		"limitations": []any{},
	}
}

func reportBytes(t *testing.T, doc map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestValidateContractBoundary(t *testing.T) {
	tests := []struct {
		name string
		edit func(map[string]any)
		ok   bool
	}{
		{"explicit unknown snapshot metadata", func(map[string]any) {}, true},
		{"unsupported version", func(d map[string]any) { d["schema_version"] = "2" }, false},
		{"unknown trust score", func(d map[string]any) { d["trust_score"] = 100 }, false},
		{"required inventory absent", func(d map[string]any) { delete(d, "inventory") }, false},
		{"invalid commit SHA", func(d map[string]any) { d["snapshot"].(map[string]any)["commit_sha"] = "abc" }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := emptyReport()
			tt.edit(doc)
			input := reportBytes(t, doc)
			before := bytes.Clone(input)
			diagnostics := Validate(input)
			if (len(diagnostics) == 0) != tt.ok {
				t.Fatalf("valid=%v; diagnostics=%+v", tt.ok, diagnostics)
			}
			if !bytes.Equal(input, before) {
				t.Fatal("validation changed submitted bytes")
			}
		})
	}
}

func TestEncodeCanonicalGolden(t *testing.T) {
	input := reportBytes(t, emptyReport())
	before := bytes.Clone(input)
	got, diagnostics := Encode(input)
	if len(diagnostics) != 0 {
		t.Fatalf("encode: %+v", diagnostics)
	}
	want, err := os.ReadFile("testdata/golden/empty-report.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("canonical report mismatch\ngot: %s\nwant: %s", got, want)
	}
	again, diagnostics := Encode(got)
	if len(diagnostics) != 0 || !bytes.Equal(got, again) {
		t.Fatalf("encoding is not idempotent: %+v", diagnostics)
	}
	if !bytes.Equal(input, before) {
		t.Fatal("encoding changed submitted bytes")
	}
}

func TestEncodeRejectsBeforeEmittingDocument(t *testing.T) {
	const secret = "sample-secret-never-echo"
	doc := emptyReport()
	doc["unexpected"] = secret
	input := reportBytes(t, doc)
	before := bytes.Clone(input)
	got, diagnostics := Encode(input)
	if len(got) != 0 || len(diagnostics) == 0 {
		t.Fatalf("rejected data emitted output or lacked diagnostic: %q %+v", got, diagnostics)
	}
	rendered, err := json.Marshal(diagnostics)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(rendered, []byte(secret)) {
		t.Fatal("diagnostic exposed rejected input value")
	}
	if !bytes.Equal(input, before) {
		t.Fatal("rejection changed submitted bytes")
	}
}

func TestSharedFixtureAcceptanceMatrix(t *testing.T) {
	manifestBytes, err := os.ReadFile("testdata/contracts/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest []struct {
		Name           string   `json:"name"`
		File           string   `json:"file"`
		Structural     bool     `json:"structural"`
		Semantic       bool     `json:"semantic"`
		StructuralCode string   `json:"structural_code"`
		SemanticCodes  []string `json:"semantic_codes"`
		Mutations      []struct {
			Old string `json:"old"`
			New string `json:"new"`
		} `json:"mutations"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest) == 0 {
		t.Fatal("fixture expectations must be authored independently")
	}
	for _, example := range manifest {
		t.Run(example.Name, func(t *testing.T) {
			input, err := os.ReadFile(filepath.Join("testdata/contracts", example.File))
			if err != nil {
				t.Fatal(err)
			}
			for _, mutation := range example.Mutations {
				if !bytes.Contains(input, []byte(mutation.Old)) {
					t.Fatal("independently authored mutation target missing")
				}
				input = bytes.Replace(input, []byte(mutation.Old), []byte(mutation.New), 1)
			}
			before := bytes.Clone(input)
			semanticDiagnostics := ValidateSemantics(input)
			semanticCodes := []string{}
			for _, diagnostic := range semanticDiagnostics {
				if !slices.Contains(semanticCodes, diagnostic.Code) {
					semanticCodes = append(semanticCodes, diagnostic.Code)
				}
			}
			wantCodes := slices.Clone(example.SemanticCodes)
			slices.Sort(semanticCodes)
			slices.Sort(wantCodes)
			if (len(semanticDiagnostics) == 0) != example.Semantic || !slices.Equal(semanticCodes, wantCodes) {
				t.Fatalf("standalone semantic verdict/code mismatch: got %+v; want accepted=%v codes=%v", semanticDiagnostics, example.Semantic, wantCodes)
			}
			diagnostics := Validate(input)
			valid := example.Structural && example.Semantic
			if (len(diagnostics) == 0) != valid {
				t.Fatalf("fixture expected valid=%v; diagnostics=%+v", valid, diagnostics)
			}
			wantCode := example.StructuralCode
			if example.Structural && !example.Semantic {
				if len(example.SemanticCodes) == 0 {
					t.Fatal("semantic negative fixture needs independently specified diagnostic codes")
				}
				wantCode = example.SemanticCodes[0]
			}
			if !valid && wantCode != "" {
				found := false
				for _, diagnostic := range diagnostics {
					found = found || diagnostic.Code == wantCode
				}
				if !found {
					t.Fatalf("missing expected code %q: %+v", wantCode, diagnostics)
				}
			}
			got, encodingDiagnostics := Encode(input)
			if valid {
				if len(encodingDiagnostics) != 0 || len(got) == 0 {
					t.Fatalf("accepted fixture failed serialization: %+v", encodingDiagnostics)
				}
				repeated, repeatedDiagnostics := Encode(got)
				if len(repeatedDiagnostics) != 0 || !bytes.Equal(got, repeated) {
					t.Fatalf("accepted fixture serialization is not idempotent: %+v", repeatedDiagnostics)
				}
			} else if len(got) != 0 || len(encodingDiagnostics) == 0 {
				t.Fatal("rejected fixture emitted report bytes or lacked diagnostics")
			}
			if !bytes.Equal(input, before) {
				t.Fatal("fixture input changed")
			}
		})
	}
}

func TestExplicitRedactionBeforeFullSerialization(t *testing.T) {
	const secret = "sample-credential"
	const hostPrefix = "C:/private/workspace"
	doc := emptyReport()
	doc["limitations"] = []any{"No evidence collected at " + hostPrefix + " with " + secret}
	input := reportBytes(t, doc)
	before := bytes.Clone(input)
	redacted, diagnostics := Redact(input, []string{secret, hostPrefix})
	if len(diagnostics) != 0 {
		t.Fatalf("redaction: %+v", diagnostics)
	}
	got, diagnostics := Encode(redacted)
	if len(diagnostics) != 0 {
		t.Fatalf("encode redacted report: %+v", diagnostics)
	}
	for _, privateValue := range []string{secret, hostPrefix} {
		if bytes.Contains(got, []byte(privateValue)) {
			t.Fatal("provided sensitive value survived in serialized report")
		}
	}
	if !bytes.Equal(input, before) {
		t.Fatal("redaction/encoding changed submitted bytes")
	}
}

func TestSharedFixtureCanonicalOrdering(t *testing.T) {
	input, err := os.ReadFile("testdata/contracts/bounded-evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	original := bytes.Clone(input)
	want, diagnostics := Encode(input)
	if len(diagnostics) != 0 {
		t.Fatalf("independent valid fixture rejected: %+v", diagnostics)
	}
	var doc map[string]any
	if err := json.Unmarshal(input, &doc); err != nil {
		t.Fatal(err)
	}
	var reverseSets func(any, string)
	reverseSets = func(value any, key string) {
		switch v := value.(type) {
		case map[string]any:
			for name, child := range v {
				reverseSets(child, name)
			}
		case []any:
			for _, child := range v {
				reverseSets(child, "")
			}
			switch key {
			case "inventory", "capabilities", "workflows", "evidence", "findings", "diagnostics", "jobs", "events", "needs", "evidence_refs":
				slices.Reverse(v)
			}
		}
	}
	reverseSets(doc, "")
	reordered := reportBytes(t, doc)
	got, diagnostics := Encode(reordered)
	if len(diagnostics) != 0 || !bytes.Equal(got, want) {
		t.Fatalf("equivalent set ordering changed canonical report: %+v", diagnostics)
	}
	if !bytes.Equal(input, original) {
		t.Fatal("fixture input mutated")
	}
}
