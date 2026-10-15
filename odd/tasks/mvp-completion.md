# Complete remaining CI Trust MVP issues
Objective: finish accepted remaining issue outcomes through verified delivery, preserving every evidence and safety boundary.
Branch: feat/mvp-workflow-analysis; baseline c51c1dacec23d67637df723105fce833a3ab635a.

## Specs
### S1 — User authorization (verbatim)
sigamos el mismo ciclo con todos los issues, te lo dejo, yo me voy a dormir, buenas noches 

avanza, estamos dirigiendo nuestras herramientas a salir del human in the loop y hacer algo mas parecido a lo que hace Lauren Tan

### S4 — Issue #4: [P0] Analyze GitHub Actions jobs, matrices, conditions, and command provenance
## Problem
Workflow and job names do not explain which commands are configured to run, on which platforms, or under which conditions.

## Motivation
Expose the configured path from triggers to validation commands and its static-analysis limits.

## Scope
- Parse .github/workflows/*.yml and *.yaml; extract triggers/filters, jobs, needs, conditions, runners, literal matrices with include/exclude, steps, uses/run, shell, working-directory, and continue-on-error.
- Link supported literal invocations to discovery definitions with workflow/job/step provenance.
- Bound matrix expansion and retain unresolved expressions, reusable workflows, and external/composite actions as limitations unless local support is documented.

### Additional bounded static analysis
- Extract workflow/job permissions, timeout-minutes, concurrency/cancel-in-progress and matrix strategy fail-fast with source provenance. Preserve declared values, known inheritance and unresolved/default semantics without guessing remote policy.
- Analyze pull_request, push and merge_group separately, including known event filters, event-conditioned jobs, conditions that may exclude validations and needs dependencies that condition eligibility.
- Preserve job/step continue-on-error as non-blocking configuration, not proof of required-check enforcement.
- Distinguish configured jobs, conditional eligibility, static ineligibility under explicitly known conditions, unresolved dynamic conditions and unknown historical execution.
- Support only documented bounded literal/event cases. Preserve unsupported expressions as unresolved; explicitly defer interpreter-heavy cases rather than building a full expression interpreter or GitHub scheduler emulator.

## Non-goals
- No workflow/action execution, arbitrary remote fetching, full expression emulation, historical run inference, or branch-protection inference.

## Acceptance Criteria
- [ ] Triggers named on parse correctly rather than becoming a YAML boolean. Graph fixtures preserve needs edges, filters, source locations, and job/step identity.
- [ ] Literal matrices including include/exclude yield exact configured combinations; computed runners/matrices and expansion limits produce UNKNOWN without invented coverage.
- [ ] Unconditional, statically false, and unresolved conditions stay distinct. continue-on-error is non-blocking configuration, not enforcement proof. Static false does not prove a historical skipped run.
- [ ] A supported npm test invocation links to the exact manifest definition. Unreferenced scripts remain unlinked; unknown action internals are not treated as validation.
- [ ] Malformed YAML, missing needs targets, unresolved reusable workflows, and unsupported shells produce source-backed diagnostics. Analysis never starts a subprocess or changes files.

- [ ] Workflow/job permissions, timeout-minutes, concurrency/cancel-in-progress and strategy fail-fast appear with source locations, scope and unresolved-value limitations.
- [ ] Fixtures analyze pull_request, push and merge_group separately and identify possible configured validation gaps under known event contexts, including conditions and needs edges.
- [ ] Configured, conditionally eligible, statically ineligible and dynamically unresolved jobs remain distinct. Historical execution stays UNKNOWN without run evidence; static conditions never establish an actual GitHub skip.
- [ ] Conditions remain source expressions without executing untrusted expressions. Dynamic matrix combinations stay UNKNOWN, not confirmed coverage.
- [ ] Non-blocking jobs are not mandatory merge guarantees. Required status checks and actual branch-protection/ruleset enforcement remain UNKNOWN without remote evidence; workflow YAML alone cannot establish them.
- [ ] No remote calls or action execution occur. Interpreter-heavy cases are documented limitations and deferred, not silently approximated.

## Technical Considerations
Configuration describes potential behavior, not actual execution. Preserve expressions without evaluating untrusted code. A Linux runner does not imply Windows/macOS test coverage.

Required-check selection and actual protection rules are a separate remote-evidence dimension. This offline analyzer does not collect them. Cancellation, fail-fast and dependencies describe potential configured paths, not historical outcomes.

## Dependencies
- https://github.com/dnlrsls/ci-trust/issues/1
- https://github.com/dnlrsls/ci-trust/issues/2
- https://github.com/dnlrsls/ci-trust/issues/3

## Testing Strategy
Golden graph fixtures for trigger filters, conditions, literal/dynamic matrices, expansion bounds, needs, working directories, npm indirection, reusable/external actions, YAML edge cases, and no execution.

Add source-backed fixtures for workflow/job permission scope, literal/dynamic timeouts and concurrency, cancel-in-progress, fail-fast, event-specific eligibility, non-blocking jobs/steps and dependency-conditioned paths. Assert static conclusions separately from historical UNKNOWN; unsupported expression evaluation stays out of scope.

## Priority
P0

## Suggested Size
M

### S5 — Issue #5: [P0] Evaluate validation coverage and evidence-backed platform gaps
## Problem
Declared support and named jobs can hide missing validation paths or imply guarantees that evidence does not support.

## Motivation
Produce actionable, explainable gaps without equating absence of evidence with definitive failure.

## Scope
- Evaluate static analysis, unit, integration, E2E, build, platform, and security categories only when evidence supports classification.
- Compare machine-readable support declarations or explicit target-platform input with configured validation paths; retain declaration provenance.
- Identify unlinked commands, missing configured platforms, unresolved paths, omissions, and non-blocking checks. Separate configuration, execution, and enforcement.

## Non-goals
- No arbitrary README prose as authoritative platform policy, job-name-based coverage, complete functional coverage estimates, trust score, or remote policy inference.

## Acceptance Criteria
- [ ] A Linux/Windows/macOS declaration with only Linux test configuration yields Windows/macOS configured-coverage findings citing both declaration and matrix.
- [ ] Absent declarations produce UNKNOWN, not invented platform failures. Relevant dynamic runners or conditions prevent unjustified definite absence claims.
- [ ] A job named E2E running echo establishes no E2E guarantee. Recognized test commands establish only documented bounded category evidence, never complete functional coverage.
- [ ] An available but unreferenced npm lint script is reported as configured/unlinked. Known omission, unresolved conditions, and continue-on-error remain distinct.
- [ ] Required checks remain UNKNOWN offline. Every finding includes rule/evidence/scope/limitations and distinguishes configured coverage from observed execution or enforcement.
- [ ] Cross-compilation does not count as tests executing on the target OS. Repeated rule evaluation on identical observations is deterministic.

## Technical Considerations
Use a small deterministic rule set over existing observations. Distinguish runner OS, target OS, and cross-build targets. Unsupported categories remain explicit.

## Dependencies
- https://github.com/dnlrsls/ci-trust/issues/2
- https://github.com/dnlrsls/ci-trust/issues/3
- https://github.com/dnlrsls/ci-trust/issues/4

## Testing Strategy
Table-driven positive/negative fixtures for declarations, dynamic matrices, misleading names, unlinked scripts, cross-builds, skips, non-blocking checks, and required-check UNKNOWN; assert exact rule IDs/evidence.

## Priority
P0

## Suggested Size
M

### S6 — Issue #6: [P0] Deliver read-only audit and report CLI with human and JSON output
## Problem
Users and agents need one public entry point exposing the same evidence and uncertainty without mutating their repository.

## Motivation
Deliver the first usable vertical slice: discover, analyze, evaluate, and explain an existing CI configuration.

## Scope
- Wire Core into ci-trust audit PATH (human default; --format json).
- Implement report --input FILE|- --format human|json with stdin/human defaults; render existing reports without rescanning.
- Provide help, version, deterministic output, explicit partial-analysis diagnostics, and portable path handling.

## Non-goals
- No hidden report storage, database, dashboard, network lookup, credentials, agent dependency, script execution, automatic fixes, or verify/bootstrap implementation.

## Acceptance Criteria
- [ ] Public audit invocation emits expected findings, relative evidence, limitations, and UNKNOWN required checks in human and JSON formats.
- [ ] JSON stdout contains exactly one schema-valid document without progress text. Diagnostics use stderr; exit codes follow the foundation contract. Findings alone do not imply process failure or trusted CI.
- [ ] report renders a saved file or stdin without the source repository. Invalid JSON/version and missing input fail explicitly and never trigger an implicit cwd audit.
- [ ] Help documents all implemented options, output/exit semantics, and no-execution behavior. Invalid arguments/formats return exit 2.
- [ ] Audit leaves tracked/untracked inputs and sentinel counters unchanged, starts no repository-controlled processes, requires no network, and is deterministic for a fixed snapshot.
- [ ] Integration checks cover spaces/Unicode paths and Linux/Windows/macOS behavior. Unavailable host verification is reported rather than assumed.

## Technical Considerations
Keep rendering separate from Core. Human reports show limitations instead of an aggregate green trust label. Bounded analysis limitations remain valid report data; operational inability to produce a valid result returns exit 1.

## Dependencies
- https://github.com/dnlrsls/ci-trust/issues/1
- https://github.com/dnlrsls/ci-trust/issues/2
- https://github.com/dnlrsls/ci-trust/issues/3
- https://github.com/dnlrsls/ci-trust/issues/4
- https://github.com/dnlrsls/ci-trust/issues/5

## Testing Strategy
Public binary golden tests assert exact exit/stdout/stderr, schema, stdin/file parity, deterministic output, input immutability, and no execution. Run portable cases on Linux/Windows/macOS.

## Priority
P0

## Suggested Size
M

### S7 — Issue #7: [P0] Add end-to-end audit fixtures and portable dogfooding evidence
## Problem
Plausible reports can still be wrong if expected results merely repeat analyzer implementation assumptions.

## Motivation
Make the MVP demonstrable using minimal independently understood repositories.

## Scope
- Create correct-config, incomplete-platform, omitted-job, malformed-workflow, misleading-job-name, and dynamic/unknown fixtures.
- Define expected observations/findings independently before implementation-driven golden updates.
- Exercise public audit/report on isolated copies across supported hosts. Document self-audit and a later opt-in revision-pinned Gentleman-Programming/engram study.

### Required adversarial audit fixtures
Define expected observations/findings and limitations independently before implementation-driven golden updates. Extend/reuse existing minimal fixtures rather than creating a large corpus.

| Fixture | Minimal input | Independent expected result |
| --- | --- | --- |
| A - Misleading E2E | A job named E2E only runs echo. | No functional E2E coverage attributed from the name or echo command. |
| B - Skipped Validation | A condition excludes validation for an explicitly known event. | Identify condition, event and static scope without inventing historical skips or results. |
| C - Dynamic Matrix | Combinations depend on a statically unresolved expression. | Unproven combinations remain UNKNOWN, not confirmed coverage. |
| D - Non-blocking Check | A job uses continue-on-error. | Record non-blocking configuration without claiming effective required-check enforcement. |
| E - Declared Cross-platform Support | A machine-readable declaration or explicit platform input names Linux, Windows and macOS; workflows configure tests only on Linux. | Report configured Windows/macOS coverage gaps with declaration/workflow provenance, not OS product failures. |
| F - Configured but Unexecuted | A validation tool/script is declared but not linked from analyzed workflows. | Separate availability/configuration/linkage from observed execution; no link does not prove it never ran elsewhere. |

These are offline audit fixtures, without arbitrary repository execution or complex GitHub mocks. Real-tool controlled execution belongs to #8 and #9.

## Non-goals
- No claim that fixture YAML proves actual CI execution, mandatory live network access, upstream engram writes, upstream-specific Core rules, or large corpus.

## Acceptance Criteria
- [ ] Each fixture names expected rule IDs, states, evidence, limitations, and public CLI exit/stdout/stderr behavior.
- [ ] The correct-config fixture has no configured gaps within its declared scope but retains UNKNOWN execution/enforcement facts when not observed.
- [ ] Omitted/malformed fixtures are never reported as executed checks. Dynamic configurations retain UNKNOWN instead of invented guarantees.
- [ ] Isolated end-to-end runs prove input bytes and sentinel counters unchanged, including rejected inputs.
- [ ] A documented demo audits a repository to JSON then renders that JSON. A later pinned engram case study uses generic adapters only and records its limitations.
- [ ] Portable cases have observed Linux/Windows/macOS results. Local unit tests never stand in for driven or remote CI execution evidence.

- [ ] A-F each have independently authored expected rule IDs where applicable, states, evidence, scope, limitations and exact public CLI exit/stdout/stderr behavior.
- [ ] A rejects name-only coverage; B separates static omission from historical skip; C preserves UNKNOWN for unproven combinations.
- [ ] D retains non-blocking configuration and UNKNOWN enforcement; E reports only configured gaps, not OS product failures; F does not fabricate execution or global non-execution.
- [ ] Inputs and expectations are minimal, reproducible and deterministic. Analyzer output is not the oracle; golden changes are checked against independently written expectations.
- [ ] No repository validators/actions, network or complex GitHub mocks are needed. Controlled real-tool scenarios remain in #8/#9.

## Technical Considerations
Fixture design can start after the evidence contract in parallel with adapters; final acceptance needs the integrated CLI. Avoid circular expectations generated by the analyzer itself.

## Dependencies
- https://github.com/dnlrsls/ci-trust/issues/1
- https://github.com/dnlrsls/ci-trust/issues/2
- https://github.com/dnlrsls/ci-trust/issues/6

## Testing Strategy
Public CLI golden tests, schema validation, fixture hashes/counters, three-host integration checks, and a reproducible manual demo. External case studies remain opt-in and revision-pinned.

Run A-F through public audit/report against isolated inputs, asserting evidence/state boundaries, exact output and source/sentinel immutability. These results are not execution evidence for the workflows being audited.

## Priority
P0

## Suggested Size
M

### S8 — Issue #8: [P1] Gate controlled verification behind consent and bounded isolation
## Problem
Executing validators may run arbitrary code, expose secrets, or exhaust resources; static audit must not silently become execution.

## Motivation
Establish a narrow fail-closed boundary before validator sensitivity probes exist.

## Scope
- Define explicit consent to a displayed plan containing target snapshot, adapter/tool identity, fixture/config scope, resource limits, and effects.
- Implement a first existing-runtime container runner for owned controlled fixtures, without writable host repository mounts, credentials, or network.
- Enforce wall-time, CPU, memory, process/output limits, cleanup, and cancellation. Separate harness and infrastructure outcomes.

## Non-goals
- No automatic audit execution, arbitrary repository scripts/actions/plugins, package installation, implicit image pulls, host-shell fallback, privileged containers, or custom sandbox.
- No absolute security guarantee or automatic runtime provisioning.

## Acceptance Criteria
- [ ] Without explicit consent to the current plan, no verification process/container starts and no repository file/counter changes. Snapshot/tool/image/limit changes invalidate consent.
- [ ] Unavailable runtime or unenforceable promised limits return UNVERIFIABLE with infrastructure reason before execution; there is no unsandboxed fallback.
- [ ] Owned controlled probes cannot access host credentials/network, write source files, or escape allowed scratch space under a documented threat model.
- [ ] Timeout, memory/process/output bounds, cancellation, and normal termination produce bounded observable outcomes and cleanup evidence; infrastructure/harness errors are not validator failures.
- [ ] Pre-provisioned immutable tool-image identity is recorded. No implicit downloads or credential pass-through occur.
- [ ] Document supported runtime/host combinations, residual container risks, and missing proof explicitly. Linux/Windows/macOS hosts may require an installed Linux-container backend.

## Technical Considerations
Use an established isolation runtime; do not build a sandbox from scratch. Validate arguments without shell interpolation. P0 must work without any container runtime.

## Dependencies
- https://github.com/dnlrsls/ci-trust/issues/1
- https://github.com/dnlrsls/ci-trust/issues/2

## Testing Strategy
Fake-runner tests for denial/stale consent/preflight; real isolated-runtime tests for no network/secrets, source immutability, resource bounds, cancellation, cleanup. Use owned fixtures only; report unavailable isolation proof.

## Priority
P1

## Suggested Size
M

### S9 — Issue #9: [P1] Verify lint sensitivity with clean and controlled-fault fixtures
## Problem
A configured linter might accept a known violation, while a nonzero exit might be a harness failure rather than successful validation.

## Motivation
Demonstrate narrowly scoped validator sensitivity without running unknown repository code.

## Scope
- Implement verify --check lint via the consented isolated runner.
- Start with one documented ESLint adapter, pinned tool version, clean fixture, one violated built-in rule, and restored control.
- Record command/config/tool identity, diagnostics, scope, and clean/fault/restored outcomes. Refuse executable or unsupported repository configs/plugins.

## Non-goals
- No full mutation testing, arbitrary validators/plugins, repository edits, historical CI execution claims, or implication that tool-level sensitivity proves project lint enforcement.

## Acceptance Criteria
- [ ] Clean passes, controlled fault fails with the expected lint diagnostic, and restored control passes; only this sequence supports sensitivity evidence.
- [ ] A no-op validator accepting the fault is a sensitivity failure. Missing tool, startup crash, timeout, or sandbox denial is a distinct harness/infrastructure outcome.
- [ ] Reports distinguish tool-level from configuration-specific evidence. Tool-level fixtures never upgrade project configuration to verified or prove a CI job ran.
- [ ] Unsupported/executable repository configs or plugins are not loaded and produce explicit UNVERIFIABLE. Unsafe configuration cannot trigger arbitrary code.
- [ ] Without valid consent and runner preflight no lint process starts. Source bytes/counters remain unchanged after success, failure, denied consent, and invalid arguments.
- [ ] JSON matches the evidence schema. Human output/help state the controlled fault, diagnostic, supported adapter/version, consent requirements, scope, and limits.

## Technical Considerations
Only owned minimal fixtures under approved adapter/config run. A project that cannot be safely/factually represented remains UNKNOWN/UNVERIFIABLE. Match a specific diagnostic; any exit 1 is not sufficient.

## Dependencies
- https://github.com/dnlrsls/ci-trust/issues/2
- https://github.com/dnlrsls/ci-trust/issues/3
- https://github.com/dnlrsls/ci-trust/issues/8
- https://github.com/dnlrsls/ci-trust/issues/6

## Testing Strategy
Public CLI clean/fault/restored tests plus no-op/wrong-diagnostic/crashing/missing tool, stale/denied consent, resource exhaustion, unsafe config, and unchanged source. Real pinned-tool tests run only in the supported sandbox.

## Priority
P1

## Suggested Size
M

### S10 — Issue #10: [P2] Generate evidence-backed CI recommendations and dry-run workflow diffs
## Problem
Users need actionable improvements without granting a tool authority to overwrite workflows or branch policy.

## Motivation
Offer minimal reviewable remediation with a verifiable explanation for each proposal.

## Scope
- Implement bootstrap --dry-run with explicit target and human/JSON plan output.
- Generate minimal GitHub Actions suggestions for supported stacks using declared platforms and audit evidence.
- Emit proposed contents/diffs, finding references, preconditions, conflicts, and limitations.

## Non-goals
- No apply mode, writes, automatic workflow replacement, branch-protection changes, remote mutations, dependency installation, guessed commands/platforms, or dynamic scheduling.

- No autonomous LLM-based CI generation; recommendations remain evidence-backed, reviewable and dry-run-only.

## Acceptance Criteria
- [ ] An empty supported fixture produces a minimal proposed workflow with evidence-backed explanations; unsupported stacks/unresolved commands yield limitations, not invented commands.
- [ ] Existing workflows remain byte-identical. Conflicting proposals require review and are never overwritten or merged automatically.
- [ ] Every recommendation references source findings/evidence and states which bounded gap it addresses and which guarantees it does not establish.
- [ ] Identical inputs yield identical plans/diffs. Supplying the proposed configuration in a separate isolated fixture makes a second dry-run propose no duplicate equivalent changes.
- [ ] Human/JSON output includes dry-run-only status, paths, preconditions, and unresolved limits. Invalid options preserve files/counters and follow the CLI exit contract.
- [ ] Generated YAML parses under the workflow analyzer and matches the proposed supported commands/platforms. Help explicitly states there is no apply behavior.

## Technical Considerations
Reuse audit observations/rules rather than a second policy engine. Keep templates small/versioned and permissions minimal. Document a verifiable action-version pinning policy.

## Dependencies
- https://github.com/dnlrsls/ci-trust/issues/2
- https://github.com/dnlrsls/ci-trust/issues/3
- https://github.com/dnlrsls/ci-trust/issues/4
- https://github.com/dnlrsls/ci-trust/issues/5
- https://github.com/dnlrsls/ci-trust/issues/6

## Testing Strategy
Golden diffs for empty/existing/conflicting/unknown fixtures, YAML reanalysis, traceability, determinism, isolated two-pass idempotence, and public CLI file/counter immutability.

## Priority
P2

### Sequencing rationale
First demonstrate correct auditing of existing pipelines through P0 and #7 E2E evidence. Generating CI before that risks reproducing unsupported guarantees. P2 changes scheduling, not functional scope: retain all existing criteria and dependencies, evidence-backed recommendations and reviewable diffs, mandatory dry-run, and no automatic apply. #7 is the audit-validation milestone; no remote collector is required.

## Suggested Size
M

## Tasks
- T4 | S1,S4 | delegated workflow analyzer; parent review/correction/recheck/delivery | done | #20–#22 MERGED; main79c3a37540de9aab43bbb6db86d2baeb1dca3355 exact reviewed tree116c562; issue#4 CLOSED, labels preserved
- T8 | S1,S8 | delegated consent/isolation runner, independent of workflow/rule packages | blocked | foundation safely checkpointed on feat/mvp-isolated-runner in .git/issue8-worktree; awaits existing supported runtime/proof before production backend can be attested; #8 remains open
- T5 | S1,S5 | delegated pure coverage/report producer, integrated on corrected #4 | in_progress | a4873cc on feat/mvp-coverage-report from main79c3a3; first review FAIL B1–B4; correction COMPLETE with RED/GREEN; blocker-only recheck mv98uh1m-p-0mjl PASS; parent final spot/commit and delivery next
- T6 | S1,S6 | delegated CLI service plus parent real binary integration, isolated | in_progress | service c8a0a00 source ready; integrating in registered issue6-worktree on a4873cc; true terminal detection/public binary tests still pending
- T7 | S1,S7 | independent expected fixtures before goldens, public CLI and three-host evidence | pending | depends on T6; remote host proof required
- T9 | S1,S9 | lint sensitivity adapter/CLI after T6/T8 | pending | real pinned-tool proof requires supported existing sandbox and consent
- T10 | S1,S10 | dry-run recommendations after T5/T6/T7 | pending | no apply, no unknown commands/platforms
- TD | S1,S4-S10 | per-issue conventional PR/review/correction/merge/closure; never close unmet criteria | pending | preserve labels, SHA/tree readbacks; no RDD

## Log
L1: Original user request verbatim:
sigamos el mismo ciclo con todos los issues, te lo dejo, yo me voy a dormir, buenas noches 

avanza, estamos dirigiendo nuestras herramientas a salir del human in the loop y hacer algo mas parecido a lo que hace Lauren Tan
L2: Authorization supersedes the previous stop-after-#3 boundary for accepted issues #4–#10 only. Continue without per-transition permission, including reversible implementation choices, tests, conventional review/corrections, publication, merges and closure. RDD remains OFF. Do not auto-consent to security execution, provision a runtime/image, choose a license, publish releases, or claim unavailable proof. A criterion blocked by unavailable runtime/host proof stays OPEN while independent authorized work continues. Parent owns commits/delivery; workers never publish or commit. No Gentleman-specific labels or oversized-diff minification.
L3: Parallel route: workflow parser/graph (#4) and owned-fixture isolation runner (#8) are independent heavier-than-start units, disjoint internal/workflows and internal/runner surfaces. Source/seam mapping delegated read-only; parent will spot-check at most one mapped location. Existing Go/native Windows tests are not portable-host or container proof.
L4: Read-only map mv975jn2-d-74x3 complete; source confinement/admission remains in discovery, workflow DTO/parser separate, rules/CLI consume it, runner independent. Branch prepared from remote main c51c1da. Tracker committed 904d25c. Parallel writers running: T4 mv97b6yy-e-ejjl (workflows/docs/go.mod/go.sum); T8 mv97bpvl-f-qmq6 (runner/docs only). Neither verified or complete. Parent owns discovery admission and seam test. command -v docker returned no executable: no real-isolation PASS or #8/#9 closure may be claimed. Visible task groups the two parallel implementations; #8 proof/delivery remains pending. Rule IDs and public CLI text must be explicit before independent golden authoring.
L5: T8 first worker PARTIAL: trusted Runtime seam only, no production Docker backend/catalog and no real isolation proof; positive fake lifecycle tests do not close S8. One bounded correction mv97fswq-g-9854 addresses admitted immutability/active-cancellation/race/format gaps, not runtime provisioning. Keep #8/#9 OPEN when proof unavailable and continue independent P0/P2 work.
L6: Reversible #5/#6 design chosen before expectation authoring: optional root .ci-trust.json, strict JSON object {"version":1,"platforms":["linux","windows","macos"]}; unknown/duplicate keys/invalid versions/platforms become bounded UNKNOWN declaration diagnostics, not invented support. Optional audit --platform may repeat known lower-case platform values (linux/windows/macos); explicit CLI values override file declaration and retain explicit-input provenance; unsupported CLI values are exit 2. No README policy inference. Stable rule IDs: CIT_PLATFORM_CONFIGURED_GAP (declared platform lacks configured test path only under closed literal/event scope), CIT_DECLARATION_UNKNOWN (absent/unsupported declaration), CIT_COMMAND_UNLINKED (available npm script not linked by understood workflow), CIT_STATIC_OMISSION (known event-context exclusion), CIT_DYNAMIC_PATH (unresolved matrix/condition/path), CIT_NON_BLOCKING (continue-on-error), CIT_REQUIRED_CHECKS_UNKNOWN (offline enforcement). These are configuration observations/findings, never execution/absence/enforcement proof. A generic go test/node --test/jest/vitest command establishes bounded generic TEST intent only, not inferred unit/integration/E2E completeness. Bootstrap reuses these rules; names and echo establish no coverage. Independent fixtures must precede analyzer-driven output/goldens.
L7: Parent snapshot admission observed assertion RED: workflows test exit1 (0.525s), declaration test exit1 (0.512s). ReadSnapshot admits only exact root workflow YAML and root .ci-trust.json under existing confinement/byte/entry budgets; Analyze excludes workflow YAML from marker interpretation (including pnpm-lock.yaml collision). Workflow seam GREEN0.496s and whole discovery0.639s before declaration addition. Both fixture families assert source/counter immutability and unchanged generic manifest inventory. Final combined focused GREEN0.513s and discovery suite GREEN0.648s. Admission/docs work-unit committed separately; dependency/workflow files not staged.
L8: T8 local correction mv97fswq-g-9854 settled PARTIAL: immutability, active cancellation/independent cleanup, accepted-byte count, synchronized overflow/content and formatting fixed. RED accepted8/retained2, GREEN runner1.417s and native race2.344s, zero stderr. Each suite 7 fake starts/7 cleanup calls, all rejected/unavailable cases zero starts; zero real Docker processes. Production backend, trusted image/catalog and real resource/isolation/cleanup proof still OPEN. Moved ONLY untracked, settled worker-owned runner/docs files into registered same-clone .git/issue8-worktree, branch feat/mvp-isolated-runner from904d25c, checkpoint commit; no partial runner code enters offline #4 PR. Continue independent #4/#5/#6/#7/#10; don't close #8 or #9 or infer consent. Independent fixture author mv97keio-h-a7vt running BEFORE analyzer/rule golden outputs; only owned inputs/semantic expectations, not claimed E2E proof.
L9: Workflow writer mv97b6yy-e-ejjl settled PARTIAL for incomplete per-rule TDD history and outstanding formatting; S4 AC1–11 assertions present. Parent formatted workflow Go files and ran focused GREEN0.492s; full seam suite discovery0.700s/evidence0.604s/workflows0.514s, vet/whitespace PASS. Do not claim per-criterion historical RED: only initial TestEventConditions stub RED and later event/missing-needs assertions were observed. Source is not independently verified yet. Discovery admission commit7fd05cf (127+/4-); runner checkpoint48b86a5 isolated from offline PR.
L10: Independent fixture author mv97keio-h-a7vt completed passive preparation: nine repositories/semantic manifests, 13 JSON parsed/21 source refs/one counter0, exit0/no stderr. NOT exact byte goldens/public CLI/immutability/three-host results. Parent structurally read guide and E expectation. Official action pins: checkout v6 d23441a48e516b6c34aea4fa41551a30e30af803, setup-go v6 924ae3a1cded613372ab5595356fb5720e22ba16. Actions enabled; no workflow/run or observed cross-host proof yet. One failed targeted metadata edit was atomic; corrected without touching source or starting its skipped later operations.
L11: T4 source checkpoint6969e39faf1b8127050b70211ecf56969bcaccce committed (1334+/3- for parser/dependency/docs/log unit, oversized coherent semantic unit disclosed; aggregate range1818 lines with tracking/admission). Independent passive fixture work separately committed054efa3664d51d7df59adeafdb8b896b82225d48 in registered .git/issue7-worktree (31files/270lines); no fixture data or partial runner enters #4 PR. Native read-only assessment high via securityitem2 escalation, RDDoff/reviewDuefalse; no review lifecycle. Independent verifier mv97v9ae-i-o17i running frozen #4 candidate; don't edit source until settled. A second atomic metadata edit failed on an unnecessary unmatched no-op AFTER successful assessment/launch; no repeated verification or lifecycle call is needed. Correct only metadata. Main workspace code remains frozen and clean except this task log.
L12: Prepared (not published/executed) issue7-worktree CI commit on top of independent fixture054efa3: source-pinned checkout/setup-go official v6 commits, permissions contentsread, persist-credentialsfalse/cachefalse, ten-minute jobs on three GitHub-hosted OS labels, verbose owned tests/vet and observed go-version. Added prospective root declaration and scoped LF attributes for all audit fixture bytes. Passive configuration has no meaningful RED; staged whitespace check PASS. No actual run/job/portable proof claimed. This is isolated from frozen #4 and will only be integrated with public CLI/driven fixture tests.
L13: Independent next units proceed on isolated same-clone worktrees from6969e39 while #4 source stays frozen: pure audit/rules+schema producer in .git/issue5-worktree and service-injected CLI/rendering in .git/issue6-worktree. They share only existing evidence contract and function interface, not edit surfaces. Fixed seam: audit.Options{Platforms []string}; audit.Scan(path string, options Options) ([]byte,error), audit.Analyze(snapshot discovery.Snapshot, options Options) ([]byte,error); bytes MUST come through evidence.Encode, no schema change/fake source anchors/execution. cli.Run(args []string, stdin io.Reader, stdout,stderr io.Writer, services Services) int; Services{Audit func(string,[]string)([]byte,error), Version string, InteractiveInput bool}. CLI imports evidence, NOT audit; parent cmd/ci-trust binds real audit.Scan only after both units self-review and #4 corrections. Error diagnostics go stderr; schema diagnostic records remain in report and are also emitted as sanitized deterministic CODE/message lines to stderr, with no JSON stdout pollution. The earlier independent fixture prospective empty-stderr assumption for malformed/dynamic analysis must be revised from this foundation CLI contract before final golden binding, never copied from analyzer outputs. Positive report data, findings and UNKNOWN still exit0; invalid invocation2, operational failure1. No native consent or active probe route added.
L14: Settled independent #4 ledger FAIL: B1 unsupported always() + statically false prerequisite became definite static omission (public probe reproduced); B2 U+00A0 fabricated npm linkage (public probe reproduced); B3 incompatible include with equal tag value overwrote axis=b source9 with11 (static trace). Single correction mv983qik-j-8uce now COMPLETE: public Analyze tests genuinely RED for all three, then focused GREEN0.687s/full workflows0.512s, zero stderr, formatting/check PASS. Preserve unsupported condition UNKNOWN, validate separators before normalization, carry actual per-combination include sources. Supported/default conditions, ASCII spaces/tabs, harmless trailing newline and typed matrix behavior retained; inputs unchanged asserted. No broad new review. Blocker-only recheck mv9873i6-m-amk1 running now on stable source; no #4 publication until PASS. Parallel #5 mv984tvp-k-a0my and #6 mv985efy-l-2j2i running only in their isolated registered worktrees; parent will integrate corrected #4 dependency before final whole-slice checks.
L15: Blocker-only recheck mv9873i6-m-amk1 settled overall PASS on B1/B2/B3, exact UNKNOWN/link absence/per-combination source9/11 and input immutability confirmed. Focused review tests exit0 0.640s; full suite exit0 discovery0.711s/evidence0.607s/workflows0.552s, combined stream showed no warnings/errors. No writes or new sweep. Source is ready for parent final spot and commit, then minimal named-branch-chain publication/inside-out SHA/tree-pinned merge and explicit issue #4 closure. #5/#6 writers remain isolated; runner/fixture sources are not part of this candidate. Do not add GitHub closing-keyword phrases to future-step PR text before manual closure guard.
L16: Source/correction9645ede committed; parent final full suite discovery0.697/evidence0.584/workflows0.543, vet/format/check PASS. Delivery job16 exit0: PR20 tracker356lines, PR21 admission131, PR22 graph/corrections1512 (coherent overage disclosed), all MERGED. Main79c3a37540de9aab43bbb6db86d2baeb1dca3355 exact reviewed tree116c562f97876475586631bb01e7ebc5dbd37647; issue4 explicitly CLOSED with posted comment and unchanged labels. No branch deletion/RDD lifecycle/CLI or cross-host claims. Script harmless log literal said four branches although the exact unit/ref arrays and verified receipts contain three; no extra branch was pushed.
L17: CLI worker layer partial for history/integration only, focused0.492/vet/format PASS; source componentc8a0a0029acc9cb252d5a7795b5e41d03e586d8b, 580+/109-. Parent first spot mistakenly ran in main without cli (setupfailure directorynotfound); && prevented staging/commit. Read-only cwd/branch/status diagnosis confirmed correct isolated issue6-worktree; explicit-cd spot GREEN0.493, source/docs committed only, copied spec preserved separately before branch integration. No unsafe retry or source mutation from incident.
L18: Audit worker settled partial for history/upstream dependency integration; initial contract assertion RED, final focused0.677/vet/format PASS, not per-rule RED. Sourcefd2a4fe (979lines) cherry-picked as a4873cc50c9cad0e06a4ac45a1d967615c5eff34 onto fresh feat/mvp-coverage-report from merged main79c3a3. Corrected-upstream parent focused0.676, full audit0.737/discovery0.667/evidence0.562/workflows0.493, vet/check PASS. Native read-only assessmenthigh securityitem2/RDDoff; independent #5 verifier mv98gsf8-n-5dvs running stable source. #5 not yet delivered. Parent integrates CLI in separate registered issue6-worktree froma4873cc, preserving old branch/source and handoff metadata (no reset/rebase/data loss). Needs actual public binary, output oracles/immutability and observed3hosts; not claimed.
L19: #5 verifier mv98gsf8-n-5dvs settled FAIL despite all five checks PASS (focused0.765; full0.778/0.684/0.580/0.588; vet/gofmt/diff silent). Exact P2 ledger: B1 same-line flow steps lose distinct IDs and report fails DUPLICATE_ID (public reproduced); B2 case-insensitive declaration struct decode violates exact-key/provenance contract (static); B3 multiple linked categories overwrite/drop supported intent (public reproduced); B4 oversized path scanned/normalized before length guard (static). Root tracker modifications during frozen review were parent-owned metadata only; audit/docs source stayed unchanged, no ownership decision remains. Correction mv98rquq-o-epj6 running on primary allowed audit/docs only; exported regression RED then GREEN required B1–B3, B4 static guard-order proof if no deterministic observable RED. No new broad review or #5 publication yet.
L20: Isolated CLI branch integrated e1654cd froma4873cc (full corrected #4 dependency, audited core pending fixes, CLI service); old metadata preserved35a7c60, no data loss. Parent entrypoint skeleton exit99 and public-binary report/help/invalid-argument/owned-sentinel tests added. Test setup Encode type mismatch corrected BEFORE any claimed RED, using independently owned empty evidence fixture, not analyzer output. Job19 currently runs expected assertion RED against exit99 stub; preserve its exact exit/output before GREEN. No valid audit or public result claimed yet. Job18 pinned true cross-platform IsTerminal dependency x/termv0.47.0 and x/sysv0.49.0, explicit current docs consulted; no char-device-only guess or host subprocess. This dependency is development tooling, not target execution/runtime provisioning.
L21: T5 correction mv98rquq-o-epj6 settled COMPLETE: same-line columns13/37 distinct stable IDs, exact declaration keys/overrides, category-specific TEST/LINT evidence both script orders, length-first 4096 guard. Actual RED B1–B3 then focused GREEN0.578/full0.752; reversed canonical anchor assertion sorted rather than imposing source order. B4 static operation order only; no timer/heap/novel sentinel proof. Blocker-only recheck mv98uh1m-p-0mjl running stable primary source, requested same B3 per-leaf reference review, no new sweep. Tracker-only changes confirmed parent-owned, no source drift or unresolved ownership.
L22: Public CLI job19 observed assertion RED exit1 (all11 cases sawstub99, test1.268s), not compiler/setup failure. Real entrypoint then binds actual audit.Scan/cli.Run and term.IsTerminal; job20 exit0 GREEN public report/invocation tests1.840s, CLI0.476s, vet silent. Exact stdout/stderr/exits and source/counter/report immutability covered for report/help/version/rejections, not yet positive-audit whole-report golden acceptance. Binary source/docs/dependency checkpoint on isolated feat/mvp-cli-integrated; no publication or #6 closure yet. Actual module pins x/term0.47.0/x/sys0.49.0, no target process or network by CLI. Core producer correction must be integrated before final driven fixtures/CLI review.
L23: T5 ledger-only recheck mv98uh1m-p-0mjl settled overall PASS for B1–B4. Focused exit0 0.716s; full exit0 audit0.880/discovery0.706/evidence0.542/workflows0.482, combined output no warnings/errors. B3 category capabilities statically traced to matching category-specific evidence/located manifest chains; no cross-category reference fabrication. B4 guard order static, 4096/oversized API behavior tested; no timer/heap or new Scan sentinel claims. No new sweep/writes/lifecycle/delivery. Parent final spot and correction commit precede one coherent coverage producer PR (honest measured size, retain tests/docs), SHA/tree-pinned merge and explicit #5 closure. #6 remains isolated pending corrected dependency, genuine audit oracles and observed3hosts.
