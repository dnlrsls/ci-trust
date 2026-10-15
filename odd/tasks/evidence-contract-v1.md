# Evidence contract v1 — issue #2
Objective: versioned evidence schema, independent fixtures and offline Go validation, not audit/CLI execution.
Branch: feat/evidence-contract-v1; delivery: feature-branch-chain (no publication authorized); runner: Go 1.26; baseline: a2db220ae2faf761678800fc609a4d9ac6343e25.

## Specs

### S1 — Authorized outcome
> sigamos con el issue 2, ya pasamos a merge

### S2 — Accepted issue #2 contract (verbatim remote snapshot)
## Problem
Configuration, actual execution, and inferred guarantees can be confused without explicit provenance and bounded evidence.

## Motivation
Give users and agents the same machine-checkable facts without arbitrary trust scores.

## Scope
- Define JSON schema v1 and valid/invalid fixtures for inventory, capabilities, workflow graphs, findings, diagnostics, limitations, and evidence.
- Represent CONFIGURED, EXECUTED, SKIPPED, UNKNOWN, and UNVERIFIABLE explicitly; keep outcome distinct from observation state.
- Define stable IDs, relative source locations, provenance, scope, categorical certainty, redaction, and deterministic serialization.

### Bounded evidence dimensions
Keep configuration, execution, outcome, efficacy and enforcement independent. A green invocation establishes neither functional coverage nor merge enforcement.

- Extend schema v1 minimally with repository/snapshot identity; commit SHA when available; execution OS/environment; tool, version and configuration identity; check identity; evidence provenance; validation scope; observation state; optional execution outcome; evidence-backed cause; limitations and uncertainty.
- Preserve CONFIGURED, EXECUTED, SKIPPED, UNKNOWN and UNVERIFIABLE. Explicitly represent known non-execution (NOT_EXECUTED or an equivalently documented field), distinct from an observed skip, unknown history, and unavailable/unrealizable verification. Document allowed combinations and their evidence requirements before freezing v1.
- PASS/FAIL are outcomes independent of observation state. Unobserved results remain absent/explicitly unknown. Static ineligibility under known conditions is not a historical SKIPPED execution.
- Allow Product failure, Test/fixture failure, Harness failure, Environment/infrastructure failure, and Unknown cause. A non-unknown cause requires supporting evidence references, provenance and limitations; exit status alone is insufficient.
- Missing metadata stays explicitly unknown/not applicable. Prefer small optional fields/extensions over a complex telemetry model.

## Non-goals
- No numeric confidence score, overall trust score, security certification, database, remote client, or UI.

- No agent sessions, costs, tokens or Gentle Shell telemetry. No additional persistence system; Core remains harness-independent.

## Acceptance Criteria
- [ ] Every finding has a stable ID, rule ID, evidence references, scope, explanation, limitations, and a categorical certainty backed by a documented evidence rule; no numeric score is emitted.
- [ ] Fixtures distinguish configured-only from observed execution, known omission from unresolved conditions, and UNKNOWN from UNVERIFIABLE. Validator failure is not a skipped check.
- [ ] YAML-only observations cannot produce EXECUTED or establish remote required checks. A controlled run proves only its named tool/config/fixture invocation, not that a CI job ran.
- [ ] Unsupported schema versions, invalid enums, dangling references, and missing required provenance are rejected with attributable diagnostics.
- [ ] Identical snapshot/options yield stable IDs, ordering, and byte-equivalent deterministic JSON. Volatile metadata is separated; secrets and absolute host paths are not exposed.

- [ ] A runner failing before the intended test executes cannot establish a product defect; insufficient attribution remains Unknown cause.
- [ ] Fixtures distinguish configured checks, observed executions, observed skips, known non-execution, unknown results and unavailable verification. PASS/FAIL never substitutes for these distinctions.
- [ ] Configured-but-unexecuted workflows never become EXECUTED. Successful tests demonstrate only the scope actually verified; efficacy and enforcement need independent evidence.
- [ ] Available snapshot/SHA, OS/environment, tool/version/configuration, check identity and provenance survive serialization; missing values remain explicit without leaking secrets or host paths.
- [ ] Fixtures cover evidence-backed product, test/fixture, harness and environment/infrastructure failures, plus ambiguous/nonzero-exit-only cases that remain Unknown cause. Cause provenance and limitations are retained.
- [ ] Equivalent inputs remain deterministic with volatile metadata separated.

