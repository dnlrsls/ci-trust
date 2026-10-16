# Lint runtime preparation and integration plan (#9)

Status: **design candidate, not an admitted runtime or execution approval**.
Reviewed against public upstream sources on 2026-10-15. No dependency resolution,
installation, image build/load, Node/ESLint execution, workflow dispatch or remote
session is performed by this document. #9 remains open.

## Candidate versions and unresolved pins

- ESLint **10.12.0**, released on 2026-10-02, is the candidate tool release.
  Its [tagged package manifest](https://raw.githubusercontent.com/eslint/eslint/v10.12.0/package.json)
  declares Node `^20.19.0 || ^22.13.0 || >=24`. This is a declared compatibility
  range, not an observed compatibility result for our adapter.
- Node **24.22.0**, the [2026-10-14 LTS release](https://nodejs.org/en/blog/release/v24.22.0),
  is a compatibility baseline only, **not approved for runtime admission**.
  The [upcoming security release notice](https://nodejs.org/en/blog/vulnerability/october-2026-security-releases)
  includes the 24.x line and a highest severity of HIGH. Reassess the notice and
  patched release before admitting a concrete runtime; do not silently replace a
  version or claim the announced patch already exists.
- Dependency closure, package-manager version, platform-specific Node archive
  checksum, base filesystem/shared-library closure, image config ID, layer diff
  IDs and complete image archive digest are **not yet selected or captured**.
  No placeholder digest is an admissible pin.

Prefer an LTS runtime over Current to reduce runtime churn. Prefer the maintained
ESLint major over the synthetic `9.0.0` identity used in unit tests. Neither choice
establishes security, output compatibility or a supported catalog entry.
Release reference: [ESLint 10.12.0](https://eslint.org/blog/2026/10/eslint-v10.12.0-released/).

## Reproducible preparation gate

A separately authorized preparation environment must produce reviewable inputs:

1. An owned package manifest with exact ESLint version and no project plugins,
   imported shareable configuration, custom parser or lifecycle scripts.
2. A lockfile with the complete production dependency graph, exact resolved
   package versions, origins and integrity values. An exact top-level version
   alone is insufficient: upstream declares ranged transitive dependencies.
   Pin the package-manager version and preparation flags; disable lifecycle
   scripts and exclude optional peers not required by the owned JS config.
3. Verified Node distribution bytes and the entire runtime filesystem closure,
   including dynamic libraries. The #8 static-ELF-only archive inspector cannot
   attest a Node/npm filesystem and must not be reused as if it could.
4. An independently inspected immutable Linux/amd64 image/archive inventory,
   owned config and fixture hashes, permissions, runtime entrypoint, environment
   and absence of unexpected execution defaults. No mutable image tag or runtime
   package-manager invocation is execution authority.
5. A compiled, reviewed catalog binding all those identities to the effective
   config, fixed arguments, parser revision and owned fixture triplet. Changed
   bytes invalidate previous plans and consent.

No download/build command is supplied as an implicit execution authorization.
Preparation provenance, integrity checking and runtime approval are separate.

## Existing seams that require deliberate changes

| Current boundary | Required #9 change |
| --- | --- |
| `runner.valid` admits only `owned-smoke-v1`, with legacy or opt-in split transport | Add a separate reviewed lint profile; retain exact rejection of arbitrary adapters and commands |
| `dockerArgs` fixes `/ci-trust/owned-smoke` and smoke arguments | Bind a fixed lint launch vector and role, never caller paths/configuration |
| Legacy `Runtime.Start` merges streams; private captured mode separates them | Bind the lint profile to captured mode; the strict parser requires empty stderr and JSON stdout |
| Captured smoke receives only fixed empty stdin | Bind role-specific owned fixture bytes to a reviewed lint profile; do not accept caller input or inherit host stdin |
| `ownedimage.Inspect` admits exactly the static probe and marker | Introduce an independent lint filesystem inspection contract without weakening smoke inspection |
| `controlledverify` binds one smoke catalog and four proof cases | Bind a separate lint triplet and exact human review; smoke consent cannot authorize lint |

Keep #8's working smoke profile, limits, image checks and tests intact. Do not
generalize the runner into an arbitrary command executor to satisfy this plan.
The source-only container transport uses direct stdin through a fixed interactive,
non-TTY attach operation. It avoids a launcher for the smoke transport. A future
lint launcher may still be needed to distinguish tool exit from client transport
failure; adding one introduces executable trusted code and a receipt protocol to
review, not an automatic extension of smoke admission.

## Compatibility and acceptance gates

The tagged [JSON formatter](https://raw.githubusercontent.com/eslint/eslint/v10.12.0/lib/cli-engine/formatters/json.js)
serializes supplied results; it does not define or authenticate their complete
shape. The tagged [no-debugger implementation](https://raw.githubusercontent.com/eslint/eslint/v10.12.0/lib/rules/no-debugger.js)
reports a debugger statement. Static inspection does not replace captured tests.
Retain the strict parser until real captures establish which fields are needed;
do not broaden it merely to turn an unknown result into success.

Before public execution admission:

- Review Node security status and complete every unresolved identity above.
- Implement and test authenticated exact-plan consent, stale/denied no-start,
  supported Linux preflight and rejection of target executable configuration.
- Test independent stream bounds, cancellation, timeout, crash, missing tool,
  wrong diagnostic, no-op validator and unverified cleanup. Harness failure is
  not a lint finding; UNVERIFIABLE is not efficacy PRESENT or ABSENT.
- Bind CLEAN, FAULT and RESTORED in order to one source snapshot and tool/runtime
  identity, with source remeasurement, exact container cleanup and bounded output.
- Add public `verify PATH --check lint`, help and existing schema-v1 report mapping.
  No new evidence schema or host-shell fallback is required.
- Under a separately authorized concrete isolated plan, capture clean exit 0,
  fault exit 1 with exactly one `no-debugger` error at 1:1, and restored exit 0.
  Feed actual captures through the parser and classifier; preserve their binding
  and TOOL_FIXTURE scope. This does not verify repository config or CI enforcement.

Local fake/unit tests remain necessary but cannot close the real execution gate.
Publish and review the implementation and receipts before maintainer closure.

## Implemented source-only transport

The private process layer now has a split capture seam with immutable stdin
limited to 64 KiB, a shared stdout/stderr retention budget of at most 64 KiB,
cancellation, a maximum one-minute wall bound and the existing 200 ms pipe wait.
Owned test executables exercise separate streams, exact/over-budget output,
input bounds, timeout, caller cancellation and failed processes. Existing smoke
calls reuse the same process setup and retain their merged-output contract.

`Plan.Transport` now permits the opt-in `split-stdin-v1` mode for the existing
`owned-smoke-v1` / `owned-v1` profile, with output at most 64 KiB and **fixed empty
stdin only**. Default transport omits the field, preserving original plan JSON
and digests. Selecting split mode changes the digest: old consent is rejected
before runtime access. This is not an ESLint catalog entry. Existing public
preparation envelopes continue to reject transport changes.

`Runner.Execute` selects the private captured capability only for that bound
mode. Container creation uses `--interactive` but no TTY; inspected `OpenStdin`
must be true and `Tty` false before attachment. The fixed client command is
`start --attach --interactive <inspected ID>`, with explicit owned input and no
inherited stdin. Sources: Docker's [create](https://docs.docker.com/reference/cli/docker/container/create/)
and [start](https://docs.docker.com/reference/cli/docker/container/start/) references.
Both modes share exact ID/image/secret-label ownership, single-use admission,
wall bounds, final state/exit inspection and verified exact-ID removal. If I/O
inspection fails after ownership was established, cleanup still has that exact
authority; ambiguous creation/identity never grants removal authority.

Captured results retain bounded `Stdout` and `Stderr` separately, leaving legacy
`Output` empty. Default-mode results retain their original merged `Output`.
The legacy proof/OOM profile cannot select split mode. Tests cover denied/old
consent, changed source, absent capabilities, input/argv rejection, TTY/input
state rejection, overflow, timeout/cancellation, attach failure, ownership,
single-use and failed cleanup. Tests use fake metadata and the owned test binary,
not a daemon: they do not establish actual Docker stdin/EOF or stream behavior.

## Remaining lint execution gates

No production lint call uses this transport and no nonempty fixture input is
admitted at the container boundary. The process seam can carry bounded nonempty
bytes, but that does not authorize target bytes or a lint role.

Client failure remains fail-closed even when the inspected container exit agrees.
Docker CLI's current [start implementation](https://raw.githubusercontent.com/docker/cli/master/cli/command/container/start.go)
also returns a nonzero container exit through its client status. Thus the lint
fault's expected exit 1 is **not yet representable as completed real-tool proof**
with this conservative contract. Review a pinned-client completion protocol or
an owned launcher/receipt before lint admission; do not treat matching exits as
proof of complete attachment. The upstream moving branch explains the risk,
not a tested or pinned production client version.

Next bind the fixed lint role/input, immutable image and transport-completion
protocol to a separate reviewed catalog and exact consent path. Dependency/image
preparation and real execution remain separate operations requiring their
concrete environment and authorization. See the
[current inert adapter scope](lint-sensitivity-preparation.md).
