package lintpreparation

import (
	"archive/tar"
	"bytes"
	"io"
	"path"
	"strings"
	"unicode/utf8"
)

// Node is an image-root entry. Kind is dir, file or link; Target is the original
// image symlink text (never a host path). Data belongs only to files.
type Node struct {
	Kind   string
	Mode   int64
	Data   []byte
	Target string
}

// Tree holds validated image entries, not host filesystem objects.
type Tree map[string]Node

func SafePath(name string) (string, error) {
	name = strings.TrimRight(strings.TrimPrefix(name, "./"), "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return "", NewValidationFailure("path-unsafe")
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." {
			return "", NewValidationFailure("path-unsafe")
		}
	}
	if !utf8.ValidString(name) || utf8.RuneCountInString(name) > 4096 {
		return "", NewValidationFailure("path-invalid")
	}
	for _, r := range name {
		if r < 32 || r == 127 {
			return "", NewValidationFailure("path-invalid")
		}
	}
	return name, nil
}

func LinkPath(name, target string) (string, error) {
	if target == "" || strings.Contains(target, "\\") {
		return "", NewValidationFailure("link-invalid")
	}
	for _, r := range target {
		if r < 32 {
			return "", NewValidationFailure("link-invalid")
		}
	}
	joined := strings.TrimLeft(target, "/")
	if !strings.HasPrefix(target, "/") {
		joined = path.Dir(name) + "/" + target
	}
	return SafePath(path.Clean(joined))
}

func Resolve(tree Tree, name string) (Node, error) {
	name, err := SafePath(name)
	if err != nil {
		return Node{}, err
	}
	for range 64 {
		parts := strings.Split(name, "/")
		followed := false
		for i := 1; i <= len(parts); i++ {
			prefix := strings.Join(parts[:i], "/")
			n, ok := tree[prefix]
			if !ok || n.Kind != "link" {
				continue
			}
			target, e := LinkPath(prefix, n.Target)
			if e != nil {
				return Node{}, e
			}
			if i < len(parts) {
				target += "/" + strings.Join(parts[i:], "/")
			}
			name, err = SafePath(target)
			if err != nil {
				return Node{}, err
			}
			followed = true
			break
		}
		if !followed {
			if n, ok := tree[name]; ok {
				return n, nil
			}
			return Node{}, NewValidationFailure("rootfs-missing")
		}
	}
	return Node{}, NewValidationFailure("link-cycle")
}

// Overlay imports a raw tar without host extraction. Rejected entries are not
// installed; previously accepted entries in this layer are not rolled back.
// Global PAX headers are deliberately unsupported: archive/tar does not apply
// their inherited metadata, so accepting them could discard capabilities.
func Overlay(tree Tree, data []byte) error {
	if len(data) < 512 || len(data)%512 != 0 {
		return tar.ErrHeader
	}
	reader := tar.NewReader(bytes.NewReader(data))
	seen := map[string]bool{}
	var total int64
	for {
		h, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return tar.ErrHeader
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			return tar.ErrHeader
		}
		isDir := h.Typeflag == tar.TypeDir
		privileged := h.Mode&04000 != 0 || (!isDir && h.Mode&02000 != 0) || h.PAXRecords["SCHILY.xattr.security.capability"] != ""
		if (h.Name == "." || h.Name == "./") && isDir {
			if privileged {
				return NewValidationFailure("layer-privileged")
			}
			continue
		}
		name, err := SafePath(h.Name)
		if err != nil {
			return err
		}
		if seen[name] || len(seen) >= MaxFiles {
			return NewValidationFailure("layer-duplicate-or-limit")
		}
		seen[name] = true
		if privileged {
			return NewValidationFailure("layer-privileged")
		}
		if strings.HasPrefix(path.Base(name), ".wh.") {
			return NewValidationFailure("layer-whiteout")
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if n, ok := tree[parent]; ok && n.Kind != "dir" {
				return NewValidationFailure("layer-ancestor")
			}
		}
		var node Node
		switch h.Typeflag {
		case tar.TypeDir:
			node = Node{Kind: "dir", Mode: 0555}
		case tar.TypeReg, tar.TypeRegA, tar.TypeCont:
			if h.Size < 0 || h.Size > MaxBytes-total {
				return NewValidationFailure("layer-expanded-limit")
			}
			total += h.Size
			body, e := io.ReadAll(reader)
			if e != nil || int64(len(body)) != h.Size {
				return tar.ErrHeader
			}
			mode := int64(0444)
			if h.Mode&0111 != 0 {
				mode = 0555
			}
			node = Node{Kind: "file", Mode: mode, Data: body}
		case tar.TypeSymlink:
			if _, err := LinkPath(name, h.Linkname); err != nil {
				return err
			}
			node = Node{Kind: "link", Mode: 0777, Target: h.Linkname}
		case tar.TypeLink:
			target, e := SafePath(h.Linkname)
			if e != nil {
				return e
			}
			node, err = Resolve(tree, target)
			if err != nil {
				return err
			}
			if node.Kind != "file" {
				return NewValidationFailure("layer-hardlink")
			}
		default:
			return NewValidationFailure("layer-special-entry")
		}
		old, exists := tree[name]
		if exists && old.Kind != node.Kind {
			return NewValidationFailure("layer-type-replacement")
		}
		count := len(tree)
		if !exists {
			count++
		}
		var size int64
		for key, n := range tree {
			if key != name && n.Kind == "file" {
				size += int64(len(n.Data))
			}
		}
		if node.Kind == "file" {
			size += int64(len(node.Data))
		}
		if count > MaxFiles || size > MaxBytes {
			return NewValidationFailure("rootfs-limit")
		}
		tree[name] = node
	}
}
