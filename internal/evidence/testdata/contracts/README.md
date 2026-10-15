# Independently authored contract fixtures

Use `manifest.json` as the expectation source, not analyzer output. Every entry
names a base report (`file`), a scenario (`name`), structural acceptance and
standalone semantic acceptance. Rejections carry `structural_code` and/or
`semantic_codes`. Semantic codes are the expected distinct code set, not a
required ordering or number of occurrences. These semantic expectations are
independently authored and checked by the composed validation tests and the
independent issue #2 verification; they are not inferred from analyzer output.

## Apply a scenario

1. Read the named report as bytes.
2. For each `mutations` entry in order, require `old` to occur and replace its
   **first** occurrence with `new`, exactly as written. No mutation means the
   original report. Do not normalize whitespace before replacement.
3. Validate the resulting bytes against the declared structural verdict.
4. Independently run semantic validation and compare its verdict/code set.

A structural rejection may intentionally have `semantic: true`: the standalone
semantic validator does not replace the schema's version, path or metadata checks.
The composed validator must reject if either layer rejects. Schema acceptance
never overrides a negative semantic verdict.

## Base reports

| File | Authored evidence |
|---|---|
| `valid.json` | Configured-only check with source provenance. |
| `states.json` | All six states; bounded clean PASS; historical skip; pre-validation startup failure with exit-only UNKNOWN cause; unknown versus unavailable history. |
| `bounded-evidence.json` | All non-unknown causes with separate same-check support, controlled sensitivity triplet and independent remote policy enforcement. Includes workflow/job, finding and diagnostic records. |
| `invalid-enum.json` | Invalid state; both shape and semantics reject. |

All four classified failures in `bounded-evidence.json` reference separate
support records with the same `check_id` (`check`). Their supporting bases are
ASSERTION, FIXTURE, RUNNER and INFRASTRUCTURE respectively. The mismatched-check
mutation introduces a valid second CHECK and moves only product support to it;
its rejection cannot be explained by a dangling check ID. Startup failures do
not execute product assertions.

The CLEAN PASS / FAULT FAIL / RESTORED PASS observations share check, scope, OS,
environment and tool/version/configuration identity. Their capability remains
TOOL_FIXTURE-scoped. The remote policy record has no execution claim; configured
YAML remains separate. Mutations deliberately break identity, broaden scope,
substitute a single PASS, or replace policy with YAML/controlled execution.

## Acceptance matrix

| Requirement | Manifest scenarios |
|---|---|
| Configuration versus execution/skip | configured, six-states, yaml-executed, yaml-skipped |
| Observation versus outcome | six-states, configured-with-outcome, invalid-outcome |
| Startup cannot prove product failure | startup-product-claim |
| Supported classified attribution | supported-causes-sensitivity-policy |
| Ambiguous/exit-only cause remains UNKNOWN | six-states, unsupported-attribution-remains-unknown, exit-only-product-claim |
| Same-check support | same-check-required |
| Bounded sensitivity and identities | supported-causes-sensitivity-policy, single-pass-not-efficacy, identity-mismatch, bounded-scope-not-repository |
| Remote policy is not YAML or a run | supported-causes-sensitivity-policy, yaml-not-enforcement, controlled-run-not-enforcement |
| Attributable invalid references | dangling-reference |
| Unsupported version, enums, provenance | bad-version, invalid-enum, missing-provenance |
| Metadata and relative paths | metadata-error, unsafe-path |

The Go tests additionally mutate every defined object boundary to add an unknown
property. They check precision, trailing JSON, SHA shape, absolute repository/path
metadata and unchanged input bytes. These are explicit test expectations, not
implementation-regex assertions.

## Serialization/redaction fixture recipes

These recipes describe inputs for the sibling's existing serializer/redactor
checks; this structural unit does not implement or re-test those operations.

- **Ordering:** Starting with `bounded-evidence.json`, reverse the inventory,
  capabilities, evidence, findings and diagnostics collections, then reverse
  workflow event sets where applicable. Both inputs remain structurally valid.
  Reverse evidence-reference sets too, including sensitivity references: fixture
  roles live in referenced evidence records, not caller reference ordering.
  Canonical output for equivalent set orderings must be byte-identical, including
  one final newline.
- **Redaction:** In `valid.json`, replace the inventory display name `Lint` with
  `fixture-secret C:/fixture-host/repo`. Supply exactly `fixture-secret` and
  `C:/fixture-host` to `Redact`. The original remains structurally valid (display
  text is not a source location); neither supplied sensitive string may survive
  in redacted canonical bytes. No claim of automatic secret detection follows.
- **Unknown metadata:** `states.json` deliberately retains null repository/tool/
  environment information; serialization must not synthesize values.
