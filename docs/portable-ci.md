# Observed native audit/report validation

The owned public CLI tests passed on three native GitHub-hosted systems on
2026-10-15. This is execution evidence for **CI Trust's tests**, not for any
repository workflow being audited, required-check enforcement or container isolation.

## Source and receipt

- [Workflow run 38715049086](https://github.com/dnlrsls/ci-trust/actions/runs/38715049086),
  `workflow_dispatch`, attempt 1, completed successfully.
- Tested source: `cfe4cf2770479e9539b98fe510c4b5fc471e267c`.
- Tested tree: `1037f5fe94dccf54f3b229f3506d8520b7ed13c3`.
- Application merge: `14afb5ea9cc52c3877e6e773913a9319b7ebd4b9`, with exactly
  the same tree. PRs #24–#29 were integrated inside-out after current-head checks;
  the separate CI-only registration was PR #30.
- This documentation annotation changes no runtime code, workflow, fixture input,
  or JSON/human/stderr oracle. It does not claim that its own later commit was
  the revision executed by the run above.

| Native job | Observed Go toolchain | Result |
| --- | --- | --- |
| [Ubuntu](https://github.com/dnlrsls/ci-trust/actions/runs/38715049086/job/116355595423) | `go1.26.9 linux/amd64` | PASS |
| [Windows](https://github.com/dnlrsls/ci-trust/actions/runs/38715049086/job/116355595747) | `go1.26.8 windows/amd64` | PASS |
| [macOS](https://github.com/dnlrsls/ci-trust/actions/runs/38715049086/job/116355595648) | `go1.26.8 darwin/arm64` | PASS |

Each job successfully checked out source, set up Go, recorded `go version`, ran
`go test -count=1 -v ./...`, and ran `go vet ./...`. Verbose logs were checked for
both public test roots and all nine `TestPublicAdversarial` cases: A–F,
correct-config, malformed-workflow and omitted-job. Every case passed on each
host; no skipped tests or failures appeared. Different patch versions are recorded
rather than replaced with one assumed version. The workflow selects `1.26.x`.

## What the public tests establish

Tests build and invoke the owned CI Trust binary, not fixture validators. They
assert exact exits/stdout/stderr, canonical schema-valid JSON, independent scoped
semantics and decisive provenance, deterministic human/JSON output, unchanged
owned input bytes/file lists/counters after audits and rejected invocations, and
saved-file/stdin parity after the original source directory is detached.

Schema validity and captured bytes alone are not truth oracles: the
[independent semantic requirements](audit-fixtures.md) still govern acceptance.
The [local demo and self-audit](audit-demo.md) remain historical observations of
checkpoint `167d095`, not a substitute for this native receipt.

## Limits and operational notes

- No audited scripts, hooks, executable configs, package managers or actions were
  executed. Running CI Trust's own Go tests is a distinct authorized operation.
- Cross-builds and configured YAML did not stand in for these native executions.
- No live interactive-terminal exercise, remote enforcement collection, real
  container isolation or lint sensitivity is claimed here.
- Ubuntu's announced runner-image migration and macOS ARM64 queue-capacity
  annotations were informational, not failed checks. Future label/toolchain
  changes require fresh observed results, not extrapolation from this run.
- Initial PR/push admission produced no observed runs. Registering the reviewed
  workflow on the default branch enabled the documented manual-dispatch route;
  this does not establish the cause of those earlier missing events.
