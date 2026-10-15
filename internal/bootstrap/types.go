// Package bootstrap describes conditional, read-only CI workflow proposals.
// A plan is not execution evidence or authority to apply any change.
package bootstrap

import "encoding/json"

// Version identifies the proposal protocol, not an alternative evidence schema.
const Version = "ci-trust-bootstrap-plan/v1"

// Options supplies explicit support targets with the same semantics as audit.
type Options struct {
	Platforms []string
}

// Plan retains the existing validated report-v1 as its evidence source.
// Runtime/dependency preparation may be an explicit unresolved precondition.
type Plan struct {
	Version         string           `json:"version"`
	DryRun          bool             `json:"dry_run"`
	SourceReport    json.RawMessage  `json:"source_report"`
	Recommendations []Recommendation `json:"recommendations"`
	Limitations     []string         `json:"limitations"`
}

// Recommendation is proposed content only. References resolve in SourceReport;
// conflicts require human review and never permit replacement or automatic merge.
type Recommendation struct {
	ID            string   `json:"id"`
	Path          string   `json:"path"`
	Content       string   `json:"content"`
	Diff          string   `json:"diff"`
	FindingRefs   []string `json:"finding_refs"`
	EvidenceRefs  []string `json:"evidence_refs"`
	Preconditions []string `json:"preconditions"`
	Conflicts     []string `json:"conflicts"`
	Limitations   []string `json:"limitations"`
}
