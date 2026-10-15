// owned-smoke is internal tooling, not part of the public ci-trust CLI.
package main

import (
	"os"

	"github.com/dnlrsls/ci-trust/internal/ownedprobe"
)

func main() {
	os.Exit(ownedprobe.Run(os.Args[1:], ownedprobe.NewSystem(), os.Stdout, os.Stderr))
}
