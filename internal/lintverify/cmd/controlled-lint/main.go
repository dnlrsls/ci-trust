// controlled-lint is the fixed internal workflow control, never a target runner CLI.
package main

import (
	"context"
	"fmt"
	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	"github.com/dnlrsls/ci-trust/internal/lintverify"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const help = `controlled-lint: owned TOOL_FIXTURE lint only; no approval arguments.
Usage: controlled-lint prepare | verify
prepare emits a NON_AUTHORIZING_PLAN for the exact current workflow scope.
verify reads that bounded request on stdin; it is not imported execution proof.
The owner must approve the existing protected Environment with: approve lint <binding_digest>.
Compiled admission and exact authenticated review are required before delivery/load/capture.
Only fixed owned roles run; no target configuration, commands, image IDs or overrides.
Exit 0: prepared data, or verify produced owned sensitive evidence (not project/CI PASS).
Exit 1: held, unavailable, insensitive, unverifiable or operational failure; exit 2: invalid arguments.
The current Node hold denies both operations; --help never accesses services.
`

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
func run(ctx context.Context, args []string, input io.Reader, out, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		if _, e := io.WriteString(out, help); e != nil {
			return 1
		}
		return 0
	}
	if len(args) != 1 || (args[0] != "prepare" && args[0] != "verify") {
		fmt.Fprintln(stderr, "controlled-lint: invalid arguments")
		return 2
	}
	if reason := lintcatalog.BlockedReason(); reason != "" {
		fmt.Fprintln(stderr, "controlled-lint: "+reason)
		return 1
	}
	if ctx == nil || ctx.Err() != nil {
		fmt.Fprintln(stderr, "controlled-lint: caller unavailable")
		return 1
	}
	source := os.Getenv("GITHUB_WORKSPACE")
	var data []byte
	var e error
	if args[0] == "prepare" {
		data, e = lintverify.PrepareGitHubReview(source)
	} else {
		data, e = lintverify.ExecuteGitHub(ctx, source, input)
	}
	if e != nil {
		fmt.Fprintln(stderr, "controlled-lint: workflow unavailable")
		return 1
	}
	if n, e := out.Write(data); e != nil || n != len(data) {
		return 1
	}
	if args[0] == "verify" && !reportPassed(data) {
		return 1
	}
	return 0
}

// Interpret only our just-produced report for CI exit status, never as runtime
// admission/consent. There is no command accepting imported observations.
func reportPassed(data []byte) bool { return lintverify.SensitiveReport(data) }
