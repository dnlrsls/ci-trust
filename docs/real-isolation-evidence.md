# First owned isolation execution evidence

## Scope and provenance

Run: https://github.com/dnlrsls/ci-trust/actions/runs/38838143461

The execution job completed successfully on 2026-10-16 at 02:06:43 UTC.
GitHub API reads independently confirmed its revision, successful execution
step, and the exact approval comment from `dnlrsls`. The detailed execution JSON
reviewed below was supplied by the maintainer from the job summary; it was not
independently downloaded or cryptographically attested. There are no uploaded
run artifacts. Preserve the original summary when retaining this evidence.

This is a bounded owned-fixture observation, **not a security PASS**, proof of
validator sensitivity, or authorization for another execution.

## Identity and integrity

| Field | Recorded value |
| --- | --- |
| Revision | `66158aba6da5001803eabb6cbc7031d80ad14353` |
| Source snapshot, before and after | `307859d9f19d9b33ad882bc56ad278c76784a9e2cdd2356ba9b0601356f02d28` |
| Image | `sha256:2e926eafbfd9d90e2a697443b659adb7ea967cbd5b22691508064c0b7fe504bf` |
| Approval binding | `0f0a497068fc4b1e17568bd6bfb88d662dc8f19b2d9af0933017f1482ea5ed0c` |
| Source integrity | `UNCHANGED`, `SourceUnchanged=true` |
| Image disposition | `RETAINED_UNTAGGED` (deliberate, not container cleanup failure) |

The approval API comment matches this binding exactly. At the recorded
revision, the harness rechecks authorization, image identity, and live source;
it measures source again after effects. These are trusted harness assertions,
not independent host attestation.

## Observations

The normal case completed with exit code zero in approximately 2.855 seconds.
All twelve owned checks reported `OBSERVED`:

- UID 65534, zero capability sets, enabled no-new-privileges and seccomp filter.
- Read-only root marker rejected a write open with `EROFS`.
- Exclusive owned scratch write/removal succeeded; bounded quota write reached
  `ENOSPC` and cleaned up.
- Local network checks and a no-send route lookup agreed on unreachability.
- Named credential environment keys and harmless host sentinels were absent.
- CPU throttling counter increased from 0 to 19.
- Memory limit event counter increased from 0 to 41 after the bounded child.
- Process limit event counter increased from 0 to 20 after the bounded child.

`OOMEvent=true` records a cgroup OOM flag, not the parent's cause of death.
The exact runner source allows it for this proof only when attachment succeeds,
the parent exits zero, and complete bounded proof output is validated. The
normal record satisfies those reported conditions.

| Case | Outcome and interpretation | Elapsed | Cleanup |
| --- | --- | --- | --- |
| Normal | `COMPLETED`, exit 0 | 2.855 s | Verified |
| Timeout | `UNVERIFIABLE`, wall/cancellation reason; expected timeout behavior | 1.149 s | Verified |
| Active cancellation | `UNVERIFIABLE`, wall/cancellation reason; start-marker cancellation observed | 0.262 s | Verified |
| Combined output limit | `UNVERIFIABLE`, output limit exceeded; retained output exactly 32 bytes | 0.264 s | Verified |

All four cases report `ExpectationObserved=true`. The non-normal
`UNVERIFIABLE` outcomes are the intended fail-closed results of the harness
tests, not successful tool completion. Output ordering between stdout and
stderr is not relied upon. Durations are converted from Go nanoseconds and
include runner overhead.

## Acceptance boundaries and remaining work

### Issue #8 criteria mapping

