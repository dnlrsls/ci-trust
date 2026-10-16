package lintpreparation

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Absolute synthetic paths only: these tests never create files or run commands.
func npmPaths() (string, string, string) {
	root := filepath.Join(string(filepath.Separator), "owned")
	if filepath.VolumeName(os.TempDir()) != "" {
		root = filepath.Join(filepath.VolumeName(os.TempDir())+string(filepath.Separator), "owned")
	}
	return filepath.Join(root, "node"), filepath.Join(root, "target"), filepath.Join(root, "home")
}
func expectedNPMFlags(home string) []string {
	return []string{"--ignore-scripts", "--omit=dev", "--omit=optional", "--omit=peer", "--install-strategy=hoisted", "--no-bin-links", "--no-audit", "--no-fund", "--registry=https://registry.npmjs.org", "--noproxy=", "--cache=" + filepath.Join(home, "cache")}
}
func TestNPMProcessPolicy(t *testing.T) {
	node, target, home := npmPaths()
	for _, phase := range []string{"npm-lock", "npm-ci"} {
		t.Run(phase, func(t *testing.T) {
			calls := 0
			err := runNPMPhaseWith(context.Background(), phase, node, target, home, 1234, func(ctx context.Context, cmd *exec.Cmd) error {
				calls++
				args := []string{filepath.Join(node, "bin", "node"), filepath.Join(node, "lib", "node_modules", "npm", "bin", "npm-cli.js")}
				if phase == "npm-lock" {
					args = append(args, "install", "--package-lock-only")
				} else {
					args = append(args, "ci")
				}
				args = append(args, expectedNPMFlags(home)...)
				args = append(args, "--https-proxy=http://127.0.0.1:1234", "--proxy=http://127.0.0.1:1234")
				if !reflect.DeepEqual(cmd.Args, args) || cmd.Path != args[0] || !filepath.IsAbs(cmd.Path) || cmd.Dir != target {
					t.Errorf("command policy: %#v", cmd)
				}
				want := []string{"HOME=" + home, "TMPDIR=" + home, "PATH=" + filepath.Join(node, "bin"), "LANG=C", "LC_ALL=C", "NPM_CONFIG_USERCONFIG=" + filepath.Join(home, "user.npmrc"), "NPM_CONFIG_GLOBALCONFIG=" + filepath.Join(home, "global.npmrc"), "HTTP_PROXY=http://127.0.0.1:1234", "HTTPS_PROXY=http://127.0.0.1:1234", "ALL_PROXY=http://127.0.0.1:1234", "NO_PROXY="}
				if !reflect.DeepEqual(cmd.Env, want) {
					t.Errorf("environment: %v", cmd.Env)
				}
				if cmd.Stdin != nil || cmd.Stdout != nil || cmd.Stderr != nil {
					t.Error("streams not configured for null devices")
				}
				deadline, ok := ctx.Deadline()
				remaining := time.Until(deadline)
				if !ok || remaining > 180*time.Second || remaining < 179*time.Second {
					t.Errorf("budget %v", remaining)
				}
				return nil
			})
			if err != nil || calls != 1 {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
	flags := npmCommonFlags(home)
	if !reflect.DeepEqual(flags, expectedNPMFlags(home)) {
		t.Fatal("common flags")
	}
	flags[0] = "mutated"
	if !reflect.DeepEqual(npmCommonFlags(home), expectedNPMFlags(home)) {
		t.Fatal("shared flags")
	}
}
func TestNPMProcessFailures(t *testing.T) {
	node, target, home := npmPaths()
	for _, phase := range []string{"npm-lock", "npm-ci"} {
		for _, tc := range []struct {
			name, category string
			err            error
		}{
			{"exit", "process-failed", &exec.ExitError{}},
			{"deadline", "timeout", context.DeadlineExceeded},
			{"cancel", "network", context.Canceled},
			{"path", "filesystem", &os.PathError{Op: "private-secret", Path: "private-secret", Err: os.ErrPermission}},
			{"other", "unexpected", errors.New("private-secret")},
		} {
			t.Run(phase+tc.name, func(t *testing.T) {
				calls := 0
				err := runNPMPhaseWith(context.Background(), phase, node, target, home, 1, func(context.Context, *exec.Cmd) error { calls++; return tc.err })
				if err == nil || err.Error() != phase+":"+tc.category || calls != 1 || strings.Contains(Diagnostic(err), "private-secret") {
					t.Fatalf("err=%v calls=%d", err, calls)
				}
			})
		}
	}
}
func TestNPMProcessContext(t *testing.T) {
	node, target, home := npmPaths()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	fake := func(context.Context, *exec.Cmd) error { calls++; return nil }
	if err := runNPMPhaseWith(ctx, "npm-ci", node, target, home, 65535, fake); err == nil || err.Error() != "npm-ci:network" || calls != 0 {
		t.Fatalf("precancel: %v %d", err, calls)
	}
	ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := runNPMPhaseWith(ctx, "npm-ci", node, target, home, 1, fake); err == nil || err.Error() != "npm-ci:timeout" || calls != 0 {
		t.Fatalf("expired: %v %d", err, calls)
	}
	parent, end := context.WithTimeout(context.Background(), time.Minute)
	defer end()
	want, _ := parent.Deadline()
	err := runNPMPhaseWith(parent, "npm-ci", node, target, home, 65535, func(ctx context.Context, _ *exec.Cmd) error {
		got, _ := ctx.Deadline()
		if !got.Equal(want) {
			t.Error("parent deadline extended")
		}
		end()
		return nil
	})
	if err == nil || err.Error() != "npm-ci:network" {
		t.Fatalf("returned context: %v", err)
	}
}
func TestNPMProcessRejectionAndState(t *testing.T) {
	node, target, home := npmPaths()
	before := os.Environ()
	for _, tc := range []struct {
		phase, node, target, home string
		port                      int
	}{
		{"private-secret", node, target, home, 1}, {"npm-ci", node, target, home, 0}, {"npm-ci", node, target, home, 65536},
		{"npm-ci", "relative", target, home, 1}, {"npm-ci", node, "relative", home, 1}, {"npm-ci", node, target, "relative", 1},
	} {
		calls := 0
		err := runNPMPhaseWith(context.Background(), tc.phase, tc.node, tc.target, tc.home, tc.port, func(context.Context, *exec.Cmd) error { calls++; return nil })
		if err == nil || calls != 0 || strings.Contains(err.Error(), "private-secret") {
			t.Fatalf("rejection: %v %d", err, calls)
		}
	}
	if !reflect.DeepEqual(before, os.Environ()) {
		t.Fatal("host environment changed")
	}
}
