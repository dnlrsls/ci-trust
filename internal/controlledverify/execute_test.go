package controlledverify

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/ownedimage"
	"github.com/dnlrsls/ci-trust/internal/runner"
)

type composedRuntime struct {
	index      int
	mode       string
	plan       runner.Plan
	calls      *[]string
	loadHook   func()
	removeHook func()
}

func (f *composedRuntime) LoadOwnedImage(ctx context.Context, a []byte, binary, marker string, m ownedimage.Metadata) error {
	*f.calls = append(*f.calls, "load")
	if ctx.Err() != nil || string(a) != "inert owned archive fixture" || m != reviewedImage() {
		return errors.New("bad fake load")
	}
	if f.mode == "load-failure" {
		return errors.New("ambiguous load")
	}
	if f.loadHook != nil {
		f.loadHook()
	}
	return nil
}
func (f *composedRuntime) Preflight(ctx context.Context, p runner.Plan) error {
	*f.calls = append(*f.calls, "preflight")
	f.plan = p
	if f.mode == "preflight" {
		return errors.New("capability unavailable")
	}
	return ctx.Err()
}
func (f *composedRuntime) Start(ctx context.Context, name string, args []string, w io.Writer) (int, error) {
	*f.calls = append(*f.calls, "start")
	switch f.index {
	case 0:
		_, err := io.WriteString(w, "owned-smoke: proof-started\nowned-smoke: owned stderr\nowned fake report\n")
		if f.mode == "tool-exit" {
			return 1, nil
		}
		return 0, err
	case 1:
		if f.mode == "false-timeout" {
			return -1, errors.New("runtime interrupted")
		}
		<-ctx.Done()
		return -1, ctx.Err()
	case 2:
		if f.mode == "false-cancel" {
			return 0, nil
		}
		// Force the marker across output chunks; no pre-start cancellation.
		_, _ = io.WriteString(w, "owned-smoke: proof-")
		_, _ = io.WriteString(w, "started\n")
		<-ctx.Done()
		return -1, ctx.Err()
	case 3:
		_, err := io.WriteString(w, strings.Repeat("x", f.plan.Limits.OutputBytes+1))
		return -1, err
	}
	return -1, errors.New("unexpected case")
}
func (f *composedRuntime) Remove(context.Context, string) error {
	*f.calls = append(*f.calls, "remove")
	if f.removeHook != nil {
		f.removeHook()
	}
	if f.mode == "cleanup" {
		return errors.New("absence unavailable")
	}
	return nil
}

