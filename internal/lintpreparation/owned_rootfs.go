package lintpreparation

import (
	"archive/tar"
	"context"
	"path"
	"strings"
)

// seedOwnedRootfs assembles only the owned seed, NOT an installed lint rootfs.
// T7A2 must import installed dependencies before artifact construction. This is
// data assembly, not filesystem/process authority or runtime compatibility proof.
// base must already be a validated normalized Tree. All input bytes (including
// retained sealed bundle bytes) are borrowed immutable data for the lifetime of
// the result. Neither this function nor its caller may mutate those byte slices;
// the result map is independent. No unnecessary payload cloning is performed.
func seedOwnedRootfs(ctx context.Context, base Tree, bundle *NodeBundle, launcher, module []byte) (Tree, error) {
	return seedOwnedRootfsWith(ctx, base, bundle, launcher, module, MaxFiles, MaxBytes)
}

// Private tiny-limit seam: the byte cap applies separately to payload and path/
// target metadata, matching the normalized Tree packing precondition.
func seedOwnedRootfsWith(ctx context.Context, base Tree, bundle *NodeBundle, launcher, module []byte, files int, byteLimit int64) (Tree, error) {
	invalid := func() error { return NewValidationFailure("") }
	limit := func() error { return NewValidationFailure("rootfs-limit") }
	if bundle == nil || !bundle.sealed || len(bundle.entries) == 0 || len(bundle.entries) > MaxFiles {
		return nil, invalid()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var node, license []byte
	foundNode, foundLicense := false, false
	for _, e := range bundle.entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch e.name {
		case nodeBundlePrefix + "/bin/node":
			if e.kind != tar.TypeReg || foundNode {
				return nil, invalid()
			}
			node, foundNode = e.data, true
		case nodeBundlePrefix + "/LICENSE":
			if e.kind != tar.TypeReg || foundLicense {
				return nil, invalid()
			}
			license, foundLicense = e.data, true
		}
	}
	if !foundNode || !foundLicense {
		return nil, invalid()
	}
	if files < 0 || byteLimit < 0 || len(base) > files {
		return nil, limit()
	}
	additions := []struct {
		name string
		data []byte
		mode int64
	}{
		{"owned/node/bin/node", node, 0555},
		{"owned/node/LICENSE", license, 0444},
		{"ci-trust/owned-eslint", launcher, 0555},
		{"owned/eslint.config.mjs", module, 0444},
	}
	var count int
	var payload, metadata int64
	// Check bounds and bans before allocating the independent map. work is
	// deliberately replaced, even if it was a file/link, as in the source.
	for name, n := range base {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		switch path.Base(name) {
		case "sh", "bash", "dash", "npm", "npx", "gcc", "cc", "python", "python3":
			return nil, invalid()
		}
		for _, addition := range additions {
			if strings.HasPrefix(name, addition.name+"/") {
				return nil, invalid()
			}
		}
		if name == "work" {
			continue
		}
		count++
		if n.Kind == "file" {
			payload += int64(len(n.Data))
		}
		metadata += int64(len(name)) + int64(len(n.Target))
		if payload > byteLimit || metadata > byteLimit {
			return nil, limit()
		}
	}
	out := make(Tree, len(base))
	for name, n := range base {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if name != "work" {
			out[name] = n
		}
	}
	insert := func(name string, n Node) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if count >= files || int64(len(n.Data)) > byteLimit-payload || int64(len(name)) > byteLimit-metadata {
			return limit()
		}
		count++
		payload += int64(len(n.Data))
		metadata += int64(len(name))
		out[name] = n
		return nil
	}
	addFile := func(name string, data []byte, mode int64) error {
		// All names here are fixed safe literals, never host or caller paths.
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if old, ok := out[parent]; ok {
				if old.Kind != "dir" {
					return invalid()
				}
			} else if err := insert(parent, Node{Kind: "dir", Mode: 0555}); err != nil {
				return err
			}
		}
		if _, exists := out[name]; exists {
			return invalid()
		}
		return insert(name, Node{Kind: "file", Mode: mode, Data: data})
	}
	for _, addition := range additions {
		if err := addFile(addition.name, addition.data, addition.mode); err != nil {
			return nil, err
		}
	}
	if err := insert("work", Node{Kind: "dir", Mode: 0555}); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
