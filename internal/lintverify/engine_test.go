package lintverify

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

const cleanJSON = `[{"filePath":"/work/fixture.js","messages":[],"suppressedMessages":[],"errorCount":0,"fatalErrorCount":0,"warningCount":0,"fixableErrorCount":0,"fixableWarningCount":0}]`
const faultJSON = `[{"filePath":"/work/fixture.js","messages":[{"ruleId":"no-debugger","severity":2,"message":"Unexpected debugger statement.","line":1,"column":1}],"suppressedMessages":[],"errorCount":1,"fatalErrorCount":0,"warningCount":0,"fixableErrorCount":0,"fixableWarningCount":0}]`

type counters struct {
	review, preflight, capture, snapshot int
	trace                                []string
}

func simulation(t *testing.T, mode string) (dependencies, *counters, context.Context) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	count := &counters{}
	now := time.Now()
	catalog := productionCatalog()
	catalog.securityHold = false
	catalog.admitted = true
	// Synthetic test identity, never the held production image.
	catalog.image = "sha256:" + strings.Repeat("1", 64)
	d := dependencies{catalog: catalog, now: func() time.Time { return now }}
	d.review = func(_ context.Context, p plan) (approval, error) {
		count.review++
		count.trace = append(count.trace, "review")
		if mode == "denied" {
			return approval{}, errors.New("private review detail must not escape")
		}
		a := approval{binding: p.digest(), expires: now.Add(time.Minute)}
		if mode == "stale" {
			a.binding = strings.Repeat("b", 64)
		}
		if mode == "expired" {
			a.expires = now
		}
		if mode == "cancel-review" {
			cancel()
		}
		return a, nil
	}
	d.preflight = func(context.Context, plan) error {
		count.preflight++
		count.trace = append(count.trace, "preflight")
		if mode == "preflight" {
			return errors.New("private runtime detail")
		}
		return nil
	}
	d.snapshot = func() (string, error) {
		count.snapshot++
		count.trace = append(count.trace, "snapshot")
		if mode == "drift-before" || mode == "drift-after" && count.snapshot == 2 {
			return strings.Repeat("b", 64), nil
		}
		return strings.Repeat("a", 64), nil
	}
	d.capture = func(_ context.Context, p plan, item ownedCase, binding string) captureResult {
		count.capture++
		count.trace = append(count.trace, string(item.Role))
		fixture, _ := ls.FixtureFor(item.Role)
		inv, _ := ls.OwnedESLintInvocation(item.Role)
		if binding != p.digest() || item.Stdin != fixture.Source || !reflect.DeepEqual(item.Args[:], inv.Args) {
			t.Fatal("capture received unbound/caller-selected input")
		}
		tool := ls.ESLintCapture{Status: ls.Completed, ExitCode: 0, Stdout: []byte(cleanJSON)}
		if item.Role == ls.Fault && mode != "noop" {
			tool.ExitCode = 1
			tool.Stdout = []byte(faultJSON)
		}
		if mode == "wrong" && item.Role == ls.Fault {
			tool.Stdout = []byte(strings.Replace(faultJSON, "no-debugger", "no-alert", 1))
		}
		if mode == "binding" {
			binding = strings.Repeat("b", 64)
		}
		frame, err := ls.EncodeESLintReceipt(item.Role, binding, tool)
		if err != nil {
			t.Fatal(err)
		}
		result := captureResult{capture: ls.ESLintCapture{Status: ls.Completed, ExitCode: 0, Stdout: frame}, cleanupVerified: true}
		switch mode {
		case "missing":
			result.capture.Status = ls.Unavailable
		case "crash":
			result.capture.Status = ls.Crashed
		case "timeout":
			result.capture.Status = ls.TimedOut
		case "cleanup":
			result.cleanupVerified = false
		case "outer-exit":
			result.capture.ExitCode = 1
		case "oversized":
			result.capture.Stdout = make([]byte, (64<<10)+1)
		case "cancel-capture":
			cancel()
		case "expire-capture":
			now = now.Add(2 * time.Minute)
		}
		return result
	}
	if mode == "cancel-before" {
		cancel()
	}
	if mode == "missing-dependency" {
		d.capture = nil
	}
	return d, count, ctx
}

