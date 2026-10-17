# Local lint integration: execution gate closed

The private `internal/lintverify` catalog describes the prepared ESLint 10.12.0 /
Node 24.22.0 candidate. Its Node security hold is **true** and policy admission is
**false**. Describing the image is not admitting it. No runtime is constructed.

The catalog check precedes every review, clock, source remeasurement, preflight
and capture dependency. A held or unadmitted candidate returns an internal
`UNVERIFIABLE` result with no observations. There is no public approval flag,
caller-supplied process callback or input-file path that changes this gate.

Owned plans bind the candidate image/archive/layer, source digest, effective
module and recipe identities, parser revision, resource-limit declarations and
ordered CLEAN / FAULT / RESTORED roles. Each role has fixed owned stdin and
arguments; source mutation cannot change the immutable strings/arrays of a copied
plan. Limits in this descriptor are data, not independently observed enforcement.

The private orchestration now rehearses an exact-plan review binding and expiry,
preflight, ordered roles, bounded outer capture, verified-cleanup assertions and
source digest checks immediately before and after every capture. It decodes the
owned receipt before classifying the complete triplet. Denied/stale decisions,
source drift, missing/crashed/timed-out capture, bad framing and unverified cleanup
cannot produce positive efficacy. Post-capture source measurement also runs on
failure. Private fake dependencies are test-only; no live review channel or
runtime factory is composed in production.

Current units: catalog/plan/gate and simulated triplet orchestration. The public
`verify PATH --check lint` surface and evidence-v1 mapping remain a subsequent
local unit. Existing smoke runner and human review authentication are unchanged
and do not authorize lint. The private decision expiry/cleanup flags are assertions
from a future trusted adapter, not authentication or observed isolation by themselves.

No Node, npm, ESLint, owned launcher, loader or Docker execution is established
or authorized here. Synthetic tests cannot establish real sensitivity, Linux
isolation, image admission, ABI compatibility or independent reconstruction.
Node hold removal, a real lint runtime profile, live authenticated consent and
real CLEAN-pass / FAULT-expected-fail / RESTORED-pass evidence remain separate.
