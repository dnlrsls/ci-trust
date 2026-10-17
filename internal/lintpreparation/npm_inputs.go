package lintpreparation

import (
	"encoding/base64"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

var npmVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9.-]+)?$`)
var npmIntegrityPattern = regexp.MustCompile(`^sha512-[A-Za-z0-9+/]+={0,2}$`)

func npmObject(data []byte) (map[string]any, error) {
	v, err := UniqueJSON(data)
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, NewValidationFailure("")
	}
	return m, nil
}
func npmDependencies(v any) bool {
	m, ok := v.(map[string]any)
	return ok && len(m) == 1 && m["eslint"] == ESLintVersion
}

// ValidateOwnedManifest validates without rewriting the original artifact bytes.
func ValidateOwnedManifest(data []byte) error {
	m, err := npmObject(data)
	if err != nil {
		return err
	}
	if len(m) != 4 || m["name"] != "ci-trust-owned-eslint-preparation" || m["version"] != "0.0.0" || m["private"] != true || !npmDependencies(m["dependencies"]) {
		return NewValidationFailure("")
	}
	return nil
}
func validateBundledNPM(data []byte) error {
	m, err := npmObject(data)
	if err != nil {
		return err
	}
	if m["version"] != NPMVersion {
		return NewValidationFailure("")
	}
	return nil
}

// npmTruthy follows JSON-representable Python truthiness, not strict bool typing.
// Integer zero detection scans digits, never allocating arbitrary-precision ints.
// Decimal/exponent values use Python's binary64 model, including underflow to zero.
func npmTruthy(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case bool:
		return x
	case string:
		return x != ""
	case []any:
		return len(x) != 0
	case map[string]any:
		return len(x) != 0
	case json.Number:
		s := string(x)
		if strings.ContainsAny(s, ".eE") {
			f, _ := strconv.ParseFloat(s, 64)
			return f != 0
		}
		return strings.Trim(s, "-0") != ""
	default:
		return true
	}
}

// npmRegistryURL checks urlsplit's relevant components, deliberately NOT the
// download policy: empty query/fragment delimiters and arbitrary paths are legal.
// Python removes TAB/CR/LF anywhere and leading WHATWG C0 controls/spaces. Avoid
// net/url's extra path-escape restrictions and authority normalization entirely.
func npmRegistryURL(s string) bool {
	s = strings.TrimLeftFunc(s, func(r rune) bool { return r <= 32 })
	s = strings.NewReplacer("\t", "", "\r", "", "\n", "").Replace(s)
	scheme, rest, ok := strings.Cut(s, "://")
	if !ok || len(scheme) != len("https") || !strings.EqualFold(scheme, "https") {
		return false
	}
	end := strings.IndexAny(rest, "/?#")
	if end < 0 {
		end = len(rest)
	}
	if rest[:end] != "registry.npmjs.org" {
		return false
	}
	tail := rest[end:]
	beforeFragment, fragment, hasFragment := strings.Cut(tail, "#")
	_, query, hasQuery := strings.Cut(beforeFragment, "?")
	return (!hasFragment || fragment == "") && (!hasQuery || query == "")
}

// ValidateNPMLock returns no partial authority and performs no I/O. Diagnostic
// details remain in the existing fixed vocabulary; callers own stage wrapping.
func ValidateNPMLock(data []byte) error {
	lock, err := npmObject(data)
	if err != nil {
		return err
	}
	version, ok := lock["lockfileVersion"].(json.Number)
	n, _ := strconv.ParseFloat(string(version), 64)
	packages, pok := lock["packages"].(map[string]any)
	root, _ := packages[""].(map[string]any)
	if !ok || n != 3 || !pok || !npmDependencies(root["dependencies"]) {
		return NewValidationFailure("")
	}
	if len(packages) > MaxPackages {
		return NewValidationFailure("")
	}
	for name, value := range packages {
		if name == "" {
			continue
		}
		if _, err := SafePath(name); err != nil {
			return err
		}
		p, ok := value.(map[string]any)
		if !ok {
			return NewValidationFailure("")
		}
		version, vok := p["version"].(string)
		if !strings.HasPrefix(name, "node_modules/") || npmTruthy(p["link"]) || !vok || !npmVersionPattern.MatchString(version) {
			return NewValidationFailure("")
		}
		resolved, rok := p["resolved"].(string)
		if !rok || !npmRegistryURL(resolved) {
			return NewValidationFailure("")
		}
		integrity, iok := p["integrity"].(string)
		// A 64-byte digest has exactly 88 base64 characters (including two pads).
		// Non-strict decoding retains Python validate=True's unused-pad-bit behavior.
		if !iok || len(integrity) != 95 || !npmIntegrityPattern.MatchString(integrity) {
			return NewValidationFailure("")
		}
		digest, err := base64.StdEncoding.DecodeString(integrity[7:])
		if err != nil || len(digest) != 64 {
			return NewValidationFailure("")
		}
	}
	eslint, _ := packages["node_modules/eslint"].(map[string]any)
	if eslint["version"] != ESLintVersion {
		return NewValidationFailure("")
	}
	return nil
}
