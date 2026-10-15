package workflows

import (
	"bytes"
	"io"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/dnlrsls/ci-trust/internal/discovery"
	"go.yaml.in/yaml/v3"
)

func limits(o Options) (Options, bool) {
	for _, p := range []struct {
		v        *int
		def, max int
	}{
		{&o.MaxDocuments, 64, 256}, {&o.MaxBytes, 1 << 20, 8 << 20}, {&o.MaxNodes, 32768, 131072},
		{&o.MaxDepth, 32, 64}, {&o.MaxJobs, 256, 1024}, {&o.MaxSteps, 2048, 8192}, {&o.MaxMatrix, 256, 4096},
	} {
		if *p.v < 0 || *p.v > p.max {
			return o, false
		}
		if *p.v == 0 {
			*p.v = p.def
		}
	}
	return o, true
}

type parser struct {
	result                                                                         Result
	options                                                                        Options
	file                                                                           string
	nodes, jobs, steps, matrixCells, linkCount, valueBytes, matrixBytes, linkBytes int
}

func (p *parser) source(n *yaml.Node) Source {
	s := Source{Path: p.file}
	if n != nil {
		s.Line = n.Line
		s.Column = n.Column
	}
	return s
}
func (p *parser) diag(code string, n *yaml.Node) { p.at(code, p.source(n)) }
func (p *parser) at(code string, source Source) {
	p.result.State = Unknown
	if len(p.result.Diagnostics) >= 4096 {
		return
	}
	p.result.Diagnostics = append(p.result.Diagnostics, Diagnostic{Code: code, Message: code, Source: source, State: Unknown})
}
func exactWorkflow(s string) bool {
	return path.Clean(s) == s && strings.HasPrefix(s, ".github/workflows/") && path.Dir(s) == ".github/workflows" && (path.Ext(s) == ".yml" || path.Ext(s) == ".yaml") && !strings.Contains(s, "\\")
}

