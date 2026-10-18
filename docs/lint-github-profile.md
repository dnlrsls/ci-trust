# Closed lint profile and separate GitHub consent

`internal/lintcatalog` is the single owner of the existing compiled candidate
identities and closed policy. `Current` returns value-owned metadata, not approval.
Changing a returned `State`, decoded report or plan cannot change `BlockedReason`.
The current Node security hold takes precedence over missing image admission.

`lintverify` composes its existing private catalog from that owner. This extraction
does not change held report output, plan serialization, runner allowlists, smoke
consent, Environment settings or runtime access. No lint execution is admitted.

The selected consent channel has a separate manual lint workflow source using
the existing protected Environment and exact lint-specific human review. The
fixed runner, independent loader and review/delivery composition are implemented
behind the compiled gate; none is enabled by metadata or channel selection.
Smoke approval cannot be reused. See [the workflow control](lint-workflow-control.md).
No workflow was dispatched and Node/ESLint were not executed.
