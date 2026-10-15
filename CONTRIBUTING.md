# Contributing to CI Trust

Start with the [architecture](docs/architecture.md) and the
[CLI contract](docs/cli.md). The first Go unit is the evidence contract and
validator under internal/evidence; no CLI executable exists yet.

## Local checks

Use Go 1.26 (module language version 1.26.0). The implementation environment is
Go 1.26.4 on Windows/amd64; this does not establish Linux/macOS behavior.
Dependencies are pinned in go.mod/go.sum. Initial module download needs network;
contract validation itself never fetches schemas or contacts remote services.

- Run the suite: go test ./...
- Run static checks: go vet ./...
- Check formatting: gofmt -l internal/evidence

Read the [schema contract](docs/evidence-contract.md) and
[evidence rules](docs/evidence-semantics.md) before changing fixtures. Schema
validation and semantic validation are both required; neither proves evidence
truth. No licensed distribution until the maintainer chooses a license.

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
- The baseline is Go 1.26 with the pure-Go JSON Schema validator pinned in
  go.mod. No YAML parser or CLI framework is selected yet. New dependencies
  require a concrete need and compatible distribution assumptions.

## Verification expectations

For testable behavior, first observe a failing focused test, then implement and
run it plus the applicable suite. Go's standard test tooling is the baseline.
Contract tests exercise exported internal library boundaries. There is no
executable yet: library tests are not CLI, isolation or remote CI evidence.

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
