package bootstrap

import (
	"encoding/json"
	"errors"
	"path"
	"sort"
	"strings"

	"github.com/dnlrsls/ci-trust/internal/audit"
	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/evidence"
	"github.com/dnlrsls/ci-trust/internal/workflows"
)

const destination = ".github/workflows/ci-trust.yml"

type sourceReport struct {
	Findings []struct {
		ID   string   `json:"id"`
		Rule string   `json:"rule_id"`
		Refs []string `json:"evidence_refs"`
	} `json:"findings"`
	Evidence []struct {
		ID    string `json:"id"`
		State string `json:"state"`
	} `json:"evidence"`
}
type command struct{ text, root string }

// Scan collects only through the confined discovery boundary. It never applies a plan.
func Scan(root string, o Options) (Plan, error) {
	// Validate options before any filesystem collection, using the authoritative analyzer.
	if _, err := audit.Analyze(discovery.Snapshot{}, audit.Options{Platforms: o.Platforms}); err != nil {
		return Plan{}, err
	}
	s := discovery.ReadSnapshot(root, discovery.Options{})
	for _, d := range s.Diagnostics {
		if d.Code == "ROOT_UNAVAILABLE" || d.Code == "UNSUPPORTED_HOST" || d.Code == "INVALID_OPTIONS" {
			return Plan{}, errors.New("bootstrap root unavailable")
		}
	}
	return Analyze(s, o)
}

