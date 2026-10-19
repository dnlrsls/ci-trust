# CI Trust

**Understand configured CI without executing it.**

CI Trust implements an offline, read-only `audit` and a saved-report renderer.
It inventories bounded Go/npm declarations, analyzes GitHub Actions configuration
and explains evidence-backed configured gaps. Configuration is not observed
execution, validator efficacy or required-check enforcement; those remain UNKNOWN
without separate evidence. There is no aggregate green trust score.

## Start here: download → demo → limits

1. **Download** a native archive for your listed OS/architecture from
   [GitHub releases](https://github.com/dnlrsls/ci-trust/releases), verify it
   against `SHA256SUMS`, extract it and run `ci-trust --version`
   (`ci-trust.exe` on Windows). No Go or Node installation is needed for the
   downloaded binary. Archives are unsigned and not notarized.
2. **Try the before/after demo:** [see two configured platform gaps disappear
   without running CI](docs/audit-before-after-v0.1.1.md). It uses fresh copies
   of an existing fixture, not your project's tests or scripts.
3. **Read the limits:** the [CLI contract](docs/cli.md) explains why exit 0 can
   include findings and UNKNOWN. Configured paths are not executed tests,
   validator efficacy or required-check enforcement.

Building from source is optional; use it if no archive for your architecture is
listed. Existing releases and their checksums remain the download authority.

## Build and use

Requires Go 1.26; dependencies are pinned in go.mod/go.sum. An initial dependency
download may need network; the audit/report commands do not.

```sh
go build -o ci-trust ./cmd/ci-trust
./ci-trust --help
./ci-trust --version
./ci-trust audit PATH --format json > audit.json
./ci-trust report --input audit.json --format human
./ci-trust verify PATH --check lint --format json # currently UNVERIFIABLE; no tool execution
```

On Windows build `ci-trust.exe` and invoke `./ci-trust.exe`. The development version
is `dev`; build-time `-ldflags "-X main.version=VALUE"` sets it explicitly.
Versioned archives and `SHA256SUMS` are published on the
[GitHub releases page](https://github.com/dnlrsls/ci-trust/releases).
Choose the listed OS/architecture, verify the archive checksum, extract it and
run `ci-trust --version` (`ci-trust.exe` on Windows). Archives include the source
license, third-party notices and license texts; no Node/Docker image is bundled.
Build from source if an archive for your architecture is not listed.

- `audit PATH` requires an explicit target and defaults to human output.
  `--platform linux`, `--platform windows`, and `--platform macos` may each be
  supplied once to override the optional root `.ci-trust.json` declaration.
- `report` defaults to stdin and human output; `--input FILE|-` selects saved data.
  It validates and renders without rescanning or needing the source repository.
- Both accept `--format human|json`. JSON stdout is one canonical schema-v1
  document; diagnostics also appear on stderr. Human output retains scoped records
  and limitations, not a simplified success badge.
- `bootstrap PATH --dry-run` emits [conditional workflow proposals](docs/bootstrap.md),
  human by default or `--format json`. It never applies changes; missing runtime
  and dependency preparation remain explicit preconditions, not invented setup.
- `verify PATH --check lint` exposes [gated owned-fixture reporting](docs/lint-offline-integration.md).
  Ordinary host verification remains UNVERIFIABLE with no execution services;
  no project configuration/plugin is loaded and no real lint sensitivity is claimed.
  The explicit [workflow request mode](docs/lint-public-cli.md) connects only to
  the protected owned GitHub route, with strict sensitivity exit status; it is
  limited to the [risk-accepted exact profile](docs/lint-admission-policy.md)
  and separate authenticated review, never a host execution fallback.
- Exit **0** means valid report/plan (including findings/FAIL/UNKNOWN), help or version;
  **1** means operational failure; **2** means invalid invocation. Report input is
  bounded to 8 MiB; interactive stdin is refused.

**Release scope: offline MVP — audit, report and conditional bootstrap dry-run.**
Controlled lint execution is experimental and outside this release's acceptance.
The
[observed demo](docs/audit-demo.md) records input immutability, report parity and a
bounded historical self-audit. [Native CI receipts](docs/portable-ci.md) record
independently accepted public cases passing on Linux, Windows and macOS at the
exact tested revision. This is execution of CI Trust's own tests, not execution
or enforcement evidence for the workflows it audits.

The [0.1.1 release notes](docs/releases/0.1.1.md) describe repository boundaries
and lossless human summaries; the [first release notes](docs/releases/0.1.0.md)
record the original offline scope. [Issue #9](https://github.com/dnlrsls/ci-trust/issues/9)
remains open for a later execution-verification stage. No real isolated ESLint
sensitivity, project efficacy, historical CI execution or enforcement is claimed.

## Read next

1. [Try the v0.1.1 before/after demo](docs/audit-before-after-v0.1.1.md), or read
   the [historical audit/report observations](docs/audit-demo.md)
2. [Exact CLI options, rendering and errors](docs/cli.md)
3. [Architecture and support boundaries](docs/architecture.md)
4. [Evidence schema](docs/evidence-contract.md) and [semantics](docs/evidence-semantics.md)
5. [Static discovery bounds](docs/discovery.md) and [independent fixtures](docs/audit-fixtures.md)
6. [Contribution guidance](CONTRIBUTING.md) and [Go decision](docs/decisions/0001-use-go.md)

## Future work and safety

Ordinary `verify --check lint` remains cold; it does not run ESLint. The fixed
lint catalog has source admission with documented residual-risk acceptance, not
observed runtime compatibility or sensitivity. Actual execution still requires
exact protected human consent and bounded isolation. Smoke/proof approval cannot
authorize lint, and there is no host-shell fallback.
Bootstrap is evidence-backed conditional dry-run recommendations only, never apply.
The older [native audit receipt](docs/portable-ci.md) does not establish execution
of this later bootstrap implementation; new source requires its own verification.
The later opt-in, revision-pinned Gentleman-Programming/engram study must use generic
adapters without upstream writes; no study or network access is claimed here.

Core works without agents, LLMs or Gentle Shell. Audit/report do not execute target
scripts, hooks, actions or package managers, use credentials, fetch remote policy,
write hidden reports or fix files. User-directed stdout redirection is explicit.
The issue tagline “Trust your CI. Verify everything.” remains an aspiration.

## License

CI Trust source is licensed under the [MIT License](LICENSE).
Copyright (c) 2026 dnlrsls.

Third-party software retains its own licenses; see
[distribution notices](THIRD_PARTY_NOTICES.md) and [license texts](LICENSES/).
The MIT grant does not relicense Go dependencies, Node.js, npm, ESLint or container
base components. This release does not admit a new runtime or complete issue #9.
