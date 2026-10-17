package lintverify

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"
)

var reviewPlanUnavailable = errors.New("exact lint review plan unavailable")
var reviewRevision = regexp.MustCompile(`^[a-f0-9]{40}([a-f0-9]{24})?$`)

// githubReviewPlan is planning data, NEVER an approval or admission capability.
// The separate GitHub audit must authenticate its context and human review.
type githubReviewPlan struct {
	Domain        string `json:"domain"`
	Disposition   string `json:"disposition"`
	Repository    string `json:"repository"`
	Workflow      string `json:"workflow"`
	Environment   string `json:"environment"`
	EnvironmentID int64  `json:"environment_id"`
	Revision      string `json:"revision"`
	Branch        string `json:"branch"`
	RunID         int64  `json:"run_id"`
	Attempt       int    `json:"attempt"`
	IssuedAt      string `json:"issued_at"`
	ExpiresAt     string `json:"expires_at"`
	Plan          plan   `json:"plan"`
	BindingDigest string `json:"binding_digest"`
}

func makeGitHubReviewPlan(p plan, revision string, run int64, now time.Time) (githubReviewPlan, error) {
	expected, ok := planFor(productionCatalog(), p.Identity.Snapshot)
	if !ok || p != expected || !reviewRevision.MatchString(revision) || run <= 0 {
		return githubReviewPlan{}, reviewPlanUnavailable
	}
	issued := now.UTC().Truncate(time.Second)
	text := issued.Format(time.RFC3339)
	if parsed, e := time.Parse(time.RFC3339, text); e != nil || !parsed.Equal(issued) {
		return githubReviewPlan{}, reviewPlanUnavailable
	}
	r := githubReviewPlan{
		Domain: "ci-trust/lint-review/v1", Disposition: "NON_AUTHORIZING_PLAN",
		Repository: "dnlrsls/ci-trust", Workflow: ".github/workflows/lint-sensitivity-verification.yml",
		Environment: "ci-trust-controlled-verification", EnvironmentID: 24415568518,
		Revision: revision, Branch: "main", RunID: run, Attempt: 1,
		IssuedAt: text, ExpiresAt: issued.Add(15 * time.Minute).Format(time.RFC3339), Plan: p,
	}
	r.BindingDigest = r.digest()
	return r, nil
}
func (r githubReviewPlan) digest() string {
	r.BindingDigest = ""
	b, _ := json.Marshal(r)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (r githubReviewPlan) JSON() []byte { b, _ := json.Marshal(r); return append(b, '\n') }

// Only the exact canonical owned document (optionally one terminal LF) is
// accepted: duplicate/unknown keys, alternate encodings and extra data fail.
func readGitHubReviewPlan(reader io.Reader, now time.Time) (githubReviewPlan, error) {
	if reader == nil {
		return githubReviewPlan{}, reviewPlanUnavailable
	}
	raw, e := io.ReadAll(io.LimitReader(reader, (16<<10)+1))
	if e != nil || len(raw) > 16<<10 {
		return githubReviewPlan{}, reviewPlanUnavailable
	}
	raw = bytes.TrimSuffix(raw, []byte{'\n'})
	var r githubReviewPlan
	if json.Unmarshal(raw, &r) != nil {
		return githubReviewPlan{}, reviewPlanUnavailable
	}
	canonical, e := json.Marshal(r)
	if e != nil || !bytes.Equal(raw, canonical) {
		return githubReviewPlan{}, reviewPlanUnavailable
	}
	issued, e := time.Parse(time.RFC3339, r.IssuedAt)
	if e != nil {
		return githubReviewPlan{}, reviewPlanUnavailable
	}
	expected, e := makeGitHubReviewPlan(r.Plan, r.Revision, r.RunID, issued)
	if e != nil || r != expected || now.Before(issued) {
		return githubReviewPlan{}, reviewPlanUnavailable
	}
	expires, e := time.Parse(time.RFC3339, r.ExpiresAt)
	if e != nil || !now.Before(expires) {
		return githubReviewPlan{}, reviewPlanUnavailable
	}
	return r, nil
}
