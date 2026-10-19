# Static Node test declarations

Discovery recognizes these npm script definitions as `CONFIGURED`, origin
`SCRIPT`, kind `test`:

```text
node --test test/*.test.mjs
node --test --test-concurrency=1 test/*.test.mjs
```

This is declaration evidence only. Discovery does not expand globs, inspect test
files, execute Node, fetch a runtime, infer success, or verify Node-version support.
The npm script name does not determine classification.

## Supported subset

- Existing `node --test` and safe literal relative test paths remain supported,
  including `.`, `./`, `test/`, `test//child.test.mjs`,
  `test/./child.test.mjs`, and `./-test.mjs`. These legacy spellings are retained
  exactly in command/provenance; no directory or file is resolved.
- Relative paths may have `*` within the final filename component only. Multiple
  nonadjacent stars are permitted there; recursive `**` is not.
- For wildcard paths, an optional leading `./` is preserved in the normalized
  command. The remainder must satisfy the existing canonical safe-path boundary.
  Literal paths allow redundant separators and dot components, but every `..`
  traversal component is rejected before validation of the cleaned relative path.
- One `--test-concurrency=N` may precede paths. `N` is 1–2147483647, encoded as
  at most ten ASCII decimal digits. This parsing ceiling is not a runtime claim.
- ASCII spaces/tabs normalize to single spaces. Literal path and numeric spelling
  are retained; npm references retain their ordered definition/reference chain,
  owning manifest, and existing stable-ID rules.

## Still unknown

Unknown flags, repeated concurrency flags, environment prefixes, absolute paths,
traversal, directory wildcards, quoting, substitutions, shell metacharacters,
Unicode separators, and control characters remain `UNKNOWN`. Unsupported terms
reject the entire script, not merely the unsupported suffix.

Arbitrary `node esbuild.config.mjs` build/dev scripts and `npx lockfile-lint`
preinstall scripts remain unknown regardless of their npm script names. Globs
for other executables are not newly admitted. Go conventions, global token
validation, reference budgets, and collection guards are unchanged.

## Review and verification

Review `node_test_literals.go`, then its two narrow integration points in
`manifests.go`. Public `Analyze` table tests compare the complete inventory,
including diagnostics, ownership, normalized commands, reference chains, stable
IDs, input immutability, and repeatability. No target code is executed.
