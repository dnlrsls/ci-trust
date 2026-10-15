package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/dnlrsls/ci-trust/internal/bootstrap"
	"github.com/dnlrsls/ci-trust/internal/evidence"
)

type sourceDiagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// renderPlan validates the complete artifact before returning any output.
func renderPlan(p bootstrap.Plan, format string) ([]byte, []sourceDiagnostic, error) {
	invalid := fmt.Errorf("invalid plan")
	raw, err := json.Marshal(p)
	if err != nil || len(raw) > maxInput || p.Version != bootstrap.Version || !p.DryRun {
		return nil, nil, invalid
	}
	canonical, ds := evidence.Encode(p.SourceReport)
	if len(ds) > 0 {
		return nil, nil, invalid
	}
	p.SourceReport = canonical
	var report map[string]json.RawMessage
	_ = json.Unmarshal(canonical, &report)
	ids := func(key string) map[string]bool {
		var items []struct {
			ID string `json:"id"`
		}
		_ = json.Unmarshal(report[key], &items)
		result := map[string]bool{}
		for _, item := range items {
			result[item.ID] = true
		}
		return result
	}
	findings, evidences := ids("findings"), ids("evidence")
	// Copy every sortable slice: the injected service retains ownership of its plan.
	sorted := func(values []string) []string {
		if values == nil {
			return nil
		}
		v := append([]string{}, values...)
		sort.Strings(v)
		return v
	}
	p.Limitations = sorted(p.Limitations)
	if p.Recommendations != nil {
		p.Recommendations = append([]bootstrap.Recommendation{}, p.Recommendations...)
	}
	seen := map[string]bool{}
	for i := range p.Recommendations {
		r := &p.Recommendations[i]
		if r.ID == "" || seen[r.ID] || r.Path == "" || strings.ContainsAny(r.Path, "\\:") || strings.HasPrefix(r.Path, "/") || path.Clean(r.Path) != r.Path || r.Path == ".." || strings.HasPrefix(r.Path, "../") || safe(r.Path) != r.Path {
			return nil, nil, invalid
		}
		seen[r.ID] = true
		if len(r.FindingRefs) == 0 || len(r.EvidenceRefs) == 0 {
			return nil, nil, invalid
		}
		for _, ref := range r.FindingRefs {
			if !findings[ref] {
				return nil, nil, invalid
			}
		}
		for _, ref := range r.EvidenceRefs {
			if !evidences[ref] {
				return nil, nil, invalid
			}
		}
		r.FindingRefs = sorted(r.FindingRefs)
		r.EvidenceRefs = sorted(r.EvidenceRefs)
		r.Preconditions = sorted(r.Preconditions)
		r.Conflicts = sorted(r.Conflicts)
		r.Limitations = sorted(r.Limitations)
	}
	sort.Slice(p.Recommendations, func(i, j int) bool { return p.Recommendations[i].ID < p.Recommendations[j].ID })
	var records []sourceDiagnostic
	_ = json.Unmarshal(report["diagnostics"], &records)
	raw, err = json.Marshal(p)
	if err != nil {
		return nil, nil, invalid
	}
	if format == "json" {
		return append([]byte(safe(string(raw))), '\n'), records, nil
	}
	var b bytes.Buffer
	b.WriteString("CI Trust bootstrap plan (dry-run only; no apply)\nConditional draft: not ready, executed, or enforced; review all preconditions and conflicts.\n")
	// JSON field names make all DTO fields explicit; safe escapes terminal controls.
	fmt.Fprintf(&b, "%s\n", safe(string(raw)))
	b.WriteString("Source uncertainty (configuration is not execution or enforcement):\n")
	b.Write(human(report))
	return b.Bytes(), records, nil
}
