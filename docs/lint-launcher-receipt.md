# Owned ESLint launcher receipt candidate (#9)

Status: **source-only owned launcher candidate, pure receipt framing and owned
process tests**. No real Node/ESLint/Docker execution, image, runtime catalog,
consent or runner connection is established. This internal framing is not a
second public evidence schema. #9 is still open and the real lint fault-exit
gate is not yet satisfied.

## Separate the two exits

The candidate owned launcher collects one fixed ESLint invocation completely,
then emits one bounded receipt. Tool exit 0 or 1 belongs inside that receipt.
Launcher/container/client success must remain independently zero. A missing or
crashed tool, timeout, output overflow, unsupported tool exit, incomplete capture
or failed receipt write must not become a completed lint observation.

This does not relax the runner's conservative attach-error handling. The launcher
buffers tool output rather than forwarding it as protocol bytes; only the owned
launcher emits the outer frame. Do not infer complete stdout/stderr drainage
merely from a child exit code. A tool exiting 1 still requires verified complete
capture. Source implementation is not a reviewed/pinned image or runtime proof.

## Owned executable candidate

`internal/lintlauncher/cmd/owned-eslint` has only this execution interface:
`owned-eslint --role CLEAN|FAULT|RESTORED --binding DIGEST`, in that exact order.
`--help` starts no tool. Extra/duplicate/unknown arguments, arbitrary source,
executable paths, flags, target configs and plugins are not accepted.

On Linux it invokes `/owned/node/bin/node` with the fixed first argument
`/owned/node_modules/eslint/bin/eslint.js` followed by `OwnedESLintInvocation`'s
owned arguments. Working directory is `/work`; environment is exactly `LANG=C`
and `LC_ALL=C`. There is no shell, PATH lookup, inherited Node options or inherited
stdin. Compiled role fixtures are sent through an explicit pipe. The reviewed
image must eventually supply the exact owned config at `/owned/eslint.config.mjs`.
The launcher neither writes that config nor proves it matches the expected bytes.

The source rejects non-Linux execution, but **does not authenticate a container,
image, human or binding**. It must never be used as a host execution fallback.
The role/digest arguments are data, not consent. No provisioned Node distribution
or dependency/image pin is selected by these fixed pathnames.

Tool capture has a ten-second wall deadline and a shared 60 KiB stdout/stderr
retention cap, leaving space for receipt framing. The implementation owns actual
OS pipe endpoints and joins full stdin delivery plus both output EOFs separately
from child wait. After any child exit, including exit 1, drainage has a 200 ms
ceiling. A missing EOF, short/failed input write, output read failure, overflow or
cancellation rejects the capture without emitting a receipt. Input delivery means
bytes were written, not independent proof that the tool analyzed them.
Go's [`ErrWaitDelay` contract](https://pkg.go.dev/os/exec#ErrWaitDelay) applies to
successful child exits; these independently owned pipes avoid relying on that
status alone when the tool legitimately exits 1.

Receipt emission occurs only after capture succeeds. Short/failed writes or
cancellation cannot report success; a prefix may already have reached stdout.
The final stdout write is synchronous: the external runner must bound the total
container lifetime, including blocked receipt/diagnostic writes. Killing the
direct child is not process-tree or container cleanup. External exact-ID container
cleanup, resource enforcement and source preservation remain mandatory.

Exit 0 means a complete receipt was written (or help succeeded), **not lint PASS**.
Exit 1 means launcher/capture/output/host failure; exit 2 means invalid arguments.
Failure diagnostics are fixed strings, not raw paths or tool logs. A tool's
expected exit 1 is retained inside a receipt while the successful launcher exits 0.

## Exact internal wire format

Six LF-terminated ASCII header lines precede the two concatenated raw streams:

```text
CI-TRUST-ESLINT-RECEIPT/1
<expected execution binding: 64 lowercase hexadecimal characters>
<CLEAN or FAULT or RESTORED>
<tool exit: 0 or 1>
<stdout byte length>
<stderr byte length>
<exact stdout bytes><exact stderr bytes>
```

Lengths are canonical unsigned decimal (`0` or a nonzero digit followed by digits),
at most 65536. No CRLF aliases, signs, padding, leading zeros, unknown versions,
extra header fields, trailing bytes, second receipt or truncated body are accepted.
The **whole frame**, including its header, is at most 64 KiB. Thus the direct
parser's 64 KiB tool-output ceiling is not all available as receipt payload.
The launcher's 60 KiB stream cap reserves space within the total wire budget.
No newline or terminator is implicitly appended after the raw body.

The expected execution binding must eventually cover source snapshot, run/attempt,
role, fixture, effective configuration, argv, parser/launcher revisions, tool and
Node versions, dependency closure, immutable image, limits and effects. The exact
binding producer is not implemented here. A syntactically valid digest is not a
trusted pin, signature, consent token or replay defense.

## Decoder gates

`EncodeESLintReceipt` frames caller-supplied completed exit-0/1 tool captures; it
does not validate their ESLint semantics or prove that any tool ran.

`DecodeESLintReceipt` requires an independently supplied outer `ESLintCapture`:

- Failed outer statuses remain failures, regardless of a valid-looking receipt.
- Outer completion additionally requires exit 0 and empty outer stderr.
- Binding, role, version, canonical lengths and exact whole-frame bounds match.
- The inner tool bytes pass the existing strict `DecodeESLint` parser. Inner
  stderr, malformed JSON, inconsistent counters/exits and unsupported diagnostics
  are not hidden by the receipt. Classification still requires the full triplet.

The caller's outer `Completed` remains an assertion that trusted execution,
cleanup and source preservation succeeded. This pure package cannot authenticate
that assertion, origin or chronology. A fabricated frame plus fabricated outer
success can still fabricate an observation; only trusted composition can close
that boundary. Preserve TOOL_FIXTURE scope and the existing schema-v1 report map.

## Verification and next integration

Pure tests use independently authored formatter-shaped bytes and receipt examples,
not Node/ESLint output. They cover the expected fault exit 1 inside successful
outer exit 0, no-op classification, failed outer captures, identity/role binding,
malformed lengths/framing, exact wire cap, overflow and input immutability. The
fuzzer exercises only this pure parser. Launcher tests execute only the owned Go
test binary: authored lint results, input delivery, missing tools, unsupported exits,
timeout/cancellation, combined overflow, failed output writes and held output
pipes after both exit 0 and exit 1. Held-pipe helpers have a finite lifetime and
cooperative owned release/acknowledgment files; no ambient PID discovery is used.
Instrumented helpers use owned test-only coverage destinations so Go warnings
cannot contaminate tool stderr. Production environment remains unchanged.

Next review the candidate launcher and build, then bind exact runtime/dependency/
config/launcher bytes to an independently inspected image and a separate lint
catalog, role binding and exact consent path. Only then may the runner consume
these receipts. No smoke plan can authorize that change. No real-tool compatibility
or isolation has been demonstrated by the helper tests. See the
[runtime preparation plan](lint-runtime-plan.md).
