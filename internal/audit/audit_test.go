package audit

import (
	"bytes"
	"encoding/json"
	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/evidence"
	"testing"
)

func snapshot(files map[string]string) discovery.Snapshot {
	s := discovery.Snapshot{}
	for p, b := range files {
		s.Documents = append(s.Documents, discovery.Document{Path: p, Content: []byte(b)})
	}
	return s
}

const linuxWorkflow = "on: push\njobs:\n  test:\n    runs-on: ${{ matrix.os }}\n    strategy:\n      matrix:\n        os: [ubuntu-latest]\n    steps:\n      - run: go test ./...\n"
const validDeclaration = `{"version":1,"platforms":["linux","windows","macos"]}`

func report(t *testing.T, s discovery.Snapshot, o Options) map[string]any {
	t.Helper()
	b, e := Analyze(s, o)
	if e != nil {
		t.Fatal(e)
	}
	if ds := evidence.Validate(b); len(ds) > 0 {
		t.Fatalf("invalid report: %v bytes %s", ds, b)
	}
	again, e := Analyze(s, o)
	if e != nil || !bytes.Equal(b, again) {
		t.Fatal("nondeterministic")
	}
	var r map[string]any
	if e = json.Unmarshal(b, &r); e != nil {
		t.Fatal(e)
	}
	return r
}
func findings(r map[string]any, rule string) []map[string]any {
	out := []map[string]any{}
	for _, v := range r["findings"].([]any) {
		m := v.(map[string]any)
		if m["rule_id"] == rule {
			out = append(out, m)
		}
	}
	return out
}
func TestDeclaredPlatformGaps(t *testing.T) {
	s := snapshot(map[string]string{".ci-trust.json": validDeclaration, "go.mod": "module example.test/x\n", ".github/workflows/ci.yml": linuxWorkflow})
	r := report(t, s, Options{})
	got := findings(r, "CIT_PLATFORM_CONFIGURED_GAP")
	if len(got) != 2 {
		t.Fatalf("want exactly Windows/macOS configured gaps, got %d", len(got))
	}
	for _, f := range got {
		if len(f["evidence_refs"].([]any)) < 2 {
			t.Fatal("gap lacks declaration and matrix proof")
		}
	}
}
