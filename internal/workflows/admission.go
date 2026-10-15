package workflows

// admit is a conservative lexical preflight before invoking the YAML parser.
// It bounds deeply nested flow collections and extreme block indentation even
// when malformed input would otherwise fail only after parser allocation.
// Quoted/comment braces also count: rejection is UNKNOWN, never coverage.
func (p *parser) admit(data []byte) bool {
	depth, indent, tokens := 0, 0, 0
	start := true
	for _, c := range data {
		switch c {
		case '\n':
			start = true
			indent = 0
			tokens++
		case ' ', '\t':
			if start {
				indent++
				if indent > p.options.MaxDepth*16 {
					return false
				}
			}
		default:
			start = false
			switch c {
			case '[', '{':
				depth++
				tokens++
				if depth > p.options.MaxDepth {
					return false
				}
			case ']', '}':
				if depth > 0 {
					depth--
				}
				tokens++
			case ',', ':':
				tokens++
			}
		}
		if tokens > p.options.MaxNodes {
			return false
		}
	}
	return true
}
