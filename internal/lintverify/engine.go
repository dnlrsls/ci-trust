package lintverify

import (
	"context"
	"time"

	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

func evaluate(ctx context.Context, snapshot string, d dependencies) outcome {
	// Production is stopped here; no live authorizer or runtime is composed.
	if reason := d.catalog.blocked(); reason != "" {
		return unresolved(reason)
	}
	if ctx.Err() != nil {
		return unresolved("LINT_CANCELLED")
	}
	p, ok := planFor(d.catalog, snapshot)
	if !ok {
		return unresolved("LINT_IDENTITY_INVALID")
	}
	if d.review == nil || d.preflight == nil || d.capture == nil || d.snapshot == nil || d.now == nil {
		return unresolved("LINT_EXECUTION_UNAVAILABLE")
	}
	approved, err := d.review(ctx, p)
	if ctx.Err() != nil {
		return unresolved("LINT_CANCELLED")
	}
	if err != nil {
		return unresolved("LINT_REVIEW_UNAVAILABLE")
	}
	now := d.now()
	if approved.binding != p.digest() || !now.Before(approved.expires) || approved.expires.Sub(now) > 15*time.Minute {
		return unresolved("LINT_REVIEW_INVALID")
	}
	if err := d.preflight(ctx, p); err != nil {
		return unresolved("LINT_PREFLIGHT_UNAVAILABLE")
	}
	if ctx.Err() != nil {
		return unresolved("LINT_CANCELLED")
	}
	observations := []ls.Observation{}
	for _, item := range p.Cases {
		if ctx.Err() != nil {
			return unresolved("LINT_CANCELLED")
		}
		now = d.now()
		if !now.Before(approved.expires) {
			return unresolved("LINT_REVIEW_EXPIRED")
		}
		before, err := d.snapshot()
		if err != nil || before != snapshot {
			return unresolved("LINT_SOURCE_CHANGED")
		}
		if ctx.Err() != nil {
			return unresolved("LINT_CANCELLED")
		}
		remaining := approved.expires.Sub(d.now())
		if remaining <= 0 {
			return unresolved("LINT_REVIEW_EXPIRED")
		}
		wall := time.Duration(p.WallSeconds) * time.Second
		if remaining < wall {
			wall = remaining
		}
		runCtx, cancel := context.WithTimeout(ctx, wall)
		captured := d.capture(runCtx, p, item, p.digest())
		runErr := runCtx.Err()
		cancel()
		// Remeasure even on capture, cancellation or cleanup failure. A transport
		// assertion alone never establishes completed/preserved execution.
		after, err := d.snapshot()
		if err != nil || after != snapshot {
			return unresolved("LINT_SOURCE_CHANGED")
		}
		if !captured.cleanupVerified {
			return unresolved("LINT_CLEANUP_UNVERIFIED")
		}
		if ctx.Err() != nil {
			return unresolved("LINT_CANCELLED")
		}
		if !d.now().Before(approved.expires) {
			return unresolved("LINT_REVIEW_EXPIRED")
		}
		if runErr != nil {
			captured.capture.Status = ls.TimedOut
		}
		observation := ls.DecodeESLintReceipt(item.Role, p.Identity, p.digest(), captured.capture)
		observations = append(observations, observation)
		if observation.Status != ls.Completed {
			return outcome{decision: ls.Result{Outcome: ls.Unverifiable, Reason: "LINT_HARNESS_FAILURE", Scope: "TOOL_FIXTURE"}, observations: observations}
		}
	}
	return outcome{decision: ls.Classify(observations), observations: observations}
}
