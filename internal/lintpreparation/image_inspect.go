package lintpreparation

import (
	"archive/tar"
	"bytes"
	"io"
	"strings"
)

const inspectJSONLimit int64 = 4096

// Each entry needs a header and up to 511 payload padding bytes. A local PAX
// header adds 512 plus 511 padding; its two records add at most 64 bytes of
// lengths/keys/newlines beyond path+target. Metadata itself is capped at
// MaxBytes independently of file payload. Reserve a full 10240-byte end record.
func inspectLayerBudget(data, metadata, files int64) int64 {
	return data + metadata + files*(512+511+512+511+64) + 10240
}

const inspectLayerLimit int64 = MaxBytes + MaxBytes + MaxFiles*(512+511+512+511+64) + 10240

// Outer has three short ASCII names, no PAX, two fixed-small JSON bodies.
const inspectOuterLimit int64 = inspectLayerLimit + 2*inspectJSONLimit + 3*(512+511) + 10240

func inspectEqual(a, b any) bool {
	x, e := Canonical(a)
	if e != nil {
		return false
	}
	y, e := Canonical(b)
	return e == nil && bytes.Equal(x, y)
}

// inspectHeaders checks effective metadata BEFORE Overlay can normalize it.
// archive/tar applies local PAX; only the writer's path/linkpath keys are allowed.
// It exposes global PAX explicitly, which is rejected (S6). No host extraction.
func inspectHeaders(data []byte, outer bool, visit func(*tar.Header, *tar.Reader) error) error {
	limit := inspectLayerLimit
	if outer {
		limit = inspectOuterLimit
	}
	if len(data) < 1024 || len(data)%512 != 0 || int64(len(data)) > limit {
		return tar.ErrHeader
	}
	r := tar.NewReader(bytes.NewReader(data))
	seen := map[string]bool{}
	var payload, metadata int64
	for {
		h, e := r.Next()
		if e == io.EOF {
			return nil
		}
		if e != nil {
			return tar.ErrHeader
		}
		if h.Typeflag == tar.TypeXGlobalHeader {
			return tar.ErrHeader
		}
		name, e := SafePath(h.Name)
		if e != nil {
			return NewValidationFailure("")
		}
		if seen[name] || len(seen) >= MaxFiles {
			return NewValidationFailure("")
		}
		seen[name] = true
		if h.Uid != 0 || h.Gid != 0 || h.Uname != "" || h.Gname != "" || h.ModTime.Unix() != 0 || !h.AccessTime.IsZero() || !h.ChangeTime.IsZero() || h.Devmajor != 0 || h.Devminor != 0 {
			return NewValidationFailure("")
		}
		for k := range h.PAXRecords {
			if k != "path" && k != "linkpath" {
				return NewValidationFailure("")
			}
		}
		metadata += int64(len(h.Name) + len(h.Linkname))
		if metadata > MaxBytes {
			return NewValidationFailure("")
		}
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			if h.Mode != 0444 && (outer || h.Mode != 0555) || h.Linkname != "" {
				return NewValidationFailure("")
			}
			cap := int64(MaxBytes) - payload
			if outer {
				cap = inspectJSONLimit
				if name == "layer.tar" {
					cap = inspectLayerLimit
				}
			}
			if h.Size < 0 || h.Size > cap {
				return NewValidationFailure("")
			}
			payload += h.Size
		case tar.TypeDir:
			if outer || h.Mode != 0555 || h.Size != 0 || h.Linkname != "" {
				return NewValidationFailure("")
			}
		case tar.TypeSymlink:
			if outer || h.Mode != 0777 || h.Size != 0 {
				return NewValidationFailure("")
			}
			if _, e := LinkPath(name, h.Linkname); e != nil {
				return NewValidationFailure("")
			}
		default:
			return NewValidationFailure("")
		}
		if outer && (len(seen) > 3 || (name != "manifest.json" && name != "layer.tar" && !(len(name) == 69 && ValidDigest("sha256:"+strings.TrimSuffix(name, ".json")) && strings.HasSuffix(name, ".json")))) {
			return NewValidationFailure("")
		}
		if e := visit(h, r); e != nil {
			return e
		}
	}
}

// InspectArchive independently checks supplied candidate bytes. It is a
// preparation self-check, not trusted production lint admission or full TAR
// compatibility. Reads are bounded by checked declared sizes, never claims alone.
func InspectArchive(archive []byte, expectedInventory []any, identities ImageIdentities) error {
	fail := NewValidationFailure("")
	outer := map[string][]byte{}
	err := inspectHeaders(archive, true, func(h *tar.Header, r *tar.Reader) error {
		b, e := io.ReadAll(io.LimitReader(r, h.Size+1))
		if e != nil || int64(len(b)) != h.Size {
			return tar.ErrHeader
		}
		outer[h.Name] = b
		return nil
	})
	if err != nil {
		return err
	}
	if len(outer) != 3 {
		return fail
	}
	manifest, e := UniqueJSON(outer["manifest.json"])
	if e != nil {
		return fail
	}
	configName := strings.TrimPrefix(identities.ImageID, "sha256:") + ".json"
	wantManifest := []any{map[string]any{"Config": configName, "RepoTags": []any{}, "Layers": []any{"layer.tar"}}}
	if !inspectEqual(manifest, wantManifest) {
		return fail
	}
	config, ok := outer[configName]
	if !ok {
		return fail
	}
	layer, ok := outer["layer.tar"]
	if !ok {
		return fail
	}
	actual := ImageIdentities{Digest(config), Digest(layer), Digest(archive)}
	if actual != identities {
		return fail
	}
	decoded, e := UniqueJSON(config)
	if e != nil {
		return fail
	}
	want := map[string]any{"architecture": "amd64", "os": "linux", "config": map[string]any{"User": "65532:65532", "WorkingDir": "/work", "Env": []any{}, "Entrypoint": []any{}, "Cmd": []any{}}, "rootfs": map[string]any{"type": "layers", "diff_ids": []any{actual.LayerDiffID}}, "history": []any{}}
	if !inspectEqual(decoded, want) {
		return fail
	}
	if e := inspectHeaders(layer, false, func(*tar.Header, *tar.Reader) error { return nil }); e != nil {
		return e
	}
	tree := Tree{}
	if e := Overlay(tree, layer); e != nil {
		return fail
	}
	records, e := Inventory(tree)
	if e != nil || !inspectEqual(records, expectedInventory) {
		return fail
	}
	return nil
}
