package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestIndependentExecutionAndCleanupFacts(t *testing.T) {
	for _, tc := range []struct {
		name, primary, reason      string
		pre, run, clean            error
		started, removed, verified bool
	}{
		{"pre-start", "PREFLIGHT_FAILED", "preflight: private probe", errors.New("private probe"), nil, nil, false, false, false},
		{"start", "START_FAILED", "harness: private start", nil, errors.New("private start"), nil, true, true, true},
		{"both", "START_FAILED", "cleanup unverified: private remove", nil, errors.New("private start"), errors.New("private remove"), true, true, false},
		{"cleanup", "", "cleanup unverified: private remove", nil, nil, errors.New("private remove"), true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := owned()
			f := &fake{pre: tc.pre, run: tc.run, clean: tc.clean}
			r, err := NewOwned(f, p, func(context.Context) (string, error) { return p.Snapshot, nil })
			if err != nil {
				t.Fatal(err)
			}
			got := r.Execute(context.Background(), p, Consent{Digest: p.Digest(), Approved: true})
			if got.Outcome != Unverifiable || got.Reason != tc.reason || got.CleanupVerified != tc.verified || (f.starts == 1) != tc.started || (f.removes == 1) != tc.removed {
				t.Fatalf("legacy contract changed: %+v %+v", got, f)
			}
			if got.Facts == nil {
				t.Fatal("producer discarded independent facts")
			}
			if got.Facts.StartRequested != tc.started || got.Facts.CleanupAttempted != tc.removed || got.Facts.Primary != tc.primary {
				t.Fatalf("incorrect producer facts: %+v", got.Facts)
			}
			b, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			code := 0
			if !tc.started {
				code = -1
			}
			want, e := json.Marshal(Result{Outcome: Unverifiable, Reason: tc.reason, ExitCode: code, CleanupVerified: tc.verified})
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(b, want) {
				t.Fatal("legacy Result JSON changed")
			}
			if strings.Contains(string(b), "Facts") || strings.Contains(string(b), "StartRequested") || strings.Contains(string(b), "START_FAILED") {
				t.Fatal("internal metadata changed serialized Result")
			}
		})
	}
}

func TestSplitTransportRetainsBothFailures(t *testing.T) {
	p, _ := FixedLintPlan(strings.Repeat("a", 64), "CLEAN", strings.Repeat("b", 64))
	rt := &lintRuntimeFixture{capture: capturedRun{ExitCode: -1}, startErr: errors.New("private start"), cleanupErr: errors.New("private cleanup")}
	r, e := newLintOwned(rt, p, rt.snapshot)
	if e != nil {
		t.Fatal(e)
	}
	got := r.Execute(context.Background(), p, Consent{Approved: true, Digest: p.Digest()})
	if got.Facts == nil || got.Facts.Primary != "START_FAILED" || !got.Facts.StartRequested || !got.Facts.CleanupAttempted || got.CleanupVerified || got.Outcome != Unverifiable || got.Reason != "cleanup unverified: private cleanup" || rt.calls != 3 || rt.snapshots != 1 {
		t.Fatalf("split transport discarded a cause or changed calls: %+v %+v", got, rt)
	}
}
