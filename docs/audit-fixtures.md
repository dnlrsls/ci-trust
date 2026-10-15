# Independent audit expectations (T7 preparation)

Nine owned input repositories and semantic manifests are ready for review under
`internal/audittest/testdata/`. **This is not completion of issue #7.** No analyzer,
rule evaluator, public CLI, fixture command, or GitHub action was run to author
these semantic expectations. Local public-binary serialization candidates now
exist; **independent local acceptance passed**, while portable execution proof remains
unmet. Capturing output did not redefine the semantic requirements.

## Review path

1. Read each `repo/` input directly, then its sibling `expected.json`.
2. Check the scoped conclusions and forbidden claims against S4/S5/S7 and Log L6
   in `odd/tasks/mvp-completion.md`, not against implementation output.
3. After #6, bind the approved semantics to independently reviewed whole-report
   canonical byte oracles for human and JSON audit/report output.

| Case directory | Independent requirement | Expected rules beyond offline required-check UNKNOWN |
| --- | --- | --- |
| `correct-config` | Literal Linux/Windows/macOS configured generic TEST paths; no declared-scope gaps | None |
| `omitted-job` | Go marker, echo-only workflow, no configured test invocation in that workflow | `CIT_DECLARATION_UNKNOWN` |
| `malformed-workflow` | Source-backed partial-analysis diagnostic; no inferred jobs or executed checks | `CIT_DECLARATION_UNKNOWN` |
| `a-misleading-e2e` | A: E2E name and echo do not establish E2E coverage | `CIT_DECLARATION_UNKNOWN` |
| `b-skipped-validation` | B: pull_request excludes test; push satisfies the same literal event comparison | `CIT_STATIC_OMISSION`, `CIT_DECLARATION_UNKNOWN` |
| `c-dynamic-matrix` | C/dynamic: unresolved computed matrix and runner; no invented combinations or definite platform absence | `CIT_DYNAMIC_PATH` |
| `d-non-blocking` | D: job continue-on-error is non-blocking, not mandatory | `CIT_NON_BLOCKING`, `CIT_DECLARATION_UNKNOWN` |
| `e-incomplete-platform` | E: declaration names exactly all three platforms; Linux-only go test matrix yields Windows/macOS configured gaps | `CIT_PLATFORM_CONFIGURED_GAP` for Windows and macOS |
| `f-unlinked-command` | F: npm test links exactly to node --test; declared lint remains unlinked, not globally unexecuted | `CIT_COMMAND_UNLINKED`, `CIT_DECLARATION_UNKNOWN` |

Every case requires `CIT_REQUIRED_CHECKS_UNKNOWN`. Finding presence alone does not
mean process failure or trusted CI. Generic `go test` / `node --test` establishes
bounded TEST intent, not unit/integration/E2E completeness. No fixture needs a
package install, subprocess, workflow execution, network, or GitHub mock. The npm
manifest is passive declaration data, not a request to fetch ESLint. Its `counter`
is a passive sentinel containing `0`, not proof that a mutation detector was driven.

## Manifest contract (not the report schema)

Each case owns `repo/` input and a sibling `expected.json` outside the audited
repository. Manifests are independently written requirements, **not analyzer
snapshots or generated goldens**. `case_id` matches the directory; `requirements`
records spec coverage; `sources` lists existing repository-relative input paths.
`observations` records semantic state, scope and field-level provenance.
`required_rule_ids` and `forbidden_rule_ids` assert membership constraints, not a
complete finding order/count oracle. E explicitly requires one gap per named
platform. `unknowns`, `forbidden_claims` and `limitations` are normative uncertainty
boundaries. State labels here express meaning, not new public report enum values.

Public audit requirements in every manifest are prospective: exit 0 and JSON
stdout containing exactly one schema-valid document. Per foundation CLI contract
and Log L13, partial-analysis diagnostics belong in both report data and sanitized
deterministic CODE/message stderr lines. C and malformed originally left those exact bytes UNMET; their diagnostic
semantics and separate byte candidates were independently accepted before those
presentation fields were explicitly bound. Cases without diagnostics require empty
stderr. Absence of the optional declaration is an UNKNOWN observation, not an
operational failure. For A, B, D, F and omitted-job, the independently required
`CIT_DECLARATION_UNKNOWN` is accompanied by the existing `DECLARATION_UNKNOWN`
diagnostic; L13 requires its exact `CODE: MESSAGE` stderr line. Earlier empty
stderr entries were prospective author assumptions, not quoted user strings;
they were explicitly reconciled with that higher-level diagnostic contract
before any byte-oracle capture, without changing semantic rules/states/sources.
This correction was made from the CLI contract before output binding,
not by replacing semantic expectations with analyzer results. Such diagnostics do not mean the audit cannot
produce a valid report. Human output must preserve the same
semantics, without a green trust label. `observed: false` and `exact_rendering:
UNMET...` explicitly prohibit treating these requirements as recorded CLI results.
These manifests do not fix exact stdout bytes, source line spans or serialized
order. Separate candidate files below bind those bytes without changing the
independent rule/state/provenance requirements.

The optional root declaration follows L6: strict
`{"version":1,"platforms":["linux","windows","macos"]}`. No README-derived policy
is assumed. Missing declarations mean UNKNOWN, not platform failure. Invalid or
unsupported declarations, CLI platform overrides and precedence are later focused
contract cases, not silently claimed as covered by this small corpus.

## Boundaries reviewers should protect

