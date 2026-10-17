# Protected lint image candidate preparation (#9)

This is preparation-only source. No workflow run, resolved lockfile, image or
real-tool evidence is claimed by publishing it. Existing smoke/proof workflows
and their admission rules remain unchanged.

## Dispatch and review boundary

`.github/workflows/lint-image-preparation.yml` is manual-only, with no inputs,
restricted to the repository owner on `dnlrsls/ci-trust` `main`. The existing
`ci-trust-controlled-verification` Environment gates the entire job; its policy
is not edited. A new workflow must first be reviewed and merged into the default
branch before dispatch. Publishing a PR is not merge authorization. Environment
review remains a human action; an agent must not approve on the reviewer's behalf.

Preparation requires a fresh hosted Ubuntu 24.04 job, up to fifteen minutes,
contents-read token permissions, fixed Go 1.26.4, no shared package caches and
checkout of the exact dispatch SHA without persisted Git credentials. Workflow
source now uses the Go preparer: offline preparation-subtree tests run before
compiling the fixed launcher and preparation binary, with private project caches
and temporary storage under umask 077. Toolchain/dependency network fallback and
shared caches are disabled. The fixed compiler is not a separately pinned builder
image or independent reproduction proof; metadata records `go_version`, not the
legacy `python_version`. Portable Go CI already tests all packages on Linux,
Windows and macOS. No host/SSH session or image registry write is needed.

The compiled preparation binary runs directly under a 600-second TERM deadline
with a ten-second kill grace; compilation happens before that deadline. See the
[CLI guide](owned-lint-prepare-cli.md) for inputs, outputs and safe help. The local
Go source migration is complete: the legacy Python preparer/tests are retired,
while the owned npm manifest remains. Supporting source review and synthetic/
offline tests do not demonstrate hosted preparation success, actual-pin format
compatibility, Linux permissions/ACL behavior, Python parity, runtime/library
closure or independent reproduction. No native approval or execution consent is
claimed; the separate admission gates below remain unchanged.

## Allowed effects

- Compile the owned Go launcher and preparer; do not execute the launcher. Emit compiled owned config,
  recipe and fixture identities through an owned Go data-only helper.
