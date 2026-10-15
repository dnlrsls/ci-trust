package runner

import (
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
)

type mutateClient struct {
	base              *ownedClient
	command, old, new string
}

func (f mutateClient) call(ctx context.Context, w io.Writer, args ...string) (string, error) {
	s, e := f.base.call(ctx, w, args...)
	if args[0] == f.command {
		s = strings.ReplaceAll(s, f.old, f.new)
	}
	return s, e
}
func TestDockerPreflightRefusals(t *testing.T) {
	tests := []struct{ name, command, old, new string }{
		{"cgroup", "info", `"2"`, `"1"`},
		{"memory", "info", `"MemoryLimit":true`, `"MemoryLimit":false`},
		{"swap", "info", `"SwapLimit":true`, `"SwapLimit":false`},
		{"pids", "info", `"PidsLimit":true`, `"PidsLimit":false`},
		{"cpu", "info", `"CpuCfsQuota":true`, `"CpuCfsQuota":false`},
		{"seccomp", "info", "name=seccomp,profile=builtin", "unknown"},
		{"warning", "info", `"Warnings":[]`, `"Warnings":["unsupported"]`},
		{"volumes", "image", `"Volumes":null`, `"Volumes":{"/data":{}}`},
		{"environment", "image", `"Env":null`, `"Env":["SECRET=owned"]`},
		{"healthcheck", "image", `"Cmd":null`, `"Cmd":null,"Healthcheck":{"Test":["CMD","unowned"]}`},
		{"identity", "image", strings.Repeat("b", 64), strings.Repeat("d", 64)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &ownedClient{}
			d := &LocalDocker{host: "linux", client: mutateClient{f, tt.command, tt.old, tt.new}}
			if err := d.Preflight(context.Background(), owned()); err == nil {
				t.Fatal("unsupported capabilities accepted")
			}
			for _, a := range f.calls {
				if a[0] == "create" {
					t.Fatal("preflight created container")
				}
			}
		})
	}
	d := &LocalDocker{host: "windows", client: &ownedClient{}}
	if err := d.Preflight(context.Background(), owned()); err == nil {
		t.Fatal(fmt.Errorf("unsupported host accepted"))
	}
}

func TestIntermediateLifecycleUnavailable(t *testing.T) {
	f := &ownedClient{}
	d := &LocalDocker{host: "linux", client: f}
	if err := d.Preflight(context.Background(), owned()); err != nil {
		t.Fatal(err)
	}
	before := len(f.calls)
	if _, err := d.Start(context.Background(), "ci-trust-123-1", dockerArgs(owned(), "ci-trust-123-1"), io.Discard); err == nil {
		t.Fatal("start available")
	}
	if err := d.Remove(context.Background(), "ci-trust-123-1"); err == nil {
		t.Fatal("removal available")
	}
	if len(f.calls) != before {
		t.Fatal("unavailable lifecycle reached client")
	}
}
