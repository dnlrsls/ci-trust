package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

func TestExistingContractsPreserved(t *testing.T) {
	for _, name := range []string{"valid", "states", "bounded-evidence"} {
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile("../evidence/testdata/contracts/" + name + ".json")
			if err != nil {
				t.Fatal(err)
			}
			original := bytes.Clone(data)
			want, ds := evidence.Encode(data)
			if len(ds) > 0 {
				t.Fatal(ds)
			}
			var out, stderr bytes.Buffer
			code := Run([]string{"audit", "owned ü fixture", "--format", "json"}, forbiddenReader{t}, &out, &stderr, Services{Audit: func(string, []string) ([]byte, error) { return data, nil }})
			wantStderr := ""
			if name == "bounded-evidence" {
				wantStderr = "fixture.note: Independent authored evidence.\n"
			}
			if code != 0 || !bytes.Equal(out.Bytes(), want) || stderr.String() != wantStderr || !bytes.Equal(data, original) {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
		})
	}
}
