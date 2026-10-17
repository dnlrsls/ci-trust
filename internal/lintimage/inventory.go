package lintimage

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type node struct{ kind, target string }

// InspectInventory compares every filesystem record without extracting it.
// Consistency with supplied metadata is not security admission or ELF proof.
func InspectInventory(archive, inventory io.Reader, want Identities) (Report, error) {
	layer, e := envelope(archive, want)
	if e != nil {
		return Report{}, ErrInvalid
	}
	data, e := bounded(inventory, 16<<20)
	if e != nil {
		return Report{}, ErrInvalid
	}
	expected, e := canonical(data)
	if e != nil || physicalLayer(layer) != nil {
		return Report{}, ErrInvalid
	}
	nodes := map[string]node{}
	records := map[string]map[string]any{}
	var total, metadata int64
	e = scan(layer, false, func(h *tar.Header, b []byte) error {
		name := h.Name
		if h.Typeflag == tar.TypeDir {
			name = strings.TrimSuffix(name, "/")
		}
		if !safeName(name) || len(nodes) >= 50000 {
			return ErrInvalid
		}
		if _, ok := nodes[name]; ok {
			return ErrInvalid
		}
		for k := range h.PAXRecords {
			if k != "path" && k != "linkpath" {
				return ErrInvalid
			}
		}
		r := map[string]any{"path": name, "mode": "0o" + strconv.FormatInt(h.Mode, 8), "uid": 0, "gid": 0}
		n := node{}
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			if h.Mode != 0444 && h.Mode != 0555 || h.Linkname != "" {
				return ErrInvalid
			}
			n.kind = "file"
			r["size"] = len(b)
			r["sha256"] = sum(b)
			total += int64(len(b))
		case tar.TypeDir:
			if h.Mode != 0555 || h.Size != 0 || h.Linkname != "" {
				return ErrInvalid
			}
			n.kind = "dir"
		case tar.TypeSymlink:
			if h.Mode != 0777 || h.Size != 0 || !safeText(h.Linkname) {
				return ErrInvalid
			}
			n.kind = "link"
			n.target = h.Linkname
			r["target"] = h.Linkname
		default:
			return ErrInvalid
		}
		metadata += int64(len(name) + len(n.target))
		if metadata > 8<<20 || total > MaxArchiveBytes {
			return ErrInvalid
		}
		r["type"] = n.kind
		nodes[name] = n
		records[name] = r
		return nil
	})
	if e != nil || namespace(nodes) != nil {
		return Report{}, ErrInvalid
	}
	names := make([]string, 0, len(nodes))
	for name := range nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	ordered := make([]map[string]any, 0, len(names))
	for _, name := range names {
		ordered = append(ordered, records[name])
	}
	actual, e := json.Marshal(ordered)
	if e != nil || !bytes.Equal(actual, expected) {
		return Report{}, ErrInvalid
	}
	return Report{Verdict: "INVENTORY_CONSISTENT_NOT_ADMITTED", Identities: want, InventoryEntries: len(nodes), FileBytes: total}, nil
}
func safeText(s string) bool {
	if s == "" || len(s) > 16384 || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r < 32 || r == 127 || r == '\\' {
			return false
		}
	}
	return true
}
func safeName(s string) bool {
	if !safeText(s) || path.IsAbs(s) || path.Clean(s) != s || s == "." || s == ".." || strings.HasPrefix(s, "../") {
		return false
	}
	for _, part := range strings.Split(s, "/") {
		if strings.HasPrefix(part, ".wh.") {
			return false
		}
	}
	return true
}
func namespace(nodes map[string]node) error {
	budget := 1000000
	for name, n := range nodes {
		for p := path.Dir(name); p != "."; p = path.Dir(p) {
			if nodes[p].kind != "dir" {
				return ErrInvalid
			}
		}
		if n.kind != "link" {
			continue
		}
		pending := strings.Split(name, "/")
		resolved := []string{}
		rewrites := 0
		for len(pending) > 0 {
			budget--
			if budget < 0 {
				return ErrInvalid
			}
			component := pending[0]
			pending = pending[1:]
			if component == "" || component == "." {
				continue
			}
			if component == ".." {
				if len(resolved) == 0 {
					return ErrInvalid
				}
				resolved = resolved[:len(resolved)-1]
				continue
			}
			prefix := strings.Join(append(resolved, component), "/")
			entry, ok := nodes[prefix]
			if !ok {
				return ErrInvalid
			}
			if entry.kind == "link" {
				rewrites++
				if rewrites > 128 {
					return ErrInvalid
				}
				if strings.HasPrefix(entry.target, "/") {
					resolved = nil
				}
				pending = append(strings.Split(entry.target, "/"), pending...)
				continue
			}
			// Even a trailing slash/dot requires a directory. Never cancel an
			// unresolved component or symlink by lexically cleaning '..'.
			if len(pending) > 0 && entry.kind != "dir" {
				return ErrInvalid
			}
			resolved = append(resolved, component)
		}
	}
	return nil
}

