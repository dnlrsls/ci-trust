# Closed lint profile and separate GitHub consent

`internal/lintcatalog` is the single owner of the existing compiled candidate
identities and scoped admission policy. `Current` returns value-owned metadata, not approval.
Changing a returned `State`, decoded report or plan cannot change `BlockedReason`.
The exact profile now has [risk-accepted source admission](lint-admission-policy.md).
If revoked, Node hold still takes precedence over missing image admission.

`lintverify` composes its existing private catalog from that owner. This extraction
originally preserved the held report. The later scoped admission changes only
the cold diagnostic to execution-unavailable, not plan serialization, runner
allowlists, smoke consent or Environment settings. No actual run is approved.

The selected consent channel has a separate manual lint workflow source using
the existing protected Environment and exact lint-specific human review. The
fixed runner, independent loader and review/delivery composition are implemented
behind the compiled gate; none is enabled by metadata or channel selection.
Smoke approval cannot be reused. See [the workflow control](lint-workflow-control.md).
No workflow was dispatched and Node/ESLint were not executed.
