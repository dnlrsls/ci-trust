// Package lintpreparation contains preparation-only host tooling primitives.
package lintpreparation

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	NodeVersion   = "24.22.0"
	NPMVersion    = "11.19.1"
	ESLintVersion = "10.12.0"
	NodeArchive   = "node-v24.22.0-linux-x64.tar.gz"
	NodeSHA256    = "28ea0a438de9a13bd7891fd758eee6c7819ee8af5442dfd8d45bd65e56d4228c"
	Base          = "https://gcr.io/v2/distroless/cc-debian12/"
	BaseManifest  = "sha256:777e96cf322c46bc32aca926c263624c4dc8d7cf37e2fa65ba2c7e697318ebbb"
	MaxBytes      = 512 << 20
	MaxFiles      = 50000
	MaxLayers     = 64
	MaxPackages   = 1024
)
const stages = "host-inputs node-download node-extract owned-manifest npm-setup npm-lock npm-ci base-image rootfs base-manifest base-config base-platform base-layers base-layer-download base-layer-decompress base-layer-diff base-layer-import image-validation artifact-write"
const categories = "http network timeout filesystem archive validation process-failed origin-denied unexpected"
const reasons = "download-identity platform layer-count layer-media-type layer-diff-id json-duplicate-key path-unsafe path-invalid link-invalid layer-duplicate-or-limit layer-privileged layer-whiteout layer-ancestor layer-expanded-limit layer-hardlink layer-special-entry layer-type-replacement rootfs-limit rootfs-missing link-cycle"

var shaPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var integerPattern = regexp.MustCompile(`^-?(0|[1-9][0-9]*)$`)

func ValidDigest(s string) bool { return shaPattern.MatchString(s) }
func member(list, s string) bool {
	for _, v := range strings.Fields(list) {
		if v == s {
			return true
		}
	}
	return false
}

type ValidationFailure struct{ Reason string }

func (*ValidationFailure) Error() string { return "owned validation failure" }
func NewValidationFailure(reason string) *ValidationFailure {
	if !member(reasons, reason) {
		reason = ""
	}
	return &ValidationFailure{reason}
}

type PreparationFailure struct{ Stage, Category, Reason string }

func NewPreparationFailure(stage, category, reason string) *PreparationFailure {
	if !member(stages, stage) {
		stage = "unknown"
	}
	if !member(categories, category) {
		category = "unexpected"
	}
	if !member(reasons, reason) {
		reason = ""
	}
	return &PreparationFailure{stage, category, reason}
}
func (f *PreparationFailure) Error() string {
	safe := NewPreparationFailure(f.Stage, f.Category, f.Reason)
	return safe.Stage + ":" + safe.Category
}

// ErrorCategory never formats the error. HTTP status failures must be explicitly
// marked by the downloader (Go's HTTP client does not return status errors).
func ErrorCategory(err error) string {
	var n net.Error
	var u *url.Error
	var p *os.PathError
	var v *ValidationFailure
	var j *json.SyntaxError
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &n) && n.Timeout()) {
		return "timeout"
	}
	if errors.As(err, &u) {
		return "network"
	}
	if errors.Is(err, tar.ErrHeader) || errors.Is(err, tar.ErrFieldTooLong) {
		return "archive"
	}
	if errors.As(err, &p) || errors.Is(err, os.ErrNotExist) || errors.Is(err, os.ErrPermission) {
		return "filesystem"
	}
	if errors.As(err, &v) || errors.As(err, &j) {
		return "validation"
	}
	return "unexpected"
}
func InStage(stage string, run func() error) error {
	if !member(stages, stage) {
		return NewPreparationFailure("unknown", "unexpected", "")
	}
	err := run()
	if err == nil {
		return nil
	}
	var f *PreparationFailure
	if errors.As(err, &f) {
		return NewPreparationFailure(f.Stage, f.Category, f.Reason)
	}
	var v *ValidationFailure
	if errors.As(err, &v) {
		return NewPreparationFailure(stage, "validation", v.Reason)
	}
	return NewPreparationFailure(stage, ErrorCategory(err), "")
}
func Diagnostic(err error) string {
	f := NewPreparationFailure("unknown", ErrorCategory(err), "")
	var nested *PreparationFailure
	if errors.As(err, &nested) {
		f = NewPreparationFailure(nested.Stage, nested.Category, nested.Reason)
	}
	reason := ""
	if f.Reason != "" {
		reason = "; reason=" + f.Reason
	}
	return "owned lint preparation failed; stage=" + f.Stage + "; category=" + f.Category + reason + "; no runtime admission or ESLint execution"
}
func Digest(data []byte) string {
	h := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(h[:])
}

// MaxJSONDepth bounds container nesting (root container 1, scalar 0). This
// technical budget reflects Python's implicit recursion bound, not its exact limit.
const MaxJSONDepth = 1000

