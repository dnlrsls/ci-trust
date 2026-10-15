// Package runner gates owned fixture execution. It never executes repository code.
package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"
	"time"
)

// Limits bounds all caller-controlled resource allocations; invalid values fail before start.
type Limits struct {
	Wall         time.Duration
	MemoryBytes  int64
	CPUs         int
	PIDs         int
	OutputBytes  int
	ScratchBytes int64
}

// Plan binds an owned adapter identity and source digest, not executable target configuration.
type Plan struct {
	Snapshot, Adapter, Tool, Image, Fixture, Config string
	Limits                                          Limits
	Effects                                         string
}

// Consent transports a trusted human UI decision; it does not authenticate human presence.
// Agent-provided approval arguments must never be converted into this DTO.
type Consent struct {
	Digest   string
	Approved bool
}

// Outcome classifies harness execution, never validator sensitivity or CI enforcement.
type Outcome string

const (
	Denied       Outcome = "DENIED"
	Invalid      Outcome = "INVALID"
	Unverifiable Outcome = "UNVERIFIABLE"
	Completed    Outcome = "COMPLETED"
)

// Result preserves a tool exit code without interpreting it as validator PASS/FAIL.
// CleanupVerified is true only when the trusted runtime attests container absence.
type Result struct {
	Outcome         Outcome
	Reason          string
	ExitCode        int
	Output          string
	CleanupVerified bool
}

// Snapshot must remeasure the same target immediately before Start, without executing it.
type Snapshot func(context.Context) (string, error)

// Runtime is a trusted harness boundary, NOT an extension point for target data.
// Preflight must check local Linux Docker scope, immutable image presence and
// required limit capabilities without starting a container. Capability metadata
// is not independent enforcement proof. Start must honor cancellation and send
// both streams to output. Remove must bind ownership and an exact container ID
// before removal, then verify absence; ambiguous creation must fail closed.
type Runtime interface {
	Preflight(context.Context, Plan) error
	Start(context.Context, string, []string, io.Writer) (int, error)
	Remove(context.Context, string) error
}

// Runner executes only its trusted composition-time owned plan through a trusted runtime.
type Runner struct {
	runtime  Runtime
	owned    Plan
	snapshot Snapshot
}

var imageID = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

func valid(p Plan) bool {
	l := p.Limits
	return regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(p.Snapshot) && p.Adapter == "owned-smoke-v1" && p.Tool == "owned-smoke-v1" && imageID.MatchString(p.Image) && p.Fixture == "smoke-v1" && (p.Config == "owned-v1" || p.Config == "owned-proof-v1") && p.Effects == "owned fixture only; no host mounts; no network" && l.Wall > 0 && l.Wall <= time.Minute && l.MemoryBytes >= 16<<20 && l.MemoryBytes <= 1<<30 && l.CPUs >= 1 && l.CPUs <= 2 && l.PIDs >= 1 && l.PIDs <= 64 && l.OutputBytes >= 1 && l.OutputBytes <= 1<<20 && l.ScratchBytes >= 1 && l.ScratchBytes <= 64<<20
}

// NewOwned is for trusted application composition only. owned must come from a
// reviewed, compiled adapter catalog, never repository configuration/tool arguments.
// No production adapter catalog is currently shipped.
func NewOwned(rt Runtime, owned Plan, snapshot Snapshot) (*Runner, error) {
	if rt == nil || snapshot == nil || !valid(owned) {
		return nil, errors.New("invalid owned plan")
	}
	return &Runner{rt, owned, snapshot}, nil
}

