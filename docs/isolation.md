# Controlled-fixture runner: bounded local Docker backend

**Status: backend code and owned-fake tests only; S8 real-isolation acceptance
remains OPEN.** No production adapter/image catalog or executable plan binding
is shipped. The separate [image archive preparation](owned-image.md) and
[exact-plan preparation binding](controlled-plan.md) provide source-only inputs
for a manual review workflow, not runtime admission or execution consent. This unit ran ZERO real Docker commands, containers or image
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

## Owned smoke source: partial observations only (T8C)

`internal/ownedprobe/cmd/owned-smoke` is a standalone internal command, not a
public CLI mode or host fallback. Source and a scratch-image **recipe** are
provided; no image is provisioned, registered or approved. Do not run the valid
probe on the host. Unit tests inject all kernel and filesystem operations,
including adapter descriptor operations; they do not access real procfs, root
markers, credentials or network. Existing audit/report CLI code shares no new
code, so a differential CLI campaign does not apply to this unit.

### Fixed first profile and interface

| Setting | Required future runtime configuration |
| --- | --- |
| Entry point | `/ci-trust/owned-smoke` |
| Arguments | `--fixture=smoke-v1 --config=owned-v1` (either order) |
| UID | 65534, including all four kernel UID fields |
| CPU / memory / swap / pids | 1 CPU (`100000 100000`), 128 MiB, zero, 32 |
| Scratch | `/tmp` tmpfs, 8 MiB, rw/nosuid/nodev/noexec |
| Wall / combined output | 10 seconds upper bound / 64 KiB, enforced by harness |
| Client ceiling | Existing backend two-second ceiling remains unchanged |

This is one source profile, not a default image, production catalog or approved
plan. The probe requires cgroup v2 rooted at `0::/` and reads only fixed bounded
control files. Other layouts, representations or unavailable controls fail
closed rather than discovering arbitrary cgroup paths. CPU, memory, swap and
pids values are **configuration observations**, not stress/enforcement proof.
The wall upper bound does not override the shorter backend client ceiling.

`--help` alone prints two lines and exits 0 without provider calls:

```text
Usage: owned-smoke --fixture=smoke-v1 --config=owned-v1
Owned-fixture-only partial observations; not isolation proof.
```

Unknown, missing, duplicate, positional or split-value arguments exit 2 with
empty stdout and `owned-smoke: invalid arguments` on stderr. A guard, observation,
write or cleanup failure exits 1 with empty stdout and `owned-smoke: unavailable`
on stderr; system error text and file contents are not echoed. Successful owned
observations exit 0 with empty stderr and exactly:

```text
owned-smoke: partial owned-fixture-only
observed: uid65534 caps0 no-new-privileges seccomp-filter root-marker-erofs scratch-write-cleanup cgroup-v2-profile
not-proven: resource-enforcement network credentials wall-time output-limit full-isolation
```

Each line ends with LF. This is not report-v1 data, a validator verdict or
`isolation_proof=true`. Network and credential observations are deliberately
absent: there is no network, environment or credential API. No CPU/pid/OOM stress
modes exist. Wall/output enforcement, cancellation and runtime cleanup still
require separately approved real-runtime evidence.

### Owned file effects and limits

Before writes the real adapter rechecks Linux, UID 65534, all five capability
sets zero, NoNewPrivs=1 and seccomp filter mode 2. Reads have fixed path/bound
allowlists; final-component symlinks and nonregular files are refused, using
nonblocking opens to avoid FIFO hangs. The image's parent directories and kernel
mounts remain trusted prerequisites, not an escape-proof custom sandbox.

The literal marker at `/ci-trust/root-marker` must match
`tools/owned-smoke/root-marker` byte-for-byte before any write. It is owned by
65534 and mode 0600 so discretionary permissions do not masquerade as a read-only
root. Only EROFS from the write-open establishes the narrow read-only-marker
observation; EACCES does not. Unexpected writable access is always failure. The
adapter restores its known marker on the same descriptor even after partial
write/truncate failure; restoration failure remains unavailable, never success.
Run also requests restoration on unexpected writable success.

Scratch is a single 23-byte exclusive-created file,
`/tmp/ci-trust-owned-smoke-v1`, mode 0600. Ownership starts at successful exclusive
open, including subsequent short-write or close failure. Cleanup checks recorded
file identity and refuses foreign collisions/replacements. Failure to establish
identity or complete cleanup remains unavailable and can leave owned scratch;
the harness must still clean up its container. Identity-check/unlink races are
not an absolute guarantee against concurrent hostile processes; this owned
single-process image is not an arbitrary-code sandbox.

### Compile and prepare source context only

Prerequisites: an already-installed compatible Go toolchain and locally cached
module dependencies; never acquire tools, packages or images implicitly. The
parent owns checks. A compile-only Linux/amd64 artifact uses package
`./internal/ownedprobe/cmd/owned-smoke`, `GOOS=linux GOARCH=amd64 CGO_ENABLED=0` and
`go build -o <authorized-scratch>/owned-smoke`. Keep GOTOOLCHAIN=local,
GOPROXY=off, GOSUMDB=off and GOTELEMETRY=off, with GOCACHE and TEMP/TMP/TMPDIR in
the explicitly authorized project-common-Git scratch area. Do not execute the
result. Cross-compilation is not Linux execution evidence.

For future review, prepare a dedicated context containing only the compiled
`owned-smoke`, the literal `root-marker` and the reviewed
`tools/owned-smoke/Dockerfile`. Do not copy the repository, credentials or caches
into that context. The recipe has only FROM scratch and two explicit owned-file
COPY instructions. It has no frontend directive, remote base, RUN, ADD, ENV,
USER, CMD, ENTRYPOINT, HEALTHCHECK or VOLUME instructions. Numeric runtime user
and explicit entrypoint come from a later approved plan, not image defaults.

**The recipe alone is not admissible as-is.** Ordinary Dockerfile/BuildKit
conversion can supply a default PATH in image `Config.Env`, even with FROM
scratch and no ENV instruction. Absence of Dockerfile instructions does not
prove empty image configuration. This is a static recipe limitation, not an
observation of actual image metadata; no image was built or inspected here.

Before catalog or preflight admission, a future explicitly approved preparation
must include reviewed empty-config finalization and independent observation and
attestation of every required image configuration field, including `Env`, `Cmd`,
`Entrypoint`, `Healthcheck` and `Volumes`. All must satisfy the existing backend's
empty-default requirements. Its nonempty-Env refusal remains unchanged; do not
work around it by admitting the injected PATH. No config rewriter, image builder
or finalization procedure is implemented or authorized by this source recipe.

The recipe above still has no image-building automation or Docker commands.
The separate [owned image archive tool](owned-image.md) constructs an explicit
empty-config archive without invoking a builder or daemon; it does not finalize
or authorize this Dockerfile recipe. Its independent offline inspection is not
observed daemon metadata, production catalog admission, or isolation evidence.
[The manual preparation workflow](isolation-preparation.md#owned-archive-and-exact-plan-review)
displays the complete run-bound plan before the configured review boundary and
rechecks it afterward, but never creates runtime Consent. Image import,
immutable-content review/catalog admission, exact runtime-plan consent and
execution still require explicit human review and approval.
Full S8/#8 and #9 acceptance remain OPEN; mocks and this recipe do not fill that
proof gap.
