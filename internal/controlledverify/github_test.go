package controlledverify

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

type fakeHTTP func(*http.Request) (*http.Response, error)

func (f fakeHTTP) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func auditClient(t *testing.T, mutation string) (githubClient, *[]string) {
	t.Helper()
	calls := []string{}
	transport := fakeHTTP(func(r *http.Request) (*http.Response, error) {
		if r.Method != "GET" || r.URL.Scheme != "https" || r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer owned-test-token" {
			t.Fatal("request escaped fixed read-only boundary")
		}
		calls = append(calls, r.URL.Path)
		var body string
		switch r.URL.Path {
		case "/repos/dnlrsls/ci-trust":
			body = `{"full_name":"dnlrsls/ci-trust","owner":{"id":111}}`
		case "/repos/dnlrsls/ci-trust/actions/runs/123":
			body = fmt.Sprintf(`{"id":123,"run_attempt":1,"workflow_id":386038019,"head_sha":%q,"head_branch":"feat/issue8-real-isolation","event":"workflow_dispatch","status":"in_progress","repository":{"full_name":"dnlrsls/ci-trust"},"actor":{"id":111,"type":"User"}}`, strings.Repeat("a", 40))
		case "/repos/dnlrsls/ci-trust/environments/ci-trust-controlled-verification":
			body = `{"id":24415568518,"can_admins_bypass":false,"deployment_branch_policy":{"protected_branches":false,"custom_branch_policies":true},"protection_rules":[{"type":"required_reviewers","prevent_self_review":false,"reviewers":[{"type":"User","reviewer":{"id":111}}]}]}`
		case "/repos/dnlrsls/ci-trust/environments/ci-trust-controlled-verification/deployment-branch-policies":
			body = `{"total_count":2,"branch_policies":[{"name":"main","type":"branch"},{"name":"feat/issue8-real-isolation","type":"branch"}]}`
		case "/repos/dnlrsls/ci-trust/actions/runs/123/approvals":
			body = `[]`
		default:
			t.Fatalf("unexpected endpoint %s", r.URL.Path)
		}
		switch mutation {
		case "bypass":
			body = strings.Replace(body, `"can_admins_bypass":false`, `"can_admins_bypass":true`, 1)
		case "missing-bypass":
			body = strings.Replace(body, `"can_admins_bypass":false,`, "", 1)
		case "reviewer":
			body = strings.Replace(body, `"reviewer":{"id":111}`, `"reviewer":{"id":222}`, 1)
		case "wildcard":
			body = strings.Replace(body, `"name":"feat/issue8-real-isolation"`, `"name":"*"`, 1)
		case "tag":
			body = strings.Replace(body, `"type":"branch"`, `"type":"tag"`, 1)
		case "foreign-repository":
			body = strings.ReplaceAll(body, `"full_name":"dnlrsls/ci-trust"`, `"full_name":"other/repo"`)
		case "rerun":
			body = strings.Replace(body, `"run_attempt":1`, `"run_attempt":2`, 1)
		case "oversize":
			body = strings.Repeat(" ", 1048577)
		case "unavailable":
			return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("owned-test-token"))}, nil
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
	})
	return githubClient{http: &http.Client{Transport: transport}, token: "owned-test-token"}, &calls
}

func TestGitHubAuditUsesOnlyReadOnlyBoundContext(t *testing.T) {
	g, calls := auditClient(t, "")
	run, owner, reviews, err := g.audit(context.Background(), 123)
	if err != nil || owner != 111 || run.ID != 123 || run.Attempt != 1 || run.WorkflowID != workflowID || run.Repository != repository || len(reviews) != 0 || len(*calls) != 5 {
		t.Fatalf("audit=%+v owner=%d reviews=%v calls=%v err=%v", run, owner, reviews, *calls, err)
	}
}

func TestGitHubAuditFailsClosedWithoutLeakingToken(t *testing.T) {
	for _, mutation := range []string{"bypass", "missing-bypass", "reviewer", "wildcard", "tag", "foreign-repository", "rerun", "oversize", "unavailable"} {
		t.Run(mutation, func(t *testing.T) {
			g, _ := auditClient(t, mutation)
			if _, _, _, err := g.audit(context.Background(), 123); err == nil || strings.Contains(err.Error(), "owned-test-token") {
				t.Fatal("unsafe or leaking audit result")
			}
		})
	}
}
