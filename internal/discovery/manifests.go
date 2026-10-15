package discovery

import (
	"bytes"
	"encoding/json"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

// Analyze inventories static declarations only. It never evaluates a command or config.
func Analyze(snapshot Snapshot, options Options) Result {
	r := Result{Roots: []Root{}, Tools: []Tool{}, Candidates: []Candidate{}, Diagnostics: []Diagnostic{}, Limitations: []string{}}
	diag := func(code, p string) {
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Code: code, Path: p, Message: "Static discovery could not resolve this input.", State: "UNKNOWN"})
	}
	defaults := DefaultOptions()
	values := []int64{int64(options.MaxDepth), int64(options.MaxEntries), options.MaxFileBytes, options.MaxTotalBytes, int64(options.MaxReferences), int64(options.MaxScriptDepth)}
	defs := []int64{int64(defaults.MaxDepth), int64(defaults.MaxEntries), defaults.MaxFileBytes, defaults.MaxTotalBytes, int64(defaults.MaxReferences), int64(defaults.MaxScriptDepth)}
	caps := []int64{64, 65536, 16 << 20, 64 << 20, 4096, 64}
	for i, v := range values {
		if v < 0 || v > caps[i] {
			diag("invalid_options", "")
			return r
		}
		if v == 0 {
			values[i] = defs[i]
		}
	}
	for _, d := range snapshot.Diagnostics {
		p := d.Path
		if p != "" && !safePath(p) {
			p = ""
		}
		r.Diagnostics = append(r.Diagnostics, Diagnostic{Code: safeCode(d.Code), Path: p, Message: "Snapshot reported an unresolved input.", State: "UNKNOWN"})
	}
	// Snapshot messages are untrusted and can contain host paths or payloads.
	if len(snapshot.Limitations) > 0 {
		r.Limitations = append(r.Limitations, "Snapshot reported omitted or limited inputs.")
	}
	docs := append([]Document(nil), snapshot.Documents...)
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Path == docs[j].Path {
			return bytes.Compare(docs[i].Content, docs[j].Content) < 0
		}
		return docs[i].Path < docs[j].Path
	})
	valid := map[string]Document{}
	rejected := map[string]bool{}
	total := int64(0)
	for i, d := range docs {
		if !safePath(d.Path) {
			diag("unsafe_path", "")
			continue
		}
		if i >= int(values[1]) {
			diag("entry_limit", "")
			break
		}
		if strings.Count(d.Path, "/") > int(values[0]) {
			diag("depth_limit", d.Path)
			continue
		}
		if int64(len(d.Content)) > values[2] {
			diag("file_limit", d.Path)
			continue
		}
		if total+int64(len(d.Content)) > values[3] {
			diag("total_limit", d.Path)
			continue
		}
		total += int64(len(d.Content))
		if _, ok := valid[d.Path]; ok || rejected[d.Path] {
			diag("duplicate_document", d.Path)
			delete(valid, d.Path)
			rejected[d.Path] = true
			continue
		}
		valid[d.Path] = d
	}
	accepted := make([]Document, 0, len(valid))
	for _, d := range valid {
		accepted = append(accepted, d)
	}
	sort.Slice(accepted, func(i, j int) bool { return accepted[i].Path < accepted[j].Path })
	roots := map[string][]Root{}
	addTool := func(root Root, name string, src Source) {
		r.Tools = append(r.Tools, Tool{ID: evidence.StableID("tool", root.ID, name, src.Path, src.Symbol), Root: root.ID, Name: name, Source: src, State: "CONFIGURED"})
	}
	for _, d := range accepted {
		v, ok := valid[d.Path]
		if !ok || !bytes.Equal(v.Content, d.Content) {
			continue
		}
		delete(valid, d.Path)
		base := path.Base(d.Path)
		if base != "go.mod" && base != "package.json" {
			continue
		}
		stack := "npm"
		if base == "go.mod" {
			stack = "go"
		}
		dir := path.Dir(d.Path)
		root := Root{ID: evidence.StableID("root", dir, stack), Path: dir, Stack: stack, Source: Source{Path: d.Path}}
		r.Roots = append(r.Roots, root)
		roots[dir] = append(roots[dir], root)
		if stack == "go" {
			if !moduleLiteral(d.Content) {
				diag("malformed_go_module", d.Path)
				r.Limitations = append(r.Limitations, "Go module syntax outside the supported literal subset remains unknown.")
				continue
			}
			addTool(root, "go", root.Source)
			for _, kind := range []string{"build", "test"} {
				cmd := "go " + kind + " ./..."
				r.Candidates = append(r.Candidates, Candidate{ID: evidence.StableID("candidate", root.ID, cmd), Root: root.ID, Kind: kind, Command: cmd, Source: root.Source, Chain: []Source{root.Source}, State: "UNKNOWN", Origin: "CONVENTION"})
			}
			continue
		}
		obj, err := strictObject(d.Content)
		if err != nil {
			diag("malformed_package_json", d.Path)
			continue
		}
		addTool(root, "npm", root.Source)
		if pm, ok := obj["packageManager"]; ok {
			var text string
			if json.Unmarshal(pm, &text) != nil || !strings.HasPrefix(text, "npm@") {
				diag("unsupported_package_manager", d.Path)
				r.Limitations = append(r.Limitations, "Unsupported package manager declaration.")
			}
		}
		raw, ok := obj["scripts"]
		if !ok {
			continue
		}
		scriptsRaw, err := strictObject(raw)
		if err != nil {
			diag("malformed_scripts", d.Path)
			continue
		}
		scripts := map[string]string{}
		for name, b := range scriptsRaw {
			var text string
			if json.Unmarshal(b, &text) != nil || !scriptName.MatchString(name) {
				diag("malformed_script", d.Path)
				continue
			}
			scripts[name] = text
		}
		names := make([]string, 0, len(scripts))
		for name := range scripts {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			src := Source{Path: d.Path, Symbol: name}
			edges := 0
			cmd, kind, chain, code := resolveScript(name, scripts, d.Path, map[string]bool{}, 0, int(values[5]), &edges, int(values[4]))
			c := Candidate{ID: evidence.StableID("candidate", root.ID, name), Root: root.ID, Kind: kind, Command: cmd, Source: src, Chain: chain, State: "CONFIGURED", Origin: "SCRIPT"}
			if code != "" {
				c.State = "UNKNOWN"
				c.Command = ""
				c.Kind = "unknown"
				diag(code, d.Path)
			} else {
				for _, term := range strings.Split(cmd, " && ") {
					addTool(root, strings.Fields(term)[0], src)
				}
			}
			if code != "" {
				r.Candidates = append(r.Candidates, c)
			} else {
				for index, term := range strings.Split(cmd, " && ") {
					leaf := c
					leaf.Command = term
					leaf.Kind = leafKind(strings.Fields(term))
					leaf.ID = evidence.StableID("candidate", root.ID, name, term, strings.Repeat("x", index))
					r.Candidates = append(r.Candidates, leaf)
				}
			}
		}
	}
	for _, d := range accepted {
		base := path.Base(d.Path)
		if base == "go.mod" || base == "package.json" || !safePath(d.Path) {
			continue
		}
		owners := roots[path.Dir(d.Path)]
		name, stack := marker(base)
		if name == "" {
			continue
		}
		if stack == "unsupported" {
			diag("unsupported_stack", d.Path)
			r.Limitations = append(r.Limitations, "Unsupported stack or package manager marker.")
			continue
		}
		found := false
		for _, root := range owners {
			if root.Stack == stack {
				addTool(root, name, Source{Path: d.Path})
				found = true
			}
		}
		if !found {
			diag("orphan_marker", d.Path)
			r.Limitations = append(r.Limitations, "Tool marker has no same-directory owning manifest.")
		}
	}
	sort.Slice(r.Roots, func(i, j int) bool { return r.Roots[i].ID < r.Roots[j].ID })
	sort.Slice(r.Tools, func(i, j int) bool { return r.Tools[i].ID < r.Tools[j].ID })
	r.Tools = uniqueTools(r.Tools)
	sort.Slice(r.Candidates, func(i, j int) bool { return r.Candidates[i].ID < r.Candidates[j].ID })
	sort.Slice(r.Diagnostics, func(i, j int) bool {
		a, b := r.Diagnostics[i], r.Diagnostics[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Code < b.Code
	})
	sort.Strings(r.Limitations)
	r.Limitations = uniqueStrings(r.Limitations)
	return r
}

