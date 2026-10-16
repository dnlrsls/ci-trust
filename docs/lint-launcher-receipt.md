# Owned ESLint launcher receipt candidate (#9)

Status: **pure encoder/decoder and authored tests only**. No launcher executable,
Node invocation, image, runtime catalog, consent or runner connection is added by
this unit. This internal framing is not a second public evidence schema. #9 is
still open and the real lint fault-exit gate is not yet satisfied.

## Separate the two exits

The proposed owned launcher collects one fixed ESLint invocation completely,
then emits one bounded receipt. Tool exit 0 or 1 belongs inside that receipt.
Launcher/container/client success must remain independently zero. A missing or
crashed tool, timeout, output overflow, unsupported tool exit, incomplete capture
or failed receipt write must not become a completed lint observation.

This does not relax the runner's conservative attach-error handling. A future
launcher must buffer tool output rather than forwarding it as protocol bytes;
only the owned launcher emits the outer frame. Its fixed executable, arguments,
environment, input, pipe completion, output budget and build need separate review.
In particular, do not infer complete stdout/stderr drainage merely from a child
exit code. A tool exiting 1 still requires verified complete capture.

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
The future launcher must reserve header space within its shared stream budget.
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

Tests use independently authored formatter-shaped bytes and receipt examples,
not Node/ESLint output. They cover the expected fault exit 1 inside successful
outer exit 0, no-op classification, failed outer captures, identity/role binding,
malformed lengths/framing, exact wire cap, overflow and input immutability. The
fuzzer exercises only this pure parser.

Next implement the owned launcher subprocess lifecycle and complete-drain checks,
then review its fixed invocation and receipt emission. Only after a separate lint
catalog, role binding and exact consent path exist may the runner consume these
receipts. No smoke plan can authorize that change. See the
[runtime preparation plan](lint-runtime-plan.md).
