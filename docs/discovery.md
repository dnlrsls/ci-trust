# Safe static discovery

`internal/discovery` inventories static Go/npm candidates without executing target
code. A candidate is not proof of installation, a CI invocation, successful
validation, efficacy or enforcement. The audit CLI remains owned by issue #6.

## Library boundary

| Function | Responsibility |
| --- | --- |
| `ReadSnapshot(path, options)` | Read a confined, bounded set of recognized files and report omissions |
| `Analyze(snapshot, options)` | Resolve supported static manifests and literal script references |
| `Scan(path, options)` | Compose those two boundaries without starting a subprocess |

Results are typed intermediate inventory, not an alternative versioned JSON
report. Future report consumers must use the [evidence contract](evidence-contract.md)
and its structural/semantic validation. Results carry root-relative source
anchors, stable IDs, explicit uncertainty and owning manifest roots.

## Evidence and ownership

- Each Go/npm manifest directory is independent, including nested and mixed
  roots. A script reference cannot resolve through a neighboring root.
- A Go module can suggest conventional `go build ./...` and `go test ./...`
  candidates. They retain UNKNOWN invocation and CONVENTION origin: a module
  does not prove a configured CI command.
- Supported literal npm script definitions are CONFIGURED with SCRIPT origin,
  never EXECUTED. Commands retain their entry-definition anchor and ordered
  reference chain. Tool presence is not proof that the tool is installed.
- Missing, cyclic, computed, malformed and unsupported references remain UNKNOWN.
  Unresolved script entries retain their definition anchor with an empty command;
  they are not usable resolved commands. Reader omissions remain diagnostic records.
  Exact token positions may be unknown (zero Line/Column in the intermediate DTO);
  this is not a schema-v1 source position and must not be serialized as one.
  Unsupported composite scripts cannot masquerade as fully resolved candidates.
- Unknown stacks, unsupported package managers and unowned markers have explicit
  diagnostics/limitations rather than invented commands.

## Deliberately narrow interpretation

The baseline recognizes `go.mod`, `package.json`, npm lockfiles and selected
validation-tool markers. Literal npm references and a small set of build/test/lint
leaf commands are resolved statically. There is no general shell interpreter.
Environment assignments, substitutions, pipes, redirection and other dynamic or
unsupported syntax do not execute and do not produce guessed commands.

JS configuration markers are data, never imported or evaluated. Lockfiles do
not cause dependency resolution, lifecycle hooks, installation or network access.
Unsupported-stack markers include Python, Cargo and Maven inputs; detection does
not claim an adapter for their tooling. No submodules or remote actions are fetched.

The reader also admits exact root `.github/workflows/*.yml` / `*.yaml` files and
root `.ci-trust.json`, under the same entry/depth/byte budgets and confinement.
Nested workflow directories, workflows under nested repository roots, arbitrary
YAML and nested support declarations are not admitted by these extra rules.
Workflow YAML bypasses generic manifest/tool-marker analysis, even if named
`pnpm-lock.yaml`; declarations likewise create no tool or command candidates.
Separate static workflow and declaration analyzers consume the retained bytes.
Admission is not YAML/JSON validity, CI execution or declared support proof.

## Finite bounds and exclusions

Zero-valued options select these defaults; invalid/excessive limits return
explicit diagnostics rather than unbounded scans.

| Bound | Default | Hard cap |
| --- | --- | --- |
| Directory depth | 16 | 64 |
| Visited entries, including skipped/unrecognized entries | 4096 | 65536 |
| Bytes per recognized file | 1 MiB | 16 MiB |
| Aggregate attempted read bytes, including rejected files | 8 MiB | 64 MiB |
| npm reference edges across one analysis | 256 | 4096 |
| npm reference depth | 16 | 64 |

Reader, analyzer and composed scans share the same option normalization. Direct
`Analyze` calls first bound the combined count of documents, diagnostics and
limitations against MaxEntries, before copying/sorting or inspecting paths. It
bounds accepted content bytes before parsing and caps the complete output at
4096 records. Overflow fails closed with one UNKNOWN diagnostic, not a selected
partial inventory. Source paths and copied diagnostic codes have finite lengths.

Only ASCII space/tab are supported token separators. Unicode whitespace and
other control separators remain UNKNOWN rather than being normalized into a
configured command. Each resolved composite leaf keeps only its own ordered
reference chain; sibling definitions are not evidence for that leaf.

Enumeration uses finite batches. A directory that cannot be enumerated completely
within the remaining entry budget is omitted in full, avoiding an arbitrary
filesystem-order-dependent subset. Read overflow detection may consume one extra
byte. Limits and omissions remain visible through diagnostics.

Descendant directories with a `.git` entry (file, directory or symlink) are
omitted before their entries or manifests are read, with an UNKNOWN
`EXCLUDED_DIRECTORY` diagnostic. Only confined entry metadata is inspected: marker
payloads and gitdir pointers are never read or followed, and Git is never run.
Marker-inspection failures other than absence omit that directory with an UNKNOWN
`REPOSITORY_MARKER_READ` diagnostic. The explicitly selected root remains admitted
even with its own marker; ordinary nested packages without markers remain included,
regardless of their directory names. Entries inside omitted repositories do not
consume enumeration or file-read budgets; their parent entries still count.
This marker boundary does not identify every possible independently owned tree.

Excluded directories: `.git`, `node_modules`, `vendor`, `.venv`, `dist`, `build`
and `.next`. Symlinks and non-regular entries are skipped. Go `os.Root` confines
file opens; unsupported containment hosts fail closed. It does not prohibit
mounted filesystems or provide an atomic snapshot against concurrent changes.

No repository files are written. Diagnostics do not echo raw executable payloads,
absolute host paths or OS error strings. This is not universal secret detection:
producers remain responsible for the [redaction boundary](evidence-semantics.md).

## Verification scope

Tests use owned temporary repositories and the exported library boundaries.
Hostile scripts/configs remain inert; sentinels, counters and input bytes are
checked alongside deterministic complete results and explicit omissions.
Library tests do not establish public CLI, remote CI or cross-host runtime support.
