# Independent offline lint candidate inspection

`internal/lintimage.InspectEnvelope` independently checks the three-member
Docker-save-shaped envelope, SHA-256 archive/config/layer identities, strict JSON,
and fixed Linux/amd64 configuration. It does not import preparation inspection.
It accepts no execution authority and never extracts members or starts a process.

Input is capped at 256 MiB; config and manifest at 4 KiB each. Duplicate members,
non-owned metadata, duplicate JSON keys, unsupported entry types, missing terminal
blocks, hidden GNU/PAX extension headers and nonzero trailing bytes are rejected.
JSON nesting is capped at 32.
These are parser bounds, not host memory, time or sandbox enforcement guarantees.

The result is `ARCHIVE_ENVELOPE_ONLY_NOT_ADMITTED`, with
`runtime_admitted: false`. Supplied expected hashes are comparison data, not
approval. The layer filesystem/inventory, ELF compatibility, dependency security,
independent reconstruction and real lint sensitivity are NOT established by this
partial result. Catalog, smoke admission, runner, consent and Node hold are unchanged.
