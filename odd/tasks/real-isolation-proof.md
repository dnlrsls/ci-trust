# Complete one owned real isolation route
Objective: complete only issue #8 using existing contracts; no Docker/load/provision/security changes before exact human authorization.
Original implementation branch: feat/issue8-real-isolation (preserved at4c7c7b9); base2c665fd. Published source-chain head: feat/issue8-protected-proof-workflow at eb12dfb37c7496344a4f2cf6a6ad575dd8801551. Go1.26.4 local/offline tests; no merge/closure without maintainer approval.

## Specs
### S1 — Full user request (verbatim)
```text
# CI Trust — Complete Real Isolation Verification (#8)

Trabaja sobre el repositorio `dnlrsls/ci-trust`.

## Objetivo

Quiero completar el siguiente hito de CI Trust:

**Demostrar una primera ejecución real de una herramienta controlada dentro de Docker, con autorización humana explícita, aislamiento verificable, límites de recursos y limpieza comprobada.**

Ya hemos invertido considerablemente en preparar el runner. No quiero más infraestructura especulativa ni nuevas abstracciones que no sean estrictamente necesarias.

**Prioridad: convertir lo que ya implementamos en evidencia real de funcionamiento.**

## 1. Inspección inicial

Antes de modificar código:

1. Revisa el estado actual de `main`, el issue #8 y los PR #35–#45.
2. Inspecciona `internal/runner`, `internal/ownedprobe`, `internal/ownedimage`, `internal/controlledplan` y los workflows relacionados.
3. Identifica qué componentes están completos, cuáles utilizan únicamente fakes/mocks y cuáles impiden una ejecución real.
4. Identifica las dependencias mínimas para completar una única ejecución controlada.
5. Reutiliza los contratos y mecanismos de seguridad existentes.

No implementes nuevamente lo que ya funciona.

## 2. Completar la ruta mínima de ejecución real

Necesitamos conectar de manera segura:

`Reviewed source → Verified image → Trusted catalog → Exact execution plan → Human consent → Docker execution → Evidence → Verified cleanup`

Requisitos:

- Utilizar exclusivamente una imagen propia cuyo contenido e identidad SHA-256 hayan sido revisados.
- Comprobar el archivo de imagen y su configuración final contra el contenido esperado antes de admitirlo.
- Verificar la identidad de la imagen observada por Docker después de cargarla.
- Conservar el vínculo entre commit, snapshot, imagen, herramienta, configuración, límites y plan aprobado.
- Mostrar el plan completo antes de solicitar consentimiento.
- Obtener la autorización mediante un mecanismo humano confiable, nunca mediante un booleano suministrado por el agente.
- Rechazar cualquier cambio del plan después de su aprobación.
- Ejecutar únicamente `owned-smoke-v1`, usando el runner y las restricciones existentes.
- Garantizar límites de tiempo, memoria, CPU, procesos, salida y almacenamiento temporal.
- Verificar la limpieza del contenedor mediante su identidad y propiedad, sin eliminar recursos ajenos.

**No interpretes GitHub Environment approval, `workflow_dispatch` ni un argumento `approved=true` como consentimiento suficiente por sí mismos.** El mecanismo elegido debe vincular verificablemente la decisión humana al plan exacto.

Si el entorno no cumple los requisitos, devuelve `UNVERIFIABLE` sin utilizar un fallback inseguro.

## 3. Seguridad y autorización

No ejecutes Docker, cargues imágenes, aprovisiones recursos ni cambies configuraciones de seguridad sin aprobación humana explícita del plan concreto.

Puedes realizar inspección estática, implementar código y ejecutar pruebas inocuas sobre componentes propios.

Si la comprobación real requiere autorización, presenta:

- Entorno donde se ejecutará.
- Identidad exacta de la imagen.
- Comandos y efectos previstos.
- Recursos y límites.
- Riesgos residuales.
- Evidencia que se capturará.
- Procedimiento de limpieza.

Detente únicamente ante esa autorización necesaria. No la simules.

No ejecutes código del repositorio auditado, no expongas credenciales, no habilites red, no uses contenedores privilegiados y no montes el repositorio anfitrión.

## 4. Verificación real

Una vez obtenida la autorización correspondiente, ejecuta un conjunto acotado de pruebas reales sobre el entorno aislado.

Comprueba, mediante observaciones independientes y sin confiar únicamente en la configuración declarada:

- Ejecución como usuario sin privilegios.
- Restricciones de capacidades y seguridad.
- Ausencia de conectividad de red.
- Ausencia de credenciales del anfitrión.
- Sistema de archivos raíz de solo lectura.
- Escritura limitada al scratch autorizado.
- Aplicación efectiva de límites de CPU, memoria y procesos, hasta donde permita una prueba segura.
- Timeouts y cancelación.
- Límites combinados de stdout/stderr.
- Limpieza y ausencia posterior del contenedor propio.
- Integridad del repositorio original.

Distingue lo **observado**, lo **inferido** y lo **no verificable**.

Una prueba que no pudo ejecutarse no cuenta como PASS.

No conviertas un exit code exitoso en evidencia automática de seguridad o eficacia.

## 5. Corregir bloqueos reales

Investiga especialmente la interacción entre el límite de dos segundos del cliente Docker y el límite mayor previsto para la ejecución.

Si esa restricción impide una ejecución válida, corrige el problema sin eliminar los límites de seguridad.

Cualquier otra corrección debe justificarse mediante un fallo reproducido o una incompatibilidad demostrable.

Evita refactors no relacionados, nuevos frameworks, abstracciones innecesarias o la creación de otro sistema de orquestación.

## 6. Pruebas y criterios de aceptación

Mantén los tests existentes y añade únicamente los necesarios para respaldar los nuevos comportamientos.

Exijo:

- Pruebas negativas de consentimiento inválido o caducado.
- Rechazo ante identidad de imagen o snapshot diferente.
- Clasificación separada de fallos del runtime, harness y herramienta.
- Evidencia real de aislamiento en el entorno autorizado.
- Comprobación de que no quedan contenedores propios después de una ejecución válida.
- Ausencia de escrituras inesperadas.
- Resultados reproducibles asociados al commit exacto.
- Pruebas Go y `go vet` satisfactorias.
- Validación multiplataforma del código cuando corresponda, sin presentar Windows/macOS como hosts con aislamiento Docker verificado si no se probaron.

Si un criterio no puede demostrarse, decláralo explícitamente.

## 7. Alcance

Este trabajo pertenece exclusivamente al issue #8.

NO implementar todavía:

- Sensibilidad de ESLint (#9).
- Mutation testing.
- Ejecución de tests arbitrarios del repositorio.
- Integraciones con agentes.
- MCP o dashboards.
- Generación automática de PRs.
- Nuevos motores de CI.
- Capacidades que requieran ampliar innecesariamente la superficie de ejecución.

**RDD permanece desactivado.**

Utiliza verificación focalizada, pruebas de regresión y revisión independiente proporcional al riesgo. No introduzcas un nuevo protocolo obligatorio de recibos o revisiones.

## 8. Entregables

Al finalizar, presenta:

1. Estado inicial y bloqueos identificados.
2. Cambios mínimos implementados.
3. Evidencia real de la ejecución, incluyendo revisión exacta, imagen y resultados.
4. Resultados de las pruebas de seguridad.
5. Estado de limpieza y recursos.
6. Riesgos residuales y garantías que siguen sin demostrarse.
7. Criterios de aceptación del issue #8 cumplidos y pendientes.
8. PRs relacionados y recomendación de cierre.

Trabaja en PRs pequeños y revisables cuando existan unidades independientes. No fragmentes artificialmente una implementación de seguridad de modo que el comportamiento quede separado de sus pruebas.

No hagas merge ni cierres el issue hasta contar con evidencia suficiente y autorización del maintainer.

## Principio final

**Evidence before guarantees.**

No quiero una implementación que simplemente parezca segura. Quiero pruebas verificables de lo que realmente podemos garantizar.

Nuestro objetivo no es construir otro Gentle Shell.

Nuestro objetivo es convertir CI Trust en una herramienta independiente, determinista, segura y capaz de demostrar la eficacia de las verificaciones que realiza.
```

