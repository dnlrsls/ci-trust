# Audit, report and conditional bootstrap CLI contract

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
ci-trust --help
ci-trust --version
```

Options may precede or follow PATH. Each platform uses a separate `--platform`;
repeated platform values are rejected. Explicit values override declaration policy
through the injected audit callback. No implicit cwd audit exists. Report defaults
to stdin and human format, never rescans and needs no source repository.
`--help` or `help` alone after a subcommand shows its help. `help audit` and
`help report` and `help bootstrap` also work. Other mixtures of help/options are invalid.

## Pinned text and rendering

Root help is the invocation/help lines and bootstrap dry-run notice above followed by:

```
Offline/read-only: no execution, network, hidden storage, or writes.
Exit 0: valid report (including FAIL/UNKNOWN), help/version; 1: operational error; 2: invalid invocation.
```

Subcommand help is its invocation line followed by those same two lines;
bootstrap additionally includes its dry-run/no-apply notice.
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
`--` ends audit/bootstrap options; put `--dry-run` before it for bootstrap.

Operational failures have empty stdout, exit 1, and exactly one of:

```
ci-trust: audit service unavailable
ci-trust: bootstrap service unavailable
ci-trust: bootstrap failed
ci-trust: invalid bootstrap plan
ci-trust: audit failed
ci-trust: report input required (interactive stdin)
ci-trust: cannot open report input
ci-trust: cannot read report input
ci-trust: report exceeds 8 MiB
ci-trust: invalid report
ci-trust: cannot write output
```

Report stdin/file input and injected audit bytes are bounded to 8 MiB. Empty,
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

## Future design, not implemented

`verify PATH --check lint` requires separately specified target-bound consent and
pre-provisioned bounded isolation; no unsafe yes flag or host-shell fallback.
It currently returns invalid invocation (2). Bootstrap is implemented only as
conditional dry-run proposals, never apply.
No validators, hooks, scripts, package managers, actions, shell/eval, network,
credentials or hidden report storage are used by this layer.
