# Proposed bounded lint admission review

**Status: PROPOSED, NOT ADOPTED. The candidate is NOT ADMITTED.**
This document prepares a security-policy amendment, not a security clearance,
execution plan or permission to change the compiled catalog. The current
`securityHold = true` and `imageAdmitted = false` remain unchanged.

## Proposed change

Replace an automatic requirement to wait for a future Node release with a
candidate-specific review of exposure, producer evidence, material uncertainties
and an explicit maintainer risk disposition. This amendment is not active until
separately adopted through a reviewed policy change. A corrected supported Node
release remains the preferred route when its coverage is established.

The upcoming HIGH notice alone does not identify an exploitable path in these
fixtures. Equally, missing CVE details and owned inputs do not demonstrate that
the candidate is safe. Keep the execution block when the disposition is absent,
rejected or insufficient. An identified affected feature or host-exposure path
would strengthen the case to retain the hold and replace the candidate.

This is a proposed alternative to the corrected-candidate prerequisite in the
[unreleased notes](releases/0.1.0.md), not a claim that prerequisite or either
compiled gate has already changed. Node 24.22.0 is a profile choice, not an
issue #9 requirement. No alternative linter or narrower MVP is selected here.

## Exact candidate and scope

| Identity | Reviewed proposal scope |
|---|---|
| Runtime/tool | Node 24.22.0, npm 11.19.1, ESLint 10.12.0 |
| Profile | `owned-eslint-v1`, Linux/amd64, owned TOOL_FIXTURE evidence only |
| Producer source | `88e8fd002e1e128fad0d35bdbc4572a1f7f7dad9` |
| Producer | Repository `1420154884`, run `39016088214`, attempt `1`, artifact `12094603849` |
| ZIP SHA-256 | `67190310d485cc0c9d0633f0b64d6228da5ae91f0f48e8197bf91a91cce4545c` |
| Image ID | `sha256:f047932f89d4798983a5c5e13ec32843c00fd44795844ea7969c51a15d8289dc` |
| Archive SHA-256 | `sha256:371afa8ec9404f48a353782f3654975ff80349b67130d4986d44e8f3edca3ceb` |
| Layer diff ID | `sha256:97343ac30b461e4faef346c9ffe56a047efe90699c5aeda727bc987ada93cad7` |