// Analyze accepts owned snapshot bytes and a public discovery inventory only.
// Limits apply even when callers bypass discovery. No filesystem or process APIs are used.
func Analyze(snapshot discovery.Snapshot, inventory discovery.Result, options Options) Result {
	p := parser{result: Result{State: Configured, Historical: Unknown, Enforcement: Unknown, Limitations: []string{
		"Static eligibility is not historical execution or a GitHub scheduler simulation.",
		"Required checks and remote protection enforcement are UNKNOWN offline.",
		"Unsupported expressions, action internals and reusable workflows are not interpreted.",
	}}}
	var ok bool
	p.options, ok = limits(options)
	if !ok {
		p.diag("invalid-options", nil)
		return p.result
	}
	// Bound all supplied documents, including ignored paths, before sorting or decoding.
	if len(snapshot.Documents) > p.options.MaxDocuments {
		p.diag("document-limit", nil)
		return p.result
	}
	total := 0
	for _, d := range snapshot.Documents {
		if len(d.Path) > 4096 || len(d.Content) > p.options.MaxBytes-total {
			p.diag("byte-limit", nil)
			return p.result
		}
		total += len(d.Content)
	}
	if len(snapshot.Diagnostics) > 0 || len(snapshot.Limitations) > 0 {
		p.diag("snapshot-omissions", nil)
	}
	docs := append([]discovery.Document(nil), snapshot.Documents...)
	sort.SliceStable(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	counts := map[string]int{}
	for _, d := range docs {
		counts[d.Path]++
	}
	seen := map[string]bool{}
	for _, d := range docs {
		if !exactWorkflow(d.Path) {
			continue
		}
		p.file = d.Path
		if counts[d.Path] > 1 {
			if !seen[d.Path] {
				p.diag("duplicate-document", nil)
			}
			seen[d.Path] = true
			continue
		}
		seen[d.Path] = true
		if !p.admit(d.Content) {
			p.diag("yaml-admission-limit", nil)
			continue
		}
		dec := yaml.NewDecoder(bytes.NewReader(d.Content))
		var root, extra yaml.Node
		if err := dec.Decode(&root); err != nil {
			line := regexp.MustCompile(`line ([0-9]+)`).FindStringSubmatch(err.Error())
			if len(line) == 2 {
				root.Line, _ = strconv.Atoi(line[1])
				root.Column = 1
			}
			if root.Line == 0 {
				root.Line = 1
				root.Column = 1
			}
			p.diag("malformed-yaml", &root)
			continue
		}
		if err := dec.Decode(&extra); err != io.EOF {
			p.diag("multiple-documents", &extra)
			continue
		}
		if !p.validate(&root, 0) {
			continue
		}
		if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
			p.diag("workflow-mapping-required", &root)
			continue
		}
		w := p.workflow(root.Content[0], inventory)
		p.result.Workflows = append(p.result.Workflows, w)
	}
	return p.result
}
func (p *parser) validate(n *yaml.Node, depth int) bool {
	p.nodes++
	if depth > p.options.MaxDepth || p.nodes > p.options.MaxNodes {
		p.diag("yaml-limit", n)
		return false
	}
	if n.Kind == yaml.AliasNode || n.Anchor != "" {
		p.diag("yaml-alias", n)
		return false
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode || k.Value == "<<" || seen[k.Value] {
				p.diag("duplicate-or-merge-key", k)
				return false
			}
			seen[k.Value] = true
		}
	}
	for _, c := range n.Content {
		if !p.validate(c, depth+1) {
			return false
		}
	}
	return true
}
func field(n *yaml.Node, key string) *yaml.Node {
	if n != nil && n.Kind == yaml.MappingNode {
		for i := 0; i < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				return n.Content[i+1]
			}
		}
	}
	return nil
}
func text(n *yaml.Node) string {
	if n == nil {
		return ""
	}
	if n.Kind == yaml.ScalarNode {
		return n.Value
	}
	var b strings.Builder
	var walk func(*yaml.Node)
	walk = func(x *yaml.Node) {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(x.Value)
		for _, c := range x.Content {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
func dynamic(s string) bool { return strings.Contains(s, "${{") }
func (p *parser) value(n *yaml.Node, scope string) Value {
	v := Value{Source: p.source(n), Scope: scope, State: Unknown}
	if p.valueBytes >= 8<<20 {
		p.diag("value-output-limit", n)
		return v
	}
	v.Raw = text(n)
	p.valueBytes += len(v.Raw)
	if p.valueBytes > 8<<20 {
		p.diag("value-output-limit", n)
		v.Raw = ""
		return v
	}
	if n == nil {
		return v
	}
	v.Tag = n.Tag
	if !dynamic(v.Raw) {
		v.State = Configured
	}
	if n.Kind == yaml.MappingNode {
		v.Fields = map[string]Value{}
		for i := 0; i < len(n.Content); i += 2 {
			v.Fields[n.Content[i].Value] = p.value(n.Content[i+1], scope)
		}
	}
	if n.Kind == yaml.SequenceNode {
		for _, c := range n.Content {
			v.Items = append(v.Items, p.value(c, scope))
		}
	}
	return v
}
func stringsOf(n *yaml.Node) []string {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.ScalarNode {
		return []string{n.Value}
	}
	if n.Kind != yaml.SequenceNode {
		return nil
	}
	var out []string
	for _, v := range n.Content {
		if v.Kind != yaml.ScalarNode {
			return nil
		}
		out = append(out, v.Value)
	}
	return out
}
func (p *parser) settings(n *yaml.Node, scope string, parent map[string]Value) map[string]Value {
	out := map[string]Value{}
	for _, k := range []string{"permissions", "timeout-minutes", "concurrency", "cancel-in-progress", "continue-on-error", "fail-fast"} {
		v := p.value(field(n, k), scope)
		if field(n, k) == nil {
			if x, ok := parent[k]; ok && k == "permissions" && x.Source.Line > 0 {
				v = x
				v.Inherited = true
			}
		}
		out[k] = v
		if field(n, k) != nil && v.State == Unknown {
			p.diag("unresolved-setting", field(n, k))
		}
	}
	if c := field(n, "concurrency"); c != nil && c.Kind == yaml.MappingNode {
		out["cancel-in-progress"] = p.value(field(c, "cancel-in-progress"), scope)
	}
	return out
}
func (p *parser) workflow(n *yaml.Node, inventory discovery.Result) Workflow {
	w := Workflow{Path: p.file, Name: text(field(n, "name")), Source: p.source(n), Settings: p.settings(n, "workflow", nil)}
	on := field(n, "on")
	if on == nil {
		p.diag("missing-trigger", n)
	} else if on.Kind == yaml.MappingNode {
		for i := 0; i < len(on.Content); i += 2 {
			k, v := on.Content[i], on.Content[i+1]
			e := Event{Name: k.Value, Source: p.source(k), Filters: map[string]Value{}, State: Configured}
			if v.Kind == yaml.MappingNode {
				for j := 0; j < len(v.Content); j += 2 {
					e.Filters[v.Content[j].Value] = p.value(v.Content[j+1], "event")
					e.State = Conditional
				}
			} else if v.Tag != "!!null" {
				e.State = Unknown
				p.diag("unresolved-event", v)
			}
			w.Events = append(w.Events, e)
		}
	} else {
		names := stringsOf(on)
		if names == nil {
			p.diag("unresolved-event", on)
		}
		for _, name := range names {
			w.Events = append(w.Events, Event{Name: name, Source: p.source(on), State: Configured})
		}
	}
	if len(w.Events) > 64 {
		p.diag("event-limit", on)
		w.Events = nil
		return w
	}
	jobs := field(n, "jobs")
	if jobs == nil || jobs.Kind != yaml.MappingNode {
		p.diag("jobs-mapping-required", n)
		return w
	}
	for i := 0; i < len(jobs.Content); i += 2 {
		p.jobs++
		if p.jobs > p.options.MaxJobs {
			p.diag("job-limit", jobs)
			break
		}
		k, v := jobs.Content[i], jobs.Content[i+1]
		j := Job{ID: k.Value, Name: text(field(v, "name")), Uses: text(field(v, "uses")), Source: p.source(k), Needs: stringsOf(field(v, "needs")), Condition: p.value(field(v, "if"), "job"), Runner: p.value(field(v, "runs-on"), "job"), Settings: p.settings(v, "job", w.Settings), State: Configured, Historical: Unknown, Enforcement: Unknown, Eligibility: map[string]State{}}
		if v.Kind != yaml.MappingNode {
			p.diag("job-mapping-required", v)
			j.State = Unknown
		}
		if nn := field(v, "needs"); nn != nil {
			j.NeedSources = append(j.NeedSources, p.source(nn))
			if j.Needs == nil || dynamic(text(nn)) {
				p.diag("unresolved-needs", nn)
				j.State = Unknown
			}
		}
		if j.Uses != "" {
			p.diag("reusable-workflow-unresolved", field(v, "uses"))
			j.State = Unknown
		}
		if j.Runner.State == Unknown && j.Uses == "" {
			p.diag("unresolved-runner", field(v, "runs-on"))
		}
		strategy := field(v, "strategy")
		j.Settings["fail-fast"] = p.value(field(strategy, "fail-fast"), "job.strategy")
		if ff := field(strategy, "fail-fast"); ff != nil && j.Settings["fail-fast"].State == Unknown {
			p.diag("unresolved-setting", ff)
		}
		j.Matrix = p.matrix(field(strategy, "matrix"))
		for _, e := range w.Events {
			j.Eligibility[e.Name] = conditionValue(j.Condition, e.Name)
			if e.State == Conditional {
				j.Eligibility[e.Name] = combine(j.Eligibility[e.Name], Conditional)
			}
			if e.State == Unknown || j.State == Unknown {
				j.Eligibility[e.Name] = combine(j.Eligibility[e.Name], Unknown)
			}
		}
		steps := field(v, "steps")
		if steps != nil && steps.Kind != yaml.SequenceNode {
			p.diag("steps-sequence-required", steps)
		} else if steps != nil {
			for _, sn := range steps.Content {
				p.steps++
				if p.steps > p.options.MaxSteps {
					p.diag("step-limit", sn)
					break
				}
				s := Step{ID: text(field(sn, "id")), Name: text(field(sn, "name")), Uses: text(field(sn, "uses")), Run: text(field(sn, "run")), Source: p.source(sn), Condition: p.value(field(sn, "if"), "step"), Shell: p.defaultValue(sn, v, n, "shell"), WorkingDirectory: p.defaultValue(sn, v, n, "working-directory"), ContinueOnError: p.value(field(sn, "continue-on-error"), "step"), Historical: Unknown, Eligibility: map[string]State{}}
				for _, e := range w.Events {
					s.Eligibility[e.Name] = combine(j.Eligibility[e.Name], conditionValue(s.Condition, e.Name))
				}
				s.RunValue = p.value(field(sn, "run"), "step")
				s.UsesValue = p.value(field(sn, "uses"), "step")
				if sn.Kind != yaml.MappingNode {
					p.diag("step-mapping-required", sn)
				}
				if s.Uses != "" {
					p.diag("action-internals-unresolved", field(sn, "uses"))
				}
				if s.Shell.Raw != "" && s.Shell.Raw != "bash" && s.Shell.Raw != "sh" {
					p.at("unsupported-shell", s.Shell.Source)
				} else {
					s.Links = p.links(s, inventory)
				}
				if s.Condition.Raw != "" {
					for _, e := range w.Events {
						if conditionValue(s.Condition, e.Name) == Unknown {
							p.diag("unresolved-condition", field(sn, "if"))
							break
						}
					}
				}
				j.Steps = append(j.Steps, s)
			}
		}
		for _, e := range w.Events {
			if conditionValue(j.Condition, e.Name) == Unknown {
				p.diag("unresolved-condition", field(v, "if"))
				break
			}
		}
		w.Jobs = append(w.Jobs, j)
	}
	// Resolve dependency eligibility per event with cycle detection; never infer run results.
	indices := map[string]int{}
	for i, j := range w.Jobs {
		indices[j.ID] = i
	}
	for _, e := range w.Events {
		marks := make([]int, len(w.Jobs))
		var visit func(int) State
		visit = func(i int) State {
			j := &w.Jobs[i]
			if marks[i] == 1 {
				p.result.State = Unknown
				if len(p.result.Diagnostics) < 4096 {
					p.result.Diagnostics = append(p.result.Diagnostics, Diagnostic{Code: "needs-cycle", Message: "needs-cycle", Source: j.Source, State: Unknown})
				}
				return Unknown
			}
			if marks[i] == 2 {
				return j.Eligibility[e.Name]
			}
			marks[i] = 1
			s := j.Eligibility[e.Name]
			for _, need := range j.Needs {
				idx, ok := indices[need]
				if !ok {
					p.result.State = Unknown
					src := j.Source
					if len(j.NeedSources) > 0 {
						src = j.NeedSources[0]
					}
					if len(p.result.Diagnostics) < 4096 {
						p.result.Diagnostics = append(p.result.Diagnostics, Diagnostic{Code: "missing-needs-target", Message: "missing-needs-target", Source: src, State: Unknown})
					}
					s = combine(s, Unknown)
					continue
				}
				d := visit(idx)
				if d == Eligible {
					d = Conditional
				}
				s = combine(s, d)
			}
			marks[i] = 2
			j.Eligibility[e.Name] = s
			for k := range j.Steps {
				j.Steps[k].Eligibility[e.Name] = combine(s, conditionValue(j.Steps[k].Condition, e.Name))
			}
			return s
		}
		for i := range w.Jobs {
			visit(i)
		}
	}
	return w
}
func (p *parser) defaultValue(step, job, workflow *yaml.Node, key string) Value {
	if n := field(step, key); n != nil {
		return p.value(n, "step")
	}
	for _, x := range []struct {
		n     *yaml.Node
		scope string
	}{{job, "job.defaults.run"}, {workflow, "workflow.defaults.run"}} {
		if n := field(field(field(x.n, "defaults"), "run"), key); n != nil {
			v := p.value(n, x.scope)
			v.Inherited = true
			return v
		}
	}
	return p.value(nil, "default")
}

var eventCondition = regexp.MustCompile(`^github\.event_name\s*(==|!=)\s*(['"])(push|pull_request|merge_group)(['"])$`)

func conditionValue(v Value, event string) State {
	if v.Source.Line == 0 {
		return Eligible
	}
	if (v.Tag != "!!str" && v.Tag != "!!bool") || v.Raw == "" {
		return Unknown
	}
	return condition(v.Raw, event)
}
func condition(raw, event string) State {
	s := strings.TrimSpace(raw)
	if strings.HasPrefix(s, "${{") && strings.HasSuffix(s, "}}") {
		s = strings.TrimSpace(s[3 : len(s)-2])
	}
	switch s {
	case "", "true":
		return Eligible
	case "false":
		return Ineligible
	}
	m := eventCondition.FindStringSubmatch(s)
	if m == nil || m[2] != m[4] {
		return Unknown
	}
	yes := event == m[3]
	if m[1] == "!=" {
		yes = !yes
	}
	if yes {
		return Eligible
	}
	return Ineligible
}
func combine(a, b State) State {
	if a == Ineligible || b == Ineligible {
		return Ineligible
	}
	if a == Unknown || b == Unknown {
		return Unknown
	}
	if a == Conditional || b == Conditional {
		return Conditional
	}
	return Eligible
}

func (p *parser) links(s Step, inventory discovery.Result) []Link {
	// Bound caller-supplied inventory as well: never perform an unbounded cross-product.
	if len(inventory.Candidates) > 8192 || len(inventory.Roots) > 4096 {
		p.diag("inventory-limit", nil)
		return nil
	}
	if (s.RunValue.State == Unknown && s.RunValue.Source.Line > 0) || (s.WorkingDirectory.State == Unknown && s.WorkingDirectory.Source.Line > 0) || (s.Shell.State == Unknown && s.Shell.Source.Line > 0) || s.Run == "" || s.Uses != "" || strings.ContainsAny(strings.TrimSpace(s.Run), "\r\n") || dynamic(s.Run) || dynamic(s.WorkingDirectory.Raw) {
		return nil
	}
	words := strings.Fields(s.Run)
	if len(words) < 2 || words[0] != "npm" {
		return nil
	}
	name := ""
	if len(words) == 2 && words[1] == "test" {
		name = "test"
	}
	if len(words) == 3 && words[1] == "run" {
		name = words[2]
	}
	if name == "" || !regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`).MatchString(name) {
		return nil
	}
	wd := s.WorkingDirectory.Raw
	if wd == "" {
		wd = "."
	}
	if strings.ContainsAny(wd, "\\\n\r") || path.IsAbs(wd) || path.Clean(wd) == ".." || strings.HasPrefix(path.Clean(wd), "../") {
		return nil
	}
	wd = path.Clean(wd)
	roots := map[string]bool{}
	for _, r := range inventory.Roots {
		if len(r.Path) > 4096 || len(r.ID) > 4096 {
			p.diag("inventory-limit", nil)
			return nil
		}
		if path.Clean(r.Path) == wd {
			roots[r.ID] = true
		}
	}
	size := 0
	for _, c := range inventory.Candidates {
		for _, src := range c.Chain {
			size += len(src.Path) + len(src.Symbol)
		}
		if len(c.Chain) > 64 {
			p.diag("inventory-limit", nil)
			return nil
		}
		size += len(c.Command) + len(c.ID) + len(c.Root) + len(c.Source.Path) + len(c.Source.Symbol)
		if size > 1<<20 {
			p.diag("inventory-limit", nil)
			return nil
		}
	}
	var links []Link
	for _, c := range inventory.Candidates {
		if roots[c.Root] && c.Source.Symbol == name && path.Clean(c.Source.Path) == path.Join(wd, "package.json") && c.State == "CONFIGURED" {
			chain := []Source{s.RunValue.Source}
			chain = append(chain, c.Source)
			if len(c.Chain) <= 64 {
				for _, src := range c.Chain {
					if src != c.Source {
						chain = append(chain, src)
					}
				}
			}
			p.linkBytes += len(c.Command)
			for _, src := range chain {
				p.linkBytes += len(src.Path) + len(src.Symbol)
			}
			if p.linkBytes > 8<<20 {
				p.diag("link-byte-limit", nil)
				return nil
			}
			if p.linkCount+len(links) >= 4096 {
				p.diag("link-output-limit", nil)
				return nil
			}
			links = append(links, Link{CandidateID: c.ID, Root: c.Root, Command: c.Command, Chain: chain, State: Configured})
		}
	}
	if len(links) == 0 {
		p.at("npm-definition-unresolved", s.RunValue.Source)
	}
	p.linkCount += len(links)
	return links
}

func (p *parser) matrix(n *yaml.Node) Matrix {
	out := Matrix{Source: p.source(n), State: Configured}
	literals := map[string]Value{}
	if n == nil {
		out.Combinations = []map[string]string{{}}
		out.Literals = []map[string]Value{{}}
		return out
	}
	fail := func(code string) Matrix { p.diag(code, n); return Matrix{Source: p.source(n), State: Unknown} }
	if n.Kind != yaml.MappingNode || dynamic(text(n)) {
		return fail("unresolved-matrix")
	}
	combos := []map[string]string{{}}
	axes := map[string]bool{}
	for i := 0; i < len(n.Content); i += 2 {
		k, v := n.Content[i].Value, n.Content[i+1]
		if k == "include" || k == "exclude" {
			continue
		}
		axes[k] = true
		if len(axes) > 32 {
			return fail("matrix-axis-limit")
		}
		if v.Kind != yaml.SequenceNode || len(v.Content) == 0 {
			return fail("invalid-matrix")
		}
		if len(v.Content) > p.options.MaxMatrix/len(combos) {
			return fail("matrix-limit")
		}
		var next []map[string]string
		for _, base := range combos {
			for _, val := range v.Content {
				if !literal(val) {
					return fail("invalid-matrix")
				}
				c := clone(base)
				token := literalToken(val)
				literals[k+"\x00"+token] = p.value(val, "matrix")
				c[k] = token
				next = append(next, c)
			}
		}
		combos = next
	}
	entries := func(key string) ([]map[string]string, bool) {
		v := field(n, key)
		if v == nil {
			return nil, true
		}
		if v.Kind != yaml.SequenceNode || len(v.Content) > p.options.MaxMatrix {
			return nil, false
		}
		var entries []map[string]string
		for _, e := range v.Content {
			if e.Kind != yaml.MappingNode || len(e.Content) > 64 {
				return nil, false
			}
			m := map[string]string{}
			for i := 0; i < len(e.Content); i += 2 {
				if !literal(e.Content[i+1]) {
					return nil, false
				}
				key := e.Content[i].Value
				val := e.Content[i+1]
				token := literalToken(val)
				literals[key+"\x00"+token] = p.value(val, "matrix")
				m[key] = token
			}
			entries = append(entries, m)
		}
		return entries, true
	}
	excludes, ok := entries("exclude")
	if !ok {
		return fail("invalid-matrix")
	}
	includes, ok := entries("include")
	if !ok {
		return fail("invalid-matrix")
	}
	var original []map[string]string
	for _, c := range combos {
		excluded := false
		for _, ex := range excludes {
			match := true
			for k, v := range ex {
				if got, ok := c[k]; !ok || got != v {
					match = false
				}
			}
			if match {
				excluded = true
			}
		}
		if !excluded {
			original = append(original, c)
		}
	}
	if len(axes) == 0 {
		if len(includes) == 0 {
			return fail("invalid-matrix")
		}
		original = nil
	}
	combos = nil
	for _, c := range original {
		combos = append(combos, clone(c))
	}
	// Include may augment original combinations only, never combinations added by include.
	for _, inc := range includes {
		matched := false
		for i, base := range original {
			compatible := true
			for k, v := range inc {
				if axes[k] && base[k] != v {
					compatible = false
				}
			}
			if compatible {
				matched = true
				for k, v := range inc {
					combos[i][k] = v
				}
			}
		}
		if !matched {
			if len(combos) >= p.options.MaxMatrix {
				return fail("matrix-limit")
			}
			combos = append(combos, clone(inc))
		}
	}
	for _, c := range combos {
		p.matrixCells += len(c)
		if p.matrixCells > 65536 {
			return fail("matrix-output-limit")
		}
		values := map[string]Value{}
		for k, v := range c {
			values[k] = literals[k+"\x00"+v]
			if values[k].State == Unknown {
				return fail("matrix-output-limit")
			}
			p.matrixBytes += len(k) + len(values[k].Raw)
			if p.matrixBytes > 8<<20 {
				return fail("matrix-byte-limit")
			}
			c[k] = values[k].Raw
		}
		out.Literals = append(out.Literals, values)
	}
	out.Combinations = combos
	return out
}
func literal(n *yaml.Node) bool {
	if n.Kind != yaml.ScalarNode || dynamic(n.Value) {
		return false
	}
	switch n.Tag {
	case "!!str", "!!bool":
		return true
	case "!!int", "!!float":
		var f float64
		return n.Decode(&f) == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	}
	return false
}
func literalToken(n *yaml.Node) string {
	if n.Tag == "!!int" || n.Tag == "!!float" {
		var f float64
		_ = n.Decode(&f)
		return "number\x00" + strconv.FormatFloat(f, 'g', -1, 64)
	}
	return n.Tag + "\x00" + n.Value
}
func clone(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}
