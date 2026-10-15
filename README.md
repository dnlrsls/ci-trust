# CI Trust

**Build, verify and understand your CI.**

A green CI does not necessarily mean a trustworthy CI. CI Trust will explain
which validations are configured, what execution is observed, what was actually
verified, and which guarantees remain unknown.

**Status: early MVP.** Go is accepted. The evidence contract is the first Go
library unit; the audit/analyzer CLI is not implemented. There is no executable,
installation command, implemented adapter or verified cross-host support yet.

## Read the design

1. [Architecture and support boundaries](docs/architecture.md)
2. [Why Go](docs/decisions/0001-use-go.md)
3. [Proposed CLI contract](docs/cli.md)
4. [Evidence schema and fixtures](docs/evidence-contract.md)
5. [Evidence rules, canonicalization and redaction](docs/evidence-semantics.md)
6. [Contribution guidance](CONTRIBUTING.md)

The Core will operate without agents, LLMs or Gentle Shell. Agents may consume
the same versioned JSON reports as developers; they are not required to produce
or interpret evidence.

## MVP sequence

- **P0:** architecture, evidence contract, static discovery, GitHub Actions
  analysis, bounded gap evaluation, audit/report CLI and independent E2E fixtures
  ([#1–#7](https://github.com/dnlrsls/ci-trust/issues)).
- **P1:** explicit consent and isolation, then controlled lint sensitivity
  ([#8](https://github.com/dnlrsls/ci-trust/issues/8),
  [#9](https://github.com/dnlrsls/ci-trust/issues/9)).
- **P2:** evidence-backed recommendations and reviewable dry-run diffs
  ([#10](https://github.com/dnlrsls/ci-trust/issues/10)).

The existing issue tagline, “Trust your CI. Verify everything.”, is an aspiration,
not a certification or an exhaustive-coverage claim.

## License status

No license has been selected or granted by these documents. Public visibility
does not itself grant an open-source license. A maintainer must explicitly select
one before licensed distribution; this repository is intended to become open source.
