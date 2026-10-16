package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Metadata and ownership use the existing inert client; only attach executes our
// own test binary. No daemon, socket, image or target code is accessed.
type attachExecutable struct {
	ownedClient
	executable *executableDocker
}

func (f *attachExecutable) call(ctx context.Context, w io.Writer, args ...string) (string, error) {
	if args[0] == "start" {
		f.calls = append(f.calls, append([]string(nil), args...))
		return f.executable.call(ctx, w, args...)
	}
	return f.ownedClient.call(ctx, w, args...)
}

func TestOwnedAttachDeadlines(t *testing.T) {
	for _, scenario := range []string{"longer-than-metadata", "caller-cancellation", "direct-background-plan-wall"} {
		t.Run(scenario, func(t *testing.T) {
			source := ownedTree(t)
			before := source()
			p := owned()
			p.Snapshot = before
			p.Limits.Wall = 5 * time.Second
			executable, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			f := &attachExecutable{executable: &executableDocker{executable, t.TempDir()}}
			d := &LocalDocker{client: f, host: "linux"}
			ctx := context.Background()
			if scenario == "caller-cancellation" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer cancel()
			}
			if scenario == "direct-background-plan-wall" {
				p.Limits.Wall = 100 * time.Millisecond
			}
			if err := d.Preflight(ctx, p); err != nil {
				t.Fatal(err)
			}
			var output strings.Builder
			start := time.Now()
			code, err := d.Start(ctx, "ci-trust-1-1", dockerArgs(p, "ci-trust-1-1"), &output)
			elapsed := time.Since(start)
			if scenario == "longer-than-metadata" {
				if code != 7 || err != nil || output.String() != "owned attach stdout\nowned attach stderr\n" || elapsed < 2300*time.Millisecond || elapsed >= 5*time.Second {
					t.Fatalf("exit=%d output=%q err=%v elapsed=%v", code, output.String(), err, elapsed)
				}
			} else if code != -1 || !errors.Is(err, context.DeadlineExceeded) || output.String() != "" || elapsed >= time.Second {
				t.Fatalf("exit=%d output=%q err=%v elapsed=%v", code, output.String(), err, elapsed)
			}
			cleanup, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := d.Remove(cleanup, "ci-trust-1-1"); err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{}
			for _, args := range f.calls {
				counts[args[0]]++
			}
			inspections := 2
			if scenario == "longer-than-metadata" {
				inspections = 3
			}
			want := map[string]int{"info": 1, "image": 1, "create": 1, "inspect": inspections, "start": 1, "rm": 1, "ps": 1}
			if !reflect.DeepEqual(counts, want) || !f.removed || source() != before {
				t.Fatalf("calls=%v want=%v removed=%v source unchanged=%v", counts, want, f.removed, source() == before)
			}
			t.Logf("exit=%d output=%q err=%v elapsed=%v calls=%v", code, output.String(), err, elapsed, counts)
		})
	}
}

func TestFixedProofConfig(t *testing.T) {
	for _, config := range []string{"owned-v1", "owned-proof-v1", "other", "owned-proof-v1 --evil", ""} {
		t.Run(config, func(t *testing.T) {
			source := ownedTree(t)
			before := source()
			p := owned()
			p.Snapshot = before
			p.Config = config
			original := p
			f := &ownedClient{}
			r, err := NewOwned(&LocalDocker{client: f, host: "linux"}, p, func(context.Context) (string, error) { return source(), nil })
			allowed := config == "owned-v1" || config == "owned-proof-v1"
			if !allowed {
				if err == nil || len(f.calls) != 0 || source() != before || p != original {
					t.Fatalf("invalid config admitted: err=%v calls=%v", err, f.calls)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			result := r.Execute(context.Background(), p, Consent{p.Digest(), true})
			if result.Outcome != Completed || result.ExitCode != 7 || result.Output != "stdout\nstderr\n" || !result.CleanupVerified {
				t.Fatalf("%+v", result)
			}
			counts := map[string]int{}
			for _, args := range f.calls {
				counts[args[0]]++
				if args[0] == "create" && args[len(args)-1] != "--config="+config {
					t.Fatalf("config vector=%q", args)
				}
			}
			if !reflect.DeepEqual(counts, map[string]int{"info": 1, "image": 1, "create": 1, "inspect": 3, "start": 1, "rm": 1, "ps": 1}) || source() != before || p != original {
				t.Fatalf("calls=%v source unchanged=%v plan unchanged=%v", counts, source() == before, p == original)
			}
		})
	}
}
