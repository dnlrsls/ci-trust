package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Metadata and attachment are emulated. No Docker executable or socket is accessed.
type splitClient struct {
	ownedClient
	input    string
	attaches int
}

type splitExecutableClient struct {
	splitClient
	executable *executableDocker
}

func (f *splitExecutableClient) attach(ctx context.Context, id, input string, max int, wall time.Duration) (string, string, error) {
	f.attaches++
	f.calls = append(f.calls, []string{"start", "--attach", "--interactive", id})
	return f.executable.attach(ctx, id, input, max, wall)
}

func TestContainerIOOwnedExecutable(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	f := &splitExecutableClient{executable: &executableDocker{executable, t.TempDir()}}
	source := ownedTree(t)
	before := source()
	p := splitPlan()
	p.Snapshot = before
	p.Limits.OutputBytes = 64 << 10
	d := &LocalDocker{client: f, host: "linux"}
	r, err := NewOwned(d, p, func(context.Context) (string, error) { return source(), nil })
	if err != nil {
		t.Fatal(err)
	}
	got := r.Execute(context.Background(), p, Consent{p.Digest(), true})
	if got.Outcome != Completed || got.Stdout != "owned attach stdout\n" || got.Stderr != "owned attach stderr\n" || got.ExitCode != 7 || !got.CleanupVerified || source() != before {
		t.Fatalf("%+v", got)
	}
	// Tool exit is independently inspected (the fake says 7); client exit is 0.
	// Neither the client exit nor its output is substituted for container state.
}

func TestFixedAttachProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := &executableDocker{executable, t.TempDir()}
	for _, input := range []string{"owned fixture\n\x00tail", "owned-attach-delay", "owned-attach-overflow", "owned-attach-error"} {
		wall := time.Second
		if input == "owned-attach-delay" {
			wall = 4 * time.Second
		}
		stdout, stderr, err := c.attach(context.Background(), fakeID, input, 64<<10, wall)
		if len(stdout)+len(stderr) > 64<<10 {
			t.Fatal("shared attach output budget exceeded")
		}
		switch input {
		case "owned-attach-overflow", "owned-attach-error":
			if err == nil {
				t.Fatal("failed attach accepted")
			}
		default:
			if err != nil || stdout != "owned attach stdout\n"+input || stderr != "owned attach stderr\n" {
				t.Fatalf("%q %q %v", stdout, stderr, err)
			}
		}
	}
	if _, _, err := c.attach(context.Background(), "not-an-id", "", 64<<10, time.Second); !errors.Is(err, invalidTransport) {
		t.Fatalf("invalid ID accepted: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, _, err := c.attach(ctx, fakeID, "owned-attach-delay", 64<<10, time.Minute); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("attach cancellation lost: %v", err)
	}
}

func (f *splitClient) call(ctx context.Context, w io.Writer, args ...string) (string, error) {
	s, err := f.ownedClient.call(ctx, w, args...)
	if args[0] == "inspect" && err == nil {
		var rows []map[string]any
		if err := json.Unmarshal([]byte(s), &rows); err != nil {
			return "", err
		}
		config, ok := rows[0]["Config"].(map[string]any)
		if !ok {
			return s, nil
		}
		config["Tty"], config["OpenStdin"] = false, true
		if f.mode == "tty" {
			config["Tty"] = true
		}
		if f.mode == "closed-input" {
			config["OpenStdin"] = false
		}
		if f.mode == "missing-io" {
			delete(config, "Tty")
			delete(config, "OpenStdin")
		}
		b, _ := json.Marshal(rows)
		s = string(b)
	}
	return s, err
}

func (f *splitClient) attach(ctx context.Context, id, input string, max int, wall time.Duration) (string, string, error) {
	f.attaches++
	f.input = input
	f.calls = append(f.calls, []string{"start", "--attach", "--interactive", id})
	if f.started != nil {
		close(f.started)
	}
	switch f.mode {
	case "timeout", "cancel":
		<-ctx.Done()
		return "", "", ctx.Err()
	case "overflow":
		return strings.Repeat("x", max), "overflow", errors.New("output limit")
	case "attach":
		return "partial", "transport failed", errors.New("attach interrupted")
	}
	return "owned stdout\n", "owned stderr\n", nil
}

