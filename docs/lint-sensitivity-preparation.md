# Offline lint sensitivity preparation (#9)

**Status: pure classifier and owned fixture data only. No ESLint adapter, pinned
runtime image, public `verify` command, or real sensitivity evidence is shipped
by this unit. #8 real-isolation acceptance and #9 acceptance remain open.**

`internal/lintsensitivity` has no filesystem reads, subprocesses, network,
configuration loading, consent creation, or runner connection. Its embedded
JavaScript is data, not executed code. It accepts only normalized, supplied
observations. A caller can fabricate them: classification is not authentication.

## Owned fixture contract

Each role uses the virtual filename `fixture.js` in a future isolated run:

| Role | Owned source | Independently authored expectation |
| --- | --- | --- |
| CLEAN | Declaration and `void` expression | Exit 0, no diagnostics |
| FAULT | Same bytes prefixed with `debugger;` and LF | Exit 1, one nonfatal `no-debugger` error at line 1, column 1 |
| RESTORED | Byte-identical to CLEAN | Exit 0, no diagnostics |

The embedded JSON recipe enables only `no-debugger` at error severity. It is
**not** an ESLint config loader or executable flat config. No target config,
plugin, parser, ignore file, script, or package manager is discovered or loaded.
Unsupported project configurations therefore cannot be evaluated by this unit;
the future public adapter must explicitly report them as UNVERIFIABLE.

The rule's intended behavior is documented by
[ESLint's no-debugger reference](https://eslint.org/docs/latest/rules/no-debugger).
[ESLint's exit contract](https://eslint.org/docs/latest/use/command-line-interface#exit-codes)
distinguishes lint findings from configuration/internal errors. These sources
inform authored expectations; they are not receipts from running the fixtures.

## Classification boundary

`Classify` requires exactly CLEAN, FAULT, RESTORED in that asserted order. The
three observations must share tool name, exact release-shaped version,
configuration SHA-256, immutable image-shaped ID, environment, Linux OS, and
source snapshot SHA-256. Each fixture digest must match its embedded role. The
recipe/configuration digest binds these owned bytes, not a repository config.

Version/image syntax is **not** a trusted catalog or a support promise. The
unit tests use synthetic identities (including version `9.0.0`); no ESLint version
or image is selected, installed, fetched, approved, or executed here. A future
adapter must bind the recipe to its reviewed effective configuration and pin
tool, Node runtime, dependency closure, image, command vector and output parser.

| Internal outcome | Meaning |
| --- | --- |
| SENSITIVE | Supplied controls pass and fault has exactly the expected error |
| INSENSITIVE | All three supplied invocations complete without any finding |
| UNVERIFIABLE | Incomplete/mismatched identity or fixtures, failed controls, unexpected diagnostic, or harness failure |

`HARNESS_FAILURE` includes denied consent, missing tool/runtime, timeout, crash,
invalid output, unverified cleanup, unknown status, and exit codes other than
0/1. Original per-observation status remains with the caller. A wrong-rule error
or exit 1 alone is never sensitivity success. Returned reason codes are bounded
constants; raw logs and supplied identity values are not echoed. Inputs are not
mutated. Scope is always TOOL_FIXTURE, never repository configuration or CI.

## Existing report contract and remaining work

This internal result is **not** a second JSON evidence format. The existing
[v1 evidence contract](evidence-contract.md) and
[semantic rules](evidence-semantics.md) remain unchanged. A future producer must
map authenticated triplets to TOOL_FIXTURE EFFICACY PRESENT/ABSENT observations,
retain diagnostic/fixture support and limitations, then validate the whole
report. UNVERIFIABLE must not become efficacy PRESENT/ABSENT. Schema acceptance
alone does not establish diagnostic matching, isolation, chronology or truth.

Before runtime integration: finish/review #8's real proof; review exact pins and
the bounded structured-output parser; add a trusted allowlisted adapter without
repository config discovery; authenticate exact-plan human consent; prove
no-start on denied/stale consent and source immutability; implement public
CLI/help/report mapping; and run the real pinned tool only under a separately
authorized isolated plan. Do not widen the current smoke-only runner allowlist
or treat these synthetic unit observations as controlled execution evidence.

## Local verification

Run `go test ./internal/lintsensitivity` for authored positive/negative
classification, fixture bytes, and input immutability. These Go tests never
launch Node, ESLint or Docker, and do not establish real validator sensitivity.
