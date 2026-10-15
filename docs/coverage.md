# Audit configured paths without claiming execution

`audit.Analyze(snapshot, Options{Platforms: ...})` purely composes discovery,
workflow analysis, bounded rules and the existing v1 report contract.
`audit.Scan(path, options)` adds only the confined discovery reader. Both return
canonical bytes through `evidence.Redact` then `evidence.Encode`; neither starts
processes, fetches policy, or executes repository code.

## Declare the comparison scope

Optional root `.ci-trust.json`:

```json
{"version":1,"platforms":["linux","windows","macos"]}
```

The object is strict: duplicate/unknown keys, trailing JSON, wrong versions,
empty/duplicate/unknown platforms are UNKNOWN declarations. Only lower-case
platforms above are accepted. Explicit nonempty `Options.Platforms` override the
file; invalid API options are operational errors before reading files. Duplicate
explicit platforms are normalized. Explicit inputs have adapter-attributed
DIAGNOSTIC/UNKNOWN evidence without counterfeit file locations.

## Bounded rules

| ID | Meaning |
| --- | --- |
| `CIT_PLATFORM_CONFIGURED_GAP` | Declared platform lacks a generic TEST runner path within closed supplied workflow/event scope. |
| `CIT_DECLARATION_UNKNOWN` | Declaration absent or unsupported, not a platform failure. |
| `CIT_COMMAND_UNLINKED` | Available npm definition has no understood workflow link, not globally never executed. |
| `CIT_STATIC_OMISSION` | Known event-context exclusion, not an observed historical skip. |
| `CIT_DYNAMIC_PATH` | Unresolved runner, matrix, condition or unsupported command/action path. |
| `CIT_NON_BLOCKING` | Potentially non-blocking job/step configuration, not remote enforcement. |
| `CIT_REQUIRED_CHECKS_UNKNOWN` | Required-check selection and enforcement are unknown offline. |

Every finding carries evidence, scope and limitations. Gap findings carry both
declaration and workflow/matrix/run evidence and name the compared platform and
events. V1 has no admissible negative CONFIGURATION capability: gaps are findings,
never ABSENT capabilities. All execution, efficacy and enforcement capabilities
remain UNKNOWN. An entirely empty snapshot has no honest CHECK source anchor:
it returns a valid empty bounded report with diagnostics/UNKNOWN limitations,
not synthetic inventory or findings referencing invented files.

## Supported intent and closure

- Exact npm links come from the workflow graph's candidate identity and owning
  working directory; manifest key locations are recovered from JSON tokens.
- Literal single-line `go test` / `go build` accept only relative `./...`-style
  package arguments and `-race`, `-v`, `-short`, `-count=1`, `-trimpath` flags.
  Direct `node --test`, `jest`, `jest --runInBand`, `vitest run` establish TEST
  intent. No shell chains, environment wrappers, expressions or command names
  inferred from labels are accepted. Explicit shells must be `sh` or `bash`;
  working directories must be confined relative literals.
- npm discovery categories remain bounded TEST, LINT or BUILD intent. Generic
  TEST never implies unit/integration/E2E completeness; unsupported security and
  other categories remain unknown. Job/action names and echo establish nothing.
- Literal hosted runner labels beginning `ubuntu-`, `windows-`, `macos-` with
  documented latest/numeric versions are resolved, including exact
  `${{ matrix.os }}` substitutions from expanded literal matrices. Runner arrays,
  arbitrary expressions and unknown labels remain unknown. Cross-compilation
  targets are never substituted for runner OS.
- Reader omissions, relevant unresolved inputs, unsupported actions/wrappers,
  dynamic matrices/runners/conditions, filters or dependency-conditioned paths
  prevent definite absence findings. Known static omissions remain separate
  from unresolved paths. This is intentionally conservative.

## Report and resource choices

IDs are deterministic; snapshot identity hashes length-framed canonical relative
paths and owned bytes, not timestamps, host paths or git state. Direct inputs
reject duplicate/unsafe paths, over 4096 total records, files over 1 MiB or total
bytes over 8 MiB before parsing. Existing adapters retain their own tighter
analysis bounds. Missing roots/invalid options return errors; malformed or
partially understood inputs remain valid report data.

V1 cannot represent unknown positions: such diagnostics have null source;
configuration evidence includes only actual positive token anchors. Invalid
workflow dependency graphs are represented with evidence and diagnostics rather
than serialized as invalid report DAGs. Valid graph needs edges are preserved.
No raw commands, action names, payloads, host paths or caller-authored diagnostic
prose are retained. Known token-shaped secret prefixes are redacted at the
existing boundary; this is not a universal secret detector.

## Verification boundary

`go test -count=1 ./internal/audit` and `go vet ./internal/audit` validate only the
static producer. CLI binding, independent whole-report byte oracles, corrected
workflow dependency integration and portable-host execution belong to the parent.
These tests are not GitHub workflow execution or remote enforcement evidence.
