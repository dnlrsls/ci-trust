# Admissible evidence and deterministic JSON

The internal evidence API checks supplied reports; it does not execute checks or
verify whether an adapter's assertions are true. Structural validation remains a
separate required boundary. No CLI or sensitivity runner is implemented.

## API

| Function | Responsibility |
| --- | --- |
| `ValidateSemantics` | Cross-record references, observation state, attributed causes, capabilities and workflow graphs |
| `Canonicalize` | JSON decoding, semantic checks and deterministic serialization |
| `StableID` | Kind-prefixed SHA-256 of length-framed kind and parts |
| `Redact` | Explicit replacement in every string value and object key |

All functions preserve input bytes. Rejections return safe diagnostics without
including supplied values. Serialization failures return no output bytes.

## Evidence acceptance

- Configuration is not execution. EXECUTED requires VALIDATION and controlled or
  historical run provenance. Historical SKIPPED cannot come from configuration.
- Known non-execution can carry STARTUP evidence. Other non-executed states do
  not carry outcomes. UNKNOWN does not establish a positive capability.
- Classified causes require a failed invocation, limitations and same-check
  supporting evidence with ASSERTION, FIXTURE, RUNNER or INFRASTRUCTURE basis.
  PRODUCT additionally requires VALIDATION. Exit status alone is insufficient.
- Resolved capabilities require same-check evidence and exact scope equality.
  This conservative v1 policy applies to historical runs too: no inferred scope
  hierarchy or promotion. Remote policy alone supports enforcement; controlled
  runs cannot establish repository CI.
- Efficacy references are a set of controlled TOOL_FIXTURE observations. Exactly
  one record for each CLEAN, FAULT and RESTORED role is required. Reference order
  is irrelevant. Tool name/version/configuration, environment and OS must be
  known and match. By role, PRESENT requires PASS/FAIL/PASS; ABSENT requires
  PASS/PASS/PASS. Duplicate or missing roles are rejected.
- UNKNOWN certainty on a finding is admissible; an unknown observation cannot
  establish a PRESENT execution capability.

These are admissibility rules, not security certification or a guarantee that
submitted evidence is trustworthy.

## Canonical ordering

Object keys use Go's deterministic JSON encoding. ID collections and workflow
jobs sort by ID; evidence references (including efficacy), needs and events
sort lexically. Arrays not declared as sets retain order. Numbers use
`json.Number` rather than floating-point conversion. Source line/column integers
are normalized using exact rational arithmetic: `1`, `1.0` and `1e0` produce the
same canonical position. Large integers retain every digit. Output ends in one
newline.

## Redaction responsibilities and limits

Producers supply every known secret and host prefix before persistence. Empty
values are ignored; longest matches take priority without recursively replacing
the replacement marker. Replacement applies to keys as well as values; key
collisions reject the document rather than silently losing data. Redaction is
not schema validation and makes no universal secret-detection promise. Do not
persist raw stdout, environment maps or captured shell logs. Stable ID inputs
must already exclude sensitive values.

## Integration review

The parent `Validate` and `Encode` wrappers must compose structural validation
with these checks. Lower-level `Canonicalize` checks JSON, unknown record fields
and semantics, but deliberately does not duplicate the complete structural
schema. Required fields, version selection, safe source locations, enums and
nested field types remain the structural boundary's responsibility. It returns
no output after a semantic rejection. Malformed nested records consumed by
semantic rules are guarded without trusting type assertions.

## Focused coverage matrix

| Tests | Accepted | Rejected |
| --- | --- | --- |
| `TestSemanticStates` | Configuration, unknown, unverifiable, known omission, historical skip | YAML execution and skip |
| `TestCauseMatrix` | Every classified cause with its matching basis; UNKNOWN with no references or exit-status evidence; non-product startup failures | Exit-only classification, unrelated-check support, missing references/limitations, passing invocation, startup PRODUCT |
| `TestCapabilityMatrix` | Remote policy, same-scope historical execution/skip, known omission, configuration | YAML/controlled enforcement, scope promotion, unknown/unverifiable execution guarantees |
| `TestEfficacyMatrix` | PRESENT, ABSENT, reversed reference set | Duplicate/missing roles, wrong outcomes/check/scope/tool/version/configuration/OS/environment, unknown identities |
| `TestNestedMalformed` | No panic or input mutation | Malformed provenance, execution, cause and scope records |
| `TestGraphAndReferenceFailures` | — | Dangling references/needs, duplicate IDs/events, cycles |
| `TestUnknownFindingAccepted` | UNKNOWN certainty | — |
| Canonical boundary tests | Stable IDs, deterministic ordering, equivalent source numbers, large integers, permuted efficacy sets | Unknown fields and semantic rejection without bytes |
| Redaction boundary tests | Explicit secrets/prefixes, longest-first replacement, unchanged input/numbers | Colliding transformed keys and ambiguous duplicate JSON keys |

The explicit-source focused command intentionally excludes the parent's schema
and wrapper files. Full composed malformed-shape rejection is a parent seam
check, not an observed result of this unit's focused tests. The initial semantic
implementation historically preceded its tests; this correction batch does not
claim to repair that past test-first exception.
