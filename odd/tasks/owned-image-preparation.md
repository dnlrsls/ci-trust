# Prepare owned images and bind protected plan review
Objective: source-only Linux CI preparation for issue #8 and newly authorized PR publication; no merge, manual dispatch or container execution.
Branch: feat/owned-image-preparation; baseline b318d8d; auto-chain/feature-branch-chain; publication authorized by S4, no merge/runtime authority; Go 1.26.

## Specs
### S1 — Current user request (verbatim)
sigamos con lo recomendado, que necesitas de mi?

sip, tienes mi autorizacion, avanza

### S2 — Accepted preparation proposal (verbatim)
Necesito **tu autorización para preparar la imagen controlada y su flujo de verificación en Linux CI**, en lugar de instalar Docker en tu Windows.

Ese primer paso incluye:
- Preparar una imagen mínima con sólo nuestro probe, sin código del repositorio objetivo ni credenciales.
- Verificar su contenido, configuración e identidad inmutable.
- Implementar la aprobación humana vinculada al plan exacto.

**Antes de ejecutar contenedores**, te voy a mostrar el plan, los límites y los efectos para que lo apruebes explícitamente. Esta autorización inicial no incluye ejecución, publicación ni cambios en tu equipo.

La licencia la podemos resolver antes del corte de release; no bloquea este trabajo.

¿Autorizás esa preparación en Linux CI con esos límites?

### S3 — Existing accepted issue #8: Issue #8: [P1] Gate controlled verification behind consent and bounded isolation
## Problem
Executing validators may run arbitrary code, expose secrets, or exhaust resources; static audit must not silently become execution.

## Motivation
Establish a narrow fail-closed boundary before validator sensitivity probes exist.

## Scope
- Define explicit consent to a displayed plan containing target snapshot, adapter/tool identity, fixture/config scope, resource limits, and effects.
- Implement a first existing-runtime container runner for owned controlled fixtures, without writable host repository mounts, credentials, or network.
- Enforce wall-time, CPU, memory, process/output limits, cleanup, and cancellation. Separate harness and infrastructure outcomes.

## Non-goals
- No automatic audit execution, arbitrary repository scripts/actions/plugins, package installation, implicit image pulls, host-shell fallback, privileged containers, or custom sandbox.
- No absolute security guarantee or automatic runtime provisioning.

## Acceptance Criteria
- [ ] Without explicit consent to the current plan, no verification process/container starts and no repository file/counter changes. Snapshot/tool/image/limit changes invalidate consent.
- [ ] Unavailable runtime or unenforceable promised limits return UNVERIFIABLE with infrastructure reason before execution; there is no unsandboxed fallback.
- [ ] Owned controlled probes cannot access host credentials/network, write source files, or escape allowed scratch space under a documented threat model.
- [ ] Timeout, memory/process/output bounds, cancellation, and normal termination produce bounded observable outcomes and cleanup evidence; infrastructure/harness errors are not validator failures.
- [ ] Pre-provisioned immutable tool-image identity is recorded. No implicit downloads or credential pass-through occur.
- [ ] Document supported runtime/host combinations, residual container risks, and missing proof explicitly. Linux/Windows/macOS hosts may require an installed Linux-container backend.

## Technical Considerations
Use an established isolation runtime; do not build a sandbox from scratch. Validate arguments without shell interpolation. P0 must work without any container runtime.

## Dependencies
- https://github.com/dnlrsls/ci-trust/issues/1
- https://github.com/dnlrsls/ci-trust/issues/2

## Testing Strategy
Fake-runner tests for denial/stale consent/preflight; real isolated-runtime tests for no network/secrets, source immutability, resource bounds, cancellation, cleanup. Use owned fixtures only; report unavailable isolation proof.

## Priority
P1

## Suggested Size
M


### S4 — Publication authorization (verbatim)
¿autorizás publicar los PRs de esta preparación?

sip, hagamos eso

