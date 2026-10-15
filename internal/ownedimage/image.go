// Package ownedimage prepares bounded owned-image archives, never execution authority.
package ownedimage

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"
)

const Marker = "ci-trust owned-smoke-v1 root marker\n"
const MaxBinary = 32 << 20
const MaxArchive = 40 << 20

var ErrInvalid = errors.New("invalid owned image input")

type Metadata struct {
	ImageID       string `json:"image_id"`
	ArchiveSHA256 string `json:"archive_sha256"`
	BinarySHA256  string `json:"binary_sha256"`
	MarkerSHA256  string `json:"marker_sha256"`
	LayerDiffID   string `json:"layer_diff_id"`
	Scope         string `json:"scope"`
}

func digest(b []byte) string {
	h := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(h[:])
}

// ReadFile refuses links and bounds reads even if the file grows after stat.
func ReadFile(path string, max int64) ([]byte, error) {
	s, e := os.Lstat(path)
	if e != nil || !s.Mode().IsRegular() || s.Size() > max {
		return nil, ErrInvalid
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, ErrInvalid
	}
	defer f.Close()
	actual, e := f.Stat()
	if e != nil || !actual.Mode().IsRegular() || !os.SameFile(s, actual) {
		return nil, ErrInvalid
	}
	b, e := io.ReadAll(io.LimitReader(f, max+1))
	if e != nil || int64(len(b)) > max {
		return nil, ErrInvalid
	}
	return b, nil
}
func inputs(p, m string) ([]byte, []byte, error) {
	b, e := ReadFile(p, MaxBinary)
	if e != nil {
		return nil, nil, e
	}
	marker, e := ReadFile(m, 1024)
	if e != nil || string(marker) != Marker {
		return nil, nil, ErrInvalid
	}
	// Bound ELF table counts and offsets before debug/elf can allocate tables.
	if len(b) < 64 || !bytes.Equal(b[:7], []byte{'\x7f', 'E', 'L', 'F', 2, 1, 1}) || (b[7] != 0 && b[7] != 3) {
		return nil, nil, ErrInvalid
	}
	order := binary.LittleEndian
	phoff := order.Uint64(b[32:])
	shoff := order.Uint64(b[40:])
	phsize := order.Uint16(b[54:])
	phnum := order.Uint16(b[56:])
	shsize := order.Uint16(b[58:])
	shnum := order.Uint16(b[60:])
	fits := func(off uint64, size, count uint16) bool {
		return off <= uint64(len(b)) && uint64(size)*uint64(count) <= uint64(len(b))-off
	}
	if order.Uint16(b[52:]) != 64 || phnum == 0 || phnum > 128 || phsize != 56 || !fits(phoff, phsize, phnum) || shnum > 4096 || (shnum == 0 && shoff != 0) || (shnum != 0 && (shsize != 64 || !fits(shoff, shsize, shnum))) {
		return nil, nil, ErrInvalid
	}
	f, e := elf.NewFile(bytes.NewReader(b))
	if e != nil {
		return nil, nil, ErrInvalid
	}
	defer f.Close()
	if f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB || f.Machine != elf.EM_X86_64 || f.Type != elf.ET_EXEC || f.Version != elf.EV_CURRENT {
		return nil, nil, ErrInvalid
	}
	load := false
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP || p.Type == elf.PT_DYNAMIC {
			return nil, nil, ErrInvalid
		}
		if p.Off > uint64(len(b)) || p.Filesz > uint64(len(b))-p.Off {
			return nil, nil, ErrInvalid
		}
		if p.Type == elf.PT_LOAD {
			load = true
		}
	}
	if !load {
		return nil, nil, ErrInvalid
	}
	return b, marker, nil
}

type entry struct {
	name string
	mode int64
	uid  int
	kind byte
	data []byte
}

