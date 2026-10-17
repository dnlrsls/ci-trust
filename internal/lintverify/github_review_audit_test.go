package lintverify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type lintRoundTrip func(*http.Request) (*http.Response, error)

func (f lintRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func lintAuditFixture(t *testing.T) (lintGitHubClient, githubReviewPlan, map[string]any, *int) {
	t.Helper()
	r, now := reviewFixture(t)
	prefix := "/repos/" + r.Repository
	bodies := map[string]any{
		prefix: map[string]any{"full_name": r.Repository, "owner": map[string]any{"id": 7, "type": "User"}},
		prefix + "/actions/workflows/lint-sensitivity-verification.yml":                      map[string]any{"id": 77, "path": r.Workflow, "state": "active"},
		prefix + "/actions/runs/42":                                                          map[string]any{"id": 42, "run_attempt": 1, "workflow_id": 77, "head_sha": r.Revision, "head_branch": "main", "event": "workflow_dispatch", "status": "in_progress", "repository": map[string]any{"full_name": r.Repository}, "actor": map[string]any{"id": 7, "type": "User"}},
		prefix + "/environments/ci-trust-controlled-verification":                            map[string]any{"id": r.EnvironmentID, "can_admins_bypass": false, "deployment_branch_policy": map[string]any{"protected_branches": false, "custom_branch_policies": true}, "protection_rules": []any{map[string]any{"type": "required_reviewers", "prevent_self_review": false, "reviewers": []any{map[string]any{"type": "User", "reviewer": map[string]any{"id": 7}}}}}},
		prefix + "/environments/ci-trust-controlled-verification/deployment-branch-policies": map[string]any{"total_count": 1, "branch_policies": []any{map[string]any{"name": "main", "type": "branch"}}},
		prefix + "/actions/runs/42/approvals":                                                []any{map[string]any{"state": "approved", "comment": "approve lint " + r.BindingDigest, "user": map[string]any{"id": 7, "type": "User"}, "environments": []any{map[string]any{"id": r.EnvironmentID}}}},
	}
	calls := 0
	g := lintGitHubClient{token: "owned-test-token", now: func() time.Time { return now }}
	g.http = &http.Client{Transport: lintRoundTrip(func(q *http.Request) (*http.Response, error) {
		calls++
		if q.Method != "GET" || q.URL.Scheme != "https" || q.URL.Host != "api.github.com" || q.Header.Get("Authorization") != "Bearer owned-test-token" {
			t.Fatal("unowned request")
		}
		body, ok := bodies[q.URL.Path]
		if !ok {
			t.Fatal("unexpected API path", q.URL.Path)
		}
		b, _ := json.Marshal(body)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}, nil
	})}
	return g, r, bodies, &calls
}
func TestLintGitHubAuditExactApproval(t *testing.T) {
	g, r, _, calls := lintAuditFixture(t)
	a, e := g.audit(context.Background(), r)
	if e != nil || a.binding != r.Plan.digest() || a.expires.Format(time.RFC3339) != r.ExpiresAt || *calls != 7 {
		t.Fatalf("audit=%+v error=%v calls=%d", a, e, *calls)
	}
}
func TestLintGitHubAuditRejectsOtherScopes(t *testing.T) {
	for _, tc := range []struct {
		name, path, key string
		value           any
	}{
		{"smoke-workflow", "/actions/workflows/lint-sensitivity-verification.yml", "path", ".github/workflows/controlled-verification.yml"},
		{"attempt", "/actions/runs/42", "run_attempt", 2},
		{"wrong-workflow", "/actions/runs/42", "workflow_id", 386038019},
		{"wrong-actor", "/actions/runs/42", "actor", map[string]any{"id": 9, "type": "User"}},
		{"missing-reviewer", "/environments/ci-trust-controlled-verification", "protection_rules", []any{}},
		{"wrong-branch-policy", "/environments/ci-trust-controlled-verification/deployment-branch-policies", "branch_policies", []any{map[string]any{"name": "other", "type": "branch"}}}, {"revision", "/actions/runs/42", "head_sha", strings.Repeat("c", 40)},
		{"branch", "/actions/runs/42", "head_branch", "feat/issue8-real-isolation"}, {"event", "/actions/runs/42", "event", "push"}, {"complete", "/actions/runs/42", "status", "completed"},
		{"environment", "/environments/ci-trust-controlled-verification", "id", 1}, {"bypass", "/environments/ci-trust-controlled-verification", "can_admins_bypass", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, r, b, _ := lintAuditFixture(t)
			b["/repos/"+r.Repository+tc.path].(map[string]any)[tc.key] = tc.value
			a, e := g.audit(context.Background(), r)
			if e == nil || a != (approval{}) {
				t.Fatal("other scope accepted")
			}
		})
	}
	g, r, b, _ := lintAuditFixture(t)
	reviews := b["/repos/"+r.Repository+"/actions/runs/42/approvals"].([]any)
	reviews[0].(map[string]any)["comment"] = "approve " + r.BindingDigest
	if a, e := g.audit(context.Background(), r); e == nil || a != (approval{}) {
		t.Fatal("smoke comment accepted")
	}
}
func TestLintGitHubAuditRejectsExpiryAndCancellation(t *testing.T) {
	g, r, _, calls := lintAuditFixture(t)
	now, _ := time.Parse(time.RFC3339, r.IssuedAt)
	ticks := 0
	g.now = func() time.Time {
		ticks++
		if ticks > 1 {
			return now.Add(15 * time.Minute)
		}
		return now
	}
	if a, e := g.audit(context.Background(), r); e == nil || a != (approval{}) {
		t.Fatal("expiry during audit accepted")
	}
	g, r, _, calls = lintAuditFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := g.audit(ctx, r); e == nil || *calls != 0 {
		t.Fatal("canceled audit reached network")
	}
}
func TestLintGitHubHTTPBoundsAndRedirect(t *testing.T) {
	for _, status := range []int{200, 403, 302} {
		g := newLintGitHubClient("owned-test-token")
		calls := 0
		g.http.Transport = lintRoundTrip(func(q *http.Request) (*http.Response, error) {
			calls++
			if q.URL.Host != "api.github.com" {
				t.Fatal("redirect leaked")
			}
			return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://unowned.invalid/"}}, Body: io.NopCloser(strings.NewReader(strings.Repeat("x", (1<<20)+1)))}, nil
		})
		var data any
		if e := g.get(context.Background(), "/repos/dnlrsls/ci-trust", &data); e != githubReviewUnavailable || calls != 1 || strings.Contains(e.Error(), "owned-test-token") {
			t.Fatalf("HTTP %d: error=%v calls=%d", status, e, calls)
		}
	}
}

func TestLintGitHubProductionRemainsClosed(t *testing.T) {
	_, r, _, _ := lintAuditFixture(t)
	before := r
	if a, e := reviewGitHub(context.Background(), r.Plan, r); e == nil || a != (approval{}) || r != before {
		t.Fatal("production gate opened")
	}
}
