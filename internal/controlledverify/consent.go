// Package controlledverify composes one owned smoke proof, not arbitrary validation.
package controlledverify

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
	"github.com/dnlrsls/ci-trust/internal/runner"
)

const repository = "dnlrsls/ci-trust"
const workflowID int64 = 386038019
const environmentID int64 = 24415568518

var denied = errors.New("exact current human plan decision unavailable")

type runIdentity struct {
	ID                                                    int64
	Attempt                                               int
	WorkflowID                                            int64
	Revision, Snapshot, Branch, Event, Status, Repository string
}
type reviewUser struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
type reviewEnvironment struct {
	ID int64 `json:"id"`
}
type approvalReview struct {
	State        string              `json:"state"`
	Comment      string              `json:"comment"`
	User         reviewUser          `json:"user"`
	Environments []reviewEnvironment `json:"environments"`
}

// Only this package's read-only GitHub audit path constructs this private value.
// It is never decoded from CLI input or a supplied Approved boolean.
type humanDecision struct {
	cases   controlledplan.ProofCases
	expires time.Time
}

func verifyHumanDecision(p controlledplan.Envelope, run runIdentity, reviews []approvalReview, owner int64, now time.Time) (humanDecision, error) {
	if owner <= 0 || run.ID <= 0 || run.Attempt != 1 || run.WorkflowID != workflowID || run.Repository != repository || run.Event != "workflow_dispatch" || run.Status != "in_progress" || (run.Branch != "main" && run.Branch != "feat/issue8-real-isolation") || p.Plan.Snapshot != run.Snapshot {
		return humanDecision{}, denied
	}
	checked, err := controlledplan.CheckProof(bytes.NewReader(p.JSON()), p.BindingDigest, run.Revision, strconv.FormatInt(run.ID, 10), strconv.Itoa(run.Attempt), now)
	if err != nil || len(reviews) != 1 {
		return humanDecision{}, denied
	}
	review := reviews[0]
	// Accept one terminal text-area line ending, not arbitrary whitespace.
	comment := review.Comment
	if strings.HasSuffix(comment, "\r\n") {
		comment = strings.TrimSuffix(comment, "\r\n")
	} else {
		comment = strings.TrimSuffix(comment, "\n")
	}
	if review.State != "approved" || review.User.ID != owner || review.User.Type != "User" || len(review.Environments) != 1 || review.Environments[0].ID != environmentID || comment != "approve "+checked.BindingDigest {
		return humanDecision{}, denied
	}
	expires, err := time.Parse(time.RFC3339, checked.ExpiresAt)
	if err != nil {
		return humanDecision{}, denied
	}
	return humanDecision{cases: *checked.Cases, expires: expires}, nil
}

func (d humanDecision) forPlan(p runner.Plan, now time.Time) (runner.Consent, error) {
	if !now.Before(d.expires) || (p != d.cases.Normal && p != d.cases.Timeout && p != d.cases.ActiveCancel && p != d.cases.OutputLimit) {
		return runner.Consent{}, denied
	}
	return runner.Consent{Digest: p.Digest(), Approved: true}, nil
}