// Validate physical records too; archive/tar hides local PAX and GNU helpers.
// Only the owned USTAR/local-PAX subset is supported, never sparse/GNU/global.
func physicalLayer(data []byte) error {
	pending := false
	for offset := 0; offset < len(data); {
		if len(data)-offset < 512 {
			return ErrInvalid
		}
		h := data[offset : offset+512]
		if bytes.Equal(h, make([]byte, 512)) {
			if pending || len(data)-offset < 1024 {
				return ErrInvalid
			}
			for _, b := range data[offset:] {
				if b != 0 {
					return ErrInvalid
				}
			}
			return nil
		}
		size, e := strconv.ParseInt(strings.Trim(string(h[124:136]), "\x00 "), 8, 64)
		if e != nil || size < 0 || size > int64(len(data)-offset-512) || string(h[257:265]) != "ustar\x0000" {
			return ErrInvalid
		}
		end := offset + 512 + int(size)
		next := end + int((512-size%512)%512)
		if next > len(data) {
			return ErrInvalid
		}
		for _, b := range data[end:next] {
			if b != 0 {
				return ErrInvalid
			}
		}
		switch h[156] {
		case 'x':
			if strings.TrimRight(string(h[:100]), "\x00") != "././@PaxHeader" {
				return ErrInvalid
			}
			for _, span := range [][]byte{h[157:257], h[345:512]} {
				for _, b := range span {
					if b != 0 {
						return ErrInvalid
					}
				}
			}
			if pending || size > 65536 {
				return ErrInvalid
			}
			pending = true
			for _, field := range [][2]int{{100, 108}, {108, 116}, {116, 124}, {136, 148}, {329, 337}, {337, 345}} {
				v, e := strconv.ParseInt(strings.Trim(string(h[field[0]:field[1]]), "\x00 "), 8, 64)
				if e != nil || v != 0 {
					return ErrInvalid
				}
			}
			for _, b := range h[265:329] {
				if b != 0 {
					return ErrInvalid
				}
			}
			if pax(data[offset+512:end]) != nil {
				return ErrInvalid
			}
		case '0', 0, '5', '2':
			pending = false
		default:
			return ErrInvalid
		}
		offset = next
	}
	return ErrInvalid
}
func pax(b []byte) error {
	seen := map[string]bool{}
	for len(b) > 0 {
		space := bytes.IndexByte(b, ' ')
		if space <= 0 {
			return ErrInvalid
		}
		n, e := strconv.Atoi(string(b[:space]))
		if e != nil || strconv.Itoa(n) != string(b[:space]) || n <= space+1 || n > len(b) || b[n-1] != '\n' {
			return ErrInvalid
		}
		pair := bytes.SplitN(b[space+1:n-1], []byte("="), 2)
		if len(pair) != 2 {
			return ErrInvalid
		}
		key := string(pair[0])
		if key != "path" && key != "linkpath" || seen[key] || !safeText(string(pair[1])) {
			return ErrInvalid
		}
		seen[key] = true
		b = b[n:]
	}
	return nil
}
