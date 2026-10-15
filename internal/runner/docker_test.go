package runner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
