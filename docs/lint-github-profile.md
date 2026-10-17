# Closed lint profile and separate GitHub consent

`internal/lintcatalog` is the single owner of the existing compiled candidate
identities and closed policy. `Current` returns value-owned metadata, not approval.
Changing a returned `State`, decoded report or plan cannot change `BlockedReason`.
The current Node security hold takes precedence over missing image admission.

`lintverify` composes its existing private catalog from that owner. This extraction
does not change held report output, plan serialization, runner allowlists, smoke
consent, Environment settings or runtime access. No lint execution is admitted.

The selected future consent channel is a separate lint GitHub workflow/context
using the existing protected Environment and an exact lint-specific human review.
This channel selection is not an execution approval and smoke approval cannot
be reused. Runner/profile and live review composition are not implemented by this
catalog extraction; no workflow is dispatched and Node/ESLint are not executed.
