package controlledverify

import (
	"bytes"
	"context"
	"strconv"
	"time"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
)

// authorizeProof joins the compiled catalog, live source and authenticated
// read-only GitHub review. Neither an input envelope nor a successful dispatch
// constructs consent. This function never touches Docker or changes policy.
func authorizeProof(ctx context.Context, p controlledplan.Envelope, root string, github githubClient, now func() time.Time) (humanDecision, error) {
	if now == nil || ctx.Err() != nil || admitPlanImage(p) != nil {
		return humanDecision{}, denied
	}
	checked, err := controlledplan.CheckProof(bytes.NewReader(p.JSON()), p.BindingDigest, p.Revision, p.RunID, p.RunAttempt, now())
	if err != nil {
		return humanDecision{}, denied
	}
	id, err := strconv.ParseInt(checked.RunID, 10, 64)
	if err != nil || id <= 0 {
		return humanDecision{}, denied
	}
	snapshot, err := sourceSnapshot(root)
	if err != nil || snapshot != checked.Plan.Snapshot {
		return humanDecision{}, denied
	}
	// Bound the complete dependent audit, not just each individual GET.
	auditCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	run, owner, reviews, err := github.audit(auditCtx, id)
	if err != nil || auditCtx.Err() != nil {
		return humanDecision{}, denied
	}
	// Audit latency cannot hide source drift or extend approval lifetime.
	after, err := sourceSnapshot(root)
	if err != nil || after != snapshot || auditCtx.Err() != nil {
		return humanDecision{}, denied
	}
	run.Snapshot = after
	return verifyHumanDecision(checked, run, reviews, owner, now())
}