## Tasks
- T1 | S1 sections1–8 | read-only parent history + delegated bounded source map | done | main2c665fd/issue8OPEN/PR35–45MERGED; explorer mva3op0l-7-cyd5, no source writes before map
- T2 | S1 sections2,5,6 | delegated runner unit; independent from T3, >100lines | done | actual2.002sRED→2.308sGREEN/directwall~102ms/config+legacyPASS; parentfocusedexit0; localcommit4c7c7b964f3d1a52ca9e9cf41788808b3d99efd4; independentcomposedreview pending
- T3 | S1 sections2,4,6 | owned-probe unit | done | source/inert tests and independent static source review accepted for source PR only; current Linux rebuild/pack/Inspect reproduces all five pinned image digests; native binary never executed; real resource observations belong to T6
- T4 | S1 sections2,3,6 | inline minimal composition | done | catalog/auth gate + stdin loader/post-load observation + four-case existing-runner evidence + internal prepare/execute command + default-off protected workflow implemented/tested; no workflow dispatch or policy edit
- T5 | S1 sections3,5–8 | focused/full/vet + independent static security review | done | full uncached Windows tests/vet + Linux/Darwin cross-builds + workflow shell syntax/lint PASS; independent static full-source review P1/P2 corrected and bounded recheck READY FOR SOURCE PR; no independent runtime execution or global isolation acceptance
- T6 | S1 sections3,4,6,8 | explicit human concrete-plan authorization then real proof/cleanup/report | pending | requires exact environment/image/commands/limits/risks/evidence/cleanup approval; missing proof is UNVERIFIABLE, not PASS; no premature merge/closure

