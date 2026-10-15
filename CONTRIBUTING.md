# Contributing to CI Trust

Start with the [architecture](docs/architecture.md), [CLI contract](docs/cli.md)
and [observed audit/report demo](docs/audit-demo.md). The public executable now
binds static discovery, workflow analysis and rule evaluation to the evidence
contract. It does not execute the repository being audited.

## Build and local checks

Use Go 1.26 (module language version 1.26.0). The recorded demo used Go 1.26.4
on Windows/amd64; this does not establish Linux/macOS behavior. Dependencies are
pinned in go.mod/go.sum; first acquisition may require network, unlike audit/report.

```sh
go build -o ci-trust ./cmd/ci-trust
./ci-trust --help
./ci-trust --version
```

Use `ci-trust.exe` on Windows. See the [README](README.md) for formats, platforms
and exits. `audit PATH` requires an explicit path; `report --input FILE|-` validates
saved data without rescanning. Findings and UNKNOWN still return 0, operational
errors return 1, invalid invocation returns 2. JSON stdout is one document and
diagnostics use stderr. No execution, network or hidden storage is involved.

For source changes, run focused tests first, then applicable broader checks:

```sh
go test ./...
go vet ./...
gofmt -l cmd internal
```

These are contributor commands, **not commands executed for the documentation
demo**. No full suite, fixture validator or third-party pipeline was run for that
unit. See its exact observed scope rather than treating these examples as receipts.

Git attributes keep Go and contract/corpus fixture bytes at LF, including Windows
checkouts with core.autocrlf enabled. Preserve byte-exact canonical assertions;
do not normalize away output differences. Read the [schema contract](docs/evidence-contract.md)
and [evidence semantics](docs/evidence-semantics.md) before changing fixtures.
Schema and semantic admissibility do not independently prove evidence truth.

## Working conventions

- Keep technical documentation, identifiers and code comments in English.
- Scope work to an agreed issue, with evidence and limitations alongside behavior.
  Use Conventional Commit messages when commits are authorized.
- Keep packages explicit: `cmd/ci-trust` wires the binary; `internal/cli` handles
  I/O/rendering; `internal/audit` produces reports from discovery/workflow data.
  Preserve the existing evidence contract rather than inventing a parallel schema.
- Keep Core independent of agents, LLMs, terminal I/O and process control.
  YAML parsing and terminal detection dependencies are already pinned; new
  dependencies need concrete justification and distribution review.

## Verification expectations

For testable changes, observe a focused failing behavior test before implementation,
then run it and applicable checks. Passive documentation has no meaningful RED;
check links, examples, scope claims and whitespace instead.

Author fixture expectations independently before capturing analyzer output. Review
whole-report byte candidates against those expectations, never use analyzer output
as its own semantic oracle. Public binary checks must record exact exit/stdout/stderr,
report file/stdin parity, deterministic serialization, input hashes and sentinel
counters before/after, including rejected inputs. Keep output and helpers in owned
scratch space; do not execute fixture scripts or copy scratch reports into docs.
See the [corpus contract and action-pin policy](docs/audit-fixtures.md).
Action updates require official identity verification and review of immutable pins,
not silent moving-tag changes or additional unreviewed CI.

Configuration, observed execution, efficacy and enforcement are distinct. Unlinked
commands do not prove global non-execution. Generic TEST intent does not prove
unit/integration/E2E completeness; runner OS does not establish cross-build execution.
Record actual Linux/Windows/macOS run/job receipts for the tested revision, or mark
them unavailable. Library tests, mocks and cross-builds are not portable or isolation
proof. Full independent corpus acceptance and remote three-host evidence remain
issue #7 work; the local demo alone does not close it.

## Safety and future scope

Never run audited scripts, hooks, package managers or actions. Treat input as
untrusted and preserve traversal/parsing bounds. Do not fetch remote evidence or
add auto-fix/apply behavior. `verify` and `bootstrap` remain future commands.
Controlled verification has no production Docker backend/trusted image catalog;
no runtime provisioning, automatic consent or unsandboxed fallback is authorized.
A later Gentleman-Programming/engram study is opt-in and revision-pinned, uses only
generic adapters and makes no upstream writes. No such study has been run here.

No license is selected. Do not grant a license or claim licensed release/distribution
on the maintainer's behalf.