func splitPlan() Plan { p := owned(); p.Transport = splitStdin; return p }

func TestContainerIOBoundConsent(t *testing.T) {
	p := splitPlan()
	legacy := p
	legacy.Transport = ""
	if p.Digest() == legacy.Digest() {
		t.Fatal("transport not bound to consent")
	}
	b, _ := json.Marshal(legacy)
	wantLegacy := `{"Snapshot":"` + strings.Repeat("a", 64) + `","Adapter":"owned-smoke-v1","Tool":"owned-smoke-v1","Image":"sha256:` + strings.Repeat("b", 64) + `","Fixture":"smoke-v1","Config":"owned-v1","Limits":{"Wall":1000000000,"MemoryBytes":33554432,"CPUs":1,"PIDs":8,"OutputBytes":32,"ScratchBytes":1048576},"Effects":"owned fixture only; no host mounts; no network"}`
	if string(b) != wantLegacy {
		t.Fatal("legacy JSON changed")
	}
	for _, scenario := range []string{"success", "denied", "old-consent", "snapshot", "timeout", "overflow", "attach", "tty", "closed-input", "missing-io", "absence", "remove", "collision", "inspect", "foreign", "oom", "startup"} {
		t.Run(scenario, func(t *testing.T) {
			source := ownedTree(t)
			before := source()
			p := splitPlan()
			p.Snapshot = before
			if scenario == "timeout" {
				p.Limits.Wall = 20 * time.Millisecond
			}
			f := &splitClient{ownedClient: ownedClient{mode: scenario, collision: scenario == "collision", inspectError: scenario == "inspect", oom: scenario == "oom", startup: scenario == "startup"}}
			d := &LocalDocker{client: f, host: "linux"}
			r, err := NewOwned(d, p, func(context.Context) (string, error) {
				if scenario == "snapshot" {
					return "changed", nil
				}
				return source(), nil
			})
			if err != nil {
				t.Fatal(err)
			}
			c := Consent{p.Digest(), true}
			if scenario == "denied" {
				c.Approved = false
			}
			if scenario == "old-consent" {
				old := p
				old.Transport = ""
				c.Digest = old.Digest()
			}
			got := r.Execute(context.Background(), p, c)
			if scenario == "success" {
				if got.Outcome != Completed || got.ExitCode != 7 || got.Stdout != "owned stdout\n" || got.Stderr != "owned stderr\n" || got.Output != "" || !got.CleanupVerified {
					t.Fatalf("%+v", got)
				}
			} else if got.Outcome == Completed {
				t.Fatalf("failure accepted: %+v", got)
			}
			if scenario == "denied" || scenario == "old-consent" {
				if got.Outcome != Denied || len(f.calls) != 0 {
					t.Fatalf("denied touched runtime: %+v %v", got, f.calls)
				}
			}
			if scenario == "snapshot" && f.attaches != 0 {
				t.Fatal("changed source started")
			}
			if scenario == "tty" || scenario == "closed-input" || scenario == "missing-io" {
				if f.attaches != 0 || !got.CleanupVerified {
					t.Fatalf("bad IO state: %+v", got)
				}
			}
			if len(got.Stdout)+len(got.Stderr) > p.Limits.OutputBytes || source() != before {
				t.Fatal("output/source bound violated")
			}
			if f.attaches > 0 && f.input != "" {
				t.Fatal("unowned smoke input")
			}
			if f.attaches > 0 && scenario != "absence" && scenario != "remove" && !got.CleanupVerified {
				t.Fatalf("cleanup missing: %+v", got)
			}
			if scenario == "attach" && (got.ExitCode != 7 || got.Stdout != "partial" || got.Stderr != "transport failed") {
				t.Fatalf("failed attach lost inspected exit or streams: %+v", got)
			}
			if scenario == "collision" || scenario == "inspect" || scenario == "foreign" {
				for _, a := range f.calls {
					if a[0] == "rm" {
						t.Fatal("ambiguous ownership granted removal authority")
					}
				}
			}
			for _, a := range f.calls {
				if a[0] == "create" {
					if !strings.Contains(strings.Join(a, " "), "--interactive") || strings.Contains(strings.Join(a, " "), "--tty") {
						t.Fatalf("%v", a)
					}
				}
				if a[0] == "start" && !reflect.DeepEqual(a, []string{"start", "--attach", "--interactive", fakeID}) {
					t.Fatalf("%v", a)
				}
			}
		})
	}
}