// Digest binds every displayed identity, limit and effect; it is not an approval token.
func (p Plan) Digest() string {
	b, _ := json.Marshal(p)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Display renders the complete plan for a future human-interaction boundary.
func (p Plan) Display() string {
	b, _ := json.MarshalIndent(p, "", "  ")
	return string(b) + "\nDigest: " + p.Digest() + "\nApproval must come from a human UI, not an agent tool argument.\n"
}

// Execute consumes a decision supplied by a trusted human-interaction boundary.
// Consent is not authentication: callers must never translate an agent's yes flag
// into Approved. This package has no CLI or autonomous consent facility.
func (r *Runner) Execute(ctx context.Context, p Plan, c Consent) Result {
	result := Result{Outcome: Invalid, ExitCode: -1}
	if !valid(p) || p != r.owned {
		result.Reason = "plan outside owned allowlist"
		return result
	}
	if !c.Approved || c.Digest != p.Digest() {
		result.Outcome = Denied
		result.Reason = "absent or stale explicit consent"
		return result
	}
	result.Outcome = Unverifiable
	if err := ctx.Err(); err != nil {
		result.Reason = err.Error()
		return result
	}
	if err := r.runtime.Preflight(ctx, p); err != nil {
		result.Reason = "preflight: " + err.Error()
		return result
	}
	snap, err := r.snapshot(ctx)
	if err != nil || snap != p.Snapshot {
		result.Reason = "source snapshot changed or unavailable"
		return result
	}
	if err := ctx.Err(); err != nil {
		result.Reason = err.Error()
		return result
	}
	// No target reads or runtime calls intervene between remeasurement and start.
	runCtx, cancel := context.WithTimeout(ctx, p.Limits.Wall)
	defer cancel()
	name := uniqueName()
	out := &boundedOutput{max: p.Limits.OutputBytes, cancel: cancel}
	code, runErr := r.runtime.Start(runCtx, name, dockerArgs(p, name), out)
	cleanupCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
	cleanupErr := r.runtime.Remove(cleanupCtx, name)
	stop()
	output, overflow := out.state()
	result.Output = output
	result.ExitCode = code
	result.CleanupVerified = cleanupErr == nil
	switch {
	case cleanupErr != nil:
		result.Reason = "cleanup unverified: " + cleanupErr.Error()
	case overflow:
		result.Reason = "output limit exceeded"
	case runCtx.Err() != nil:
		result.Reason = "execution canceled or walltime exceeded"
	case runErr != nil:
		result.Reason = "harness: " + runErr.Error()
	default:
		result.Outcome = Completed
		result.Reason = "tool exited; exit status is not validator sensitivity evidence"
	}
	return result
}

var sequence struct {
	sync.Mutex
	n uint64
}

func uniqueName() string {
	sequence.Lock()
	defer sequence.Unlock()
	sequence.n++
	return fmt.Sprintf("ci-trust-%d-%d", time.Now().UnixNano(), sequence.n)
}
func dockerArgs(p Plan, name string) []string {
	l := p.Limits
	return []string{"run", "--name", name, "--pull=never", "--network=none", "--read-only", "--user=65534:65534", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--memory", fmt.Sprint(l.MemoryBytes), "--memory-swap", fmt.Sprint(l.MemoryBytes), "--cpus", fmt.Sprint(l.CPUs), "--pids-limit", fmt.Sprint(l.PIDs), "--tmpfs", fmt.Sprintf("/tmp:rw,noexec,nosuid,nodev,size=%d", l.ScratchBytes), "--entrypoint", "/ci-trust/owned-smoke", p.Image, "--fixture=smoke-v1", "--config=" + p.Config}
}

type boundedOutput struct {
	sync.Mutex
	data     []byte
	max      int
	overflow bool
	cancel   context.CancelFunc
}

func (b *boundedOutput) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	n := len(p)
	remaining := b.max - len(b.data)
	if len(p) > remaining {
		p = p[:remaining]
		b.overflow = true
		b.cancel()
	}
	b.data = append(b.data, p...)
	if b.overflow {
		return len(p), errors.New("output limit")
	}
	return n, nil
}
func (b *boundedOutput) state() (string, bool) {
	b.Lock()
	defer b.Unlock()
	return string(b.data), b.overflow
}
func (b *boundedOutput) String() string { s, _ := b.state(); return s }

// UnavailableDocker is fail-closed until a real backend can attest local daemon,
// resource enforcement and cleanup. It never probes, provisions or starts Docker.
type UnavailableDocker struct{}

func (UnavailableDocker) Preflight(context.Context, Plan) error {
	return errors.New("local Linux Docker and enforceable limits not attested; real isolation proof unavailable")
}
func (UnavailableDocker) Start(context.Context, string, []string, io.Writer) (int, error) {
	return -1, errors.New("runtime unavailable")
}
func (UnavailableDocker) Remove(context.Context, string) error {
	return errors.New("runtime unavailable")
}
