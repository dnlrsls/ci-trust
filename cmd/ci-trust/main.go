// Command ci-trust audits local configuration without executing target code.
package main

import (
	"os"

	"github.com/dnlrsls/ci-trust/internal/audit"
	"github.com/dnlrsls/ci-trust/internal/cli"
	"golang.org/x/term"
)

// version can be set at build time; it never changes the evidence producer contract.
var version = "dev"

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, cli.Services{
		Audit: func(path string, platforms []string) ([]byte, error) {
			return audit.Scan(path, audit.Options{Platforms: platforms})
		},
		Version:          version,
		InteractiveInput: term.IsTerminal(int(os.Stdin.Fd())),
	}))
}
