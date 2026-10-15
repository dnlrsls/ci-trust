# Observed local audit → JSON → report demo

The public binary produced a valid offline report on an owned copy of independent
fixture E, then rendered saved-file and stdin reports from an empty working
directory with no source repository. JSON bytes matched the audit output; human
file/stdin bytes matched. Inputs and the passive counter stayed unchanged.
This is one local demonstration, **not complete MVP, trusted CI or three-host proof**.

## Observed environment and scope

- Source checkpoint: `167d095`; documentation/task metadata may be locally modified.
- Windows/amd64, Go `go1.26.4`, Node `v24.14.0`; binary version `ci-trust "dev"`.
- Build used cached dependencies. No target Go/npm tests, scripts, executable
  configs, actions, containers, installs or network operations were invoked.
- Source fixture: `internal/audittest/testdata/e-incomplete-platform/repo`.
  Its independently authored sibling `expected.json` predates analyzer output;
  captured reports are observations, not replacement expectations or goldens.
- Owned scratch root: `.next/owned-audit-demo/`, excluded by the source reader.
  Scratch reports/helpers are local evidence, not distributable release artifacts.

## Reproduce the public path

From the worktree root, create `.next/owned-audit-demo/fixture` as a byte-preserving
copy of the fixture above, and add a passive `counter` containing `0` plus newline.
Record a relative file list and SHA-256 for every copied input before proceeding.
The fixture declares Linux/Windows/macOS but configures only a literal Linux
`go test` path. **Do not execute that command**: it is passive audit input.

```sh
go build -o .next/owned-audit-demo/ci-trust.exe ./cmd/ci-trust
.next/owned-audit-demo/ci-trust.exe --help
.next/owned-audit-demo/ci-trust.exe --version
.next/owned-audit-demo/ci-trust.exe audit .next/owned-audit-demo/fixture --format json > .next/owned-audit-demo/audit.stdout 2> .next/owned-audit-demo/audit.stderr
```

Use `ci-trust` rather than `ci-trust.exe` on Unix hosts; those hosts have not been
observed here. Check each exit separately; do not infer success from redirection.
For an explicitly source-independent rendering, create and enter an empty
`.next/owned-audit-demo/render-only/` directory, then run:

```sh
../ci-trust.exe report --input ../audit.stdout --format json > ../report-file-json.stdout 2> ../report-file-json.stderr
../ci-trust.exe report --format json < ../audit.stdout > ../report-stdin-json.stdout 2> ../report-stdin-json.stderr
../ci-trust.exe report --input ../audit.stdout > ../report-file-human.stdout 2> ../report-file-human.stderr
../ci-trust.exe report < ../audit.stdout > ../report-stdin-human.stdout 2> ../report-stdin-human.stderr
```

Report uses only the saved document, never rescans cwd or the source path.
Compare both JSON outputs byte-for-byte with `audit.stdout`, and the human
outputs with each other. Recompute the source list/hashes and read the counter.
For contract validation use the existing `internal/evidence.Validate` boundary
(structure **and** semantics), not a second report schema or the analyzer as oracle.

## Recorded outcome

The owned Node driver used `spawnSync` (20-second per-process bound) to capture
exact stdout/stderr and numeric exits as `<name>.stdout`, `<name>.stderr` and
`<name>.exit`. Every public invocation below returned **0**:

| Capture name | Observed stdout | Observed stderr |
| --- | --- | --- |
| `audit` | One schema-v1 JSON document | Empty |
| `report-file-json`, `report-stdin-json` | Exact audit JSON bytes | Empty |
| `report-file-human`, `report-stdin-human` | Identical human records | Empty |
| `help` | Implemented invocation/exit/safety help | Empty |
| `version` | `ci-trust "dev"` plus newline | Empty |
| `self-audit` | One schema-v1 JSON document | Five partial-analysis lines |

Fixture report: one inventory record, one workflow, four capabilities, six evidence
records, three findings, zero diagnostics, two limitations. The findings were two
`CIT_PLATFORM_CONFIGURED_GAP` records (Windows/macOS, declaration and workflow
provenance) and `CIT_REQUIRED_CHECKS_UNKNOWN`. These are configured generic TEST
gaps within the supplied literal push-event scope, not OS correctness failures,
actual failed runs, or global non-execution. Generic TEST is not complete E2E proof.

`before.json` and `after.json` matched for every input path/hash; `counter` remained
`0\n`. `summary.json` recorded parity and immutability. A scratch Go helper calling
the existing evidence contract accepted audit, both JSON report outputs and the
self-audit. JSON parsing plus that contract established one admissible document,
not the truth of every producer conclusion. No rejection tests or whole-corpus
oracles were authored in this documentation unit; those belong to the separate
public E2E work.

## Explicit source self-audit

From the worktree root (not the isolated fixture or empty render directory):

```sh
.next/owned-audit-demo/ci-trust.exe audit . --format json > .next/owned-audit-demo/self-audit.stdout 2> .next/owned-audit-demo/self-audit.stderr
```

Observed valid exit 0 with one inventory record, one workflow, four capabilities,
11 evidence records, seven findings, five diagnostics and three limitations.
Findings: four `CIT_DYNAMIC_PATH`, two `CIT_COMMAND_UNLINKED`, one
`CIT_REQUIRED_CHECKS_UNKNOWN`. No all-green conclusion follows.

The stderr codes were `snapshot-omissions`, `DISCOVERY_UNRESOLVED`,
`action-internals-unresolved`, `DISCOVERY_UNRESOLVED`,
`action-internals-unresolved`; each message said static analysis could not resolve
input and inferred no execution/enforcement. External action sources referenced
`.github/workflows/test.yml` lines 23 and 27. Action internals remain unresolved.
Nested owned fixture roots can contribute passive manifest declarations (including
unlinked npm scripts); they are not additional root workflow executions or product
coverage policy. Reader omissions and unresolved paths keep configured scope open;
platform absence cannot be concluded. Required checks, historical execution,
efficacy and actual enforcement remain UNKNOWN. The report snapshot does not
supply commit/repository identity; the checkpoint above was recorded separately.

## What remains unproved

- Full independent A–F/correct/omitted/malformed public acceptance, reviewed byte
  oracles, deterministic/rejection cases and immutability across that corpus.
- Actual Linux/Windows/macOS public-binary run/job receipts for the final tested
  revision. Prepared CI or local unit/service tests are not those receipts.
- Controlled verification: no production Docker backend/trusted image catalog or
  real isolation proof is available. `verify`/`bootstrap` remain future commands;
  no shell fallback, implicit consent, provisioning or apply behavior is permitted.
- A later opt-in, revision-pinned Gentleman-Programming/engram study must use generic
  adapters with no upstream writes. No network or case study was performed now.

See the [CLI contract](cli.md), [independent corpus and action-pin policy](audit-fixtures.md),
[discovery bounds](discovery.md) and [contribution guide](../CONTRIBUTING.md).
License remains undecided; no licensed release/distribution is claimed.
