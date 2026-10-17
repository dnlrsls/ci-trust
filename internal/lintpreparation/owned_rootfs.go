package lintpreparation

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// installedLimits is a private reduced-budget test seam.
type installedLimits struct {
	files, visited, treeFiles int
	bytes, treeBytes          int64
}

// buildOwnedRootfs assembles preparation data only. The seed's normalized-base
// and borrowed-immutable-byte preconditions apply. target must be an absolute,
// private owned stable installation directory. Root acquisition is trusted;
// os.Root is not mount/device isolation or protection against a hostile host.
// Missing/non-directory node_modules is rejected (a required input schema), not
// treated as an empty installation. Real pinned npm layout remains unproved.
func buildOwnedRootfs(ctx context.Context, base Tree, bundle *NodeBundle, target string, launcher, module []byte) (Tree, error) {
	return buildOwnedRootfsWith(ctx, base, bundle, target, launcher, module, installedLimits{MaxFiles, MaxFiles, MaxFiles, 128 << 20, MaxBytes})
}

func buildOwnedRootfsWith(ctx context.Context, base Tree, bundle *NodeBundle, target string, launcher, module []byte, limits installedLimits) (Tree, error) {
	out, err := seedOwnedRootfsWith(ctx, base, bundle, launcher, module, limits.treeFiles, limits.treeBytes)
	if err != nil {
		return nil, err
	} // context/seal/seed guards precede filesystem access.
	invalid := func() error { return NewValidationFailure("") }
	limit := func() error { return NewValidationFailure("rootfs-limit") }
	if !filepath.IsAbs(target) || limits.files < 0 || limits.visited < 1 || limits.bytes < 0 {
		return nil, invalid()
	}
	names, err := imageNames(out)
	if err != nil {
		return nil, err
	}
	var payload, metadata int64
	for name, n := range out {
		payload += int64(len(n.Data))
		metadata += int64(len(name) + len(n.Target))
	}
	add := func(name string, n Node) error {
		if len(out) >= limits.treeFiles || int64(len(name)) > limits.treeBytes-metadata {
			return limit()
		}
		metadata += int64(len(name))
		out[name] = n
		return nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	st, err := os.Lstat(target)
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, invalid()
	}
	targetRoot, err := os.OpenRoot(target)
	if err != nil {
		return nil, invalid()
	}
	defer targetRoot.Close()
	opened, err := targetRoot.Stat(".")
	if err != nil || !os.SameFile(st, opened) {
		return nil, invalid()
	}
	st, err = targetRoot.Lstat("node_modules")
	if err != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, invalid()
	}
	root, err := targetRoot.OpenRoot("node_modules")
	if err != nil {
		return nil, invalid()
	}
	defer root.Close()
	opened, err = root.Stat(".")
	if err != nil || !os.SameFile(st, opened) {
		return nil, invalid()
	}
	files, visited := 0, 0
	var total int64
	// Every entry INCLUDING root/empty directories consumes one visit; SafePath
	// bounds depth and path processing. This deliberately narrows compatibility
	// for large empty-directory trees compared with Python os.walk. ReadDir is
	// incremental, never an unbounded listing. No arbitrary TAR/OS parity claim.
	var walk func(string) error
	walk = func(rel string) (err error) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if visited >= limits.visited {
			return limit()
		}
		visited++
		name := "owned/node_modules"
		if rel != "." {
			name += "/" + rel
		}
		clean, e := SafePath(name)
		if e != nil || clean != name {
			return invalid()
		}
		info, e := root.Lstat(filepath.FromSlash(rel))
		if e != nil || info.Mode()&os.ModeSymlink != 0 || (!info.IsDir() && !info.Mode().IsRegular()) {
			return invalid()
		}
		f, e := root.Open(filepath.FromSlash(rel))
		if e != nil {
			return invalid()
		}
		defer func() {
			if e := f.Close(); err == nil && e != nil {
				err = invalid()
			}
		}()
		actual, e := f.Stat()
		if e != nil || !os.SameFile(info, actual) || actual.Mode().Type() != info.Mode().Type() {
			return invalid()
		}
		if info.IsDir() {
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				batch, e := f.ReadDir(32)
				if e != nil && e != io.EOF {
					return invalid()
				}
				for _, entry := range batch {
					child := entry.Name()
					if rel != "." {
						child = rel + "/" + child
					}
					if err := walk(child); err != nil {
						return err
					}
				}
				if e == io.EOF {
					return nil
				}
			}
		}
		if files >= limits.files {
			return limit()
		}
		files++
		if _, exists := out[name]; exists {
			return invalid()
		}
		// Only seed entries can be descendants of an incoming regular file.
		// Sorted prefix lookup avoids scanning the complete Tree per file.
		i := sort.SearchStrings(names, name+"/")
		if i < len(names) && strings.HasPrefix(names[i], name+"/") {
			return invalid()
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if n, ok := out[parent]; ok {
				if n.Kind != "dir" {
					return invalid()
				}
			} else if err := add(parent, Node{Kind: "dir", Mode: 0555}); err != nil {
				return err
			}
		}
		remaining := min(limits.bytes-total, limits.treeBytes-payload)
		if actual.Size() < 0 || actual.Size() > remaining {
			return limit()
		}
		// Reserve the entry/metadata before allocating payload, and detect a
		// changed size using one extra bounded byte. No host mode is restored.
		if err := add(name, Node{Kind: "file", Mode: 0444}); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		data := make([]byte, int(actual.Size())+1)
		n, readErr := io.ReadFull(io.LimitReader(baseContextReader{ctx, f}, remaining+1), data)
		if err := ctx.Err(); err != nil {
			return err
		}
		if (readErr != io.EOF && readErr != io.ErrUnexpectedEOF) || int64(n) != actual.Size() {
			return invalid()
		}
		total += int64(n)
		payload += int64(n)
		out[name] = Node{Kind: "file", Mode: 0444, Data: data[:n]}
		return nil
	}
	if err := walk("."); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Replacing work can affect base links. Check the final packing precondition
	// without rehashing every payload or claiming runtime admission.
	for name, n := range out {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if n.Kind == "link" {
			if _, err := Resolve(out, name); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

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
