# Offline workflow graph

`workflows.Analyze(snapshot discovery.Snapshot, inventory discovery.Result, options workflows.Options) workflows.Result`
consumes owned bytes and public inventory DTOs. It does not read files, invoke a
shell, load actions, fetch dependencies, or execute expressions. Its return value
is a typed intermediate graph, **not** a schema-v1 report.

## Public DTOs

- `Result`: `Workflows`, source-backed `Diagnostics`, `Limitations`, and
  `State`, `Historical`, `Enforcement`. Historical execution and enforcement are
  always `UNKNOWN`. A diagnostic makes the result incomplete/`UNKNOWN`; it does
  not erase independently parsed observations.
- `Workflow`: relative `Path`, `Name`, `Source`, `Events`, scoped `Settings`, `Jobs`.
- `Event`: `Name`, declaration `Source`, `Filters map[string]Value`, `State`.
  Event names include `pull_request`, `push`, and `merge_group` independently.
  Filters are preserved, not matched against imagined branches/changed files.
- `Job`: `ID`, `Name`, `Uses`, `Source`, `Needs`, `NeedSources`, `Condition`,
  `Runner`, `Settings`, `Matrix`, `Steps`, per-event `Eligibility`, configuration
  `State`, `Historical`, and `Enforcement`.
- `Step`: `ID`, `Name`, `Uses`, `Run`, `Source`, `RunValue`, `UsesValue`,
  `Condition`, `Shell`, `WorkingDirectory`, `ContinueOnError`, per-event
  `Eligibility`, `Links`, and `Historical`.
- `Value`: `Raw`, YAML `Tag`, `Source`, `Scope`, `State`, `Inherited`, recursive
  `Fields` and `Items`. Scalar `Raw` preserves the decoded expression verbatim;
  collection `Raw` is a flattened summary: use `Fields`/`Items` for structure.
  Missing/default settings have `UNKNOWN`, empty raw text, and zero line/column.
  An explicit declaration's `CONFIGURED` means recorded, not policy validation.
- `Matrix`: `State`, `Source`, `Combinations []map[string]string`, and parallel
  `Literals []map[string]Value` retaining YAML types and literal anchors. No
  matrix means one empty combination. An invalid/dynamic/over-limit matrix has
  `UNKNOWN` and **no** partially confirmed combinations.
- `Link`: exact `CandidateID`, inventory `Root`, resolved `Command`, `Chain`,
  `State`. Chain starts at the workflow run token, then the manifest's named
  definition and discovery's indirection sources. No execution is implied.
- `Diagnostic`: stable `Code`, `Message`, `Source`, `State=UNKNOWN`.
  `Source` aliases `discovery.Source` (relative path, 1-based line/column,
  optional script symbol); zero positions mean an admission/default location,
  not a fabricated YAML token.

## Bounded semantics

Only exact root `.github/workflows/<file>.yml` and `.yaml` documents are parsed.
YAML v3 nodes preserve `on` as a string key. Duplicate keys, anchors/aliases,
merge keys, and additional YAML documents are rejected.

Supported conditions are literal `true`/`false` and
`github.event_name == 'push'` / `!=` with `push`, `pull_request`, or `merge_group`.
Double quotes and optional `${{ ... }}` wrappers are accepted. Everything else
stays an expression and yields `UNKNOWN`; no interpreter is invoked.

`ELIGIBLE` means eligible within that static event context, not executed.
`STATICALLY_INELIGIBLE` is a known static exclusion, **not a historical skip**.
`CONDITIONAL` means event filters or successful completion of an eligible needs
job still constrain the path. Missing dependencies and cycles are `UNKNOWN`.
Unsupported conditions remain unknown even if they resemble scheduler functions.
Needs eligibility is conservative: no failure-result/success-function emulation.

Literal matrix axes use Cartesian expansion, partial-match exclusion, then
include augmentation of compatible original combinations. Include-only entries
are supported; include-added entries are never reused as original combinations.
Scalar strings, finite numbers and booleans are supported, with typed matching;
objects, nulls, dynamic expressions and invalid shapes stay unknown. Numeric
matching uses finite IEEE-754 values. There is no cross-host coverage inference:
consumers must combine runner expressions and matrix literals themselves.

Workflow/job settings preserve permissions, timeout-minutes, concurrency,
cancel-in-progress, continue-on-error and strategy fail-fast. Only explicit
workflow permissions are inherited by jobs; remote defaults remain unknown.
Defaults for step shell and working directory follow job then workflow
`defaults.run`, with declaration sources. Continue-on-error denotes potentially
non-blocking configuration, never required-check enforcement. Cancellation and
fail-fast are configuration, not evidence of historical cancellation.

Only exact single-line `npm test` and `npm run NAME` commands are linked. They
must match both an inventory root's working directory and that root's
`package.json` script symbol, with a `CONFIGURED` discovery candidate. Chains,
flags, expressions, missing definitions and unresolved/cyclic discovery
candidates do not gain links. Unreferenced inventory entries remain unlinked.
No inference is made from job names or action names. Explicit shells other than
`sh`/`bash` receive a limitation and no links. An omitted shell permits only
lexical npm linkage, not a claim about the runner's actual shell.

External/local composite action internals, reusable workflows, environment
expansion, arbitrary shell syntax and full GitHub scheduler semantics are
explicitly unsupported. Required checks, branch protection/rulesets and actual
execution are always unknown without separate remote/run evidence.

## Resource admission

Zero-valued `Options` select defaults; negatives and values above hard caps are
rejected. Limits apply at direct `Analyze`, not only discovery.

| Option | Default | Hard cap |
| --- | ---: | ---: |
| MaxDocuments | 64 | 256 |
| MaxBytes (total input) | 1 MiB | 8 MiB |
| MaxNodes (total decoded nodes) | 32768 | 131072 |
| MaxDepth | 32 | 64 |
| MaxJobs (total) | 256 | 1024 |
| MaxSteps (total) | 2048 | 8192 |
| MaxMatrix (per job) | 256 | 4096 |

Additionally: 64 event declarations/workflow, 32 axes/entry keys, 65536 total
expanded matrix cells, 4096 diagnostics/links, and bounded inventory cardinality
and command bytes. A conservative pre-decode lexical gate counts punctuation,
flow nesting and indentation, including quoted/comment text. It may reject a
valid unusually dense document as unknown rather than risk expensive parsing.
Malformed YAML and limit failures never become coverage evidence. Job/step
limit omissions are explicit diagnostics; consumers must not interpret omitted
configuration as definite absence.

## Verification

`go test -count=1 ./internal/workflows` tests the exported boundary with owned
in-memory snapshots and supplied discovery inventory. These are static parser
checks, not execution evidence or portable GitHub-host results.
