package evidence

// Validate checks both v1 structure and semantic admissibility. A valid report
// is not proof that its producer's evidence is true. No input bytes are changed.
func Validate(data []byte) []Diagnostic {
	if diagnostics := ValidateStructure(data); len(diagnostics) != 0 {
		return diagnostics
	}
	return ValidateSemantics(data)
}

// Encode emits canonical JSON only after the full report contract accepts the
// document. Rejected input produces diagnostics and no output bytes. Producers
// must call Redact with known sensitive values before submitting report data.
func Encode(data []byte) ([]byte, []Diagnostic) {
	if diagnostics := Validate(data); len(diagnostics) != 0 {
		return nil, diagnostics
	}
	return Canonicalize(data)
}
