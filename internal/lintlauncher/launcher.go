// Package lintlauncher is an internal owned-image candidate, not a host fallback,
// runtime catalog, consent source or part of the public ci-trust CLI.
package lintlauncher

import (
	"context"
	"io"
	"os/exec"
	"runtime"
	"time"

	"github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

const help = `owned-eslint: internal candidate; no execution approval or runtime pins.
Usage: owned-eslint --role CLEAN|FAULT|RESTORED --binding DIGEST
Only fixed owned fixtures/configuration are used. No target input is accepted.
Execute only inside a separately approved, reviewed image. No host fallback.
`

type launcher struct {
	host        string
	command     func(context.Context, lintsensitivity.ESLintInvocation) *exec.Cmd
	wall, drain time.Duration
}

// Run is the internal image entrypoint. Success means a complete tool capture
// and receipt write, not sensitivity, isolation, cleanup or source preservation.
// Human consent and all image/source/plan checks belong to the external runner.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return (launcher{runtime.GOOS, fixedCommand, 10 * time.Second, 200 * time.Millisecond}).run(ctx, args, stdout, stderr)
}

func fixedCommand(ctx context.Context, invocation lintsensitivity.ESLintInvocation) *exec.Cmd {
	args := append([]string{"/owned/node_modules/eslint/bin/eslint.js"}, invocation.Args...)
	cmd := exec.CommandContext(ctx, "/owned/node/bin/node", args...)
	cmd.Dir = "/work"
	cmd.Env = []string{"LANG=C", "LC_ALL=C"}
	return cmd
}

func (l launcher) run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if stdout == nil || stderr == nil {
		return 1
	}
	if len(args) == 1 && args[0] == "--help" {
		if n, err := io.WriteString(stdout, help); err != nil || n != len(help) {
			_, _ = io.WriteString(stderr, "owned-eslint: output failed\n")
			return 1
		}
		return 0
	}
	if len(args) != 4 || args[0] != "--role" || args[2] != "--binding" {
		_, _ = io.WriteString(stderr, "owned-eslint: invalid arguments\n")
		return 2
	}
	role := lintsensitivity.Role(args[1])
	invocation, ok := lintsensitivity.OwnedESLintInvocation(role)
	_, err := lintsensitivity.EncodeESLintReceipt(role, args[3], lintsensitivity.ESLintCapture{Status: lintsensitivity.Completed})
	if !ok || err != nil {
		_, _ = io.WriteString(stderr, "owned-eslint: invalid arguments\n")
		return 2
	}
	fail := func() int { _, _ = io.WriteString(stderr, "owned-eslint: execution unavailable\n"); return 1 }
	if l.host != "linux" || ctx.Err() != nil {
		return fail()
	}
	ctx, cancel := context.WithTimeout(ctx, l.wall)
	defer cancel()
	tool, err := captureTool(ctx, l.command(ctx, invocation), invocation.Stdin, cancel, l.drain)
	if err != nil {
		return fail()
	}
	frame, err := lintsensitivity.EncodeESLintReceipt(role, args[3], tool)
	if err != nil || ctx.Err() != nil {
		return fail()
	}
	if n, err := stdout.Write(frame); err != nil || n != len(frame) {
		_, _ = io.WriteString(stderr, "owned-eslint: output failed\n")
		return 1
	}
	if ctx.Err() != nil {
		return fail()
	}
	return 0
}
