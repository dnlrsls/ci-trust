// Package audit composes offline observations into the existing evidence contract.
package audit

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/evidence"
	"github.com/dnlrsls/ci-trust/internal/workflows"
)

// Options declares target runner platforms, overriding the optional root declaration.
type Options struct{ Platforms []string }
type record = map[string]any

var platforms = map[string]bool{"linux": true, "windows": true, "macos": true}
var secret = regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|sk-[A-Za-z0-9_-]+|AKIA[A-Z0-9]{16})`)
var relative = regexp.MustCompile(`^[^/\\:]+(/[^/\\:]+)*$`)

func validPath(p string) bool {
	return relative.MatchString(p) && path.Clean(p) == p && p != "." && !strings.HasPrefix(p, "../") && !secret.MatchString(p)
}
func validateOptions(o Options) error {
	if len(o.Platforms) > 64 {
		return errors.New("platform option limit exceeded")
	}
	for _, p := range o.Platforms {
		if !platforms[p] {
			return errors.New("unsupported platform option")
		}
	}
	return nil
}

// Scan reads only through discovery's confined, bounded reader. Partial inputs
// remain report data; inability to open the root is an operational error.
func Scan(root string, o Options) ([]byte, error) {
	if err := validateOptions(o); err != nil {
		return nil, err
	}
	s := discovery.ReadSnapshot(root, discovery.Options{})
	for _, d := range s.Diagnostics {
		if d.Code == "ROOT_UNAVAILABLE" || d.Code == "UNSUPPORTED_HOST" || d.Code == "INVALID_OPTIONS" {
			return nil, errors.New("audit root unavailable")
		}
	}
	return Analyze(s, o)
}

type builder struct {
	r      record
	anchor discovery.Source
	check  string
	repo   record
}

func newBuilder(id string) *builder {
	r := record{"schema_version": "1", "snapshot": record{"id": id, "repository_id": nil, "commit_sha": nil}}
	for _, k := range []string{"inventory", "capabilities", "workflows", "evidence", "findings", "diagnostics", "limitations"} {
		r[k] = []any{}
	}
	return &builder{r: r, repo: record{"kind": "REPOSITORY", "subject": id}}
}
func (b *builder) add(k string, v any) { b.r[k] = append(b.r[k].([]any), v) }
func src(s discovery.Source) any {
	if !validPath(s.Path) || s.Line < 1 || s.Column < 1 {
		return nil
	}
	return record{"path": s.Path, "line": s.Line, "column": s.Column}
}
func (b *builder) diag(code string, s discovery.Source) {
	b.add("diagnostics", record{"id": evidence.StableID("diagnostic", code, s.Path, fmt.Sprint(s.Line, s.Column)), "code": code, "message": "Static analysis could not resolve this input; no execution or enforcement is inferred.", "source": src(s)})
}
func (b *builder) ev(key, state string, scope record, sources []discovery.Source, diagnostic bool) string {
	ss := []any{}
	seen := map[string]bool{}
	for _, s := range sources {
		if v := src(s); v != nil {
			key := fmt.Sprint(v)
			if !seen[key] {
				ss = append(ss, v)
				seen[key] = true
			}
		}
	}
	kind, basis := "CONFIGURATION", "SOURCE"
	if diagnostic || len(ss) == 0 {
		kind, basis, state = "DIAGNOSTIC", "UNRESOLVED", "UNKNOWN"
	}
	id := evidence.StableID("evidence", key)
	b.add("evidence", record{"id": id, "check_id": b.check, "state": state, "scope": scope, "provenance": record{"kind": kind, "adapter": "audit-v1", "basis": basis, "sources": ss}, "execution": nil, "cause": nil, "limitations": []string{"Static configuration only; historical execution and remote enforcement remain UNKNOWN."}})
	return id
}
func (b *builder) finding(rule, key, explanation, certainty string, scope record, refs []string, limits ...string) {
	b.add("findings", record{"id": evidence.StableID("finding", rule, key), "rule_id": rule, "scope": scope, "evidence_refs": refs, "explanation": explanation, "certainty": certainty, "certainty_rule": "Bounded offline literal configuration analysis; no run or remote policy evidence.", "limitations": append([]string{"No historical execution, global non-execution, or enforcement claim."}, limits...)})
}
func (b *builder) cap(key, dimension, status string, scope record, refs []string) {
	b.add("capabilities", record{"id": evidence.StableID("capability", key, dimension), "check_id": b.check, "dimension": dimension, "status": status, "scope": scope, "evidence_refs": refs, "limitations": []string{"Bounded generic intent only; no complete functional coverage or remote policy inference."}})
}

// Analyze is pure: all inputs are owned observations, never live paths or commands.
// Oversized/ambiguous direct snapshots are operational errors before composition.
func Analyze(s discovery.Snapshot, o Options) ([]byte, error) {
	if err := validateOptions(o); err != nil {
		return nil, err
	}
	if len(s.Documents)+len(s.Diagnostics)+len(s.Limitations) > 4096 {
		return nil, errors.New("snapshot record limit exceeded")
	}
	docs := append([]discovery.Document(nil), s.Documents...)
	total := 0
	seen := map[string]bool{}
	for _, d := range docs {
		if !validPath(d.Path) || len(d.Path) > 4096 || seen[d.Path] {
			return nil, errors.New("invalid or ambiguous snapshot path")
		}
		seen[d.Path] = true
		total += len(d.Content)
		if len(d.Content) > 1<<20 || total > 8<<20 {
			return nil, errors.New("snapshot byte limit exceeded")
		}
	}
	// Upstream analyzers sanitize diagnostics. Do not retain caller-authored prose.
	for _, d := range s.Diagnostics {
		if len(d.Code)+len(d.Path)+len(d.Message) > 16384 {
			return nil, errors.New("snapshot diagnostic limit exceeded")
		}
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	h := sha256.New()
	for _, d := range docs {
		fmt.Fprintf(h, "%d:%s%d:", len(d.Path), d.Path, len(d.Content))
		h.Write(d.Content)
	}
	id := "snapshot-" + hex.EncodeToString(h.Sum(nil))
	b := newBuilder(id)
	b.add("limitations", "Offline bounded configuration only. Historical execution, efficacy and required-check enforcement are UNKNOWN. Runner OS is not a cross-compilation target. Generic TEST intent is not unit, integration or E2E completeness.")
	inv := discovery.Analyze(s, discovery.Options{})
	graph := workflows.Analyze(s, inv, workflows.Options{})
	// The workflow adapter deliberately leaves matrix runner interpretation to
	// consumers. Discharge only that diagnostic with exact literal OS resolution.
	unresolved := graph.Diagnostics[:0]
	for _, d := range graph.Diagnostics {
		resolved := false
		if d.Code == "unresolved-runner" {
			for _, w := range graph.Workflows {
				for _, j := range w.Jobs {
					if j.Runner.Source == d.Source {
						_, resolved = runnerPlatforms(j)
					}
				}
			}
		}
		if !resolved {
			unresolved = append(unresolved, d)
		}
	}
	graph.Diagnostics = unresolved
	for _, d := range inv.Diagnostics {
		b.diag("DISCOVERY_UNRESOLVED", discovery.Source{Path: d.Path})
	}
	for _, d := range graph.Diagnostics {
		b.diag(d.Code, d.Source)
	}
	// V1 requires a positive source anchor for CHECK inventory. A bounded empty
	// report uses limitations/diagnostics rather than inventing a repository file.
	for _, w := range graph.Workflows {
		if src(w.Source) != nil {
			b.anchor = w.Source
			break
		}
	}
	if src(b.anchor) == nil {
		for _, d := range docs {
			if len(bytes.TrimSpace(d.Content)) > 0 {
				b.anchor = firstToken(d)
				break
			}
		}
	}
	if src(b.anchor) == nil {
		b.diag("NO_SOURCE_ANCHOR", discovery.Source{})
		return finish(b)
	}
	b.check = evidence.StableID("check", id, "offline-configuration")
	b.add("inventory", record{"id": b.check, "kind": "CHECK", "name": "Bounded offline validation configuration", "source": src(b.anchor), "evidence_refs": []string{}})
	unknown := b.ev("offline-policy", "UNKNOWN", b.repo, nil, true)
	for _, dimension := range []string{"EXECUTION", "ENFORCEMENT", "EFFICACY"} {
		b.cap("repository", dimension, "UNKNOWN", b.repo, []string{unknown})
	}
	b.finding("CIT_REQUIRED_CHECKS_UNKNOWN", "offline", "Required checks and actual enforcement remain UNKNOWN offline.", "UNKNOWN", b.repo, []string{unknown}, "No remote policy collection is performed.")
	declared, declSources, declOK := declaration(docs, o)
	declRef := b.ev("declaration", "CONFIGURED", b.repo, declSources, len(o.Platforms) > 0 || !declOK)
	if !declOK {
		b.diag("DECLARATION_UNKNOWN", declarationSource(docs))
		b.finding("CIT_DECLARATION_UNKNOWN", "declaration", "Platform declaration absent or unsupported; support scope remains UNKNOWN.", "UNKNOWN", b.repo, []string{declRef}, "README prose and names are not authoritative platform declarations.")
	}
	if len(o.Platforms) > 0 {
		b.add("limitations", "Target platforms supplied explicitly to audit.Options; DIAGNOSTIC provenance attributed to audit-v1, not a repository file.")
	}
	linked := map[string]bool{}
	closed := len(graph.Diagnostics) == 0 && len(inv.Diagnostics) == 0 && len(s.Diagnostics) == 0 && len(s.Limitations) == 0 && len(graph.Workflows) > 0
	coverage := map[string]bool{}
	proof := []discovery.Source{}
	events := map[string]bool{}
	for _, w := range graph.Workflows {
		wid := evidence.StableID("workflow", w.Path)
		wf := record{"id": wid, "source": src(w.Source), "events": []string{}, "jobs": []any{}}
		if src(w.Source) == nil {
			closed = false
			continue
		}
		eventNames := []string{}
		for _, e := range w.Events {
			events[e.Name] = true
			eventNames = append(eventNames, e.Name)
			if len(e.Filters) > 0 || e.State == workflows.Unknown {
				closed = false
			}
		}
		wf["events"] = eventNames
		if len(w.Events) == 0 {
			closed = false
		}
		for _, j := range w.Jobs {
			jid := evidence.StableID("job", w.Path, j.ID)
			scope := record{"kind": "JOB", "subject": jid}
			js := []discovery.Source{j.Source, j.Runner.Source, j.Matrix.Source}
			proof = append(proof, js...)
			for _, literal := range j.Matrix.Literals {
				keys := []string{}
				for k := range literal {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					proof = append(proof, literal[k].Source)
				}
			}
			je := b.ev(jid, "CONFIGURED", scope, js, false)
			jr := []string{je}
			// V1 graphs reject missing/cyclic needs. Preserve the analyzed graph as
			// source evidence/diagnostics instead of serializing a competing invalid DAG.
			needs := []string{}
			if len(graph.Diagnostics) == 0 {
				for _, n := range j.Needs {
					needs = append(needs, evidence.StableID("job", w.Path, n))
				}
			}
			wf["jobs"] = append(wf["jobs"].([]any), record{"id": jid, "needs": needs, "evidence_refs": jr})
			if len(j.Needs) > 0 && len(graph.Diagnostics) > 0 {
				b.add("limitations", "Job dependencies constrain eligibility; report-v1 compact jobs omit needs edges when graph validity is not established. Consult source-backed configuration evidence.")
			}
			runners, known := runnerPlatforms(j)
			if !known {
				closed = false
				b.finding("CIT_DYNAMIC_PATH", jid+"runner", "Runner or matrix path remains UNKNOWN.", "UNKNOWN", scope, jr, "Dynamic runners and matrices cannot establish platform absence.")
			}
			if j.Uses != "" {
				closed = false
			}
			observeEligibility(b, jid, j.Eligibility, scope, jr, &closed)
			if nonblocking(j.Settings["continue-on-error"]) {
				b.finding("CIT_NON_BLOCKING", jid, "Job is configured as potentially non-blocking.", "DIRECT", scope, jr, "Not a mandatory merge guarantee.")
			}
			for _, st := range j.Steps {
				sid := evidence.StableID("step", jid, st.ID, fmt.Sprint(st.Source.Line))
				sscope := record{"kind": "STEP", "subject": sid}
				sources := []discovery.Source{st.Source, st.RunValue.Source, st.Condition.Source, st.ContinueOnError.Source}
				kind := ""
				if safeStep(st) {
					kind = literalKind(st.Run)
				}
				for _, l := range st.Links {
					linked[l.CandidateID] = true
					for _, c := range inv.Candidates {
						if c.ID == l.CandidateID && c.State == "CONFIGURED" {
							kind = strings.ToUpper(c.Kind)
							for _, x := range l.Chain {
								sources = append(sources, locate(x, docs))
							}
						}
					}
				}
				se := b.ev(sid, "CONFIGURED", sscope, sources, false)
				sr := []string{se}
				observeEligibility(b, sid, st.Eligibility, sscope, sr, &closed)
				if nonblocking(st.ContinueOnError) {
					b.finding("CIT_NON_BLOCKING", sid, "Step is configured as potentially non-blocking.", "DIRECT", sscope, sr, "Not a mandatory merge guarantee.")
				}
				if strings.Contains(st.ContinueOnError.Raw, "${{") {
					closed = false
				}
				if st.Uses != "" || strings.TrimSpace(st.Run) != "" && kind == "" {
					closed = false
					b.finding("CIT_DYNAMIC_PATH", sid+"command", "Command/action internals are not a recognized literal validation path.", "UNKNOWN", sscope, sr, "Names, echo and unsupported wrappers establish no validation category.")
				}
				if kind != "" {
					b.cap(sid+"-"+kind, "CONFIGURATION", "PRESENT", sscope, sr)
					b.add("limitations", "Configured "+kind+" intent only within a source-backed step; no inferred category completeness.")
				}
				if kind == "TEST" && known {
					for _, p := range runners {
						coverage[p] = true
					}
					proof = append(proof, st.RunValue.Source)
				}
			}
		}
		b.add("workflows", wf)
	}
	for _, c := range inv.Candidates {
		if c.Origin != "SCRIPT" || linked[c.ID] {
			continue
		}
		source := locate(c.Source, docs)
		if src(source) == nil {
			b.diag("UNLOCATED_SCRIPT", c.Source)
			continue
		}
		e := b.ev("candidate-"+c.ID, c.State, b.repo, []discovery.Source{source}, c.State != "CONFIGURED")
		b.finding("CIT_COMMAND_UNLINKED", c.ID, "Available npm definition has no understood workflow link.", "DIRECT", b.repo, []string{e}, "Unlinked does not prove never executed elsewhere; unsupported invocations may remain unresolved.")
	}
	if declOK && closed && len(events) > 0 {
		pe := b.ev("closed-paths", "CONFIGURED", b.repo, proof, false)
		names := []string{}
		for e := range events {
			names = append(names, e)
		}
		sort.Strings(names)
		for _, p := range declared {
			if !coverage[p] {
				b.finding("CIT_PLATFORM_CONFIGURED_GAP", p, "Declared "+p+" lacks a configured generic TEST runner path within supplied literal workflows/events: "+strings.Join(names, ", ")+".", "DERIVED", b.repo, []string{declRef, pe}, "Closed supplied configuration scope only; not OS correctness or global execution absence. No ABSENT capability is asserted.")
			}
		}
	} else if declOK && !closed {
		b.add("limitations", "Platform absence cannot be established: reader omissions, unresolved commands, event filters, conditions, dependencies, runners or matrices leave the configured scope open.")
	}
	return finish(b)
}
func finish(b *builder) ([]byte, error) {
	// Repeated upstream diagnostics can describe the same bounded unknown.
	for _, k := range []string{"diagnostics"} {
		seen := map[string]bool{}
		out := []any{}
		for _, v := range b.r[k].([]any) {
			id := v.(record)["id"].(string)
			if !seen[id] {
				seen[id] = true
				out = append(out, v)
			}
		}
		b.r[k] = out
	}
	// Deduplicate authored limitations before canonical encoding.
	ls := b.r["limitations"].([]any)
	seen := map[string]bool{}
	out := []any{}
	for _, v := range ls {
		t := v.(string)
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	b.r["limitations"] = out
	raw, err := json.Marshal(b.r)
	if err != nil {
		return nil, errors.New("cannot assemble report")
	}
	sensitive := secret.FindAllString(string(raw), -1)
	redacted, ds := evidence.Redact(raw, sensitive)
	if len(ds) > 0 {
		return nil, errors.New("cannot redact report")
	}
	encoded, ds := evidence.Encode(redacted)
	if len(ds) > 0 {
		return nil, fmt.Errorf("report contract rejected: %v", ds)
	}
	return encoded, nil
}
func firstToken(d discovery.Document) discovery.Source {
	i := 0
	for i < len(d.Content) && strings.ContainsRune(" \t\r\n", rune(d.Content[i])) {
		i++
	}
	if i == len(d.Content) {
		return discovery.Source{}
	}
	return position(d.Path, d.Content, i)
}
func position(p string, b []byte, i int) discovery.Source {
	line, col := 1, 1
	for _, c := range b[:i] {
		if c == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return discovery.Source{Path: p, Line: line, Column: col}
}

// jsonLocations walks actual decoder tokens, retaining exact key anchors; no
// textual search can confuse a script name with a value elsewhere in the file.
func jsonLocations(d discovery.Document) (map[string]discovery.Source, error) {
	dec := json.NewDecoder(bytes.NewReader(d.Content))
	locs := map[string]discovery.Source{}
	var walk func(string) error
	walk = func(prefix string) error {
		t, err := dec.Token()
		if err != nil {
			return err
		}
		if t == json.Delim('{') {
			seen := map[string]bool{}
			for dec.More() {
				start := int(dec.InputOffset())
				for start < len(d.Content) && (strings.ContainsRune(" \t\r\n,", rune(d.Content[start]))) {
					start++
				}
				k, e := dec.Token()
				if e != nil {
					return e
				}
				key, ok := k.(string)
				if !ok || seen[key] {
					return errors.New("duplicate JSON key")
				}
				seen[key] = true
				locs[prefix+"/"+key] = position(d.Path, d.Content, start)
				if e = walk(prefix + "/" + key); e != nil {
					return e
				}
			}
			_, err = dec.Token()
		} else if t == json.Delim('[') {
			for dec.More() {
				if e := walk(prefix + "/[]"); e != nil {
					return e
				}
			}
			_, err = dec.Token()
		}
		return err
	}
	if err := walk(""); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON")
	}
	return locs, nil
}
func locate(s discovery.Source, docs []discovery.Document) discovery.Source {
	if src(s) != nil {
		return s
	}
	for _, d := range docs {
		if d.Path == s.Path && s.Symbol != "" {
			locs, err := jsonLocations(d)
			if err == nil {
				return locs["/scripts/"+s.Symbol]
			}
		}
	}
	return discovery.Source{}
}
func declarationSource(docs []discovery.Document) discovery.Source {
	for _, d := range docs {
		if d.Path == ".ci-trust.json" {
			return firstToken(d)
		}
	}
	return discovery.Source{}
}
func declaration(docs []discovery.Document, o Options) ([]string, []discovery.Source, bool) {
	if len(o.Platforms) > 0 {
		return unique(o.Platforms), nil, true
	}
	for _, d := range docs {
		if d.Path != ".ci-trust.json" {
			continue
		}
		locs, err := jsonLocations(d)
		if err != nil {
			return nil, nil, false
		}
		var v struct {
			Version   int      `json:"version"`
			Platforms []string `json:"platforms"`
		}
		dec := json.NewDecoder(bytes.NewReader(d.Content))
		dec.DisallowUnknownFields()
		if dec.Decode(&v) != nil || v.Version != 1 || len(v.Platforms) == 0 || len(v.Platforms) > 3 {
			return nil, nil, false
		}
		seen := map[string]bool{}
		for _, p := range v.Platforms {
			if !platforms[p] || seen[p] {
				return nil, nil, false
			}
			seen[p] = true
		}
		return unique(v.Platforms), []discovery.Source{locs["/version"], locs["/platforms"]}, true
	}
	return nil, nil, false
}
func unique(xs []string) []string {
	m := map[string]bool{}
	out := []string{}
	for _, x := range xs {
		if !m[x] {
			m[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
func nonblocking(v workflows.Value) bool { return v.Raw == "true" || strings.Contains(v.Raw, "${{") }
func observeEligibility(b *builder, key string, elig map[string]workflows.State, scope record, refs []string, closed *bool) {
	names := []string{}
	for n := range elig {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		state := elig[n]
		if state == workflows.Ineligible {
			*closed = false
			b.finding("CIT_STATIC_OMISSION", key+n, "Known static exclusion for event "+n+"; not an observed historical skip.", "DIRECT", scope, refs, "Static event eligibility only.")
		} else if state != workflows.Eligible {
			*closed = false
			if state == workflows.Unknown {
				b.finding("CIT_DYNAMIC_PATH", key+n, "Unresolved condition or dependency path for event "+n+".", "UNKNOWN", scope, refs, "Expression evaluation and historical outcomes remain unsupported.")
			}
		}
	}
}

var matrixOS = regexp.MustCompile(`^\$\{\{\s*matrix\.os\s*\}\}$`)
var runnerLabel = regexp.MustCompile(`^(ubuntu|windows|macos)-(latest|[0-9]+(?:\.[0-9]+)?)(?:-(?:arm|arm64|large|xlarge))?$`)

func runnerPlatforms(j workflows.Job) ([]string, bool) {
	if j.Matrix.State == workflows.Unknown {
		return nil, false
	}
	raw := j.Runner.Raw
	labels := []string{}
	if matrixOS.MatchString(raw) {
		for _, c := range j.Matrix.Combinations {
			x, ok := c["os"]
			if !ok {
				return nil, false
			}
			labels = append(labels, x)
		}
	} else {
		if len(j.Runner.Items) > 0 || len(j.Runner.Fields) > 0 {
			return nil, false
		}
		labels = append(labels, raw)
	}
	out := []string{}
	for _, l := range labels {
		m := runnerLabel.FindStringSubmatch(l)
		if m == nil {
			return nil, false
		}
		p := m[1]
		if p == "ubuntu" {
			p = "linux"
		}
		out = append(out, p)
	}
	return unique(out), len(out) > 0
}
func safeStep(s workflows.Step) bool {
	wd := s.WorkingDirectory.Raw
	return (s.Shell.Raw == "" || s.Shell.Raw == "bash" || s.Shell.Raw == "sh") && (wd == "" || wd == "." || validPath(wd))
}

var goLiteral = regexp.MustCompile(`^go (test|build)(?: (?:\./[A-Za-z0-9_./-]+|\./\.\.\.|-race|-v|-short|-count=1|-trimpath))+$`)

func literalKind(command string) string {
	if command != strings.TrimSpace(command) || strings.ContainsAny(command, "\n\r\t") {
		return ""
	}
	if m := goLiteral.FindStringSubmatch(command); m != nil {
		return strings.ToUpper(m[1])
	}
	if command == "node --test" || command == "jest" || command == "jest --runInBand" || command == "vitest run" {
		return "TEST"
	}
	return ""
}
