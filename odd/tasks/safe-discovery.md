# Safe static discovery — issue #3
Objective: bounded read-only Go/npm inventory; no execution, CLI or remote evidence.
Branch: feat/safe-discovery; baseline d4046f66e138d4d1f1c11280e6011b91573e2eee; runner Go 1.26; delivery auto-chain / feature-branch-chain (retain prior selected chain shape).

## Specs

### S1 — Authorized bounded loop (verbatim)
> porque no cerramos lo 2 issues que ya avanzamos? ahora vamos con el siguiente issue? te parece bien si te dejo en loop avanzando esto?

User selected:
> Completar #3 (recomendado)
> Implementar, verificar, revisar, corregir, publicar PR, mergear y cerrar #3; después parar.

### S2 — Issue #3 (verbatim)
## Problem
Available tools/scripts do not establish CI invocation, and executing repository scripts during discovery is unsafe.

## Motivation
Build a safe, repeatable inventory that later analyzers can trace to source files.

## Scope
- Read supported manifests, lockfiles, and validation-tool markers; resolve literal Go commands and npm script references.
- Inventory lint/build/test candidates and source chains. Preserve independent monorepo roots, ambiguity, unsupported stacks, and dynamic commands.
- Document traversal bounds and exclusions.

## Non-goals
- No dependency installation, shell evaluation, hooks, script execution, network access, exhaustive language support, or general shell interpreter.
- No claim that discovered commands execute or provide comprehensive coverage.

## Acceptance Criteria
- [ ] Go and npm fixtures yield the expected tools, candidate commands, and manifest-relative evidence. Script availability is CONFIGURED, not EXECUTED.
- [ ] Literal npm references resolve to definitions; cyclic, missing, computed, or unsupported references yield bounded UNKNOWN diagnostics without hanging.
- [ ] Mixed roots stay independent. Unsupported stacks and malformed manifests produce explicit limitations rather than fabricated commands.
- [ ] A hostile script cannot create a sentinel file or increment a counter; discovery does not start subprocesses and leaves input files unchanged.
- [ ] Symlinks cannot escape the target root. Traversal/file-size limits are enforced and visible; repeated scans produce identical inventories.

## Technical Considerations
Use static parsers and a small adapter boundary. Tool detection is not proof of installation. Never fetch submodules or follow external paths.

## Dependencies
- https://github.com/dnlrsls/ci-trust/issues/1
- https://github.com/dnlrsls/ci-trust/issues/2

## Testing Strategy
Table-driven fixture repositories: Go, npm, mixed roots, absent/malformed manifests, reference cycles, hostile scripts, symlinks, bounded input. Assert exact inventory/diagnostics and no writes/subprocesses.

## Priority
P0

## Suggested Size
M

### S3 — Repository workflow (verbatim)
> recuerda que tenemos desactivado RDD para este repo
> entremos en loop, review a los PRs, correcciones, y luego merge, nada mas

