package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestDockerActiveCancellation(t *testing.T) {
	source := ownedTree(t)
	before := source()
	p := owned()
	p.Snapshot = before
	f := &ownedClient{mode: "cancel", started: make(chan struct{})}
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
	case <-time.After(time.Second):
		t.Fatal("owned start not observed")
	}
	cancel()
	select {
	case got := <-done:
		if got.Outcome != Unverifiable || !got.CleanupVerified || !f.removed {
			t.Fatalf("%+v calls=%v", got, f.calls)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation cleanup did not return")
	}
	if source() != before {
		t.Fatal("source tree changed")
	}
}
func TestDockerExactVectorAndOneShot(t *testing.T) {
	f := &ownedClient{}
	d := &LocalDocker{client: f, host: "linux"}
	p := owned()
	ctx := context.Background()
	if err := d.Preflight(ctx, p); err != nil {
		t.Fatal(err)
	}
	name := "ci-trust-123-1"
	args := dockerArgs(p, name)
	modified := append([]string(nil), args...)
	modified = append(modified, "--privileged")
	before := len(f.calls)
	if _, err := d.Start(ctx, name, modified, io.Discard); err == nil || len(f.calls) != before {
		t.Fatal("unowned vector reached client")
	}
	if _, err := d.Start(ctx, name, args, io.Discard); err != nil {
		t.Fatal(err)
	}
	var create []string
	for _, a := range f.calls {
		if a[0] == "create" {
			create = a
		}
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(f.owner()) {
		t.Fatalf("invalid ownership token %q", f.owner())
	}
	// Independent complete oracle: do not derive expected policy from dockerArgs,
	// Plan fields or production vector builders.
	expected := []string{"create", "--log-driver=none", "--ipc=none", "--label", "ci-trust.owner=" + f.owner(),
		"--name", "ci-trust-123-1", "--pull=never", "--network=none", "--read-only", "--user=65534:65534", "--cap-drop=ALL", "--security-opt=no-new-privileges",
		"--memory", "33554432", "--memory-swap", "33554432", "--cpus", "1", "--pids-limit", "8", "--tmpfs", "/tmp:rw,noexec,nosuid,nodev,size=1048576",
		"--entrypoint", "/ci-trust/owned-smoke", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "--fixture=smoke-v1", "--config=owned-v1"}
	if !reflect.DeepEqual(create, expected) {
		t.Fatalf("create vector=%q want=%q", create, expected)
	}
	for _, flag := range []string{"--pull=never", "--network=none", "--read-only", "--user=65534:65534", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--memory", "--memory-swap", "--cpus", "--pids-limit", "--tmpfs"} {
		if !strings.Contains(strings.Join(create, " "), flag) {
			t.Fatal(flag)
		}
	}
	before = len(f.calls)
	if _, err := d.Start(ctx, name, args, io.Discard); err == nil || len(f.calls) != before {
		t.Fatal("second start allowed")
	}
	if err := d.Preflight(ctx, p); err == nil {
		t.Fatal("used runtime reset")
	}
	if err := d.Remove(ctx, "foreign"); err == nil || len(f.calls) != before {
		t.Fatal("foreign cleanup reached client")
	}
	if err := d.Remove(ctx, name); err != nil {
		t.Fatal(err)
	}
	expectedLifecycle := [][]string{
		{"info", "--format", "{{json .}}"},
		{"image", "inspect", "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		expected,
		{"inspect", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
		{"start", "--attach", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
		{"inspect", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
		{"inspect", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
		{"rm", "--force", "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"},
		{"ps", "--all", "--no-trunc", "--filter", "id=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc", "--format", "{{.ID}}"},
	}
	if !reflect.DeepEqual(f.calls, expectedLifecycle) {
		t.Fatalf("lifecycle=%q want=%q", f.calls, expectedLifecycle)
	}
}

func TestDockerIncompleteState(t *testing.T) {
	for _, field := range []string{`"ExitCode":7,`, `"OOMKilled":false,`, `"Error":""`} {
		t.Run(field, func(t *testing.T) {
			replacement := ""
			if field == `"Error":""` {
				replacement = `"Ignored":""`
			}
			f := &ownedClient{}
			d := &LocalDocker{host: "linux", client: mutateClient{f, "inspect", field, replacement}}
			p := owned()
			r, err := NewOwned(d, p, func(context.Context) (string, error) { return p.Snapshot, nil })
			if err != nil {
				t.Fatal(err)
			}
			got := r.Execute(context.Background(), p, Consent{p.Digest(), true})
			if got.Outcome != Unverifiable {
				t.Fatalf("missing state field accepted: %+v", got)
			}
		})
	}
}

type exitClient struct {
	base dockerClient
	exit error
}

func (f exitClient) call(ctx context.Context, w io.Writer, args ...string) (string, error) {
	s, e := f.base.call(ctx, w, args...)
	if args[0] == "start" {
		return s, f.exit
	}
	return s, e
}
func TestObservedToolExit(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := &executableDocker{executable, t.TempDir()}
	_, exit := c.call(context.Background(), nil, "owned-tool-exit")
	if exit == nil {
		t.Fatal("fake tool exit not observed")
	}
	source := ownedTree(t)
	before := source()
	p := owned()
	p.Snapshot = before
	d := &LocalDocker{host: "linux", client: exitClient{&ownedClient{}, exit}}
	r, err := NewOwned(d, p, func(context.Context) (string, error) { return source(), nil })
	if err != nil {
		t.Fatal(err)
	}
	got := r.Execute(context.Background(), p, Consent{p.Digest(), true})
	if got.Outcome != Unverifiable || got.ExitCode != 7 || !got.CleanupVerified || got.Output != "stdout\nstderr\n" {
		t.Fatalf("%+v", got)
	}
	if source() != before {
		t.Fatal("source tree changed")
	}
}

func TestMatchingExitDoesNotProveTransport(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	client := &executableDocker{executable, t.TempDir()}
	diagnostic, exit := client.call(context.Background(), nil, "owned-transport-error")
	var genuine *exec.ExitError
	if !errors.As(exit, &genuine) || genuine.ExitCode() != 1 || diagnostic != "owned attach transport interrupted\n" {
		t.Fatalf("not a genuine transport fixture: %q %v", diagnostic, exit)
	}
	source := ownedTree(t)
	before := source()
	p := owned()
	p.Snapshot = before
	base := mutateClient{&ownedClient{}, "inspect", `"ExitCode":7`, `"ExitCode":1`}
	d := &LocalDocker{host: "linux", client: exitClient{base, exit}}
	r, err := NewOwned(d, p, func(context.Context) (string, error) { return source(), nil })
	if err != nil {
		t.Fatal(err)
	}
	got := r.Execute(context.Background(), p, Consent{p.Digest(), true})
	if got.Outcome != Unverifiable || got.ExitCode != 1 || !got.CleanupVerified || got.Output != "stdout\nstderr\n" {
		t.Fatalf("ambiguous transport promoted: %+v", got)
	}
	if source() != before {
		t.Fatal("source tree changed")
	}
}

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
