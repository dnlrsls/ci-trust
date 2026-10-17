package runner

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

type lintRuntimeFixture struct {
	calls, snapshots     int
	capture              capturedRun
	startErr, cleanupErr error
	args                 []string
	input                string
}

func (r *lintRuntimeFixture) Preflight(context.Context, Plan) error { r.calls++; return nil }
func (r *lintRuntimeFixture) Start(context.Context, string, []string, io.Writer) (int, error) {
	panic("lint used merged transport")
}
func (r *lintRuntimeFixture) startCaptured(_ context.Context, _ string, args []string, input string) (capturedRun, error) {
	r.calls++
	r.args = append([]string{}, args...)
	r.input = input
	return r.capture, r.startErr
}
func (r *lintRuntimeFixture) Remove(context.Context, string) error { r.calls++; return r.cleanupErr }
func (r *lintRuntimeFixture) snapshot(context.Context) (string, error) {
	r.snapshots++
	return strings.Repeat("a", 64), nil
}
func TestFixedLintPlanAndClosedConstruction(t *testing.T) {
	binding := strings.Repeat("b", 64)
	for _, role := range []ls.Role{ls.Clean, ls.Fault, ls.Restored} {
		p, e := FixedLintPlan(strings.Repeat("a", 64), role, binding)
		if e != nil {
			t.Fatal(e)
		}
		if p.Image != lintcatalog.Current().Image || p.Transport != splitStdin || p.Fixture != string(role) || p.Limits != (Limits{Wall: 10 * time.Second, MemoryBytes: 256 << 20, CPUs: 1, PIDs: 16, OutputBytes: 64 << 10, ScratchBytes: 16 << 20}) {
			t.Fatal("wrong fixed plan", p)
		}
		rt := &lintRuntimeFixture{}
		if r, e := NewLintOwned(rt, p, rt.snapshot); e == nil || e.Error() != "NODE_SECURITY_HOLD" || r != nil || rt.calls != 0 || rt.snapshots != 0 {
			t.Fatal("held construction crossed dependency")
		}
		if r, e := NewOwned(rt, p, rt.snapshot); e == nil || r != nil {
			t.Fatal("legacy constructor admitted lint")
		}
		argv := dockerArgs(p, "ci-trust-1-1")
		want := []string{"run", "--interactive", "--name", "ci-trust-1-1", "--pull=never", "--network=none", "--read-only", "--user=65532:65532", "--workdir=/work", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--memory", "268435456", "--memory-swap", "268435456", "--cpus", "1", "--pids-limit", "16", "--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=16777216", "--entrypoint", "/ci-trust/owned-eslint", p.Image, "--role", string(role), "--binding", binding}
		if !reflect.DeepEqual(argv, want) {
			t.Fatalf("unowned argv: %q", argv)
		}
	}
	for _, bad := range []string{"", strings.Repeat("b", 63), strings.Repeat("B", 64)} {
		if _, e := FixedLintPlan(strings.Repeat("a", 64), ls.Clean, bad); e == nil {
			t.Fatal("invalid binding accepted")
		}
	}
}
func TestLintCapturedCompletionIsNotToolExit(t *testing.T) {
	binding := strings.Repeat("b", 64)
	for _, role := range []ls.Role{ls.Clean, ls.Fault, ls.Restored} {
		p, _ := FixedLintPlan(strings.Repeat("a", 64), role, binding)
		inner := 0
		if role == ls.Fault {
			inner = 1
		}
		frame, e := ls.EncodeESLintReceipt(role, binding, ls.ESLintCapture{Status: ls.Completed, ExitCode: inner, Stdout: []byte("[]")})
		if e != nil {
			t.Fatal(e)
		}
		rt := &lintRuntimeFixture{capture: capturedRun{Stdout: string(frame), ExitCode: 0}}
		// Private trusted composition fixture exercises the public Execute path;
		// the public production constructor above is still denied under the hold.
		r, e := newLintOwned(rt, p, rt.snapshot)
		if e != nil {
			t.Fatal(e)
		}
		result := r.Execute(context.Background(), p, Consent{Approved: true, Digest: p.Digest()})
		if result.Outcome != Completed || result.ExitCode != 0 || !result.CleanupVerified || result.Stdout != string(frame) || result.Stderr != "" || result.Output != "" || rt.input != "" || rt.calls != 3 || rt.snapshots != 1 {
			t.Fatalf("capture=%+v calls=%d", result, rt.calls)
		}
	}
}

type lintClientCounter struct{ calls int }

func (c *lintClientCounter) call(context.Context, io.Writer, ...string) (string, error) {
	c.calls++
	return "", errors.New("unexpected client call")
}
func (c *lintClientCounter) attach(context.Context, string, string, int, time.Duration) (string, string, error) {
	c.calls++
	return "", "", errors.New("unexpected attach")
}
func TestLintLocalDockerGateAndEmptyInput(t *testing.T) {
	p, _ := FixedLintPlan(strings.Repeat("a", 64), ls.Clean, strings.Repeat("b", 64))
	client := &lintClientCounter{}
	d := &LocalDocker{client: client, host: "linux"}
	if e := d.Preflight(context.Background(), p); e == nil || client.calls != 0 {
		t.Fatal("held preflight reached client")
	}
	// Trusted test-only lifecycle fixture; no daemon or production admission.
	d.plan = p
	d.ready = true
	if _, e := d.startCaptured(context.Background(), "ci-trust-1-1", dockerArgs(p, "ci-trust-1-1"), "unowned"); e == nil || client.calls != 0 {
		t.Fatal("nonempty input reached client")
	}
	rt := &lintRuntimeFixture{}
	r, e := newLintOwned(rt, p, func(context.Context) (string, error) { rt.snapshots++; return strings.Repeat("c", 64), nil })
	if e != nil {
		t.Fatal(e)
	}
	got := r.Execute(context.Background(), p, Consent{Approved: true, Digest: p.Digest()})
	if got.Outcome != Unverifiable || rt.calls != 1 || rt.snapshots != 1 {
		t.Fatal("changed source reached capture", got)
	}
}

func TestLintDeniedAndFailedHarnessNeverComplete(t *testing.T) {
	p, _ := FixedLintPlan(strings.Repeat("a", 64), ls.Fault, strings.Repeat("b", 64))
	for _, tc := range []string{"denied", "stale", "outer-one", "attach", "cleanup"} {
		t.Run(tc, func(t *testing.T) {
			rt := &lintRuntimeFixture{capture: capturedRun{ExitCode: 0}}
			consent := Consent{Approved: true, Digest: p.Digest()}
			switch tc {
			case "denied":
				consent.Approved = false
			case "stale":
				consent.Digest = "other"
			case "outer-one":
				rt.capture.ExitCode = 1
			case "attach":
				rt.startErr = errors.New("client failure")
			case "cleanup":
				rt.cleanupErr = errors.New("not absent")
			}
			r, e := newLintOwned(rt, p, rt.snapshot)
			if e != nil {
				t.Fatal(e)
			}
			got := r.Execute(context.Background(), p, consent)
			if got.Outcome == Completed {
				t.Fatal("harness failure completed")
			}
			if (tc == "denied" || tc == "stale") && (rt.calls != 0 || rt.snapshots != 0) {
				t.Fatal("denied request reached runtime")
			}
		})
	}
	for _, mutate := range []func(*Plan){func(q *Plan) { q.Image = "sha256:" + strings.Repeat("c", 64) }, func(q *Plan) { q.Config += " --fix" }, func(q *Plan) { q.Limits.PIDs++ }, func(q *Plan) { q.Fixture = "OTHER" }} {
		q := p
		mutate(&q)
		rt := &lintRuntimeFixture{}
		if _, e := newLintOwned(rt, q, rt.snapshot); e == nil {
			t.Fatal("mutated plan accepted")
		}
	}
}
