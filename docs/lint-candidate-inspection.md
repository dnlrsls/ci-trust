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
partial result. Inspection cannot change catalog admission, smoke or consent;
see the separate [bounded admission disposition](lint-admission-policy.md).

## Filesystem inventory consistency

`InspectInventory` adds complete sorted record/hash comparison and independent
physical USTAR/owned local-PAX checks. It rejects unsafe paths, whiteouts, writable
or special entries, duplicates, absent/non-directory parents, unresolved links,
escapes and cycles. PAX may contain only unique path/linkpath records with owned
zero metadata. Nothing is extracted; file bytes are hashed in memory.

Inventory input is capped at 16 MiB, 50,000 filesystem entries, 16 KiB per path/link,
8 MiB combined path/link metadata, and one million link-resolution steps with
128 rewrites per link. Results are `INVENTORY_CONSISTENT_NOT_ADMITTED`, never
runtime authority. Matching caller-supplied metadata does not independently prove
package provenance, dependency safety, full content policy, ELF/loader compatibility,
reconstruction, consent, isolation or real ESLint sensitivity. Inspection is
comparison data, never authority to lift a hold or approve execution.
