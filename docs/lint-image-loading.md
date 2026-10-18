# Closed lint image loading boundary

`LocalDocker.LoadLintImage` is trusted composition, not human authentication.
Call it only after authenticated exact lint review. The compiled gate is checked
before dependencies, source bytes or clock access; the current held profile is
denied even for a nil runtime/context. No caller IDs, report or approval flag
can select an image or grant admission.

The separate lint path freezes bounded archive/inventory copies and always runs
the independent inventory inspector against compiled image/archive/layer IDs.
The inspector's `INVENTORY_CONSISTENT_NOT_ADMITTED` result is consistency only.
The exact private bytes inspected are the bytes sent to the fixed stdin loader.
No filesystem extraction, target configuration or arbitrary load argv is used.

After daemon checks, one 20-second bounded load is followed by immutable image,
single-layer, Linux/amd64 and strict fixed nonroot/working-directory inspection.
Load stdout is never success evidence. Unknown or active defaults are rejected;
smoke's inspector, loader and empty-config observer remain unchanged.

An attempted load cannot be retried on that runtime, including ambiguous errors.
Images are retained without removal authority: concurrent loads make ownership
of a prior absence unprovable. Tests use inert bytes and fake Docker clients;
no image load or tool execution occurred. Compiled admission, real compatibility,
workflow composition and the actual sensitivity triad remain unproved.
