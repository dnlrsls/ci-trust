package lintverify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
)

var githubReviewUnavailable = errors.New("authenticated lint GitHub review unavailable")

type lintReviewer struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}
type lintGitHubClient struct {
	http  *http.Client
	token string
	now   func() time.Time
}

// This is the only production composition entry. Planning data and selection
// of GitHub as a channel cannot cross the compiled gate or read a credential.
func reviewGitHub(ctx context.Context, p plan, r githubReviewPlan) (approval, error) {
	if lintcatalog.BlockedReason() != "" || r.Plan != p {
		return approval{}, githubReviewUnavailable
	}
	return newLintGitHubClient(os.Getenv("GITHUB_TOKEN")).audit(ctx, r)
}
func newLintGitHubClient(token string) lintGitHubClient {
	return lintGitHubClient{token: token, now: time.Now, http: &http.Client{
		Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: nil, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second},
		CheckRedirect: func(*http.Request, []*http.Request) error { return githubReviewUnavailable },
	}}
}
func (g lintGitHubClient) get(ctx context.Context, p string, out any) error {
	if ctx.Err() != nil || g.http == nil || g.token == "" {
		return githubReviewUnavailable
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com"+p, nil)
	if e != nil {
		return githubReviewUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	response, e := g.http.Do(req)
	if e != nil {
		return githubReviewUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return githubReviewUnavailable
	}
	b, e := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if e != nil || len(b) > 1<<20 || json.Unmarshal(b, out) != nil {
		return githubReviewUnavailable
	}
	return nil
}
func (g lintGitHubClient) run(ctx context.Context, r githubReviewPlan, wf, owner int64) error {
	var v struct {
		ID            int64  `json:"id"`
		Attempt       int    `json:"run_attempt"`
		Workflow      int64  `json:"workflow_id"`
		Revision      string `json:"head_sha"`
		Branch        string `json:"head_branch"`
		Event, Status string
		Repository    struct {
			FullName string `json:"full_name"`
		}
		Actor lintReviewer
	}
	if g.get(ctx, "/repos/"+r.Repository+"/actions/runs/"+strconv.FormatInt(r.RunID, 10), &v) != nil || v.ID != r.RunID || v.Attempt != 1 || v.Workflow != wf || v.Revision != r.Revision || v.Branch != "main" || v.Event != "workflow_dispatch" || v.Status != "in_progress" || v.Repository.FullName != r.Repository || v.Actor.ID != owner || v.Actor.Type != "User" {
		return githubReviewUnavailable
	}
	return nil
}
func (g lintGitHubClient) environment(ctx context.Context, r githubReviewPlan, owner int64) error {
	var v struct {
		ID       int64
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
	base := "/repos/" + r.Repository + "/environments/" + r.Environment
	if g.get(ctx, base, &v) != nil || v.ID != r.EnvironmentID || v.Bypass == nil || *v.Bypass || v.Branches.Protected == nil || *v.Branches.Protected || v.Branches.Custom == nil || !*v.Branches.Custom {
		return githubReviewUnavailable
	}
	count := 0
	for _, rule := range v.Rules {
		switch rule.Type {
		case "branch_policy":
		case "required_reviewers":
			count++
			if rule.Prevent == nil || *rule.Prevent || len(rule.Reviewers) != 1 || rule.Reviewers[0].Type != "User" || rule.Reviewers[0].Reviewer.ID != owner {
				return githubReviewUnavailable
			}
		default:
			return githubReviewUnavailable
		}
	}
	if count != 1 {
		return githubReviewUnavailable
	}
	var branches struct {
		Total    int                           `json:"total_count"`
		Policies []struct{ Name, Type string } `json:"branch_policies"`
	}
	if g.get(ctx, base+"/deployment-branch-policies", &branches) != nil || branches.Total != len(branches.Policies) || branches.Total < 1 || branches.Total > 2 {
		return githubReviewUnavailable
	}
	seen := map[string]bool{}
	for _, p := range branches.Policies {
		if p.Type != "branch" || seen[p.Name] || (p.Name != "main" && p.Name != "feat/issue8-real-isolation") {
			return githubReviewUnavailable
		}
		seen[p.Name] = true
	}
	if !seen["main"] {
		return githubReviewUnavailable
	}
	return nil
}

// audit is a private authenticated-metadata component, not an admission API.
// Tests can supply fake HTTP metadata; production reaches it only via the gate.
func (g lintGitHubClient) audit(ctx context.Context, r githubReviewPlan) (approval, error) {
	if ctx == nil || ctx.Err() != nil || g.now == nil || g.http == nil || g.token == "" {
		return approval{}, githubReviewUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if _, e := readGitHubReviewPlan(bytes.NewReader(r.JSON()), g.now()); e != nil {
		return approval{}, githubReviewUnavailable
	}
	prefix := "/repos/" + r.Repository
	var repo struct {
		FullName string `json:"full_name"`
		Owner    lintReviewer
	}
	if g.get(ctx, prefix, &repo) != nil || repo.FullName != r.Repository || repo.Owner.ID <= 0 || repo.Owner.Type != "User" {
		return approval{}, githubReviewUnavailable
	}
	var wf struct {
		ID          int64
		Path, State string
	}
	if g.get(ctx, prefix+"/actions/workflows/"+path.Base(r.Workflow), &wf) != nil || wf.ID <= 0 || wf.Path != r.Workflow || wf.State != "active" {
		return approval{}, githubReviewUnavailable
	}
	if g.run(ctx, r, wf.ID, repo.Owner.ID) != nil || g.environment(ctx, r, repo.Owner.ID) != nil {
		return approval{}, githubReviewUnavailable
	}
	var reviews []struct {
		State, Comment string
		User           lintReviewer
		Environments   []struct{ ID int64 }
	}
	if g.get(ctx, prefix+"/actions/runs/"+strconv.FormatInt(r.RunID, 10)+"/approvals", &reviews) != nil || len(reviews) != 1 {
		return approval{}, githubReviewUnavailable
	}
	review := reviews[0]
	comment := strings.TrimSuffix(review.Comment, "\r\n")
	if comment == review.Comment {
		comment = strings.TrimSuffix(comment, "\n")
	}
	if review.State != "approved" || review.User.ID != repo.Owner.ID || review.User.Type != "User" || len(review.Environments) != 1 || review.Environments[0].ID != r.EnvironmentID || comment != "approve lint "+r.BindingDigest {
		return approval{}, githubReviewUnavailable
	}
	if g.run(ctx, r, wf.ID, repo.Owner.ID) != nil || ctx.Err() != nil {
		return approval{}, githubReviewUnavailable
	}
	if _, e := readGitHubReviewPlan(bytes.NewReader(r.JSON()), g.now()); e != nil {
		return approval{}, githubReviewUnavailable
	}
	expires, e := time.Parse(time.RFC3339, r.ExpiresAt)
	if e != nil {
		return approval{}, githubReviewUnavailable
	}
	return approval{binding: r.Plan.digest(), expires: expires}, nil
}
