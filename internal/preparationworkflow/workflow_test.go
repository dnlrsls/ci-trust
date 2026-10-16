// Package preparationworkflow checks the source CI boundary without running Actions.
package preparationworkflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

type workflowStep struct {
	Name, ID, Uses, Run string
	With                map[string]any    `yaml:"with"`
	Env                 map[string]string `yaml:"env"`
}
type workflowJob struct {
	If          string                     `yaml:"if"`
	Needs       string                     `yaml:"needs"`
	RunsOn      string                     `yaml:"runs-on"`
	Timeout     int                        `yaml:"timeout-minutes"`
	Permissions map[string]string          `yaml:"permissions"`
	Environment struct{ Name, URL string } `yaml:"environment"`
	Outputs     map[string]string          `yaml:"outputs"`
	Steps       []workflowStep             `yaml:"steps"`
}
type workflowSource struct {
	On map[string]struct {
		Inputs map[string]struct {
			Type     string
			Required bool
			Default  bool
		} `yaml:"inputs"`
	} `yaml:"on"`
	Permissions map[string]string      `yaml:"permissions"`
	Jobs        map[string]workflowJob `yaml:"jobs"`
}

func ownedWorkflow(t *testing.T) workflowSource {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "owned-image-preparation.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var workflow workflowSource
	if err := yaml.Unmarshal(b, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow
}

func TestManualPreparationAndExactReviewBoundary(t *testing.T) {
	workflow := ownedWorkflow(t)
	if len(workflow.On) != 1 {
		t.Fatal("manual-only events required")
	}
	dispatch, ok := workflow.On["workflow_dispatch"]
	if !ok || len(dispatch.Inputs) != 1 {
		t.Fatal("single explicit dispatch mode required")
	}
	mode, ok := dispatch.Inputs["execution_proof"]
	if !ok || mode.Type != "boolean" || !mode.Required || mode.Default {
		t.Fatal("execution must be opt-in, false by default")
	}
	if len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		t.Fatal("unexpected default token permissions")
	}
	if len(workflow.Jobs) != 3 {
		t.Fatal("expected preparation, default review and opt-in execution only")
	}
	prepare, ok := workflow.Jobs["prepare"]
	if !ok {
		t.Fatal("missing prepare job")
	}
	review, ok := workflow.Jobs["review"]
	if !ok {
		t.Fatal("missing default review job")
	}
	execute, ok := workflow.Jobs["execute"]
	if !ok {
		t.Fatal("missing opt-in execution job")
	}
	wantScope := "github.repository == 'dnlrsls/ci-trust' && github.actor == github.repository_owner && (github.ref == 'refs/heads/main' || (inputs.execution_proof == true && github.ref == 'refs/heads/feat/issue8-real-isolation'))"
	if strings.Join(strings.Fields(prepare.If), " ") != wantScope {
		t.Fatalf("untrusted repository/ref/actor scope: %q", prepare.If)
	}
	if review.If != "${{ !inputs.execution_proof }}" || execute.If != "${{ inputs.execution_proof }}" {
		t.Fatal("default review and execution must be mutually exclusive")
	}
	if len(prepare.Permissions) != 0 || len(review.Permissions) != 0 {
		t.Fatal("source-only jobs must inherit contents:read only")
	}
	if len(execute.Permissions) != 2 || execute.Permissions["contents"] != "read" || execute.Permissions["actions"] != "read" {
		t.Fatal("execution audit must be read-only and minimally scoped")
	}
	for name, job := range map[string]workflowJob{"review": review, "execute": execute} {
		if job.Needs != "prepare" || job.Environment.Name != "ci-trust-controlled-verification" || job.Environment.URL != "https://github.com/dnlrsls/ci-trust/actions/runs/${{ github.run_id }}" {
			t.Fatalf("missing run-bound protected boundary: %s", name)
		}
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
					if step.With["cache"] != false || step.With["go-version"] != "1.26.4" {
						t.Fatal("compiler identity must be exact; shared cache forbidden")
					}
				default:
					t.Fatalf("unreviewed action %q", step.Uses)
				}
			}
			if strings.Contains(step.Run, "${{") {
				t.Fatal("expression interpolation in shell source")
			}
			for _, forbidden := range []string{"docker ", "sudo ", "curl ", "wget ", "npm ", "runner.Consent", "--approved", "owned-smoke --fixture"} {
				if strings.Contains(step.Run, forbidden) {
					t.Fatalf("unexpected direct execution/provisioning surface %q", forbidden)
				}
			}
			isExecution := strings.Contains(step.Run, `"$prep/controlled-verify" execute`)
			if isExecution && name != "execute" {
				t.Fatal("runtime entry outside opt-in protected job")
			}
			if token, exists := step.Env["GITHUB_TOKEN"]; exists && (!isExecution || token != "${{ github.token }}") {
				t.Fatal("audit credential outside fixed execution step")
			}
		}
	}
	var prep, check string
	for _, step := range prepare.Steps {
		prep += step.Run + "\n"
		if step.ID == "plan" && step.Env["EXECUTION_PROOF"] != "${{ inputs.execution_proof }}" {
			t.Fatal("mode must enter shell only as data")
		}
	}
	for _, step := range review.Steps {
		check += step.Run + "\n"
		if strings.Contains(step.Run, "controlled-plan check") && (step.Env["PLAN_BASE64"] != "${{ needs.prepare.outputs.plan }}" || step.Env["EXPECTED_BINDING"] != "${{ needs.prepare.outputs.binding }}") {
			t.Fatal("default review must consume exact upstream outputs")
		}
	}
	for _, required := range []string{"owned-image pack", "owned-image inspect", "controlled-plan prepare", "controlled-verify prepare", "GITHUB_STEP_SUMMARY", "--run-id", "--run-attempt", "git archive", `if [[ "$EXECUTION_PROOF" == true ]]`, "git status --porcelain --untracked-files=all", "approve %s", "not consent", "retained"} {
		if !strings.Contains(prep, required) {
			t.Fatalf("missing preparation/display %q", required)
		}
	}
	for _, required := range []string{"controlled-plan check", "--binding", "--revision", "--run-id", "--run-attempt", "GITHUB_SHA", "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT"} {
		if !strings.Contains(check, required) {
			t.Fatalf("missing default review context %q", required)
		}
	}
}

