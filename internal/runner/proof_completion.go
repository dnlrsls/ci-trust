package runner

import (
	"encoding/json"
	"io"
	"strings"
)

// completedOwnedProof distinguishes the opt-in parent's completed report from a
// cgroup-wide OOM notification. Docker's OOMKilled flag records an OOM event, not
// necessarily PID 1's death. Main-process exit 0, successful attachment and this
// complete bounded owned report are all required; no legacy mode is relaxed.
// This validates completion evidence only, never a general security guarantee.
func completedOwnedProof(output string) bool {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 3 {
		return false
	}
	var document string
	start, stderr := 0, 0
	for _, line := range lines {
		switch line {
		case "owned-smoke: proof-started":
			start++
		case "owned-smoke: owned stderr":
			stderr++
		default:
			if document != "" {
				return false
			}
			document = line
		}
	}
	if start != 1 || stderr != 1 || document == "" {
		return false
	}
	var report struct {
		Version int    `json:"version"`
		Scope   string `json:"scope"`
		Checks  []struct {
			Name   string  `json:"name"`
			State  string  `json:"state"`
			Detail string  `json:"detail"`
			Before *uint64 `json:"before"`
			After  *uint64 `json:"after"`
		} `json:"checks"`
		Limitations []string `json:"limitations"`
	}
	d := json.NewDecoder(strings.NewReader(document))
	d.DisallowUnknownFields()
	if d.Decode(&report) != nil || report.Version != 1 || report.Scope != "owned-smoke-v1" || len(report.Limitations) == 0 {
		return false
	}
	var tail any
	if d.Decode(&tail) != io.EOF {
		return false
	}
	names := map[string]bool{
		"nonroot": false, "capabilities": false, "no_new_privileges": false,
		"seccomp": false, "root_readonly": false, "scratch_write_cleanup": false,
		"scratch_limit": false, "network_unreachable": false, "host_credentials_absent": false,
		"cpu_limit": false, "memory_limit": false, "pids_limit": false,
	}
	if len(report.Checks) != len(names) {
		return false
	}
	for _, c := range report.Checks {
		seen, known := names[c.Name]
		if !known || seen || c.State != "OBSERVED" || c.Detail == "" {
			return false
		}
		names[c.Name] = true
		switch c.Name {
		case "cpu_limit", "memory_limit", "pids_limit":
			if c.Before == nil || c.After == nil || *c.After <= *c.Before {
				return false
			}
		default:
			if c.Before != nil || c.After != nil {
				return false
			}
		}
	}
	return true
}
