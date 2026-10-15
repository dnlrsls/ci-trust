# Controlled-fixture runner: consent and bounded runtime seam

**Intermediate client/preflight snapshot: LocalDocker execution is unavailable.**
The trusted Runtime seam, UnavailableDocker and metadata-only LocalDocker are available.
Owned in-process fake tests are not real-isolation evidence or execution consent.
No production adapter catalog, executable plan binding or CLI is shipped.
S8 and issues #8/#9 remain OPEN.

## Public boundary

`internal/runner` exports `Plan`, `Limits`, `Consent`, `Result`, `Outcome`,
`Snapshot`, `Runtime`, `Runner`, `NewOwned`, `UnavailableDocker`, `LocalDocker`
and `NewLocalDocker`.
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
execution. LocalDocker Start and Remove are unavailable stubs at this cut;
runtime confinement requires real proof.


## Evidence and limits

Owned fake Runtime tests cover consent, digest/identity, stale source, output
bounds, cancellation and independent cleanup, including unchanged source bytes
and sentinels. Concurrent output accounting is synchronized. No real runtime
enforcement, credential/network denial, scratch confinement or cleanup is proven.
Runtime implementations must honor contexts; Go cannot forcibly stop a broken
in-process seam. Containers share kernel/daemon trust. No host is attested.

## Intermediate local client and preflight

LocalDocker and NewLocalDocker now provide the bounded direct executable client
and fail-closed capability/image preflight only. Start and Remove are unavailable
stubs and make no client calls, even after a successful preflight. Container
creation, attachment and ownership-authorized removal land together next.

`NewLocalDocker` takes trusted absolute executable/config paths, not target data.
It requires an existing empty private config directory; the trusted owner must
keep both paths immutable and private. No files are created by the constructor.
The executable is invoked directly, never through a shell. Every invocation fixes
`--config` and `--host`; inherited environment, contexts, credential config,
proxies and target variables are suppressed. Only HOME/DOCKER_CONFIG pointing at
the clean directory and C locale are supplied; PATH is absent. The owned
executable test compares the entire child environment against that independent
allowlist, permitting only Go's required SYSTEMROOT addition on Windows test
hosts (not Windows runtime support). There is no image acquisition, runtime provisioning,
stdin forwarding or host fallback.

Preflight uses bounded `info` and immutable-ID `image inspect` metadata only.
Image volumes, environment, entrypoint, command and healthcheck defaults are
refused. Image contents must separately be reviewed/catalogued; an ID is not
trust. Each metadata command shares a 64 KiB stdout/stderr bound. Every client
command has a two-second ceiling and 200 ms WaitDelay, additionally bounded by
its parent context. Consequently long-running smoke tools are conservatively
UNVERIFIABLE even if their displayed wall limit is greater than two seconds.
The wall limit is an upper bound, not a guaranteed minimum execution duration.


Owned executable fakes test the exact child environment, streams, metadata
bounds and deadlines. Fake metadata refusals and concurrent admission tests
exercise preflight only. These checks do not contact a Docker runtime.
