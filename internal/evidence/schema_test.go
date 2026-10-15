package evidence

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestStructureFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/contracts/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name       string `json:"name"`
		File       string `json:"file"`
		Structural bool   `json:"structural"`
		Code       string `json:"structural_code"`
		Mutations  []struct {
			Old string `json:"old"`
			New string `json:"new"`
		} `json:"mutations"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name+"/"+tc.File, func(t *testing.T) {
			input, err := os.ReadFile("testdata/contracts/" + tc.File)
			if err != nil {
				t.Fatal(err)
			}
			for _, mutation := range tc.Mutations {
				if !bytes.Contains(input, []byte(mutation.Old)) {
					t.Fatalf("mutation target absent: %q", mutation.Old)
				}
				input = bytes.Replace(input, []byte(mutation.Old), []byte(mutation.New), 1)
			}
			before := bytes.Clone(input)
			got := ValidateStructure(input)
			if !bytes.Equal(input, before) {
				t.Fatal("input changed")
			}
			if tc.Structural && len(got) != 0 {
				t.Fatalf("valid fixture rejected: %+v", got)
			}
			if !tc.Structural && (len(got) == 0 || got[0].Code != tc.Code) {
				t.Fatalf("expected %s rejection, got %+v", tc.Code, got)
			}
		})
	}
}

// Every defined object boundary is independently closed, including nested
// nullable records. The base report provides actual non-null instances.
func TestStructureRecordBoundaries(t *testing.T) {
	base, err := os.ReadFile("testdata/contracts/bounded-evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"", "/snapshot", "/inventory/0", "/inventory/0/source", "/capabilities/0", "/capabilities/0/scope", "/workflows/0", "/workflows/0/jobs/0", "/evidence/0", "/evidence/0/provenance", "/evidence/1/execution", "/evidence/1/execution/tool", "/evidence/5/cause", "/findings/0", "/diagnostics/0"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			var document any
			decoder := json.NewDecoder(bytes.NewReader(base))
			decoder.UseNumber()
			if err := decoder.Decode(&document); err != nil {
				t.Fatal(err)
			}
			record := document
			if path != "" {
				for _, part := range strings.Split(path[1:], "/") {
					switch value := record.(type) {
					case map[string]any:
						record = value[part]
					case []any:
						index, err := strconv.Atoi(part)
						if err != nil {
							t.Fatal(err)
						}
						record = value[index]
					default:
						t.Fatalf("not a record path: %s", path)
					}
				}
			}
			object, ok := record.(map[string]any)
			if !ok {
				t.Fatalf("not a record: %s", path)
			}
			object["unexpected"] = true
			input, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			before := bytes.Clone(input)
			diagnostics := ValidateStructure(input)
			if len(diagnostics) == 0 {
				t.Fatal("accepted extra property")
			}
			found := false
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == "schema.invalid" && diagnostic.Path == path {
					found = true
				}
			}
			if !found {
				t.Fatalf("expected attributable rejection at %q, got %+v", path, diagnostics)
			}
			if !bytes.Equal(input, before) {
				t.Fatal("input changed")
			}
		})
	}
}

func TestStructureMutations(t *testing.T) {
	base, err := os.ReadFile("testdata/contracts/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		old, new string
		code     string
	}{
		{"version", `"schema_version": "1"`, `"schema_version": "2"`, "schema.version"},
		{"unknown root field", `"schema_version": "1"`, `"extra": true, "schema_version": "1"`, "schema.invalid"},
		{"missing provenance", `"provenance":`, `"missing":`, "schema.invalid"},
		{"bad state", `"CONFIGURED"`, `"GREEN"`, "schema.invalid"},
		{"unsafe source", `"ci/check.yml"`, `"../secret"`, "schema.invalid"},
		{"precision", `"line": 1`, `"line": 1.00000000000000001`, "schema.invalid"},
		{"snapshot extras", `"repository_id": null`, `"extra": true, "repository_id": null`, "schema.invalid"},
		{"inventory extras", `"kind": "CHECK"`, `"extra": true, "kind": "CHECK"`, "schema.invalid"},
		{"source extras", `"column": 1`, `"extra": true, "column": 1`, "schema.invalid"},
		{"scope extras", `"subject": "repo"`, `"extra": true, "subject": "repo"`, "schema.invalid"},
		{"provenance extras", `"adapter": "fixture"`, `"extra": true, "adapter": "fixture"`, "schema.invalid"},
		{"evidence extras", `"check_id": "check:lint"`, `"extra": true, "check_id": "check:lint"`, "schema.invalid"},
		{"bad SHA", `"commit_sha": null`, `"commit_sha": "abc"`, "schema.invalid"},
		{"host repository", `"repository_id": null`, `"repository_id": "C:/private/repo"`, "schema.invalid"},
		{"absolute source", `"ci/check.yml"`, `"/private/check.yml"`, "schema.invalid"},
		{"trailing JSON", "", "", "json.invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := bytes.Replace(base, []byte(tc.old), []byte(tc.new), 1)
			if tc.name == "trailing JSON" {
				input = append(bytes.Clone(base), []byte(" {}")...)
			}
			before := bytes.Clone(input)
			got := ValidateStructure(input)
			if len(got) == 0 || got[0].Code != tc.code {
				t.Fatalf("expected %s, got %+v", tc.code, got)
			}
			if !bytes.Equal(before, input) {
				t.Fatal("input changed")
			}
			for _, d := range got {
				if bytes.Contains([]byte(d.Message), []byte("secret")) {
					t.Fatal("rejected value leaked")
				}
			}
		})
	}
}
