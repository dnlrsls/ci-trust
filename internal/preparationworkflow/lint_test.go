package preparationworkflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestLintPreparationCannotBecomeRuntimeExecution(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "lint-image-preparation.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowSource
	if err := yaml.Unmarshal(b, &workflow); err != nil {
		t.Fatal(err)
	}
	if len(workflow.On) != 1 {
		t.Fatal("manual only")
	}
	if dispatch, ok := workflow.On["workflow_dispatch"]; !ok || len(dispatch.Inputs) != 0 {
		t.Fatal("no arbitrary input or execution mode")
	}
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" || len(workflow.Jobs) != 1 {
		t.Fatal("unexpected permission/job surface")
	}
	job := workflow.Jobs["prepare"]
	want := "github.repository == 'dnlrsls/ci-trust' && github.actor == github.repository_owner && github.ref == 'refs/heads/main'"
	if strings.Join(strings.Fields(job.If), " ") != want || job.Environment.Name != "ci-trust-controlled-verification" || job.Environment.URL != "https://github.com/dnlrsls/ci-trust/actions/runs/${{ github.run_id }}" || job.RunsOn != "ubuntu-24.04" || job.Timeout != 15 || len(job.Permissions) != 0 {
		t.Fatal("scope or protected boundary changed")
	}
	var source string
	uploads := 0
	for _, step := range job.Steps {
		source += step.Run + "\n"
		if strings.Contains(step.Run, "${{") {
			t.Fatal("shell interpolation")
		}
		if len(step.Env) != 0 {
			t.Fatal("no credential injection")
		}
		if step.Uses != "" {
			switch step.Uses {
			case "actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803":
				if step.With["persist-credentials"] != false || step.With["ref"] != "${{ github.sha }}" {
					t.Fatal("checkout scope")
				}
			case "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16":
				if step.With["cache"] != false || step.With["go-version"] != "1.26.4" {
					t.Fatal("compiler scope")
				}
			case "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02":
				uploads++
				paths, ok := step.With["path"].(string)
				if !ok {
					t.Fatal("artifact path")
				}
				files := strings.Fields(paths)
				if len(files) != 7 || step.With["retention-days"] != 7 || step.With["if-no-files-found"] != "error" {
					t.Fatal("unbounded upload")
				}
				for _, p := range files {
					if !strings.HasPrefix(p, ".git/lint-image-preparation/artifacts/") || strings.ContainsAny(p, "*?") {
						t.Fatal("upload scope")
					}
				}
			default:
				t.Fatalf("unreviewed action %s", step.Uses)
			}
		}
	}
	if uploads != 1 {
		t.Fatal("fixed artifact upload required")
	}
	for _, required := range []string{"CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid=", "./internal/lintlauncher/cmd/owned-eslint", "./internal/lintpreparation/cmd/owned-lint-inputs", "600s python3 -I -B tools/lint-preparation/prepare.py", "test -z", "GITHUB_STEP_SUMMARY", "NOT admitted", "security hold"} {
		if !strings.Contains(source, required) {
			t.Fatalf("missing %q", required)
		}
	}
	for _, forbidden := range []string{"docker ", "sudo ", "npx ", "npm run", "owned-eslint --", "controlled-verify", "--approve", "GITHUB_TOKEN"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("execution surface %q", forbidden)
		}
	}
}
