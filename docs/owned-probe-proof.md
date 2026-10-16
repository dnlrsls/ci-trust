# Owned proof: bounded native source, not isolation acceptance

`owned-smoke --fixture=smoke-v1 --config=owned-proof-v1` selects bounded native observations in the same tool. Linux source is implemented, but **no real proof has been executed during development**. Inert tests establish dispatch, parsing and cleanup contracts, not isolation acceptance. Unsupported controls fail closed.

## Review path

1. Review `internal/ownedprobe/probe.go`: fixed arguments and existing guards precede the proof source.
2. Review `internal/ownedprobe/proof.go`: fixed observations, sanitized output and fail-closed completion.
3. Review fake-only tests in `proof_test.go` and `proof_source_test.go`.
4. Review `native_proof.go`, `proof_effects_linux.go` and `proof_process.go` before requesting an authorized real invocation. Review compile-only evidence separately; never execute the artifact on the host.

`owned-v1` retains its existing stdout and guard behavior. Only `--help` adds an opt-in description; help performs no observations. Unknown options, target arguments, alternate fixtures and child-mode arguments remain invalid. There are no new general-purpose mode flags.

## Wire contract

After all existing guards and owned scratch cleanup succeed, stdout starts with the exact line:

```text
owned-smoke: proof-started
```

The line ends with a real LF. Stderr receives `owned-smoke: owned stderr` followed by LF. Stdout then receives exactly one JSON object followed by LF:

```json
{"version":1,"scope":"owned-smoke-v1","checks":[{"name":"nonroot","state":"OBSERVED","detail":"uid65534","before":null,"after":null}],"limitations":["..."]}
```

The example abbreviates the array: actual output always includes all twelve checks in this order:

| Check | Current evidence source |
|---|---|
| `nonroot` | Existing Linux/UID guard |
| `capabilities` | Existing safe status parser, all capability sets zero |
| `no_new_privileges` | Existing safe status parser |
| `seccomp` | Existing safe status parser, filter mode |
| `root_readonly` | Existing owned marker check; only open EROFS qualifies |
| `scratch_write_cleanup` | Existing exclusive creation and identity-checked cleanup |
| `scratch_limit` | Fixed owned file reaches ENOSPC within 8 MiB plus one byte; identity cleanup |
| `network_unreachable` | Only loopback, no usable routes, UDP connect returns ENETUNREACH; no send |
| `host_credentials_absent` | Named credential keys and the specified harmless sentinel key/path are absent |
| `cpu_limit` | 1.8-second bounded demand and increased `nr_throttled` |
| `memory_limit` | One capped child, failed waited exit and increased memory max/oom_kill counter |
| `pids_limit` | One bounded-thread child, failed waited exit and increased pids max counter |

States are observations, not security PASS. The implemented serializer emits `OBSERVED` or `UNVERIFIABLE`; it makes no inferred claims. Counters appear only for supported resource observations with a strictly increasing counter. Counter reset, equal counters, unavailable operations and cleanup/wait failures must not become successful observations. Unsupported counters are `null`, not invented zeros. Detail and limitation strings are fixed literals; raw errors, environment values and child output are never serialized.

Exit zero requires all twelve supported observations. Missing optional support emits explicit `UNVERIFIABLE` checks and exits one with an additional fixed `owned-smoke: unavailable` stderr line. A guard failure exits one with that fixed diagnostic and no stdout, before invoking the optional source. Invalid arguments exit two with the existing fixed diagnostic. Stream write failures exit one.

## Fixed optional source contract

`ProofSystem.ObserveProof() ProofObservations` is optional, so existing `System` fakes still compile. It exposes only three fixed boolean observations and three resource counter observations, with no paths, commands, arbitrary strings or environment forwarding. This is a trusted owned adapter boundary, not a validator of untrusted claims.

The caller has already run `observe`, including marker and scratch checks. The native adapter reuses it before dispatch and each bounded operation, including direct proof invocation and child workload dispatch. Between quota writes it rechecks the same read-only kernel, mount and marker prerequisites through `guardControls`, without allocating another scratch file: a full authorized tmpfs must not prevent the final ENOSPC attempt. Its dynamic defaults are Linux-only and independently guarded. `CounterObservation.Supported` requires successful owned operation, cleanup and wait/reap, not just readable configuration. A missing operation, malformed counter, counter reset, unexpected wait result or timeout stops the sequence with unavailable observations.