## Technical Considerations
Separate evidence observations from evaluation rules. Record adapter/tool identity and snapshot provenance. Keep the schema minimal and version compatibility explicit.

## Dependencies
- https://github.com/dnlrsls/ci-trust/issues/1

## Testing Strategy
Schema validation and golden fixtures for every state, unsupported versions, invalid references, redaction, and deterministic serialization. All consumers reuse these fixtures.

Extend shared golden fixtures with pre-test runner failure, incompatible test/fixture evidence, environment failure, ambiguous causes, bounded-scope success and valid/invalid state/outcome combinations. No cause inferred solely from exit status; no execution inferred from YAML.

## Priority
P0

## Suggested Size
M


### S3 — Original user constraints (verbatim)
> CI Trust debe distinguir configuración, ejecución, resultado, eficacia y enforcement, sin confundir una dimensión con otra.
> No inferir ejecuciones reales a partir de YAML.
> Un runner que falla antes de ejecutar un test no produce evidencia de un defecto del producto.
> Un workflow configurado pero no ejecutado no puede reportarse como EXECUTED.
> Un test que finaliza correctamente solo demuestra el alcance efectivamente verificado.
> Los estados desconocidos se representan explícitamente.
> Toda clasificación de causa mantiene su procedencia y limitaciones.
> La serialización permanece determinista para entradas equivalentes.

### S4 — Review shape (verbatim user selection)
> Cadena sobre rama de feature (recomendado)

### Implementation decisions / assumptions (not additional product scope)

These fix the shared boundary for disjoint writers. Keep the schema small; no executor,
CLI, YAML parser, GitHub client, persistent storage or telemetry. If this design is
contradictory, report it instead of silently inventing another format.

- Module github.com/dnlrsls/ci-trust, Go 1.26, package internal/evidence.
- Schema is JSON Schema draft 2020-12 at internal/evidence/schema/report-v1.json.
  Schema identifier https://ci-trust.invalid/schema/report-v1.json is an identifier,
  never fetched. Embed it in Go, compile once per call/validator as appropriate;
  deny external resource loading. Select pinned pure-Go jsonschema/v6 for validation.
- Required root fields: schema_version ("1"), snapshot, inventory, capabilities,
  workflows, evidence, findings, diagnostics, limitations. Arrays are explicit,
  including when empty. No additional properties at defined record boundaries.
  No volatile timestamps/run IDs in the canonical document; optional external
  envelope is outside this contract. Unknown version is rejected.
- snapshot: {id, repository_id, commit_sha}; id is an opaque safe stable identifier;
  repository_id is a safe identifier or null, never a local absolute path;
  commit_sha is null or full lowercase 40/64 hex SHA. Unknown stays null.
- identifiers: nonempty [A-Za-z0-9][A-Za-z0-9._:-]*. Collections with IDs
  must reject duplicates. StableID(kind string, parts ...string) produces kind plus
  SHA-256 of unambiguous length-prefixed inputs. Caller includes snapshot/options,
  adapter/rule identity, relative locations and scope; never raw secrets.
- source: {path,line,column}; normalized root-relative forward-slash path,
  positive integer line/column. No absolute, drive, backslash, empty, dot or
  parent traversal segments. scope: {kind,subject}; kind is REPOSITORY, WORKFLOW,
  JOB, STEP or TOOL_FIXTURE; subject is a safe stable identifier.
- inventory entries: {id,kind,name,source,evidence_refs}; kind STACK, TOOL or CHECK.
  source is a source object. name is display text, not raw shell/log content.
  check_id fields refer to inventory CHECK entries. All references resolve.
- evidence: {id,check_id,state,scope,provenance,execution,cause,limitations}.
  states CONFIGURED, EXECUTED, SKIPPED, NOT_EXECUTED, UNKNOWN, UNVERIFIABLE.
  provenance: {kind,adapter,sources,basis}; kind CONFIGURATION, CONTROLLED_RUN,
  HISTORICAL_RUN, REMOTE_POLICY or DIAGNOSTIC. sources is array of source objects;
  adapter is identifier; basis SOURCE, ASSERTION, FIXTURE, RUNNER, INFRASTRUCTURE,
  EXIT_STATUS or UNRESOLVED. Configuration needs source provenance.
