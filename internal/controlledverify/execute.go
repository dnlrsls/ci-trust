package controlledverify

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
	"github.com/dnlrsls/ci-trust/internal/ownedimage"
	"github.com/dnlrsls/ci-trust/internal/runner"
)

// ProofInput names reviewed source and owned image data, never a command or
// approval flag. The workflow must provision the private empty client config
// at Source/.git/controlled-proof/docker-config before calling ExecuteProof.
// No target configuration or arbitrary Docker executable is accepted.
type ProofInput struct {
	Plan                            controlledplan.Envelope
	Source, Archive, Binary, Marker string
}

type CaseRecord struct {
	Name                       string
	Result                     runner.Result
	Elapsed                    time.Duration
	MarkerCancellationObserved bool
	ExpectationObserved        bool
}

// ExecutionRecord preserves tool bytes and independent runtime cleanup results.
// RECORDED means only the fixed harness expectations were observed: the tool's
// JSON observations still need review. It is never a security PASS.
type ExecutionRecord struct {
	State, Reason                              string
	Revision, Snapshot, ImageID, BindingDigest string
	ImageDisposition                           string
	SourceUnchanged                            bool
	SourceIntegrity, SourceSnapshotAfter       string
	Cases                                      []CaseRecord
}

type loadRuntime interface {
	runner.Runtime
	LoadOwnedImage(context.Context, []byte, string, string, ownedimage.Metadata) error
}

type proofServices struct {
	github     githubClient
	now        func() time.Time
	inspect    func([]byte, string, string) (ownedimage.Metadata, error)
	newRuntime func() (loadRuntime, error)
}

// ExecuteProof is the single production composition: fixed local Linux Docker,
// compiled catalog, authenticated read-only human review and the existing runner.
// It does not provision Docker, change policy, approve a review or delete images.
// There is deliberately no externally supplied runtime or Approved boolean.
func ExecuteProof(ctx context.Context, input ProofInput, token string) ExecutionRecord {
	if runtime.GOOS != "linux" || !filepath.IsAbs(input.Source) {
		return ExecutionRecord{State: "UNVERIFIABLE", Reason: "local Linux source scope unavailable", ImageDisposition: "NOT_REQUESTED", SourceIntegrity: "NOT_MEASURED"}
	}
	config := filepath.Join(input.Source, ".git", "controlled-proof", "docker-config")
	return executeProof(ctx, input, proofServices{
		github: newGitHubClient(token), now: time.Now,
		inspect: func(a []byte, binary, marker string) (ownedimage.Metadata, error) {
			return inspectCatalog(a, binary, marker, ownedimage.Inspect)
		},
		newRuntime: func() (loadRuntime, error) { return runner.NewLocalDocker("/usr/bin/docker", config) },
	})
}

