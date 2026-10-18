package lintverify

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/discovery"
	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
	"github.com/dnlrsls/ci-trust/internal/runner"
)

type diagnosticRuntime struct {
	workflowRuntime
	role   ls.Role
	mutate func(*runner.Result)
}

func (r diagnosticRuntime) execute(ctx context.Context, p runner.Plan, c runner.Consent, snapshot runner.Snapshot) runner.Result {
	got := r.workflowRuntime.execute(ctx, p, c, snapshot)
	if p.Fixture == string(r.role) {
		r.mutate(&got)
	}
	return got
}

func TestHarnessFailureReportSafeDetails(t *testing.T) {
	const private = "ghp_private_capture_token /private/runtime-path"
	for _, tc := range []struct {
		name, stage, status, code, exit string
		mutate                          func(*runner.Result)
	}{
		{"launcher", "OUTER_CAPTURE", "UNAVAILABLE", "LAUNCHER_EXIT", "1", func(r *runner.Result) {
			r.Outcome, r.Reason, r.ExitCode = runner.Unverifiable, "lint launcher completion unverified", 1
			r.Stdout, r.Stderr = private, private
		}},
		{"transport", "OUTER_CAPTURE", "UNAVAILABLE", "TRANSPORT_ERROR", "-1", func(r *runner.Result) {
			r.Outcome, r.Reason, r.ExitCode = runner.Unverifiable, "harness: "+private, -1
		}},
		{"limit", "OUTER_CAPTURE", "UNAVAILABLE", "OUTPUT_LIMIT", "0", func(r *runner.Result) {
			r.Outcome, r.Reason = runner.Unverifiable, "output limit exceeded"
		}},
		{"walltime", "OUTER_CAPTURE", "UNAVAILABLE", "WALLTIME_OR_CANCEL", "0", func(r *runner.Result) {
			r.Outcome, r.Reason = runner.Unverifiable, "execution canceled or walltime exceeded"
		}},
		{"unknown", "OUTER_CAPTURE", "UNAVAILABLE", "UNSPECIFIED", "UNAVAILABLE", func(r *runner.Result) {
			r.Outcome, r.Reason, r.ExitCode = runner.Unverifiable, private, 999999
		}},
		{"outer exit", "OUTER_EXIT", "COMPLETED", "UNSPECIFIED", "1", func(r *runner.Result) { r.ExitCode = 1 }},
		{"outer stderr", "OUTER_STDERR", "COMPLETED", "UNSPECIFIED", "0", func(r *runner.Result) { r.Stderr = private }},
		{"receipt", "RECEIPT_OR_ESLINT_OUTPUT", "COMPLETED", "UNSPECIFIED", "0", func(r *runner.Result) { r.Stdout = private }},
	} {
		for index, role := range []ls.Role{ls.Clean, ls.Fault, ls.Restored} {
			t.Run(tc.name+"/"+string(role), func(t *testing.T) {
				s, f := workflowFixture(t, "sensitive")
				original := s.newRuntime
				s.newRuntime = func() (workflowRuntime, error) {
					r, err := original()
					return diagnosticRuntime{r, role, tc.mutate}, err
				}
				source := []byte(`{"scripts":{"lint":"eslint ."}}`)
				snapshot := discovery.Snapshot{Documents: []discovery.Document{{Path: "package.json", Content: source}}}
				b, err := analyze(snapshot, func(hash string) outcome {
					p, _ := planFor(s.catalog, hash)
					review, makeErr := makeGitHubReviewPlan(p, f.r.Revision, f.r.RunID, f.clock)
					if makeErr != nil {
						t.Fatal(makeErr)
					}
					f.r = review
					return evaluateGitHubWorkflow(context.Background(), hash, f.r, s)
				})
				if err != nil {
					t.Fatal(err)
				}
				r := decodeReport(t, b)
				want := "Owned lint sensitivity: UNVERIFIABLE. " + scopeLimit + fmt.Sprintf(" Harness detail: role=%s; stage=%s; capture=%s; outer_exit=%s; runtime=%s. This metadata does not establish Node execution.", role, tc.stage, tc.status, tc.exit, tc.code)
				found := 0
				for _, raw := range r["diagnostics"].([]any) {
					d := raw.(map[string]any)
					if d["code"] == "LINT_HARNESS_FAILURE" {
						found++
						if d["message"] != want {
							t.Fatalf("message=%q want=%q", d["message"], want)
						}
					}
				}
				if found != 1 || strings.Contains(string(b), "ghp_private_capture_token") || strings.Contains(string(b), "/private/runtime-path") || string(source) != `{"scripts":{"lint":"eslint ."}}` {
					t.Fatal("missing/duplicate diagnostic, leaked capture, or changed source")
				}
				for _, raw := range r["capabilities"].([]any) {
					c := raw.(map[string]any)
					if c["dimension"] == "EFFICACY" && c["scope"].(map[string]any)["kind"] == "TOOL_FIXTURE" && c["status"] != "UNVERIFIABLE" {
						t.Fatal("harness metadata promoted efficacy")
					}
				}
				for _, raw := range r["evidence"].([]any) {
					e := raw.(map[string]any)
					if e["state"] == "EXECUTED" || e["execution"] != nil {
						t.Fatal("failure promoted to execution")
					}
				}
				if f.loads != 1 || f.captures != index+1 || f.snapshots != 2+3*(index+1) || f.events[len(f.events)-1] != "snapshot" {
					t.Fatalf("changed execution/cleanup/snapshot boundaries: %+v", f)
				}
			})
		}
	}
}

func TestHarnessDetailUnknownMetadataCannotEscape(t *testing.T) {
	const private = "ghp_private_token /private/path"
	valid := harnessDetail{role: ls.Clean, stage: "OUTER_CAPTURE", status: ls.Status(private), outerExit: -2, runtimeCode: private}
	want := " Harness detail: role=CLEAN; stage=OUTER_CAPTURE; capture=UNKNOWN; outer_exit=UNAVAILABLE; runtime=UNSPECIFIED. This metadata does not establish Node execution."
	if got := valid.text(); got != want {
		t.Fatalf("untrusted metadata escaped: %q", got)
	}
	for _, mutate := range []func(*harnessDetail){
		func(d *harnessDetail) { d.role = ls.Role(private) },
		func(d *harnessDetail) { d.stage = private },
	} {
		d := valid
		mutate(&d)
		if d.text() != "" {
			t.Fatal("unknown role/stage emitted a detail")
		}
	}
	if got := runtimeFailureCode("preflight: " + private); got != "RUNTIME_PREFLIGHT" {
		t.Fatal("raw preflight detail or unknown classification", got)
	}
}