func TestContainerIORejectsUnownedInput(t *testing.T) {
	p := splitPlan()
	f := &splitClient{}
	d := &LocalDocker{client: f, host: "linux"}
	if err := d.Preflight(context.Background(), p); err != nil {
		t.Fatal(err)
	}
	before := len(f.calls)
	if _, err := d.startCaptured(context.Background(), "ci-trust-1-1", dockerArgs(p, "ci-trust-1-1"), "unowned"); err == nil {
		t.Fatal("unowned input accepted")
	}
	if len(f.calls) != before || d.attempted {
		t.Fatal("unowned input touched lifecycle")
	}
	if _, err := d.Start(context.Background(), "ci-trust-1-1", dockerArgs(p, "ci-trust-1-1"), io.Discard); err == nil || len(f.calls) != before || d.attempted {
		t.Fatal("legacy Start bypassed split transport")
	}
	args := dockerArgs(p, "ci-trust-1-1")
	args = append(args, "--unowned")
	if _, err := d.startCaptured(context.Background(), "ci-trust-1-1", args, ""); err == nil || len(f.calls) != before || d.attempted {
		t.Fatal("arbitrary argv accepted")
	}
}

func TestContainerIOCancellationAndSingleUse(t *testing.T) {
	p := splitPlan()
	source := ownedTree(t)
	beforeSource := source()
	p.Snapshot = beforeSource
	p.Limits.Wall = 5 * time.Second
	f := &splitClient{ownedClient: ownedClient{mode: "cancel", started: make(chan struct{})}}
	d := &LocalDocker{client: f, host: "linux"}
	r, err := NewOwned(d, p, func(context.Context) (string, error) { return source(), nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- r.Execute(ctx, p, Consent{p.Digest(), true}) }()
	select {
	case <-f.started:
		cancel()
	case <-time.After(2 * time.Second):
		cancel()
		<-done
		t.Fatal("attach not entered")
	}
	var got Result
	select {
	case got = <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("cancellation stranded lifecycle")
	}
	if got.Outcome != Unverifiable || !got.CleanupVerified || !f.removed || source() != beforeSource {
		t.Fatalf("%+v", got)
	}
	before := len(f.calls)
	if _, err := d.startCaptured(context.Background(), "ci-trust-1-1", dockerArgs(p, "ci-trust-1-1"), ""); err == nil || len(f.calls) != before {
		t.Fatal("reused single-use runtime")
	}
}

func TestContainerIOProfileAndCapabilities(t *testing.T) {
	for _, mutate := range []func(*Plan){
		func(p *Plan) { p.Transport = "unknown" },
		func(p *Plan) { p.Config = "owned-proof-v1" },
		func(p *Plan) { p.Limits.OutputBytes = (64 << 10) + 1 },
		func(p *Plan) { p.Adapter = "owned-eslint-v1" },
	} {
		p := splitPlan()
		mutate(&p)
		if _, err := NewOwned(&LocalDocker{}, p, func(context.Context) (string, error) { return p.Snapshot, nil }); err == nil {
			t.Fatal("unreviewed split profile admitted")
		}
	}
	p := splitPlan()
	f := &fake{}
	r, err := NewOwned(f, p, func(context.Context) (string, error) { return p.Snapshot, nil })
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Execute(context.Background(), p, Consent{p.Digest(), true}); got.Outcome != Unverifiable || f.probes != 0 || f.starts != 0 {
		t.Fatalf("missing capability reached runtime: %+v", got)
	}
	client := &ownedClient{}
	d := &LocalDocker{client: client, host: "linux"}
	if err := d.Preflight(context.Background(), p); err == nil || len(client.calls) != 0 {
		t.Fatal("missing client capability touched daemon")
	}
}
