# CI Trust

**Understand configured CI without executing it.**

CI Trust implements an offline, read-only `audit` and a saved-report renderer.
It inventories bounded Go/npm declarations, analyzes GitHub Actions configuration
and explains evidence-backed configured gaps. Configuration is not observed
execution, validator efficacy or required-check enforcement; those remain UNKNOWN
without separate evidence. There is no aggregate green trust score.

## Build and use

Requires Go 1.26; dependencies are pinned in go.mod/go.sum. An initial dependency
download may need network; the audit/report commands do not.

```sh
go build -o ci-trust ./cmd/ci-trust
./ci-trust --help
./ci-trust --version
./ci-trust audit PATH --format json > audit.json
./ci-trust report --input audit.json --format human
```

On Windows build `ci-trust.exe` and invoke `./ci-trust.exe`. The development version
is `dev`; build-time `-ldflags "-X main.version=VALUE"` sets it explicitly.
No installation or licensed release/distribution is claimed.

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
- Exit **0** means valid report/plan (including findings/FAIL/UNKNOWN), help or version;
  **1** means operational failure; **2** means invalid invocation. Report input is
  bounded to 8 MiB; interactive stdin is refused.

**Status: implemented audit/report slice, not a complete MVP.** The
[observed demo](docs/audit-demo.md) records input immutability, report parity and a
bounded historical self-audit. [Native CI receipts](docs/portable-ci.md) record
independently accepted public cases passing on Linux, Windows and macOS at the
exact tested revision. This is execution of CI Trust's own tests, not execution
or enforcement evidence for the workflows it audits.

## Read next

1. [Reproduce the audit/report demo and understand its limits](docs/audit-demo.md)
2. [Exact CLI options, rendering and errors](docs/cli.md)
3. [Architecture and support boundaries](docs/architecture.md)
4. [Evidence schema](docs/evidence-contract.md) and [semantics](docs/evidence-semantics.md)
5. [Static discovery bounds](docs/discovery.md) and [independent fixtures](docs/audit-fixtures.md)
6. [Contribution guidance](CONTRIBUTING.md) and [Go decision](docs/decisions/0001-use-go.md)

## Future work and safety

`verify` is not implemented. Controlled verification
requires explicit target-bound consent and bounded isolation; no production Docker
backend or trusted image/catalog is available. There is no host-shell fallback.
Bootstrap is evidence-backed conditional dry-run recommendations only, never apply.
The older [native audit receipt](docs/portable-ci.md) does not establish execution
of this later bootstrap implementation; new source requires its own verification.
The later opt-in, revision-pinned Gentleman-Programming/engram study must use generic
adapters without upstream writes; no study or network access is claimed here.

Core works without agents, LLMs or Gentle Shell. Audit/report do not execute target
scripts, hooks, actions or package managers, use credentials, fetch remote policy,
write hidden reports or fix files. User-directed stdout redirection is explicit.
The issue tagline “Trust your CI. Verify everything.” remains an aspiration.

## License status

No license has been selected or granted by these documents. Public visibility
is not an open-source license. Licensed release/distribution waits for an explicit
maintainer license choice.
