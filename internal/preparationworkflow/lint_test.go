package preparationworkflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestPortableGoCICoversPreparationWithoutPython(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "test.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(string(b)), "python") {
		t.Fatal("redundant legacy Python CI step")
	}
	var workflow workflowSource
	if err := yaml.Unmarshal(b, &workflow); err != nil {
		t.Fatal(err)
	}
	job := workflow.Jobs["test"]
	if len(workflow.Jobs) != 1 || job.Timeout != 10 || job.RunsOn != "${{ matrix.os }}" || len(workflow.Permissions) != 1 || workflow.Permissions["contents"] != "read" {
		t.Fatal("portable CI scope")
	}
	var matrix struct {
		Jobs map[string]struct {
			Steps    []struct{ Run, If string }
			Strategy struct {
				FailFast bool `yaml:"fail-fast"`
				Matrix   struct {
					OS []string `yaml:"os"`
				}
			}
		}
	}
	if err := yaml.Unmarshal(b, &matrix); err != nil {
		t.Fatal(err)
	}
	for _, step := range matrix.Jobs["test"].Steps {
		if strings.Contains(step.Run, "go test") && step.If != "" {
			t.Fatal("portable Go tests must run on every matrix host")
		}
	}
	m := matrix.Jobs["test"].Strategy
	if m.FailFast || strings.Join(m.Matrix.OS, ",") != "ubuntu-latest,windows-latest,macos-latest" {
		t.Fatal("portable three-host matrix")
	}
	tests, vets := 0, 0
	for _, step := range job.Steps {
		if strings.Contains(step.Run, "go test") {
			tests++
			if strings.TrimSpace(step.Run) != "go test -count=1 -v ./..." {
				t.Fatal("one all-package Go suite")
			}
		}
		if strings.TrimSpace(step.Run) == "go vet ./..." {
			vets++
		}
		if strings.HasPrefix(step.Uses, "actions/setup-go@") && (step.Uses != "actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16" || step.With["go-version"] != "1.26.x" || step.With["cache"] != false) {
			t.Fatal("portable compiler")
		}
	}
	if tests != 1 || vets != 1 {
		t.Fatal("portable tests/vet")
	}
}

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
	var environment struct {
		Env map[string]string `yaml:"env"`
	}
	if err := yaml.Unmarshal(b, &environment); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"GOTOOLCHAIN": "local", "GOPROXY": "off", "GOSUMDB": "off", "GOTELEMETRY": "off", "GOENV": "off", "GOWORK": "off", "GOFLAGS": "", "GOAUTH": "off", "GOVCS": "*:off"} {
		if got, ok := environment.Env[key]; !ok || got != value {
			t.Fatalf("offline environment %s=%q", key, got)
		}
	}
	if len(environment.Env) != 9 {
		t.Fatal("unexpected workflow environment")
	}
	if strings.Contains(strings.ToLower(string(b)), "python") {
		t.Fatal("legacy Python workflow invocation/environment")
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
				wantFiles := []string{"image.tar", "package.json", "package-lock.json", "inventory.json", "owned-inputs.json", "preparation.json", "SHA256SUMS"}
				for i, name := range wantFiles {
					if files[i] != ".git/lint-image-preparation/artifacts/"+name {
						t.Fatal("fixed seven-file upload scope")
					}
				}
				if step.With["include-hidden-files"] != true || step.With["compression-level"] != 1 || step.With["name"] != "lint-preparation-${{ github.sha }}-${{ github.run_id }}-${{ github.run_attempt }}" {
					t.Fatal("artifact binding")
				}
			default:
				t.Fatalf("unreviewed action %s", step.Uses)
			}
		}
	}
	if uploads != 1 {
		t.Fatal("fixed artifact upload required")
	}
	for _, required := range []string{"CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid=", "./internal/lintlauncher/cmd/owned-eslint", "./internal/lintpreparation/cmd/owned-lint-inputs", `timeout --signal=TERM --kill-after=10s 600s "$prep/owned-lint-prepare"`, `test "$(git rev-parse HEAD)" = "$GITHUB_SHA"`, `test -z "$(git status --porcelain --untracked-files=all)"`, "GITHUB_STEP_SUMMARY", "NOT admitted", "security hold"} {
		if !strings.Contains(source, required) {
			t.Fatalf("missing %q", required)
		}
	}
	ordered := []string{
		`go test ./internal/lintpreparation/... -count=1 -timeout=20s`,
		`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$prep/owned-eslint" ./internal/lintlauncher/cmd/owned-eslint`,
		`GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags=-buildid= -o "$prep/owned-lint-prepare" ./internal/lintpreparation/cmd/owned-lint-prepare`,
		`go run ./internal/lintpreparation/cmd/owned-lint-inputs > "$prep/inputs.json"`,
		`timeout --signal=TERM --kill-after=10s 600s "$prep/owned-lint-prepare"`,
	}
	previous := -1
	for _, command := range ordered {
		i := strings.Index(source, command)
		if i <= previous || strings.Count(source, command) != 1 {
			t.Fatalf("missing, repeated or reordered %q", command)
		}
		previous = i
	}
	var shells struct {
		Jobs map[string]struct{ Steps []struct{ Run, Shell string } }
	}
	if err := yaml.Unmarshal(b, &shells); err != nil {
		t.Fatal(err)
	}
	for _, step := range shells.Jobs["prepare"].Steps {
		if step.Run == "" {
			continue
		}
		for _, required := range []string{"set -euo pipefail", "umask 077", `prep="$GITHUB_WORKSPACE/.git/lint-image-preparation"`} {
			if !strings.Contains(step.Run, required) {
				t.Fatalf("missing owned shell setup %q", required)
			}
		}
		if step.Shell != "bash" {
			t.Fatal("fixed shell")
		}
		if strings.Contains(step.Run, "go test ") || strings.Contains(step.Run, "go build ") {
			for _, required := range []string{`mkdir -p "$prep/tmp" "$prep/cache" "$prep/modcache"`, `export GOCACHE="$prep/cache" GOMODCACHE="$prep/modcache" TEMP="$prep/tmp" TMP="$prep/tmp" TMPDIR="$prep/tmp"`} {
				if !strings.Contains(step.Run, required) {
					t.Fatal("private offline cache/tmp setup")
				}
			}
		}
		if i := strings.Index(step.Run, "timeout --signal=TERM"); i >= 0 && !strings.Contains(step.Run[:i], `prep="$GITHUB_WORKSPACE/.git/lint-image-preparation"`) {
			t.Fatal("prep must precede invocation")
		}
	}
	for _, forbidden := range []string{"docker ", "sudo ", "npx ", "npm run", "owned-eslint --", "controlled-verify", "--approve", "GITHUB_TOKEN", "go run ./internal/lintpreparation/cmd/owned-lint-prepare"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("execution surface %q", forbidden)
		}
	}
}
