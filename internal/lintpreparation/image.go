package lintpreparation

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ImageIdentities names the source SHA-256 identities, including their prefixes.
// These are preparation metadata, not trusted production admission evidence.
type ImageIdentities struct {
	ImageID       string `json:"image_id"`
	LayerDiffID   string `json:"layer_diff_id"`
	ArchiveSHA256 string `json:"archive_sha256"`
}

// Inventory returns sorted source records directly consumable by Canonical.
// File hashes cover bytes, links retain their original text (without a hash),
// and modes describe the supplied tree, not host permissions. Links must resolve.
func Inventory(tree Tree) ([]any, error) {
	names, err := imageNames(tree)
	if err != nil {
		return nil, err
	}
	records := make([]any, 0, len(names))
	for _, name := range names {
		n := tree[name]
		item := map[string]any{"path": name, "type": n.Kind, "mode": "0o" + strconv.FormatInt(n.Mode, 8), "uid": 0, "gid": 0}
		switch n.Kind {
		case "file":
			item["size"], item["sha256"] = len(n.Data), Digest(n.Data)
		case "link":
			if _, err := Resolve(tree, name); err != nil {
				return nil, err
			}
			item["target"] = n.Target
		}
		records = append(records, item)
	}
	return records, nil
}

// imageNames checks bounds before any archive buffers are allocated. Callers
// supply the normalized Tree produced by Overlay; packing never changes modes.
func imageNames(tree Tree) ([]string, error) {
	if len(tree) > MaxFiles {
		return nil, NewValidationFailure("rootfs-limit")
	}
	names := make([]string, 0, len(tree))
	var size, metadata int64
	for name, n := range tree {
		clean, err := SafePath(name)
		if err != nil {
			return nil, err
		}
		if clean != name {
			return nil, NewValidationFailure("path-unsafe")
		}
		if n.Mode < 0 || n.Mode > 07777 {
			return nil, NewValidationFailure("")
		}
		switch n.Kind {
		case "dir":
		case "file":
			size += int64(len(n.Data))
		case "link":
			if !utf8.ValidString(n.Target) {
				return nil, NewValidationFailure("link-invalid")
			}
			if _, err := LinkPath(name, n.Target); err != nil {
				return nil, err
			}
		default:
			return nil, NewValidationFailure("layer-special-entry")
		}
		metadata += int64(len(name)) + int64(len(n.Target))
		if size > MaxBytes || metadata > MaxBytes {
			return nil, NewValidationFailure("rootfs-limit")
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// PackImage serializes a validated image-root tree into the source three-file
// Docker-save shape. No tags, command, environment defaults or execution occur.
func PackImage(tree Tree) ([]byte, ImageIdentities, error) {
	if _, err := Inventory(tree); err != nil {
		return nil, ImageIdentities{}, err
	}
	layer := imageTar(tree)
	config, err := Canonical(map[string]any{
		"architecture": "amd64", "os": "linux",
		"config": map[string]any{"User": "65532:65532", "WorkingDir": "/work", "Env": []any{}, "Entrypoint": []any{}, "Cmd": []any{}},
		"rootfs": map[string]any{"type": "layers", "diff_ids": []any{Digest(layer)}}, "history": []any{},
	})
	if err != nil {
		return nil, ImageIdentities{}, err
	}
	name := strings.TrimPrefix(Digest(config), "sha256:") + ".json"
	manifest, err := Canonical([]any{map[string]any{"Config": name, "RepoTags": []any{}, "Layers": []any{"layer.tar"}}})
	if err != nil {
		return nil, ImageIdentities{}, err
	}
	archive := imageTar(Tree{
		name:            {Kind: "file", Mode: 0444, Data: config},
		"layer.tar":     {Kind: "file", Mode: 0444, Data: layer},
		"manifest.json": {Kind: "file", Mode: 0444, Data: manifest},
	})
	return archive, ImageIdentities{Digest(config), Digest(layer), Digest(archive)}, nil
}

// imageTar implements the source's bounded PAX-writing subset explicitly:
// Python does not use USTAR prefix splitting; it emits ././@PaxHeader with path
// before linkpath, ASCII replacement in fallback fields, directory slashes,
// zero ownership/time, and 10240-byte record padding. Go archive/tar defaults
// differ in PAX pseudo-names/order and record padding. No executable Python
// differential has established byte parity; tests decode these emitted bytes.
// Only validated root trees and the internally constructed outer tree reach here.
func imageTar(tree Tree) []byte {
	names := make([]string, 0, len(tree))
	for name := range tree {
		names = append(names, name)
	}
	sort.Strings(names)
	var out bytes.Buffer
	for _, name := range names {
		n := tree[name]
		kind, size := byte('0'), int64(0)
		switch n.Kind {
		case "dir":
			kind = '5'
			name += "/"
		case "link":
			kind = '2'
		case "file":
			size = int64(len(n.Data))
		}
		var pax strings.Builder
		for _, field := range [][2]string{{"path", name}, {"linkpath", n.Target}} {
			if len(field[1]) > 100 || !imageASCII(field[1]) {
				body := field[0] + "=" + field[1] + "\n"
				length := len(body) + 2
				for next := len(body) + len(strconv.Itoa(length)) + 1; next != length; next = len(body) + len(strconv.Itoa(length)) + 1 {
					length = next
				}
				fmt.Fprintf(&pax, "%d %s", length, body)
			}
		}
		if pax.Len() > 0 {
			imageHeader(&out, "././@PaxHeader", "", 'x', 0, int64(pax.Len()))
			imageBody(&out, []byte(pax.String()))
		}
		imageHeader(&out, name, n.Target, kind, n.Mode, size)
		if n.Kind == "file" {
			imageBody(&out, n.Data)
		}
	}
	out.Write(make([]byte, 1024))
	out.Write(make([]byte, (10240-out.Len()%10240)%10240))
	return out.Bytes()
}

func imageASCII(s string) bool {
	for _, r := range s {
		if r > 127 {
			return false
		}
	}
	return true
}

func imageBody(out *bytes.Buffer, data []byte) {
	out.Write(data)
	out.Write(make([]byte, (512-len(data)%512)%512))
}

func imageHeader(out *bytes.Buffer, name, target string, kind byte, mode, size int64) {
	var h [512]byte
	text := func(start, width int, s string) {
		ascii := strings.Map(func(r rune) rune {
			if r > 127 {
				return '?'
			}
			return r
		}, s)
		copy(h[start:start+width], ascii)
	}
	number := func(start, width int, n int64) { copy(h[start:start+width], fmt.Sprintf("%0*o\x00", width-1, n)) }
	text(0, 100, name)
	number(100, 8, mode)
	number(108, 8, 0)
	number(116, 8, 0)
	number(124, 12, size)
	number(136, 12, 0)
	copy(h[148:156], "        ")
	h[156] = kind
	text(157, 100, target)
	copy(h[257:265], "ustar\x0000")
	number(329, 8, 0)
	number(337, 8, 0)
	var sum int
	for _, b := range h {
		sum += int(b)
	}
	copy(h[148:156], fmt.Sprintf("%06o\x00 ", sum))
	out.Write(h[:])
}
