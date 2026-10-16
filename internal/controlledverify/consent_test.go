package controlledverify

import (
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
	"github.com/dnlrsls/ci-trust/internal/ownedimage"
	"github.com/dnlrsls/ci-trust/internal/runner"
)

func consentFixture(t *testing.T) (controlledplan.Envelope, runIdentity, []approvalReview, time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	meta := ownedimage.Metadata{ImageID: "sha256:" + strings.Repeat("a", 64), ArchiveSHA256: "sha256:" + strings.Repeat("b", 64), BinarySHA256: "sha256:" + strings.Repeat("c", 64), MarkerSHA256: "sha256:" + strings.Repeat("d", 64), LayerDiffID: "sha256:" + strings.Repeat("e", 64), Scope: "preparation-only"}
	p, err := controlledplan.PrepareProof(strings.Repeat("f", 64), meta, strings.Repeat("a", 40), "123", "1", now)
	if err != nil {
		t.Fatal(err)
	}
	run := runIdentity{ID: 123, Attempt: 1, WorkflowID: 386038019, Revision: p.Revision, Snapshot: p.Plan.Snapshot, Branch: "feat/issue8-real-isolation", Event: "workflow_dispatch", Status: "in_progress", Repository: "dnlrsls/ci-trust"}
	reviews := []approvalReview{{State: "approved", Comment: "approve " + p.BindingDigest, User: reviewUser{ID: 111, Type: "User"}, Environments: []reviewEnvironment{{ID: 24415568518}}}}
	return p, run, reviews, now.Add(time.Minute)
}

func TestExactHumanDecisionAndFixedCases(t *testing.T) {
	p, run, reviews, now := consentFixture(t)
	d, err := verifyHumanDecision(p, run, reviews, 111, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name string
		plan runner.Plan
	}{{"normal", p.Cases.Normal}, {"timeout", p.Cases.Timeout}, {"cancel", p.Cases.ActiveCancel}, {"output", p.Cases.OutputLimit}} {
		c, err := d.forPlan(item.plan, now)
		if err != nil || !c.Approved || c.Digest != item.plan.Digest() {
			t.Fatalf("exact %s human decision lost", item.name)
		}
	}
	changed := p.Cases.Normal
	changed.Limits.PIDs++
	if _, err := d.forPlan(changed, now); err == nil {
		t.Fatal("changed case admitted")
	}
	if _, err := d.forPlan(p.Cases.Normal, now.Add(time.Hour)); err == nil {
		t.Fatal("expired decision admitted")
	}
}

func TestNoDispatchEnvironmentOrAgentBooleanConsent(t *testing.T) {
	for _, name := range []string{"absent-review", "environment-only", "wrong-comment", "wrong-user", "bot", "wrong-environment", "rejected", "duplicate", "wrong-run", "rerun", "wrong-source", "wrong-snapshot", "wrong-workflow", "wrong-repository", "not-running", "expired"} {
		t.Run(name, func(t *testing.T) {
			p, run, reviews, now := consentFixture(t)
			switch name {
			case "absent-review":
				reviews = nil
			case "environment-only":
				reviews[0].Comment = "approved=true"
			case "wrong-comment":
				reviews[0].Comment = "approve " + strings.Repeat("0", 64)
			case "wrong-user":
				reviews[0].User.ID++
			case "bot":
				reviews[0].User.Type = "Bot"
			case "wrong-environment":
				reviews[0].Environments[0].ID++
			case "rejected":
				reviews[0].State = "rejected"
			case "duplicate":
				reviews = append(reviews, reviews[0])
			case "wrong-run":
				run.ID++
			case "rerun":
				run.Attempt++
			case "wrong-source":
				run.Revision = strings.Repeat("b", 40)
			case "wrong-snapshot":
				run.Snapshot = strings.Repeat("b", 64)
			case "wrong-workflow":
				run.WorkflowID++
			case "wrong-repository":
				run.Repository = "other/repo"
			case "not-running":
				run.Status = "completed"
			case "expired":
				now = now.Add(time.Hour)
			}
			if _, err := verifyHumanDecision(p, run, reviews, 111, now); err == nil {
				t.Fatal("unbound decision admitted")
			}
		})
	}
}
