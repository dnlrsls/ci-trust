# Controlled-fixture runner: consent and bounded runtime seam

**Intermediate foundation snapshot: no LocalDocker backend is present.**
Only the trusted Runtime seam and fail-closed UnavailableDocker are available.
Owned in-process fake tests are not real-isolation evidence or execution consent.
No production adapter catalog, executable plan binding or CLI is shipped.
S8 and issues #8/#9 remain OPEN.

## Public boundary

`internal/runner` exports `Plan`, `Limits`, `Consent`, `Result`, `Outcome`,
`Snapshot`, `Runtime`, `Runner`, `NewOwned`, `UnavailableDocker`.
`Plan.Display()` includes the complete plan and SHA-256 digest. `Execute` requires
an approved matching digest, checks the owned plan, preflights, then remeasures
the source snapshot immediately before starting. The snapshot callback must hash
a bounded immutable target view; it must not execute target code. Files changing
after remeasurement remain a TOCTOU limitation, but the repository is never
mounted or passed to the fixture.

A future terminal UI must display the plan and obtain an actual human decision.
`Consent` is a transport DTO, not authentication or proof of human presence.
Never build it from agent-provided tool arguments or an unsafe yes flag. There
is deliberately no CLI yet. Synthetic approval in these tests is test-only.

`NewOwned` is trusted application composition, not a target-facing registration
API. Only the fixed smoke adapter/tool, fixture and config identities are
accepted. Its image ID must be an immutable SHA-256 identity from a future
reviewed catalog. A syntactically valid hash does **not** establish trusted image
contents. No production catalog currently authorizes any image. The smoke
entrypoint is `/ci-trust/owned-smoke`; this package does not create that image.
ESLint/config/plugin support and sensitivity claims belong to #9, not this unit.

## Runtime contract and intended Docker policy

The trusted runtime seam must use direct argument vectors, never host shells,
with inherited environment suppressed (including Docker context/host variables,
credential config, proxies and target variables). It must prove a local Linux
Docker daemon and immutable locally present image, not trust ambient contexts.
Remote/ambiguous contexts, missing enforcement, unsupported hosts and unavailable
runtime must fail closed. Preflight may inspect but may not start a container.
No provisioning, builds, installation, pulls, repository execution, credentials,
network, privileged mode, host mounts or fallback execution are authorized.

The generated vector uses `--pull=never`, network none, read-only root, numeric
nonroot user, cap-drop ALL, no-new-privileges, explicit memory/memory-swap, CPU,
pids and bounded noexec/nosuid/nodev tmpfs. The tmpfs is the only intended writable
scratch, not a host mount. Walltime and combined stdout/stderr are bounded by
the harness. All direct resource inputs are bounded before allocation/start.
These requested controls and daemon capability metadata do NOT prove runtime
enforcement. Independent real-runtime tests must establish that they work before
any production admission claim. `UnavailableDocker` continues to refuse all
execution. No concrete backend is included at this cut; runtime confinement requires real proof.


## Evidence and limits

Owned fake Runtime tests cover consent, digest/identity, stale source, output
bounds, cancellation and independent cleanup, including unchanged source bytes
and sentinels. Concurrent output accounting is synchronized. No real runtime
enforcement, credential/network denial, scratch confinement or cleanup is proven.
Runtime implementations must honor contexts; Go cannot forcibly stop a broken
in-process seam. Containers share kernel/daemon trust. No host is attested.