- execution is null, or {phase,outcome,os,environment_id,tool,fixture_role}.
  phase STARTUP or VALIDATION; outcome PASS, FAIL or UNKNOWN; os null or
  LINUX/WINDOWS/MACOS; environment_id safe identifier or null.
  tool: {name,version,configuration_id}; each identifier/text nullable when unknown.
  fixture_role null or CLEAN/FAULT/RESTORED (only meaningful for controlled runs).
- EXECUTED requires VALIDATION execution plus CONTROLLED_RUN/HISTORICAL_RUN
  provenance. NOT_EXECUTED may carry STARTUP execution with run/diagnostic
  provenance, or no execution (known scoped static ineligibility). All other
  states have null execution. SKIPPED needs HISTORICAL_RUN provenance. Static
  YAML facts cannot prove an actual historical skip. CONFIGURED uses CONFIGURATION.
- cause is null or {category,evidence_refs,limitations}. Categories PRODUCT,
  TEST_FIXTURE, HARNESS, ENVIRONMENT_INFRASTRUCTURE, UNKNOWN.
  Classified cause must accompany a failed invocation, have evidence refs and
  limitations. PRODUCT requires VALIDATION phase, never STARTUP.
  Supporting referenced evidence must have corresponding provenance basis:
  ASSERTION for PRODUCT, FIXTURE for TEST_FIXTURE, RUNNER for HARNESS,
  INFRASTRUCTURE for ENVIRONMENT_INFRASTRUCTURE. EXIT_STATUS alone cannot
  support any classified cause. UNKNOWN may have no supporting refs.
  Validation enforces a documented evidence rule, not truth of submitted evidence.
- findings: {id,rule_id,evidence_refs,scope,explanation,certainty,certainty_rule,
  limitations}. evidence_refs nonempty, certainty DIRECT/DERIVED/UNKNOWN,
  certainty_rule is a nonempty rule explanation/identifier, not a numeric score.
- capabilities: {id,check_id,dimension,status,evidence_refs,scope,limitations}.
  dimension CONFIGURATION, EXECUTION, EFFICACY or ENFORCEMENT;
  status PRESENT, ABSENT, UNKNOWN or UNVERIFIABLE.
  Resolved facts need nonempty evidence refs. ENFORCEMENT resolved only from
  REMOTE_POLICY, not YAML or controlled runs. EXECUTION PRESENT needs EXECUTED
  evidence for that check; ABSENT needs SKIPPED/NOT_EXECUTED, scoped appropriately.
  Controlled execution capabilities must remain TOOL_FIXTURE-scoped, never prove
  repository/workflow CI execution. CONFIGURATION resolved from CONFIGURATION facts.
  EFFICACY resolved only for controlled TOOL_FIXTURE evidence, matching check,
  tool/config/environment identity and CLEAN PASS / FAULT FAIL / RESTORED PASS
  for PRESENT, or all PASS for ABSENT. Same tool invocation PASS alone is insufficient.
  This describes admissible evidence only; no sensitivity runner or evaluation
  engine is implemented. Unknown/unverifiable facts do not invent execution.
- workflows: {id,source,events,jobs}; events unique strings; jobs:
  {id,needs,evidence_refs}. needs refers to jobs in that workflow; reject dangling
  needs, duplicate IDs and cycles. This is an evidence graph shape, not an analyzer.
- diagnostics: {id,code,message,source}; source object or null.
  limitations are arrays of authored safe strings (may be empty on known facts).
  No raw logs, captured environment maps, command outputs or arbitrary extensions.
- Validation diagnostics are {code,path,message}, path a JSON pointer, messages
  must not echo rejected input values or absolute schema/compiler host paths.
- Shared Go type Diagnostic {Code string, Path string, Message string} JSON tags
  code/path/message; parent owns its definition before delegation.
- Structural unit exports ValidateStructure(data []byte) []Diagnostic.
  Semantic unit exports ValidateSemantics(data []byte) []Diagnostic,
  Canonicalize(data []byte) ([]byte, []Diagnostic), StableID(kind string,
  parts ...string) string and Redact(data []byte, sensitive []string)
  ([]byte, []Diagnostic). Semantic unit must not depend on the schema unit so
  both can be verified independently. These are internal package boundaries,
  not a promised public module API. Parent adds Validate and Encode wrappers
  which compose structural/semantic checks before serialization.
