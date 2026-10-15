# ADR 0001: Use Go for the MVP

**Status: accepted.** The maintainer selected Go during issue #1 discussion.
This accepts a language, not a working implementation, dependency set or release.

## Decision

Use Go for a small Core library and a thin CLI. Prefer pure-Go dependencies so
the audit executable can be distributed per OS/architecture without requiring
Python, Node.js, a container runtime or a C toolchain on the user's machine.
Controlled verification may require a separately provisioned isolation runtime.

Do not add a plugin framework, agent runtime or concurrency merely because Go
supports them. Start with bounded, straightforward processing.

## Alternatives and tradeoffs

| Concern | Go | Python | TypeScript/Node.js |
| --- | --- | --- | --- |
| Distribution | Per-target executable; users need not install Go | Interpreter/environment or additional executable packaging | Node.js runtime or additional executable packaging |
| Portability | GOOS/GOARCH cross-builds are practical for pure Go | Portable source; packaging and native dependencies still vary | Portable runtime; packaging and native dependencies still vary |
| Parsing/tooling | Explicit typed models; YAML needs a reviewed dependency | Concise data transformations and mature parser tooling | Strong JSON/YAML tooling and npm familiarity |
| Performance | Suitable compiled baseline for file scanning and rules | Likely adequate for this bounded MVP | Likely adequate for this bounded MVP |
| Maintenance cost | More explicit error handling and dynamic-data modeling | Runtime environment and type-validation discipline | Runtime/dependency lifecycle and type-validation discipline |

No benchmark establishes a performance advantage for this project. Distribution
and operational simplicity, not presumed speed, decide the choice.

## Consequences

- Types help separate observations, outcomes and limitations, but do not make
  evidence true. JSON schema compatibility and deterministic ordering require
  explicit validation and tests.
- A YAML parser is not a GitHub Actions interpreter. Correct trigger parsing,
  source locations, bounded expansion and unresolved expressions are project work.
- Cross-compilation is not proof that a binary or its tests work on the target OS.
  Native Linux, Windows and macOS execution evidence is still required.
- Avoid cgo for the baseline. Introducing C dependencies can require target
  toolchains and changes the cross-build/distribution assumptions.
- Built-in testing and fuzzing support are useful; fixtures still need independent
  expected results and public-interface checks.
- Choose the Go version, YAML parser and any CLI dependency when introducing the
  module, with supported toolchain policy and pinned dependencies. This ADR
  installs nothing and selects no third-party package.

Revisit if concrete adapter or distribution constraints outweigh these benefits,
not merely because another language can parse YAML.

## References

- [Issue #1](https://github.com/dnlrsls/ci-trust/issues/1)
- [Go build command and environment](https://pkg.go.dev/cmd/go)
- [cgo and cross-compilation constraints](https://pkg.go.dev/cmd/cgo)
- [Go testing](https://pkg.go.dev/testing)
- [Go fuzzing tutorial](https://go.dev/doc/tutorial/fuzz)
