package workflows_test

import (
	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/workflows"
	"testing"
)

func TestEventConditions(t *testing.T) {
	r := workflows.Analyze(discovery.Snapshot{Documents: []discovery.Document{{Path: ".github/workflows/ci.yml", Content: []byte("on: [push, pull_request, merge_group]\njobs:\n  check:\n    if: github.event_name == 'push'\n    runs-on: ubuntu-latest\n    steps:\n      - run: npm test\n")}}}, discovery.Result{}, workflows.Options{})
	if len(r.Workflows) != 1 {
		t.Fatalf("workflows: %+v", r)
	}
	j := r.Workflows[0].Jobs[0]
	if j.Eligibility["push"] != workflows.Eligible || j.Eligibility["pull_request"] != workflows.Ineligible || j.Historical != workflows.Unknown {
		t.Fatalf("static versus historical: %+v", j)
	}
	if r.Workflows[0].Events[0].Source.Line != 1 {
		t.Fatal("missing on source")
	}
}
