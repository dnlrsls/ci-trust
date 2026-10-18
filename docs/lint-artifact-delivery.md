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

The source delivery wrapper checks the compiled gate before token, clock or
client construction. Behind that gate it re-audits the exact lint review, then
checks the compiled repository/producer workflow/run/attempt, successful owner
dispatch, owner preparation approval and artifact origin/digest/size/expiry.
Preparation approval establishes provenance only, never lint execution consent.

Retrieval follows exactly one authenticated API 302 to a documented HTTPS Azure
blob account, with no userinfo, fragment or nonstandard port. A fresh request
sends no API Authorization, Cookie or Referer; further redirects are rejected.
Shared deadline: 45 seconds, shortened by review/artifact expiry; expiry is
rechecked after byte verification. Errors never expose the token or signed URL.
Original metadata-client redirect refusal is unchanged. This code is tested
with fake HTTP, not a live authenticated download. See [GitHub's artifact API](https://docs.github.com/en/rest/actions/artifacts)
and [artifact communication domains](https://docs.github.com/en/actions/reference/runners/self-hosted-runners#requirements-for-communication-with-github).

The separate independently inspecting lint loader and workflow composition exist
as closed source. Actual authenticated delivery, loading and compatibility remain
unproved. The current gate still prevents delivery or image loading. Request/report fields, caller catalog copies and decoded data cannot
authorize them; no candidate download or target execution was performed.