// Analyze selects existing audit findings, never independently asserts a gap.
// Unknown reader scope suppresses proposals rather than asserting a destination absent.
func Analyze(s discovery.Snapshot, o Options) (Plan, error) {
	raw, err := audit.Analyze(s, audit.Options{Platforms: o.Platforms})
	if err != nil {
		return Plan{}, err
	}
	p := Plan{Version: Version, DryRun: true, SourceReport: raw, Recommendations: []Recommendation{}, Limitations: []string{"Dry-run only; no execution, efficacy, historical run or required-check enforcement is established."}}
	var report sourceReport
	if err := json.Unmarshal(raw, &report); err != nil {
		return Plan{}, err
	}
	partial := len(s.Limitations) > 0
	for _, d := range s.Diagnostics {
		// An intentional descendant boundary is outside root authority, not a
		// failed root read. It still blocks a draft whose destination it hides.
		excludedRepo := d.Code == "EXCLUDED_REPOSITORY" && d.State == "UNKNOWN" &&
			d.Path != "" && d.Path != "." && path.Clean(d.Path) == d.Path &&
			!path.IsAbs(d.Path) && !strings.HasPrefix(d.Path, "../") &&
			!strings.ContainsAny(d.Path, "\\:")
		if !excludedRepo || strings.HasPrefix(destination, d.Path+"/") {
			partial = true
			break
		}
	}
	if partial {
		p.Limitations = append(p.Limitations, "Partial or unsafe collection: destination and source scope are unresolved; no draft proposed.")
		return p, nil
	}
	declared := true
	findings := map[string][]string{}
	refs := map[string][]string{}
	for _, f := range report.Findings {
		if f.Rule == "CIT_DECLARATION_UNKNOWN" {
			declared = false
		}
		findings[f.Rule] = append(findings[f.Rule], f.ID)
		refs[f.ID] = f.Refs
	}
	if !declared {
		p.Limitations = append(p.Limitations, "No valid declared or explicit platform scope; no platforms guessed.")
		return p, nil
	}
	platforms := append([]string{}, o.Platforms...)
	if len(platforms) == 0 {
		for _, d := range s.Documents {
			if d.Path == ".ci-trust.json" {
				var v struct {
					Platforms []string `json:"platforms"`
				}
				_ = json.Unmarshal(d.Content, &v)
				platforms = v.Platforms
			}
		}
	}
	platforms = unique(platforms)
	if len(platforms) == 0 {
		p.Limitations = append(p.Limitations, "No evidenced platforms available.")
		return p, nil
	}
	inv := discovery.Analyze(s, discovery.Options{})
	graph := workflows.Analyze(s, inv, workflows.Options{})
	for _, d := range inv.Diagnostics {
		// The reader boundary was validated above; keep it in the source report,
		// while inventory's own unresolved declarations still suppress drafts.
		if d.Code != "EXCLUDED_REPOSITORY" {
			p.Limitations = append(p.Limitations, "Unresolved inventory: no command draft proposed.")
			return p, nil
		}
	}
	roots := map[string]string{}
	for _, r := range inv.Roots {
		roots[r.ID] = r.Path
	}
	selected := []string{}
	commands := []command{}
	for _, c := range inv.Candidates {
		fid := evidence.StableID("finding", "CIT_COMMAND_UNLINKED", c.ID)
		if c.Origin != "SCRIPT" || c.State != "CONFIGURED" || len(refs[fid]) == 0 {
			continue
		}
		if c.Kind == "unknown" || c.Kind == "" || c.Command == "" {
			continue
		}
		root := roots[c.Root]
		if strings.Contains(root, "${{") || strings.Contains(c.Source.Symbol, "${{") {
			p.Limitations = append(p.Limitations, "Literal npm root or script name contains an Actions expression; no draft can preserve its literal meaning.")
			return p, nil
		}
		// The unlinked finding proves its entry definition, not an indirect
		// script's resolved leaf chain. Do not claim that missing provenance.
		for _, source := range c.Chain {
			if source != c.Source {
				p.Limitations = append(p.Limitations, "Indirect npm script chain lacks decisive leaf/invocation provenance in the selected finding; no draft proposed.")
				return p, nil
			}
		}
		commands = append(commands, command{"npm run " + c.Source.Symbol, root})
		selected = append(selected, fid)
	}
	// Actual configured invocations can support a platform-gap draft;
	// go.mod conventions cannot supply commands.
	if len(commands) == 0 && len(findings["CIT_PLATFORM_CONFIGURED_GAP"]) > 0 {
		for _, w := range graph.Workflows {
			for _, j := range w.Jobs {
				for _, st := range j.Steps {
					isTest := st.Run == "go test ./..." || st.Run == "go test ."
					for _, link := range st.Links {
						for _, candidate := range inv.Candidates {
							if candidate.Command == link.Command && candidate.Kind == "test" && candidate.State == "CONFIGURED" && candidate.Origin == "SCRIPT" {
								isTest = true
							}
						}
					}
					if !isTest {
						continue
					}
					// Gap evidence does not retain the decisive inherited run
					// declarations. Limit instead of dropping shell/root semantics.
					if st.Shell.Raw != "" || st.WorkingDirectory.Raw != "" {
						p.Limitations = append(p.Limitations, "Configured execution context requires shell/working-directory provenance not retained by the gap evidence; no draft proposed.")
						return p, nil
					}
					root := st.WorkingDirectory.Raw
					if root == "" {
						root = "."
					}
					if strings.ContainsAny(root, "${}\\\n\r") || strings.HasPrefix(root, "/") || strings.Contains(root, "..") {
						continue
					}
					commands = append(commands, command{st.Run, root})
				}
			}
		}
		if len(commands) == 0 {
			p.Limitations = append(p.Limitations, "No supported TEST command addresses the reported TEST platform gap; no lint-only substitution proposed.")
		}
		selected = append(selected, findings["CIT_PLATFORM_CONFIGURED_GAP"]...)
		gaps := []string{}
		for _, v := range platforms {
			if len(refs[evidence.StableID("finding", "CIT_PLATFORM_CONFIGURED_GAP", v)]) > 0 {
				gaps = append(gaps, v)
			}
		}
		platforms = gaps
	}
	if len(commands) == 0 {
		p.Limitations = append(p.Limitations, "No supported source-backed unlinked command or bounded platform-gap command; conventions and unresolved commands are not templates.")
		return p, nil
	}
	sort.Slice(commands, func(i, j int) bool {
		if commands[i].root != commands[j].root {
			return commands[i].root < commands[j].root
		}
		return commands[i].text < commands[j].text
	})
	dedup := commands[:0]
	for _, c := range commands {
		if len(dedup) == 0 || dedup[len(dedup)-1] != c {
			dedup = append(dedup, c)
		}
	}
	commands = dedup
	content := workflow(commands, platforms)
	r := Recommendation{ID: evidence.StableID("recommendation", "bootstrap-v1", content), Path: destination, Content: content, FindingRefs: unique(selected), EvidenceRefs: []string{}, Preconditions: []string{"Review proposed pull_request, push and workflow_dispatch triggers; these defaults are proposed policy, not discovered intent.", "Provide and verify the required runtime on each runner; no runtime version or setup is inferred.", "Provide and verify required dependencies and tools before these commands; no installation is proposed.", "Review command portability and repository-controlled script effects before applying manually."}, Conflicts: []string{}, Limitations: []string{"Conditional configuration draft only; commands and actions have not been executed.", "No branch policy, complete functional coverage or remote enforcement guarantee."}}
	for _, id := range r.FindingRefs {
		r.EvidenceRefs = append(r.EvidenceRefs, refs[id]...)
	}
	// Declaration provenance is authoritative audit evidence, including explicit-option UNKNOWN provenance.
	for _, ev := range report.Evidence {
		if ev.ID == evidence.StableID("evidence", "declaration") {
			r.EvidenceRefs = append(r.EvidenceRefs, ev.ID)
		}
	}
	r.EvidenceRefs = unique(r.EvidenceRefs)
	old := ""
	for _, d := range s.Documents {
		if d.Path == destination {
			old = string(d.Content)
			r.Conflicts = append(r.Conflicts, "Destination is occupied: manual review required; no replacement or merge is authorized.")
		}
	}
	if old == content {
		return p, nil
	}
	r.Diff = proposalDiff(old, content, len(r.Conflicts) > 0)
	p.Recommendations = append(p.Recommendations, r)
	return p, nil
}
func unique(xs []string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
