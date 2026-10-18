package lintverify

import (
	"fmt"
	"strconv"
	"strings"

	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
	"github.com/dnlrsls/ci-trust/internal/runner"
)

// Diagnostic data cannot change acceptance or carry runtime output into reports.
// The combined receipt/output stage deliberately does not guess a parser cause.
type harnessDetail struct {
	facts           *runner.ExecutionFacts
	cleanupVerified bool
	role            ls.Role
	stage           string
	status          ls.Status
	outerExit       int
	runtimeCode     string
}

func failedHarness(role ls.Role, result captureResult) harnessDetail {
	c := result.capture
	stage := "RECEIPT_OR_ESLINT_OUTPUT"
	switch {
	case c.Status != ls.Completed:
		stage = "OUTER_CAPTURE"
	case c.ExitCode != 0:
		stage = "OUTER_EXIT"
	case len(c.Stderr) != 0:
		stage = "OUTER_STDERR"
	}
	return harnessDetail{role: role, stage: stage, status: c.Status, outerExit: c.ExitCode, runtimeCode: result.runtimeCode, facts: result.facts, cleanupVerified: result.cleanupVerified}
}

func runtimeFailureCode(reason string) string {
	switch reason {
	case "lint launcher completion unverified":
		return "LAUNCHER_EXIT"
	case "output limit exceeded":
		return "OUTPUT_LIMIT"
	case "execution canceled or walltime exceeded":
		return "WALLTIME_OR_CANCEL"
	}
	if strings.HasPrefix(reason, "harness: ") {
		return "TRANSPORT_ERROR"
	}
	if strings.HasPrefix(reason, "preflight: ") {
		return "RUNTIME_PREFLIGHT"
	}
	return "UNSPECIFIED"
}

func (d harnessDetail) text() string {
	switch d.role {
	case ls.Clean, ls.Fault, ls.Restored:
	default:
		return ""
	}
	if d.facts != nil {
		return d.factText()
	}
	switch d.stage {
	case "OUTER_CAPTURE", "OUTER_EXIT", "OUTER_STDERR", "RECEIPT_OR_ESLINT_OUTPUT":
	default:
		return ""
	}
	status := "UNKNOWN"
	switch d.status {
	case ls.Completed, ls.Denied, ls.Unavailable, ls.TimedOut, ls.Crashed, ls.CleanupUnverified, ls.InvalidOutput:
		status = string(d.status)
	}
	code := "UNSPECIFIED"
	switch d.runtimeCode {
	case "LAUNCHER_EXIT", "OUTPUT_LIMIT", "WALLTIME_OR_CANCEL", "TRANSPORT_ERROR", "RUNTIME_PREFLIGHT":
		code = d.runtimeCode
	}
	exit := "UNAVAILABLE"
	if d.outerExit >= -1 && d.outerExit <= 255 {
		exit = strconv.Itoa(d.outerExit)
	}
	return fmt.Sprintf(" Harness detail: role=%s; stage=%s; capture=%s; outer_exit=%s; runtime=%s. This metadata does not establish Node execution.", d.role, d.stage, status, exit, code)
}

func (d harnessDetail) factText() string {
	primary := "UNSPECIFIED"
	switch d.facts.Primary {
	case "":
		primary = "NONE"
	case "PLAN_INVALID", "CONSENT_INVALID", "CONTEXT_ENDED", "TRANSPORT_UNAVAILABLE", "PREFLIGHT_FAILED", "SOURCE_CHANGED", "OUTPUT_LIMIT", "WALLTIME_OR_CANCEL", "START_FAILED", "LAUNCHER_EXIT", "CAPTURE_GUARD", "RUNTIME_CREATION", "ROLE_PLAN_OR_REVIEW", "RUNNER_SETUP":
		primary = d.facts.Primary
	}
	cleanup := "NOT_ATTEMPTED"
	if d.facts.CleanupAttempted {
		cleanup = "REMOVE_UNVERIFIED"
		if d.cleanupVerified {
			cleanup = "VERIFIED"
		}
	}
	return fmt.Sprintf(" Execution facts: role=%s; start_requested=%t; primary=%s; cleanup_attempted=%t; cleanup=%s. This metadata does not establish Node execution.", d.role, d.facts.StartRequested, primary, d.facts.CleanupAttempted, cleanup)
}