| Criterion | Evidence | Scope |
| --- | --- | --- |
| Current exact consent; changes invalidate it; no effects on denial | `TestBoundaries`, `TestDigestBindsDisplay`, `TestNoDispatchEnvironmentOrAgentBooleanConsent`, `TestProofPlanRejectsExpiredChangedAndForgedInputs`, `TestCompositionNeverConstructsRuntimeWithoutImageAndHumanDecision` | Fake/API-mocked denial and mutation tests; real successful approval independently read from GitHub |
| Unavailable runtime or unsupported controls fail closed before execution | `TestUnavailableDocker`, `TestDockerPreflightRefusals`, `TestCompositionStopsOnAmbiguityDriftOrCaseFailure` | Fake runtime/capability failures; no unsandboxed fallback; zero container creation on refused preflight |
| Owned network/credential/source/scratch boundary under a documented threat model | Twelve normal observations, unchanged final snapshot; `TestDockerExactVectorAndOneShot`, `TestExactChildEnvironment`; [runtime contract](isolation.md) | Actual owned Linux observations plus fake policy/environment oracles; no absolute escape or exhaustive credential claim |
| Bounded normal, resource, timeout, cancellation, output and cleanup outcomes | Four real cases above; `TestFailedCleanupPreservesReasonAndSeparatesSourceStates`, `TestDockerExecute`, `TestExpectedOwnedChildOOMRequiresCompletedParentEvidence` | Real expected paths plus fake failure/ambiguity tests; infrastructure results remain distinct from validator results |
| Immutable owned image; no implicit pulls or forwarded credentials | Recorded image/catalog binding; `TestCatalogRejectsEveryChangedIdentity`, `TestLoadOwnedImageRejectsUnverifiedInputsBeforeDocker`, `TestLoadOwnedImageObservesAfterLoadAndNeverDeletes`, `TestDockerExactVectorAndOneShot`, `TestExactChildEnvironment` | Explicitly authorized archive load, post-load identity/config verification, `--pull=never`, empty/suppressed client environment |
| Supported host/runtime and residual limits documented | [runtime contract](isolation.md), [execution route](owned-execution-route.md), this report | Real coverage is the GitHub-hosted Ubuntu 24.04 Linux-container route only; no Windows/macOS backend execution claimed |

The combined real record and inert regression evidence support proposing
acceptance of the first owned route for issue #8. This is an audit
recommendation, not maintainer acceptance or a general security certification.

### Local regression verification

On 2026-10-16 UTC, 40 local files in the three audited packages, their command
subpackages, the two runtime documents, and Go manifests were compared with the
GitHub Git tree for the exact execution revision. Canonical Git blob identities
matched. The new evidence document is not part of that historical source tree.

The following uncached command passed all five packages on Windows, with
`GOTOOLCHAIN=local`, `GOPROXY=off`, and `GOSUMDB=off`:

```text
go test -count=1 ./internal/runner ./internal/controlledverify/... ./internal/controlledplan/...
```

These tests use owned inert/fake executables and injected runtime/API clients;
they do not access a Docker daemon or perform another real proof. The tests
check absent/wrong review, changed source/tool/image/limits, expiry and run
binding, unsupported enforcement, no runtime construction on authorization
failure, source/counter preservation, exact-ID cleanup and ambiguous failures.

This record supports the first real owned Linux-container execution, bounded
resource observations, timeout/cancellation/output handling, owned-container
cleanup, and unchanged source. Exact-revision review of
`internal/controlledverify/execute.go`, `internal/runner/runner.go`, and
`internal/runner/docker.go` was performed against the remote source.

It does not by itself exercise denial without consent, stale-plan rejection,
unavailable runtime, or every unsupported-limit path. Those require separate
source/test evidence; they must not be described as real cases in this run.
The network probe sends no packets, credential checks cover only named keys
and sentinels, and the container shares a trusted host kernel and Docker daemon.
No absolute escape resistance, exhaustive credential absence, other tools,
other host platforms, or validator efficacy is established.

Issue #8 was already closed at 01:56:30 UTC, before this execution. PR #56 was
merged at 01:56:29 UTC and its body contained a negated closing keyword. That
timing and wording are consistent with unintended GitHub issue closure, not
evidence acceptance. This audit did not change the issue state. Maintainer
acceptance and an explicit evidence publication decision remain separate.
