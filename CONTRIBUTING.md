# Contributing to CI Trust

Start with the [architecture](docs/architecture.md) and the
[CLI contract](docs/cli.md). This repository currently contains design documents,
not a buildable Go module or executable.

## Working conventions

- Keep technical documentation, identifiers and code comments in English.
- Keep changes scoped to an agreed issue; document evidence and limitations
  alongside the behavior. Use Conventional Commit messages when commits are made.
- Use small explicit Go packages, standard formatting and minimal reviewed
  dependencies. Do not introduce a plugin framework or a public API prematurely.
- Proposed implementation layout: cmd/ci-trust for CLI wiring, internal packages
  for model/discovery/workflows/rules/rendering and testdata for owned fixtures.
  Names may be refined when #2 establishes contracts; no directories or API
  exports are created by this proposal.
- Separate Core logic from terminal I/O, consent and process control. The Core
  must not depend on an agent, LLM, Gentle Shell or an online service.
- Select a supported Go toolchain and pinned dependencies when adding go.mod.
  No Go version, YAML parser or CLI framework is prescribed yet.

## Verification expectations

For testable behavior, first observe a failing focused test, then implement and
run it plus the applicable suite. Go's standard test tooling is the baseline.
There are no runnable product tests yet; do not present future commands as checks
already performed.

Define fixture expectations independently of analyzer output. Public-interface
checks must cover exit status, exact stdout/stderr, no execution during audit,
source immutability and deterministic serialization. Preserve unknowns instead
of promoting configuration into execution, efficacy or enforcement.

Cross-builds do not establish host behavior. Record observed Linux/Windows/macOS
results or explicitly mark missing evidence. Real controlled verification requires
the approved isolation boundary; unit mocks cannot replace isolation proof.

For documentation-only changes, check local links, command examples against the
written contract, consistency with the owning issues and whitespace. Runtime
RED/GREEN is not meaningful without executable behavior.

## Safety and license

Never run an audited repository's scripts, hooks, package managers or actions.
Treat input files as untrusted; bound parsing and traversal. Do not fetch remote
evidence during static audit. No auto-fix/apply mode belongs to the MVP.

No license is selected. Do not add a license header, grant or claim an open-source
license on behalf of the maintainer. Licensed distribution waits for an explicit
license choice.