### Implementation decisions / assumptions (not additional product scope)
- New package internal/discovery provides typed intermediate inventory, NOT a competing report schema or CLI. Future report wiring remains #6; consumers must use #2's validated evidence boundary.
- Public boundaries: ReadSnapshot(path string, options Options) Snapshot; Analyze(snapshot Snapshot, options Options) Result; parent composes Scan(path string, options Options) Result.
- Parent owns types.go, scan.go, scan_test.go and docs/discovery.md. Reader worker owns snapshot.go/snapshot_test.go. Analyzer worker owns manifests.go/manifests_test.go. No overlapping writers or module/dependency changes.
- Reader uses Go os.Root confinement, skips all symlinks and non-regular entries, excludes .git, node_modules, vendor, .venv, dist, build and .next directories. Never executes or imports target code. Paths/diagnostics are root-relative; error messages omit raw host paths, OS errors and payloads. Unsupported hosts js/plan9 fail closed rather than claiming containment.
- Defaults: depth 16; visited entries 4096; per recognized file 1 MiB; aggregate bytes 8 MiB; npm reference edges 256 and script recursion depth 16. Zero selects defaults; negative or excessive options reject explicitly. Directory enumeration must be incremental and globally bounded, including skipped/unrecognized entries; no full unbounded ReadDir call. Known-marker files alone are read, regular files only, with a bounded limit+1 reader and post-open checks. Diagnostics make every limit/skip visible. No atomic live-filesystem snapshot claim.
- Reader recognizes go.mod, package.json, package-lock.json, npm-shrinkwrap.json, yarn.lock, pnpm-lock.yaml, .eslintrc/.eslintrc.json, eslint.config.js/.mjs/.cjs, .golangci.yml/.yaml and unsupported-stack markers pyproject.toml, requirements.txt, Cargo.toml, pom.xml. JS/config marker presence is not evaluation. No lockfile installation or remote resolution.
- Analyzer treats each manifest directory as an independent root (including nested/mixed roots). Go module detection yields Go tooling plus conventional build/test CANDIDATES with explicit convention origin and UNKNOWN invocation, not observed/configured CI execution. Literal Go commands explicitly present in supported npm scripts can be CONFIGURED.
- npm package.json parsing is strict/bounded JSON, no JS/config execution. Supported literal script leaf commands are deliberately narrow: go test/build, eslint, tsc, vitest, jest, node --test; literal npm run <name> / npm test references may form bounded chains (simple && sequences only if every term is supported). No environment assignments, shell expansions, pipes, redirection, command substitution, interpreter execution or general shell parsing. Unsupported/dynamic text produces safe UNKNOWN diagnostics without echoing script text. Names need not imply execution or installed tools. Sources and reference chains stay at their owning root. Reject duplicate/ambiguous JSON keys and malformed scripts instead of choosing silently. Unknown fields unrelated to discovery need not invalidate a package manifest.
- Tool markers/lockfiles are availability/configuration evidence only; unsupported lock managers/stacks and orphan markers remain limited/unknown. Stable IDs use existing evidence.StableID; inventory is deterministically sorted; no clocks/randomness/host prefixes in results. No full secret-detection guarantee.
- Forecast: ~1000–1600 authored lines including hostile/boundary fixtures and tests; retain coherent behavior/tests, disclose actual slice sizes rather than strip tests to meet ~400 heuristic. Keep the feature/tracker branch chain previously chosen, no Gentle AI labels/workflows. Commit/push/PR/merge/issue closure are explicitly authorized for #3 only. Do not start #4.

## Tasks
- T1 | S1-S3 | inline: shared typed boundary and defaults | done | fe51894b0d744e4348f0934d2bae7ca8d94d5cf1; defaults RED exit 1, GREEN exit 0 (0.342s); issues #1/#2 CLOSED.
- T2 | S2 | delegated parallel: confined reader + static adapters, disjoint surfaces, each heavier than a worker start | done | reader 2c82ef6 (404 lines), adapters cd40945 (694 lines); unit RED/GREEN; parent composed GREEN and full suite 0.614s/0.600s.
- T3 | S2-S3 | inline composition/docs/seam tests, independent high-risk verify and bounded correction review | in_progress | all four blockers independently PASS on recheck; final spot/commit pending; no RDD.
- T4 | S1-S3 | inline delivery: publish coherent PR chain, review, merge, close #3 and stop | pending | commits/PRs/readbacks pending.

