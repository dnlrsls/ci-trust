# Manual owned lint workflow control

The source workflow is `.github/workflows/lint-sensitivity-verification.yml`.
It is manual-only, owner/main/first-attempt scoped and uses the existing
`ci-trust-controlled-verification` Environment without policy changes. The plan
job produces non-authorizing request data; the dependent verification job needs
the owner's exact `approve lint <binding_digest>` review. Environment approval
alone is insufficient. Expired requests need a separately authorized fresh run,
not an automatic retry.

The plan job uses the internal `controlled-lint prepare` command. The protected
verification job builds and invokes the public CLI:

```text
ci-trust verify "$GITHUB_WORKSPACE" --check lint --workflow-request - --format json
```

Its stdin is the bounded non-authorizing plan, never imported execution proof.
Request mode emits the validated own report and exits 0 only for owned
TOOL_FIXTURE sensitivity; unresolved/insensitive/operational outcomes exit 1,
invalid arguments exit 2. The shell propagates this exit unchanged. Normal public
`verify` remains cold/valid-report exit 0; the request flag is neither approval nor
a local execution fallback. See [public entry limits](lint-public-cli.md).

The internal Go command still accepts exactly `--help`, `prepare` or `verify`;
its existing verify operation remains compatible.
Workspace/run/revision come from the fixed workflow context, not override flags.
`verify` reads the bounded canonical request on stdin; it never imports tool
observations, admission state or approval arguments. Its compiled gate precedes
input/environment/service access. The [source-admitted profile](lint-admission-policy.md)
may prepare data only in the fixed trusted context; absent that context, operations
exit 1 with `controlled-lint: workflow unavailable`. Admission is not exact review.

Exit 0 means prepared data, or owned TOOL_FIXTURE sensitivity evidence for verify,
never project configuration efficacy or historical CI enforcement. Verify emits
its own v1 report then exits 1 for insensitive/unverifiable outcomes. Operational
failure also exits 1; invalid arguments exit 2. This differs deliberately from
the public read-only `verify --check lint`, whose valid unresolved report exits 0.

The workflow builds locked Go sources (internal planner/public verifier) with
existing action/compiler pins; its
future builds may fetch checksum-locked Go modules, not target dependencies.
The private empty Docker config is created only inside the Environment-protected
job after human review. The Go composition then independently authenticates that
review before delivery/load/capture. Only plan/report JSON is retained for seven
days; no runtime image, source, cache or credential is uploaded.

## Failed harness diagnostics

`LINT_HARNESS_FAILURE` remains UNVERIFIABLE, never a lint PASS. Its message adds
only allowlisted failed role, stage, capture status, reported outer exit and
runtime reason code. Stages distinguish `OUTER_CAPTURE`, `OUTER_EXIT`,
`OUTER_STDERR` and `RECEIPT_OR_ESLINT_OUTPUT`; the last intentionally does not
claim whether framing or strict ESLint output parsing failed. An outer launcher
failure is not the expected inner FAULT exit 1. Raw stdout/stderr, arbitrary
runtime reasons, paths and credentials are never included. Unknown values are
sanitized or the detail is omitted. This metadata does not establish Node
execution or identify an ELF/Node/Docker root cause; no acceptance, consent,
resource limit, plan digest or schema changes follow from it.

Source admission permits non-authorizing planning in that fixed context, not
execution without separate exact review. No workflow was dispatched or externally
validated here; no Node, Docker or image was executed,
no new candidate was fetched, and no Environment approval/settings were changed.

## Failed preflight diagnostics

`LINT_PREFLIGHT_UNAVAILABLE` remains UNVERIFIABLE and precedes the case loop.
Its message may add only `REVIEW_BEFORE_DELIVERY`,
`ARTIFACT_DELIVERY_OR_REVIEW`, `RUNTIME_CREATION` or `IMAGE_LOAD`. The combined
delivery/review stage intentionally does not guess whether delivery, empty
content or the following review check failed. Source-change precedence remains
`LINT_SOURCE_CHANGED`, without this detail. Unknown stages are suppressed.
These stages identify a workflow boundary, not the underlying cause or Node
execution. No raw errors, outputs, paths or credentials are reported; acceptance,
consent, pins, limits, report schema and diagnostic IDs do not change.

## Independent execution and cleanup facts

The existing internal runner result now retains whether Start was requested,
a closed primary failure code and whether Remove was attempted. Cleanup remains
derived from the existing verified-absence flag; no-start does not become
verified cleanup. A failed Start and unverified Remove therefore survive together
even though cleanup still wins the legacy rejection reason. These optional facts
are excluded from Result JSON; existing wire/report schemas and IDs do not change.

One allowlisted projection carries the same producer facts through capture,
source-change, cleanup, cancellation and expiry decisions without promoting
execution or efficacy. Nil facts mean unavailable metadata, not no-start.
Unknown causes become UNSPECIFIED; raw errors, streams, paths and tokens are
never copied. A requested Start is not proof of container creation or Node
execution. No new gate, option, admission or relaxation of cleanup is introduced.
