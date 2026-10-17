// Command owned-lint-prepare creates preparation-only artifacts, not runtime admission.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/dnlrsls/ci-trust/internal/lintpreparation"
)

const help = `Usage: owned-lint-prepare [-h|--help]
No operational flags. No arguments starts effectful preparation on Linux/amd64.
Required environment: GITHUB_WORKSPACE, GITHUB_SHA, GITHUB_RUN_ID, GITHUB_RUN_ATTEMPT.
Requires protected pre-created inputs and launcher; writes seven preparation artifacts.
Output: GITHUB_WORKSPACE/.git/lint-image-preparation/artifacts (must not exist).
Downloads pinned Node/base data and runs fixed Node/npm dependency setup only.
No package scripts, ESLint, launcher, Docker or target execution; not runtime admission.
Use only in a separately authorized fresh protected hosted job.
Source/run metadata are recorded, not authenticated. Success is silent.
Failures use fixed stage/category and optional redacted reason identifiers.
`

type preparer func(context.Context, string, string, string, string) error

func main() { os.Exit(realMain()) }
func realMain() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop() // Runs before main calls os.Exit, including on preparation failure.
	return run(ctx, os.Args[1:], os.Stdout, os.Stderr, os.Getenv, lintpreparation.Prepare)
}

func writeText(w io.Writer, text string) error {
	n, e := io.WriteString(w, text)
	if e == nil && n != len(text) {
		return io.ErrShortWrite
	}
	return e
}

// Only this private command seam accepts fakes; the executable binds fixed APIs.
func run(ctx context.Context, args []string, out, stderr io.Writer, getenv func(string) string, prepare preparer) int {
	fail := func(code int, e error) int {
		e = lintpreparation.InStage("host-inputs", func() error { return e })
		if writeText(stderr, lintpreparation.Diagnostic(e)+"\n") != nil {
			return 1
		}
		return code
	}
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		if e := writeText(out, help); e != nil {
			return fail(1, e)
		}
		return 0
	}
	if len(args) != 0 {
		return fail(2, lintpreparation.NewValidationFailure(""))
	}
	var values [4]string
	for i, key := range []string{"GITHUB_WORKSPACE", "GITHUB_SHA", "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT"} {
		values[i] = getenv(key)
		if values[i] == "" {
			return fail(2, lintpreparation.NewValidationFailure(""))
		}
	}
	if e := prepare(ctx, values[0], values[1], values[2], values[3]); e != nil {
		return fail(1, e)
	}
	return 0
}