## Log
L1: porque no cerramos lo 2 issues que ya avanzamos? ahora vamos con el siguiente issue? te parece bien si te dejo en loop avanzando esto?
L2: User selected "Completar #3 (recomendado)" with the explicit implement/verify/review/correct/publish/merge/close/stop description in S1. RDD stays off. No authority for #4 or unrelated work.
L3: Exact issue #3 read on GitHub; dependencies #1/#2 delivered in #11 and #12–#15. ADMIN capability verified. One close attempt per #1/#2 succeeded and exact CLOSED readback preserved unrelated labels. Comments reference merged delivery and verification limitations. No license choice made.
L4: Prepared feat/safe-discovery from fast-forwarded main d4046f6; clean. os.Root local Go documentation confirms confined opens and Windows device-name restrictions; it does not prevent mount traversal or make a live filesystem atomic. Standard library only; symlinks skipped and unsupported containment hosts fail closed.
L5: Writer trigger is real parallelism: confined filesystem reader and pure static adapters have independent public boundaries and disjoint files. Parent defines shared types first and will not duplicate worker units. Each worker reads this document until Log, self-reviews linked S#, observes real RED/GREEN via explicit-source unit tests, and returns evidence. One full seam check follows both workers; independent verify covers all five issue ACs and hostile/boundary cases.
L6: Both writers launched together on feat/safe-discovery-01-reader with narrow allowed surfaces, no commits or RDD. Parent authored seam tests and compiling Scan stub on disjoint files; explicit-source RED exited 1 (0.408s) for absent roots and lost limit/missing-root diagnostics. No composed GREEN claimed until both worker boundaries exist. Analyzer contract clarified: leaf commands in Candidate.Command, entry script in Source.Symbol, ordered entry-to-definition Chain, SCRIPT/CONVENTION origins, lowercase lint/build/test kinds. Unknown composite branches cannot emit a partially resolved entry as complete.
L7: Reader returned completed, final unit exit 0 (0.496s), parent spot exit 0 (0.552s). Adapter returned completed, final unit exit 0 (0.497s). Both observed genuine assertion RED/GREEN; no full suite during concurrent writes. Reader commit 2c82ef6, adapter commit cd40945, current third child feat/safe-discovery-03-integration. Coherent review slices are 404/694 lines, not hidden by minification.
L8: First composed package run failed because parent seam expected unknown scripts to be omitted and reader bounds to populate the Limitations slice. Neither extra representation rule was requested: explicit UNKNOWN entries with empty command plus safe diagnostics preserve uncertainty. Updated seam expectations to assert exactly five resolved/conventional + two unresolved records, never a guessed command. Focused package GREEN 0.574s. Full suite then exposed existing fixture byte-mutation failure: fresh Windows checkout converted contract JSON to CRLF. Raw mutation target absent; LF-normalized input contains it. Extended existing LF attributes from golden-only to all evidence JSON fixtures, retaining byte-exact expectations. No committed JSON content changed. Full suite GREEN: discovery 0.614s, evidence 0.600s; vet/format/whitespace passed.
L9: Worker limitations retained for independent review: special-file/injected-read-error branches and unsupported-host behavior not directly exercised; no atomic live snapshot or mount exclusion claim. Intermediate Source zero positions mean unknown and cannot be emitted as v1 source positions. Parent docs state these boundaries; no CLI or remote/cross-host proof claimed.
L10: Read-only risk routing over committed reader/adapters reports high security risk (native medium, explicit item-2 raise), RDD off, reviewDue false; independent verify required, no native review lifecycle started. Fresh staged checkout proof: 25 Go/JSON files LF with core.autocrlf=true; 22 local link targets pass. Verifier mv96bhmf-b-kg9j returned FAIL despite all commands passing (verbose package 0.649s; full discovery 0.580s/evidence 0.544s; vet/format/whitespace pass), zero skips and Windows symlink test passed. Four blockers were originally static: direct Analyze collection/diagnostic bounds, Unicode Fields token fabrication, composite sibling-chain attribution, inconsistent reader/analyzer hard caps.
L11: Parent reproduced ALL four through exported boundaries: go test -count=1 ./internal/discovery -run '^TestReview' exited 1 (0.501s), including unsafe documents and copied diagnostics bypassing MaxEntries, output overrun, NBSP/em-space/NEL/vertical-tab/form-feed becoming CONFIGURED, wrong nested per-leaf chain, reader accepting analyzer-invalid 65537/4097 caps. Root causes: pre-bound copy/sort and path validation before budget check; unbounded metadata/output append; Unicode Fields semantics; flattened recursive chain; duplicated option validation. One correction batch introduced shared normalization, constant-time collection preflight, bounded bytes/metadata/output (4096 records), ASCII token validation, global edge accounting and per-leaf chains. Existing literal ASCII and exact budget boundaries remain covered; tests assert rejection output/state, deterministic results and input immutability. Focused GREEN exit 0 (0.491s); full GREEN discovery 0.600s/evidence 0.533s; vet/format/whitespace pass. Only blocker-ledger recheck follows, not a fresh sweep.
L12: Recheck mv96pkr0-c-4c3e returned overall PASS for all four original blockers; focused regressions exit 0 (0.502s), full suite discovery 0.609s/evidence 0.537s, no stderr. No source/scratch writes, remote or lifecycle actions. Prior verbose run established zero skips and Windows symlink proof; unsupported-host and special-file/concurrent-replacement/CLI/cross-host limits remain explicit. Parent final spot and work-unit commit precede publication. No new broad sweep or extra acceptance criteria.
