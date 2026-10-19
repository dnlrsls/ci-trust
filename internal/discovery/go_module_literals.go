package discovery

import (
	"regexp"
	"strings"
)

// Versions are inert metadata, including prereleases and Go pseudo-versions.
// This deliberately does not resolve versions or infer validation commands.
var moduleMetadataVersion = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9]+([.-][A-Za-z0-9]+)*)?(\+incompatible)?$`)

func moduleLiteral(b []byte) bool {
	found := false
	block := ""
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.Trim(strings.SplitN(line, "//", 2)[0], " \t\r")
		if line == "" {
			continue
		}
		if !asciiLiteral(line) {
			return false
		}
		f := strings.Fields(line)
		if block != "" {
			if line == ")" {
				block = ""
				continue
			}
			if !moduleMetadata(block, f) {
				return false
			}
			continue
		}
		if (f[0] == "require" || f[0] == "tool") && len(f) == 2 && f[1] == "(" {
			block = f[0]
			continue
		}
		switch f[0] {
		case "module":
			if found || len(f) != 2 || !literalToken.MatchString(f[1]) || !safePath(f[1]) || strings.HasPrefix(f[1], "-") {
				return false
			}
			found = true
		case "go":
			if len(f) != 2 || !goVersion.MatchString(f[1]) {
				return false
			}
		case "toolchain":
			if len(f) != 2 || !toolchainVersion.MatchString(f[1]) {
				return false
			}
		case "require", "tool":
			if !moduleMetadata(f[0], f[1:]) {
				return false
			}
		default:
			return false
		}
	}
	return found && block == ""
}

func moduleMetadata(directive string, fields []string) bool {
	arity := 1
	if directive == "require" {
		arity = 2
	}
	if len(fields) != arity {
		return false
	}
	p := fields[0]
	// Keep metadata paths relative and literal; no URL schemes, versions,
	// drive letters, flags, traversal, quoting or shell substitutions.
	if !literalToken.MatchString(p) || !safePath(p) || strings.HasPrefix(p, "-") || strings.ContainsAny(p, ":@") {
		return false
	}
	return directive != "require" || moduleMetadataVersion.MatchString(fields[1])
}
