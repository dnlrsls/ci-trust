# MVP architecture: evidence before guarantees

Go is accepted. Build a small library with explicit data flow and a thin CLI,
not an agent platform. All components below are planned, not implemented.

## Component boundaries

| Component | Input | Output | Effects / owner |
| --- | --- | --- | --- |
| Snapshot reader | Explicit target path and bounded scan options | Root-contained files, source locations, snapshot identity and read diagnostics | Read-only; no repository subprocesses; #2/#3 |
| Discovery adapters | Supported manifests, lockfiles and tool markers | Candidate commands, definitions, links, ambiguities and provenance | No installation or script execution; #3 |
| Workflow analyzer | Local GitHub Actions YAML plus discovery inventory | Configured graph, events, dependencies, commands and unresolved paths | No actions, remote lookups or full expression emulation; #4 |
| Evaluation rules | Observations, graph and supported platform declarations | Findings with evidence, scope, certainty and limitations | Deterministic rules, not an LLM; #5 |
| Report model | Snapshot identity, observations, findings and diagnostics | Validatable versioned document | No hidden storage; schema owned by #2 |
| CLI / renderers | Arguments, Core results or an existing report | Human text or one JSON document, stderr diagnostics, exit status | Rendering cannot silently rescan; #6 |
| Controlled verifier | Current plan, explicit consent and approved isolated runtime | Bounded tool/config/fixture execution evidence and cleanup outcomes | Separate opt-in path; #8/#9, never called by audit |
| Recommendations | Audit evidence and supported templates | Explanations, proposed contents and reviewable diffs | Dry-run only, no apply; #10 |

Data flow: snapshot → discovery / workflow analysis → evaluation → report →
renderer. Controlled verification supplies separate observations; recommendations
consume evidence rather than creating a second policy engine.

Core owns observations and rules, not prompts, terminal interactions or process
exit. The CLI owns argument validation, consent presentation and output routing.
Future agent integration can invoke the CLI or consume its JSON without changing
the Core. No public Go API is promised until contracts are designed and tested.

## Evidence invariants

Configuration, execution, outcome, efficacy and enforcement are independent:

- Declared scripts or YAML establish configuration, not EXECUTED.
- Known static ineligibility is not evidence of a historical skipped run.
- PASS/FAIL are execution outcomes, not observation states or trust scores.
- A passing invocation proves only its documented verified scope.
- Pre-test runner failure does not establish a product defect. Cause attribution
  needs evidence, not just exit status; unsupported attribution remains unknown.
- Required checks and actual branch/ruleset enforcement stay UNKNOWN offline.
- Controlled lint sensitivity does not prove project enforcement or a CI run.

[Issue #2](https://github.com/dnlrsls/ci-trust/issues/2) owns schema v1, including
snapshot/SHA when available, environment, tool/config/check identity, provenance,
states, causes, redaction and compatibility. Preserve CONFIGURED, EXECUTED,
SKIPPED, UNKNOWN and UNVERIFIABLE concepts; finalize explicit known non-execution
and valid state/outcome combinations there. This design does not freeze field names.

Equivalent snapshots/options must produce stable identifiers, ordering and JSON.
Separate volatile metadata. Keep evidence paths relative, redact secrets, expose
bounds and do not guess unavailable metadata.

## Safe static audit

Audit reads only within the explicit target. It never invokes hooks, scripts,
package managers, actions, validators or repository-controlled subprocesses.
Do not fetch submodules, resolve remote actions or follow symlinks outside the root.
Traversal and file-size bounds must be documented and surfaced as limitations.

Discovery identifies candidate tooling, not installed executables. Preserve raw
conditions and unknown action internals without executing expressions. Bound
literal matrices; unresolved expressions cannot become confirmed coverage.
Permissions, timeouts, cancellation and fail-fast describe configuration, not
observed effects.

Known malformed/unsupported inputs may produce a valid partial report with
diagnostics. If a valid report cannot be produced, use an operational error;
do not hide it behind a successful-looking partial report.

## Initial support targets

These are planned reference fixtures, not claims of implemented support.

| Surface | Initial target | Explicit boundary |
| --- | --- | --- |
| Go repositories | go.mod and supported literal Go build/test commands | No module download, command execution or complete shell interpretation |
| Node.js/npm repositories | package.json, supported lockfile markers and literal npm script links | No lifecycle hooks, install, dynamic JS/config execution or arbitrary plugins |
| GitHub Actions | Local workflow/job/step graphs, known event filters, literal bounded matrices, needs and source-backed settings | Dynamic expressions, remote/reusable/action internals stay unresolved unless specifically supported |
| Platform gaps | Machine-readable support declarations or explicit target-platform input | Not arbitrary README prose; exact declaration format belongs to #5 |
| Audit hosts | Linux, Windows and macOS | Native public CLI evidence required; cross-builds are not host validation |
| Controlled lint | One pinned ESLint adapter on owned clean/fault/restored fixtures | P1 only, approved configuration and provisioned isolation; unsupported cases fail closed |

Runner OS, target OS and cross-compilation targets remain distinct. A Linux-only
test configuration can reveal declared Windows/macOS coverage gaps, not defects
on those systems. Unsupported stacks remain explicit rather than fabricated.

## Non-goals and open decisions

No database, web server, dashboard, MCP server, agent orchestration, token/session
telemetry, LLM acceptance decisions, automatic apply, dynamic CI planner, full
mutation testing or automatic Merge Queue administration.

A future read-only GitHub evidence collector could compare workflows with actual
runs/checks/protection policy. It is not required for the offline MVP; no remote
execution or enforcement facts are inferred in its absence.

Before implementation, #2 must stabilize the shared schema; #3/#4 must document
parser and scan bounds. #8 owns the threat model, consent binding and approved
runtime/host combinations before any verification execution. Go toolchain and
dependency versions are not selected yet. The maintainer's license choice blocks
licensed distribution.

## Design review for issue #1

| Acceptance area | Review evidence |
| --- | --- |
| Core boundaries and independence | Component table and data flow above |
| Language comparison | [Accepted Go ADR](decisions/0001-use-go.md) |
| Commands, examples, effects and exits | [CLI contract](cli.md) |
| Offline/read-only behavior | Safe static audit above |
| Initial adapters and host scope | Initial support table above |
| Repository conventions and licensing | [Contribution guide](../CONTRIBUTING.md) |

Next: [#2 evidence and finding contracts](https://github.com/dnlrsls/ci-trust/issues/2).