The [producer run](https://github.com/dnlrsls/ci-trust/actions/runs/39016088214)
completed successfully. Textual API metadata checked on 2026-10-18 matched the
source/run/attempt/artifact, size 65,571,165 bytes, and unexpired status. Its
recorded expiry was 2026-10-24T08:11:37Z. That snapshot is not a promise of future
availability; authenticated delivery must still check the exact live identity.
The [compiled descriptor](../internal/lintcatalog/artifact.go) is provenance data,
not approval. Changing any pinned bytes or profile invalidates this review scope.

Only the three fixed CLEAN, FAULT and RESTORED fixtures and the owned, hashed
configuration are proposed. No repository JavaScript, executable project config,
plugins, caller-selected commands, runtime installation or host fallback is
allowed. The owned configuration is JavaScript, but it is not imported from the
audited project. Source snapshot measurement does not grant target-code execution.

## Evidence and its limits

| Evidence | What it supports | What it does not establish |
|---|---|---|
| Node producer checks | The [downloader](../internal/lintpreparation/download.go) compares received bytes to the pinned digest; [bundle ingestion](../internal/lintpreparation/node_bundle.go) repeats digest/framing checks | Independent reproduction of upstream bytes, signature review or absence of vulnerabilities |
| npm preparation | [Lock validation](../internal/lintpreparation/npm_inputs.go) checks metadata, exact ESLint identity, registry origins and SHA-512 SRI shape; [installation](../internal/lintpreparation/npm_install.go) runs lock/ci phases and requires an unchanged lock | An independent tarball-content, dependency-range or security audit; disabled install scripts do not make dependency code harmless |
| Candidate consistency | Existing checksum/inventory review and the separate [independent inspector](lint-candidate-inspection.md) compare immutable archive/config/layer/filesystem data | Package trust, full security clearance, admission or observed runtime behavior |
| Static ELF work | Existing global export-availability checks found candidates for the recorded versioned requirements | Effective loader resolution or successful execution; earlier per-provider misses were not proven ABI failures |
| Runner source | [Fixed lint plans](../internal/runner/lint.go), [lifecycle checks](../internal/runner/docker.go) and the [lint image observer](../internal/runner/lint_image.go) constrain the path before start | Live enforcement for this candidate; container inspection does not independently re-read every effective HostConfig field |

The preparation source is unchanged between the producer revision and the source
reviewed for this proposal. A successful producer and matching inventory provide
useful evidence, but producer declarations are not independent admission authority.
The installer runs npm; the Go lock validator's SRI-shape checks must not be
misrepresented as independent checksums of every downloaded package.

Do not turn unestablished assurances into universal new gates. A complete manual
audit of every dependency, independent reconstruction, or a formal model of every
loader relocation is not introduced here as a prerequisite. Review material gaps
for this exact trust boundary and record why each is resolved, accepted or blocking.

## Residual risk and unchanged containment

Node trusts executed application and dependency code. Fixed fixtures eliminate
neither compromised dependencies nor an unknown applicable runtime vulnerability.
Node process compromise does not itself demonstrate host escape, but shared-kernel,
Docker-daemon/runtime and supply-chain risks remain. Node's Permission Model and a
`noexec` scratch mount are not sandboxes against malicious JavaScript.

The existing profile requests no network or host mounts, read-only rootfs,
UID/GID 65532, all capabilities dropped, no-new-privileges, and bounded scratch.
Its fixed limits remain 10 seconds, 256 MiB, 1 CPU, 16 PIDs, 64 KiB retained output
and 16 MiB scratch. The Docker client uses its fixed local socket and minimal
environment; the Node child does not inherit host credentials. The supported
Linux daemon, exact image, ownership and I/O preflight must still fail closed.
These are source-level controls, not claims of an already observed Node run.

## Decision required before a later catalog change

A reviewer and maintainer must record a disposition for this exact candidate:

1. Identify the trusted producer, immutable inputs and integrity evidence. State
   which claims depend on the producer/npm and which were checked independently.
2. Resolve or disposition material provenance, content, compatibility and security
   uncertainties. Do not infer harmlessness from the absence of an identified CVE.
3. Explicitly accept or reject the residual risk for only the owned-fixture
   profile. Approval to prepare this proposal is not that acceptance.
4. Only after a supported disposition, prepare and independently verify any
   security-relevant catalog change. Preserve smoke/#8 and all existing boundaries.

The current disposition is **PENDING**. This document does not record acceptance,
clear the Node hold, admit the image, reorder `BlockedReason`, or add a policy
override. Clearing the Node hold alone would still leave image admission closed.
A label-only change is not evidence of progress in admission.

Compatibility observations can be gathered during a later explicitly approved
bounded attempt; a successful real triad is not circularly required before the
attempt can be authorized. Missing tools, startup/ABI failures, timeout, OOM or
unverified cleanup remain infrastructure/UNVERIFIABLE outcomes, not lint findings.

## Execution remains a separate decision

Even an adopted disposition and reviewed catalog change would not authorize a
run. The [exact GitHub review](lint-github-consent.md) and
[protected workflow](lint-workflow-control.md) still require the owner's exact
`approve lint <binding_digest>` and protected Environment review. No digest or
approval is created by this document, a report, a source merge or an imported plan.

No Node/npm/ESLint/launcher/Docker execution, candidate download/extraction,
workflow dispatch, Environment change, tag, release or issue closure is included.
Do not alter existing sandbox constraints or claim Node is patched, CVE-free or
safe. No automatic monitoring is scheduled. Real CLEAN/expected-FAULT/RESTORED
captures and all six #9 acceptance criteria remain necessary before closure.
