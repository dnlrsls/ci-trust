package lintverify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/dnlrsls/ci-trust/internal/audit"
	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
	"github.com/dnlrsls/ci-trust/internal/runner"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var workflowUnavailable = errors.New("lint workflow unavailable")

type workflowRuntime interface {
	load(context.Context, []byte, []byte) error
	execute(context.Context, runner.Plan, runner.Consent, runner.Snapshot) runner.Result
}

// Private trusted test seams; no caller data can supply these services.
type workflowServices struct {
	catalog    catalog
	now        func() time.Time
	review     func(context.Context, plan, githubReviewPlan) (approval, error)
	deliver    func(context.Context, githubReviewPlan) (deliveredArtifact, error)
	newRuntime func() (workflowRuntime, error)
	snapshot   func() (string, error)
}

// PrepareGitHubReview emits non-authorizing data, never an approval.
func PrepareGitHubReview(source string) ([]byte, error) {
	if reason := lintcatalog.BlockedReason(); reason != "" {
		return nil, errors.New(reason)
	}
	revision, run, e := workflowContext(source)
	if e != nil {
		return nil, e
	}
	s, hash, e := workflowSource(source)
	if e != nil || unsupported(s) || lintAnchor(s) == nil {
		return nil, workflowUnavailable
	}
	p, ok := planFor(productionCatalog(), hash)
	if !ok {
		return nil, workflowUnavailable
	}
	r, e := makeGitHubReviewPlan(p, revision, run, time.Now())
	if e != nil {
		return nil, workflowUnavailable
	}
	return r.JSON(), nil
}

// ExecuteGitHub accepts review request DATA only. Actual authentication, source
// measurements and fixed execution are owned by this composition, not a report.
func ExecuteGitHub(ctx context.Context, source string, request io.Reader) ([]byte, error) {
	if reason := lintcatalog.BlockedReason(); reason != "" {
		return nil, errors.New(reason)
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, workflowUnavailable
	}
	revision, run, e := workflowContext(source)
	if e != nil {
		return nil, e
	}
	r, e := readGitHubReviewPlan(request, time.Now())
	if e != nil || r.Revision != revision || r.RunID != run {
		return nil, workflowUnavailable
	}
	s, _, e := workflowSource(source)
	if e != nil {
		return nil, e
	}
	services := workflowServices{catalog: productionCatalog(), now: time.Now, review: reviewGitHub, deliver: deliverLintArtifact}
	services.snapshot = func() (string, error) { _, hash, e := workflowSource(source); return hash, e }
	services.newRuntime = func() (workflowRuntime, error) {
		d, e := runner.NewLocalDocker("/usr/bin/docker", filepath.Join(source, ".git", "lint-sensitivity", "docker-config"))
		if e != nil {
			return nil, workflowUnavailable
		}
		return dockerWorkflow{d}, nil
	}
	return analyze(s, func(hash string) outcome { return evaluateGitHubWorkflow(ctx, hash, r, services) })
}
func workflowContext(source string) (string, int64, error) {
	runText := os.Getenv("GITHUB_RUN_ID")
	run, e := strconv.ParseInt(runText, 10, 64)
	revision := os.Getenv("GITHUB_SHA")
	if runtime.GOOS != "linux" || !filepath.IsAbs(source) || filepath.Clean(source) != filepath.Clean(os.Getenv("GITHUB_WORKSPACE")) || os.Getenv("GITHUB_ACTIONS") != "true" || os.Getenv("GITHUB_REPOSITORY") != "dnlrsls/ci-trust" || os.Getenv("GITHUB_REF") != "refs/heads/main" || os.Getenv("GITHUB_RUN_ATTEMPT") != "1" || e != nil || run <= 0 || strconv.FormatInt(run, 10) != runText || !reviewRevision.MatchString(revision) {
		return "", 0, workflowUnavailable
	}
	return revision, run, nil // Scope checks are not human authentication.
}
func workflowSource(source string) (discovery.Snapshot, string, error) {
	s := discovery.ReadSnapshot(source, discovery.Options{})
	for _, d := range s.Diagnostics {
		if d.Code == "ROOT_UNAVAILABLE" || d.Code == "UNSUPPORTED_HOST" || d.Code == "INVALID_OPTIONS" {
			return s, "", workflowUnavailable
		}
	}
	b, e := audit.Analyze(s, audit.Options{})
	var r struct{ Snapshot struct{ ID string } }
	if e != nil || json.Unmarshal(b, &r) != nil {
		return s, "", workflowUnavailable
	}
	hash := strings.TrimPrefix(r.Snapshot.ID, "snapshot-")
	if !sourceHash.MatchString(hash) {
		return s, "", workflowUnavailable
	}
	return s, hash, nil
}

type dockerWorkflow struct{ docker *runner.LocalDocker }

