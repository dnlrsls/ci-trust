package controlledverify

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"
)

var apiUnavailable = errors.New("read-only GitHub approval metadata unavailable")

type githubClient struct {
	http  *http.Client
	token string
}

func newGitHubClient(token string) githubClient {
	return githubClient{token: token, http: &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second}, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" || r.URL.Host != "api.github.com" || len(via) > 3 {
			return apiUnavailable
		}
		return nil
	}}}
}

// get accepts only package-owned fixed paths. No approval/write endpoint exists.
// Tokens, response errors and bodies never enter diagnostics.
func (g githubClient) get(ctx context.Context, path string, out any) error {
	if g.token == "" || g.http == nil {
		return apiUnavailable
	}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com"+path, nil)
	if err != nil {
		return apiUnavailable
	}
	r.Header.Set("Accept", "application/vnd.github+json")
	r.Header.Set("Authorization", "Bearer "+g.token)
	response, err := g.http.Do(r)
	if err != nil {
		return apiUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return apiUnavailable
	}
	b, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(b) > 1<<20 || json.Unmarshal(b, out) != nil {
		return apiUnavailable
	}
	return nil
}

func (g githubClient) identity(ctx context.Context, id int64) (runIdentity, int64, error) {
	if id <= 0 {
		return runIdentity{}, 0, apiUnavailable
	}
	var repo struct {
		FullName string `json:"full_name"`
		Owner    struct {
			ID int64 `json:"id"`
		} `json:"owner"`
	}
	if g.get(ctx, "/repos/"+repository, &repo) != nil || repo.FullName != repository || repo.Owner.ID <= 0 {
		return runIdentity{}, 0, apiUnavailable
	}
	var run struct {
		ID            int64  `json:"id"`
		Attempt       int    `json:"run_attempt"`
		Workflow      int64  `json:"workflow_id"`
		Revision      string `json:"head_sha"`
		Branch        string `json:"head_branch"`
		Event, Status string
		Repository    struct {
			FullName string `json:"full_name"`
		}
		Actor reviewUser
	}
	if g.get(ctx, "/repos/"+repository+"/actions/runs/"+strconv.FormatInt(id, 10), &run) != nil || run.ID != id || run.Attempt != 1 || run.Workflow != workflowID || run.Repository.FullName != repository || run.Actor.ID != repo.Owner.ID || run.Actor.Type != "User" || run.Event != "workflow_dispatch" || run.Status != "in_progress" || (run.Branch != "main" && run.Branch != "feat/issue8-real-isolation") {
		return runIdentity{}, 0, apiUnavailable
	}
	return runIdentity{ID: id, Attempt: run.Attempt, WorkflowID: run.Workflow, Revision: run.Revision, Branch: run.Branch, Event: run.Event, Status: run.Status, Repository: repository}, repo.Owner.ID, nil
}

func (g githubClient) audit(ctx context.Context, id int64) (runIdentity, int64, []approvalReview, error) {
	run, owner, err := g.identity(ctx, id)
	if err != nil {
		return runIdentity{}, 0, nil, err
	}
	var environment struct {
		ID       int64 `json:"id"`
		Bypass   *bool `json:"can_admins_bypass"`
		Branches struct {
			Protected *bool `json:"protected_branches"`
			Custom    *bool `json:"custom_branch_policies"`
		} `json:"deployment_branch_policy"`
		Rules []struct {
			Type      string
			Prevent   *bool `json:"prevent_self_review"`
			Reviewers []struct {
				Type     string
				Reviewer struct{ ID int64 }
			}
		} `json:"protection_rules"`
	}
	base := "/repos/" + repository + "/environments/ci-trust-controlled-verification"
	if g.get(ctx, base, &environment) != nil || environment.ID != environmentID || environment.Bypass == nil || *environment.Bypass || environment.Branches.Protected == nil || *environment.Branches.Protected || environment.Branches.Custom == nil || !*environment.Branches.Custom {
		return runIdentity{}, 0, nil, apiUnavailable
	}
	reviewers := 0
	for _, rule := range environment.Rules {
		switch rule.Type {
		case "branch_policy":
		case "required_reviewers":
			reviewers++
			if rule.Prevent == nil || *rule.Prevent || len(rule.Reviewers) != 1 || rule.Reviewers[0].Type != "User" || rule.Reviewers[0].Reviewer.ID != owner {
				return runIdentity{}, 0, nil, apiUnavailable
			}
		default:
			return runIdentity{}, 0, nil, apiUnavailable
		}
	}
	if reviewers != 1 {
		return runIdentity{}, 0, nil, apiUnavailable
	}
	var branches struct {
		Total    int                           `json:"total_count"`
		Policies []struct{ Name, Type string } `json:"branch_policies"`
	}
	if g.get(ctx, base+"/deployment-branch-policies", &branches) != nil || branches.Total != len(branches.Policies) || branches.Total < 1 || branches.Total > 2 {
		return runIdentity{}, 0, nil, apiUnavailable
	}
	seen := map[string]bool{}
	for _, p := range branches.Policies {
		if p.Type != "branch" || seen[p.Name] || (p.Name != "main" && p.Name != "feat/issue8-real-isolation") {
			return runIdentity{}, 0, nil, apiUnavailable
		}
		seen[p.Name] = true
	}
	if !seen["main"] || !seen[run.Branch] {
		return runIdentity{}, 0, nil, apiUnavailable
	}
	var reviews []approvalReview
	if g.get(ctx, "/repos/"+repository+"/actions/runs/"+strconv.FormatInt(id, 10)+"/approvals", &reviews) != nil {
		return runIdentity{}, 0, nil, apiUnavailable
	}
	return run, owner, reviews, nil
}
