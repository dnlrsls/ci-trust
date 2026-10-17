package lintpreparation

import (
	"bytes"
	"context"
	"runtime"
	"sort"
	"strings"
)

// artifactRequest is preparation data, not runtime admission. The future caller
// must authenticate inputs, enforce host/root ownership, and supply the complete
// validated normalized Tree from owned assembly. No arbitrary rootfs is admitted
// here. baseManifest must be the verified pinned manifest (integer JSON subset),
// not caller-authored policy. Source/run strings and npmFlags must come from the
// guarded pipeline; recording them does not authenticate their provenance.
// All inputs remain immutable throughout the call. The result borrows only the
// original manifest/lock slices: callers must keep them immutable for its lifetime.
// Packing retains two archives temporarily; no streaming/peak-memory guarantee.
// Context checkpoints do not interrupt existing pack/inspect/canonical operations.
// No ELF/library closure, tool execution or independent reconstruction is proved.
type artifactRequest struct {
	tree                              Tree
	manifest, lock, launcher          []byte
	ownedInputs, baseManifest         map[string]any
	npmFlags                          []string
	sourceRevision, runID, runAttempt string
}

// artifactPacker is a private data-only failure-fixture seam, never an origin,
// pin, public packer or runtime-policy override.
type artifactPacker func(Tree) ([]byte, ImageIdentities, error)

func buildArtifacts(ctx context.Context, r artifactRequest) (map[string][]byte, error) {
	return buildArtifactsWith(ctx, r, PackImage)
}

func buildArtifactsWith(ctx context.Context, r artifactRequest, pack artifactPacker) (map[string][]byte, error) {
	var image []byte
	var ids ImageIdentities
	var records []any
	err := InStage("image-validation", func() error {
		if e := ctx.Err(); e != nil {
			return e
		}
		var e error
		records, e = Inventory(r.tree)
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		image, ids, e = pack(r.tree)
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if e = InspectArchive(image, records, ids); e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		repeated, repeatedIDs, e := pack(r.tree)
		if e != nil {
			return e
		}
		if e = ctx.Err(); e != nil {
			return e
		}
		if !bytes.Equal(image, repeated) || ids != repeatedIDs {
			return NewValidationFailure("")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	var out map[string][]byte
	// artifact-write is the reference stage name; this only constructs bytes.
	err = InStage("artifact-write", func() error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if e := ValidateOwnedManifest(r.manifest); e != nil {
			return e
		}
		if e := ValidateNPMLock(r.lock); e != nil {
			return e
		}
		module, ok := r.ownedInputs["Module"].(string)
		moduleHash, hashOK := r.ownedInputs["ModuleSHA256"].(string)
		launcher, launcherOK := r.tree["ci-trust/owned-eslint"]
		config, configOK := r.tree["owned/eslint.config.mjs"]
		layers, layersOK := r.baseManifest["layers"].([]any)
		// These consistency checks bind recorded hashes to supplied bytes, not to
		// an external trust root, optional metadata, or independently admitted image.
		if !ok || !hashOK || moduleHash != Digest([]byte(module))[7:] || !launcherOK || launcher.Kind != "file" || !bytes.Equal(launcher.Data, r.launcher) || !configOK || config.Kind != "file" || !bytes.Equal(config.Data, []byte(module)) || !layersOK {
			return NewValidationFailure("")
		}
		flags := make([]any, len(r.npmFlags))
		for i, flag := range r.npmFlags {
			flags[i] = flag
		}
		metadata := map[string]any{
			"image_id": ids.ImageID, "layer_diff_id": ids.LayerDiffID, "archive_sha256": ids.ArchiveSHA256,
			"status": "PREPARATION_ONLY_NOT_ADMITTED", "node_security_hold": true,
			"node_version": NodeVersion, "npm_version": NPMVersion, "eslint_version": ESLintVersion,
			"node_archive_sha256": "sha256:" + NodeSHA256, "base_manifest": BaseManifest,
			"lock_sha256": Digest(r.lock), "launcher_sha256": Digest(r.launcher), "manifest_sha256": Digest(r.manifest),
			"source_revision": r.sourceRevision, "run_id": r.runID, "run_attempt": r.runAttempt,
			"npm_flags": flags, "base_layers": layers, "go_version": runtime.Version(),
			"serialization_check":          "same-process repeat; not independent offline reconstruction",
			"independent_policy_admission": false, "real_tool_execution": false,
			"elf_library_closure": "NOT_VERIFIED; static and runtime compatibility remain separate gates",
			"limits":              "15 minute hosted job; 180 seconds per npm phase",
		}
		payloads := map[string][]byte{"image.tar": image, "package.json": r.manifest, "package-lock.json": r.lock}
		for name, v := range map[string]any{"inventory.json": records, "owned-inputs.json": r.ownedInputs, "preparation.json": metadata} {
			if e := ctx.Err(); e != nil {
				return e
			}
			b, e := Canonical(v)
			if e != nil {
				return e
			}
			payloads[name] = append(b, '\n')
		}
		names := make([]string, 0, len(payloads))
		for name := range payloads {
			names = append(names, name)
		}
		sort.Strings(names)
		var checksums strings.Builder
		for _, name := range names {
			if e := ctx.Err(); e != nil {
				return e
			}
			checksums.WriteString(Digest(payloads[name])[7:] + "  " + name + "\n")
		}
		payloads["SHA256SUMS"] = []byte(checksums.String())
		if e := ctx.Err(); e != nil {
			return e
		}
		out = payloads
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