- Canonicalization sorts object keys (standard encoder), ID collections by id,
  reference/set arrays lexically, workflow jobs by id; preserves ordered arrays
  not documented as sets. End with one newline, no mutation of input bytes.
  Scope/source objects and numeric values remain intact. No random IDs or clocks.
- Redact replaces explicitly supplied secret values/host prefixes in all JSON
  string values and keys with [REDACTED], longest values first; empty secrets
  ignored. Do not guess that a generic regex can detect every secret. Schema
  rejects host paths in location fields; producers must redact before adding
  repository content. Reject unsafe JSON in Encode; no raw payload in errors.
  Tests must prove provided secrets/absolute host prefixes do not appear in
  redacted canonical output. No claim of complete secret detection.
- Fixtures: independently authored valid examples for all six states,
  cause classes (including ambiguous exit-only UNKNOWN), startup failure,
  bounded passing scope, controlled clean/fault/restored and remote policy.
  A manifest gives explicit expected acceptance/rejection and diagnostic code
  for invalid cases. Cases cover bad version/enum, missing provenance, dangling
  refs, unsafe paths, YAML EXECUTED/skip, invalid outcome/state, unsupported cause,
  schema-invalid metadata, deterministic ordering and redaction.
  Schema validation alone handles shape; semantic validator handles cross-record
  facts and IDs. Both are necessary.

## Tasks
- T1 | S1-S3 | delegated parallel structural + semantic units, disjoint surfaces; inline shared composition | done | schema ae1b6cad68ea77336a655e0021044572260fe96d; semantics 443fe8ff1513aa155a85f1a7bcf46b08cd18bf14; unit/full GREEN and independent PASS.
- T2 | S1-S4 | inline integration, docs and full-suite seam check | done | integration 94159185f4073fc715e30c095c0f19f17c6f1b48; focused/full GREEN, vet/format/links and independent PASS.
- T3 | S1-S4 | independent verification, candidate assessment/readback and closure | done | work-unit 94159185f4073fc715e30c095c0f19f17c6f1b48 plus docs-only closure checkpoint on third child; independent PASS all eleven ACs, all reviewDue false, branch/publication readback observed.

