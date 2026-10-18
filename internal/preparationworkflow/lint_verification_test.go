package preparationworkflow

import (
	"go.yaml.in/yaml/v3"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lintVerificationSource(t *testing.T) workflowSource {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "lint-sensitivity-verification.yml"))
	if e != nil {
		t.Fatal(e)
	}
	var w workflowSource
	if yaml.Unmarshal(b, &w) != nil {
		t.Fatal("bad lint workflow YAML")
	}
	return w
}
func TestLintVerificationManualOwnerProtectedBoundary(t *testing.T) {
	w := lintVerificationSource(t)
	dispatch, ok := w.On["workflow_dispatch"]
	if !ok || len(w.On) != 1 || len(dispatch.Inputs) != 0 || len(w.Jobs) != 2 {
		t.Error("not fixed manual-only lint route")
	}
	if len(w.Permissions) != 1 || w.Permissions["contents"] != "read" {
		t.Error("unexpected default permissions")
	}
	scope := "github.repository == 'dnlrsls/ci-trust' && github.actor == github.repository_owner && github.ref == 'refs/heads/main'"
	for _, name := range []string{"plan", "verify"} {
		j := w.Jobs[name]
		if strings.Join(strings.Fields(j.If), " ") != scope || j.RunsOn != "ubuntu-24.04" || j.Timeout < 1 || j.Timeout > 15 {
			t.Error("unscoped lint job", name)
		}
	}
	p, v := w.Jobs["plan"], w.Jobs["verify"]
	if p.Environment.Name != "" || v.Environment.Name != "ci-trust-controlled-verification" || v.Environment.URL != "https://github.com/dnlrsls/ci-trust/actions/runs/${{ github.run_id }}" || v.Needs != "plan" {
		t.Error("missing separate protected human boundary")
	}
	if p.Outputs["request"] != "${{ steps.plan.outputs.request }}" {
		t.Error("missing exact planning output")
	}
	if len(v.Permissions) != 2 || v.Permissions["contents"] != "read" || v.Permissions["actions"] != "read" {
		t.Error("execution audit permissions not read-only")
	}
}
func TestLintVerificationFixedStepsNoShortcuts(t *testing.T) {
	w := lintVerificationSource(t)
	pins := map[string]bool{
		"actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803":        true,
		"actions/setup-go@924ae3a1cded613372ab5595356fb5720e22ba16":        true,
		"actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02": true}
	for _, name := range []string{"plan", "verify"} {
		j := w.Jobs[name]
		runs := ""
		checkout, compiler := false, false
		for _, s := range j.Steps {
			runs += s.Run + "\n"
			if s.Uses != "" && !pins[s.Uses] {
				t.Error("unreviewed action pin")
			}
			if strings.HasPrefix(s.Uses, "actions/checkout@") {
				checkout = true
				if s.With["ref"] != "${{ github.sha }}" || s.With["persist-credentials"] != false {
					t.Error("unbound checkout")
				}
			}
			if strings.HasPrefix(s.Uses, "actions/setup-go@") {
				compiler = true
				if s.With["go-version"] != "1.26.4" || s.With["cache"] != false {
					t.Error("unfixed compiler/cache")
				}
			}
		}
		if !checkout || !compiler || !strings.Contains(runs, "go build -mod=readonly") || !strings.Contains(runs, "timeout --signal=TERM --kill-after=10s 600s") {
			t.Error("missing owned bounded control", name)
		}
		for _, bad := range []string{"npm ", "npx ", "node ", "docker run", "owned-lint-prepare", "--approve", "gh api", "curl "} {
			if strings.Contains(runs, bad) {
				t.Error("unowned runtime/approval shortcut", name, bad)
			}
		}
		if name == "plan" && !strings.Contains(runs, "approve lint") {
			t.Error("exact human review comment not displayed")
		}
		if name == "verify" {
			found := false
			for _, s := range j.Steps {
				if s.ID == "verify" {
					found = true
					if s.Env["REQUEST"] != "${{ needs.plan.outputs.request }}" || s.Env["GITHUB_TOKEN"] != "${{ secrets.GITHUB_TOKEN }}" || !strings.Contains(s.Run, "install -d -m 700") || !strings.Contains(s.Run, "controlled-lint\" verify") {
						t.Error("missing protected fixed execution")
					}
				}
			}
			if !found {
				t.Error("missing verify step")
			}
		}
	}
}
