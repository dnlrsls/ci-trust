# Prepare an exact run-bound plan

`controlled-plan` produces **preparation-only review data**, not execution consent. It neither creates `runner.Consent` nor starts a runner. Digests and CLI arguments do not authenticate a human; the protected Environment workflow caller remains the trust boundary.

## Quick path

The source command lives at `internal/controlledplan/cmd/controlled-plan`. Its two forms are:

```text
controlled-plan prepare --snapshot SHA256 --image sha256:HEX --revision GIT40 --run-id DECIMAL --run-attempt DECIMAL
controlled-plan check --input FILE --binding HEX --revision GIT40 --run-id DECIMAL --run-attempt DECIMAL
```

1. Prepare using a lowercase 64-hex snapshot, immutable lowercase image ID, lowercase 40-hex revision, and canonical positive decimal run ID/attempt (no signs, spaces or leading zeroes).
2. Display the entire `plan` and `plan_digest` before protected review. Go callers can use `Envelope.Display()`, which delegates directly to `runner.Plan.Display()`.
3. After the gate, check the saved envelope against independently pinned binding, revision, run ID and attempt. Do not extract those expected values from the untrusted input itself.

Success prints one compact JSON envelope plus a newline, with no stderr, and exits 0. Argument errors exit 2 with `controlled-plan: invalid arguments`; rejected data or unavailable input exits 2 with `controlled-plan: validation failed`. Errors print only that diagnostic plus a newline to stderr, with no envelope on stdout. `--help`, `prepare --help`, and `check --help` explain the preparation-only boundary. Output failures exit 1 with `controlled-plan: output failed` on stderr.

## Exact profile

| Field | Fixed value |
|---|---|
| Repository / version / scope | `dnlrsls/ci-trust` / `1` / `preparation-only` |
| Adapter / tool | `owned-smoke-v1` / `owned-smoke-v1` |
| Fixture / config | `smoke-v1` / `owned-v1` |
| Wall / memory / CPU / PIDs | 10 seconds / 128 MiB / 1 / 32 |
| Output / scratch | 64 KiB / 8 MiB |
| Effects | `owned fixture only; no host mounts; no network` |

The existing runner owns plan JSON field names, digest and display. Durations serialize as integer nanoseconds. Existing runner behavior specifies UID `65534:65534`; this package does not modify runner preflight or construct runtime arguments.

The envelope fields, in canonical encoding order, are `plan`, `plan_digest`, `binding_digest`, `revision`, `run_id`, `run_attempt`, `repository`, `version`, `scope`. The binding is lowercase SHA-256 of compact Go JSON encoding in that order **with `binding_digest` omitted**, without a trailing newline. `plan_digest` is exclusively `runner.Plan.Digest()`.

Check rederives the fixed plan and entire envelope and compares the externally supplied binding/context. Input is limited to 16 KiB; decimal IDs are limited to 4096 characters each. Unknown fields, case aliases, duplicate fields (including nested fields), missing fields, malformed values and trailing JSON are rejected. Whitespace and object key order need not be canonical. The CLI reads only an explicitly named regular file; it performs no writes, network, runtime or subprocess operations. These are preparation contracts, not a guarantee against hostile concurrent filesystem replacement.

## Review boundary and missing proof

- Snapshot/image substitution changes the binding; a matching plan digest alone is insufficient.
- Stale revision, run or attempt must fail even if the stored envelope is internally consistent.
- A valid envelope is not image-content validation, observed daemon attestation, catalog admission or authenticated approval. This package intentionally does not import the independent image preparation package.
- No host probe, Docker build/load, container execution, isolation enforcement or protected-gate outcome is proven here. Publication and execution require separate authorization.
- Future integration must preserve the protected workflow boundary and must never turn successful check into a consent boolean.

Focused tests use owned fake bytes only. They cover a literal profile oracle, all plan identities/limits/effects, envelope mutations, stale external context, strict JSON, prepare/check command results, and command input/counter/inventory immutability. Command tests launch the owned test executable as a helper that calls real `main`, asserting actual process exit status, stdout and stderr for prepare/check/help and rejected stale or invalid input. They also verify unchanged valid/invalid input bytes, counter and directory inventory after each case. A failing-writer test verifies that output failure cannot report success. No additional binary compilation, host probe or runtime operation is used.
