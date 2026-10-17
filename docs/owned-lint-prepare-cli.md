# Owned lint preparation CLI

## Safe help

From the repository root, display help without reading preparation metadata,
downloading dependencies or executing the preparer:

```sh
go run ./internal/lintpreparation/cmd/owned-lint-prepare --help
```

Only a single `-h` or `--help` is accepted. There are no operational flags.
**Do not remove `--help` for a local demonstration.** The no-argument operation
is effectful and requires separate authorization for a fresh protected hosted
Linux/amd64 job. This guide is not that authorization.

## Effectful operation prerequisites

The caller must provide these four nonempty environment values:

| Key | Purpose |
| --- | --- |
| `GITHUB_WORKSPACE` | Trusted, stable workspace |
| `GITHUB_SHA` | Recorded source revision |
| `GITHUB_RUN_ID` | Recorded run identifier |
| `GITHUB_RUN_ATTEMPT` | Recorded attempt |

Values pass unchanged; source/run metadata are recorded, not authenticated.
No other environment keys are read by the command. The accepted preparation
helpers control process environment; host Node options, credentials and npm
configuration are not inherited.

Before invocation, the protected caller must create the private
`.git/lint-image-preparation` preparation area (including workflow `tmp`/`cache`
directories), its generated `inputs.json` and built Linux/amd64 `owned-eslint`
launcher. The source manifest is `tools/lint-preparation/package.json`.
The workspace, owned inputs and executable provenance are caller trust
preconditions, not host/mount isolation. The `artifacts` directory must not
already exist; the preparer never overwrites preexisting artifact data.

The fixed operation downloads authenticated pinned Node/base data, materializes
Node, and runs Node/npm **only for dependency preparation** through the fixed
registry-only setup. Package scripts are disabled. It does not execute ESLint,
the owned launcher, target code, Docker or the catalog. Success is silent and
is **preparation only, not runtime admission** or a library-compatibility proof.
SIGINT/SIGTERM cancel the preparation context; owned temporary cleanup is checked.
Writes are not an atomic whole-job rollback.

## Outputs and exit codes

Seven files are written under `.git/lint-image-preparation/artifacts`:
`image.tar`, `package.json`, `package-lock.json`, `inventory.json`,
`owned-inputs.json`, `preparation.json`, and `SHA256SUMS`.
The Go producer records `go_version` in metadata rather than legacy
`python_version`; this is not an independent reproducibility claim.

| Exit | Meaning |
| --- | --- |
| 0 | Help displayed, or preparation completed silently |
| 2 | Invalid arguments or missing required metadata |
| 1 | Preparation, help-output or diagnostic-output failure |

Failures attempt exactly one existing redacted preparation diagnostic plus a
newline on stderr, with fixed stage/category/reason vocabulary. Arguments,
environment values, raw errors, paths, URLs and process output are not echoed.
A failed diagnostic write gets no secondary raw-error fallback.

## Migration status and gates

The existing workflow still uses Python until the separate workflow-cutover
unit. Adding this command does not dispatch a workflow, authorize execution,
retire Python or complete the migration's independent source verification.
See [lint image preparation](lint-image-preparation.md) for the separate gates.