## Native operations and bounds

- Scratch: fixed exclusive owned file, small chunk writes totaling at most 8 MiB plus one byte, observed ENOSPC, descriptor identity cleanup on all paths; never remove foreign files.
- Network: documentation-only UDP connect route lookup without send, plus local interface/route observations. No external packet, listener, scan or network enablement.
- Credentials: presence-only checks for named credential keys and harmless sentinel key/path; no credential file reads or echoed environment values. Docker-generated PATH/HOSTNAME/HOME alone are not inherited credentials.
- CPU: two workers for 1.8 seconds with two Go scheduling slots, under the required one-CPU cgroup quota; observed `cpu.stat` throttling delta. Scheduling slots are restored afterward.
- Memory: one same-immutable-executable child with equal full guards, bounded approximately 160 MiB attempt under 128 MiB, relevant `memory.events` delta and owned wait result.
- Pids: one same-executable child, bounded own thread demand, observed `pids.events` max delta while preserving parent control capacity. Do not replace this with an unconstrained runtime thread loop.
- Each child: fixed internal environment key `CI_TRUST_OWNED_PROOF_CHILD=memory|pids` plus only GOMAXPROCS=1, GOMEMLIMIT=off and GOTRACEBACK=none. Arguments are the same fixed public proof arguments. No shell or external tools. Deadline is 3.5 seconds with 200 ms pipe WaitDelay; cancellation kills the exact owned process and waits/reaps. Child output is drained to a zero-storage discard writer. The parent reserves its current OS thread and uses one Go scheduling slot while the child is live, then restores it. Memory demand additionally requires `memory.oom.group` equal to zero to reject group-wide OOM killing.

Native reads have a separate exact allowlist: cpu.stat, memory.events and pids.events (4096 bytes each), memory.oom.group (16 bytes), and /proc/net/dev, /proc/net/route, /proc/net/ipv6_route (16384 bytes each). Existing System.Read paths and limits are unchanged. The network destination is UDP 192.0.2.1:9 with no send/write. Environment checks cover CI_TRUST_HOST_CREDENTIAL_SENTINEL, GITHUB_TOKEN, GH_TOKEN, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_SESSION_TOKEN, SSH_AUTH_SOCK, DOCKER_AUTH_CONFIG, GOOGLE_APPLICATION_CREDENTIALS and AZURE_CLIENT_SECRET. They inspect presence only, never serialize values. /home/runner/work/ci-trust/ci-trust/.git/controlled-proof/host-sentinel is lstat-only. This is not an exhaustive proof about all possible host credentials.

The scratch path is /tmp/ci-trust-owned-proof-limit-v1, exclusively created mode0600, with chunks at most32KiB. Acquired identity is required before unlink; identity loss, replacement or cleanup failure yields unavailable and never removes a foreign file. If descriptor stat fails, safe identity cleanup cannot be established; container cleanup must remove the owned temporary filesystem.

The executable must be exactly os.Executable()=/ci-trust/owned-smoke, regular, root-owned, mode0555, with matching identity immediately before launch. Both children run the same full guards. Memory allocation is capped at160MiB and page-touched; pids attempts at most80 locked OS threads. Unsupported controls stop cleanly rather than claiming success.

None of these native effects was executed or proved during development. Linux cross-compilation establishes source portability only, not isolation or resource enforcement. Unit tests use injected inert operations, not host procfs, network, CPU demand, memory stress or child stress. The nominal sequence is bounded to1.8+3.5+3.5seconds of demand/deadlines; guard and launch overhead also count toward the runner's10-second wall, so slower unsupported environments can time out rather than yield proof.

## Residual scope

Even successful owned observations will not establish absolute escape resistance, absence of every credential or general tool security. Group OOM killing is rejected, but kernel OOM victim selection and scheduler behavior are not absolute parent-survival guarantees; loss of the parent cannot become successful proof. Kernel-uninterruptible operations cannot be given a universal reap-time guarantee. These outcomes require independent runner cleanup, not an invented observation. The host composition must independently establish timeout, cancellation, combined stdout/stderr limits, image/source identity, source integrity and owned-container cleanup. Real execution still requires concrete human authorization; neither the new profile nor a fake test supplies it.
