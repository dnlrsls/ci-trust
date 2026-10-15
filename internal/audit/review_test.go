package audit

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

func reviewReport(t *testing.T, files map[string]string, o Options) map[string]any {
	t.Helper()
	s := snapshot(files)
	before, _ := json.Marshal(s)
	r := report(t, s, o)
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("input mutated")
	}
	for _, x := range r["evidence"].([]any) {
		if x.(map[string]any)["state"] == "EXECUTED" {
			t.Fatal("execution fabricated")
		}
	}
	return r
}

func TestReviewSameLineSteps(t *testing.T) {
	w := "on: push\njobs:\n  test:\n    runs-on: ubuntu-latest\n    steps: [{run: 'go test ./...'}, {run: 'go build ./...'}]\n"
	r := reviewReport(t, map[string]string{".github/workflows/ci.yml": w}, Options{})
	ids := map[string]bool{}
	cols := []float64{}
	for _, x := range r["evidence"].([]any) {
		e := x.(map[string]any)
		scope := e["scope"].(map[string]any)
		if scope["kind"] != "STEP" {
			continue
		}
		id := scope["subject"].(string)
		if ids[id] {
			continue
		}
		ids[id] = true
		sources := e["provenance"].(map[string]any)["sources"].([]any)
		anchor := sources[0].(map[string]any)
		if anchor["line"] != float64(5) {
			t.Fatal(anchor)
		}
		cols = append(cols, anchor["column"].(float64))
	}
	sort.Float64s(cols)
	if len(cols) != 2 || cols[0] != 13 || cols[1] != 37 {
		t.Fatalf("anchors %v", cols)
	}
}

func TestReviewDeclarationKeys(t *testing.T) {
	for _, d := range []string{`{"version":1,"Platforms":["linux","windows","macos"]}`, `{"Version":1,"platforms":["linux"]}`, `{"version":1,"platforms":["linux"],"Platforms":["windows"]}`, `{"version":1,"version":1,"platforms":["linux"]}`, `{"version":1,"platforms":["linux"],"extra":0}`} {
		t.Run(d, func(t *testing.T) {
			files := map[string]string{".ci-trust.json": d, ".github/workflows/ci.yml": linuxWorkflow}
			r := reviewReport(t, files, Options{})
			if len(findings(r, "CIT_DECLARATION_UNKNOWN")) != 1 || len(findings(r, "CIT_PLATFORM_CONFIGURED_GAP")) != 0 {
				t.Fatal("malformed declaration accepted")
			}
			for _, x := range r["evidence"].([]any) {
				e := x.(map[string]any)
				if e["id"] == evidence.StableID("evidence", "declaration") {
					p := e["provenance"].(map[string]any)
					if e["state"] != "UNKNOWN" || len(p["sources"].([]any)) != 0 {
						t.Fatal(e)
					}
				}
			}
			override := reviewReport(t, files, Options{Platforms: []string{"linux"}})
			if len(findings(override, "CIT_DECLARATION_UNKNOWN")) != 0 {
				t.Fatal("override rejected")
			}
		})
	}
	reviewReport(t, map[string]string{".ci-trust.json": validDeclaration, ".github/workflows/ci.yml": linuxWorkflow}, Options{})
}

func TestReviewLinkedCategories(t *testing.T) {
	for _, command := range []string{"node --test && eslint .", "eslint . && node --test"} {
		t.Run(command, func(t *testing.T) {
			r := reviewReport(t, map[string]string{"package.json": `{"scripts":{"test":"` + command + `"}}`, ".ci-trust.json": `{"version":1,"platforms":["linux"]}`, ".github/workflows/ci.yml": strings.ReplaceAll(linuxWorkflow, "go test ./...", "npm test")}, Options{})
			kinds := map[string]bool{}
			for _, x := range r["capabilities"].([]any) {
				c := x.(map[string]any)
				for _, k := range []string{"TEST", "LINT"} {
					for _, e := range r["evidence"].([]any) {
						ev := e.(map[string]any)
						scope := ev["scope"].(map[string]any)
						if scope["kind"] == "STEP" && c["id"] == evidence.StableID("capability", scope["subject"].(string)+"-"+k, "CONFIGURATION") {
							kinds[k] = true
						}
					}
				}
			}
			if !kinds["TEST"] || !kinds["LINT"] {
				t.Fatalf("categories %v", kinds)
			}
			if len(findings(r, "CIT_PLATFORM_CONFIGURED_GAP")) != 0 {
				t.Fatal("false linux gap")
			}
		})
	}
}

func TestReviewPathBound(t *testing.T) {
	for _, n := range []int{4096, 4097, 1 << 20} {
		s := snapshot(map[string]string{strings.Repeat("a", n): "x"})
		before, _ := json.Marshal(s)
		b, err := Analyze(s, Options{})
		if n > 4096 && (err == nil || len(b) != 0 || err.Error() != "invalid or ambiguous snapshot path") {
			t.Fatalf("n=%d bytes=%d err=%v", n, len(b), err)
		}
		if n == 4096 && err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(s)
		if !bytes.Equal(before, after) {
			t.Fatal("input mutated")
		}
	}
}
