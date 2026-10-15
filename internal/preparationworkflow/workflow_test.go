// Package preparationworkflow checks the source-only CI boundary without running Actions.
package preparationworkflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestManualPreparationAndExactReviewBoundary(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "owned-image-preparation.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow struct {
		On          map[string]any    `yaml:"on"`
		Permissions map[string]string `yaml:"permissions"`
		Jobs        map[string]struct {
			If          string                     `yaml:"if"`
			Needs       string                     `yaml:"needs"`
			RunsOn      string                     `yaml:"runs-on"`
			Timeout     int                        `yaml:"timeout-minutes"`
			Environment struct{ Name, URL string } `yaml:"environment"`
			Outputs     map[string]string          `yaml:"outputs"`
			Steps       []struct {
				ID, Uses, Run string
				With          map[string]any    `yaml:"with"`
				Env           map[string]string `yaml:"env"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(b, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.On) != 1 {
		t.Fatalf("manual-only events: %#v", workflow.On)
	}
	if _, ok := workflow.On["workflow_dispatch"]; !ok {
		t.Fatal("manual dispatch missing")
	}
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		t.Fatal("unexpected token permissions")
	}
	if len(workflow.Jobs) != 2 {
		t.Fatal("expected preparation and protected review only")
	}
	prepare, ok := workflow.Jobs["prepare"]
	if !ok {
		t.Fatal("missing prepare job")
	}
	review, ok := workflow.Jobs["review"]
	if !ok {
		t.Fatal("missing review job")
	}
	if prepare.If != "github.repository == 'dnlrsls/ci-trust' && github.ref == 'refs/heads/main'" {
		t.Fatal("untrusted repository/ref scope")
	}
	if review.Needs != "prepare" || review.Environment.Name != "ci-trust-controlled-verification" {
		t.Fatal("missing protected dependent review")
	}
	if !strings.Contains(review.Environment.URL, "github.run_id") {
		t.Fatal("review must link displayed run plan")
	}
	if prepare.Outputs["plan"] != "${{ steps.plan.outputs.plan }}" || prepare.Outputs["binding"] != "${{ steps.plan.outputs.binding }}" {
		t.Fatal("unbound preparation outputs")
	}
	for name, job := range workflow.Jobs {
		if job.RunsOn != "ubuntu-24.04" || job.Timeout < 1 || job.Timeout > 10 {
			t.Fatalf("unbounded/non-Linux %s", name)
		}
		for _, step := range job.Steps {
			if step.Uses != "" {
				switch step.Uses {
				case "actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803":
					if step.With["persist-credentials"] != false || step.With["ref"] != "${{ github.sha }}" {
						t.Fatal("checkout must bind exact revision without credentials")
					}
				case "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16":
					if step.With["cache"] != false {
						t.Fatal("unexpected shared tool cache")
					}
				default:
					t.Fatalf("unreviewed action %q", step.Uses)
				}
			}
			// Values from expressions are data, never interpolated into shell source.
			if strings.Contains(step.Run, "${{") {
				t.Fatal("expression interpolation in shell source")
			}
			for _, forbidden := range []string{"docker ", "sudo ", "curl ", "wget ", "npm ", "runner.Consent", "--approved", "owned-smoke --fixture"} {
				if strings.Contains(step.Run, forbidden) {
					t.Fatalf("unexpected execution/provisioning surface %q", forbidden)
				}
			}
		}
	}
	var prep, check string
	for _, step := range prepare.Steps {
		prep += step.Run + "\n"
	}
	for _, step := range review.Steps {
		check += step.Run + "\n"
		if strings.Contains(step.Run, "controlled-plan check") {
			if step.Env["PLAN_BASE64"] != "${{ needs.prepare.outputs.plan }}" || step.Env["EXPECTED_BINDING"] != "${{ needs.prepare.outputs.binding }}" {
				t.Fatal("review must consume exact upstream outputs")
			}
		}
	}
	for _, required := range []string{"owned-image pack", "owned-image inspect", "controlled-plan prepare", "GITHUB_STEP_SUMMARY", "--run-id", "--run-attempt", "git archive"} {
		if !strings.Contains(prep, required) {
			t.Fatalf("missing preparation %q", required)
		}
	}
	for _, required := range []string{"controlled-plan check", "--binding", "--revision", "--run-id", "--run-attempt", "GITHUB_SHA", "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT"} {
		if !strings.Contains(check, required) {
			t.Fatalf("missing review context %q", required)
		}
	}
}