func TestSimulatedTripletAndNoOpClassification(t *testing.T) {
	for _, tc := range []struct {
		mode    string
		outcome ls.Outcome
		reason  string
	}{
		{"sensitive", ls.Sensitive, "EXPECTED_DIAGNOSTIC"}, {"noop", ls.Insensitive, "FAULT_ACCEPTED"}, {"wrong", ls.Unverifiable, "UNEXPECTED_DIAGNOSTIC"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			d, c, ctx := simulation(t, tc.mode)
			got := evaluate(ctx, strings.Repeat("a", 64), d)
			if got.decision != (ls.Result{Outcome: tc.outcome, Reason: tc.reason, Scope: "TOOL_FIXTURE"}) || len(got.observations) != 3 {
				t.Fatalf("decision=%+v observations=%d", got.decision, len(got.observations))
			}
			wantTrace := []string{"review", "preflight", "snapshot", "CLEAN", "snapshot", "snapshot", "FAULT", "snapshot", "snapshot", "RESTORED", "snapshot"}
			if !reflect.DeepEqual(c.trace, wantTrace) || c.review != 1 || c.preflight != 1 || c.capture != 3 || c.snapshot != 6 {
				t.Fatalf("wrong ordered boundaries: %+v", c)
			}
			for i, role := range []ls.Role{ls.Clean, ls.Fault, ls.Restored} {
				o := got.observations[i]
				wantExit := 0
				if role == ls.Fault && tc.mode != "noop" {
					wantExit = 1
				}
				if o.Role != role || o.Status != ls.Completed || o.ExitCode != wantExit || o.Identity.Snapshot != strings.Repeat("a", 64) {
					t.Fatal("capture identity/status/exit lost", o)
				}
			}
		})
	}
}

func TestDeniedStalePreflightAndSourceNoStart(t *testing.T) {
	for _, tc := range []struct {
		mode, reason                            string
		reviews, preflight, captures, snapshots int
	}{
		{"denied", "LINT_REVIEW_UNAVAILABLE", 1, 0, 0, 0},
		{"stale", "LINT_REVIEW_INVALID", 1, 0, 0, 0},
		{"expired", "LINT_REVIEW_INVALID", 1, 0, 0, 0},
		{"preflight", "LINT_PREFLIGHT_UNAVAILABLE", 1, 1, 0, 0},
		{"drift-before", "LINT_SOURCE_CHANGED", 1, 1, 0, 1},
		{"drift-after", "LINT_SOURCE_CHANGED", 1, 1, 1, 2},
		{"cancel-before", "LINT_CANCELLED", 0, 0, 0, 0},
		{"cancel-review", "LINT_CANCELLED", 1, 0, 0, 0},
		{"missing-dependency", "LINT_EXECUTION_UNAVAILABLE", 0, 0, 0, 0},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			d, c, ctx := simulation(t, tc.mode)
			got := evaluate(ctx, strings.Repeat("a", 64), d)
			if got.decision.Outcome != ls.Unverifiable || got.decision.Reason != tc.reason || len(got.observations) != 0 || c.review != tc.reviews || c.preflight != tc.preflight || c.capture != tc.captures || c.snapshot != tc.snapshots {
				t.Fatalf("unexpected rejection/effects: %+v observations=%d calls=%+v", got.decision, len(got.observations), c)
			}
		})
	}
}

func TestFailedCaptureStillRemeasuresAndNeverClaimsEfficacy(t *testing.T) {
	for _, tc := range []struct {
		mode, reason string
		observations int
	}{
		{"missing", "LINT_HARNESS_FAILURE", 1}, {"crash", "LINT_HARNESS_FAILURE", 1}, {"timeout", "LINT_HARNESS_FAILURE", 1},
		{"binding", "LINT_HARNESS_FAILURE", 1}, {"outer-exit", "LINT_HARNESS_FAILURE", 1}, {"oversized", "LINT_HARNESS_FAILURE", 1},
		{"cleanup", "LINT_CLEANUP_UNVERIFIED", 0}, {"cancel-capture", "LINT_CANCELLED", 0}, {"expire-capture", "LINT_REVIEW_EXPIRED", 0},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			d, c, ctx := simulation(t, tc.mode)
			got := evaluate(ctx, strings.Repeat("a", 64), d)
			if got.decision.Outcome != ls.Unverifiable || got.decision.Reason != tc.reason || len(got.observations) != tc.observations || c.capture != 1 || c.snapshot != 2 {
				t.Fatalf("failure hidden or source not remeasured: %+v observations=%d calls=%+v", got.decision, len(got.observations), c)
			}
		})
	}
}