- Download the official `node-v24.22.0-linux-x64.tar.gz`, verified against SHA-256
  `28ea0a438de9a13bd7891fd758eee6c7819ee8af5442dfd8d45bd65e56d4228c`, captured
  from the [release checksum list](https://nodejs.org/dist/v24.22.0/SHASUMS256.txt).
  Preparation uses the fixed gzip archive pin directly, not another checksum-list
  request. Bundled npm must be exactly 11.19.1.
- Run that pinned Node/npm **only as the preparation package manager**, never
  ESLint or arbitrary package scripts. Resolve exact ESLint 10.12.0 in a fresh
  owned directory, validate a complete lockfile v3, then perform frozen install
  with lifecycle scripts, optional/peer/dev installation, shims, audit and funding
  requests disabled. The target repository is not the npm working directory.
- npm receives an explicit credential-free environment, empty owned configs and
  PATH containing only the downloaded Node directory (no Git/SSH/shell lookup).
  Its HTTP/HTTPS proxy permits only CONNECT to registry.npmjs.org:443. Lock origins
  and SHA-512 integrity are checked before installation. This constrains the
  trusted package manager's fetch path; it is not independent OS egress isolation
  or a security guarantee for the hosted kernel/runtime.
- Fetch Linux/amd64 distroless cc-debian12 by platform manifest digest
  `sha256:777e96cf322c46bc32aca926c263624c4dc8d7cf37e2fa65ba2c7e697318ebbb`.
  Verify manifest/config/compressed-layer hashes and uncompressed diff IDs; import
  layers as in-memory data, never host extraction or container execution. Whiteouts,
  special/privileged entries, duplicate paths and unsafe link ancestry fail closed.
  Input SGID is allowed only on directories. Named directories are normalized to
  0555 in memory and in the emitted archive (for example, `var/local` mode 02775).
  Root directory markers `.` and `./` are ignored, including SGID-only markers,
  but are still checked for setuid and capabilities before being ignored.
  Setuid on every entry type, SGID on every non-directory (including links), and
  any truthy exact `SCHILY.xattr.security.capability` PAX value on any entry type
  remain rejected with `layer-privileged` before that entry changes the tree.
  Public download redirects are limited to Node/GCR/Google Storage HTTPS origins.
- Assemble an untagged single-layer Docker-save-shaped archive with fixed Linux/
  amd64, UID/GID 65532, `/work`, no command/entrypoint/environment or execution
  defaults. Normalize archive metadata and runtime write permissions. Inventory
  and separately decode the resulting bytes; compare two same-process serializations.
- Upload only seven named preparation files to this Actions run, retained seven
  days: image.tar, package.json, package-lock.json, inventory.json, owned-inputs.json,
  preparation.json and SHA256SUMS. Never upload the npm cache, home, client configs,
  whole checkout, tokens or logs. The package manager does not inherit the job token.

No Docker daemon is accessed; no image is loaded, pushed or registered; neither
ESLint nor the launcher runs. GitHub Actions infrastructure actions use their own
reviewed runtimes but do not receive authority to execute the prepared tool.

## Supported archive and resource profile

The Go readers explicitly reject global PAX headers rather than ignoring their
metadata. Local PAX is supported within the bounded profile; unsupported sparse/
GNU extensions and malformed framing fail closed. This is deliberately narrower
than the legacy Python reader, not a Python-format parity claim. Compatibility
of the actual pinned archives with every profile restriction has not been proved
by synthetic tests or this source cutover.

Path/link validation rejects escapes, unsafe ancestry and cycles. Node host-link
paths/targets have a 16 KiB byte cap, alias resolution has a shared deterministic
work budget, and pending resolution state has a 512 KiB weighted accounting cap.
Costlier finite aliases can also be rejected. Rootfs data/metadata and file counts
remain bounded (512 MiB and 50,000 entries); installed data additionally has a
128 MiB cap. JSON nesting is capped at 1,000. These technical accounting limits
are not elapsed-time, peak-memory, host-isolation or real-pin compatibility proofs.

## Failure diagnostics

Preparation failures exit nonzero with a single owned diagnostic, for example
`stage=npm-ci; category=process-failed`. Stages distinguish host inputs, Node
download/extraction, the owned manifest, npm setup/lock/install, base import,
rootfs assembly, image validation and artifact writing. Nested npm failures retain
their phase rather than being relabeled as setup failures.

Base import has nested stages for manifest/config retrieval and parsing, platform,
layer descriptors, compressed download, decompression, diff ID verification and
in-memory import. Known invariant failures add a fixed allowlisted `reason`, such
as `stage=base-layer-import; category=validation; reason=layer-ancestor`.
Reasons distinguish digest/size identity, unsupported layer media type, layer count,
diff IDs, duplicate JSON keys and rejected archive paths, links, privileges,
whiteouts, special entries, type replacements and resource bounds. Unknown failures
still report only stage/category; a missing reason does not imply acceptance.
No layer index, archive member name or external error text is logged. Existing
digest pins, bounds, origin restrictions and fail-closed import checks are unchanged.

Stage and category values come from fixed allowlists, never exception messages,
URLs, filesystem paths, subprocess arguments or package output. Categories identify
HTTP, network, timeout, filesystem, archive, validation, failed npm exit or denied
proxy origin; unclassified failures remain `unexpected`. A category is not proof
of the underlying cause. npm stdout/stderr remain discarded; top-level failures
do not print raw tracebacks, and logs are not uploaded as artifacts. Diagnostics
do not relax any preparation or admission gate.

## What the artifacts do NOT prove

Node remains on the previously documented security hold. Package lock generation
selects transitive versions at that moment; the returned lock and tarball integrity
must be independently reviewed before any catalog entry or later reproduction.
The image is a candidate, not a production runtime. ELF/shared-library compatibility
is NOT yet checked. Distroless provenance and full file policy still need review.
The archive inspector compares against the prepared inventory; it is not the
independent trusted lint inspector required for production admission.

Repeated serialization checks determinism within this process, not independent
clean offline reconstruction, upstream binary reproducibility, runtime compatibility
or enforcement. Exact Node/ESLint formatter compatibility, bounded runner connection,
authenticated one-shot consent and actual isolated CLEAN/FAULT/RESTORED evidence
remain separate requirements. No dispatch, Environment approval, artifact checksum
or exit code closes issue #9 or approves real ESLint execution.
