# Public CLI request to the owned GitHub lint route

`ci-trust verify PATH --check lint --workflow-request - --format json` reads the
bounded non-authorizing workflow plan on stdin and uses the existing authenticated
`ExecuteGitHub` composition. It does not import reports or caller approval. Only
the fixed owned GitHub workspace/repository/main/run/attempt context can proceed.
There is no local-host execution fallback or environment-only automatic selection.

Without `--workflow-request`, the command still uses cold `Scan`, never reads
stdin and keeps valid-report exit 0, including UNVERIFIABLE. The option is legal
only on `verify --check lint`; only the literal `-` is accepted, never a file path.

Request mode emits the validated report, then exits 0 only for just-produced
owned TOOL_FIXTURE sensitivity. Insensitive/unverifiable reports exit 1;
operational failures exit 1 without a report; invalid arguments exit 2. The
trusted producer derives the exit disposition from its own result. It is not
admission, consent or proof that project configuration or historical CI worked.

The exact compiled profile has [bounded source admission](lint-admission-policy.md).
An invalid trusted workflow context is rejected before request input; later exact
authenticated human review still precedes delivery/runtime effects. Fixed
image/provenance, preflight, cleanup/source preservation and strict receipt
classification remain unchanged. Revoked admission still denies before services;
no caller data changes it. Exposing this entry originally changed no pins or gates.

The protected workflow prepares the fixed request with `controlled-lint prepare`,
then builds and invokes the public CLI in request mode after human Environment
review. Its exit status is propagated unchanged. The internal `controlled-lint
verify` operation remains compatible, but is not the workflow's verification
entry. No workflow was dispatched and no Node/npm/ESLint/Docker or image was
executed to implement this route. Synthetic Go tests are not the real lint triad.

See [CLI contract](cli.md) and [manual workflow control](lint-workflow-control.md).
