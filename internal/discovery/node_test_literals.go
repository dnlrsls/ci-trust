package discovery

import (
	"path"
	"strconv"
	"strings"
)

// nodeTestLiteral admits declarations, not shell expansion or Node compatibility.
// The numeric ceiling is platform-independent; it is not a runtime capacity claim.
func nodeTestLiteral(fields []string) bool {
	if len(fields) < 2 || fields[0] != "node" || fields[1] != "--test" {
		return false
	}
	concurrency := false
	paths := false
	for _, field := range fields[2:] {
		if strings.HasPrefix(field, "--test-concurrency=") {
			if concurrency || paths {
				return false
			}
			value := strings.TrimPrefix(field, "--test-concurrency=")
			if value == "" || len(value) > 10 {
				return false
			}
			for _, digit := range value {
				if digit < '0' || digit > '9' {
					return false
				}
			}
			n, err := strconv.ParseUint(value, 10, 31)
			if err != nil || n == 0 {
				return false
			}
			concurrency = true
			continue
		}
		if !nodeTestPath(field) {
			return false
		}
		paths = true
	}
	return true
}

func nodeTestPath(token string) bool {
	if !strings.Contains(token, "*") {
		// Literal directory spellings are legacy command data, not document paths.
		// Validate components before cleaning so traversal cannot disappear, and
		// never replace the original token in command/reference provenance.
		if len(token) > 4096 || !literalToken.MatchString(token) || strings.HasPrefix(token, "-") || strings.HasPrefix(token, "/") || strings.ContainsAny(token, ":@") {
			return false
		}
		for _, component := range strings.Split(token, "/") {
			if component == ".." {
				return false
			}
		}
		clean := path.Clean(token)
		return clean == "." || safePath(clean)
	}
	// Only a filename component can contain stars. A leading ./ is retained in
	// command provenance but removed for the existing canonical safePath check.
	p := strings.TrimPrefix(token, "./")
	if !safePath(p) || strings.HasPrefix(p, "-") || strings.Contains(p, "**") {
		return false
	}
	components := strings.Split(p, "/")
	for i, component := range components {
		if component == "." || component == ".." {
			return false
		}
		if strings.Contains(component, "*") && i != len(components)-1 {
			return false
		}
	}
	// Do not widen literalToken for any other executable or argument family.
	plain := strings.ReplaceAll(p, "*", "x")
	return literalToken.MatchString(plain) && !strings.ContainsAny(plain, ":@")
}
