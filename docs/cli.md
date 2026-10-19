# Audit, report, bootstrap and gated lint CLI contract

`cmd/ci-trust` binds the offline audit producer to `internal/cli` and implements
the commands below. Local public-binary tests cover report file/stdin parity,
help/version, exact argument errors and unchanged source/counters on rejection.
Driven audit-fixture acceptance and [native three-host audit proof](portable-ci.md)
are recorded separately. That earlier receipt does not prove the later bootstrap
command; service tests and successful builds alone are not native evidence.

Build from source with `go build ./cmd/ci-trust`. The development version is
`dev`; `-ldflags "-X main.version=VALUE"` sets it explicitly. Initial terminal
checks use pinned `golang.org/x/term` on the actual stdin descriptor, not a
character-device guess. No raw terminal mode or shell process is used.

```
ci-trust audit PATH [--format human|json] [--platform linux|windows|macos ...]
ci-trust report [--input FILE|-] [--format human|json]
ci-trust bootstrap PATH --dry-run [--format human|json] [--platform linux|windows|macos ...]
Dry-run only: no apply behavior; conditional drafts are not executed or enforced.
ci-trust verify PATH --check lint [--format human|json] [--workflow-request -]
Owned ESLint 10.12.0 / Node 24.22.0: source-admitted owned fixture profile; ordinary host verification remains UNVERIFIABLE.
TOOL_FIXTURE only: CLEAN 0 -> FAULT 1 (no-debugger error at 1:1) -> RESTORED 0; exact human consent and preflight required; project configs/plugins are never loaded.
ci-trust --help
ci-trust --version
```

Options may precede or follow PATH. Each platform uses a separate `--platform`;
repeated platform values are rejected. Explicit values override declaration policy
through the injected audit callback. No implicit cwd audit exists. Report defaults
to stdin and human format, never rescans and needs no source repository.
`--help` or `help` alone after a subcommand shows its help. `help audit` and
`help report`, `help bootstrap` and `help verify` also work. Other mixtures of help/options are invalid.

## Gated lint verification

`verify PATH --check lint` reads bounded recognized source data and returns the
existing v1 report in human or JSON format. Both PATH and exactly one literal
`--check lint` are mandatory. No `--approve`, caller commands, plugins, input
reports or platform overrides are accepted. `--check` is not an audit/report/
bootstrap option. For option-looking PATH values, put options before `-- PATH`.

Ordinary host verification is **UNVERIFIABLE**, not sensitivity success:
source admission of the exact owned profile supplies no local execution services. Unsupported project
configuration/plugins are not loaded and yield an explicit diagnostic. A missing
genuine declaration anchor never creates a fictional CHECK. TOOL_FIXTURE output
cannot establish project efficacy, historical CI execution or enforcement.

A valid unresolved report exits 0; this is not execution approval. Real sensitivity
requires separately authenticated exact-plan consent/preflight and ordered CLEAN
exit0, FAULT exit1 with `no-debugger` error at1:1, RESTORED exit0 under an admitted
runtime. The normal invocation cannot run that sequence. Private simulated tests
and the internal launcher do not admit it. The optional request-data route below
still requires the fixed trusted context and exact authenticated review. See [integration limits](lint-offline-integration.md).

### Explicit protected workflow request

`--workflow-request -` selects the existing authenticated owned GitHub composition
and forwards stdin as a bounded canonical non-authorizing plan, not as a report or
approval. Only `-` is accepted; this option is invalid on audit/report/bootstrap.
No additional root/image/command/approval override or automatic environment-based
selection is introduced. The compiled gate precedes request/context/service access.

In this explicit mode, a validated just-produced owned sensitive report exits 0.
An insensitive or UNVERIFIABLE report is still emitted but exits 1; operational
errors exit 1 without report bytes; invalid arguments exit 2. Other commands and
normal `verify` keep their previous exit meanings and do not consume workflow data.
See [public route limits](lint-public-cli.md). Only the exact source-admitted
profile may proceed, and separate authenticated review remains mandatory.

## Human review summary

Human output with findings starts with a review summary grouped by `rule_id`
and exact explanation, sorted by those two values. Counts are **record counts,
not confirmed defects**: identical explanations across different scopes can share
a summary row without establishing a shared root cause. Different explanations
remain separate. Terminal controls and bidi characters are escaped.

Every original full section follows unchanged, including scoped finding IDs,
evidence, diagnostics and limitations. No evidence is merged and no UNKNOWN
certainty is upgraded. Reports without findings keep their previous human bytes.
JSON output, canonical ordering, schema, IDs, stderr and exit meanings are unchanged.
The summary is display-only and does not authorize execution or confirm defects.

## Pinned text and rendering

Root help is the invocation/help lines, bootstrap dry-run notice and verify gate/scope notice above followed by:

```
Audit/report/bootstrap: Offline/read-only: no execution, network, hidden storage, or writes.
Exit 0: valid report (including FAIL/UNKNOWN), help/version; 1: operational error; 2: invalid invocation.
Without --workflow-request: offline/read-only valid-report exit 0, including UNVERIFIABLE.
--workflow-request -: bounded non-authorizing request on stdin; owned protected GitHub workflow only, never host fallback.
Workflow exit 0: just-produced owned sensitivity; 1: unavailable, insensitive, unverifiable or operational error; 2: invalid invocation.
```