func (d dockerWorkflow) load(ctx context.Context, a, i []byte) error {
	return d.docker.LoadLintImage(ctx, a, i)
}
func (d dockerWorkflow) execute(ctx context.Context, p runner.Plan, a runner.Consent, source runner.Snapshot) runner.Result {
	r, e := runner.NewLintOwned(d.docker, p, source)
	if e != nil {
		return runner.Result{Outcome: runner.Unverifiable, Reason: "lint runner unavailable", Facts: &runner.ExecutionFacts{Primary: "RUNNER_SETUP"}}
	}
	return r.Execute(ctx, p, a)
}
func evaluateGitHubWorkflow(ctx context.Context, snapshot string, r githubReviewPlan, s workflowServices) outcome {
	if reason := s.catalog.blocked(); reason != "" {
		return unresolved(reason)
	}
	if ctx == nil || s.now == nil || s.review == nil || s.deliver == nil || s.newRuntime == nil || s.snapshot == nil {
		return unresolved("LINT_EXECUTION_UNAVAILABLE")
	}
	p, ok := planFor(s.catalog, snapshot)
	if !ok || p != r.Plan {
		return unresolved("LINT_SOURCE_CHANGED")
	}
	if _, e := readGitHubReviewPlan(bytes.NewReader(r.JSON()), s.now()); e != nil {
		return unresolved("LINT_REVIEW_INVALID")
	}
	var approved approval
	var first workflowRuntime
	changed := false
	validReview := func() bool { return approved.binding == p.digest() && s.now().Before(approved.expires) }
	measure := func() bool {
		hash, e := s.snapshot()
		if e != nil || hash != snapshot {
			changed = true
			return false
		}
		return true
	}
	d := dependencies{catalog: s.catalog, now: s.now, snapshot: s.snapshot}
	d.review = func(ctx context.Context, p plan) (approval, error) {
		var e error
		approved, e = s.review(ctx, p, r)
		return approved, e
	}
	preflightStage := ""
	d.preflight = func(ctx context.Context, p plan) error {
		preflightStage = "REVIEW_BEFORE_DELIVERY"
		if !validReview() {
			return workflowUnavailable
		}
		preflightStage = "ARTIFACT_DELIVERY_OR_REVIEW"
		image, e := s.deliver(ctx, r)
		if e != nil || len(image.archive) == 0 || len(image.inventory) == 0 || !validReview() {
			return workflowUnavailable
		}
		if !measure() {
			return workflowUnavailable
		}
		preflightStage = "RUNTIME_CREATION"
		rt, e := s.newRuntime()
		if e != nil || rt == nil {
			return workflowUnavailable
		}
		preflightStage = "IMAGE_LOAD"
		loadCtx, cancel := context.WithTimeout(ctx, approved.expires.Sub(s.now()))
		e = rt.load(loadCtx, image.archive, image.inventory)
		cancel()
		// Always remeasure after requesting the image effect, even on failure/cancel.
		preserved := measure()
		if e != nil || !preserved {
			return workflowUnavailable
		}
		first = rt
		return nil
	}
	d.capture = func(ctx context.Context, p plan, item ownedCase, binding string) captureResult {
		failed := captureResult{capture: ls.ESLintCapture{Status: ls.Unavailable}, facts: &runner.ExecutionFacts{Primary: "CAPTURE_GUARD"}}
		if !validReview() || ctx.Err() != nil || binding != p.digest() {
			return failed
		}
		failed.facts.Primary = "RUNTIME_CREATION"
		rt := first
		first = nil
		if item.Role != ls.Clean {
			var e error
			rt, e = s.newRuntime()
			if e != nil {
				return failed
			}
		}
		if rt == nil {
			return failed
		}
		failed.facts.Primary = "ROLE_PLAN_OR_REVIEW"
		role, e := runner.FixedLintPlan(snapshot, item.Role, binding)
		if e != nil || !validReview() {
			return failed
		}
		// Only the private authenticated decision above can produce this DTO. Request
		// fields, the channel choice and receipts never become Consent.Approved.
		result := rt.execute(ctx, role, runner.Consent{Approved: true, Digest: role.Digest()}, func(context.Context) (string, error) { return s.snapshot() })
		status := ls.Unavailable
		if result.Outcome == runner.Completed {
			status = ls.Completed
		}
		return captureResult{facts: result.Facts, cleanupVerified: result.CleanupVerified, runtimeCode: runtimeFailureCode(result.Reason), capture: ls.ESLintCapture{Status: status, ExitCode: result.ExitCode, Stdout: []byte(result.Stdout), Stderr: []byte(result.Stderr)}}
	}
	out := evaluate(ctx, snapshot, d)
	if changed {
		return unresolved("LINT_SOURCE_CHANGED")
	}
	if out.decision.Reason == "LINT_PREFLIGHT_UNAVAILABLE" && out.decision.Outcome == ls.Unverifiable {
		out.preflightStage = preflightStage
	}
	return out
}
