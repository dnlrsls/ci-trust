package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

func TestCommands(t *testing.T) {
	data, err := os.ReadFile("../evidence/testdata/golden/empty-report.json")
	if err != nil {
		t.Fatal(err)
	}
	canonical, ds := evidence.Encode(data)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	human := "CI Trust report (configuration is not execution or enforcement)\nschema_version: \"1\"\nsnapshot: {\"commit_sha\":null,\"id\":\"snapshot:example\",\"repository_id\":null}\n"
	for _, section := range []string{"inventory", "capabilities", "workflows", "evidence", "findings", "diagnostics", "limitations"} {
		human += section + ":\n  (none)\n"
	}
	for _, tt := range []struct {
		name  string
		args  []string
		input string
		out   string
		code  int
		calls int
	}{
		{"report json", []string{"report", "--format", "json"}, string(data), string(canonical), 0, 0},
		{"report default", []string{"report"}, string(data), human, 0, 0},
		{"audit after path", []string{"audit", "space ü", "--format", "json", "--platform", "linux"}, "", string(canonical), 0, 1},
		{"audit before path", []string{"audit", "--format", "json", "space ü"}, "", string(canonical), 0, 1},
		{"audit human", []string{"audit", "space ü"}, "", human, 0, 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			calls := 0
			code := Run(tt.args, strings.NewReader(tt.input), &out, &stderr, Services{Audit: func(path string, p []string) ([]byte, error) {
				calls++
				if path != "space ü" {
					t.Fatal(path)
				}
				return data, nil
			}})
			if code != tt.code || out.String() != tt.out || stderr.String() != "" || calls != tt.calls {
				t.Fatalf("exit=%d stdout=%q stderr=%q calls=%d", code, out.String(), stderr.String(), calls)
			}
		})
	}
}
