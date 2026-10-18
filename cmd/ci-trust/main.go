// Command ci-trust reports configuration and gated lint without executing target code.
package main

import (
	"context"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/dnlrsls/ci-trust/internal/audit"
	"github.com/dnlrsls/ci-trust/internal/bootstrap"
	"github.com/dnlrsls/ci-trust/internal/cli"
	"github.com/dnlrsls/ci-trust/internal/lintverify"
	"golang.org/x/term"
)

// version can be set at build time; it never changes the evidence producer contract.
var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, cli.Services{
		Audit: func(path string, platforms []string) ([]byte, error) {
			return audit.Scan(path, audit.Options{Platforms: platforms})
		},
		Bootstrap: func(path string, platforms []string) (bootstrap.Plan, error) {
			return bootstrap.Scan(path, bootstrap.Options{Platforms: platforms})
		},
		Verify:           lintverify.Scan,
		VerifyWorkflow:   verifyWorkflow,
		Version:          version,
		InteractiveInput: term.IsTerminal(int(os.Stdin.Fd())),
	}))
}

func verifyWorkflow(path string, request io.Reader) ([]byte, bool, error) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	data, err := lintverify.ExecuteGitHub(ctx, path, request)
	return data, err == nil && lintverify.SensitiveReport(data), err
}