Subcommand help is its invocation line followed by those same two lines;
bootstrap additionally includes its dry-run/no-apply notice. Verify help uses its
gate/scope notice and the three workflow-specific lines above, not the ordinary
offline-effects suffix.
Version is `ci-trust VERSION\n` (Go-quoted VERSION, including quotes, to
escape untrusted control characters).

Human output is deterministic, lossless scoped data, not a green trust score:
`CI Trust report (configuration is not execution or enforcement)\n`, then
`schema_version: VALUE\n`, `snapshot: VALUE\n`, and these sections in order:
`inventory`, `capabilities`, `workflows`, `evidence`, `findings`, `diagnostics`,
`limitations`. Each section is `NAME:\n`, then one `  VALUE\n` per canonical
record, or `  (none)\n`. VALUE is compact canonical JSON. All strings in human
values escape Unicode control/format characters using JSON `\uXXXX` (including
terminal ESC, DEL, C1 and bidi controls). Every finding, scope, evidence reference,
unknown status and limitation remains visible, without inferring guarantees.

JSON stdout is exactly one evidence.Encode canonical schema-v1 document with
newline. Schema diagnostics remain report data and additionally produce
`CODE: MESSAGE\n` on stderr in canonical diagnostic order. Their code/message
control and format characters are escaped as `\uXXXX`; they never pollute stdout.
Rendering does not discover secrets: producers must apply evidence.Redact with
known sensitive values before injection. CLI applies existing admissibility and
canonical encoding, without changing the schema or input bytes.

## Errors and safety

All invocation failures have empty stdout and exactly
`ci-trust: invalid arguments\n` plus root help on stderr; exit 2. Arguments,
formats and platforms are validated before callback or report input access.
Duplicate singleton options, unknown options, extra paths and empty values fail.
Paths may contain spaces/Unicode, but option-looking paths must use `-- PATH`.
`--` ends audit/bootstrap/verify options; put `--dry-run` before it for bootstrap and `--check lint` before it for verify.

Operational failures have empty stdout, exit 1, and exactly one of:

```
ci-trust: audit service unavailable
ci-trust: bootstrap service unavailable
ci-trust: bootstrap failed
ci-trust: invalid bootstrap plan
ci-trust: audit failed
ci-trust: lint verification service unavailable
ci-trust: lint verification failed
ci-trust: report input required (interactive stdin)
ci-trust: cannot open report input
ci-trust: cannot read report input
ci-trust: report exceeds 8 MiB
ci-trust: invalid report
ci-trust: cannot write output
```

Report stdin/file input and injected audit/verify report bytes are bounded to 8 MiB. Empty,
malformed, duplicate-key, unsupported-version, schema-invalid and semantically
inadmissible data fail before output. Interactive stdin (including explicit `-`)
is refused without reading. Findings, FAIL and UNKNOWN are valid data, exit 0.
Output writers may themselves fail after accepting bytes; no stream API can
undo those bytes. No report bytes are written before full validation.

## Conditional bootstrap plan

The explicit target and `--dry-run` are mandatory; no apply behavior exists.
Platforms use audit's declaration/override semantics. A valid conditional plan,
conflict or no-proposal limitation is exit 0, not a ready-to-run guarantee.
Version `ci-trust-bootstrap-plan/v1` identifies a proposal artifact with `dry_run`,
existing validated `source_report`, `recommendations` and `limitations`. Every
recommendation contains `id`, relative `path`, `content`, `diff`, `finding_refs`,
`evidence_refs`, `preconditions`, `conflicts` and `limitations`. References resolve
inside the source report, not a replacement evidence schema. Unknown runtime and
dependencies stay explicit preconditions; no version/setup/install is guessed.

JSON is one deterministic plan document plus newline, not a report-v1 document.
Human output begins `CI Trust bootstrap plan (dry-run only; no apply)`, the line
`Conditional draft: not ready, executed, or enforced; review all preconditions and conflicts.`,
then the complete terminal-safe JSON plan and `Source uncertainty (configuration is not execution or enforcement):`
followed by the ordinary human source report. This retains every field and
uncertainty. Source diagnostics use the same exact stderr contract as audit.
Artifacts are fully validated before stdout, including version, dry-run status,
source report, relative destinations and reference existence; the input artifact
bound is 8 MiB. No source file is created or replaced, even on a conflict.
See [supported templates and action-pin policy](bootstrap.md).

## Execution remains unavailable

`verify PATH --check lint` is implemented as gated, read-only reporting. A valid
UNVERIFIABLE report exits 0; invalid arguments exit 2 and operational failures
exit 1. Real lint execution still requires separately authenticated target-bound
consent, an admitted runtime and pre-provisioned bounded isolation. There is no
unsafe yes flag or host-shell fallback. Bootstrap remains conditional dry-run
proposals, never apply.
Normal commands do not execute validators/hooks/scripts/package managers, use
network/credentials or write hidden reports. Explicit workflow request mode can
reach only the separately gated owned GitHub composition under the
[bounded admission disposition](lint-admission-policy.md). Source admission alone
cannot replace exact review or authorize a live runtime.
