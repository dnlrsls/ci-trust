// Package lintimage inspects candidate bytes without granting runtime authority.
package lintimage

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

const MaxArchiveBytes int64 = 256 << 20

var ErrInvalid = errors.New("lint candidate inspection failed")

type Identities struct {
	ImageID       string `json:"image_id"`
	LayerDiffID   string `json:"layer_diff_id"`
	ArchiveSHA256 string `json:"archive_sha256"`
}
type Report struct {
	Verdict          string     `json:"verdict"`
	Identities       Identities `json:"identities"`
	RuntimeAdmitted  bool       `json:"runtime_admitted"`
	InventoryEntries int        `json:"inventory_entries,omitempty"`
	FileBytes        int64      `json:"file_bytes,omitempty"`
}

// InspectEnvelope checks the fixed archive envelope, not its filesystem policy.
// Expected identities are comparison data, never trusted approval or admission.
func InspectEnvelope(r io.Reader, want Identities) (Report, error) {
	_, err := envelope(r, want)
	if err != nil {
		return Report{}, ErrInvalid
	}
	return Report{Verdict: "ARCHIVE_ENVELOPE_ONLY_NOT_ADMITTED", Identities: want}, nil
}
func sum(b []byte) string { s := sha256.Sum256(b); return "sha256:" + hex.EncodeToString(s[:]) }
func validDigest(s string) bool {
	if len(s) != 71 || !strings.HasPrefix(s, "sha256:") {
		return false
	}
	_, err := hex.DecodeString(s[7:])
	return err == nil && strings.ToLower(s) == s
}
func bounded(r io.Reader, limit int64) ([]byte, error) {
	if r == nil {
		return nil, ErrInvalid
	}
	b, e := io.ReadAll(io.LimitReader(r, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, ErrInvalid
	}
	return b, nil
}
func envelope(r io.Reader, want Identities) ([]byte, error) {
	if !validDigest(want.ImageID) || !validDigest(want.LayerDiffID) || !validDigest(want.ArchiveSHA256) {
		return nil, ErrInvalid
	}
	data, err := bounded(r, MaxArchiveBytes)
	if err != nil || sum(data) != want.ArchiveSHA256 {
		return nil, ErrInvalid
	}
	name := want.ImageID[7:] + ".json"
	members := map[string][]byte{}
	err = scan(data, true, func(h *tar.Header, b []byte) error {
		if h.Typeflag != tar.TypeReg && h.Typeflag != tar.TypeRegA {
			return ErrInvalid
		}
		if h.Mode != 0444 || h.Linkname != "" || len(h.PAXRecords) != 0 {
			return ErrInvalid
		}
		if h.Name != "layer.tar" && h.Name != "manifest.json" && h.Name != name {
			return ErrInvalid
		}
		if _, ok := members[h.Name]; ok || len(members) >= 3 {
			return ErrInvalid
		}
		if h.Name != "layer.tar" && len(b) > 4096 {
			return ErrInvalid
		}
		members[h.Name] = b
		return nil
	})
	if err != nil || len(members) != 3 || sum(members[name]) != want.ImageID || sum(members["layer.tar"]) != want.LayerDiffID {
		return nil, ErrInvalid
	}
	manifest := fmt.Sprintf(`[{"Config":"%s","RepoTags":[],"Layers":["layer.tar"]}]`, name)
	config := fmt.Sprintf(`{"architecture":"amd64","os":"linux","config":{"User":"65532:65532","WorkingDir":"/work","Env":[],"Entrypoint":[],"Cmd":[]},"rootfs":{"type":"layers","diff_ids":["%s"]},"history":[]}`, want.LayerDiffID)
	if !equalJSON(members["manifest.json"], []byte(manifest)) || !equalJSON(members[name], []byte(config)) {
		return nil, ErrInvalid
	}
	return members["layer.tar"], nil
}

// scan exposes only in-memory slices. EOF requires real terminal blocks and
// zero-only remaining padding, not archive/tar's permissive physical EOF alone.
func scan(data []byte, plain bool, visit func(*tar.Header, []byte) error) error {
	if len(data) < 1024 || len(data)%512 != 0 {
		return ErrInvalid
	}
	source := bytes.NewReader(data)
	tr := tar.NewReader(source)
	padding := 0
	for n := 0; ; n++ {
		before := source.Len()
		h, e := tr.Next()
		if e == io.EOF {
			if before-source.Len() < 1024 || plain && before-source.Len() != padding+1024 {
				return ErrInvalid
			}
			// Check the consumed terminal span too: physical EOF after a
			// hidden GNU helper is not evidence of two actual zero blocks.
			for _, v := range data[len(data)-source.Len()-1024:] {
				if v != 0 {
					return ErrInvalid
				}
			}
			return nil
		}
		// Next may hide local PAX/GNU extension records. The outer envelope
		// permits exactly one physical header per entry, plus prior padding.
		if e == nil && plain && before-source.Len() != padding+512 {
			return ErrInvalid
		}
		if e != nil || n >= 50000 || h.Uid != 0 || h.Gid != 0 || h.Uname != "" || h.Gname != "" || h.ModTime.Unix() != 0 || !h.AccessTime.IsZero() || !h.ChangeTime.IsZero() || h.Devmajor != 0 || h.Devminor != 0 || h.Size < 0 || h.Size > int64(source.Len()) {
			return ErrInvalid
		}
		start := len(data) - source.Len()
		body := data[start : start+int(h.Size)]
		if e = visit(h, body); e != nil {
			return ErrInvalid
		}
		if _, e = io.Copy(io.Discard, tr); e != nil {
			return ErrInvalid
		}
		padding = int((512 - h.Size%512) % 512)
	}
}
func equalJSON(a, b []byte) bool {
	x, e := canonical(a)
	if e != nil {
		return false
	}
	y, e := canonical(b)
	return e == nil && bytes.Equal(x, y)
}
func canonical(b []byte) ([]byte, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if walk(d, 0) != nil {
		return nil, ErrInvalid
	}
	if _, e := d.Token(); e != io.EOF {
		return nil, ErrInvalid
	}
	var v any
	d = json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	if d.Decode(&v) != nil {
		return nil, ErrInvalid
	}
	return json.Marshal(v)
}

// Duplicate keys and excessive nesting are invalid even when encoding/json
// would silently choose a last value or accept an irrelevant nested object.
func walk(d *json.Decoder, depth int) error {
	if depth > 32 {
		return ErrInvalid
	}
	t, e := d.Token()
	if e != nil {
		return ErrInvalid
	}
	open, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	if open != '{' && open != '[' {
		return ErrInvalid
	}
	keys := map[string]bool{}
	for d.More() {
		if open == '{' {
			k, e := d.Token()
			s, ok := k.(string)
			if e != nil || !ok || keys[s] {
				return ErrInvalid
			}
			keys[s] = true
		}
		if walk(d, depth+1) != nil {
			return ErrInvalid
		}
	}
	close, e := d.Token()
	if e != nil || open == '{' && close != json.Delim('}') || open == '[' && close != json.Delim(']') {
		return ErrInvalid
	}
	return nil
}
