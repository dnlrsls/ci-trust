# Local lint integration: cold reporting

The private `internal/lintverify` catalog describes the prepared ESLint 10.12.0 /
Node 24.22.0 profile. Its [source disposition](lint-admission-policy.md) now records
bounded risk acceptance and admission. Describing the image is not admitting it;
ordinary cold reporting still constructs no runtime.

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
failure. Private fake dependencies are test-only. Live services are composed only
in the separate protected GitHub request route, never ordinary cold `Scan`.

The bounded `lintverify.Scan` service now composes the existing audit snapshot
with lint-specific evidence-v1 reporting. Production returns UNVERIFIABLE, never
fabricated execution or efficacy. A recognized ESLint declaration document may
anchor a lint CHECK; without a genuine anchor, only a diagnostic and limitations
are added, not an imaginary source path or capability. Executable configuration,
embedded project ESLint configuration and declared plugins are not loaded and
produce `UNSUPPORTED_LINT_CONFIGURATION`. Repository efficacy, execution and
CI enforcement are not upgraded by owned fixture observations.

Private tests map simulated complete triplets to TOOL_FIXTURE-only v1 capability
records. Those tests are not observed execution evidence. The public
`verify PATH --check lint` without request mode composes only this cold service and the existing v1
renderer; it does not accept approval flags, input captures or caller-selected
commands. Valid unresolved reports exit 0, not a sensitivity PASS. Invocation
validation precedes producers and input reads. Existing smoke runner and human
review authentication are unchanged and do not authorize lint. The private decision expiry/cleanup flags are assertions from a future
trusted adapter, not authentication or observed isolation by themselves.

No Node, npm, ESLint, owned launcher, loader or Docker execution is established
or authorized here. Synthetic tests cannot establish real sensitivity, Linux
isolation, image admission, ABI compatibility or independent reconstruction.
Source admission is recorded separately; live authenticated consent and real
CLEAN-pass / FAULT-expected-fail / RESTORED-pass evidence remain separate.