func executeProof(ctx context.Context, input ProofInput, services proofServices) (record ExecutionRecord) {
	p := input.Plan
	record = ExecutionRecord{State: "UNVERIFIABLE", Reason: "exact image or human authorization unavailable", Revision: p.Revision, Snapshot: p.Plan.Snapshot, ImageID: p.Plan.Image, BindingDigest: p.BindingDigest, ImageDisposition: "NOT_REQUESTED", SourceIntegrity: "NOT_MEASURED"}
	if services.now == nil || services.inspect == nil || services.newRuntime == nil || ctx.Err() != nil || admitPlanImage(p) != nil {
		return record
	}
	archive, err := ownedimage.ReadFile(input.Archive, ownedimage.MaxArchive)
	if err != nil {
		return record
	}
	meta, err := services.inspect(archive, input.Binary, input.Marker)
	if err != nil || admitImage(meta) != nil {
		return record
	}
	decision, err := authorizeProof(ctx, p, input.Source, services.github, services.now)
	if err != nil {
		return record
	}
	// The decision owns copied, canonical case values, not input's mutable pointer.
	cases := decision.cases
	first, err := services.newRuntime()
	if err != nil || first == nil {
		record.Reason = "trusted local runtime unavailable"
		return record
	}
	// Recheck after runtime construction, immediately before the image effect.
	beforeLoad, err := sourceSnapshot(input.Source)
	if err != nil || beforeLoad != cases.Normal.Snapshot {
		record.Reason = "source changed before image load"
		return record
	}
	if _, err := decision.forPlan(cases.Normal, services.now()); err != nil || ctx.Err() != nil {
		return record
	}
	// Independently measure source on EVERY return after requesting an image or
	// container effect, including cancellation and unverified cleanup. Preserve
	// the original failure reason; report source state separately, not as false
	// meaning both "changed" and "never measured".
	defer func() {
		after, err := sourceSnapshot(input.Source)
		record.SourceSnapshotAfter = after
		record.SourceUnchanged = err == nil && after == cases.Normal.Snapshot
		switch {
		case err != nil:
			record.SourceIntegrity = "UNVERIFIABLE"
		case record.SourceUnchanged:
			record.SourceIntegrity = "UNCHANGED"
		default:
			record.SourceIntegrity = "CHANGED"
		}
		if record.State == "RECORDED" && !record.SourceUnchanged {
			record.State = "UNVERIFIABLE"
			record.Reason = "source integrity after execution unverified"
		}
	}()
	loadCtx, cancelLoad := context.WithDeadline(ctx, decision.expires)
	record.ImageDisposition = "MAY_BE_PRESENT"
	err = first.LoadOwnedImage(loadCtx, archive, input.Binary, input.Marker, meta)
	cancelLoad()
	if err != nil {
		record.Reason = "image load or post-load identity/config unverified; no retry or deletion authority"
		return record
	}
	record.ImageDisposition = "RETAINED_UNTAGGED"
	for index, item := range []struct {
		name string
		plan runner.Plan
	}{
		{"normal", cases.Normal}, {"timeout", cases.Timeout}, {"active_cancel", cases.ActiveCancel}, {"combined_output_limit", cases.OutputLimit},
	} {
		consent, err := decision.forPlan(item.plan, services.now())
		if err != nil || ctx.Err() != nil {
			record.Reason = "decision expired or caller canceled before case"
			return record
		}
		var rt runner.Runtime = first
		if index != 0 {
			fresh, err := services.newRuntime()
			if err != nil || fresh == nil {
				record.Reason = "fresh trusted runtime unavailable"
				return record
			}
			rt = fresh
		}
		caseCtx, cancel := context.WithDeadline(ctx, decision.expires)
		var marker *cancelRuntime
		if item.name == "active_cancel" {
			marker = &cancelRuntime{Runtime: rt, cancel: cancel}
			rt = marker
		}
		snapshot := func(c context.Context) (string, error) {
			if c.Err() != nil {
				return "", c.Err()
			}
			return sourceSnapshot(input.Source)
		}
		run, err := runner.NewOwned(rt, item.plan, snapshot)
		if err != nil {
			cancel()
			record.Reason = "owned runner unavailable"
			return record
		}
		start := time.Now()
		result := run.Execute(caseCtx, item.plan, consent)
		entry := CaseRecord{Name: item.name, Result: result, Elapsed: time.Since(start)}
		if marker != nil {
			entry.MarkerCancellationObserved = marker.observed()
		}
		entry.ExpectationObserved = expectedCase(entry, item.plan)
		cancel()
		record.Cases = append(record.Cases, entry)
		if !entry.ExpectationObserved {
			record.Reason = "case expectation or owned cleanup unverified"
			return record
		}
	}
	record.State = "RECORDED"
	record.Reason = "fixed harness outcomes recorded; review owned tool observations separately; not a security PASS"
	return record
}

func expectedCase(c CaseRecord, p runner.Plan) bool {
	r := c.Result
	if !r.CleanupVerified {
		return false
	}
	switch c.Name {
	case "normal":
		return r.Outcome == runner.Completed && r.ExitCode == 0
	case "timeout":
		return r.Outcome == runner.Unverifiable && r.Reason == "execution canceled or walltime exceeded" && c.Elapsed >= p.Limits.Wall
	case "active_cancel":
		return r.Outcome == runner.Unverifiable && r.Reason == "execution canceled or walltime exceeded" && c.MarkerCancellationObserved
	case "combined_output_limit":
		return r.Outcome == runner.Unverifiable && r.Reason == "output limit exceeded" && len(r.Output) == p.Limits.OutputBytes
	}
	return false
}

// cancelRuntime observes the exact owned start marker in the real output path;
// dispatch, preflight and a generic client error cannot fake active cancellation.
type cancelRuntime struct {
	runner.Runtime
	cancel context.CancelFunc
	mu     sync.Mutex
	seen   bool
}

func (r *cancelRuntime) Start(ctx context.Context, name string, args []string, w io.Writer) (int, error) {
	if r.cancel == nil {
		return -1, errors.New("missing owned cancellation")
	}
	return r.Runtime.Start(ctx, name, args, &markerWriter{destination: w, runtime: r})
}
func (r *cancelRuntime) observed() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.seen }
func (r *cancelRuntime) OOMEvent() *bool {
	if source, ok := r.Runtime.(interface{ OOMEvent() *bool }); ok {
		return source.OOMEvent()
	}
	return nil
}

type markerWriter struct {
	mu          sync.Mutex
	destination io.Writer
	runtime     *cancelRuntime
	tail        string
}

func (w *markerWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.destination.Write(p)
	const marker = "owned-smoke: proof-started\n"
	if n > 0 {
		prefix := n
		if prefix > len(marker) {
			prefix = len(marker)
		}
		if strings.Contains(string(p[:n]), marker) || strings.Contains(w.tail+string(p[:prefix]), marker) {
			w.runtime.mu.Lock()
			w.runtime.seen = true
			w.runtime.mu.Unlock()
			w.runtime.cancel()
		}
		if n >= len(marker)-1 {
			w.tail = string(p[n-(len(marker)-1) : n])
		} else {
			w.tail += string(p[:n])
			if len(w.tail) > len(marker)-1 {
				w.tail = w.tail[len(w.tail)-(len(marker)-1):]
			}
		}
	}
	return n, err
}
