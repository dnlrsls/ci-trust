package lintverify

import (
	"context"
	"errors"
	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
	"github.com/dnlrsls/ci-trust/internal/runner"
	"strings"
	"testing"
	"time"
)

type unreadWorkflow struct{}

func (unreadWorkflow) Read([]byte) (int, error) { panic("held workflow read caller data") }
func TestLintWorkflowPublicGateBeforeInputs(t *testing.T) {
	if b, e := PrepareGitHubReview("unowned"); e == nil || len(b) != 0 {
		t.Error("held plan read source")
	}
	if b, e := ExecuteGitHub(nil, "unowned", unreadWorkflow{}); e == nil || len(b) != 0 {
		t.Error("held workflow returned report")
	}
}

type workflowTrace struct {
	t                          *testing.T
	mode                       string
	clock                      time.Time
	r                          githubReviewPlan
	events                     []string
	loads, captures, snapshots int
}
type fakeWorkflowRuntime struct{ f *workflowTrace }

func (f *workflowTrace) event(s string) { f.events = append(f.events, s) }
func (c fakeWorkflowRuntime) load(_ context.Context, a, i []byte) error {
	f := c.f
	f.event("load")
	f.loads++
	if string(a) != "owned-archive" || string(i) != "owned-inventory" {
		f.t.Error("unowned load input")
	}
	if f.mode == "load" {
		return errors.New("private load details")
	}
	if f.mode == "expired" {
		f.clock = f.clock.Add(16 * time.Minute)
	}
	return nil
}
func (c fakeWorkflowRuntime) execute(ctx context.Context, p runner.Plan, a runner.Consent, measure runner.Snapshot) runner.Result {
	f := c.f
	f.event(p.Fixture)
	f.captures++
	role := ls.Role(p.Fixture)
	want, e := runner.FixedLintPlan(f.r.Plan.Identity.Snapshot, role, f.r.Plan.digest())
	if e != nil || p != want || !a.Approved || a.Digest != p.Digest() {
		f.t.Fatal("unbound runtime plan/decision")
	}
	if got, e := measure(ctx); e != nil || got != p.Snapshot {
		f.t.Fatal("missing runner snapshot")
	}
	tool := ls.ESLintCapture{Status: ls.Completed, ExitCode: 0, Stdout: []byte(cleanJSON)}
	if role == ls.Fault && f.mode != "noop" {
		tool.ExitCode = 1
		tool.Stdout = []byte(faultJSON)
	}
	b, e := ls.EncodeESLintReceipt(role, f.r.Plan.digest(), tool)
	if e != nil {
		f.t.Fatal(e)
	}
	r := runner.Result{Outcome: runner.Completed, ExitCode: 0, Stdout: string(b), CleanupVerified: true}
	if f.mode == "cleanup" {
		r.CleanupVerified = false
	}
	if f.mode == "outer" {
		r.ExitCode = 1
	}
	return r
}
func workflowFixture(t *testing.T, mode string) (workflowServices, *workflowTrace) {
	t.Helper()
	clock := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	p, _ := planFor(productionCatalog(), strings.Repeat("a", 64))
	r, e := makeGitHubReviewPlan(p, strings.Repeat("b", 40), 42, clock)
	if e != nil {
		t.Fatal(e)
	}
	f := &workflowTrace{t: t, mode: mode, clock: clock, r: r}
	cat := productionCatalog()
	cat.securityHold = false
	cat.admitted = true
	s := workflowServices{catalog: cat, now: func() time.Time { return f.clock }}
	s.review = func(_ context.Context, p plan, got githubReviewPlan) (approval, error) {
		f.event("review")
		if got != f.r || p != f.r.Plan {
			t.Error("unbound review")
		}
		if mode == "denied" {
			return approval{}, errors.New("private review details")
		}
		a := approval{binding: p.digest(), expires: f.clock.Add(15 * time.Minute)}
		if mode == "stale" {
			a.binding = strings.Repeat("c", 64)
		}
		return a, nil
	}
	s.deliver = func(_ context.Context, got githubReviewPlan) (deliveredArtifact, error) {
		f.event("deliver")
		if got != f.r {
			t.Error("unbound delivery")
		}
		if mode == "delivery" {
			return deliveredArtifact{}, errors.New("private delivery details")
		}
		return deliveredArtifact{archive: []byte("owned-archive"), inventory: []byte("owned-inventory")}, nil
	}
	s.newRuntime = func() (workflowRuntime, error) { f.event("new"); return fakeWorkflowRuntime{f}, nil }
	s.snapshot = func() (string, error) {
		f.event("snapshot")
		f.snapshots++
		if mode == "drift-load" && f.loads > 0 {
			return strings.Repeat("c", 64), nil
		}
		return f.r.Plan.Identity.Snapshot, nil
	}
	return s, f
}
func TestLintWorkflowComposesExactTriplet(t *testing.T) {
	for _, tc := range []struct {
		mode   string
		want   ls.Outcome
		reason string
	}{
		{"sensitive", ls.Sensitive, "EXPECTED_DIAGNOSTIC"}, {"noop", ls.Insensitive, "FAULT_ACCEPTED"},
		{"denied", ls.Unverifiable, "LINT_REVIEW_UNAVAILABLE"}, {"stale", ls.Unverifiable, "LINT_REVIEW_INVALID"},
		{"delivery", ls.Unverifiable, "LINT_PREFLIGHT_UNAVAILABLE"}, {"load", ls.Unverifiable, "LINT_PREFLIGHT_UNAVAILABLE"},
		{"drift-load", ls.Unverifiable, "LINT_SOURCE_CHANGED"}, {"expired", ls.Unverifiable, "LINT_REVIEW_EXPIRED"},
		{"cleanup", ls.Unverifiable, "LINT_CLEANUP_UNVERIFIED"}, {"outer", ls.Unverifiable, "LINT_HARNESS_FAILURE"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			s, f := workflowFixture(t, tc.mode)
			got := evaluateGitHubWorkflow(context.Background(), f.r.Plan.Identity.Snapshot, f.r, s)
			if got.decision.Outcome != tc.want || got.decision.Reason != tc.reason {
				t.Error("wrong workflow outcome", got.decision)
			}
			if tc.mode == "sensitive" || tc.mode == "noop" {
				if f.loads != 1 || f.captures != 3 || len(got.observations) != 3 || f.snapshots != 11 {
					t.Error("wrong effect/snapshot counts", f.loads, f.captures, f.snapshots)
				}
			}
			if tc.mode == "denied" || tc.mode == "stale" {
				if f.loads != 0 || f.captures != 0 || strings.Join(f.events, ",") != "review" {
					t.Error("rejected review reached effects")
				}
			}
			if tc.mode == "load" || tc.mode == "drift-load" {
				if f.loads != 1 || f.captures != 0 || f.snapshots != 2 {
					t.Error("failed load not remeasured", f.loads, f.captures, f.snapshots)
				}
			}
			if tc.mode == "expired" && f.captures != 0 {
				t.Error("expired approval reached capture")
			}
		})
	}
	s, f := workflowFixture(t, "sensitive")
	s.catalog = productionCatalog()
	if got := evaluateGitHubWorkflow(context.Background(), f.r.Plan.Identity.Snapshot, f.r, s); got.decision.Reason != "NODE_SECURITY_HOLD" || len(f.events) != 0 {
		t.Error("private composed hold crossed seam")
	}
}
