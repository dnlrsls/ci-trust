# Controlled-fixture runner: bounded local Docker backend

**Status: backend code and owned-fake tests only; S8 real-isolation acceptance
remains OPEN.** No production adapter/image catalog or executable plan binding
is shipped. This unit ran ZERO real Docker commands, containers or image
operations. Fake tests are not isolation evidence or execution consent.

## Public boundary

`internal/runner` exports `Plan`, `Limits`, `Consent`, `Result`, `Outcome`,
`Snapshot`, `Runtime`, `Runner`, `NewOwned`, `UnavailableDocker`, `LocalDocker`,
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
execution. The local backend also disables Docker logging to avoid an unbounded
persistent daemon-side output sink. It also fixes `--ipc=none` to avoid Docker's
additional writable `/dev/shm` scratch mount; runtime confinement still requires
real proof.

## Local client and lifecycle

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

The backend accepts only the exact Runner-owned vector and is single-use after
a start attempt. Concurrent operations are rejected immediately, rather than
waiting behind a client call and losing cancellation. Sequential Runner cleanup
still uses its independent bounded context. It translates `run` to fixed-policy `create`, binds the returned
ID, then uses `start --attach ID`. Tool output goes to Runner's combined stream
bound. After attachment, inspect must establish a nonzero start timestamp,
exited state, no daemon state error and no OOM indication. Transport/startup
errors remain infrastructure outcomes, not validator failure. A nonzero client
exit is ambiguous even when it matches the independently inspected container
exit: it cannot prove that attach transport completed. Any attach client error
therefore remains UNVERIFIABLE, with the inspected container exit retained when
complete state is available. This conservatively includes legitimate nonzero tool
exits; no independent wait/log transport protocol is implemented. Missing
exit/OOM/error fields are refused rather
than defaulted to a successful zero value. No public method
accepts arbitrary argv for execution; direct Start calls still require preflight
and the exact owned vector. Trusted composition is not a human-consent boundary.

## Results and cleanup

DENIED means no current approval. INVALID means an unowned/invalid plan.
UNVERIFIABLE covers missing runtime, changed snapshots, timeout/cancellation,
output overflow, harness faults and unverified cleanup. COMPLETED means only
that the process returned normally, with its exit code preserved; it does not
mean a validator PASS or FAIL. No exit status establishes sensitivity evidence.

Every attempted Start gets a unique name and a fresh, bounded three-second
cleanup context, including launch failure, cancellation and overflow. Names alone
NEVER authorize removal. LocalDocker creates with a random ownership label,
requires a complete container ID, then checks that ID, label and immutable image
before recording removal authority. It rechecks ownership before force-removing
only that ID. A successful exact-ID all-container listing must then be empty;
generic inspect failures never establish absence. Creation ambiguity, collisions,
failed ownership checks and failed cleanup remain UNVERIFIABLE. In ambiguous
creation, an owned container may remain: this backend refuses unsafe name-based
recovery rather than risking a foreign container. Killing the Docker client alone
is not cleanup. Runtime implementations must honor contexts; Go cannot forcibly
stop a malicious or broken in-process runtime seam.

## Evidence and residual risk

Owned-fake coverage includes the public Execute boundary, exact create/start/ID
cleanup vectors, zero client calls for denied/stale/invalid plans, snapshot change
before creation, foreign ownership and name collisions, ambiguous inspection,
startup/OOM/attachment faults, incomplete state, capability/image refusals,
output/deadline limits, active cancellation and cleanup absence failures. Owned
source snapshots include file/directory names, untracked sentinels and file bytes.
The test executable itself doubles as an owned fake client to exercise direct
argv, suppressed ambient credential/context variables, both output streams,
nonzero exits, combined metadata limits and client deadlines. It never connects
to Docker. The full lifecycle policy oracle uses independent literal vectors and
numeric values, not production vector construction. Temporary fake-only privileged
argv and extra-environment mutations were observed failing these oracles, then
removed. A blocked-holder/canceled-waiter regression checks prompt admission
rejection without a second client start, then releases and joins the holder.
Matching genuine client/container exit errors remain explicitly ambiguous.
No tests claim skipped real-runtime coverage as a pass.


Unit fakes count preflight, process/container attempts and cleanup; tests exercise
consent/identity rejection, unavailable preflight, stale source, output bounds,
timeout, pre-cancellation, launch faults and cleanup faults. They do not execute
repository code or contact a runtime. The runner has no source-writing interface.
Owned TempDir source bytes and sentinel counters are measured before and after
public Execute calls for success, denial, stale consent/source, invalid plans,
unavailable preflight and harness/cleanup/output/timeout faults. An active
cancellation fake observes Start before cancellation and verifies bounded return
and cleanup with a fresh live deadline. Concurrent output writes and state reads
share a mutex; accepted byte counts reflect truncation. These are local harness
tests only. Byte/counter immutability and active cancellation against real Docker,
memory/pids exhaustion, credential/network denial, scratch confinement and cleanup
verification still need independent real-runtime tests.

Supported production runtime/host combinations: **none yet attested**. The code
supports only a Linux host and the fixed local `unix:///var/run/docker.sock`
endpoint, Linux images, cgroup v2, advertised CPU quota/memory/swap/pids support,
no daemon warnings and builtin seccomp. Windows/macOS and remote/context-selected
backends are refused. No Linux, Windows or macOS isolation claim is made.
Containers share kernel/daemon trust and are not an absolute security boundary;
malicious images, daemon compromise, kernel escapes and resource enforcement
failures remain risks. Do not admit arbitrary code/config/plugins or elevate
static audit evidence based on this scaffold.
