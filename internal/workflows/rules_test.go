package workflows_test

import (
	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/workflows"
	"reflect"
	"testing"
)

func analyze(s string, o workflows.Options) workflows.Result {
	return workflows.Analyze(discovery.Snapshot{Documents: []discovery.Document{{Path: ".github/workflows/ci.yaml", Content: []byte(s)}}}, discovery.Result{}, o)
}
func has(r workflows.Result, code string) bool {
	for _, d := range r.Diagnostics {
		if d.Code == code {
			return true
		}
	}
	return false
}
func TestTriggersNeedsAndSources(t *testing.T) {
	r := analyze("on:\n  pull_request:\n    paths: ['src/**']\n  push:\n    branches: [main]\n  merge_group:\n    types: [checks_requested]\njobs:\n  a:\n    runs-on: ubuntu-latest\n    steps: [{run: echo ok}]\n  b:\n    needs: a\n    runs-on: ubuntu-latest\n", workflows.Options{})
	w := r.Workflows[0]
	if len(w.Events) != 3 || w.Events[0].Filters["paths"].Source.Line != 3 || w.Jobs[1].NeedSources[0].Line != 13 {
		t.Fatalf("sources: %+v", w)
	}
	if w.Jobs[0].Eligibility["push"] != workflows.Conditional {
		t.Fatal("branch-filtered eligibility must remain conditional")
	}
}
func TestMatrixExactness(t *testing.T) {
	r := analyze("on: push\njobs:\n  a:\n    strategy:\n      matrix:\n        os: [ubuntu, windows]\n        node: [18, 20]\n        exclude: [{os: windows, node: 18}]\n        include: [{color: green}, {os: ubuntu, color: pink}, {os: mac, node: 22}]\n", workflows.Options{})
	want := []map[string]string{{"os": "ubuntu", "node": "18", "color": "pink"}, {"os": "ubuntu", "node": "20", "color": "pink"}, {"os": "windows", "node": "20", "color": "green"}, {"os": "mac", "node": "22"}}
	if !reflect.DeepEqual(r.Workflows[0].Jobs[0].Matrix.Combinations, want) {
		t.Fatalf("matrix: %+v", r)
	}
	for _, s := range []string{"'${{ fromJSON(inputs.matrix) }}'", "{os: [a,b,c]}"} {
		r := analyze("on: push\njobs:\n  a:\n    strategy:\n      matrix: "+s+"\n", workflows.Options{MaxMatrix: 2})
		m := r.Workflows[0].Jobs[0].Matrix
		if m.State != workflows.Unknown || len(m.Combinations) != 0 {
			t.Fatalf("invented partial coverage: %+v", m)
		}
	}
}
func TestConditionsAndDependencyStates(t *testing.T) {
	for _, tt := range []struct {
		expr string
		want workflows.State
	}{{"true", workflows.Eligible}, {"false", workflows.Ineligible}, {"github.actor == 'bot'", workflows.Unknown}, {"${{ github.event_name != 'push' }}", workflows.Ineligible}} {
		t.Run(tt.expr, func(t *testing.T) {
			r := analyze("on: push\njobs:\n  a:\n    if: \""+tt.expr+"\"\n    continue-on-error: true\n    runs-on: ubuntu-latest\n    steps: [{run: echo ok, if: false, continue-on-error: true}]\n  b:\n    needs: a\n    runs-on: ubuntu-latest\n", workflows.Options{})
			j := r.Workflows[0].Jobs[0]
			if j.Eligibility["push"] != tt.want || j.Historical != workflows.Unknown || j.Enforcement != workflows.Unknown || j.Steps[0].Eligibility["push"] != workflows.Ineligible || j.Settings["continue-on-error"].Raw != "true" {
				t.Fatalf("states: %+v", j)
			}
			b := r.Workflows[0].Jobs[1]
			want := tt.want
			if want == workflows.Eligible {
				want = workflows.Conditional
			}
			if b.Eligibility["push"] != want {
				t.Fatalf("dependency: %+v", b)
			}
		})
	}
}
func TestSettingsScope(t *testing.T) {
	r := analyze("on: push\npermissions: {contents: read}\nconcurrency:\n  group: ci\n  cancel-in-progress: true\njobs:\n  a:\n    timeout-minutes: '${{ inputs.timeout }}'\n    strategy: {fail-fast: false}\n    steps: [{run: echo ok}]\n", workflows.Options{})
	w := r.Workflows[0]
	j := w.Jobs[0]
	if !j.Settings["permissions"].Inherited || j.Settings["permissions"].Scope != "workflow" || j.Settings["permissions"].Source.Line != 2 || j.Settings["timeout-minutes"].State != workflows.Unknown || j.Settings["fail-fast"].Raw != "false" || w.Settings["cancel-in-progress"].Raw != "true" {
		t.Fatalf("settings: %+v %+v", w, j)
	}
}
func TestHostileInputAndBounds(t *testing.T) {
	for _, tt := range []struct{ name, yaml, code string }{{"duplicate", "on: push\non: pull_request", "duplicate-or-merge-key"}, {"alias", "on: push\na: &x {b: 1}\njobs: *x", "yaml-alias"}, {"merge", "on: push\njobs: {a: {<<: {x: y}}}", "duplicate-or-merge-key"}, {"multi", "on: push\n---\non: pull_request", "multiple-documents"}, {"malformed", "on: [", "malformed-yaml"}} {
		t.Run(tt.name, func(t *testing.T) {
			r := analyze(tt.yaml, workflows.Options{})
			if !has(r, tt.code) || len(r.Workflows) != 0 {
				t.Fatalf("rejection: %+v", r)
			}
		})
	}
	for _, o := range []workflows.Options{{MaxBytes: 1}, {MaxNodes: 1}, {MaxDepth: 1}, {MaxMatrix: 4097}, {MaxDocuments: -1}} {
		r := analyze("on: push\njobs: {}", o)
		if len(r.Diagnostics) == 0 || len(r.Workflows) != 0 {
			t.Fatalf("bypass: %+v", r)
		}
	}
	r := workflows.Analyze(discovery.Snapshot{Documents: []discovery.Document{{Path: "sub/.github/workflows/a.yml", Content: []byte("on: push")}, {Path: ".github/workflows/sub/a.yml", Content: []byte("on: push")}}}, discovery.Result{}, workflows.Options{})
	if len(r.Workflows) != 0 {
		t.Fatal("non-root workflows interpreted")
	}
}
func TestSourceBackedLimitations(t *testing.T) {
	for _, tt := range []struct{ body, code string }{{"    needs: absent\n    runs-on: ubuntu-latest\n", "missing-needs-target"}, {"    uses: org/repo/.github/workflows/a.yml@main\n", "reusable-workflow-unresolved"}, {"    steps: [{run: npm test, shell: pwsh}]\n", "unsupported-shell"}, {"    steps: [{uses: actions/checkout@v4}]\n", "action-internals-unresolved"}} {
		t.Run(tt.code, func(t *testing.T) {
			r := analyze("on: push\njobs:\n  a:\n"+tt.body, workflows.Options{})
			for _, d := range r.Diagnostics {
				if d.Code == tt.code {
					if d.Source.Line == 0 || d.Source.Column == 0 {
						t.Fatalf("unanchored: %+v", d)
					}
					return
				}
			}
			t.Fatalf("missing %s: %+v", tt.code, r)
		})
	}
	r := analyze("on: push\njobs:\n  a: {needs: b}\n  b: {needs: a}\n", workflows.Options{})
	if !has(r, "needs-cycle") || r.Workflows[0].Jobs[0].Eligibility["push"] != workflows.Unknown {
		t.Fatalf("cycle: %+v", r)
	}
}
func TestExactInventoryLink(t *testing.T) {
	inv := discovery.Result{Roots: []discovery.Root{{ID: "root", Path: "pkg"}}, Candidates: []discovery.Candidate{{ID: "test", Root: "root", Source: discovery.Source{Path: "pkg/package.json", Symbol: "test"}, Command: "node tests.js", State: "CONFIGURED"}, {ID: "lint", Root: "root", Source: discovery.Source{Path: "pkg/package.json", Symbol: "lint"}, Command: "eslint .", State: "CONFIGURED"}}}
	for _, tt := range []struct {
		cmd, wd string
		count   int
	}{{"npm test", "pkg", 1}, {"npm run test", "pkg", 1}, {"npm test", ".", 0}, {"npm test && echo x", "pkg", 0}, {"npm run missing", "pkg", 0}} {
		t.Run(tt.cmd+tt.wd, func(t *testing.T) {
			r := workflows.Analyze(discovery.Snapshot{Documents: []discovery.Document{{Path: ".github/workflows/ci.yml", Content: []byte("on: push\njobs:\n  a:\n    steps:\n      - run: " + tt.cmd + "\n        working-directory: " + tt.wd + "\n")}}}, inv, workflows.Options{})
			s := r.Workflows[0].Jobs[0].Steps[0]
			if len(s.Links) != tt.count {
				t.Fatalf("links: %+v", s.Links)
			}
			if tt.count > 0 && (s.Links[0].Chain[0].Line != 5 || s.Links[0].Chain[1].Symbol != "test") {
				t.Fatal("provenance chain")
			}
		})
	}
}
