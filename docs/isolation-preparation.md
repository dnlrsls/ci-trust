# Linux CI preparation: metadata, not isolation proof

The manually dispatched **Isolation preparation metadata (NOT proof)** workflow uses a GitHub-hosted Ubuntu 24.04 runner. It does not change a developer's Windows installation. The existing portable Go workflow is unchanged.

## What runs

`go run ./internal/cipreflight` invokes only Docker's built-in `version` and `info` commands against the explicit `unix:///var/run/docker.sock` endpoint. It forwards only PATH and a fixed locale, uses an explicitly prepared empty `.git/ci-trust-preflight-empty` configuration directory, never loads a user's Docker context, and discards raw client diagnostics. Each call has a three-second deadline and a 64 KiB output limit. No shell interpolation, target argument, credentials, container/image command, build, pull or fixture execution is supported.

The workflow explicitly creates project-owned scratch/configuration directories. The helper itself does not write files. Its public-process tests use an owned fake Docker executable; those tests are not daemon or isolation proof.

## Interpret the result

The JSON is internal preparation metadata, **not report-v1 evidence or an executable plan**. It always records `status: UNVERIFIABLE`, `isolation_proof: false`, and `executable_plan: false`, including when allowlisted metadata is captured. Reported CPU/memory/cgroup capabilities are daemon claims, not attested limits, physical host scope, or sandbox enforcement. Only a validated source revision and allowlisted version, OS, architecture and numeric/boolean capabilities are retained; raw names, labels, environment, context and error output are not published.

Missing runtime, malformed/oversized output, timeout, or nonempty client configuration also yield a valid UNVERIFIABLE result. Exit 0 means metadata collection/reporting completed, **not that isolation works**. `--help` prints usage with exit 0; any other argument produces usage on stderr and exit 2. Output failure exits 1. No approval flag exists.

## Still missing

- Production runner and reviewed immutable adapter/tool-image catalog.
- A trusted human interface approving the exact displayed current plan. A dispatch event or agent-written boolean is **not consent**.
- Real runtime evidence for network/secrets/source isolation, resource limits, cancellation, cleanup, and then pinned-tool lint sensitivity.

Issues #8 and #9 remain open. Do not execute containers or provision images from this preparation result. A future execution proposal must identify the snapshot, tool/image, owned fixtures/configuration, resource limits and effects for explicit plan-bound human consent.
