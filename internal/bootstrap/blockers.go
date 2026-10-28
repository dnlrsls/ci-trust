package bootstrap

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"unicode"

	"github.com/dnlrsls/ci-trust/internal/discovery"
)

// Explain omissions, never copy diagnostic messages or limitation payloads.
func collectionBlocker(d discovery.Diagnostic) string {
	code := d.Code
	action := "Input could not be collected safely; review the omitted input and retry only with a complete supported source scope."
	switch code {
	case "ENTRY_LIMIT":
		action = "Entry limit reached; select a smaller complete repository scope before retrying."
	case "DEPTH_LIMIT":
		action = "Depth limit reached; select a complete source scope within supported limits."
	case "SYMLINK_SKIPPED":
		action = "Symbolic link skipped; review the link and provide regular in-root inputs before retrying."
	case "EXCLUDED_DIRECTORY":
		action = "Directory intentionally omitted; review whether its omission prevents establishing the complete source scope."
	case "EXCLUDED_REPOSITORY":
		action = "Repository boundary is invalid or covers the destination; review boundaries without following gitdir contents."
	case "FILE_LIMIT", "TOTAL_LIMIT":
		action = "Byte limit reached; review oversized inputs and select a complete source scope within supported limits."
	case "DIRECTORY_READ", "ENTRY_READ", "FILE_READ", "REPOSITORY_MARKER_READ", "SPECIAL_FILE_SKIPPED", "FILE_CHANGED", "ROOT_UNAVAILABLE", "UNSUPPORTED_HOST", "INVALID_OPTIONS":
		// Original bounded codes identify the reason without echoing OS errors.
	default:
		code = "UNRESOLVED_COLLECTION"
	}
	return fmt.Sprintf("Collection blocker %s at %s: %s", code, blockerPath(d.Path), action)
}

func blockerPath(p string) string {
	if p == "" || len(p) > 512 || path.IsAbs(p) || path.Clean(p) != p || p == ".." || strings.HasPrefix(p, "../") || strings.ContainsAny(p, "\\:") {
		return "[unavailable]"
	}
	for _, r := range p {
		if !unicode.IsPrint(r) {
			return "[unavailable]"
		}
	}
	return strconv.QuoteToASCII(p)
}
