// Package discovery inventories bounded static inputs without executing target code.
// Its typed intermediate results are not a serialized evidence report.
package discovery

// Options bounds traversal, bytes and npm reference expansion. Zero uses defaults.
type Options struct {
	MaxDepth       int
	MaxEntries     int
	MaxFileBytes   int64
	MaxTotalBytes  int64
	MaxReferences  int
	MaxScriptDepth int
}

// DefaultOptions returns conservative, finite limits for a scan.
func DefaultOptions() Options {
	return Options{MaxDepth: 16, MaxEntries: 4096, MaxFileBytes: 1 << 20, MaxTotalBytes: 8 << 20, MaxReferences: 256, MaxScriptDepth: 16}
}

// normalizeOptions is shared so direct and composed boundaries agree on caps.
func normalizeOptions(options Options) (Options, bool) {
	defaults := DefaultOptions()
	for _, item := range []struct {
		value         *int
		fallback, cap int
	}{{&options.MaxDepth, defaults.MaxDepth, 64}, {&options.MaxEntries, defaults.MaxEntries, 65536}, {&options.MaxReferences, defaults.MaxReferences, 4096}, {&options.MaxScriptDepth, defaults.MaxScriptDepth, 64}} {
		if *item.value < 0 || *item.value > item.cap {
			return options, false
		}
		if *item.value == 0 {
			*item.value = item.fallback
		}
	}
	for _, item := range []struct {
		value         *int64
		fallback, cap int64
	}{{&options.MaxFileBytes, defaults.MaxFileBytes, 16 << 20}, {&options.MaxTotalBytes, defaults.MaxTotalBytes, 64 << 20}} {
		if *item.value < 0 || *item.value > item.cap {
			return options, false
		}
		if *item.value == 0 {
			*item.value = item.fallback
		}
	}
	return options, true
}

// Source identifies a file-relative anchor; Symbol identifies a script definition.
// Zero Line/Column means the exact token position is unknown, not a v1 report source.
type Source struct {
	Path   string
	Line   int
	Column int
	Symbol string
}

// Document contains only a recognized, bounded file, never an executable module.
type Document struct {
	Path    string
	Content []byte
}

// Diagnostic describes an unresolved input without echoing its payload or host path.
type Diagnostic struct {
	Code    string
	Path    string
	Message string
	State   string
}

// Snapshot contains owned input bytes and explicit omissions, not a live-tree guarantee.
type Snapshot struct {
	Documents   []Document
	Diagnostics []Diagnostic
	Limitations []string
}

// Root keeps each manifest directory and stack independent.
type Root struct {
	ID     string
	Path   string
	Stack  string
	Source Source
}

// Tool is a static tooling candidate, not proof of installation or execution.
// Root is the stable owning Root.ID, not an absolute filesystem path.
type Tool struct {
	ID     string
	Root   string
	Name   string
	Source Source
	State  string
}

// Candidate is a bounded literal command or an explicitly conventional suggestion.
// Root is the stable owning Root.ID. Chain preserves reference/definition order.
type Candidate struct {
	ID      string
	Root    string
	Kind    string
	Command string
	Source  Source
	Chain   []Source
	State   string
	Origin  string
}

// Result is an intermediate inventory for future analyzers, never execution evidence.
type Result struct {
	Roots       []Root
	Tools       []Tool
	Candidates  []Candidate
	Diagnostics []Diagnostic
	Limitations []string
}