## Log
L1 — Original request is preserved verbatim in S1 and .git/issue8-real/authorized-request.md.
L2 — Initial evidence: main2c665fd clean; issue8OPEN; PR35metadata/36–38runner/39–41probe/42–45archive+plan allMERGED. Five packages and related workflows mapped before source modification. Existing image pack/independent inspector, plan digest/envelope, local Docker preflight/start/ID+owner cleanup and source snapshot callback are reusable. Tests/portable CI remain fake/code evidence, not real isolation.
L3 — Demonstrated blockers: docker.go caps EVERYclient call2s while the fixed plan allows10s; source smoke explicitly lacks network/credential/resource-enforcement proof; no production catalog, image load/post-load observation, exact human execution consent, or composition entry exists. Source-only preparation run38815617465/job116699417920 succeeded onmain and awaits owner review with empty approval history, but never used Docker and cannot authorize this new execution.
L4 — Route: parent does only glue/demonstrated fixes. Parallel writer trigger holds for T2runner and T3probe: independent behavior units, disjoint packages/docs, each several minutes/>100lines; launch together. T4depends on those contracts and image identity, so parent works inline rather than invent another worker/orchestrator. Risk2security/4bounded concurrency in resources/5opt-inCI; independent verification at composed source boundary. No source schema/framework rewrite, new dependency, mandatory receipt protocol, #9 or target-code execution.
L5 — T2 contract: keep legacy owned-v1 valid/byte-for-byte Docker vector. Add ONLY fixed config owned-proof-v1 for the SAME adapter/tool owned-smoke-v1, same fixture smoke-v1 and limits; dockerArgs derives --config from already-validated fixed allowlist. Metadata/creation/inspection/removal/list calls retain2s per-client cap/64KiBcombined bound/WaitDelay200ms, cleanup parent3s. Attach may honor earliest own-plan wall/caller deadline, with hard max1minute (existing valid wall ceiling); direct LocalDocker.Start must also impose its owned plan deadline, never become unbounded. No new caller-controlled timeout flag, arbitrary argv, environment/credential forwarding or cleanup weakening.
L6 — T3 contract: legacy --fixture=smoke-v1 --config=owned-v1 output and rejection semantics remain unchanged; help is updated only to describe the new opt-in profile, still exit0 with no observations. Opt-in --config=owned-proof-v1 uses SAME owned-smoke-v1 binary/image and existing UID/cap/seccomp/marker/cgroup guards BEFORE any real operation. Proof stdout first emits owned-smoke: proof-started\\n; emit fixed owned stderr marker to exercise both streams; then one bounded JSON observation line. Wire shape {version:1,scope:"owned-smoke-v1",checks:[{name,state,detail,before,after}],limitations:[...]}; states OBSERVED/INFERRED/UNVERIFIABLE, never generic securityPASS. Fixed check names nonroot,capabilities,no_new_privileges,seccomp,root_readonly,scratch_write_cleanup,scratch_limit,network_unreachable,host_credentials_absent,cpu_limit,memory_limit,pids_limit. No credential values/system errors in output. Per-effect numerical counters when meaningful; tool normal exit0 only if required bounded observations available, otherwise explicit unavailable.
L7 — Probe constraints: scratch fixed owned path(s) under /tmp only, exclusive ownership and cleanup, bounded<=8MiB+one-byte ENOSPC attempt using small chunk memory. Network proof must not emit external traffic: kernel UDP connect route check to documentation-only address WITHOUT send plus namespace routes/interface observations, no network enable or scans. Credential checks use ONLY harmless known host sentinels/allowlisted container environment and fixed absent paths, never read actual host credentials. CPU<=1quota bounded demand and observed throttle counters; memory/pids use short-lived children of SAME immutable owned binary only, no shells/other executables/plugins. Guard child modes equally; bound child lifetimes/allocations/counts and reap owned children on every return. Expected child OOM/thread-limit denials are observations, not validator efficacy. Report unsupported outcomes honestly; no host-valid probe execution during implementation.
L8 — Composition intent, not extra requirements: one fixed internal command/package is justified to connect existing APIs. One reviewed image catalog entry only, strict content/config/archive check before any load; Docker-observed ID/defaults after load; existing local client/env restrictions. Existing runner.Plan.Digest/Display and controlledplan binding reused, never agent-supplied Approved. Investigate existing Environment owner approval PLUS exact full-plan binding in human-entered review comment/current source+run+attempt+expiry; Environment alone is explicitly insufficient. Official REST exposes reviewer/user/state/comment and review UI accepts comments; no approve API will be called by agent.
L9 — Demonstrated execution constraint: current registered workflow onlymain; saved Environment accepts ONLYmain, so feature code cannot prove itself under that policy without a separately authorized narrow plan. Do NOT merge first to obtain proof. Any source workflow adaptation may be implemented/tested inertly; any branch-policy exception, hosted provisioning, Docker/load/real probes require explicit concrete-plan approval later. Keep existing policy unchanged during source work. No automatic PR-generation PRODUCT feature; any delivery is reviewable source units, no merge/issue closure without evidence and maintainer authorization.
L10 — Tests: one observed RED per requested new rule where meaningful; public/direct owned seams assert exact exit/streams/unchanged inputs/counters and legacy behavior of shared helpers. Real operations are forbidden in unit/native-host test suites; fake/inert fixtures only. Parent compiles Linux artifact only, never executes it on Windows/host. Focused GREEN, existing suite/vet and differential base/new sibling commands where shared parsing/helpers change; no retrospective RED claims. Existing portable CI can demonstrate code portability only. Go cache/temp stay project-owned .git/issue8-real/** with offline tool settings.
L11 — Forecast about1500–2200authoredlines across three cohesive behavior units including tests/docs; security behavior stays with its tests. About400 is review guide, not reason to minify or split safety assertions. Choose local auto-chain/feature-branch-chain review organization; no delivery/merge/security/runtime authority follows from it. No new public source edit has occurred before this document/mirror.

L12 — Runner writer mva42x6j-8-lrvp COMPLETE: real owned-executable RED2.002s attach/directwall mismatch + proof-config rejected; GREENattach2.308s/earliercaller+ownwall~102ms, fixedconfig legacyvectors/source+counter unchanged, focused5.200s. Parent read source/tests/evidence and focused regression onceexit0 (runner-parent stdout/stderr preserved); canonicalbugfix45172 persisted immediately, root cause globalmetadata2s cap +directStart relyingonRunner. T2sourceoutcomeDONE, independentsecurityreview remains composedT5; no realDocker claim. T3writerinitialPARTIAL onlywire/fake serialization, nativeObserveProofabsent; onebounded samewriter completionmva4abzw-a-q44x nowimplementsoriginalrequirednative effects under equal guards; no acceptance or synthetic observed proof.
L13 — Parent T4 first security subunit: reuse existing controlledplan Envelope with execution-only omitted fields for image digests/issued+expiry/image-actionwall/four fixed fullcaseplans; originalpreparation JSON/defaultscope stay unchanged. PrepareProof uses owned-proof-v1, normal10s/128MiB/CPU1/PIDs32/output64KiB/scratch8MiB; cases timeout1s, activecancelmarker, output32B are all bound. Existing PlanDigest remains existingrunner.Plan.Digest. Behavioral negativeRED exit1/.396s showedchangedimage/snapshot/archive/binary/marker/layer/effects/expiry/limits/scope/attempt andinvalidclock accepted by initialscaffold; minimumCheckProof rederivesexactcontext/profile/cases/canonicalJSON/expiry andrejectsagentapprovedfield, GREENcontrolledplan+existingrealmainhelper tests exit0/emptystderr. This is PLANvalidation only, not human authentication or execution; audit/cat/runtime composition remains unimplemented. No loader/daemon/settings operations occurred.

L14 — T2 locally committed4c7c7b964f3d1a52ca9e9cf41788808b3d99efd4. T4 private humanDecision tests showed actual behavioral RED→GREEN for exact owner User/environment/comment approve <binding_digest>, source/run/attempt/workflow/status/expiry/cases. No authenticated transport existed at that checkpoint; no approval was inferred. sourceSnapshot tests also RED→GREEN for changed bytes/names/symlink/oversize and unchanged counters.
L15 — T3 native completion delivered guarded scratch/no-send-network/known-sentinel/CPU/capped same-binary memory+pids children; inert native dispatch and group-OOM tests GREEN. Parent Linux compile-only job16 exit0 in2s, binary never executed. Parent then reproduced full-quota scratch incompatibility: observe allocated a legacy scratch file before the final extra-byte write, blocking ENOSPC evidence at8388608bytes. Factored identical read-only guards for between-write checks; RED1→fullownedprobeGREEN0, rootcause45173. Parent public Run tests also reproduced9 named credential keys accepted as OBSERVED/exit0; presence-only correction now yields UNVERIFIABLE/exit1, fixed stderr, no fixture value output and no later child/CPU demand; fullownedprobeGREEN0, rootcause45174. These source changes invalidate the earlier binary as a final catalog candidate; rebuild required. All tests injected/inert, no real credentials or host stress.
L16 — Read-only GitHub API audit source added: fixed HTTPS/GET endpoints, explicit bearer token with no logging, proxy disabled, bounded response/deadline/redirect host, repository+owner+workflow+run+attempt identity, exact sole owner environment reviewer/no-admin-bypass and main plus optional exact feature branch policy. API transport positive and9 negative cases observed actual scaffoldRED1→controlledverifyGREEN0. It reads reviews, never approves. This is source/fake evidence only; no authenticated end-to-end execution route or final image catalog yet. The current real Environment remains main-only and unchanged.
L17 — Runtime context backstop fired at77234tokens. Next substantial dependent T4 composition unit must be delegated with this document/S1/T4 and narrow allowlisted surfaces; parent handles state reconciliation, final source acceptance and T5 independent review. No new product orchestration abstraction is authorized.

L18 — Worker mva5cq6j-b-b3qj admitted after correcting only the allowed-surfaces heading boundary, then returned BLOCKED: read injected skills/spec; git status exit0; no source edits, RED/GREEN, build, Docker or external actions. Its runtime exposes synchronous bash but not required bash_background/completion events. Parent steering message arrived after exit and was not delivered. Required worker capability is unavailable; no retry/relaunch/extra verification performed. T4 blocked pending that capability/explicit resolution; this is not a completed implementation or a ready concrete Docker plan. T3 updated binary/catalog and T5 review remain pending. No runtime authorization, provisioning, policy change, merge or issue closure occurred.

L19 — User authorization (verbatim): "hagamos eso, avanza con eso y no pierdas tiempo". Resolve the tooling incident without CI Trust infrastructure or permission changes: primary session owns required background compile/pack/inspect and eventual long checks; scoped worker owns cohesive composition source/tests/docs and only short deterministic checks. Primary job17 builds corrected frozen Linux/amd64 probe ONLY, packs/inspects trusted image data; native executable is NEVER executed. This does not authorize Docker/load/provisioning/security-policy changes. Resume same bounded source unit after current artifact identity is available, not retry the missing worker tool.

L20 — User resumed with "avancemos con eso" after disabling Gentle Shell loading while preserving Pi/Engram. Existing branch/WIP verified, no reset or worker launch. Initial focused run reproduced controlledverify build failure: catalog tests referenced missing reviewedImage/admitImage/admitPlanImage/inspectCatalog; runner, controlledplan and ownedprobe passed. Implemented a single compiled image identity tuple, exact metadata/plan admission and archive SHA-256 gate before the existing independent content/config inspector. Fresh Go1.26.4 Linux/amd64 static compile, pack and Inspect reproduced ImageID2e926eaf, archive3b313de0, binaryf48e16b9, markera194da40 and layer8bfe3516 in full; artifacts under .git/issue8-real/resume, binary never executed. Pinning this candidate is not independent review or human approval.
L21 — Added private authorizeProof composition of fixed catalog + canonical CheckProof + live source remeasurement + authenticated read-only GitHub audit + exact owner approve <binding_digest> decision. Audit total wall15s, source measured before and after audit, expiry evaluated after audit; no Docker or policy mutation. Hermetic transport tests cover exact consent, zero API calls for invalid local inputs, changed source during audit, expiry during audit, cancellation, wrong approval binding, denied API and wrong revision. Full Windows go test ./... and go vet ./... exit0; git diff --check exit0. This is code/inert transport evidence only, not an actual GitHub human decision, loader, runtime proof or completed #8. Source remains uncommitted beyond existing runner4c7c7b9. Remaining concrete work: loader/post-load image observation, runtime/evidence/cleanup command and opt-in workflow, proportional independent review, then genuine exact-plan human authorization. No new orchestration or harness requirement.

L22 — User accepted the recommended next step with "hagamos eso". Implemented runner.LoadOwnedImage: independent ownedimage.Inspect against exact expected tuple before any Docker call; reused daemon capability checks; fixed image load --quiet with bounded stdin bytes, total20s including metadata/load/post-inspect, combined64KiB and WaitDelay200ms; metadata still2s, attach unchanged. Post-load observation checks exact ID/Linux-amd64/single layer/no tags and known typed empty config defaults, rejects unexpected shell/onbuild/user/env/workingdir/healthcheck etc. Failed or ambiguous loads cannot retry or bypass through same-instance Preflight; a verified loaded image cannot rebind to a different plan. New hermetic inert ELF/client tests plus owned executable >2s load, caller cancellation, combined output overflow and load→actual LocalDocker→Runner→owned cleanup connection all pass. No Docker binary/socket or probe execution used by tests.
L23 — Deliberate scope-reducing safety decision: retain the untagged content-addressed image; grant NO image deletion authority. A pre-load absence check cannot prove exclusive ownership against concurrent same-image loads. Updated proofEffects and thus full plan binding to explicitly require retention; this supersedes the earlier draft newly-loaded-unused-image deletion intent. Containers still require exact ID+owner removal and observed absence. Added private composition plus production ExecuteProof with fixed /usr/bin/docker/local socket/private empty client config; archive/content/catalog before authenticated audit, source/lifetime recheck before load, existing runner remeasurement immediately before each Start, fresh single-use runtime per case. Records normal/timeout/actual split-marker cancellation/combined-output32B and cleanup/source integrity; stops on ambiguity/cleanup failure. RECORDED means harness expectations only, never security PASS. Runtime/tool/resource observations remain to be reviewed separately.
L24 — Added internal controlled-verify prepare/execute command: prepare emits complete exact review envelope with no API/Docker; execute checks raw plan/binding against current workflow revision/run/attempt before read-only human audit, accepts no Approved flag/argv/executable, never logs token. Tests cover invalid args, output failure, foreign workflow context/rerun/changed revision/binding/unknown fields/duplicate flags and unsupported source without external access. Added docs/owned-execution-route.md. Full go test ./..., go vet ./..., Linux/amd64 and Darwin/amd64 cross-builds, whitespace checks PASS. Actual prepare command also emitted a complete four-case draft using the pinned archive and dirty local source; fixture run1 is NOT a real current workflow run or execution consent. .git/issue8-real/resume/review-draft.json is scratch/draft only and source edits invalidate it. Protected existing workflows/policies unchanged; no authenticated external audit, Docker/load/provisioning/security-policy mutation, merge/closure or real isolation proof. At this checkpoint the remaining work was exact source/candidate independent review, commit/delivery organization and protected opt-in workflow, then concrete-plan human authorization and real run; source workflow/review are completed by L25–L28 below.

L25 — User requested "hagamos eso, luego pasamos al PR". Implemented execution_proof boolean input, default false, in the existing manual owned-image workflow. Default source-only prepare/review retained; exact owner/repository/main scope with optional exact feature branch ONLY for opt-in source. Opt-in execute job depends on displayed full plan and protected existing Environment; exact checked-out SHA/clean checkout/Go1.26.4/pinned existing actions, independently reproduced owned archive, no stored checkout credentials/shared cache. Only final compiled verifier step receives read-only actions/contents audit access/token; compiler/packer do not. Full bounded record/fixed diagnostics preserved in run summary even on failure. Preparation summary displays environment, image/cases/limits/effects/expiry/exact approve <binding_digest> comment, retention and residual limits. No direct Docker/provisioning/policy/approval command; source support does NOT change saved main-only policy. Structured YAML source tests cover default-off/mutually exclusive protected jobs, exact scope/actions/compiler/outputs/context/token separation/evidence. All extracted shell bodies pass bash -n and ShellCheck with SC2016 excluded ONLY for intentional literal Markdown fences.
L26 — One independent Codex read-only review was initially unable to launch its shell due local tool policy (ZERO source inspected, BLOCKED; not a verdict). No bypass/policy/sandbox change. Supplied authorized scoped source as numbered plain data instead: 65 files including all relevant production/tests/new untracked source/workflow/docs plus actual parent checks. Independent static review found exactly P1 expected owned-child OOM rejected by global OOMKilled guard and P2 missing final source measurement on failed effect paths. Report .git/issue8-real/review/report-data.md. Static data review is independent reasoning about supplied source, not independent filesystem/test/runtime attestation; parent preserved source during review.
L27 — One bounded correction batch: P1 confirms Docker daemon OOM event flag is separate from main-process exit; retain legacy rejection, but owned-proof-v1 may tolerate expected child event ONLY with final main exit0, successful attachment and complete bounded twelve-check report/all OBSERVED/increasing resource counters. Missing/ambiguous/failed evidence, parent137 and transport errors still rejected. Nullable Result.OOMEvent preserves raw inspected flag through wrapper. Owned regression with correct64KiB profile reproduces original reviewed Docker code rejecting complete surviving-parent evidence (RED1/.461s) in an archived scratch module; current code GREEN. Initial test mistakenly inherited32B output quota and its output-overflow diagnostic is retained as fixture error, NOT OOM RED; no live production mutation or mutation testing. P2 uses named return/deferred independent source measurement on EVERY return after requesting image/container effect, preserving original failure reason. SourceIntegrity NOT_MEASURED/UNCHANGED/CHANGED/UNVERIFIABLE and available final digest distinguish unavailable vs changed. Original source all six failed-effect paths RED1/1.868s; corrected GREEN with extra changed/unavailable-source failed-cleanup cases. Parent checks separate stdout/stderr under .git/issue8-real/review/{p1,p2}-{red,green}.*.
L28 — Independent data-only recheck limited to SAME P1/P2 and direct regressions: BOTH RESOLVED, no blocking direct regression, READY FOR SOURCE PR; .git/issue8-real/review/recheck-report.md/exit0. This is source readiness only, NOT human runtime consent or isolation acceptance. Final full go test -count=1 ./... exit0 (runner8.932s/controlledverify4.246s/CLI.899s), vet, Linux+Darwin amd64 CGO0 cross-builds and whitespace PASS; source-checks.stdout/stderr retained, stderr empty. Existing legacy/vector/env regressions passed; no shared public audit/report CLI parsing changed, no extra public-CLI differential campaign or race result claimed. Rebuilt current owned Linux binary, Pack and Inspect reproduced exact full five-digest tuple in .git/issue8-real/review/image.json, binary NEVER executed. Linked docs updated to distinguish current internal route, legacy mode, OOM/source evidence and source-only acceptance. All source remains uncommitted after existing4c7c7b9; no PR/push/merge/dispatch/approval/Docker/provisioning/policy edit or closure. Read-only GitHub issue check: #8 OPEN with enhancement only; no PR for current head. Before a later PR, resolve the configured issue-label/admission convention and organize cohesive feature-branch-chain work units; preserve tests/docs and disclose >400-line cohesive exceptions rather than minify. #8 real proof remains T6 pending; #9 not started.

L29 — User explicitly approved the requested publication labels and cohesive >400-line safety-unit size exceptions, and asked where the policy came from. Explained that status:approved/type:* come from global ~/.agents/skills/branch-pr/SKILL.md (worded for Gentle AI) and the400-line/size:exception guide from chained-pr/SKILL.md, not GitHub or a pre-existing CI Trust rule. Acknowledged overly rigid transfer; no skill/global configuration edits. Disabling Gentle Shell does not uninstall those separately installed global skills. This approval is source-publication metadata only, NEVER concrete Docker execution or issue closure.
L30 — Created only status:approved, type:feature, type:bug, type:chore, size:exception labels with explicit source-only descriptions; added status:approved to OPEN #8 preserving enhancement. Preserved original branch4c and complete WIP stash plus private raw task backup under .git/issue8-real/publication/private. Applied the cached Feature Branch Chain strategy: draft/no-merge tracker doc63lines c8e8a91, then cherry-picked original runner unit to1a0b911, nativeprobe3987c1b, exactplana56475d, verifiedload17002f2, humangate2b85e02, command/evidencea2fb7cd, protectedworkfloweb12dfb. Exact48 frozen canonical candidate blobs verified after committing; Go/YAML raw bytes unchanged, Markdown Git newline canonicalization explicit. Only additional public path is docs/real-isolation-review.md; private odd task notes excluded from all PRs. No force/reset/rebase, branch deletion or stash drop.
L31 — All EIGHT exact committed archive snapshots (tracker +7children) independently exercised by parent with full go test -count=1 ./... AND go vet ./..., all16commands exit0, separate stdout/stderr/numeric receipt in .git/issue8-real/publication/validation/receipt.json; archived Go VCS stamping disabled only for private non-worktree snapshots. No real Docker/probe run. Source refs pushed atomically/non-force, remote IDs read back. Opened tracker https://github.com/dnlrsls/ci-trust/pull/46 DRAFT and children47→48→49→50→51→52→53, each base the immediate parent. Review budgets179/1519/207/720/922/776/486; approved size:exception on48,50,51,52,53, exactly one type:* each; tracker feature/47bug/53chore. Full chain diagrams/current 📍, exact commit IDs/scopes/rollback/tests/safety limits in descriptions; Refs#8, NO auto-close keywords/merges. Publication state/readbacks .git/issue8-real/publication/{plan.json,publication-state.json,pr-*.json}. Existing automatic portable Go PR CI launched normally; Docker-proof workflow NOT dispatched. Local current head child53eb12; only private task notes remain untracked. Do NOT rerun publication helpers.
L32 — Real-proof prerequisite remains separate: current auth/source workflow allowlist accepts main or original feat/issue8-real-isolation, NOT these new review-chain branch names. Original branch still holds old4c, deliberately preserved. Do not attempt to dispatch the final child or silently widen code/policy; a future concrete authorized execution-ref plan must bind the reviewed final source while respecting no-merge-before-evidence and the saved main-only Environment. No PR merge, issue closure, Docker image/container action, human review API write or security-policy change was performed.
L33 — All24 existing portable Go jobs SUCCESS (Ubuntu/Windows/macOS on each of8PRs), observed2026-10-16T00:22:01.755Z, exact head IDs verified. Runs:46→38828876450,47→38828883392,48→38828890888,49→38828901432,50→38828910451,51→38828916809,52→38828925263,53→38828932162. Read-only bounded CI observer completedexit0, no retries/reruns/dispatch. Actual source/native test+vet receipts only, NOT Docker-isolation evidence and no no-skips claim from status alone. Each PR description updated with its exact head and3successful job links. Publication state source-pr-chain-open-all-portable-ci-success, no merge/noRuntime. Main still2c665fd; #46 OPEN/DRAFT; #8 OPEN enhancement+status:approved. Original4c branch and backup stashbe9f88c4e8ea3fbddd439efc90196f50722fe613 retained. Current root eb12dfb, only this private task document untracked. At that checkpoint the next step was human review and separately planned real proof; the user subsequently authorized the FEATURE-ONLY integration loop in L34.
L34 — User: "hagamos eso, y si pasan mergea, si tienen errores correccion y asi en un bucle, avanza", replying to reviewing47–53/integrating children into tracker46 while main stays held. Scope confirmed: feature children ONLY; tracker46 remainsOPEN/DRAFT, main2c/#8OPEN/Docker consent unchanged. Rechecked all7exact source diffs/bases/heads/paths against accepted48blobs, independent static review/P1P2 recheck and original16local+24nativechecks; no source drift/new blocking review requests. Reused completed independent review, no new campaign. Started parent-owned .git/issue8-real/pr-review/integrate.cjs; inspect integration-state.json/stdout/stderr/exitcode before continuing, DO NOT replay. Exact-head synchronous REST merge only literal PR47–53, no async native-stack API/downstack merge, no queue/admin/bypass/automerge/branch deletion. All reviewed roll-up trees must equal acceptedeb12 tree and each changed head needs fresh3hostSUCCESS before next merge. First53merged into52branch as647889a99d1682285a5bd4984a63f5110d5f88ad; waiting fresh52CI. Repo merge commits enabled/automatic branch deletion false. Runtime/provisioning/policy/issue/main operations remain forbidden. Workflow authority checkpoint updated45137; final outcome pending.