var scriptName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.:-]*$`)
var literalToken = regexp.MustCompile(`^[A-Za-z0-9_./:@-]+$`)
var goVersion = regexp.MustCompile(`^[0-9]+\.[0-9]+(\.[0-9]+)?$`)
var toolchainVersion = regexp.MustCompile(`^go[0-9]+\.[0-9]+(\.[0-9]+)?$`)

func safePath(p string) bool {
	return p != "" && p != "." && !strings.ContainsAny(p, "\\:\x00\r\n") && !strings.HasPrefix(p, "/") && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}
func safeCode(s string) string {
	if scriptName.MatchString(s) {
		return s
	}
	return "snapshot_unknown"
}
func moduleLiteral(b []byte) bool {
	found := false
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(strings.SplitN(line, "//", 2)[0])
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if f[0] == "module" {
			if found || len(f) != 2 || !literalToken.MatchString(f[1]) || !safePath(f[1]) || strings.HasPrefix(f[1], "-") {
				return false
			}
			found = true
			continue
		}
		// Other go.mod directives are not interpreted, but must not imitate executable text.
		if len(f) != 2 {
			return false
		}
		switch f[0] {
		case "go":
			if !goVersion.MatchString(f[1]) {
				return false
			}
		case "toolchain":
			if !toolchainVersion.MatchString(f[1]) {
				return false
			}
		default:
			return false
		}
	}
	return found
}

// Walk every JSON object to reject duplicates even in otherwise ignored fields.
func strictObject(b []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if err := walkJSON(dec); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, io.ErrUnexpectedEOF
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(b, &obj); err != nil {
		return nil, err
	}
	if obj == nil {
		return nil, io.ErrUnexpectedEOF
	}
	return obj, nil
}
func walkJSON(d *json.Decoder) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	if delim != '{' && delim != '[' {
		return io.ErrUnexpectedEOF
	}
	seen := map[string]bool{}
	for d.More() {
		if delim == '{' {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return io.ErrUnexpectedEOF
			}
			seen[s] = true
		}
		if err := walkJSON(d); err != nil {
			return err
		}
	}
	_, err = d.Token()
	return err
}

func resolveScript(name string, scripts map[string]string, p string, active map[string]bool, depth, maxDepth int, edges *int, maxEdges int) (string, string, []Source, string) {
	chain := []Source{{Path: p, Symbol: name}}
	if depth >= maxDepth {
		return "", "", chain, "script_depth_limit"
	}
	if active[name] {
		return "", "", chain, "cyclic_reference"
	}
	text, ok := scripts[name]
	if !ok {
		return "", "", chain, "missing_reference"
	}
	active[name] = true
	defer delete(active, name)
	commands := []string{}
	kind := ""
	if strings.ContainsAny(text, "\r\n") {
		return "", "", chain, "unsupported_script"
	}
	for _, term := range strings.Split(text, "&&") {
		fields := strings.Fields(term)
		if len(fields) == 0 {
			return "", "", chain, "unsupported_script"
		}
		for _, f := range fields {
			if !literalToken.MatchString(f) && f != "-count=1" {
				return "", "", chain, "unsupported_script"
			}
		}
		if fields[0] == "npm" {
			ref := ""
			if len(fields) == 2 && fields[1] == "test" {
				ref = "test"
			}
			if len(fields) == 3 && fields[1] == "run" && scriptName.MatchString(fields[2]) {
				ref = fields[2]
			}
			if ref == "" {
				return "", "", chain, "unsupported_reference"
			}
			*edges++
			if *edges > maxEdges {
				return "", "", chain, "reference_limit"
			}
			cmd, k, ch, code := resolveScript(ref, scripts, p, active, depth+1, maxDepth, edges, maxEdges)
			chain = append(chain, ch...)
			if code != "" {
				return "", "", chain, code
			}
			commands = append(commands, cmd)
			if kind == "" {
				kind = k
			} else if kind != k {
				kind = "composite"
			}
			continue
		}
		k := leafKind(fields)
		if k == "" {
			return "", "", chain, "unsupported_script"
		}
		commands = append(commands, strings.Join(fields, " "))
		if kind == "" {
			kind = k
		} else if kind != k {
			kind = "composite"
		}
	}
	return strings.Join(commands, " && "), kind, chain, ""
}
func leafKind(f []string) string {
	start := 1
	kind := ""
	flags := map[string]bool{}
	switch f[0] {
	case "go":
		if len(f) < 2 || (f[1] != "test" && f[1] != "build") {
			return ""
		}
		kind = f[1]
		start = 2
		for _, s := range []string{"-race", "-v", "-short", "-count=1", "-trimpath"} {
			flags[s] = true
		}
	case "tsc":
		kind = "build"
		flags["--noEmit"] = true
	case "eslint":
		kind = "lint"
		flags["--no-fix"] = true
	case "vitest":
		if len(f) > 1 && f[1] == "run" {
			start = 2
		} else if len(f) > 1 && !strings.HasPrefix(f[1], "-") && !strings.HasPrefix(f[1], "./") {
			return ""
		}
		kind = "test"
		flags["--run"] = true
	case "jest":
		kind = "test"
		flags["--runInBand"] = true
	case "node":
		if len(f) < 2 || f[1] != "--test" {
			return ""
		}
		start = 2
		kind = "test"
	default:
		return ""
	}
	for _, s := range f[start:] {
		if strings.HasPrefix(s, "-") {
			if !flags[s] {
				return ""
			}
		} else if strings.HasPrefix(s, "/") || strings.Contains(s, ":") || strings.Contains(s, "@") || strings.Contains(s, "..") && !strings.HasPrefix(s, "./") {
			return ""
		}
	}
	return kind
}
func marker(base string) (string, string) {
	switch base {
	case "package-lock.json", "npm-shrinkwrap.json":
		return "npm", "npm"
	case ".eslintrc", ".eslintrc.json", "eslint.config.js", "eslint.config.mjs", "eslint.config.cjs":
		return "eslint", "npm"
	case ".golangci.yml", ".golangci.yaml":
		return "golangci-lint", "go"
	case "yarn.lock", "pnpm-lock.yaml", "pyproject.toml", "requirements.txt", "Cargo.toml", "pom.xml":
		return base, "unsupported"
	}
	return "", ""
}
func uniqueTools(a []Tool) []Tool {
	out := a[:0]
	for _, v := range a {
		if len(out) == 0 || out[len(out)-1].ID != v.ID {
			out = append(out, v)
		}
	}
	return out
}
func uniqueStrings(a []string) []string {
	out := a[:0]
	for _, v := range a {
		if len(out) == 0 || out[len(out)-1] != v {
			out = append(out, v)
		}
	}
	return out
}
