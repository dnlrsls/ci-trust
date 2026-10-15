package workflows_test

import (
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/workflows"
)

func TestAlternateMatrixCases(t *testing.T) {
	for _, tt := range []struct {
		name, matrix string
		count        int
		state        workflows.State
	}{
		{"include only", "{include: [{os: ubuntu}, {os: mac}]}", 2, workflows.Configured},
		{"typed exclusion", "{x: [true, 'true'], exclude: [{x: true}]}", 1, workflows.Configured},
		{"numeric equality", "{x: [18], exclude: [{x: 0x12}]}", 0, workflows.Configured},
		{"nested object", "{x: [{os: ubuntu}]}", 0, workflows.Unknown},
		{"empty", "{}", 0, workflows.Unknown},
		{"include not cumulative", "{include: [{os: ubuntu}, {os: ubuntu, version: 2}]}", 2, workflows.Configured},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := analyze("on: push\njobs:\n  a:\n    strategy:\n      matrix: "+tt.matrix+"\n", workflows.Options{})
			m := r.Workflows[0].Jobs[0].Matrix
			if m.State != tt.state || len(m.Combinations) != tt.count {
				t.Fatalf("matrix: %+v", m)
			}
			if tt.name == "typed exclusion" && m.Literals[0]["x"].Tag != "!!str" {
				t.Fatal("lost scalar type")
			}
		})
	}
}

func TestCountLimitsAndDirectAdmission(t *testing.T) {
	for _, tt := range []struct {
		name, body, code string
		options          workflows.Options
	}{
		{"jobs", "on: push\njobs: {a: {}, b: {}}", "job-limit", workflows.Options{MaxJobs: 1}},
		{"steps", "on: push\njobs:\n  a:\n    steps: [{run: echo a}, {run: echo b}]", "step-limit", workflows.Options{MaxSteps: 1}},
		{"flow depth", strings.Repeat("[", 1000) + strings.Repeat("]", 1000), "yaml-admission-limit", workflows.Options{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := analyze(tt.body, tt.options)
			if !has(r, tt.code) || r.State != workflows.Unknown {
				t.Fatalf("limit: %+v", r)
			}
		})
	}
	docs := []discovery.Document{{Path: ".github/workflows/a.yml", Content: []byte("on: push\njobs: {}")}, {Path: ".github/workflows/a.yml", Content: []byte("on: pull_request\njobs: {}")}}
	for _, o := range []workflows.Options{{}, {MaxDocuments: 1}} {
		r := workflows.Analyze(discovery.Snapshot{Documents: docs}, discovery.Result{}, o)
		if len(r.Workflows) != 0 || len(r.Diagnostics) == 0 {
			t.Fatal("ambiguous/bypassed snapshot")
		}
	}
}

func TestUnknownExpressionsAndMisleadingNames(t *testing.T) {
	r := analyze("on: [pull_request, push, merge_group]\njobs:\n  E2E:\n    if: inputs.enabled\n    runs-on: '${{ matrix.os }}'\n    strategy: {matrix: '${{ inputs.matrix }}'}\n    steps: [{run: echo ok}, {uses: org/tests@v1}]\n", workflows.Options{})
	j := r.Workflows[0].Jobs[0]
	for _, e := range []string{"push", "pull_request", "merge_group"} {
		if j.Eligibility[e] != workflows.Unknown {
			t.Fatal("dynamic condition confirmed")
		}
	}
	if j.Matrix.State != workflows.Unknown || j.Runner.State != workflows.Unknown || len(j.Steps[0].Links) != 0 || len(j.Steps[1].Links) != 0 || r.Historical != workflows.Unknown || r.Enforcement != workflows.Unknown {
		t.Fatalf("invented validation: %+v", r)
	}
}
