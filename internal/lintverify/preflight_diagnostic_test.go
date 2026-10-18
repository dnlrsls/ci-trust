package lintverify

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/discovery"
)

const privatePreflight = "ghp_private_preflight_token /private/preflight-path"

type failedPreflightLoad struct{ workflowRuntime }

func (r failedPreflightLoad) load(ctx context.Context, a, i []byte) error {
	_ = r.workflowRuntime.load(ctx, a, i)
	return errors.New(privatePreflight)
}

func TestPreflightFailureReportSafeStages(t *testing.T) {
	for _, tc := range []struct{ mode, stage, events string }{
		{"review", "REVIEW_BEFORE_DELIVERY", "review"},
		{"delivery", "ARTIFACT_DELIVERY_OR_REVIEW", "review,deliver"},
		{"empty", "ARTIFACT_DELIVERY_OR_REVIEW", "review,deliver"},
		{"expiry", "ARTIFACT_DELIVERY_OR_REVIEW", "review,deliver"},
		{"runtime", "RUNTIME_CREATION", "review,deliver,snapshot,new"},
		{"nil-runtime", "RUNTIME_CREATION", "review,deliver,snapshot,new"},
		{"load", "IMAGE_LOAD", "review,deliver,snapshot,new,load,snapshot"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			s, f := workflowFixture(t, "sensitive")
			if tc.mode == "review" {
				calls := 0
				s.now = func() time.Time {
					calls++
					if calls == 3 {
						f.clock = f.clock.Add(16 * time.Minute)
					}
					return f.clock
				}
			}
			deliver := s.deliver
			s.deliver = func(ctx context.Context, r githubReviewPlan) (deliveredArtifact, error) {
				image, err := deliver(ctx, r)
				switch tc.mode {
				case "delivery":
					return image, errors.New(privatePreflight)
				case "empty":
					image.archive = nil
				case "expiry":
					f.clock = f.clock.Add(16 * time.Minute)
				}
				return image, err
			}
			newRuntime := s.newRuntime
			s.newRuntime = func() (workflowRuntime, error) {
				r, err := newRuntime()
				switch tc.mode {
				case "runtime":
					return nil, errors.New(privatePreflight)
				case "nil-runtime":
					return nil, nil
				case "load":
					return failedPreflightLoad{r}, err
				}
				return r, err
			}
			source := []byte(`{"scripts":{"lint":"eslint ."}}`)
			snapshot := discovery.Snapshot{Documents: []discovery.Document{{Path: "package.json", Content: source}}}
			got, err := analyze(snapshot, func(hash string) outcome {
				p, _ := planFor(s.catalog, hash)
				r, e := makeGitHubReviewPlan(p, f.r.Revision, f.r.RunID, f.clock)
				if e != nil {
					t.Fatal(e)
				}
				f.r = r
				return evaluateGitHubWorkflow(context.Background(), hash, r, s)
			})
			if err != nil {
				t.Fatal(err)
			}
			base, err := analyze(snapshot, func(string) outcome { return unresolved("LINT_PREFLIGHT_UNAVAILABLE") })
			if err != nil {
				t.Fatal(err)
			}
			message := "Owned lint sensitivity: UNVERIFIABLE. " + scopeLimit
			want := bytes.Replace(base, []byte(message), []byte(message+" Preflight detail: stage="+tc.stage+". This metadata does not establish Node execution."), 1)
			if !bytes.Equal(got, want) {
				t.Errorf("public report differs from exact stage-only change: %s", tc.stage)
			}
			if strings.Contains(string(got), "ghp_private_preflight_token") || strings.Contains(string(got), "/private/preflight-path") {
				t.Error("private preflight error leaked")
			}
			if f.captures != 0 || strings.Join(f.events, ",") != tc.events || string(source) != `{"scripts":{"lint":"eslint ."}}` {
				t.Fatalf("changed effect/source boundaries: %+v", f)
			}
		})
	}
}

func TestPreflightDetailCannotLeakOrChangeOtherReasons(t *testing.T) {
	snapshot := discovery.Snapshot{Documents: []discovery.Document{{Path: "package.json", Content: []byte(`{"scripts":{"lint":"eslint ."}}`)}}}
	for _, tc := range []struct{ reason, stage string }{
		{"LINT_PREFLIGHT_UNAVAILABLE", privatePreflight},
		{"LINT_PREFLIGHT_UNAVAILABLE", "IMAGE_LOAD; " + privatePreflight},
		{"LINT_PREFLIGHT_UNAVAILABLE", ""},
		{"LINT_REVIEW_EXPIRED", "IMAGE_LOAD"},
		{"LINT_SOURCE_CHANGED", "IMAGE_LOAD"},
		{"LINT_HARNESS_FAILURE", "IMAGE_LOAD"},
		{"LINT_EXECUTION_UNAVAILABLE", "IMAGE_LOAD"},
		{"NODE_SECURITY_HOLD", "IMAGE_LOAD"},
	} {
		base, err := analyze(snapshot, func(string) outcome { return unresolved(tc.reason) })
		if err != nil {
			t.Fatal(err)
		}
		got, err := analyze(snapshot, func(string) outcome { out := unresolved(tc.reason); out.preflightStage = tc.stage; return out })
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, base) {
			t.Fatalf("unknown metadata escaped or changed other reason: %s", tc.reason)
		}
	}
}

func TestPreflightLoadFailureStillPrefersSourceChange(t *testing.T) {
	s, f := workflowFixture(t, "drift-load")
	original := s.newRuntime
	s.newRuntime = func() (workflowRuntime, error) { r, e := original(); return failedPreflightLoad{r}, e }
	out := evaluateGitHubWorkflow(context.Background(), f.r.Plan.Identity.Snapshot, f.r, s)
	if out.decision.Reason != "LINT_SOURCE_CHANGED" || out.preflightStage != "" || f.loads != 1 || f.captures != 0 || f.snapshots != 2 || strings.Join(f.events, ",") != "review,deliver,snapshot,new,load,snapshot" {
		t.Fatalf("source-change or effect precedence changed: %+v %+v", out, f)
	}
}
