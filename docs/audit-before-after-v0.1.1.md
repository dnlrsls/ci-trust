# See a configured CI gap disappear — without running CI

Published CI Trust **v0.1.1** detects two missing platform validation paths in a
small, controlled example. Adding those paths to a copied workflow removes the
two findings. **It does not establish that tests ran, passed or are enforced.**

| Observed result | Before | After |
| --- | --- | --- |
| Declared platforms | Linux, Windows, macOS | Same |
| Configured literal `push` test runners | Linux only | Linux, Windows, macOS |
| `CIT_PLATFORM_CONFIGURED_GAP` records | **2**: Windows and macOS | **0** |
| `CIT_REQUIRED_CHECKS_UNKNOWN` records | **1** | **1** |
| Audit exit / stderr | 0 / empty | 0 / empty |

This is a positive control for a configuration omission, not a newly discovered
project defect. Both cases intentionally return exit 0: a valid report may contain
findings or UNKNOWN. There is no aggregate green trust score.

## Quick path

Download an OS/architecture listed on the
[v0.1.1 release page](https://github.com/dnlrsls/ci-trust/releases/tag/v0.1.1),
verify the ZIP against `SHA256SUMS`, and extract it. No Go or Node installation is
needed to run this native binary and inspect these passive inputs.

From a CI Trust checkout, use two fresh copies of the existing independent
[platform-gap fixture](../internal/audittest/testdata/e-incomplete-platform/repo).
The following Windows/PowerShell example changes only an owned copy:

```powershell
$tool = 'C:\path\to\extracted\ci-trust.exe' # Set your downloaded binary path.
$source = (Resolve-Path 'internal/audittest/testdata/e-incomplete-platform/repo').Path
$demo = Join-Path (Get-Location).Path ('.next/platform-gap-' + [guid]::NewGuid().ToString('N'))
[void][IO.Directory]::CreateDirectory($demo)
Copy-Item -LiteralPath $source -Destination "$demo/before" -Recurse -Force
Copy-Item -LiteralPath $source -Destination "$demo/after" -Recurse -Force
foreach ($case in @('before', 'after')) {
    [IO.File]::WriteAllText((Join-Path $demo "$case/counter"), "0`n")
}

$workflow = Join-Path $demo 'after/.github/workflows/ci.yml'
$text = [IO.File]::ReadAllText($workflow)
$text = $text.Replace('os: [ubuntu-latest]', 'os: [ubuntu-latest, windows-latest, macos-latest]')
[IO.File]::WriteAllText($workflow, $text, (New-Object Text.UTF8Encoding($false)))

& $tool --version
if ($LASTEXITCODE -ne 0) { throw 'Version command failed' }
& $tool audit "$demo/before" --format human
if ($LASTEXITCODE -ne 0) { throw 'Before audit failed' }
& $tool audit "$demo/after" --format human
if ($LASTEXITCODE -ne 0) { throw 'After audit failed' }
```

Expected version: `ci-trust "0.1.1"`. On Linux/macOS use the corresponding native
`ci-trust` binary; the CLI arguments and expected configured gaps are the same.
The local demonstration recorded here used Windows/amd64, not local execution on
all three operating systems.

## The one configuration change

Both copies declare:

```json
{"version":1,"platforms":["linux","windows","macos"]}
```

The workflow uses `on: [push]`, `runs-on: ${{ matrix.os }}` and the literal text
`run: go test ./...`. Only the matrix changes:

```yaml
# Before
os: [ubuntu-latest]

# After
os: [ubuntu-latest, windows-latest, macos-latest]
```

**Do not execute that `go test` command.** This fixture contains metadata, not a
runnable application. CI Trust reads the workflow; it does not start its jobs,
load project scripts, install packages or establish validator efficacy.

## Read the result

Before, the report says that declared Windows and macOS lack a configured generic
TEST runner path within the supplied literal `push` workflow scope. After, those
two records are absent. The required-checks UNKNOWN record remains in both:

> Required checks and actual enforcement remain UNKNOWN offline.

Configured generic TEST paths are not full E2E coverage or operating-system
correctness. No historical execution, successful tests, branch protection or
repository-wide absence of other workflows is established by this example.

## Observed verification

The published v0.1.1 Windows executable was checked against its tested/public
binary SHA-256. Version, both audits and both saved JSON/human renders exited 0
with empty stderr. Each saved JSON replay matched its audit byte-for-byte.
The original three fixture files stayed unchanged; each owned case's four input
files stayed unchanged during inspection, including a passive `0\n` counter.
Only the after copy's matrix differed. No fixture tests, CI, installs or Engram
operations were performed.

Raw local receipts are in `.git/offline-platform-gap-demo-v0.1.1/`; they are not
release assets. This is an observed before/after demo of the existing binary,
not a RED/GREEN implementation cycle or a claim of complete product coverage.

## Use it in a conversation

Ask: **“Which platforms do we promise, and which literal validation paths does
our workflow actually configure?”** Use the two missing paths to demonstrate the
answer. Keep runtime success and enforcement as separate questions needing their
own evidence. See the [CLI contract](cli.md) for report and exit semantics.

### Collect feedback from 2–3 developers

After they try the demo, ask:

1. What missing configuration did you understand from the report?
2. What would you change in the workflow?
3. Which part of the report felt noisy or confusing?

Record what they actually understood or would change, not an assumed endorsement
or proof of CI correctness. Choose one improvement from recurring feedback before
adding more rules. Sharing the guide does not collect feedback automatically.
