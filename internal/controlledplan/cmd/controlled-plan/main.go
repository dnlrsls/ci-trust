// controlled-plan emits preparation-only review data, never execution consent.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
)

const help = `controlled-plan: preparation-only; not execution consent.
prepare --snapshot SHA256 --image sha256:HEX --revision GIT40 --run-id DECIMAL --run-attempt DECIMAL
check --input FILE --binding HEX --revision GIT40 --run-id DECIMAL --run-attempt DECIMAL
Digests and CLI arguments are not human authentication. The protected workflow caller is the trust boundary.
`

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, stderr io.Writer) int {
	fail := func(message string) int { fmt.Fprintln(stderr, "controlled-plan: "+message); return 2 }
	helpArgs := args
	if len(args) == 2 && (args[0] == "prepare" || args[0] == "check") {
		helpArgs = args[1:]
	}
	if len(helpArgs) == 1 && (helpArgs[0] == "--help" || helpArgs[0] == "-h") {
		if _, err := fmt.Fprint(out, help); err != nil {
			fmt.Fprintln(stderr, "controlled-plan: output failed")
			return 1
		}
		return 0
	}
	if len(args) != 11 || (args[0] != "prepare" && args[0] != "check") {
		return fail("invalid arguments")
	}
	allowed := map[string]bool{"--revision": true, "--run-id": true, "--run-attempt": true}
	if args[0] == "prepare" {
		allowed["--snapshot"] = true
		allowed["--image"] = true
	} else {
		allowed["--input"] = true
		allowed["--binding"] = true
	}
	values := map[string]string{}
	for i := 1; i < len(args); i += 2 {
		key, value := args[i], args[i+1]
		if !allowed[key] || value == "" || values[key] != "" {
			return fail("invalid arguments")
		}
		values[key] = value
	}
	var e controlledplan.Envelope
	var err error
	if args[0] == "prepare" {
		e, err = controlledplan.Prepare(values["--snapshot"], values["--image"], values["--revision"], values["--run-id"], values["--run-attempt"])
	} else {
		// Only an explicitly named, bounded regular input file is read. No stdin,
		// device, pipe, subprocess, environment or runtime discovery is performed.
		var f *os.File
		info, statErr := os.Lstat(values["--input"])
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > controlledplan.MaxInput {
			return fail("validation failed")
		}
		f, err = os.Open(values["--input"])
		if err == nil {
			e, err = controlledplan.Check(f, values["--binding"], values["--revision"], values["--run-id"], values["--run-attempt"])
			closeErr := f.Close()
			if err == nil {
				err = closeErr
			}
		}
	}
	if err != nil {
		return fail("validation failed")
	}
	if _, err = out.Write(e.JSON()); err != nil {
		fmt.Fprintln(stderr, "controlled-plan: output failed")
		return 1
	}
	return 0
}
