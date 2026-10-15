// Package workflows analyzes bounded, offline GitHub Actions configuration.
// It never executes commands and its graph is not a schema-v1 evidence report.
package workflows

import "github.com/dnlrsls/ci-trust/internal/discovery"

// State distinguishes static configuration from eligibility and unknown evidence.
type State string

const (
	Unknown     State = "UNKNOWN"
	Eligible    State = "ELIGIBLE"
	Ineligible  State = "STATICALLY_INELIGIBLE"
	Conditional State = "CONDITIONAL"
	Configured  State = "CONFIGURED"
)

// Source is a repository-relative declaration anchor, not a host filename.
type Source = discovery.Source

// Value retains decoded YAML spelling, type, structure, and scope.
// Inherited values point to their declaration, not the consuming scope.
type Value struct {
	Raw       string
	Tag       string
	Source    Source
	Scope     string
	State     State
	Inherited bool
	Fields    map[string]Value
	Items     []Value
}

// Diagnostic records incomplete static understanding, not validator failure.
type Diagnostic struct {
	Code    string
	Message string
	Source  Source
	State   State
}

// Options applies admission limits at the direct Analyze boundary.
// Zero selects the documented defaults; negative/above-cap values are invalid.
type Options struct {
	MaxDocuments int
	MaxBytes     int
	MaxNodes     int
	MaxDepth     int
	MaxJobs      int
	MaxSteps     int
	MaxMatrix    int
}

// Result is an intermediate graph for pure rules, not a serialized report.
type Result struct {
	Workflows   []Workflow
	Diagnostics []Diagnostic
	Limitations []string
	State       State
	Historical  State
	Enforcement State
}

type Event struct {
	Name    string
	Source  Source
	Filters map[string]Value
	State   State
}

type Workflow struct {
	Path     string
	Name     string
	Source   Source
	Events   []Event
	Settings map[string]Value
	Jobs     []Job
}

// Matrix never exposes partial combinations after an expansion failure.
// Literals parallels Combinations and retains types and declaration anchors.
type Matrix struct {
	Combinations []map[string]string
	Literals     []map[string]Value
	State        State
	Source       Source
}

type Job struct {
	ID          string
	Name        string
	Uses        string
	Source      Source
	Needs       []string
	NeedSources []Source
	Condition   Value
	Runner      Value
	Settings    map[string]Value
	Matrix      Matrix
	Steps       []Step
	Eligibility map[string]State
	State       State
	Historical  State
	Enforcement State
}

type Step struct {
	ID               string
	Name             string
	Uses             string
	Run              string
	Source           Source
	Condition        Value
	Shell            Value
	WorkingDirectory Value
	ContinueOnError  Value
	RunValue         Value
	UsesValue        Value
	Eligibility      map[string]State
	Links            []Link
	Historical       State
}

// Link connects an exact literal invocation to a supplied discovery definition.
// It is not evidence that the command ran or was mandatory.
type Link struct {
	CandidateID string
	Root        string
	Command     string
	Chain       []Source
	State       State
}
