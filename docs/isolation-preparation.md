# Linux CI preparation: metadata, not isolation proof

The manually dispatched **Isolation preparation metadata (NOT proof)** workflow uses a GitHub-hosted Ubuntu 24.04 runner. It does not change a developer's Windows installation. The existing portable Go workflow is unchanged.

## What runs

`go run ./internal/cipreflight` invokes only Docker's built-in `version` and `info` commands against the explicit `unix:///var/run/docker.sock` endpoint. It forwards only PATH and a fixed locale, uses an explicitly prepared empty `.git/ci-trust-preflight-empty` configuration directory, never loads a user's Docker context, and discards raw client diagnostics. Each call has a three-second execution deadline, an additional 200 ms maximum pipe-drain wait, and one shared 64 KiB stdout-plus-stderr budget. Only stdout is retained; diagnostic bytes still consume the budget. No shell interpolation, target argument, credentials, container/image command, build, pull or fixture execution is supported.

The workflow explicitly creates project-owned scratch/configuration directories. The helper itself does not write files. Its public-process tests use an owned fake Docker executable; those tests are not daemon or isolation proof.

## Interpret the result

The JSON is internal preparation metadata, **not report-v1 evidence or an executable plan**. It always records `status: UNVERIFIABLE`, `isolation_proof: false`, and `executable_plan: false`, including when allowlisted metadata is captured. Reported CPU/memory/cgroup capabilities are daemon claims, not attested limits, physical host scope, or sandbox enforcement. Only a validated source revision and allowlisted version, OS, architecture and numeric/boolean capabilities are retained; raw names, labels, environment, context and error output are not published.

Missing runtime, malformed/oversized output, timeout, or nonempty client configuration also yield a valid UNVERIFIABLE result. Exit 0 means metadata collection/reporting completed, **not that isolation works**. `--help` prints usage with exit 0; any other argument produces usage on stderr and exit 2. Output failure exits 1. No approval flag exists.

## Owned archive and exact-plan review

A separate manual-only workflow, **Owned image archive and plan review (NO
execution)**, prepares source artifacts without invoking Docker. It is restricted
to `dnlrsls/ci-trust` on `main`; the portable test matrix and the metadata-only
workflow above are unchanged. This source change has not been published or run
on GitHub. Do not dispatch it without a separate authorized preparation run.

1. Check out the exact run revision without persisted credentials and use the
   existing pinned Go setup action. Build only the owned static Linux/amd64 probe;
   do not execute it on the host.
2. [Pack and independently inspect](owned-image.md) a bounded image archive with
   an explicit empty execution configuration. There is no Dockerfile conversion,
   default PATH admission, base image pull, daemon image load, or container start.
   Archive inspection binds the expected binary and literal marker; it does not
   prove that the compiled contents have received production catalog review.
3. [Prepare the complete fixed plan](controlled-plan.md), image identity, limits,
   effects, source revision and run/attempt binding. The snapshot is SHA-256 of
   `git archive --format=tar` for that revision, not a future runtime snapshot
   callback. Display the full JSON and image inspection in the job summary before
   the protected job becomes eligible.
4. The dependent job names the existing
   `ci-trust-controlled-verification` Environment and links the current run. After
   that configured boundary it consumes the exact upstream plan bytes and binding
   from job outputs, rechecks revision/run/attempt and the fixed profile, then
   reports only preparation validation. Denial prevents that job from running.

No artifact is uploaded, registry is contacted, Docker command is invoked or
`runner.Consent` is created. The archive exists only in the preparation job's
private scratch and is discarded with the hosted job. Expressions enter shell
steps through environment variables, never interpolated shell source. Token
permissions stay `contents: read`; no secrets or runtime credentials are passed.
Tests validate this workflow structure without executing Actions.

**Review is not execution consent.** The Environment must retain the independently
verified owner-reviewer, main-only and no-admin-bypass policy. This workflow cannot
by itself establish that the policy is unchanged or that a genuine human approved
this run. Future live use must independently observe policy and the approval
audit, not accept dispatch, a matching digest, or an agent-written boolean as
human presence. A rerun has a different run attempt and requires a newly bound
plan. No approval receipt is reusable as runtime authority.

Before real execution, separately authorize image import and independent daemon
configuration observation, review immutable catalog admission, integrate trusted
runtime snapshot remeasurement, and approve the actual current execution plan.
The existing two-second backend client ceiling remains unchanged; its ten-second
plan wall limit is only an upper bound. Real resource stress, cancellation,
network/credential denial and cleanup proof are not supplied by this workflow.

## Still missing

- Production catalog admission and reviewed immutable adapter/tool-image identity
  with independently observed daemon configuration. Backend source exists, but
  supported production runtime combinations remain unattested.
- A trusted human interface approving the exact displayed current plan. A dispatch event or agent-written boolean is **not consent**.
- Real runtime evidence for network/secrets/source isolation, resource limits, cancellation, cleanup, and then pinned-tool lint sensitivity.

Issues #8 and #9 remain open. Do not execute containers or provision images from this preparation result. A future execution proposal must identify the snapshot, tool/image, owned fixtures/configuration, resource limits and effects for explicit plan-bound human consent.
