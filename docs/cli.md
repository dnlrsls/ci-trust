# Audit and report CLI contract

`cmd/ci-trust` binds the offline audit producer to `internal/cli` and implements
the commands below. Local public-binary tests cover report file/stdin parity,
help/version, exact argument errors and unchanged source/counters on rejection.
Driven positive audit-fixture acceptance and three-host proof remain pending;
service tests and a successful build alone are not that evidence.

Build from source with `go build ./cmd/ci-trust`. The development version is
`dev`; `-ldflags "-X main.version=VALUE"` sets it explicitly. Initial terminal
checks use pinned `golang.org/x/term` on the actual stdin descriptor, not a
character-device guess. No raw terminal mode or shell process is used.

```
ci-trust audit PATH [--format human|json] [--platform linux|windows|macos ...]
ci-trust report [--input FILE|-] [--format human|json]
ci-trust --help
ci-trust --version
```

Options may precede or follow PATH. Each platform uses a separate `--platform`;
repeated platform values are rejected. Explicit values override declaration policy
through the injected audit callback. No implicit cwd audit exists. Report defaults
to stdin and human format, never rescans and needs no source repository.
`--help` or `help` alone after a subcommand shows its help. `help audit` and
`help report` also work. Other mixtures of help/options are invalid.

## Pinned text and rendering

Root help is the four invocation lines above followed by:

```
Offline/read-only: no execution, network, hidden storage, or writes.
Exit 0: valid report (including FAIL/UNKNOWN), help/version; 1: operational error; 2: invalid invocation.
```

Subcommand help is its invocation line followed by those same two lines.
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
`--` ends audit options only.

Operational failures have empty stdout, exit 1, and exactly one of:

```
ci-trust: audit service unavailable
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

## Future design, not implemented

`verify PATH --check lint` requires separately specified target-bound consent and
pre-provisioned bounded isolation; no unsafe yes flag or host-shell fallback.
`bootstrap PATH --dry-run` will produce evidence-backed reviewable proposals,
never apply changes. Both commands currently return invalid invocation (2).
No validators, hooks, scripts, package managers, actions, shell/eval, network,
credentials or hidden report storage are used by this layer.
