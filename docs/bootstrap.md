# Review conditional CI drafts without changing files

`bootstrap.Scan(path, options)` returns a dry-run plan; `bootstrap.Analyze(snapshot, options)` is its pure producer. Neither applies changes, runs commands, installs dependencies, fetches actions, or changes branch policy. The public CLI binds the same producer; there is no separate generator policy.

```sh
ci-trust bootstrap PATH --dry-run --format human
ci-trust bootstrap PATH --dry-run --format json
```

An explicit path and `--dry-run` are mandatory. `--platform linux|windows|macos`
may repeat distinct values to override the declaration. There is no apply flag.
Valid conditional/conflicting/limited plans exit 0; operational errors exit 1;
invalid invocation exits 2. A plan JSON is not a report: render its `source_report`
through `report` separately if needed. See the [CLI contract](cli.md).

## Read a plan

1. Inspect `source_report`: the existing validated report-v1, produced by `audit.Analyze`.
2. Follow each recommendation's `finding_refs` and `evidence_refs` into that report.
3. Review the proposed content and whole-file diff, especially preconditions and conflicts. Apply nothing automatically.

The protocol is `ci-trust-bootstrap-plan/v1`. Arrays are non-null and deterministic. The destination is always `.github/workflows/ci-trust.yml`; existing files remain untouched. An occupied, non-equivalent destination produces a manual-review conflict, not permission to replace or merge it.

## Supported evidence

| Input | Result |
| --- | --- |
| Valid root `.ci-trust.json` declaration or explicit `Options.Platforms` | Same platform semantics as audit; explicit options override the declaration. |
| Direct configured npm script with an audit unlinked finding and decisive definition proof | Conditional draft invoking that actual script in its literal discovered manifest root. |
| Indirect npm wrapper whose selected finding omits resolved leaf/invocation-chain proof | Explicit limitation; do not substitute a wrapper-only reference for decisive validator definitions. |
| Literal npm root or script name containing `${{` | Explicit limitation; YAML/JSON quoting does not disable Actions expressions. |
| Audit TEST-platform gaps and supported configured TEST invocations (`go test ./...`, `go test .`, or linked npm TEST) | Draft for only the gap platforms; a linked lint command cannot remediate a TEST gap. |
| Explicit/inherited shell or working-directory context without decisive gap-evidence anchors | Explicit execution-context limitation, not a changed default shell or fabricated provenance. |
| Only `go.mod` conventional suggestions | No command draft; conventions are not configured validation. |
| Unsupported/unresolved inventory or missing platform policy | Limitations, not guessed commands or support. |
| Partial/unsafe collection | No proposal: destination absence and source scope cannot be established. |
| Intentionally excluded descendant repository | Root proposals remain conditional; no proposal if the excluded directory contains the destination. Other omissions still block. |
| Equivalent configured linkage/coverage | No duplicate recommendation. |

The producer selects existing audit findings; it does not establish a second coverage policy. Its templates are intentionally narrower than the analyzer's recognized language. Unknown configurations can therefore yield no recommendation. Even a resolved
command is not proposed when its actual execution context or decisive definition
chain cannot be represented faithfully by existing report evidence. Conditional
runtime/dependency preparation does not relax these command/provenance boundaries. Separate isolated second-pass fixtures check idempotence without modifying the original source.

## Conditional means not ready to run

The selected policy allows a **conditional draft**, not invented runtime preparation. Reviewers must supply and verify runtime availability, compatible versions, dependencies and tools on every proposed runner. There is no inferred setup action, package installation, copied Go version or claim that the draft will pass. Invoking an npm script can run repository-controlled lifecycle behavior; manually review all script effects before use.

`pull_request`, `push` and `workflow_dispatch` are proposed trigger policy requiring review, not discovered repository intent. Runner labels track hosted images, not pinned runtime versions. Jobs have a ten-minute timeout, `contents: read`, and checkout credential persistence disabled. These choices do not establish execution, efficacy, category completeness, required checks or remote enforcement.

## Verify action pins before updating

Template v1 uses the previously verified official `actions/checkout` commit `d23441a48e516b6c34aea4fa41551a30e30af803`. The producer performs no network lookup. Maintainers updating this pin must independently verify that the full commit belongs to the official `actions/checkout` repository, inspect its source and release association, record verification provenance in review, and review permission/input compatibility. Update the template and independently reviewed golden content/diff together. Do not substitute an unverified hash or mutable tag.

## Local proof boundary

Owned tests assert exported-API behavior, source-report reference resolution, literal generated-YAML runner/command linkage, deterministic content/diffs, conflict handling, isolated two-pass behavior and unchanged file lists/bytes/counters. They never execute fixture scripts or actions. Observed local public-binary RED/GREEN also covers exact stdout/stderr/exits,
full human reference rendering, JSON/report references, deterministic output,
source immutability, isolated second-pass and generated-YAML reanalysis. The
[earlier native audit receipt](portable-ci.md) predates bootstrap and does not
prove this later command on three hosts. New native results must bind their own
source/tree and jobs; local checks never substitute for those results.
