package workflows_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/workflows"
)

func reviewAnalyze(t *testing.T, content string) workflows.Result {
	t.Helper()
	snapshot := discovery.Snapshot{Documents: []discovery.Document{{Path: ".github/workflows/review.yml", Content: []byte(content)}}}
	inventory := discovery.Result{Roots: []discovery.Root{{ID: "root", Path: "."}}, Candidates: []discovery.Candidate{{ID: "test", Root: "root", Command: "node --test", State: "CONFIGURED", Source: discovery.Source{Path: "package.json", Symbol: "test"}}}}
	beforeSnapshot, _ := json.Marshal(snapshot)
	beforeInventory, _ := json.Marshal(inventory)
	result := workflows.Analyze(snapshot, inventory, workflows.Options{})
	afterSnapshot, _ := json.Marshal(snapshot)
	afterInventory, _ := json.Marshal(inventory)
	if !bytes.Equal(beforeSnapshot, afterSnapshot) || !bytes.Equal(beforeInventory, afterInventory) {
		t.Fatal("Analyze mutated input")
	}
	return result
}

func TestReviewUnsupportedConditionNeeds(t *testing.T) {
	for _, tt := range []struct {
		name, condition string
		want            workflows.State
	}{
		{"unsupported", "    if: always()\n", workflows.Unknown},
		{"default", "", workflows.Ineligible},
		{"supported true", "    if: true\n", workflows.Ineligible},
		{"supported event", "    if: github.event_name == 'push'\n", workflows.Ineligible},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := reviewAnalyze(t, "on: push\njobs:\n  prerequisite:\n    if: false\n    runs-on: ubuntu-latest\n  validation:\n    needs: prerequisite\n"+tt.condition+"    runs-on: ubuntu-latest\n    steps: [{run: npm test}]\n")
			j := r.Workflows[0].Jobs[1]
			if j.Eligibility["push"] != tt.want || j.Steps[0].Eligibility["push"] != tt.want || j.Historical != workflows.Unknown {
				t.Errorf("job=%s step=%s historical=%s; want %s, %s, UNKNOWN", j.Eligibility["push"], j.Steps[0].Eligibility["push"], j.Historical, tt.want, tt.want)
			}
			if tt.name == "unsupported" {
				found := false
				for _, d := range r.Diagnostics {
					if d.Code == "unresolved-condition" {
						found = true
						if d.State != workflows.Unknown || d.Source != (workflows.Source{Path: ".github/workflows/review.yml", Line: 8, Column: 9}) {
							t.Errorf("diagnostic: %+v", d)
						}
					}
				}
				if !found {
					t.Error("missing unresolved condition")
				}
			}
		})
	}
}

func TestReviewCommandSeparators(t *testing.T) {
	for _, tt := range []struct {
		name, command string
		linked        bool
	}{
		{"ascii space", "npm test", true}, {"ascii tab", "npm\ttest", true}, {"harmless trailing newline", "npm test\n", true},
		{"nbsp internal", "npm\u00a0test", false}, {"nbsp prefix", "\u00a0npm test", false}, {"nbsp suffix", "npm test\u00a0", false},
		{"vertical tab internal", "npm\vtest", false}, {"form feed prefix", "\fnpm test", false}, {"control suffix", "npm test\x01", false},
		{"internal newline", "npm\ntest", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := reviewAnalyze(t, fmt.Sprintf("on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    steps:\n      - shell: bash\n        run: %q\n", tt.command))
			s := r.Workflows[0].Jobs[0].Steps[0]
			source := workflows.Source{Path: ".github/workflows/review.yml", Line: 7, Column: 14}
			if s.RunValue.Source != source || s.RunValue.Raw != tt.command || s.RunValue.State != workflows.Configured {
				t.Errorf("run value: %+v", s.RunValue)
			}
			if tt.linked {
				if len(s.Links) != 1 || s.Links[0].State != workflows.Configured || s.Links[0].Chain[0] != source || s.Links[0].Chain[1].Symbol != "test" {
					t.Errorf("supported link: %+v", s.Links)
				}
			} else {
				if len(s.Links) != 0 {
					t.Errorf("unsupported separator fabricated link: %+v", s.Links)
				}
				found := false
				for _, d := range r.Diagnostics {
					if d.Code == "unsupported-command-separator" {
						found = true
						if d.State != workflows.Unknown || d.Source != source {
							t.Errorf("diagnostic: %+v", d)
						}
					}
				}
				if !found {
					t.Error("missing source-backed unsupported separator diagnostic")
				}
			}
		})
	}
}

func TestReviewMatrixIncludeProvenance(t *testing.T) {
	r := reviewAnalyze(t, "on: push\njobs:\n  j:\n    runs-on: ubuntu-latest\n    strategy:\n      matrix:\n        axis: [a,b]\n        include:\n          - tag: green\n          - axis: a\n            tag: green\n")
	m := r.Workflows[0].Jobs[0].Matrix
	if m.State != workflows.Configured || !reflect.DeepEqual(m.Combinations, []map[string]string{{"axis": "a", "tag": "green"}, {"axis": "b", "tag": "green"}}) {
		t.Fatalf("matrix: %+v", m)
	}
	for i, want := range []struct {
		axis                                     string
		tagLine, tagColumn, axisLine, axisColumn int
	}{{"a", 11, 18, 10, 19}, {"b", 9, 18, 7, 18}} {
		values := m.Literals[i]
		if values["tag"].Raw != "green" || values["tag"].State != workflows.Configured || values["tag"].Source != (workflows.Source{Path: ".github/workflows/review.yml", Line: want.tagLine, Column: want.tagColumn}) {
			t.Errorf("axis=%s tag=%+v", want.axis, values["tag"])
		}
		if values["axis"].Raw != want.axis || values["axis"].Source != (workflows.Source{Path: ".github/workflows/review.yml", Line: want.axisLine, Column: want.axisColumn}) {
			t.Errorf("axis=%s source=%+v", want.axis, values["axis"])
		}
	}
}
