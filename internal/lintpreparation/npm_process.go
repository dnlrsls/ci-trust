package lintpreparation

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"
)

const npmPhaseTimeout = 180 * time.Second

type npmProcessRunner func(context.Context, *exec.Cmd) error

// npmCommonFlags returns fresh metadata for the later lifecycle composer.
func npmCommonFlags(home string) []string {
	return []string{"--ignore-scripts", "--omit=dev", "--omit=optional", "--omit=peer", "--install-strategy=hoisted", "--no-bin-links", "--no-audit", "--no-fund", "--registry=https://registry.npmjs.org", "--noproxy=", "--cache=" + filepath.Join(home, "cache")}
}

// runNPMPhase executes only on the reference host platform. Callers own private
// absolute paths and must authenticate the Node bundle before reaching here.
// This primitive neither creates files nor reauthenticates executable identity.
func runNPMPhase(ctx context.Context, phase, nodeRoot, target, home string, port int) error {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return NewPreparationFailure("npm-setup", "validation", "")
	}
	return runNPMPhaseWith(ctx, phase, nodeRoot, target, home, port, func(_ context.Context, cmd *exec.Cmd) error {
		// CommandContext bounds the main process only, not a descendant process tree.
		return cmd.Run()
	})
}

// The private injection seam permits policy tests without executing any target.
func runNPMPhaseWith(ctx context.Context, phase, nodeRoot, target, home string, port int, run npmProcessRunner) error {
	fail := func(category string) error { return NewPreparationFailure(phase, category, "") }
	if phase != "npm-lock" && phase != "npm-ci" {
		return NewPreparationFailure("npm-setup", "validation", "")
	}
	if port < 1 || port > 65535 || !filepath.IsAbs(nodeRoot) || !filepath.IsAbs(target) || !filepath.IsAbs(home) || run == nil {
		return fail("validation")
	}
	ctx, cancel := context.WithTimeout(ctx, npmPhaseTimeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return fail(npmProcessCategory(err))
	}
	proxy := "http://127.0.0.1:" + strconv.Itoa(port)
	args := []string{filepath.Join(nodeRoot, "lib", "node_modules", "npm", "bin", "npm-cli.js")}
	if phase == "npm-lock" {
		args = append(args, "install", "--package-lock-only")
	} else {
		args = append(args, "ci")
	}
	args = append(args, npmCommonFlags(home)...)
	args = append(args, "--https-proxy="+proxy, "--proxy="+proxy)
	cmd := exec.CommandContext(ctx, filepath.Join(nodeRoot, "bin", "node"), args...)
	cmd.Dir = target
	// No os.Environ, PATH search, shell, inherited credentials or Node options.
	cmd.Env = []string{"HOME=" + home, "TMPDIR=" + home, "PATH=" + filepath.Join(nodeRoot, "bin"), "LANG=C", "LC_ALL=C", "NPM_CONFIG_USERCONFIG=" + filepath.Join(home, "user.npmrc"), "NPM_CONFIG_GLOBALCONFIG=" + filepath.Join(home, "global.npmrc"), "HTTP_PROXY=" + proxy, "HTTPS_PROXY=" + proxy, "ALL_PROXY=" + proxy, "NO_PROXY="}
	// os/exec maps nil streams directly to null devices. Unlike io.Discard,
	// nil output avoids copy pipes whose EOF could depend on descendants.
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := ctx.Err(); err != nil {
		return fail(npmProcessCategory(err))
	}
	err := run(ctx, cmd)
	if ctx.Err() != nil {
		return fail(npmProcessCategory(ctx.Err()))
	}
	if err != nil {
		return fail(npmProcessCategory(err))
	}
	return nil
}

func npmProcessCategory(err error) string {
	if errors.Is(err, context.Canceled) {
		return "network"
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return "process-failed"
	}
	return ErrorCategory(err)
}
