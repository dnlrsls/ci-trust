package lintverify

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func reviewFixture(t *testing.T) (githubReviewPlan, time.Time) {
	t.Helper()
	now := time.Unix(1800000000, 0).UTC()
	p, ok := planFor(productionCatalog(), strings.Repeat("a", 64))
	if !ok {
		t.Fatal("plan unavailable")
	}
	r, e := makeGitHubReviewPlan(p, strings.Repeat("b", 40), 42, now)
	if e != nil {
		t.Fatal(e)
	}
	return r, now
}
func TestGitHubReviewPlanExactBinding(t *testing.T) {
	r, now := reviewFixture(t)
	if r.Domain != "ci-trust/lint-review/v1" || r.Disposition != "NON_AUTHORIZING_PLAN" || r.Repository != "dnlrsls/ci-trust" || r.Workflow != ".github/workflows/lint-sensitivity-verification.yml" || r.Environment != "ci-trust-controlled-verification" || r.EnvironmentID != 24415568518 || r.Attempt != 1 || r.RunID != 42 || r.Branch != "main" || len(r.BindingDigest) != 64 {
		t.Fatalf("bad scope: %+v", r)
	}
	got, e := readGitHubReviewPlan(bytes.NewReader(r.JSON()), now)
	if e != nil || got != r {
		t.Fatalf("roundtrip: %+v %v", got, e)
	}
	for _, mutate := range []func(*githubReviewPlan){
		func(q *githubReviewPlan) { q.RunID++ }, func(q *githubReviewPlan) { q.Revision = strings.Repeat("c", 40) }, func(q *githubReviewPlan) { q.Domain = "smoke" }, func(q *githubReviewPlan) { q.Plan.Cases[0].Stdin = "unowned" }, func(q *githubReviewPlan) { q.Plan.OutputBytes++ }, func(q *githubReviewPlan) { q.ExpiresAt = now.Add(time.Hour).Format(time.RFC3339) },
	} {
		q := r
		mutate(&q)
		if q.digest() == r.BindingDigest {
			t.Fatal("unbound field")
		}
		raw, _ := json.Marshal(q)
		if _, e := readGitHubReviewPlan(bytes.NewReader(raw), now); e == nil {
			t.Fatal("tampered plan accepted")
		}
	}
	if r.Plan.Cases[0].Stdin == "unowned" {
		t.Fatal("copy mutated original")
	}
}
func TestGitHubReviewPlanCanonicalAndExpiry(t *testing.T) {
	r, now := reviewFixture(t)
	for _, at := range []time.Time{now.Add(-time.Nanosecond), now.Add(15 * time.Minute)} {
		if _, e := readGitHubReviewPlan(bytes.NewReader(r.JSON()), at); e == nil {
			t.Fatal("future or expired accepted")
		}
	}
	if _, e := readGitHubReviewPlan(bytes.NewReader(r.JSON()), now.Add(15*time.Minute-time.Nanosecond)); e != nil {
		t.Fatal(e)
	}
	raw := r.JSON()
	for _, bad := range [][]byte{append(raw, raw...), append([]byte(" "), raw...), bytes.Replace(raw, []byte(`"attempt":1`), []byte(`"attempt":1,"attempt":1`), 1), append(raw, '\n'), append(raw, '\r', '\n'), bytes.Repeat([]byte("x"), (16<<10)+1)} {
		if _, e := readGitHubReviewPlan(bytes.NewReader(bad), now); e == nil {
			t.Fatal("noncanonical/duplicate/oversize accepted")
		}
	}
}
func TestGitHubReviewPlanOwnedInputsAndNoAuthority(t *testing.T) {
	r, now := reviewFixture(t)
	for _, tc := range []struct {
		revision string
		run      int64
	}{{"", 42}, {strings.Repeat("B", 40), 42}, {strings.Repeat("b", 39), 42}, {strings.Repeat("b", 40), 0}} {
		if _, e := makeGitHubReviewPlan(r.Plan, tc.revision, tc.run, now); e == nil {
			t.Fatal("invalid context accepted")
		}
	}
	bad := r.Plan
	bad.Cases[1].Args[0] = "--fix"
	if _, e := makeGitHubReviewPlan(bad, r.Revision, 42, now); e == nil {
		t.Fatal("unowned argv accepted")
	}
	if c := productionCatalog(); c.securityHold || !c.admitted || c.blocked() != "" {
		t.Fatal("planning changed compiled scoped admission")
	}
}
