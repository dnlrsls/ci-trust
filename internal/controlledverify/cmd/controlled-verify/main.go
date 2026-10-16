// controlled-verify composes only the fixed owned smoke proof, never target code.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
	"github.com/dnlrsls/ci-trust/internal/controlledverify"
	"github.com/dnlrsls/ci-trust/internal/ownedimage"
)

const help = `controlled-verify: owned-smoke-v1 only; no approval arguments.
prepare --source DIR --archive FILE --binary FILE --marker FILE --revision GIT40 --run-id DECIMAL --run-attempt 1
execute --source DIR --archive FILE --binary FILE --marker FILE --input PLAN --binding HEX
prepare emits the complete plan without API or Docker access.
execute requires current GITHUB_SHA/GITHUB_RUN_ID/GITHUB_RUN_ATTEMPT and read-only GITHUB_TOKEN access to the real human review.
The owner must approve the protected Environment with comment: approve <binding_digest>.
Environment approval alone is insufficient. Only /usr/bin/docker and the local Unix socket are used.
Provision an empty private SOURCE/.git/controlled-proof/docker-config first.
The untagged immutable image is retained; containers are removed only by exact ID and owner.
RECORDED is not a security PASS. Review owned tool observations separately.
`

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, out, stderr io.Writer) int {
	fail := func(message string) int { fmt.Fprintln(stderr, "controlled-verify: "+message); return 2 }
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		if _, err := io.WriteString(out, help); err != nil {
			return 1
		}
		return 0
	}
	if len(args) == 0 || (args[0] != "prepare" && args[0] != "execute") {
		return fail("invalid arguments")
	}
	allowed := map[string]bool{"--source": true, "--archive": true, "--binary": true, "--marker": true}
	want := 13
	if args[0] == "prepare" {
		want = 15
		allowed["--revision"], allowed["--run-id"], allowed["--run-attempt"] = true, true, true
	} else {
		allowed["--input"], allowed["--binding"] = true, true
	}
	if len(args) != want {
		return fail("invalid arguments")
	}
	values := map[string]string{}
	for i := 1; i < len(args); i += 2 {
		key, value := args[i], args[i+1]
		if !allowed[key] || value == "" || values[key] != "" {
			return fail("invalid arguments")
		}
		values[key] = value
	}
	if ctx.Err() != nil {
		return fail("caller canceled")
	}
	if args[0] == "prepare" {
		p, err := controlledverify.PrepareReview(values["--source"], values["--archive"], values["--binary"], values["--marker"], values["--revision"], values["--run-id"], values["--run-attempt"], time.Now())
		if err != nil {
			return fail("review preparation unavailable")
		}
		if _, err := out.Write(p.JSON()); err != nil {
			return 1
		}
		return 0
	}
	// Pin the envelope to the current workflow context, not self-reported plan
	// fields or revision/run flags supplied by an autonomous caller.
	revision, runID, attempt := os.Getenv("GITHUB_SHA"), os.Getenv("GITHUB_RUN_ID"), os.Getenv("GITHUB_RUN_ATTEMPT")
	if revision == "" || runID == "" || attempt != "1" {
		return fail("current workflow context unavailable")
	}
	b, err := ownedimage.ReadFile(values["--input"], controlledplan.MaxInput)
	if err != nil {
		return fail("exact plan unavailable")
	}
	p, err := controlledplan.CheckProof(bytes.NewReader(b), values["--binding"], revision, runID, attempt, time.Now())
	if err != nil {
		return fail("exact current plan unavailable")
	}
	record := controlledverify.ExecuteProof(ctx, controlledverify.ProofInput{Plan: p, Source: values["--source"], Archive: values["--archive"], Binary: values["--binary"], Marker: values["--marker"]}, os.Getenv("GITHUB_TOKEN"))
	if err := json.NewEncoder(out).Encode(record); err != nil {
		return 1
	}
	if record.State != "RECORDED" {
		return 2
	}
	return 0
}
