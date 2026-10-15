# Prepare an owned image archive without execution

`owned-image` produces and independently inspects bounded Docker-save-compatible
archives. This is **preparation-only** data, not production catalog admission,
plan approval, a reviewed-binary attestation, or evidence of container isolation.
No public command starts a subprocess, uses a network, or writes a file.

## Interface

Build the command from `./internal/ownedimage/cmd/owned-image` in the separately
controlled preparation environment. Its public interface is:

```text
owned-image pack --binary FILE --marker FILE
owned-image inspect --archive FILE --binary FILE --marker FILE
owned-image --help
owned-image pack --help
owned-image inspect --help
```

`pack` writes an uncompressed tar to stdout. `inspect` writes one JSON object
followed by LF, with fields in this order: `image_id`, `archive_sha256`,
`binary_sha256`, `marker_sha256`, `layer_diff_id`, `scope`. All hashes use
`sha256:` followed by lowercase hex; scope is `preparation-only`.
Shell redirection, artifact storage, and archive loading are caller operations,
not command effects. Validation finishes before stdout receives archive/JSON bytes.

| Result | Exit | Stderr |
|---|---:|---|
| Success/help | 0 | Empty |
| Invalid arguments | 2 | Fixed usage line |
| Invalid input/archive | 1 | `owned-image: invalid input` plus LF |
| Output failure | 1 | `owned-image: output failed` plus LF, if stderr works |

A failed output stream may already have received a prefix. Help output failure
also exits 1. Paths and underlying system errors are never echoed.

## Exact image contract

The outer USTAR archive contains only `manifest.json`, `<config SHA256>.json`,
and `layer.tar`, with root ownership and mode 0644. There are no repository tags.
The config has architecture `amd64`, OS `linux`, explicit `config: {}`, and
`rootfs: {type: "layers", diff_ids: ["sha256:..."]}`. There are no Env, Cmd,
Entrypoint, Healthcheck, Volumes, user, working-directory, or other execution
defaults. No builder or config rewriting participates.

The layer has exactly these entries, in this producer order:

| Path | Kind | Mode | UID/GID | Content |
|---|---|---|---|---|
| `ci-trust/` | Directory | 0755 | 0/0 | Empty |
| `ci-trust/owned-smoke` | Regular | 0555 | 0/0 | Expected binary bytes |
| `ci-trust/root-marker` | Regular | 0600 | 65534/65534 | Owned marker bytes |
| `tmp/` | Directory | 01777 | 0/0 | Empty |

No links, whiteouts, extra members, traversal names, duplicate names, extended
headers, nonzero timestamps, or archive trailer additions are accepted. Tar
padding is zero and the trailer is exactly two zero blocks. The inspector
decodes tar and JSON, rejects duplicate/unknown JSON fields and case aliases
against exact case-sensitive schema keys, compares raw file
bytes and policy fields, and checks hashes; it does not regenerate the producer
archive as its validation oracle. Semantically equivalent JSON whitespace and
member order are not execution authority and need not match producer bytes.

## Input and resource boundary

Binary input is at most 32 MiB; archives are at most 40 MiB; marker input is at
most 1024 bytes and must equal the literal in `tools/owned-smoke/root-marker`
(including its LF). Inputs must be regular non-symlink files. Reads are bounded,
and file identity is checked after opening. Callers must provide stable trusted
input paths: this is not a general defense against an adversary concurrently
mutating input files or ancestor directories.

Binary validation accepts little-endian ELF64 executable AMD64, System V or
Linux ABI, with loadable segments and no interpreter or dynamic segment. ELF
program/section tables are bounded before parsing (128 program headers, 4096
sections; extended section numbering is unsupported). This structural check does
**not** prove the bytes are reviewed, functional, safe, or even runnable. Expected
binary bytes must come from the separately controlled owned build, never target
repository code. Tests use inert invented ELF bytes and never execute them.

## Review boundary and next step

Risk item 2 is the image/plan trust boundary: correct archive bytes and a bound
image ID do not establish runtime safety. No production catalog entry or consent
is created. Existing runner preflight remains unchanged.

A separately authorized Linux preparation run must still demonstrate Docker
archive loading and inspect the daemon-observed image ID, rootfs diff IDs,
filesystem policy, and absent execution defaults before any future admission.
Runtime isolation, resource limits, cleanup, and protected-review enforcement
remain unproved here. No local Docker command or real probe execution is needed
or authorized by these source-only tests.

References: [OCI config/ImageID and DiffID](https://github.com/opencontainers/image-spec/blob/main/config.md),
[Docker image load](https://docs.docker.com/reference/cli/docker/image/load/).