func executionFixture(t *testing.T, mode string) (ProofInput, proofServices, *[]string) {
	t.Helper()
	p, root, g, _, now := authorizationFixture(t)
	archive := filepath.Join(t.TempDir(), "archive")
	if err := os.WriteFile(archive, []byte("inert owned archive fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := []string{}
	index := 0
	services := proofServices{
		github: g, now: func() time.Time { return now },
		inspect: func(a []byte, binary, marker string) (ownedimage.Metadata, error) {
			calls = append(calls, "inspect")
			if string(a) != "inert owned archive fixture" {
				return ownedimage.Metadata{}, errors.New("wrong fixture")
			}
			return reviewedImage(), nil
		},
		newRuntime: func() (loadRuntime, error) {
			calls = append(calls, "factory")
			if mode == "factory" {
				return nil, errors.New("missing trusted path")
			}
			f := &composedRuntime{index: index, mode: mode, calls: &calls}
			index++
			if mode == "source-before-load" {
				if err := os.WriteFile(filepath.Join(root, "owned.go"), []byte("drift after audit"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "source-before-start" {
				f.loadHook = func() {
					if err := os.WriteFile(filepath.Join(root, "owned.go"), []byte("drift after load"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			return f, nil
		},
	}
	return ProofInput{Plan: p, Source: root, Archive: archive, Binary: "inert-binary", Marker: "inert-marker"}, services, &calls
}

func TestCompositionRecordsFourCasesWithOneLoadAndOwnedCleanup(t *testing.T) {
	input, services, calls := executionFixture(t, "")
	r := executeProof(context.Background(), input, services)
	if r.State != "RECORDED" || r.ImageDisposition != "RETAINED_UNTAGGED" || !r.SourceUnchanged || len(r.Cases) != 4 {
		t.Fatalf("record=%+v calls=%v", r, *calls)
	}
	for _, c := range r.Cases {
		if !c.ExpectationObserved || !c.Result.CleanupVerified {
			t.Fatalf("case=%+v", c)
		}
	}
	if !r.Cases[2].MarkerCancellationObserved || r.Cases[1].Elapsed < input.Plan.Cases.Timeout.Limits.Wall {
		t.Fatal("timeout or active cancellation not observed")
	}
	counts := map[string]int{}
	for _, c := range *calls {
		counts[c]++
	}
	if counts["load"] != 1 || counts["factory"] != 4 || counts["start"] != 4 || counts["remove"] != 4 {
		t.Fatalf("calls=%v", *calls)
	}
	if !strings.Contains(r.Reason, "not a security PASS") {
		t.Fatal("execution mistaken for security proof")
	}
}

func TestCompositionNeverConstructsRuntimeWithoutImageAndHumanDecision(t *testing.T) {
	for _, name := range []string{"catalog", "archive", "no-review", "context", "snapshot"} {
		t.Run(name, func(t *testing.T) {
			input, services, calls := executionFixture(t, "")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch name {
			case "catalog":
				input.Plan.BinarySHA256 = "sha256:" + strings.Repeat("0", 64)
			case "archive":
				if err := os.WriteFile(input.Archive, []byte("wrong"), 0600); err != nil {
					t.Fatal(err)
				}
			case "no-review":
				services.github, _ = auditClient(t, "")
			case "context":
				cancel()
			case "snapshot":
				if err := os.WriteFile(filepath.Join(input.Source, "owned.go"), []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			r := executeProof(ctx, input, services)
			if r.State != "UNVERIFIABLE" || r.ImageDisposition != "NOT_REQUESTED" || len(r.Cases) != 0 {
				t.Fatalf("record=%+v", r)
			}
			for _, c := range *calls {
				if c == "factory" || c == "load" || c == "start" {
					t.Fatalf("denial reached runtime: %v", *calls)
				}
			}
		})
	}
}

func TestCompositionStopsOnAmbiguityDriftOrCaseFailure(t *testing.T) {
	for _, mode := range []string{"factory", "source-before-load", "source-before-start", "load-failure", "preflight", "tool-exit", "cleanup", "false-timeout", "false-cancel"} {
		t.Run(mode, func(t *testing.T) {
			input, services, calls := executionFixture(t, mode)
			r := executeProof(context.Background(), input, services)
			if r.State != "UNVERIFIABLE" {
				t.Fatalf("record=%+v", r)
			}
			starts, loads := 0, 0
			for _, c := range *calls {
				if c == "start" {
					starts++
				}
				if c == "load" {
					loads++
				}
			}
			if mode == "load-failure" && (starts != 0 || loads != 1 || r.ImageDisposition != "MAY_BE_PRESENT") {
				t.Fatalf("ambiguous load retried or executed: %+v %v", r, *calls)
			}
			if (mode == "factory" || mode == "source-before-load") && (starts != 0 || loads != 0) {
				t.Fatalf("pre-load drift escaped: %v", *calls)
			}
			if mode == "source-before-start" && starts != 0 {
				t.Fatalf("runner snapshot failed: %v", *calls)
			}
			if mode == "cleanup" && (starts != 1 || len(r.Cases) != 1 || r.Cases[0].Result.CleanupVerified) {
				t.Fatalf("continued after cleanup failure: %+v", r)
			}
		})
	}
}

func TestFailedEffectsStillMeasureFinalSource(t *testing.T) {
	for _, mode := range []string{"load-failure", "preflight", "tool-exit", "cleanup", "false-timeout", "false-cancel"} {
		t.Run(mode, func(t *testing.T) {
			input, services, _ := executionFixture(t, mode)
			record := executeProof(context.Background(), input, services)
			if record.State != "UNVERIFIABLE" || !record.SourceUnchanged || record.SourceIntegrity != "UNCHANGED" || record.SourceSnapshotAfter != input.Plan.Plan.Snapshot {
				t.Fatalf("failure omitted independent final source measurement: %+v", record)
			}
		})
	}
}

func TestFailedCleanupPreservesReasonAndSeparatesSourceStates(t *testing.T) {
	for _, state := range []string{"UNCHANGED", "CHANGED", "UNVERIFIABLE"} {
		t.Run(state, func(t *testing.T) {
			input, services, _ := executionFixture(t, "cleanup")
			factory := services.newRuntime
			services.newRuntime = func() (loadRuntime, error) {
				rt, err := factory()
				if err != nil {
					return nil, err
				}
				rt.(*composedRuntime).removeHook = func() {
					switch state {
					case "CHANGED":
						if err := os.WriteFile(filepath.Join(input.Source, "owned.go"), []byte("changed during failed cleanup"), 0600); err != nil {
							t.Fatal(err)
						}
					case "UNVERIFIABLE":
						if err := os.RemoveAll(input.Source); err != nil {
							t.Fatal(err)
						}
					}
				}
				return rt, nil
			}
			record := executeProof(context.Background(), input, services)
			if record.State != "UNVERIFIABLE" || record.Reason != "case expectation or owned cleanup unverified" || record.SourceIntegrity != state || record.SourceUnchanged != (state == "UNCHANGED") {
				t.Fatalf("failure reason/source state lost: %+v", record)
			}
			if (record.SourceSnapshotAfter == "") != (state == "UNVERIFIABLE") {
				t.Fatal("missing/changed digest confused with an unmeasured source")
			}
		})
	}
}

func TestMarkerCancellationRequiresExactMarker(t *testing.T) {
	for _, input := range []string{"owned-smoke: proof-started", "proof-started\n", "owned-smoke: proof-startedX\n"} {
		ctx, cancel := context.WithCancel(context.Background())
		r := &cancelRuntime{cancel: cancel}
		w := &markerWriter{destination: io.Discard, runtime: r}
		_, _ = w.Write([]byte(input))
		if r.observed() || ctx.Err() != nil {
			t.Fatalf("inexact marker canceled: %q", input)
		}
		cancel()
	}
}
