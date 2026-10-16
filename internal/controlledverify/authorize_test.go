package controlledverify

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
)

func authorizationFixture(t *testing.T) (controlledplan.Envelope, string, githubClient, *int, time.Time) {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "owned.go"), []byte("owned source fixture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := sourceSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Minute)
	p, err := controlledplan.PrepareProof(snapshot, reviewedImage(), strings.Repeat("a", 40), "123", "1", now)
	if err != nil {
		t.Fatal(err)
	}
	g, calls := auditClient(t, "")
	base := g.http.Transport
	g.http.Transport = fakeHTTP(func(r *http.Request) (*http.Response, error) {
		response, err := base.RoundTrip(r)
		if err == nil && strings.HasSuffix(r.URL.Path, "/approvals") {
			reviews := []approvalReview{{State: "approved", Comment: "approve " + p.BindingDigest, User: reviewUser{ID: 111, Type: "User"}, Environments: []reviewEnvironment{{ID: environmentID}}}}
			b, marshalErr := json.Marshal(reviews)
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			response.Body.Close()
			response.Body = io.NopCloser(strings.NewReader(string(b)))
		}
		return response, err
	})
	count := new(int)
	approved := g.http.Transport
	g.http.Transport = fakeHTTP(func(r *http.Request) (*http.Response, error) {
		*count = len(*calls) + 1
		return approved.RoundTrip(r)
	})
	return p, root, g, count, now.Add(time.Minute)
}

func TestAuthorizationJoinsCatalogSnapshotAndAuthenticatedDecision(t *testing.T) {
	p, root, g, calls, now := authorizationFixture(t)
	d, err := authorizeProof(context.Background(), p, root, g, func() time.Time { return now })
	if err != nil || *calls != 5 {
		t.Fatalf("authorization failed: calls=%d err=%v", *calls, err)
	}
	consent, err := d.forPlan(p.Plan, now)
	if err != nil || !consent.Approved || consent.Digest != p.PlanDigest {
		t.Fatal("exact authenticated consent lost")
	}
}

func TestAuthorizationAcceptsAuthenticatedCommentCRLF(t *testing.T) {
	p, root, g, calls, now := authorizationFixture(t)
	base := g.http.Transport
	g.http.Transport = fakeHTTP(func(r *http.Request) (*http.Response, error) {
		response, err := base.RoundTrip(r)
		if err != nil || !strings.HasSuffix(r.URL.Path, "/approvals") {
			return response, err
		}
		var reviews []approvalReview
		err = json.NewDecoder(response.Body).Decode(&reviews)
		response.Body.Close()
		if err != nil || len(reviews) != 1 {
			t.Fatalf("invalid review fixture: %v", err)
		}
		reviews[0].Comment += "\r\n"
		b, err := json.Marshal(reviews)
		if err != nil {
			t.Fatal(err)
		}
		response.Body = io.NopCloser(bytes.NewReader(b))
		return response, nil
	})
	d, err := authorizeProof(context.Background(), p, root, g, func() time.Time { return now })
	if err != nil || *calls != 5 {
		t.Fatalf("CRLF authorization failed: calls=%d err=%v", *calls, err)
	}
	consent, err := d.forPlan(p.Plan, now)
	if err != nil || !consent.Approved || consent.Digest != p.PlanDigest {
		t.Fatal("CRLF approval lost exact plan binding")
	}
}

func TestAuthorizationDeniesBeforeAnyAuditForInvalidLocalInputs(t *testing.T) {
	for _, name := range []string{"catalog", "snapshot", "plan", "context", "clock"} {
		t.Run(name, func(t *testing.T) {
			p, root, g, calls, now := authorizationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			clock := func() time.Time { return now }
			switch name {
			case "catalog":
				m := reviewedImage()
				m.ImageID = "sha256:" + strings.Repeat("0", 64)
				var err error
				p, err = controlledplan.PrepareProof(p.Plan.Snapshot, m, p.Revision, p.RunID, p.RunAttempt, now)
				if err != nil {
					t.Fatal(err)
				}
			case "snapshot":
				if err := os.WriteFile(filepath.Join(root, "owned.go"), []byte("changed\n"), 0600); err != nil {
					t.Fatal(err)
				}
			case "plan":
				p.Cases.Normal.Limits.PIDs++
			case "context":
				cancel()
			case "clock":
				clock = nil
			}
			if _, err := authorizeProof(ctx, p, root, g, clock); err == nil || *calls != 0 {
				t.Fatalf("invalid local state reached audit: calls=%d err=%v", *calls, err)
			}
		})
	}
}

func TestAuthorizationRejectsAuditDriftExpiryAndCancellation(t *testing.T) {
	for _, name := range []string{"source-drift", "expired", "canceled", "wrong-comment", "api-denied", "wrong-source"} {
		t.Run(name, func(t *testing.T) {
			p, root, g, _, now := authorizationFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			base := g.http.Transport
			g.http.Transport = fakeHTTP(func(r *http.Request) (*http.Response, error) {
				response, err := base.RoundTrip(r)
				if err != nil {
					return response, err
				}
				if strings.HasSuffix(r.URL.Path, "/approvals") {
					switch name {
					case "source-drift":
						if err := os.WriteFile(filepath.Join(root, "owned.go"), []byte("changed during audit\n"), 0600); err != nil {
							t.Fatal(err)
						}
					case "expired":
						now = now.Add(time.Hour)
					case "canceled":
						cancel()
					case "wrong-comment":
						response.Body.Close()
						response.Body = io.NopCloser(strings.NewReader(`[{"state":"approved","comment":"approve wrong-binding","user":{"id":111,"type":"User"},"environments":[{"id":24415568518}]}]`))
					case "api-denied":
						response.StatusCode = 403
					}
				}
				if name == "wrong-source" && strings.HasSuffix(r.URL.Path, "/runs/123") {
					b, readErr := io.ReadAll(response.Body)
					response.Body.Close()
					if readErr != nil {
						t.Fatal(readErr)
					}
					response.Body = io.NopCloser(strings.NewReader(strings.ReplaceAll(string(b), strings.Repeat("a", 40), strings.Repeat("b", 40))))
				}
				return response, nil
			})
			if _, err := authorizeProof(ctx, p, root, g, func() time.Time { return now }); err == nil {
				t.Fatal("drifted or unavailable authorization accepted")
			}
		})
	}
}
