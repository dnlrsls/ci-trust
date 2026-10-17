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

Current unit: private catalog/plan/gate tests only. The triplet orchestration,
public `verify PATH --check lint` surface and evidence-v1 mapping are subsequent
local units, not shipped by this first unit. Existing smoke runner and human
review authentication are unchanged and do not authorize lint.

No Node, npm, ESLint, owned launcher, loader or Docker execution is established
or authorized here. Synthetic tests cannot establish real sensitivity, Linux
isolation, image admission, ABI compatibility or independent reconstruction.
Node hold removal, a real lint runtime profile, live authenticated consent and
real CLEAN-pass / FAULT-expected-fail / RESTORED-pass evidence remain separate.
