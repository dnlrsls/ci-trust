// Command cipreflight collects CI preparation metadata, never isolation proof.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sync"
	"time"
)

const usage = "usage: cipreflight [--help]\nMetadata only; never starts containers or requests consent.\n"
const endpoint = "unix:///var/run/docker.sock"
const config = ".git/ci-trust-preflight-empty"

type metadata struct {
	Version      string `json:"version"`
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Cgroup       string `json:"reported_cgroup_version"`
	CPUs         int    `json:"reported_cpus"`
	Memory       int64  `json:"reported_memory_bytes"`
	MemoryLimit  bool   `json:"reported_memory_limit"`
	SwapLimit    bool   `json:"reported_swap_limit"`
	PIDsLimit    bool   `json:"reported_pids_limit"`
	CPUQuota     bool   `json:"reported_cpu_quota"`
}
type preparation struct {
	Status         string    `json:"status"`
	IsolationProof bool      `json:"isolation_proof"`
	ExecutablePlan bool      `json:"executable_plan"`
	Captured       bool      `json:"metadata_captured"`
	Source         string    `json:"source_revision"`
	Endpoint       string    `json:"endpoint"`
	Runtime        *metadata `json:"runtime"`
	Reason         string    `json:"reason"`
	Missing        []string  `json:"missing"`
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }
func run(args []string, out, errOut io.Writer) int {
	if len(args) > 0 {
		if len(args) == 1 && args[0] == "--help" {
			_, _ = io.WriteString(out, usage)
			return 0
		}
		_, _ = io.WriteString(errOut, usage)
		return 2
	}
	p := collect()
	if err := json.NewEncoder(out).Encode(p); err != nil {
		_, _ = io.WriteString(errOut, "preflight: output error\n")
		return 1
	}
	return 0
}
func collect() preparation {
	p := preparation{Status: "UNVERIFIABLE", Endpoint: endpoint, Missing: []string{"production backend and immutable adapter catalog", "authenticated current-plan consent boundary", "real isolation, resource, cancellation and cleanup proof"}}
	if s := os.Getenv("GITHUB_SHA"); regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`).MatchString(s) {
		p.Source = s
	}
	// The workflow explicitly prepares this empty client directory. This command
	// never creates files, falls back to a user's Docker context, or reads credentials.
	entries, err := os.ReadDir(config)
	st, statErr := os.Lstat(config)
	if err != nil || statErr != nil || !st.IsDir() || st.Mode()&os.ModeSymlink != 0 || len(entries) != 0 {
		p.Reason = "client configuration is not an empty directory"
		return p
	}
	b, err := capture("version", "{{json .Server}}")
	if err != nil {
		p.Reason = "version metadata unavailable"
		return p
	}
	var server struct{ Version, Os, Arch string }
	if json.Unmarshal(b, &server) != nil || !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:[-+][A-Za-z0-9.-]+)?$`).MatchString(server.Version) || len(server.Version) > 64 || server.Os != "linux" || (server.Arch != "amd64" && server.Arch != "arm64") {
		p.Reason = "invalid Docker metadata"
		return p
	}
	b, err = capture("info", "{{json .}}")
	if err != nil {
		p.Reason = "info metadata unavailable"
		return p
	}
	var info struct {
		OSType, CgroupVersion                          string
		NCPU                                           int
		MemTotal                                       int64
		MemoryLimit, SwapLimit, PidsLimit, CpuCfsQuota bool
	}
	if json.Unmarshal(b, &info) != nil || info.OSType != "linux" || (info.CgroupVersion != "1" && info.CgroupVersion != "2") || info.NCPU <= 0 || info.MemTotal <= 0 {
		p.Reason = "invalid Docker metadata"
		return p
	}
	p.Runtime = &metadata{server.Version, server.Os, server.Arch, info.CgroupVersion, info.NCPU, info.MemTotal, info.MemoryLimit, info.SwapLimit, info.PidsLimit, info.CpuCfsQuota}
	p.Captured = true
	p.Reason = "metadata only; enforcement and daemon host scope not attested"
	return p
}

// capture has a fixed built-in command vocabulary. No caller-supplied arguments,
// shell, remote context, credential environment or raw diagnostics are forwarded.
func capture(command, format string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, "docker", "--host="+endpoint, "--config="+config, command, "--format", format)
	c.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C"}
	// Descendant-held pipes cannot extend collection indefinitely after exit or cancellation.
	c.WaitDelay = 200 * time.Millisecond
	output := &budget{remaining: 64 << 10}
	c.Stdout = stream{output, true}
	c.Stderr = stream{output, false}
	if err := c.Run(); err != nil {
		return nil, err
	}
	if output.overflow {
		return nil, errors.New("metadata output limit")
	}
	return output.data, nil
}

// Both streams share a synchronized quota; only stdout is retained for parsing.
type budget struct {
	sync.Mutex
	data      []byte
	remaining int
	overflow  bool
}
type stream struct {
	budget *budget
	retain bool
}

func (s stream) Write(p []byte) (int, error) {
	b := s.budget
	b.Lock()
	defer b.Unlock()
	if len(p) > b.remaining {
		b.overflow = true
		return 0, errors.New("metadata output limit")
	}
	if s.retain {
		b.data = append(b.data, p...)
	}
	b.remaining -= len(p)
	return len(p), nil
}
