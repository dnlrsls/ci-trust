# Validate bounded evidence, not inferred trust

A report must pass both the embedded draft 2020-12 schema and semantic validation.
`ValidateStructure(data)` checks JSON and shape offline; it does not establish that
submitted evidence is true or sufficient. This is an internal library contract,
not a CLI or a promised public module API.

## Quick path

1. Author a v1 report with every root collection present, even when empty.
2. Use `ValidateStructure` to find shape errors, then the full validator to check
   references and evidence rules. See [semantic rules](evidence-semantics.md).
3. Redact explicitly supplied secrets and host prefixes before serialization.
   Schema validation is not a general secret detector.

## Report fields

| Field | Meaning |
|---|---|
| `schema_version` | Exactly the string `1`; unsupported versions are rejected. |
| `snapshot` | Stable ID, nullable repository identity and nullable full lowercase 40/64-character SHA. Null means unknown, not an invented value. |
| `inventory` | STACK, TOOL or CHECK entries with relative source locations and evidence references. |
| `capabilities` | Independent CONFIGURATION, EXECUTION, EFFICACY or ENFORCEMENT facts; status PRESENT, ABSENT, UNKNOWN or UNVERIFIABLE. |
| `workflows` | Workflow source, unique event names and job dependency graph. Not an execution claim. |
| `evidence` | Check identity, scope, provenance, observation state, nullable execution/cause and limitations. |
| `findings` | Stable rule/record IDs, evidence references, scope, explanation, DIRECT/DERIVED/UNKNOWN certainty and its authored rule. No numeric score. |
| `diagnostics` | Authored ID/code/message and nullable source; distinct from returned validation diagnostics. |
| `limitations` | Authored bounds and uncertainty; not raw logs or environment dumps. |

All defined records reject unknown fields. IDs use ASCII letters/digits followed
by letters/digits, dots, underscores, colons or hyphens. Locations use nonempty
root-relative forward-slash paths without drive prefixes or dot/traversal
segments; line and column are positive integers. Scope kinds are REPOSITORY,
WORKFLOW, JOB, STEP and TOOL_FIXTURE, with a stable subject ID.

## Observation and outcome are independent

| State | Meaning and semantic requirement |
|---|---|
| CONFIGURED | Configuration provenance, not observed execution. |
| EXECUTED | VALIDATION-phase execution from CONTROLLED_RUN or HISTORICAL_RUN. |
| SKIPPED | Observed historical skip, not static YAML ineligibility. |
| NOT_EXECUTED | Known omission; may include a failed STARTUP invocation. |
| UNKNOWN | Unresolved execution history. |
| UNVERIFIABLE | Verification unavailable or unrealizable. |

Only EXECUTED and NOT_EXECUTED can carry execution records under the semantic
rules. Execution outcome is PASS, FAIL or UNKNOWN; phase is STARTUP or VALIDATION.
OS is LINUX, WINDOWS, MACOS or null; environment and tool metadata remain nullable.
Tool metadata contains name, version and configuration identity. Controlled
fixture roles are CLEAN, FAULT and RESTORED, otherwise null. A single bounded PASS
proves neither functional sensitivity nor remote enforcement.

Provenance kind is CONFIGURATION, CONTROLLED_RUN, HISTORICAL_RUN, REMOTE_POLICY
or DIAGNOSTIC. Its basis is SOURCE, ASSERTION, FIXTURE, RUNNER, INFRASTRUCTURE,
EXIT_STATUS or UNRESOLVED; adapter and source locations preserve attribution.
Cause categories are PRODUCT, TEST_FIXTURE, HARNESS,
ENVIRONMENT_INFRASTRUCTURE and UNKNOWN. Non-unknown classifications require
supporting evidence and limitations. STARTUP failure cannot prove a product defect;
exit status alone leaves cause unknown. See semantic rules for admissible support,
controlled sensitivity and remote policy requirements.

## Offline validation and compatibility

The Go validator embeds `internal/evidence/schema/report-v1.json`, compiles it
with pinned pure-Go jsonschema/v6 v6.0.3, and denies external resource loading.
The HTTPS schema identifier is never fetched. JSON numbers retain their original
precision during validation. Malformed or trailing JSON is rejected without
modifying input bytes. Returned diagnostics contain stable `code`, JSON-pointer
`path` and safe generic `message`, not rejected values or compiler host paths.
Codes include `json.invalid`, `schema.invalid` and `schema.version`.

Unknown versions are not interpreted as v1. Version changes require explicit
schema and consumer compatibility work, not silently ignored extra fields.
Canonical ordering, stable IDs and redaction are semantic/serialization concerns;
volatile timestamps and run IDs do not belong in this canonical document.

## Fixtures

[`manifest.json`](../internal/evidence/testdata/contracts/manifest.json) maps each
independently authored file to expected structural and semantic acceptance.
`structural_code` identifies the expected first shape diagnostic for rejection.
Each manifest scenario has structural and standalone semantic verdicts, with
expected rejection codes. Optional `mutations` apply exact first-occurrence byte
replacements to the named base report in order. The tests require every mutation
target to exist. See the [fixture README](../internal/evidence/testdata/contracts/README.md)
for the full acceptance mapping and deterministic serialization/redaction recipes.
These expectations are independently authored, not analyzer-generated. Structural
verdicts are exercised by this unit; semantic verdicts require the parent's seam
check against the separate semantic validator.

| Acceptance dimension | Independent examples |
|---|---|
| Six distinct states, known startup omission, bounded PASS | `states.json` |
| Product, fixture, harness and infrastructure attribution | `bounded-evidence.json`, separate same-check support records |
| Unknown versus unsupported attribution | Exit-only UNKNOWN in `states.json`; startup, exit-only and wrong-check claim mutations |
| Controlled sensitivity with matching identity | CLEAN PASS / FAULT FAIL / RESTORED PASS in `bounded-evidence.json`; single-pass, identity and scope negatives |
| Remote enforcement independent of execution | Remote policy capability in `bounded-evidence.json`; YAML and controlled-run substitution negatives |
| Reference and state admissibility | Dangling-reference, YAML EXECUTED/SKIPPED and configured-with-outcome mutations |
| Version, provenance and metadata rejection | Bad-version, missing-provenance, enum/outcome, SHA and unsafe-path scenarios |
| Closed record boundaries and precision | Direct extra-property tests for every defined object record; precise fractional line rejection |
| Stable serialization and explicit redaction | Fixture README recipes; implementation and runtime checks belong to the sibling unit |

A schema-invalid report can intentionally pass standalone semantic checks, since
that layer does not duplicate version/path/metadata shape validation. It must
still fail composed validation. Semantic codes and admissibility policy are
specified in [semantic rules](evidence-semantics.md).
