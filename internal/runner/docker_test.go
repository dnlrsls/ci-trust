package runner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type ownedClient struct {
	calls                                 [][]string
	collision, inspectError, startup, oom bool
	removed                               bool
	mode                                  string
	started                               chan struct{}
}

var fakeID = strings.Repeat("c", 64)

// Include file names, directories, source bytes and an execution sentinel.
func ownedTree(t *testing.T) func() string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"source.js", "counter", "untracked sentinel"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("owned immutable "+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return func() string {
		t.Helper()
		h := sha256.New()
		err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			fmt.Fprintf(h, "%q:%v:", relative, entry.Type())
			if !entry.IsDir() {
				b, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				fmt.Fprintf(h, "%d:", len(b))
				_, _ = h.Write(b)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return fmt.Sprintf("%x", h.Sum(nil))
	}
}

func (f *ownedClient) call(ctx context.Context, w io.Writer, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	f.calls = append(f.calls, append([]string(nil), args...))
	if args[0] == "info" && f.mode == "unsupported" {
		return `{}`, nil
	}
	if args[0] == "image" && f.mode == "image" {
		return `[]`, nil
	}
	if args[0] == "inspect" && f.mode == "foreign" {
		return `[{"Id":"foreign"}]`, nil
	}
	if args[0] == "ps" && f.mode == "absence" {
		return "", fmt.Errorf("daemon unreachable")
	}
	if args[0] == "rm" && f.mode == "remove" {
		return "", fmt.Errorf("removal failed")
	}
	switch args[0] {
	case "info":
		return `{"OSType":"linux","CgroupVersion":"2","MemoryLimit":true,"SwapLimit":true,"PidsLimit":true,"CpuCfsQuota":true,"SecurityOptions":["name=seccomp,profile=builtin"],"Warnings":[]}`, nil
	case "image":
		return `[{"Id":"sha256:` + strings.Repeat("b", 64) + `","Os":"linux","Config":{"Volumes":null,"Env":null,"Entrypoint":null,"Cmd":null}}]`, nil
	case "create":
		if f.collision {
			return "", fmt.Errorf("name collision")
		}
		return fakeID + "\n", nil
	case "inspect":
		if f.inspectError {
			return "", fmt.Errorf("daemon unreachable")
		}
		started := "2026-01-01T00:00:00Z"
		if f.startup {
			started = "0001-01-01T00:00:00Z"
		}
		return fmt.Sprintf(`[{"Id":%q,"Image":%q,"Config":{"Labels":{"ci-trust.owner":%q}},"State":{"Status":"exited","StartedAt":%q,"ExitCode":7,"OOMKilled":%v,"Error":""}}]`, fakeID, owned().Image, f.owner(), started, f.oom), nil
	case "start":
		if f.started != nil {
			close(f.started)
		}
		if f.mode == "cancel" || f.mode == "timeout" {
			<-ctx.Done()
			return "", ctx.Err()
		}
		if f.mode == "overflow" {
			_, _ = io.WriteString(w, strings.Repeat("x", 100))
			return "", ctx.Err()
		}
		if f.mode == "attach" {
			return "", fmt.Errorf("exit status 125")
		}
		_, _ = io.WriteString(w, "stdout\nstderr\n")
		return "", nil
	case "rm":
		f.removed = true
		return fakeID, nil
	case "ps":
		if f.removed {
			return "", nil
		}
		return fakeID, nil
	}
	return "", fmt.Errorf("unexpected argv %q", args)
}
func (f *ownedClient) owner() string {
	for _, a := range f.calls {
		if a[0] == "create" {
			for i, v := range a {
				if v == "--label" {
					return strings.TrimPrefix(a[i+1], "ci-trust.owner=")
				}
			}
		}
	}
	return ""
}
func TestDockerExecute(t *testing.T) {
	for _, scenario := range []string{"success", "denied", "stale", "invalid", "snapshot", "collision", "inspect", "startup", "oom", "unsupported", "image", "foreign", "absence", "remove", "attach", "overflow", "timeout"} {
		t.Run(scenario, func(t *testing.T) {
			source := ownedTree(t)
			before := source()
			p := owned()
			p.Snapshot = before
			f := &ownedClient{collision: scenario == "collision", inspectError: scenario == "inspect", startup: scenario == "startup", oom: scenario == "oom"}
			f.mode = scenario
			if scenario == "timeout" {
				p.Limits.Wall = 20 * time.Millisecond
			}
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
			if scenario == "stale" {
				c.Digest = "stale"
			}
			if scenario == "invalid" {
				p.Tool = "unowned"
			}
			got := r.Execute(context.Background(), p, c)
			if scenario == "success" {
				if got.Outcome != Completed || got.ExitCode != 7 || got.Output != "stdout\nstderr\n" || !got.CleanupVerified {
					t.Fatalf("%+v calls=%v", got, f.calls)
				}
			} else {
				want := Unverifiable
				if scenario == "denied" || scenario == "stale" {
					want = Denied
				}
				if scenario == "invalid" {
					want = Invalid
				}
				if got.Outcome != want {
					t.Fatalf("outcome=%+v want=%s", got, want)
				}
			}
			counts := map[string]int{}
			for _, a := range f.calls {
				counts[a[0]]++
			}
			noCreate := scenario == "denied" || scenario == "stale" || scenario == "invalid" || scenario == "snapshot" || scenario == "unsupported" || scenario == "image"
			wantCreate := 1
			if noCreate {
				wantCreate = 0
			}
			if counts["create"] != wantCreate {
				t.Fatalf("create calls=%v", counts)
			}
			noStart := noCreate || scenario == "collision" || scenario == "inspect" || scenario == "foreign"
			wantStart := 1
			if noStart {
				wantStart = 0
			}
			if counts["start"] != wantStart || counts["rm"] != wantStart {
				t.Fatalf("process/cleanup calls=%v", counts)
			}
			if wantStart == 1 && scenario != "absence" && scenario != "remove" && !got.CleanupVerified {
				t.Fatalf("cleanup not verified: %+v", got)
			}
			if (scenario == "absence" || scenario == "remove") && got.CleanupVerified {
				t.Fatal("ambiguous cleanup accepted")
			}
			if (scenario == "denied" || scenario == "stale" || scenario == "invalid") && len(f.calls) != 0 {
				t.Fatal("denied plan started client")
			}
			for _, a := range f.calls {
				if scenario == "snapshot" && a[0] == "create" {
					t.Fatal("changed snapshot created container")
				}
				if a[0] == "rm" && !reflect.DeepEqual(a, []string{"rm", "--force", fakeID}) {
					t.Fatalf("unbound removal %q", a)
				}
			}
			if (scenario == "collision" || scenario == "inspect") && f.removed {
				t.Fatal("removed unbound container")
			}
			if source() != before {
				t.Fatal("source bytes/counter changed")
			}
		})
	}
}
