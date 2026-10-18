# Manual owned lint workflow control

The source workflow is `.github/workflows/lint-sensitivity-verification.yml`.
It is manual-only, owner/main/first-attempt scoped and uses the existing
`ci-trust-controlled-verification` Environment without policy changes. The plan
job produces non-authorizing request data; the dependent verification job needs
the owner's exact `approve lint <binding_digest>` review. Environment approval
alone is insufficient. Expired requests need a separately authorized fresh run,
not an automatic retry.

The internal Go command accepts exactly `--help`, `prepare` or `verify`.
Workspace/run/revision come from the fixed workflow context, not override flags.
`verify` reads the bounded canonical request on stdin; it never imports tool
observations, admission state or approval arguments. Its compiled gate precedes
input/environment/service access. Current operations exit 1 with
`controlled-lint: NODE_SECURITY_HOLD`; only help/argument checks remain available.

Exit 0 means prepared data, or owned TOOL_FIXTURE sensitivity evidence for verify,
never project configuration efficacy or historical CI enforcement. Verify emits
its own v1 report then exits 1 for insensitive/unverifiable outcomes. Operational
failure also exits 1; invalid arguments exit 2. This differs deliberately from
the public read-only `verify --check lint`, whose valid unresolved report exits 0.

The workflow builds locked Go sources with existing action/compiler pins; its
future builds may fetch checksum-locked Go modules, not target dependencies.
The private empty Docker config is created only inside the Environment-protected
job after human review. The Go composition then independently authenticates that
review before delivery/load/capture. Only plan/report JSON is retained for seven
days; no runtime image, source, cache or credential is uploaded.

The current gate fails planning, so verification cannot proceed. No workflow was
dispatched or externally validated here; no Node, Docker or image was executed,
no new candidate was fetched, and no Environment approval/settings were changed.
