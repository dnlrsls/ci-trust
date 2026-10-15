package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

func TestScopedRenderingAndDiagnostics(t *testing.T) {
	data, err := os.ReadFile("../evidence/testdata/contracts/valid.json")
	if err != nil {
		t.Fatal(err)
	}
	var report map[string]any
	if err = json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	report["diagnostics"] = []any{map[string]any{"id": "diag:one", "code": "PARTIAL", "message": "Unresolved\x1b[31m\n\u009b\u202e", "source": nil}}
	report["limitations"] = []any{"Offline only"}
	data, err = json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	canonical, ds := evidence.Encode(data)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	// These records are owned extensions of the existing valid contract fixture.
	wantHuman := "CI Trust report (configuration is not execution or enforcement)\n" +
		"schema_version: \"1\"\nsnapshot: {\"commit_sha\":null,\"id\":\"snapshot:one\",\"repository_id\":null}\n" +
		"inventory:\n  {\"evidence_refs\":[\"ev:configured\"],\"id\":\"check:lint\",\"kind\":\"CHECK\",\"name\":\"Lint\",\"source\":{\"column\":1,\"line\":1,\"path\":\"ci/check.yml\"}}\n" +
		"capabilities:\n  (none)\nworkflows:\n  (none)\nevidence:\n  {\"cause\":null,\"check_id\":\"check:lint\",\"execution\":null,\"id\":\"ev:configured\",\"limitations\":[],\"provenance\":{\"adapter\":\"fixture\",\"basis\":\"SOURCE\",\"kind\":\"CONFIGURATION\",\"sources\":[{\"column\":1,\"line\":1,\"path\":\"ci/check.yml\"}]},\"scope\":{\"kind\":\"REPOSITORY\",\"subject\":\"repo\"},\"state\":\"CONFIGURED\"}\n" +
		"findings:\n  (none)\ndiagnostics:\n  {\"code\":\"PARTIAL\",\"id\":\"diag:one\",\"message\":\"Unresolved\\u001b[31m\\n\\u009b\\u202e\",\"source\":null}\nlimitations:\n  \"Offline only\"\n"
	for _, format := range []string{"human", "json"} {
		t.Run(format, func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := Run([]string{"report", "--format", format}, bytes.NewReader(data), &out, &stderr, Services{})
			want := wantHuman
			if format == "json" {
				want = string(canonical)
				if ds = evidence.Validate(out.Bytes()); len(ds) > 0 {
					t.Fatal(ds)
				}
			}
			if code != 0 || out.String() != want || stderr.String() != "PARTIAL: Unresolved\\u001b[31m\\u000a\\u009b\\u202e\n" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
			if format == "human" && strings.ContainsAny(out.String(), "\x1b\u009b\u202e") {
				t.Fatal("terminal control leaked")
			}
		})
	}
}
