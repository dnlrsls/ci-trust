package lintverify

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/runner"
)

const privateFacts = "ghp_private_failure /private/failure"

type factsDriver struct {
	mode            string
	starts, removes int
	after           func()
}

func (d *factsDriver) Preflight(context.Context, runner.Plan) error {
	if d.mode == "pre-start" {
		return errors.New(privateFacts)
	}
	return nil
}
func (d *factsDriver) Start(context.Context, string, []string, io.Writer) (int, error) {
	d.starts++
	if d.mode == "start" || d.mode == "both" {
		return -1, errors.New(privateFacts)
	}
	return 0, nil
}
func (d *factsDriver) Remove(context.Context, string) error {
	d.removes++
	if d.after != nil {
		defer d.after()
	}
	if d.mode == "cleanup" || d.mode == "both" {
		return errors.New(privateFacts)
	}
	return nil
}

type factsWorkflow struct {
	workflowRuntime
	driver *factsDriver
	trace  *workflowTrace
}

func (w factsWorkflow) execute(ctx context.Context, p runner.Plan, c runner.Consent, measure runner.Snapshot) runner.Result {
	w.trace.event(p.Fixture)
	w.trace.captures++
	if !c.Approved || c.Digest != p.Digest() {
		w.trace.t.Fatal("lost owned consent binding")
	}
	// Exercise the actual shared Runner.Execute producer using an owned fake,
	// not a pre-shaped Result. The generic transport cannot launch a process.
	smoke := p
	smoke.Adapter, smoke.Tool, smoke.Fixture, smoke.Config, smoke.Transport = "owned-smoke-v1", "owned-smoke-v1", "smoke-v1", "owned-v1", ""
	smoke.Effects = "owned fixture only; no host mounts; no network"
	r, err := runner.NewOwned(w.driver, smoke, measure)
	if err != nil {
		w.trace.t.Fatal(err)
	}
	return r.Execute(ctx, smoke, runner.Consent{Approved: true, Digest: smoke.Digest()})
}

func TestProducerFactsSurviveComposedReport(t *testing.T) {
	for _, tc := range []struct{ mode, reason, primary, cleanup, requested string }{
		{"pre-start", "LINT_CLEANUP_UNVERIFIED", "PREFLIGHT_FAILED", "NOT_ATTEMPTED", "false"},
		{"start", "LINT_HARNESS_FAILURE", "START_FAILED", "VERIFIED", "true"},
		{"both", "LINT_CLEANUP_UNVERIFIED", "START_FAILED", "REMOVE_UNVERIFIED", "true"},
		{"cleanup", "LINT_CLEANUP_UNVERIFIED", "NONE", "REMOVE_UNVERIFIED", "true"},
		{"source", "LINT_SOURCE_CHANGED", "START_FAILED", "REMOVE_UNVERIFIED", "true"},
		{"cancel", "LINT_CANCELLED", "WALLTIME_OR_CANCEL", "VERIFIED", "true"},
		{"expiry", "LINT_REVIEW_EXPIRED", "START_FAILED", "VERIFIED", "true"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			s, f := workflowFixture(t, "sensitive")
			driver := &factsDriver{mode: tc.mode}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch tc.mode {
			case "source":
				driver.mode = "both"
				measure := s.snapshot
				s.snapshot = func() (string, error) {
					hash, e := measure()
					if f.snapshots == 5 {
						return strings.Repeat("c", 64), nil
					}
					return hash, e
				}
			case "cancel":
				driver.mode = "start"
				driver.after = cancel
			case "expiry":
				driver.mode = "start"
				driver.after = func() { f.clock = f.clock.Add(16 * time.Minute) }
			}
			original := s.newRuntime
			s.newRuntime = func() (workflowRuntime, error) { r, e := original(); return factsWorkflow{r, driver, f}, e }
			source := []byte(`{"scripts":{"lint":"eslint ."}}`)
			snapshot := discovery.Snapshot{Documents: []discovery.Document{{Path: "package.json", Content: source}}}
			got, err := analyze(snapshot, func(hash string) outcome {
				p, _ := planFor(s.catalog, hash)
				r, e := makeGitHubReviewPlan(p, f.r.Revision, f.r.RunID, f.clock)
				if e != nil {
					t.Fatal(e)
				}
				f.r = r
				return evaluateGitHubWorkflow(ctx, hash, r, s)
			})
			if err != nil {
				t.Fatal(err)
			}
			base, err := analyze(snapshot, func(string) outcome { return unresolved(tc.reason) })
			if err != nil {
				t.Fatal(err)
			}
			message := "Owned lint sensitivity: UNVERIFIABLE. " + scopeLimit
			detail := " Execution facts: role=CLEAN; start_requested=" + tc.requested + "; primary=" + tc.primary + "; cleanup_attempted=" + tc.requested + "; cleanup=" + tc.cleanup + ". This metadata does not establish Node execution."
			want := bytes.Replace(base, []byte(message), []byte(message+detail), 1)
			if !bytes.Equal(got, want) {
				t.Error("primary/cleanup facts lost or report changed beyond diagnostic")
			}
			if strings.Contains(string(got), "ghp_private_failure") || strings.Contains(string(got), "/private/failure") {
				t.Fatal("raw error leaked")
			}
			calls := 1
			snapshots := 5
			if tc.mode == "pre-start" {
				calls = 0
				snapshots = 4
			}
			if driver.starts != calls || driver.removes != calls || f.captures != 1 || f.loads != 1 || f.snapshots != snapshots || string(source) != `{"scripts":{"lint":"eslint ."}}` {
				t.Fatalf("changed calls/source: %+v %+v", driver, f)
			}
		})
	}
}

func TestFailureFactsUnknownCauseCannotLeak(t *testing.T) {
	snapshot := discovery.Snapshot{Documents: []discovery.Document{{Path: "package.json", Content: []byte(`{"scripts":{"lint":"eslint ."}}`)}}}
	b, e := analyze(snapshot, func(string) outcome {
		return captureFailure("LINT_CLEANUP_UNVERIFIED", "CLEAN", captureResult{facts: &runner.ExecutionFacts{Primary: privateFacts}})
	})
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(b), "ghp_private_failure") || strings.Contains(string(b), "/private/failure") || !strings.Contains(string(b), "primary=UNSPECIFIED") || !strings.Contains(string(b), "cleanup=NOT_ATTEMPTED") {
		t.Fatal("unknown failure facts escaped or fabricated cleanup")
	}
}
