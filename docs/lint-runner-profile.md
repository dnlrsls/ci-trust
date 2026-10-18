# Closed fixed lint runner profile

`runner.FixedLintPlan` describes only CLEAN, FAULT and RESTORED using the compiled
image, module identity, receipt binding and exact resource limits. It accepts no
caller image, argv, resource overrides or target input. Planning data is not consent.

`NewLintOwned` checks the compiled lint gate before runtime or source callbacks.
The current Node hold denies construction. `NewOwned` still accepts only its old
smoke profiles and old plan JSON/digests are unchanged. Local Docker lint preflight
also denies the held profile before client access; no lint image loader is added.

The fixed vector selects `/ci-trust/owned-eslint --role ROLE --binding DIGEST`,
UID/GID 65532, `/work`, no network/mounts/capabilities and bounded read-only/scratch
resources. Container stdin remains empty: the owned launcher feeds the fixed
role fixture to Node internally. Neither target bytes nor nonempty attach input
are accepted. Split capture preserves both streams and verified cleanup.

Only outer launcher exit zero can be completed transport. An inner ESLint exit
one is framed receipt data, not a Docker-client failure or sensitivity verdict.
The existing strict decoder/classifier must still check it in trusted composition.
Tests use private trusted composition fixtures with fake runtimes; they do not
admit an image or run Docker, the launcher, Node or ESLint. Real isolation, library
compatibility, exact human review and clean/fault/restored evidence remain unproved.
