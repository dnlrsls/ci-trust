# Separate lint GitHub review protocol

The owned lint review plan uses domain `ci-trust/lint-review/v1` and disposition
`NON_AUTHORIZING_PLAN`. It binds the full fixed lint plan, source revision and
snapshot, repository, main branch, run/first attempt, workflow path, existing
protected Environment identity, issue time and fifteen-minute expiry.

Only the canonical owned JSON document, optionally followed by one LF, is read.
Unknown/duplicate keys, alternate encodings, changed fields, future or expired
plans and input over 16 KiB are rejected. The digest is an integrity/context
binding, **not authentication, approval or image admission**. Anyone who can
recompute a digest still needs the separate authenticated human review.

This unit implements planning data only. It neither contacts GitHub nor produces
an execution decision. The selected future workflow is
`.github/workflows/lint-sensitivity-verification.yml`; the Environment remains
`ci-trust-controlled-verification` without policy changes. Smoke approval is not
lint consent. The real read-only audit, runner connection and workflow/control
entrypoint are not yet composed. The Node security hold remains active; preparing
or reading this document cannot start a process or approve an Environment.