## Tasks
- T1 | S1,S2,S3 | delegated writer (parallel independent image-archive unit, >100 lines), internal/ownedimage/** + docs/owned-image.md | done | c297f2504d89fb6b7841269b7f9a059b48b5d222; source-only/independent PASS; cohesive1036-line unit
- T2 | S1,S2,S3 | delegated writer (parallel independent exact-plan unit, >100 lines), internal/controlledplan/** + docs/controlled-plan.md | done | 26ebc6620c82133d26b576085f2d10b6b921c1f7; source-only/independent PASS; cohesive491-line unit
- T3 | S1,S2,S3 | inline integration, manual source-only image-preparation workflow and source docs | done | 7a097b59d97570260e6e46f10c6fff9f57e577f9; structural RED1→GREEN0,3shell bodies parse/public seam+independent PASS
- T4 | S1,S2,S3 | parent checks + independent high-risk verifier + local work-unit commits | done | source commits c297f25/26ebc66/7a097b5; fullsuite/vet+independent PASS,17raw/canonicalGitchecksPASS,cleanupPASS; risks2/5; real Linux/Docker/gate proof outside source-only scope

- T5 | S4 | inline destination/policy/chain reconciliation | in_progress | inspect remote main, issue8, existing PRs/labels; preserve accepted source blobs
- T6 | S4 | inline branch-chain publication | pending | draft/no-merge tracker + focused children; Refs #8, never auto-close #8/#9
- T7 | S4 | inline native PR CI observation and public readback | pending | exact published SHA/all3hosts; no merge or manual workflow dispatch

## Log
L1 — User request verbatim: sigamos con lo recomendado, que necesitas de mi?
L2 — User authorization verbatim: sip, tienes mi autorizacion, avanza
L3 — Reconciliation: read canonical Engram 45144 and full fragments 45161–45166 (149924 UTF16 chars through L144), checked private c13 Git task doc size149617 and main task doc through L35. Main source b318d8d is clean. The older main tracker is historical; retain it and private branches unchanged. This focused continuation does not resume old broad delivery authority. #8/#9 remain open.
L4 — Scope: implement preparation source, tests and docs; no Docker command/image load/build, container, runtime provisioning, target execution, Windows change, push, PR, license or release operation in this unit. Existing GitHub Environment configuration is not current-plan consent.
L5 — Design: avoid builder-injected PATH by constructing a bounded deterministic image archive with an explicit empty execution config, not by weakening runner preflight. Build only owned static Linux/amd64 probe; owned marker and fixed paths only. Independent archive decoding must check content, hashes, configuration and tar entry policy. No new dependencies. Archive/metadata are preparation data, not production catalog admission.
L6 — Independent contracts: T1 CLI at internal/ownedimage/cmd/owned-image: pack --binary FILE --marker FILE (tar stdout), inspect --archive FILE --binary FILE --marker FILE (JSON stdout). Inspection metadata fields image_id, archive_sha256, binary_sha256, marker_sha256, layer_diff_id, scope. scope is preparation-only. T2 takes image ID independently, never imports T1: internal/controlledplan/cmd/controlled-plan prepare --snapshot SHA256 --image sha256:HEX --revision GIT40 --run-id DECIMAL --run-attempt DECIMAL; check --input FILE --binding HEX --revision GIT40 --run-id DECIMAL --run-attempt DECIMAL. Prepare emits JSON envelope including plan, plan_digest, binding_digest, revision/run binding. Check rederives exact fixed runner.Plan and all envelope fields, refusing tampering/stale/substituted inputs. Neither command creates runner.Consent or starts anything.
L7 — Fixed plan uses existing runner.Plan digest/display, owned-smoke-v1/smoke-v1/owned-v1; UID profile is existing runner behavior, limits wall10s/memory128MiB/CPUs1/PIDs32/output64KiB/scratch8MiB; effects exact existing allowlist. Envelope binds repository dnlrsls/ci-trust, revision, run id, run attempt, snapshot and image. No public verify mode or production catalog in this source unit.
L8 — Integration design: new manual-only Linux workflow, fixed main/repository restriction, contents:read, credentials disabled. Preparation produces and displays complete plan before protected Environment review; protected successor verifies exact same run/attempt/revision/binding after gate. Never creates a Consent boolean or runs a container. Image archive loading and observed daemon attestation are a future separately approved preparation run, not local proof. No remote settings changed.
L9 — Verification: tests first with observed RED then GREEN, independent literal archive/plan expectations, exact CLI exit/stdout/stderr and rejection input/counter immutability; fake data only. No new shared audit/report/bootstrap code: differential campaign N/A unless integration changes their helpers. Parent full suite/vet/format/whitespace and one independent security/CI verification. Real Linux/image import/protected-gate outcomes remain explicitly unavailable.
L11 — Parent workflow rule test observed actual RED exit1 (missing workflow, package0.414s, stderr empty), then GREEN exit0 after adding manual-only main/repo-restricted preparation plus protected dependent review. The workflow does not invoke Docker, generate Consent, publish or execute the probe. Docs updated with offline/daemon/policy proof boundaries. T1/T2 workers remain active; no package-seam or full-suite result yet.
L10 — Review forecast: two independent cohesive units plus integration, about900–1200 authored changed lines; local commits by behavior, no compression or omitted assertions to force400. Cohesive overages disclosed. No public PR delivery strategy selected because publication is explicitly excluded; decide delivery shape only if later authorized.

L12 — T2 initial worker returned source and direct command tests GREEN1.143s, but public OS-process proof/formatting incomplete. Same writer continuation mva11ahx-4-k06b completed that explicit requirement in one bounded batch: real main helper exact streams/exits, valid/invalid/stale source and sentinel/inventory immutability, subcommand help, output failure exit1; behavioral RED1/1.524s then GREEN0/1.427s. Root cause: help ignored writer errors and returned false success; fixed explicit output-error handling. Parent read all source/process tests; focused controlledplan+workflow once exit0/empty stderr, no skips. T2 source outcome done; independent security verification and local commit remain T4. T1 active. Parent compile-only Linux probe job-1 exit0/4s/empty streams, binary SHA256 f224b49310096c7d7351abe408ad6c0e963b02fc4c3c89946b152dfe2414c30d NEVER executed. No Docker/image operation or publication occurred.

L13 — T1 initial worker returned partial after focused packages GREEN(.528/.718s), with actual ABI and metadata-bound behavioral RED preserved separately from compile-only RED. Parent source review found documented help-error diagnostic mismatch and JSON case-alias ambiguity; same writer continuation mva15638-5-c8g6 fixes those with RED/GREEN, adds dedicated symlink rejection proof, and formats source in one bounded completion batch. Parent workflow shell syntax check passed all3 bodies without executing them. Public image→plan seam harness is prepared under excluded .git scratch, NOT RUN while T1 changes. No T1/T3 acceptance, full-suite, security-verifier or commit outcome yet.

L14 — T1 completion mva15638-5-c8g6 returned complete: alias+help-error behavioral RED1/1.552s then GREEN0/1.611s (.480/.770pkg), dedicated symlink rejection succeeds on this host with link/targets/counter/inventory unchanged. Root causes: encoding/json accepts case-insensitive aliases despite DisallowUnknownFields; help discarded diagnostic on writer error. Fixed exact schema-key validation and fixed output-failure stderr without loosening empty config. Parent read changes and focused ownedimage once exit0; gofmt empty. T1 source-only done. Native ASSESS riskHIGH via explicit item2 (alsoCIitem5), nativeRiskunassessable because untracked scope; RDDoff/independentVerifier=true, no staging/retry/lifecycle workaround. Public seam job-2 and fullsuite→vet job-3 ACTIVE; no PASS yet. No independent verifier or commits yet.

L15 — Parent job-2 seam COMPLETEexit0/3s: built only preparation tools; packed/independently inspected actual compile-only ELF into2354688byte archive, IDsha256:47e55372942f3e33a6a5e85d292400447ae475ffbe28e2d55e675ee6dd1ea0b6/archive465a2b8942609d24b6b759aba798c6e75dbe868700abafcebfdfc4610445293b. Synthetic-context plan prepare/check exact-byte roundtrip; stale attempt exit2/fixed stderr/emptyout, invalidbinary exit1/fixedstderr/emptyout; inputs/counter/inventory unchanged. This is archive data, NOT daemon image loading/attestation or approval. Job-3 fullsuite and vet COMPLETEexit0/16s/empty separate stderr; original public CLI still passes. Source structural17files/readback manifest68e34b424a301fcfd30bb69eb9783fa57a41d95728563ecee9bf925f141278c3, links/whitespace/gofmt passed. Readback total2118 is full file lines including unchanged docs, not authored diff count. T3done; T4independent security/CI verification next, no production proof/publication or commits yet.

L16 — Independent verifier mva1c621-6-s7w5 ACTIVE on frozen17-source-file manifest68e34b; source-only S1/S2 and bounded S3, no new runtime acceptance claim. Parent full readback confirms15packages PASS/no actual ---SKIP/---FAIL records; semantic test names containing SKIPPED are not skips. Local-only three-unit commit plan prepared in excluded scratch: image1036authoredlines, plan491, integration plus focused tracker; cohesive overages disclosed, no assertions compressed/removed. No commit/index/push/PR/merge/dispatch mutation yet; all publication remains excluded. Worker-reported image-scratch and plan-scratch contain caches only; cleanup after verifier outcome.

L17 — Independent verifier focused isolated test PASSexit0/2006ms/5packages/empty stderr/no skips; final verdict pending one concrete ELF section-name-table allocation premise. Child setup disclosed missing prefix/Python unavailable/GNUtar Windows-colon archive rejection, resolved using relative tar extraction before the single test; no source mutation or test retry. Parent denied implied outside-project Go file access; fetched official public Go1.26.4 debug/elf constructor excerpt and complete internal/saferio source into excluded project evidence, NOT installed-file inspection. Same reviewer now evaluates that one premise without dangerous allocation probe/extra campaign. Source manifest unchanged; no commits, native lifecycle or runtime operation.

L18 — Independent frozen final mva1c621-6-s7w5 PASS: S1/S2 and bounded preparation-only S3 PASS, no blockers; single focused isolated run exit0/2006ms/fivepackages/empty stderr/no skips;19of19 source/task/manifest hashes unchanged at reviewer boundary. Advisory A1 image.go82–85: table/file limits do NOT bound adversarial ELF section-name decompression/allocation in debug/elf.NewFile; public Go1.26.4 NewFile→shstr.Data→Section.Open→saferio.ReadData confirms chunking is not total decoded cap. Nonblocking ONLY because expected ELF comes from separately controlled owned build, never target input; untrusted archive payload is byte-compared, not ELF-parsed. Preserve that trust contract; do not generalize CLI. No dangerous probe or installed-source read. Real daemon/load/config/gate-humanapproval/catalog/isolation remain unverified and outside this source unit. Parent corrected receipt basename after one ENOENT: actual focused.receipt.json (not receipt.json), no lost evidence or test retry. Next local three commits, no publication.

L19 — Local image work unit committed c297f2504d89fb6b7841269b7f9a059b48b5d222/tree52bbce09661f09249ee9256b232a9fc0d6db66f5,7files/+1036, staged whitespace passed. Risk2 unit remains alone, no source rewritten after verified manifest. Before next commit choose reversible auto-chain/feature-branch-chain review organization under live standing permission; future public delivery still EXCLUDED and needs separate authorization. First slice boundary b318d8d→c297f25 is cohesive image producer+independent inspector+process/literaltests+docs; disclosed636-line overage, never minified/deleted assertions. L10's deferred strategy is historical; this selection organizes local units only, creates no PR/remote branch/review or runtime authority. Next slice exact-plan491lines; final integration slice closes this preparation feature. Existing Git LF/CRLF warning for markdown is expected policy, not source mutation.

L20 — Local plan work unit26ebc6620c82133d26b576085f2d10b6b921c1f7/tree93b78947807c80d80437a16d2ccf3da07348d241,6files/+491/staged-whitespacePASS. Review slice c297f25→26ebc66 is fixed profile/envelope+strictJSON/runcontext+realmain tests+docs;91-line cohesive overage disclosed. Auto-chain/feature-branch-chain remains local organization only; no public shape exception requested or granted. Parent preserved independent focused receipt/stdout/stderr/before/after byte-for-byte in .git/owned-image-preparation/verify-evidence, then removed exactly4session-generated worker/verifier scratch directories (cleanup-receiptPASS); parent proofs/archive/source remain. Plan adjusted to3implementation commits plus a final small tracking-only checkpoint: record actual implementation commit IDs without impossible self-reference or dirty tracker after source closure. Next integration unit excludes tracker, then observed final source commit closes T4 before tracking checkpoint.

L21 — Source preparation CLOSED LOCALLY after observed final integration7a097b59d97570260e6e46f10c6fff9f57e577f9/tree403f3b0e9d903f6578c6bd929f42ab1c7069bffa,4files/+293/-5 (298authoredlines). Slice26ebc66→7a097b5 contains manual workflow, structural test and corresponding docs; prior source slices1036/491, total1825authoredsourcechanges. Parent final check17/17rawfiles unchanged from independently reviewed68e34b manifest and17/17existing-filter canonicalGitblobs match committed source, staged/tracked diff empty (only new focused tracker awaiting checkpoint). T1–T4done; current applicable tests/vet/format/shell/links/sourceintegrity allPASS, no actual skips/current unexpected failures. Native assess unavailable for untracked scope was not retried; required independent source verifier PASS fulfills named-risk plan, not native RDD approval. Snapshot/archive image is preparation data ONLY, binary neverexecuted, no daemon/image load/container/runtimeproof/humanapproval/catalog; #8/#9remainOPEN. Next NEEDS HUMAN DECISION: authorize publication/PR delivery of these local units (cohesive1036/491overages explicitly disclosed, no repo-specific size-exception label invented) before any main-only Linux preparation run. This closes only current source scope; it does not authorize publication, merge, dispatch or real execution. Final small tracker checkpoint records these known source IDs; its own eventual hash is recorded only in Engram to avoid self-reference.

L22 — User publication approval verbatim: sip, hagamos eso . This answers the preceding exact question in S4; supersedes earlier no-publication restrictions ONLY for nonforce pushes/PR creation and necessary ordinary portable CI. No merge/issue closure/manual isolation-preparation dispatch/Docker/runtime/image provisioning or new execution consent. Original source checkpoints8a7aa15/c297f25/26ebc66/7a097b5 preserved. Extend current feature tracker with T5–T7 and existing auto-chain/feature-branch-chain rather than create competing source plan. Need materialize tracker-only base plus children preserving exact17sourceblobs; no replay old delivery helpers.

L23 — Read-only destination check job-4 exit0: dnlrsls/ci-trust ADMIN/defaultmain, remote main stillb318d8d, no open PRs or conflicting remote delivery branches; issue8OPEN/enhancement. Repository has no PR template, approved/type/size labels or Gentle-owned PR gate: use Refs #8 (not closure keywords), existing enhancement/documentation labels only; do not create labels/close issues or import unrelated Gentle repository policy. First shellcheck on three extracted workflow bodies exited1 ONLY informationalSC2016 on intentional literal Markdown backticks in a printf. No source/runtime defect; one scoped rerun excludingSC2016 passed0/emptyoutput. All source bytes remain frozen; this explicitly scoped false-positive adjudication is not an unavailable-check retry or CI proof. Publication chain will use tracker-only doc root and3children via new worktree/cherry-picks, retaining original local history and exact17Gitblobs; no force push or merge.
