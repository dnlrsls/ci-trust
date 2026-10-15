# Proposed CLI contract

**Design only: none of these commands is implemented.** Examples specify future
behavior; they are not evidence of successful runs.

## Commands

Options shown in brackets are optional. PATH is required where shown; there is
no implicit target repository. Formats are human (default) or json.

| Invocation | Input / output | Side effects |
| --- | --- | --- |
| ci-trust audit PATH [--format human\|json] | Bounded local snapshot → audit report | Offline/read-only; no repository processes |
| ci-trust report [--input FILE\|-] [--format human\|json] | Existing schema-valid report → rendering | No scan, execution or hidden report lookup |
| ci-trust verify PATH --check lint [--format human\|json] | Target-bound consented plan → verification report | Owned isolated fixture execution only; P1 |
| ci-trust bootstrap PATH --dry-run [--format human\|json] | Local audit evidence → recommendation plan/diffs | Read-only, no apply; P2 |
| ci-trust --help | Usage, options, effects, limitations and exits | None |
| ci-trust --version | Executable version | None |

Help is also available for each subcommand. Unknown commands/options, missing
required arguments and unsupported formats are argument errors. Version/help
return 0; JSON's one-document rule applies to report/plan-producing commands.

### Audit and report

Audit findings and unknown facts are valid report data, not process failures.
Audit never invokes validators, scripts, hooks, package managers or actions.

Report defaults to stdin, equivalent to --input -. It must work without access
to the source repository. An empty stream, invalid JSON or unsupported schema
version is an explicit operational input failure, not an implicit cwd audit.
When stdin is an interactive terminal and no file is supplied, fail explicitly
rather than silently looking for stored reports. Invalid option syntax is exit 2.

### Controlled verification

The command must show the complete target snapshot, tool/image/config/fixture
identity, resource limits and effects before requesting explicit consent.
Present prompts/plan diagnostics on stderr, not JSON stdout. Require an
interactive terminal for the initial consent route. Without explicit consent
to the current plan, no verification process or container starts; noninteractive
invocation cannot imply consent. Do not add an unsafe --yes or host-shell fallback.

Denied/missing consent or unavailable isolation yields a valid UNVERIFIABLE
report with a reason and no execution. Snapshot/tool/image/limit changes invalidate
consent. Exact plan/consent representation and cancellation behavior must be
specified in #8 before implementing this route; future machine-readable consent
is not part of this foundation contract.

Only the owned clean/fault/restored lint fixtures from #9 can run, under #8's
pre-provisioned approved runtime. No arbitrary repository configs/plugins,
dependency installation, implicit image pulls, credentials, writable host source
mounts or network. A controlled fault's expected failure is evidence, not a CLI
operational failure or a product defect.

### Bootstrap

--dry-run is mandatory. Its absence is exit 2, never permission to apply.
Read the explicit target and emit proposed contents/diffs, findings/evidence,
preconditions, conflicts and limitations. Unsupported stacks or unresolved
commands produce limitations, not invented commands. Never modify workflows,
branch policy or dependencies. Templates are not autonomous LLM generation.

## Output and exit status

| Exit | Meaning | stdout / stderr |
| --- | --- | --- |
| 0 | A valid report/plan was produced, even with findings, FAIL or UNKNOWN; or help/version succeeded | Requested rendering; diagnostics on stderr |
| 1 | Operational failure prevented a valid result, including unreadable/missing report input | No report document; actionable diagnostic on stderr |
| 2 | Invalid invocation | No report document; usage diagnostic on stderr |

Exit 0 never means “trusted CI”, passing validators or mandatory merge protection.
An unavailable capability is valid report data if representable; inability to
produce a valid report is exit 1.

JSON stdout contains exactly one schema-valid document and a final newline, no
progress, prompts or logs. No partial JSON is emitted on failure. Human output
shows evidence scope and limitations, not an aggregate green trust label.
Exact JSON fields belong to #2; do not publish a competing example schema here.

Equivalent input/options produce deterministic report/plan JSON; stable
serialization and redaction are explicit obligations, not default Go guarantees.
Report rendering does not append volatile metadata or reinterpret configured
observations as execution.

## Examples and required observations

All paths are examples. Shell redirection below is a user-controlled output
write, not a write performed by CI Trust to the audited repository.

| Example | Expected public behavior |
| --- | --- |
| ci-trust audit ./example | Human report; exit 0 even if configured coverage gaps exist |
| ci-trust audit "./example repo" --format json > audit.json | One JSON document in user-chosen output file, stderr diagnostics; exit 0 if report valid |
| ci-trust report --input audit.json | Human rendering without rescanning source; exit 0 for valid supported report |
| ci-trust report --input - --format json < audit.json | One validated JSON report from stdin; exit 0 |
| ci-trust report < audit.json | Same stdin/human defaults; no hidden database |
| ci-trust report --input missing.json | Exit 1, empty stdout, explicit stderr error |
| ci-trust report --input invalid.json | Exit 1 for malformed/unsupported report, empty stdout, explicit stderr error |
| ci-trust audit ./example --format xml | Exit 2, empty stdout, invalid-format diagnostic |
| ci-trust verify ./example --check lint | Display bounded plan; execute only after valid consent and isolation preflight; otherwise valid unavailable report |
| ci-trust bootstrap ./example --dry-run --format json | One deterministic proposal document; target bytes unchanged |
| ci-trust bootstrap ./example | Exit 2, empty stdout; no apply behavior |

Actual wording and JSON fixtures must be pinned in their owning implementation
issues before executable tests are written. All public CLI tests must assert
exit status, exact stdout/stderr and that rejected inputs leave source bytes
and sentinel counters unchanged. Unknown remote execution/enforcement stays
UNKNOWN in every relevant example.
