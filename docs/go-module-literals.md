# Static Go module metadata

Discovery accepts literal `require` and `tool` declarations without loading modules,
resolving dependencies, installing tools or executing project code. Existing
`module`, numeric `go` and numeric `toolchain` declarations retain their behavior.

## Supported forms

```go
module example.com/app
go 1.24
toolchain go1.24.1
require example.com/lib v1.2.3
require (
    example.com/other v0.0.0-20250101123456-abcdef123456 // indirect
    example.com/legacy v2.0.0+incompatible
)
tool github.com/a-h/templ/cmd/templ
tool (
    example.com/tool/cmd/run
)
```

Blocks are line-oriented, non-nested and must close on their own line. Empty
blocks, blank lines and ordinary `//` comments are accepted. Require entries have
one literal relative path and one numeric `vMAJOR.MINOR.PATCH` version, optionally
with a literal prerelease/pseudo-version suffix and `+incompatible`. Tool entries
have one literal relative path. Metadata is discarded after recognition.

## Observable result

An accepted manifest produces the same owning Go root, Go tool inventory and
conventional `go build ./...` / `go test ./...` candidates as a basic manifest.
Candidates remain **UNKNOWN / CONVENTION**, not configured validation. Tool paths
never become commands. Stable IDs, source references and chains are unchanged.

## Deliberately unknown

`replace`, `exclude`, `retract`, inline/nested/unclosed blocks, quoted or dynamic
values, unsupported version syntax, absolute/traversing paths and shell payloads
remain unsupported. A rejected manifest retains its root and emits
`malformed_go_module` with UNKNOWN state; it contributes no partial commands or
tool inventory. This is a bounded literal recognizer, not a complete Go parser or
proof that a module path/version exists.

The public `TestStaticGoModule` table compares full Analyze results against basic
accepted/rejected manifests, including ownership, diagnostics, commands, reference
chains, determinism and unchanged input bytes/counters. No project code runs.
