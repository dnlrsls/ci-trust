# Review the first owned isolation execution route

This is the **draft, no-merge integration tracker** for the source work on #8.
The feature is not real-isolation accepted: no Docker execution, workflow
execution approval or policy change is implied by reviewing or merging a child
into this feature chain. Do not merge the tracker into `main` or close #8 until
real evidence and the maintainer's separate authorization are available.

## Review order

Each child targets the immediately preceding branch. Tests stay with behavior;
cohesive security units may exceed the 400-line guide with the maintainer's
approved size exception. Never remove assertions or compress code to fit it.

| Child | Boundary | Deliverable |
| --- | --- | --- |
| 1 | `fix/issue8-docker-wall` | Owned attachment wall deadline; metadata deadlines unchanged |
| 2 | `feat/issue8-owned-proof` | Guarded owned probe observations and bounded native effects |
| 3 | `feat/issue8-exact-proof-plan` | Exact source/image/run/case/expiry/effects review envelope |
| 4 | `feat/issue8-verified-image-load` | Independently checked archive import, immutable image observation and explicit OOM evidence |
| 5 | `feat/issue8-exact-human-gate` | Pinned catalog/live source/authenticated exact human decision |
| 6 | `feat/issue8-proof-command` | Fixed four-case runner composition, internal command and bounded evidence |
| 7 | `feat/issue8-protected-proof-workflow` | Manual, default-off protected workflow and complete route documentation |

Partial branches do not claim that the entire execution route exists. In
particular, the authenticated gate and execution command are separate trusted
composition boundaries, not agent approval flags or public audit behavior.

## What was checked

The complete candidate received an independent static source review, one bounded
correction batch and a same-findings recheck. The two corrections distinguish
expected owned-child cgroup OOM from parent failure, and measure final source
integrity even on failed effects. That review accepted **source PR readiness
only**; it did not execute tests or attest runtime behavior independently.

Parent-owned verification includes full Go tests, `go vet`, Linux/amd64 and
Darwin/amd64 cross-builds, workflow shell syntax/lint, and reproducible owned
Linux compile/archive/independent inspection. Tests use inert owned processes
and mock boundaries, never the real Docker daemon or a valid host probe.
Individual commits and native PR checks must still be checked at their exact
revision. Cross-builds and mocks are not real isolation evidence.

## Retained safety boundary

- Only the owned immutable image/tool and four fixed cases are in scope.
- The full exact human plan review is required; dispatch, a digest, an Environment
  approval alone or an agent boolean cannot authorize execution.
- No target code, host repository mounts, network, credential forwarding,
  privileged containers, implicit pulls or unsandboxed fallback.
- Container cleanup requires exact ID and owner plus observed absence. The
  untagged immutable image is deliberately retained; no image deletion authority.
- Existing saved Environment policy remains main-only. Source support for an
  exact feature branch is not a policy exception or permission to dispatch.
- `RECORDED` is only a harness outcome, not a security PASS. Keep observed,
  inferred and unverifiable claims separate; do not begin #9 here.

## Delivery gate

Merge or publish no real-isolation guarantee from these source checks. Bind the
reviewed source and pinned image candidate to an exact committed revision, present
the concrete environment, commands, limits, risks, evidence and cleanup plan,
then obtain genuine human authorization before any real Docker operation.
