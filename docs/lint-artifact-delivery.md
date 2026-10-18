# Protected lint artifact delivery contract

The selected origin is the protected GitHub Actions lint preparation artifact,
not a registry or an arbitrary local archive. `lintcatalog.CurrentArtifact()`
returns compiled repository/producer workflow/revision/run/attempt and artifact
ID/name/size/ZIP digest data. It currently describes the existing **held** Node
24.22.0 candidate; it is not a patched candidate, admission or execution consent.

The canonical GitHub review document binds this complete descriptor as
`candidate_artifact`, in addition to the fixed runtime plan. Recomputing a digest
cannot replace compiled provenance. Older documents lacking it are rejected.

The private decoder checks exact ZIP size/digest, exactly seven fixed regular
members, bounded sizes and six canonical SHA256SUMS records. It reads in memory,
never extracts files or executes payloads. Limits: ZIP 96 MiB, image 256 MiB,
inventory/other metadata 16 MiB each, checksum manifest 4 KiB, total 352 MiB.
Its output is archive/inventory **data only**, not a load capability.

Authenticated producer/run/artifact API validation, delivery, an independently
checked exact image, the separate gated lint loader and workflow composition
remain separate unfinished steps. No download, image load or target execution
is enabled here. Compiled hold checks must precede those future effects; request
or report fields, caller catalog copies and this decoder cannot authorize them.
