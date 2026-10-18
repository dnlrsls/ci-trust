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

The plan codec implements planning data only. It neither contacts GitHub nor
produces an execution decision. The separate manual workflow source is
`.github/workflows/lint-sensitivity-verification.yml`; the Environment remains
`ci-trust-controlled-verification` without policy changes. Smoke approval is not
lint consent. The runner, delivery/loader composition and workflow/control source
are wired behind the closed gate. The Node hold remains active; preparing or
reading this document cannot start a process or approve an Environment.

## Separate read-only GitHub audit

The private audit checks the fixed repository owner, lint workflow path and its
resolved numeric identity, first-attempt/main/in-progress dispatch, unchanged
protected Environment policy and one exact owner review: `approve lint <digest>`.
It rechecks the active run and expiry before returning the private plan decision.
Smoke workflow identity, cases and approval syntax cannot authorize lint.

The production wrapper checks the compiled gate before reading `GITHUB_TOKEN`,
clock or network. The current hold therefore prevents all live requests. The
HTTPS client owns `api.github.com`, GET-only paths, no proxy/redirects, a ten-second
audit deadline and one-MiB responses. Errors never include tokens or response text.
Tests use fake HTTP metadata; no real GitHub approval/audit or execution is claimed.
No client code approves a review, edits Environment policy or dispatches a workflow.

The canonical review now also binds `candidate_artifact`: compiled producer
workflow/revision/run/attempt, repository/artifact identity, name, size and ZIP
digest. This existing held candidate is not admitted by its provenance. See
[the protected artifact contract](lint-artifact-delivery.md) for data-only ZIP
checks and the closed authenticated delivery/load source. Actual interoperability
and runtime admission remain unverified.