## Log
L1: sigamos con el issue 2, ya pasamos a merge
L2: User selected "Cadena sobre rama de feature (recomendado)". Delivery strategy
feature-branch-chain; branch preparation/work-unit commits in scope. No push,
PR creation, merge, issue closure or approval authorized for this feature yet.
L3: PR #11 verified MERGED at a2db220ae2faf761678800fc609a4d9ac6343e25.
Issue #1 remains OPEN but its implementation dependency is merged. Fresh main
fast-forwarded; current branch feat/evidence-contract-v1; clean baseline.
L4: Forecast ~1,500-2,000 authored lines across schema, validator, tests and docs,
excluding repetitive fixture data. Review units: structural schema/fixtures,
semantic validation/serialization, integration. One honest slicing pass at
delivery; do not minify code or omit tests to meet a numeric budget.
L5: Routing reason is real parallel work: schema/structural tests and semantic
validation/serialization have disjoint edit surfaces and fixed shared boundary,
each heavier than starting a worker. They launch together. Parent owns module,
diagnostic type, tracking, integration wrappers and top-level docs. Generic
worker reads this document until Log; verifier reads the entire document.
L6: Native assessment/review runs at coherent work-unit/PR slice boundary,
not task checkbox. RDD is on. Metadata/redaction validation is high risk (item 2)
and dependencies are item 5: require independent verify. No audit CLI public
process exists yet; tests exercise the exported internal Go library boundary.
L7: Parent created go.mod (Go 1.26.0) and shared Diagnostic. Go proxy resolved
jsonschema/v6 v6.0.3 (requires Go 1.21); structural writer owns pinning it.
L8: Parallel writers launched together: structural unit mv941wz9-3-0uyc,
semantic unit mv941wwp-2-bnq1 (routing IDs are only session metadata). Their
Allowed edit surfaces are disjoint. Parent will not duplicate their unit work;
parent owns top-level integration tests/wrappers and documentation updates.
L9: Parent authored exported-boundary integration tests and an independent
empty-report golden; they have not run yet. README, CONTRIBUTING and architecture
are updated for the contract module, subject to final verification.
L10: Structural writer returned PARTIAL: meaningful RED exit 1 (invalid shape
accepted by stub), GREEN exit 0; final focused test exit 0 (0.477s), pinned
jsonschema/v6 v6.0.3 plus indirect x/text v0.14.0. Three report fixtures do not
satisfy the full acceptance matrix. Parent spot-read schema.go, manifest and
docs confirmed the gap. Do not mark T1 done on focused test success alone.
L11: One bounded correction batch launched as mv9477lg-4-b164 to the same
structural worker: complete classified-cause, controlled sensitivity, remote
policy and semantic-negative fixture expectations; add missing nested-boundary
negative tests and finish docs. Semantic sibling continues independently.
No extra verification pass or restarted implementation requested.
L12: Semantic writer returned PARTIAL: final focused test exit 0 (0.461s),
with prior REDs for missing newline, retained secrets, duplicate events/keys,
unknown fields/environment and unsorted refs. Initial semantic code preceded
its tests: historical test-first exception is recorded, not retroactively RED.
Coverage gaps remain for classified causes, remote policy and efficacy mismatches.
L13: Parent spot-read canonical.go and docs found efficacy refs treated as an
ordered exception despite the shared reference-set sorting contract. One bounded
semantic correction launched as mv949byw-5-th20: complete named coverage, match
roles via referenced records rather than input order, normalize equivalent source
number spellings without precision loss, and document exact-scope v1 policy.
Full structural checks remain in the parent composition, not duplicated in the
lower-level canonicalizer. Both correction results are required before T1 closes.
L14: While disjoint corrections run, parent can independently implement the
shared Validate/Encode composition. Its public-boundary integration tests use
an explicit same-package source list for meaningful RED against a compiling
stub, avoiding compilation of sibling files mid-write. GREEN and manifest seam
checks wait until both corrected units are stable. This is shared T1 work;
T2 remains integrated documentation, fixture matrix and full-suite verification.
L15: Parent composition RED observed with a compiling stub: explicit-source
contract tests exited 1 (0.375s). Unsupported version, trust score, missing
inventory and invalid SHA were incorrectly accepted; canonical newline and
no-output-on-rejection assertions failed. Implemented Validate (structure then
semantics) and Encode (full validation then canonicalize). Added manifest seam
and redaction-before-serialization tests. No GREEN claimed while sibling files
are still changing; gofmt passed on parent composition files.
L16: Structural bounded correction completed. Focused test exit 0 (0.489s),
no stderr; schema-boundary negatives and manifest now cover classified causes,
clean/fault/restored identity, remote policy and negative evidence claims.
Manifest is an independent base-file plus first-occurrence mutation protocol,
with exact distinct semantic_codes expectations, not semantic test results.
Parent adapted the integration test to this protocol and added full Encode
canonical set-order invariance. Structural fixture recipe now consistently
uses reference sets; semantic sibling was informed, not relaunched.
L17: Semantic bounded correction completed: RED exit 1 exposed efficacy-ref
ordering, malformed cause/scope and source 1.0 normalization; GREEN exit 0
(0.409s), final focused GREEN exit 0 (0.414s). Role-based refs, cause/policy/
efficacy mismatch coverage and conservative exact-scope policy are complete.
Historical initial-code-before-tests exception remains; no initial RED invented.
L18: First composed focused run exited 1 (0.531s): invalid-enum fixture expected
only INVALID_STATE but also lacked an inventory CHECK and configuration source,
producing UNKNOWN_CHECK and INVALID_PROVENANCE. Root cause was an unisolated
negative fixture, not a reason to change expected semantic codes. Rebuilt it
from the independently valid configured fixture with only CONFIGURED→GREEN.
Composed focused GREEN then observed; no mutation of expected verdict/code set.
L19: Integrated focused GREEN exit 0 (0.510s). Full suite go test -count=1 ./...
exit 0 (0.643s). go vet ./... exit 0; gofmt -l internal/evidence produced no
paths; direct filesystem checks covered untracked files (27 files, 19 local
Markdown links, zero newline/whitespace/JSON/link errors). No CLI, isolation,
Linux/macOS or remote CI behavior was exercised. All tests run on Go1.26.4
Windows/amd64. Functional self-verification is complete; independent verify
and native risk routing remain before completion claims.
L20: One honest slicing pass: schema/offline structural validation plus fixtures;
semantic admissibility/canonicalization/redaction; composed boundary and top-level
docs. Individual coherent slices may exceed the ~400-line heuristic. Do not
minify tests, strip comments or split a rule from its evidence to chase counts.
No destination size gate exists; report exact slice counts before publication.
Stage only intended issue #2 paths for assessment; staging is not publication.
L21: Native ASSESS over 27 staged paths / 2,574 changed lines returned nativeRisk
medium; explicit item-2 escalation makes verification risk high. Runtime writer
profile small. Plan requires writer self-verification plus an independent
verifier. Provider continuation: reviewDue false, reviewDueReason under_budget.
No INSPECT/START or native review is due for this candidate; do not invent one.
L22: Independent read-only verifier launched as mv94ig27-6-ff96, covering the
whole document, eleven issue ACs and all intended source/fixture/docs boundaries.
No new source edits while it runs. Work-unit commits remain pending verification;
metadata-only commits would not change candidate bytes but are not needed yet.
L23: Independent verifier mv94ig27-6-ff96 returned PASS for all eleven issue ACs,
S1/S3 and the documented S4 chain plan. Each authorized command ran once, exit 0:
full suite 0.597s, vet empty, format empty, staged whitespace empty; direct
filesystem check 27 files, 19 links, 7 JSON, zero errors. No product-source drift.
Parent confirmed L21-L22 log-only additions as expected bookkeeping, not source
changes. No severe blocker; no new correction batch or verification sweep.
L24: Two advisories corrected without dependency-version or behavior changes:
jsonschema/v6 is now marked direct in go.mod; fixture README no longer claims
that combined validation is pending. Initial semantic test-first exception stays
explicit. Before local commit slicing, parent reruns one required spot check.
Local chain boundaries: feat/evidence-contract-v1 tracker/work-plan base;
feat/evidence-contract-v1-01-schema (structural contract);
feat/evidence-contract-v1-02-semantics (admissibility/serialization);
feat/evidence-contract-v1-03-integration (full boundary, docs and evidence).
No branch is published and no PR/merge or issue closure is authorized.
L25: Parent post-advisory spot go test -count=1 ./... passed (0.558s).
Tracker/work-plan commit b0c31267d7054231c5ba7f9cf76cd41b6d02c638 assessed passive,
reviewDue false/passive. Child schema commit ae1b6cad68ea77336a655e0021044572260fe96d
contains 13 files / 863 additions; ASSESS base b0c3126 returns native medium,
verification high via item-5 escalation, reviewDue false/under_budget.
Child semantic commit 443fe8ff1513aa155a85f1a7bcf46b08cd18bf14 contains 7 files /
1,080 additions; ASSESS base ae1b6ca returns native medium, verification high via
item-2 escalation, reviewDue false/under_budget. Independent verifier PASS and
self-checks already cover the exact behavior in both slices. T1 is now complete;
T2 integration commit and T3 final evidence remain. The two slices exceed the
review heuristic; preserve coherent behavior/tests rather than hide the counts.
L26: Integration commit 94159185f4073fc715e30c095c0f19f17c6f1b48 on the third
child contains 7 files / 336 additions + 21 deletions (357 authored changes).
ASSESS base 443fe8f returns native medium, verification high via item-2 escalation,
reviewDue false/under_budget. Independent verifier already covered its exact
Validate/Encode code and tests; no additional native review is due or claimed.
T2 complete. T3 records the assessment/proof closure in a docs-only checkpoint;
final branch graph, range whitespace, remote nonpublication and clean worktree
readback remain. No product behavior changed after the independent PASS.
L27: Final metadata readback confirms the ordered local chain b0c3126 → ae1b6ca
→ 443fe8f → 9415918, based on merged main a2db220. Pinned-baseline range
whitespace check passed. No remote feat/evidence-contract-v1* branch exists;
issue #2 remains OPEN. Before the final docs-only checkpoint, the only working
diff was this task log; all product source/tests/fixtures/docs were committed.
All T1-T3 requirements and observed proof are complete locally. Final checkpoint
records these facts without changing product behavior; user-facing delivery
must report its returned commit and clean-worktree readback, not infer a merge.
L28: Remaining limitations are explicit, not failed checks: no CLI/analyzer/
executor, no remote CI evidence, no native Linux/macOS execution, no universal
secret detector, no chosen license. Initial semantic code-before-tests remains
a process deviation despite genuine later RED/GREEN. Review workload remains
863 / 1,080 / 357 changed lines for the three coherent code slices, plus tracker
and closure bookkeeping. Publication, draft tracker/child PRs, merge and issue
closure require user authorization; none was performed for issue #2.
