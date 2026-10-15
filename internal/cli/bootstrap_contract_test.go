package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/bootstrap"
	"github.com/dnlrsls/ci-trust/internal/evidence"
)

func fixturePlan(t *testing.T) bootstrap.Plan {
	t.Helper()
	data, err := os.ReadFile("../evidence/testdata/contracts/bounded-evidence.json")
	if err != nil {
		t.Fatal(err)
	}
	return bootstrap.Plan{Version: bootstrap.Version, DryRun: true, SourceReport: data, Recommendations: []bootstrap.Recommendation{{ID: "proposal", Path: ".github/workflows/proposed.yml", Content: "name: conditional\n", Diff: "+name: conditional\n", FindingRefs: []string{"finding"}, EvidenceRefs: []string{"product"}, Preconditions: []string{"Runtime must be provided", "Dependencies must be prepared"}, Conflicts: []string{"Existing workflow requires review"}, Limitations: []string{"Not executed\x1b[31m\u202e"}}}, Limitations: []string{"Enforcement UNKNOWN"}}
}

func TestBootstrapInvalidInvocations(t *testing.T) {
	cases := [][]string{{"bootstrap"}, {"bootstrap", "--dry-run"}, {"bootstrap", "target"}, {"bootstrap", "target", "--dry-run", "extra"}, {"bootstrap", "target", "--dry-run", "--apply"}, {"bootstrap", "target", "--dry-run", "--dry-run"}, {"bootstrap", "target", "--dry-run", "--input", "-"}, {"bootstrap", "target", "--dry-run", "--format", "yaml"}, {"bootstrap", "target", "--dry-run", "--format"}, {"bootstrap", "target", "--dry-run", "--platform", "Linux"}, {"bootstrap", "target", "--dry-run", "--platform", "linux", "--platform", "linux"}, {"audit", "target", "--dry-run"}, {"report", "--dry-run"}, {"audit", "target", "--apply"}}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, stderr bytes.Buffer
			calls := 0
			code := Run(args, nil, &out, &stderr, Services{Bootstrap: func(string, []string) (bootstrap.Plan, error) { calls++; return fixturePlan(t), nil }, Audit: func(string, []string) ([]byte, error) { calls++; return nil, nil }})
			if code != 2 || out.Len() != 0 || stderr.String() != "ci-trust: invalid arguments\n"+usage || calls != 0 {
				t.Fatalf("exit=%d stdout=%q stderr=%q calls=%d", code, out.String(), stderr.String(), calls)
			}
		})
	}
}

func TestBootstrapCompleteDeterministicAndOwned(t *testing.T) {
	p := fixturePlan(t)
	before, _ := json.Marshal(p)
	for _, format := range []string{"human", "json"} {
		var prior string
		for i := 0; i < 2; i++ {
			var out, stderr bytes.Buffer
			calls := 0
			args := []string{"bootstrap", "space ü", "--dry-run", "--platform", "windows", "--platform", "linux"}
			if format == "json" {
				args = append(args, "--format", "json")
			}
			code := Run(args, nil, &out, &stderr, Services{Bootstrap: func(target string, platforms []string) (bootstrap.Plan, error) {
				calls++
				if target != "space ü" || !reflect.DeepEqual(platforms, []string{"windows", "linux"}) {
					t.Fatal(target, platforms)
				}
				return p, nil
			}})
			if code != 0 || calls != 1 || stderr.String() != "fixture.note: Independent authored evidence.\n" {
				t.Fatalf("exit=%d stdout=%q stderr=%q calls=%d", code, out.String(), stderr.String(), calls)
			}
			if i > 0 && prior != out.String() {
				t.Fatal("nondeterministic")
			}
			prior = out.String()
			if format == "json" {
				var got bootstrap.Plan
				if err := json.Unmarshal(out.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Version != bootstrap.Version || !got.DryRun || len(got.Recommendations) != 1 {
					t.Fatal(got)
				}
				// All fields survive, apart from deterministic ordering and canonical report encoding.
				want := p.Recommendations[0]
				want.Preconditions = []string{"Dependencies must be prepared", "Runtime must be provided"}
				if !reflect.DeepEqual(got.Recommendations[0], want) || !reflect.DeepEqual(got.Limitations, p.Limitations) {
					t.Fatal(got)
				}
				canonical, ds := evidence.Encode(p.SourceReport)
				if len(ds) > 0 {
					t.Fatal(ds)
				}
				var a, b any
				_ = json.Unmarshal(canonical, &a)
				_ = json.Unmarshal(got.SourceReport, &b)
				if !reflect.DeepEqual(a, b) {
					t.Fatal("source changed")
				}
			} else {
				for _, text := range []string{"dry-run only; no apply", "Conditional draft", "proposed.yml", "content", "diff", "finding", "product", "Runtime must be provided", "Existing workflow requires review", "Not executed", "Enforcement UNKNOWN", "Source uncertainty"} {
					if !strings.Contains(out.String(), text) {
						t.Fatal("missing", text)
					}
				}
				if strings.ContainsAny(out.String(), "\x1b\u202e") {
					t.Fatal("terminal injection")
				}
			}
			after, _ := json.Marshal(p)
			if !bytes.Equal(before, after) {
				t.Fatal("service plan mutated")
			}
		}
	}
}

func TestBootstrapRejectsBadArtifactsBeforeOutput(t *testing.T) {
	for _, kind := range []string{"version", "dryrun", "report", "path", "finding", "evidence", "oversize", "error"} {
		t.Run(kind, func(t *testing.T) {
			p := fixturePlan(t)
			var serviceErr error
			switch kind {
			case "version":
				p.Version = "1"
			case "dryrun":
				p.DryRun = false
			case "report":
				p.SourceReport = []byte(`{}`)
			case "path":
				p.Recommendations[0].Path = "../escape"
			case "finding":
				p.Recommendations[0].FindingRefs = []string{"missing"}
			case "evidence":
				p.Recommendations[0].EvidenceRefs = []string{"missing"}
			case "oversize":
				p.Limitations = []string{strings.Repeat("x", maxInput)}
			case "error":
				serviceErr = errors.New("hostile\x1b")
			}
			var out, stderr bytes.Buffer
			calls := 0
			code := Run([]string{"bootstrap", "target", "--dry-run", "--format", "json"}, nil, &out, &stderr, Services{Bootstrap: func(string, []string) (bootstrap.Plan, error) { calls++; return p, serviceErr }})
			want := "ci-trust: invalid bootstrap plan\n"
			if kind == "error" {
				want = "ci-trust: bootstrap failed\n"
			}
			if code != 1 || out.Len() != 0 || stderr.String() != want || calls != 1 {
				t.Fatalf("exit=%d stdout=%q stderr=%q calls=%d", code, out.String(), stderr.String(), calls)
			}
		})
	}
}
