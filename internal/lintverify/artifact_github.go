package lintverify

import (
	"bytes"
	"context"
	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// No caller approval, catalog or locator is authority. Check before even reading
// a credential or constructing a client; production remains held.
func deliverLintArtifact(ctx context.Context, r githubReviewPlan) (deliveredArtifact, error) {
	if lintcatalog.BlockedReason() != "" {
		return deliveredArtifact{}, artifactUnavailable
	}
	return newLintGitHubClient(os.Getenv("GITHUB_TOKEN")).deliverArtifact(ctx, r)
}
func (g lintGitHubClient) deliverArtifact(ctx context.Context, r githubReviewPlan) (deliveredArtifact, error) {
	bad := func() (deliveredArtifact, error) { return deliveredArtifact{}, artifactUnavailable }
	if ctx == nil || ctx.Err() != nil || g.now == nil {
		return bad()
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	a, e := g.audit(ctx, r)
	if e != nil || a.binding != r.Plan.digest() {
		return bad()
	}
	expires, e := g.artifactProvenance(ctx, r)
	if e != nil {
		return bad()
	}
	now := g.now()
	remaining := a.expires.Sub(now)
	if other := expires.Sub(now); other < remaining {
		remaining = other
	}
	if remaining <= 0 {
		return bad()
	}
	fetch, stop := context.WithTimeout(ctx, remaining)
	defer stop()
	raw, e := g.artifactArchive(fetch, r.Repository, r.Artifact)
	if e != nil {
		return bad()
	}
	data, e := decodeLintArtifact(raw, r.Artifact)
	now = g.now()
	if e != nil || fetch.Err() != nil || !now.Before(a.expires) || !now.Before(expires) {
		return bad()
	}
	return data, nil // Consistent bytes only, never image admission or execution consent.
}
func (g lintGitHubClient) artifactProvenance(ctx context.Context, r githubReviewPlan) (time.Time, error) {
	bad := func() (time.Time, error) { return time.Time{}, artifactUnavailable }
	if ctx == nil || ctx.Err() != nil || g.now == nil {
		return bad()
	}
	if _, e := readGitHubReviewPlan(bytes.NewReader(r.JSON()), g.now()); e != nil {
		return bad()
	}
	a := lintcatalog.CurrentArtifact()
	base := "/repos/" + r.Repository
	var repo struct {
		ID    int64
		Owner lintReviewer
	}
	if g.get(ctx, base, &repo) != nil || repo.ID != a.RepositoryID || repo.Owner.ID <= 0 || repo.Owner.Type != "User" {
		return bad()
	}
	var wf struct {
		ID          int64
		Path, State string
	}
	if g.get(ctx, base+"/actions/workflows/"+strings.TrimPrefix(a.Workflow, ".github/workflows/"), &wf) != nil || wf.ID <= 0 || wf.Path != a.Workflow || wf.State != "active" {
		return bad()
	}
	producer := base + "/actions/runs/" + strconv.FormatInt(a.RunID, 10)
	var run struct {
		ID                        int64
		Attempt                   int    `json:"run_attempt"`
		Workflow                  int64  `json:"workflow_id"`
		Revision                  string `json:"head_sha"`
		Branch                    string `json:"head_branch"`
		Event, Status, Conclusion string
		Repository                struct {
			ID       int64
			FullName string `json:"full_name"`
		}
		Actor lintReviewer
	}
	if g.get(ctx, producer, &run) != nil || run.ID != a.RunID || run.Attempt != a.Attempt || run.Workflow != wf.ID || run.Revision != a.Revision || run.Branch != "main" || run.Event != "workflow_dispatch" || run.Status != "completed" || run.Conclusion != "success" || run.Repository.ID != a.RepositoryID || run.Repository.FullName != r.Repository || run.Actor.ID != repo.Owner.ID || run.Actor.Type != "User" {
		return bad()
	}
	var reviews []struct {
		State        string
		User         lintReviewer
		Environments []struct{ ID int64 }
	}
	// Preparation approval establishes provenance ONLY, not this lint execution.
	if g.get(ctx, producer+"/approvals", &reviews) != nil || len(reviews) != 1 || reviews[0].State != "approved" || reviews[0].User.ID != repo.Owner.ID || reviews[0].User.Type != "User" || len(reviews[0].Environments) != 1 || reviews[0].Environments[0].ID != r.EnvironmentID {
		return bad()
	}
	var art struct {
		ID      int64
		Name    string
		Size    int64 `json:"size_in_bytes"`
		Digest  string
		Expired *bool
		Expires string `json:"expires_at"`
		Run     struct {
			ID             int64
			Repository     int64  `json:"repository_id"`
			HeadRepository int64  `json:"head_repository_id"`
			Branch         string `json:"head_branch"`
			Revision       string `json:"head_sha"`
		} `json:"workflow_run"`
	}
	if g.get(ctx, base+"/actions/artifacts/"+strconv.FormatInt(a.ID, 10), &art) != nil || art.ID != a.ID || art.Name != a.Name || art.Size != a.Size || art.Digest != a.ZipSHA256 || art.Expired == nil || *art.Expired || art.Run.ID != a.RunID || art.Run.Repository != a.RepositoryID || art.Run.HeadRepository != a.RepositoryID || art.Run.Branch != "main" || art.Run.Revision != a.Revision {
		return bad()
	}
	expires, e := time.Parse(time.RFC3339, art.Expires)
	if e != nil || !g.now().Before(expires) || ctx.Err() != nil {
		return bad()
	}
	return expires, nil
}

var artifactStorageAccount = regexp.MustCompile(`^[a-z0-9]{3,24}$`)

// GitHub documents Azure blob storage for artifact download. Accept one HTTPS
// Location from the authenticated fixed API, never an arbitrary caller URL.
func (g lintGitHubClient) artifactArchive(ctx context.Context, repository string, want lintcatalog.Artifact) ([]byte, error) {
	bad := func() ([]byte, error) { return nil, artifactUnavailable }
	if ctx == nil || ctx.Err() != nil || g.http == nil || g.token == "" || repository != "dnlrsls/ci-trust" || want.ID <= 0 || want.Size <= 0 || want.Size > 96<<20 {
		return bad()
	}
	client := *g.http
	client.Timeout = 45 * time.Second
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	endpoint := "https://api.github.com/repos/" + repository + "/actions/artifacts/" + strconv.FormatInt(want.ID, 10) + "/zip"
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if e != nil {
		return bad()
	}
	req.Header.Set("Authorization", "Bearer "+g.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	response, e := client.Do(req)
	if e != nil {
		return bad()
	}
	response.Body.Close()
	locations := response.Header.Values("Location")
	if response.StatusCode != http.StatusFound || len(locations) != 1 || len(locations[0]) > 16<<10 {
		return bad()
	}
	u, e := url.Parse(locations[0])
	if e != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.Opaque != "" || (u.Port() != "" && u.Port() != "443") {
		return bad()
	}
	host := u.Hostname()
	suffix := ".blob.core.windows.net"
	if !strings.HasSuffix(host, suffix) || !artifactStorageAccount.MatchString(strings.TrimSuffix(host, suffix)) {
		return bad()
	}
	// Fresh request: no API Authorization, Cookie, Referer or caller headers. Do
	// not follow another storage redirect or expose the signed URL in an error.
	req, e = http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if e != nil {
		return bad()
	}
	response, e = client.Do(req)
	if e != nil {
		return bad()
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return bad()
	}
	raw, e := io.ReadAll(io.LimitReader(response.Body, want.Size+1))
	if e != nil || int64(len(raw)) != want.Size || "sha256:"+artifactSHA(raw) != want.ZipSHA256 || ctx.Err() != nil {
		return bad()
	}
	return raw, nil
}
