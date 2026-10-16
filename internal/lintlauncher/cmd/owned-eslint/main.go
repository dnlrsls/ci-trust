// owned-eslint is internal owned-image tooling, not the public ci-trust CLI.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/dnlrsls/ci-trust/internal/lintlauncher"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := lintlauncher.Run(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}