func layerEntries(b, m []byte) []entry {
	return []entry{{"ci-trust/", 0755, 0, tar.TypeDir, nil}, {"ci-trust/owned-smoke", 0555, 0, tar.TypeReg, b}, {"ci-trust/root-marker", 0600, 65534, tar.TypeReg, m}, {"tmp/", 01777, 0, tar.TypeDir, nil}}
}
func archive(es []entry) ([]byte, error) {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, e := range es {
		h := &tar.Header{Name: e.name, Mode: e.mode, Uid: e.uid, Gid: e.uid, Typeflag: e.kind, Size: int64(len(e.data)), ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}
		if err := w.WriteHeader(h); err != nil {
			return nil, err
		}
		if _, err := w.Write(e.data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

type manifest struct {
	Config   string   `json:"Config"`
	RepoTags []string `json:"RepoTags"`
	Layers   []string `json:"Layers"`
}

func Pack(p, m string) ([]byte, error) {
	b, mark, e := inputs(p, m)
	if e != nil {
		return nil, e
	}
	layer, e := archive(layerEntries(b, mark))
	if e != nil {
		return nil, e
	}
	config := []byte(`{"architecture":"amd64","os":"linux","config":{},"rootfs":{"type":"layers","diff_ids":["` + digest(layer) + `"]}}`)
	name := digest(config)[7:] + ".json"
	man, _ := json.Marshal([]manifest{{name, []string{}, []string{"layer.tar"}}})
	return archive([]entry{{"manifest.json", 0644, 0, tar.TypeReg, man}, {name, 0644, 0, tar.TypeReg, config}, {"layer.tar", 0644, 0, tar.TypeReg, layer}})
}

// decode independently checks the tar envelope, including canonical zero padding.
func decode(b []byte, max int) (map[string]entry, error) {
	if len(b) > max || len(b)%512 != 0 {
		return nil, ErrInvalid
	}
	r := bytes.NewReader(b)
	tr := tar.NewReader(r)
	out := map[string]entry{}
	end := 0
	for {
		h, e := tr.Next()
		if e == io.EOF {
			break
		}
		if e != nil || len(out) >= 8 {
			return nil, ErrInvalid
		}
		if h.Size < 0 || h.Size > int64(max) || h.Linkname != "" || h.Uname != "" || h.Gname != "" || h.Devmajor != 0 || h.Devminor != 0 || len(h.PAXRecords) != 0 || h.Format != tar.FormatUSTAR || !h.ModTime.Equal(time.Unix(0, 0)) || !h.AccessTime.IsZero() || !h.ChangeTime.IsZero() || h.Uid != h.Gid {
			return nil, ErrInvalid
		}
		if _, ok := out[h.Name]; ok {
			return nil, ErrInvalid
		}
		data, e := io.ReadAll(io.LimitReader(tr, h.Size+1))
		if e != nil || int64(len(data)) != h.Size {
			return nil, ErrInvalid
		}
		out[h.Name] = entry{h.Name, h.Mode, h.Uid, h.Typeflag, data}
		end = len(b) - r.Len()
		pad := (512 - end%512) % 512
		if end+pad > len(b) || !allZero(b[end:end+pad]) {
			return nil, ErrInvalid
		}
		end += pad
	}
	if len(b)-end != 1024 || !allZero(b[end:]) {
		return nil, ErrInvalid
	}
	return out, nil
}
func allZero(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
func field(e entry, mode int64, uid int, kind byte) bool {
	return e.mode == mode && e.uid == uid && e.kind == kind
}

// allowedJSONKey defines the exact case-sensitive keys of both wire documents.
// Typed encoding/json decoding alone accepts ambiguous case aliases.
func allowedJSONKey(path, key string) bool {
	switch path {
	case "$":
		return key == "architecture" || key == "os" || key == "config" || key == "rootfs"
	case "$[]":
		return key == "Config" || key == "RepoTags" || key == "Layers"
	case "$.rootfs":
		return key == "type" || key == "diff_ids"
	default:
		// In particular, config must remain an empty object.
		return false
	}
}

// jsonValue rejects unknown, case-alias, and duplicate keys before typed decoding.
func jsonValue(d *json.Decoder, path string) error {
	t, e := d.Token()
	if e != nil {
		return ErrInvalid
	}
	if delim, ok := t.(json.Delim); ok {
		switch delim {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				k, e := d.Token()
				s, ok := k.(string)
				if e != nil || !ok || seen[s] || !allowedJSONKey(path, s) {
					return ErrInvalid
				}
				seen[s] = true
				if jsonValue(d, path+"."+s) != nil {
					return ErrInvalid
				}
			}
		case '[':
			for d.More() {
				if jsonValue(d, path+"[]") != nil {
					return ErrInvalid
				}
			}
		default:
			return ErrInvalid
		}
		_, e = d.Token()
		return e
	}
	return nil
}
func strictJSON(b []byte, v any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	if jsonValue(d, "$") != nil {
		return ErrInvalid
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrInvalid
	}
	d = json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return ErrInvalid
	}
	return nil
}
func Inspect(a []byte, p, m string) (Metadata, error) {
	fail := func() (Metadata, error) { return Metadata{}, ErrInvalid }
	b, mark, e := inputs(p, m)
	if e != nil {
		return fail()
	}
	outer, e := decode(a, MaxArchive)
	if e != nil || len(outer) != 3 {
		return fail()
	}
	me, ok := outer["manifest.json"]
	if !ok || len(me.data) > 4096 || !field(me, 0644, 0, tar.TypeReg) {
		return fail()
	}
	var mans []manifest
	if strictJSON(me.data, &mans) != nil || len(mans) != 1 || len(mans[0].RepoTags) != 0 || len(mans[0].Layers) != 1 || mans[0].Layers[0] != "layer.tar" {
		return fail()
	}
	c, ok := outer[mans[0].Config]
	if !ok || len(c.data) > 16384 || !field(c, 0644, 0, tar.TypeReg) || mans[0].Config != digest(c.data)[7:]+".json" {
		return fail()
	}
	var cfg struct {
		Architecture string                     `json:"architecture"`
		OS           string                     `json:"os"`
		Config       map[string]json.RawMessage `json:"config"`
		RootFS       struct {
			Type    string   `json:"type"`
			DiffIDs []string `json:"diff_ids"`
		} `json:"rootfs"`
	}
	if strictJSON(c.data, &cfg) != nil || cfg.Architecture != "amd64" || cfg.OS != "linux" || cfg.Config == nil || len(cfg.Config) != 0 || cfg.RootFS.Type != "layers" || len(cfg.RootFS.DiffIDs) != 1 {
		return fail()
	}
	l, ok := outer["layer.tar"]
	if !ok || !field(l, 0644, 0, tar.TypeReg) || cfg.RootFS.DiffIDs[0] != digest(l.data) {
		return fail()
	}
	ls, e := decode(l.data, MaxArchive)
	if e != nil || len(ls) != 4 {
		return fail()
	}
	for _, want := range layerEntries(b, mark) {
		got, ok := ls[want.name]
		if !ok || !field(got, want.mode, want.uid, want.kind) || !bytes.Equal(got.data, want.data) {
			return fail()
		}
	}
	return Metadata{digest(c.data), digest(a), digest(b), digest(mark), digest(l.data), "preparation-only"}, nil
}