// validJSONUnicode rejects unsupported malformed UTF-16 before encoding/json
// can replace it with U+FFFD. Escaped backslashes are not Unicode escapes.
func validJSONUnicode(data []byte) bool {
	inString := false
	for i := 0; i < len(data); i++ {
		if data[i] == '"' {
			inString = !inString
			continue
		}
		if !inString || data[i] != '\\' {
			continue
		}
		i++
		if i >= len(data) {
			return false
		}
		if data[i] != 'u' {
			continue
		}
		if i+4 >= len(data) {
			return false
		}
		r, err := strconv.ParseUint(string(data[i+1:i+5]), 16, 16)
		if err != nil {
			return false
		}
		i += 4
		if r >= 0xdc00 && r <= 0xdfff {
			return false
		}
		if r < 0xd800 || r > 0xdbff {
			continue
		}
		if i+6 >= len(data) || data[i+1] != '\\' || data[i+2] != 'u' {
			return false
		}
		low, err := strconv.ParseUint(string(data[i+3:i+7]), 16, 16)
		if err != nil || low < 0xdc00 || low > 0xdfff {
			return false
		}
		i += 6
	}
	return true
}

// UniqueJSON preserves arbitrary-precision number spelling and checks every
// object, including nested objects. It rejects any second value or trailing junk,
// unpaired surrogate escapes, invalid UTF-8 and nesting above MaxJSONDepth.
func UniqueJSON(data []byte) (any, error) {
	if !utf8.Valid(data) || !validJSONUnicode(data) {
		return nil, NewValidationFailure("")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	v, err := jsonValue(d, 0)
	if err != nil {
		return nil, err
	}
	if _, err = d.Token(); err != io.EOF {
		return nil, NewValidationFailure("")
	}
	return v, nil
}
func jsonValue(d *json.Decoder, depth int) (any, error) {
	t, err := d.Token()
	if err != nil {
		return nil, NewValidationFailure("")
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return t, nil
	}
	depth++
	if depth > MaxJSONDepth {
		return nil, NewValidationFailure("")
	}
	switch delim {
	case '{':
		m := map[string]any{}
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return nil, NewValidationFailure("")
			}
			key, ok := k.(string)
			if !ok {
				return nil, NewValidationFailure("")
			}
			if _, ok = m[key]; ok {
				return nil, NewValidationFailure("json-duplicate-key")
			}
			v, e := jsonValue(d, depth)
			if e != nil {
				return nil, e
			}
			m[key] = v
		}
		end, e := d.Token()
		if e != nil || end != json.Delim('}') {
			return nil, NewValidationFailure("")
		}
		return m, nil
	case '[':
		a := []any{}
		for d.More() {
			v, e := jsonValue(d, depth)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		end, e := d.Token()
		if e != nil || end != json.Delim(']') {
			return nil, NewValidationFailure("")
		}
		return a, nil
	}
	return nil, NewValidationFailure("")
}

// Canonical supports contract JSON: string-keyed objects, arrays, strings,
// booleans, null and integers. Floats/exponent numbers are deliberately rejected:
// Go and Python float formatting are not interchangeable. Integers are unbounded.
// Container nesting is limited to MaxJSONDepth, including direct caller values.
func Canonical(value any) ([]byte, error) {
	var b bytes.Buffer
	var emit func(any, int) error
	emit = func(v any, depth int) error {
		switch v.(type) {
		case []any, map[string]any:
			depth++
			if depth > MaxJSONDepth {
				return NewValidationFailure("")
			}
		}
		switch x := v.(type) {
		case nil:
			b.WriteString("null")
		case bool:
			b.WriteString(strconv.FormatBool(x))
		case string:
			if !utf8.ValidString(x) {
				return NewValidationFailure("")
			}
			b.WriteByte('"')
			for _, r := range x {
				switch r {
				case '"', '\\':
					b.WriteByte('\\')
					b.WriteRune(r)
				case '\b':
					b.WriteString(`\b`)
				case '\f':
					b.WriteString(`\f`)
				case '\n':
					b.WriteString(`\n`)
				case '\r':
					b.WriteString(`\r`)
				case '\t':
					b.WriteString(`\t`)
				default:
					if r < 32 || r >= 127 {
						if r > 0xffff {
							a, c := utf16.EncodeRune(r)
							fmt.Fprintf(&b, `\u%04x\u%04x`, a, c)
						} else {
							fmt.Fprintf(&b, `\u%04x`, r)
						}
					} else {
						b.WriteRune(r)
					}
				}
			}
			b.WriteByte('"')
		case json.Number:
			s := string(x)
			if !integerPattern.MatchString(s) {
				return NewValidationFailure("")
			}
			if s == "-0" {
				s = "0"
			}
			b.WriteString(s)
		case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
			b.WriteString(fmt.Sprint(x))
		case []any:
			b.WriteByte('[')
			for i, v := range x {
				if i > 0 {
					b.WriteByte(',')
				}
				if e := emit(v, depth); e != nil {
					return e
				}
			}
			b.WriteByte(']')
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			b.WriteByte('{')
			for i, k := range keys {
				if i > 0 {
					b.WriteByte(',')
				}
				if e := emit(k, depth); e != nil {
					return e
				}
				b.WriteByte(':')
				if e := emit(x[k], depth); e != nil {
					return e
				}
			}
			b.WriteByte('}')
		default:
			return NewValidationFailure("")
		}
		return nil
	}
	if err := emit(value, 0); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}
