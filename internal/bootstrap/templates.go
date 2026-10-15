package bootstrap

import (
	"encoding/json"
	"fmt"
	"strings"
)

func scalar(s string) string { b, _ := json.Marshal(s); return string(b) }

// workflow is a versioned conditional template, not runtime provisioning.
func workflow(commands []command, platforms []string) string {
	var b strings.Builder
	b.WriteString("# ci-trust conditional draft v1; review all preconditions before use.\nname: CI Trust draft\non:\n  pull_request:\n  push:\n  workflow_dispatch:\npermissions:\n  contents: read\njobs:\n")
	runners := map[string]string{"linux": "ubuntu-latest", "windows": "windows-latest", "macos": "macos-latest"}
	for _, p := range platforms {
		fmt.Fprintf(&b, "  validate-%s:\n    runs-on: %s\n    timeout-minutes: 10\n    steps:\n      - uses: actions/checkout@d23441a48e516b6c34aea4fa41551a30e30af803\n        with:\n          persist-credentials: false\n", p, runners[p])
		b.WriteString("      # PRECONDITION: provide verified runtime and dependencies; no setup inferred.\n")
		for _, c := range commands {
			fmt.Fprintf(&b, "      - run: %s\n        working-directory: %s\n", scalar(c.text), scalar(c.root))
		}
	}
	return b.String()
}

// A whole-file review diff deliberately avoids automatic merge semantics.
func proposalDiff(old, content string, occupied bool) string {
	from := "/dev/null"
	if occupied {
		from = "a/" + destination
	}
	var b strings.Builder
	oldCount, oldStart := 0, 0
	if old != "" {
		oldCount = len(strings.Split(strings.TrimSuffix(old, "\n"), "\n"))
		oldStart = 1
	}
	fmt.Fprintf(&b, "--- %s\n+++ b/%s\n@@ -%d,%d +1,%d @@\n", from, destination, oldStart, oldCount, len(strings.Split(strings.TrimSuffix(content, "\n"), "\n")))
	if old != "" {
		for _, l := range strings.Split(strings.TrimSuffix(old, "\n"), "\n") {
			b.WriteString("-" + l + "\n")
		}
		if !strings.HasSuffix(old, "\n") {
			b.WriteString("\\ No newline at end of file\n")
		}
	}
	for _, l := range strings.Split(strings.TrimSuffix(content, "\n"), "\n") {
		b.WriteString("+" + l + "\n")
	}
	return b.String()
}