- Configuration, conditional eligibility, execution and enforcement are distinct.
  All historical execution and remote enforcement remain UNKNOWN here.
- B compares explicit push versus pull_request contexts. Static exclusion does not
  establish a historical GitHub skip. It does not imply a push omission.
- C has a valid declaration but unresolved runners. Declaration alone cannot fill
  matrix combinations or justify definite missing-platform findings.
- E's absence conclusion is closed only over the supplied literal push workflow
  and declared test-platform comparison, not over all possible execution elsewhere.
- D's non-blocking setting does not reveal actual remote branch protection.
- F's absence of a workflow link does not prove global non-execution. A's job name
  does not classify echo as E2E validation.

## Local serialization bindings — independently accepted, external CI pending

`cmd/ci-trust/e2e_test.go` exercises all nine cases through an owned public
executable initially built from source `167d095b5da816a7c296b1e2deb8d27915ddf239`.
Correction `624a6ffd6e9ad100fd7f3f3ee57f5cbda336be65` preserves decisive job
condition/non-blocking field anchors and connects inherited STEP omission to its
governing job evidence. B/D JSON and human bindings were regenerated only after
new field-specific assertions failed, then independently rechecked; the other
seven cases and all stderr bindings stayed unchanged.
Each case has three candidate files **outside** its audited `repo/`:
`audit.json`, `audit.human`, and `stderr.txt` (27 files total). Root `capture.txt`
records the runtime checkpoint and local acceptance. Review `mv99t3xt-t-4e08`
and B1-only recheck `mv9a3vtc-u-5fhj` accepted all nine local cases. These byte
bindings are regression oracles subordinate to the independent semantic
requirements, not remote-portability receipts.

Before any candidate write, every case must pass independent required/forbidden
rule membership, bounded configuration/UNKNOWN checks, schema/reference validation,
source-anchor bounds and case-specific scope/provenance gates. Human bytes must
match a separate formatter reference implementing the published `docs/cli.md`
layout from validated data; that reference does not classify findings. Every
finding, evidence record, limitation and UNKNOWN remains visible. Copying a wrong
report into a candidate does not bypass these gates.

Each run copies passive inputs to a spaces/Unicode `t.TempDir` path, adds a zero
counter if absent, and snapshots every directory/file and file byte. Audit JSON
repeatability, human output, and rejected format/arguments preserve the snapshot.
Then only that temporary root is renamed to a detached sibling: the original path
is asserted absent while saved-file and stdin reports reproduce both formats and
stderr exactly. Detached bytes/counters and saved report bytes remain unchanged.
This demonstrates no source-path dependency for these reports, not process/network
monitoring or an absolute security guarantee. No fixture command is executed.

The C candidate binds `unresolved-matrix` then `unresolved-runner`; malformed binds
`DECLARATION_UNKNOWN` then `malformed-yaml`. Every diagnostic uses L13's exact
`CODE: MESSAGE` line. After independent review confirmed these diagnostics against
the source inputs, the prospective manifest stderr fields were explicitly bound
to that accepted formatting contract. No rule/state/source requirement changed. Correct-config and E have empty stderr; A/B/D/F/omitted-job use the
parent-resolved declaration diagnostic.

Local commands (both passed; one Windows host only):

```sh
go test -count=1 ./cmd/ci-trust -run '^TestPublicAdversarial' -update
go test -count=1 ./cmd/ci-trust -run '^TestPublicAdversarial'
```

The update flag writes candidates only after **all nine** semantic/presentation
and immutability gates pass. Always rerun without update and independently review
the resulting bytes against the original input/semantic requirements. Before an
authorized recapture, update `captureSource` to the actual runtime-source checkpoint
and reset local acceptance; the update path deliberately writes PENDING metadata.
Initial
harness failures exposed the prospective stderr-contract mismatch; there was no
preimplementation behavior RED for the already implemented CLI.

## Remaining issue #7 work

- Preserve the independent local acceptance and field-specific provenance tests;
  future candidate changes require renewed scoped review, not blind recapture.
- Preserve public-binary whole-byte, isolated immutability, schema, determinism
  and source-absent report checks as the integrated CLI evolves.
- Obtain observed Linux, Windows and macOS public-binary results. One host's unit
  suite is not portable proof, and auditing workflows is not executing them.
- Preserve the observed [driven demo and self-audit](audit-demo.md). The later
  Gentleman-Programming/engram study stays opt-in and revision-pinned with generic
  adapters; no upstream access or writes are part of this preparation.

## Prepared portable proof path (not yet observed)

The proposed `.github/workflows/test.yml` runs owned project tests and vet on
`ubuntu-latest`, `windows-latest` and `macos-latest`, with read-only repository
permissions and ten-minute job bounds. Actions are pinned to immutable commit
IDs resolved from their official v6 tags; checkout does not persist credentials
and setup-go caching is disabled. Updating a pin requires rechecking official
identity and reviewing the action change, never a silent moving-tag update.
Go uses the supported 1.26 patch line; each job records the actual toolchain.

This workflow is development validation of CI Trust, not a collector of the
workflows being audited. It does not install or invoke any fixture validator.
The root support declaration is a prospective support target, not proof of
portable success. At this preparation checkpoint no run ID, successful job or
public CLI host result is claimed. Bind the final tested SHA to exact run/job
receipts only after #6 and the driven tests are integrated.

This corpus is intentionally not an exhaustive S4 conformance suite: needs edges,
permissions, timeout/concurrency inheritance, include/exclude bounds, unsupported
shells/actions, and cross-compilation need their own focused contract tests.
