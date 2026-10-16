# Offline lint sensitivity preparation (#9)

**Status: pure classifier, owned fixture data, inert ESLint invocation descriptor,
bounded JSON normalizer and internal launcher-receipt framing candidate only.
No process executor, pinned runtime image,
public `verify` command, or real sensitivity evidence is shipped by this unit.
#9 acceptance remains open. The separately recorded [bounded #8 acceptance](https://github.com/dnlrsls/ci-trust/issues/8#issuecomment-6206909812)
does not authorize ESLint execution.**

The [runtime preparation and integration plan](lint-runtime-plan.md) records
candidate upstream versions, unresolved immutable pins, the Node security review
gate and the runner transport/image changes required before admission. It does
not select an executable catalog entry or authorize preparation/execution.

`internal/lintsensitivity` has no filesystem reads, subprocesses, network,
configuration loading, consent creation, or runner connection. Its JavaScript is
data, not executed code. Both raw captures and normalized observations are
caller-supplied. A caller can fabricate them: parsing and classification are not
authentication.

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

## Inert ESLint adapter contract

`OwnedESLintInvocation` returns fresh arguments, owned stdin fixture bytes and
one fixed flat-config module. It accepts only the three existing roles, no
caller-selected paths, flags, source, config or plugins. It has no executable,
process creation, image preparation, runtime admission or approval operation.
The proposed isolated paths are `/owned/eslint.config.mjs` and `/work/fixture.js`;
these are not host paths. No files are written by this package.

The descriptor selects `--no-config-lookup`, `--no-ignore`, explicit owned config,
the built-in JSON formatter, and stdin with the fixed virtual filename. The
config imports nothing, enables only `no-debugger`, fixes ECMAScript 2022/module
parsing, disables inline configuration and unused-disable reporting. This is
owned executable configuration data for a future reviewed image, never a loader
for target-repository configuration. Its exact bytes and SHA-256 are returned.
`Identity.ConfigSHA256` still binds the existing JSON recipe; future trusted
plans must **also** bind the effective module digest, arguments and parser
revision. This descriptor does not supply that trusted binding.

`DecodeESLint` normalizes supplied `ESLintCapture` bytes into the existing
`Observation`. `Completed` is a **caller assertion** that trusted execution,
consent, cleanup and source preservation succeeded. Known failure statuses stay
failures; unknown statuses become `INVALID_OUTPUT`. Invalid identities or roles
also fail closed. No capture can itself authenticate identity or chronology.

The deliberately narrow, candidate formatter profile requires:

- At most 64 KiB combined stdout/stderr, empty stderr, valid UTF-8, one JSON
  document, depth at most eight, no nulls or duplicate decoded object keys.
- Exactly one result at `/work/fixture.js`, core diagnostic fields and all counters
  required, exact case-sensitive keys, integer counters matching diagnostics,
  and exit 0/1 consistent with the error count.
- At most sixteen messages with rule, severity, prose and positive location;
  optional paired end locations must not precede their starts. Only normalized
  rule/severity/location/fatal fields are retained, not arbitrary raw prose.
- Empty suppressed messages, zero fixable counts, no transformed output or
  unknown fields. Optional deprecation metadata must be empty; optional source
  must equal the owned fixture bytes. Missing/unknown output fails closed.

Valid wrong-rule, warning or extra-diagnostic output still goes to `Classify`,
which cannot call it sensitive. Malformed or inconsistent output becomes
`INVALID_OUTPUT` without partial diagnostics. Real-tool compatibility has not
been demonstrated: an exact tool/runtime/dependency/image pin and captured
formatter compatibility tests remain prerequisites for execution admission.
The [formatter reference](https://eslint.org/docs/latest/use/formatters/) and
[CLI reference](https://eslint.org/docs/latest/use/command-line-interface) inform
this candidate contract, not a guarantee for an unselected ESLint release.

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

Before runtime integration: retain #8's bounded acceptance; review exact pins and
the bounded structured-output parser; complete a trusted allowlisted adapter without
repository config discovery; authenticate exact-plan human consent; prove
no-start on denied/stale consent and source immutability; implement public
CLI/help/report mapping; and run the real pinned tool only under a separately
authorized isolated plan. Do not widen the current smoke-only runner allowlist
or treat these synthetic unit observations as controlled execution evidence.

## Local verification

Run `go test ./internal/lintsensitivity` for authored positive/negative
classification, fixture bytes, and input immutability. These Go tests never
launch Node, ESLint or Docker, and do not establish real validator sensitivity.
Adapter tests use independently authored formatter-shaped JSON, including
malformed, duplicate, oversized, identity, counter, source and diagnostic cases.
Fuzz seeds exercise only the Go normalizer. No formatter output in these tests
is a receipt from ESLint execution.