func TestOptInExecutionReproducesExactDataAndPreservesEvidence(t *testing.T) {
	job := ownedWorkflow(t).Jobs["execute"]
	var build, execution string
	executionSteps := 0
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "go build") {
			build += step.Run
			if step.Env["PLAN_BASE64"] != "${{ needs.prepare.outputs.plan }}" {
				t.Fatal("reproduction requires upstream exact plan")
			}
			if _, ok := step.Env["GITHUB_TOKEN"]; ok {
				t.Fatal("compiler must not receive audit token")
			}
		}
		if strings.Contains(step.Run, `"$prep/controlled-verify" execute`) {
			executionSteps++
			execution = step.Run
			if step.Env["EXPECTED_BINDING"] != "${{ needs.prepare.outputs.binding }}" || step.Env["GITHUB_TOKEN"] != "${{ github.token }}" {
				t.Fatal("missing exact binding or read-only audit credential")
			}
		}
	}
	if executionSteps != 1 {
		t.Fatal("exactly one fixed execution invocation required")
	}
	for _, required := range []string{"umask 077", `mkdir -p "$prep/tmp" "$prep/cache" "$prep/docker-config"`, "GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid=", "owned-image pack", "./internal/controlledverify/cmd/controlled-verify", "git status --porcelain --untracked-files=all"} {
		if !strings.Contains(build, required) {
			t.Fatalf("missing deterministic/private reproduction %q", required)
		}
	}
	for _, required := range []string{`--input "$prep/plan.json" --binding "$EXPECTED_BINDING"`, `> "$prep/execution.json" 2> "$prep/execution.stderr"`, "result=$?", `exit "$result"`, "GITHUB_STEP_SUMMARY", `cat "$prep/execution.json"`, `cat "$prep/execution.stderr"`, "not a security PASS", "UNVERIFIABLE is not PASS"} {
		if !strings.Contains(execution, required) {
			t.Fatalf("missing bounded invocation/evidence %q", required)
		}
	}
}
