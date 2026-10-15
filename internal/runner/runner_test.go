package runner

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func owned() Plan {
	return Plan{Snapshot: strings.Repeat("a", 64), Adapter: "owned-smoke-v1", Tool: "owned-smoke-v1", Image: "sha256:" + strings.Repeat("b", 64), Fixture: "smoke-v1", Config: "owned-v1", Effects: "owned fixture only; no host mounts; no network", Limits: Limits{time.Second, 32 << 20, 1, 8, 32, 1 << 20}}
}

type fake struct {
	probes, starts, removes int
	pre, run, clean         error
	mode                    string
	args                    []string
	started                 chan struct{}
	cleanupLive             bool
}

func (f *fake) Preflight(context.Context, Plan) error { f.probes++; return f.pre }
func (f *fake) Start(ctx context.Context, _ string, args []string, w io.Writer) (int, error) {
	f.starts++
	f.args = args
	if f.started != nil {
		close(f.started)
	}
	switch f.mode {
	case "overflow":
		_, _ = w.Write([]byte(strings.Repeat("x", 100)))
	case "timeout":
		<-ctx.Done()
	case "cancel":
		<-ctx.Done()
	}
	return 0, f.run
}
func (f *fake) Remove(ctx context.Context, _ string) error {
	f.removes++
	_, deadline := ctx.Deadline()
	f.cleanupLive = ctx.Err() == nil && deadline
	return f.clean
}
func TestRejectInvalidSnapshot(t *testing.T) {
	p := owned()
	p.Snapshot = "not-a-digest"
	if _, err := NewOwned(&fake{}, p, func(context.Context) (string, error) { return p.Snapshot, nil }); err == nil {
		t.Fatal("invalid snapshot accepted")
	}
}
func TestBoundaries(t *testing.T) {
	for _, name := range []string{"denied", "stale", "snapshot", "tool", "image", "limits", "unavailable", "success", "error", "cleanup", "overflow", "timeout", "cancel"} {
		t.Run(name, func(t *testing.T) {
			p := owned()
			source := immutableSource(t)
			p.Snapshot = source()
			f := &fake{}
			snapshot := p.Snapshot
			r, err := NewOwned(f, p, func(context.Context) (string, error) {
				if snapshot != p.Snapshot {
					return snapshot, nil
				}
				return source(), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			c := Consent{p.Digest(), true}
			want := Unverifiable
			starts := 0
			ctx := context.Background()
			switch name {
			case "denied":
				c.Approved = false
				want = Denied
			case "stale":
				c.Digest = "old"
				want = Denied
			case "snapshot":
				snapshot = "changed"
			case "tool":
				p.Tool = "other"
				want = Invalid
			case "image":
				p.Image = "tag"
				want = Invalid
			case "limits":
				p.Limits.OutputBytes = -1
				want = Invalid
			case "unavailable":
				f.pre = errors.New("absent")
			case "success":
				starts = 1
				want = Completed
			case "error":
				starts = 1
				f.run = errors.New("launch failed")
			case "cleanup":
				starts = 1
				f.clean = errors.New("daemon lost")
			case "overflow":
				starts = 1
				f.mode = "overflow"
			case "timeout":
				starts = 1
				f.mode = "timeout"
			case "cancel":
				ctxCancel, cancel := context.WithCancel(ctx)
				cancel()
				ctx = ctxCancel
			}
			before := source()
			got := r.Execute(ctx, p, c)
			if source() != before {
				t.Fatal("source files/counter changed")
			}
			if got.Outcome != want || f.starts != starts || f.removes != starts {
				t.Fatalf("result=%+v process starts=%d cleanup=%d", got, f.starts, f.removes)
			}
			if starts == 0 && name != "snapshot" && name != "unavailable" && f.probes != 0 {
				t.Fatal("unexpected preflight process boundary")
			}
			if len(got.Output) > 32 {
				t.Fatal("unbounded output")
			}
			if starts == 1 && name != "cleanup" && !got.CleanupVerified {
				t.Fatal("missing cleanup")
			}
			if starts == 1 {
				args := strings.Join(f.args, " ")
				for _, s := range []string{"--pull=never", "--network=none", "--read-only", "--cap-drop=ALL", "--entrypoint /ci-trust/owned-smoke"} {
					if !strings.Contains(args, s) {
						t.Fatal(args)
					}
				}
				if strings.Contains(args, "--volume") || strings.Contains(args, "--mount") {
					t.Fatal(args)
				}
			}
		})
	}
}
func TestUnavailableDocker(t *testing.T) {
	source := immutableSource(t)
	before := source()
	p := owned()
	p.Snapshot = before
	r, err := NewOwned(UnavailableDocker{}, p, func(context.Context) (string, error) {
		t.Fatal("snapshot read after unavailable preflight")
		return "", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := r.Execute(context.Background(), p, Consent{p.Digest(), true})
	if got.Outcome != Unverifiable || !strings.Contains(got.Reason, "unavailable") {
		t.Fatalf("%+v", got)
	}
	if source() != before {
		t.Fatal("source files/counter changed")
	}
}
func TestDigestBindsDisplay(t *testing.T) {
	p := owned()
	for _, change := range []func(*Plan){func(p *Plan) { p.Snapshot = "changed" }, func(p *Plan) { p.Adapter += "x" }, func(p *Plan) { p.Tool += "x" }, func(p *Plan) { p.Image += "x" }, func(p *Plan) { p.Config += "x" }, func(p *Plan) { p.Fixture += "x" }, func(p *Plan) { p.Limits.PIDs++ }, func(p *Plan) { p.Effects += "x" }} {
		q := p
		change(&q)
		if q.Digest() == p.Digest() {
			t.Fatal("identity not bound")
		}
	}
	if !strings.Contains(p.Display(), p.Digest()) {
		t.Fatal("display missing digest")
	}
}
