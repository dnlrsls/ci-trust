package lintpreparation

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const ownedManifest = `{"name":"ci-trust-owned-eslint-preparation","version":"0.0.0","private":true,"dependencies":{"eslint":"10.12.0"}}`

func npmPackage() map[string]any {
	return map[string]any{"version": ESLintVersion, "resolved": "https://registry.npmjs.org/eslint/-/eslint.tgz", "integrity": "sha512-" + base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{'x'}, 64))}
}
func npmLock() map[string]any {
	return map[string]any{"lockfileVersion": 3, "packages": map[string]any{"": map[string]any{"dependencies": map[string]any{"eslint": ESLintVersion}}, "node_modules/eslint": npmPackage()}}
}
func npmBytes(t *testing.T, v any) []byte {
	t.Helper()
	b, e := json.Marshal(v)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func npmCheck(t *testing.T, f func([]byte) error, b []byte, valid bool, reason string) {
	t.Helper()
	before := bytes.Clone(b)
	err := f(b)
	if !bytes.Equal(b, before) {
		t.Fatal("input mutated")
	}
	if valid {
		if err != nil {
			t.Fatalf("valid input rejected: %v", err)
		}
		return
	}
	var failure *ValidationFailure
	if !errors.As(err, &failure) {
		t.Fatalf("expected redacted validation failure, got %v", err)
	}
	if failure.Reason != reason || err.Error() != "owned validation failure" {
		t.Fatalf("unexpected diagnostic: %#v", failure)
	}
}
func TestNPMInputsManifest(t *testing.T) {
	npmCheck(t, ValidateOwnedManifest, []byte(" \n"+ownedManifest), true, "")
	for _, b := range []string{`null`, `[]`, `{}`, strings.Replace(ownedManifest, `true`, `1`, 1), strings.Replace(ownedManifest, `10.12.0`, `*`, 1), strings.Replace(ownedManifest, `"name":`, `"extra":0,"name":`, 1), strings.Replace(ownedManifest, `"eslint":`, `"other":"1","eslint":`, 1)} {
		npmCheck(t, ValidateOwnedManifest, []byte(b), false, "")
	}
}
func TestNPMInputsBundledVersion(t *testing.T) {
	npmCheck(t, validateBundledNPM, []byte(`{"version":"11.19.1","ordinary":[1,null]}`), true, "")
	for _, b := range []string{`null`, `[]`, `{}`, `{"version":11}`, `{"version":"11.19.0"}`} {
		npmCheck(t, validateBundledNPM, []byte(b), false, "")
	}
}
func TestNPMInputsJSON(t *testing.T) {
	for _, f := range []func([]byte) error{ValidateOwnedManifest, validateBundledNPM, ValidateNPMLock} {
		for _, b := range []string{`{`, `null`, `[]`, `{"x":"\ud800"}`, `{"x":null} true`, strings.Repeat(`[`, MaxJSONDepth+1) + strings.Repeat(`]`, MaxJSONDepth+1)} {
			npmCheck(t, f, []byte(b), false, "")
		}
		for _, b := range []string{`{"a":1,"a":2}`, `{"nested":{"a":1,"a":2}}`} {
			npmCheck(t, f, []byte(b), false, "json-duplicate-key")
		}
	}
}
func TestNPMInputsLock(t *testing.T) {
	npmCheck(t, ValidateNPMLock, npmBytes(t, npmLock()), true, "")
	cases := []struct {
		name   string
		change func(map[string]any)
	}{
		{"root version", func(l map[string]any) { l["lockfileVersion"] = 2 }},
		{"root deps", func(l map[string]any) { l["packages"].(map[string]any)[""] = map[string]any{"dependencies": nil} }},
		{"packages type", func(l map[string]any) { l["packages"] = []any{} }},
		{"package type", func(l map[string]any) { l["packages"].(map[string]any)["node_modules/eslint"] = nil }},
		{"missing eslint", func(l map[string]any) { delete(l["packages"].(map[string]any), "node_modules/eslint") }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := npmLock()
			c.change(l)
			npmCheck(t, ValidateNPMLock, npmBytes(t, l), false, "")
		})
	}
	for _, path := range []string{"elsewhere", "node_modules/../escape", "/node_modules/x", "node_modules\\x"} {
		t.Run(path, func(t *testing.T) {
			l := npmLock()
			l["packages"].(map[string]any)[path] = npmPackage()
			reason := ""
			if path != "elsewhere" {
				reason = "path-unsafe"
			}
			npmCheck(t, ValidateNPMLock, npmBytes(t, l), false, reason)
		})
	}
	for key, values := range map[string][]any{
		"version":   {"*", "1.2", "1.2.3+build", "1.2.3", nil, 3},
		"resolved":  {"http://registry.npmjs.org/x", "https://REGISTRY.npmjs.org/x", "https://registry.npmjs.org:443/x", "https://u@registry.npmjs.org/x", "https://registry.npmjs.org/x?q", "https://registry.npmjs.org/x#f", "httpſ://registry.npmjs.org/x", "https://registry.npmjs.org:/x", "https://registry.npmjs.org./x", "https://registry.npmjs.org/x#?", nil, 3},
		"integrity": {"sha512-bad", "sha256-abc", "sha512-" + base64.StdEncoding.EncodeToString([]byte("short")), nil, 3},
		"link":      {true, "false", 1, json.Number("1e9999"), []any{false}, map[string]any{"x": false}},
	} {
		for i, v := range values {
			t.Run(fmt.Sprintf("%s/%d", key, i), func(t *testing.T) {
				l := npmLock()
				l["packages"].(map[string]any)["node_modules/eslint"].(map[string]any)[key] = v
				npmCheck(t, ValidateNPMLock, npmBytes(t, l), false, "")
			})
		}
	}
}
func TestNPMInputsAlternates(t *testing.T) {
	for _, link := range []any{nil, false, "", 0, json.Number("-0"), json.Number("0e999999"), json.Number("1e-9999"), []any{}, map[string]any{}} {
		l := npmLock()
		l["packages"].(map[string]any)["node_modules/eslint"].(map[string]any)["link"] = link
		npmCheck(t, ValidateNPMLock, npmBytes(t, l), true, "")
	}
	for _, url := range []string{"HTTPS://registry.npmjs.org/anything", "https://registry.npmjs.org", "https://registry.npmjs.org/x?#", "https://registry.npmjs.org/%zz", " \x00https://registry.npmjs.org/x\t"} {
		l := npmLock()
		l["packages"].(map[string]any)["node_modules/eslint"].(map[string]any)["resolved"] = url
		npmCheck(t, ValidateNPMLock, npmBytes(t, l), true, "")
	}
	l := npmLock()
	p := npmPackage()
	p["version"] = "01.2.3-pre..-"
	p["hasInstallScript"] = true
	l["packages"].(map[string]any)["node_modules/extra"] = p
	// Python validate=True checks padding structure, not unused pad bits.
	sri := p["integrity"].(string)
	p["integrity"] = sri[:len(sri)-3] + "B=="
	npmCheck(t, ValidateNPMLock, npmBytes(t, l), true, "")
	for _, v := range []json.Number{"3.0", "3e0"} {
		l := npmLock()
		l["lockfileVersion"] = v
		npmCheck(t, ValidateNPMLock, npmBytes(t, l), true, "")
	}
}
func TestNPMInputsExtraordinaryTypes(t *testing.T) {
	for _, v := range []any{nil, true, []any{}, map[string]any{}} {
		for _, key := range []string{"name", "version", "private", "dependencies"} {
			var m map[string]any
			if err := json.Unmarshal([]byte(ownedManifest), &m); err != nil {
				t.Fatal(err)
			}
			m[key] = v
			if key == "private" && v == true {
				continue
			}
			npmCheck(t, ValidateOwnedManifest, npmBytes(t, m), false, "")
		}
		for _, key := range []string{"version", "resolved", "integrity"} {
			l := npmLock()
			l["packages"].(map[string]any)["node_modules/eslint"].(map[string]any)[key] = v
			npmCheck(t, ValidateNPMLock, npmBytes(t, l), false, "")
		}
	}
}
func TestNPMInputsGraphLimit(t *testing.T) {
	for _, n := range []int{MaxPackages, MaxPackages + 1} {
		l := npmLock()
		p := l["packages"].(map[string]any)
		for i := len(p); i < n; i++ {
			p[fmt.Sprintf("node_modules/p%d", i)] = npmPackage()
		}
		npmCheck(t, ValidateNPMLock, npmBytes(t, l), n == MaxPackages, "")
	}
}
