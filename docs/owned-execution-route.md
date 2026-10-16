# Review one owned Docker execution route

The internal `controlled-verify` command now connects the pinned owned archive,
exact human review and existing runner. **No real Docker proof has been run.**
The owned-image workflow now has an explicit, default-off execution mode behind
the existing protected Environment and the command's independent exact-plan audit.
It has not been dispatched. Independent static source review and a bounded
recheck of its two corrected findings accepted the candidate for a source PR,
not real-runtime acceptance. Default preparation still cannot execute containers.

## What can be done without execution approval

Compile the owned Linux probe using Go **1.26.4**, `CGO_ENABLED=0`, `-trimpath`,
`-buildvcs=false` and `-ldflags=-buildid=`. Pack and independently inspect the
archive using `owned-image`. These operations must not execute the probe.

Then emit the full review envelope:

```sh
go run ./internal/controlledverify/cmd/controlled-verify prepare \
  --source "$SOURCE" --archive "$ARCHIVE" \
  --binary "$BINARY" --marker "$MARKER" \
  --revision "$GITHUB_SHA" --run-id "$GITHUB_RUN_ID" --run-attempt 1
```

`prepare` reads only owned data. It has no API, Docker or approval operation.
Its JSON includes the image identity tuple, live source snapshot, all four case
plans, resource limits, effects, revision, run, attempt, expiry and binding digest.
Source changes require a new plan and a new review.

## Protected workflow modes

`.github/workflows/owned-image-preparation.yml` remains manual-only. Its
`execution_proof` input defaults to `false`:

| Mode | Preparation | Protected dependent job |
| --- | --- | --- |
| Default `false` | Existing source-only archive/plan | Existing preparation binding check; no runtime |
| Explicit `true` | Pinned archive and full four-case execution plan | Reproduce exact source/image, then authenticate human review before any Docker access |

Both jobs check out the same exact revision without stored credentials and use
Go **1.26.4** without shared caches. Preparation and reproduction require a clean
checkout. Only the execution step receives the read-only review-audit token;
compilation and image packing do not. The execution job preserves its bounded
record and fixed diagnostics in the run summary even when the verifier fails.
There are no direct Docker commands, automatic approvals, policy writes or
provisioning commands in the workflow.

Source permits only the owner, this repository and `main`, with the exact
feature branch available only in opt-in mode. **The saved Environment is still
main-only**; adding source support does not change that policy or authorize an
exception. Do not dispatch this workflow without the separately approved
concrete operations plan.

## Execution boundary

Do not invoke `execute` until the concrete environment, image, commands, limits,
risks and cleanup have been explicitly authorized by the human. A CLI argument,
plan digest or workflow dispatch is never that decision.

The execution command checks the plan against the current
`GITHUB_SHA`, `GITHUB_RUN_ID` and `GITHUB_RUN_ATTEMPT`. A read-only GitHub audit
then requires the repository owner to have approved the protected Environment
with the exact comment `approve <binding_digest>`. It checks the configured
reviewer, no-admin-bypass policy, workflow, actor, source, run, attempt and expiry.
Environment approval without the exact comment is rejected. The command never
posts an approval or changes policy.

Production execution uses only `/usr/bin/docker`, the local Unix socket and an
existing empty private `SOURCE/.git/controlled-proof/docker-config` directory.
No caller-selected executable, Docker context, registry, arbitrary command,
parent environment or approval flag is accepted.

## Load and run sequence

| Boundary | Required observation |
| --- | --- |
| Before runtime construction | Compiled catalog, archive hash/content/config, live source and authenticated exact review agree |
| Before loading | Source remeasurement and decision lifetime still agree |
| Load | Independently inspected bytes go through stdin; no pull, build, pathname reopening or inherited stdin |
| After load | Docker reports exact image ID, Linux/amd64, exact single layer, no tags and known empty configuration defaults |
| Each case | Fresh single-use runtime, same approved fixed plan, source remeasured immediately before start |
| Cleanup | Existing runner verifies exact container ID and private owner label, removes only that container and independently lists its absence |
| Final source | Live snapshot equals the approved snapshot |

Loading has a total 20-second ceiling, including daemon prerequisites and final
inspection. Both client output streams share the existing 64 KiB bound and
200 ms pipe wait bound. Individual metadata calls retain their two-second cap.
Load output alone is not success. Failed or ambiguous loading is not retried;
the same runtime cannot bypass that failure through preflight.

The four cases are normal execution, a one-second wall timeout, cancellation
triggered by the exact observed owned start marker, and a 32-byte combined output
limit. The normal case retains the approved ten-second/resource profile.
Execution stops at an unexpected result or any unverified container cleanup.

## Image retention is deliberate

The reviewed archive has no tags. Its immutable image is **retained**, whether
newly loaded or already present. No image deletion authority is inferred from a
prior absence check: another process could have loaded the same image between
that check and this load. There is no image-remove or image-prune operation.

This retention is included in the plan effects and binding digest. It supersedes
the earlier draft promise to remove a newly loaded unused image; old plan bytes
cannot authorize the new plan. An ambiguous load records `MAY_BE_PRESENT`, not
verified absence or a successful load.

## Reading the evidence

`ExecutionRecord` preserves the binding, source/image identities, each raw runner
result and bounded tool output, observed cancellation marker, elapsed times,
container cleanup results and final source measurement. Source is remeasured on
every return after an image/container effect, including failed loading, timeout,
cancellation or failed cleanup. `SourceIntegrity` distinguishes `UNCHANGED`,
`CHANGED`, `UNVERIFIABLE` and pre-effect `NOT_MEASURED`; a post-effect snapshot is
retained when available, without overwriting the original failure reason.
`RECORDED` means only
that the fixed harness expectations were observed. **It is not a security PASS.**
`OOMEvent` preserves the final inspected cgroup OOM flag (`null` when unavailable),
not a claim that the main process died. Legacy `owned-v1` still rejects OOM. Only
`owned-proof-v1` can accept the deliberately tested child event, and only with a
successful attachment, main-process exit zero and a complete bounded owned report
with all twelve observations and increasing resource counters. Parent death,
missing/failed/ambiguous evidence and transport failure remain fail-closed. Actual
victim selection and parent survival still need the authorized real run.

Review the owned tool's per-check JSON separately, retaining its limitations and
`OBSERVED` / `INFERRED` / `UNVERIFIABLE` distinctions. A normal exit code alone
proves neither isolation nor validator efficacy. Tests use inert archives,
owned fake executables and mocked API/runtime boundaries, not real isolation.

## Still required before the first real proof

- Bind the statically reviewed source and reproducible pinned image candidate to
  the exact committed/published revision; source-PR readiness is not runtime approval.
- Validate the protected opt-in workflow from the exact reviewed commit without
  using an unreviewed source revision or merging merely to obtain permission.
- Confirmation that read-only workflow credentials can read the required review
  and Environment metadata; denied access must remain `UNVERIFIABLE`.
- Explicit human approval of any concrete provisioning or branch-policy change;
  the saved Environment remains main-only and unchanged.
- An authorized real run and review of its isolation, resource and cleanup data.

Do not merge merely to obtain execution permission, close #8 from mock results,
or proceed to #9. See [the probe observations](owned-probe-proof.md) for the
bounded operations and remaining limits of the tool itself.
