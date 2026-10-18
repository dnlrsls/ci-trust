package lintverify

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func lintDeliveryFixture(t *testing.T) (lintGitHubClient, githubReviewPlan, map[string]any, *int) {
	g, r, b, c := lintAuditFixture(t)
	a := r.Artifact
	p := "/repos/" + r.Repository
	b[p].(map[string]any)["id"] = a.RepositoryID
	b[p+"/actions/workflows/lint-image-preparation.yml"] = map[string]any{"id": 88, "path": a.Workflow, "state": "active"}
	b[p+"/actions/runs/"+strconv.FormatInt(a.RunID, 10)] = map[string]any{"id": a.RunID, "run_attempt": a.Attempt, "workflow_id": 88, "head_sha": a.Revision, "head_branch": "main", "event": "workflow_dispatch", "status": "completed", "conclusion": "success", "repository": map[string]any{"id": a.RepositoryID, "full_name": r.Repository}, "actor": map[string]any{"id": 7, "type": "User"}}
	b[p+"/actions/runs/"+strconv.FormatInt(a.RunID, 10)+"/approvals"] = []any{map[string]any{"state": "approved", "user": map[string]any{"id": 7, "type": "User"}, "environments": []any{map[string]any{"id": r.EnvironmentID}}}}
	b[p+"/actions/artifacts/"+strconv.FormatInt(a.ID, 10)] = map[string]any{"id": a.ID, "name": a.Name, "size_in_bytes": a.Size, "digest": a.ZipSHA256, "expired": false, "expires_at": g.now().Add(time.Hour).Format(time.RFC3339), "workflow_run": map[string]any{"id": a.RunID, "repository_id": a.RepositoryID, "head_repository_id": a.RepositoryID, "head_branch": "main", "head_sha": a.Revision}}
	return g, r, b, c
}
func TestLintArtifactDeliveryGateAndProvenance(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	r, _ := reviewFixture(t)
	if got, e := deliverLintArtifact(nil, r); e == nil || len(got.archive) != 0 || len(got.inventory) != 0 {
		t.Error("closed gate returned data")
	}
	g, r, _, calls := lintDeliveryFixture(t)
	if expiry, e := g.artifactProvenance(context.Background(), r); e != nil || !expiry.Equal(g.now().Add(time.Hour)) || *calls != 5 {
		t.Error("producer identity rejected", e, *calls)
	}
	for _, rule := range []string{"repo", "workflow", "producer", "attempt", "revision", "conclusion", "prep-review", "artifact-id", "digest", "size", "expired", "expiry", "origin", "cancelled"} {
		t.Run(rule, func(t *testing.T) {
			g, r, b, c := lintDeliveryFixture(t)
			a := r.Artifact
			p := "/repos/" + r.Repository
			run := b[p+"/actions/runs/"+strconv.FormatInt(a.RunID, 10)].(map[string]any)
			art := b[p+"/actions/artifacts/"+strconv.FormatInt(a.ID, 10)].(map[string]any)
			ctx := context.Background()
			switch rule {
			case "repo":
				b[p].(map[string]any)["id"] = 1
			case "workflow":
				b[p+"/actions/workflows/lint-image-preparation.yml"].(map[string]any)["path"] = r.Workflow
			case "producer":
				run["actor"] = map[string]any{"id": 9, "type": "User"}
			case "attempt":
				run["run_attempt"] = 2
			case "revision":
				run["head_sha"] = strings.Repeat("c", 40)
			case "conclusion":
				run["conclusion"] = "failure"
			case "prep-review":
				b[p+"/actions/runs/"+strconv.FormatInt(a.RunID, 10)+"/approvals"] = []any{}
			case "artifact-id":
				art["id"] = 1
			case "digest":
				art["digest"] = "other"
			case "size":
				art["size_in_bytes"] = 1
			case "expired":
				art["expired"] = true
			case "expiry":
				art["expires_at"] = g.now().Format(time.RFC3339)
			case "origin":
				art["workflow_run"].(map[string]any)["head_repository_id"] = 1
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			got, e := g.artifactProvenance(ctx, r)
			if e == nil || !got.IsZero() {
				t.Error("unowned producer accepted")
			}
			if rule == "cancelled" && *c != 0 {
				t.Error("cancelled lookup reached HTTP")
			}
		})
	}
	g, r, _, c := lintDeliveryFixture(t)
	r.Artifact.ID++
	r.BindingDigest = r.digest()
	if _, e := g.artifactProvenance(context.Background(), r); e == nil || *c != 0 {
		t.Error("caller replaced compiled origin")
	}
}
func TestLintArtifactArchiveOwnedRedirect(t *testing.T) {
	for _, rule := range []string{"ok", "http", "foreign", "userinfo", "fragment", "port", "storage-redirect", "zip-hash", "zip-size", "cancelled"} {
		t.Run(rule, func(t *testing.T) {
			raw, w := artifactFixture(t, nil, false, false)
			w.ID = 1
			calls := 0
			g := newLintGitHubClient("owned-test-token")
			location := "https://ownedartifact.blob.core.windows.net/archive?sig=owned-test"
			switch rule {
			case "http":
				location = "http://ownedartifact.blob.core.windows.net/a"
			case "foreign":
				location = "https://api.github.com.attacker.invalid/a"
			case "userinfo":
				location = "https://user@ownedartifact.blob.core.windows.net/a"
			case "fragment":
				location += "#hidden"
			case "port":
				location = "https://ownedartifact.blob.core.windows.net:444/a"
			case "zip-hash":
				w.ZipSHA256 = "sha256:" + strings.Repeat("0", 64)
			case "zip-size":
				w.Size++
			}
			g.http.Transport = lintRoundTrip(func(q *http.Request) (*http.Response, error) {
				calls++
				if q.URL.Host == "api.github.com" {
					if calls != 1 || q.Header.Get("Authorization") != "Bearer owned-test-token" || q.URL.Path != "/repos/dnlrsls/ci-trust/actions/artifacts/1/zip" {
						t.Fatal("unowned API request")
					}
					return &http.Response{StatusCode: 302, Header: http.Header{"Location": []string{location}}, Body: io.NopCloser(strings.NewReader(""))}, nil
				}
				if q.URL.Host != "ownedartifact.blob.core.windows.net" || q.Header.Get("Authorization") != "" || q.Header.Get("Cookie") != "" || calls != 2 {
					t.Fatal("redirect leaked credential or escaped scope")
				}
				status := 200
				if rule == "storage-redirect" {
					status = 302
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Location": []string{"https://unowned.invalid/"}}, Body: io.NopCloser(bytes.NewReader(raw))}, nil
			})
			ctx := context.Background()
			if rule == "cancelled" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			got, e := g.artifactArchive(ctx, "dnlrsls/ci-trust", w)
			if rule == "ok" {
				if e != nil || !bytes.Equal(got, raw) || calls != 2 {
					t.Error("owned data download failed", e, calls)
				}
			} else if e == nil || len(got) != 0 {
				t.Error("unowned download returned bytes", rule)
			}
			if rule == "cancelled" && calls != 0 {
				t.Error("cancelled archive reached HTTP")
			}
			if g.http.CheckRedirect(&http.Request{}, nil) == nil {
				t.Error("metadata redirect policy relaxed")
			}
		})
	}
}
func TestLintArtifactDeliveryRequiresExactReview(t *testing.T) {
	g, r, b, c := lintDeliveryFixture(t)
	b[fmt.Sprintf("/repos/%s/actions/runs/%d/approvals", r.Repository, r.RunID)].([]any)[0].(map[string]any)["comment"] = "approve smoke"
	got, e := g.deliverArtifact(context.Background(), r)
	if e == nil || len(got.archive) != 0 || *c != 6 {
		t.Error("wrong review reached producer/download", e, *c)
	}
}
