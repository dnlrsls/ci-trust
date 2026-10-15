// Package ownedprobe contains the internal, owned-fixture-only smoke tool.
// It does not produce report-v1 evidence or establish isolation acceptance.
package ownedprobe

import (
	"errors"
	"io"
	"strings"
	"syscall"
)

// System is the injectable boundary for fixed kernel controls and owned files.
// Read rejects data exceeding limit. Create uses exclusive creation and reports
// ownership even if a subsequent write or close fails. Write touches only the
// fingerprinted owned root marker; Remove touches only acquired scratch.
// There is deliberately no credential, environment or network API.
type System interface {
	OS() string
	UID() int
	Read(path string, limit int) ([]byte, error)
	Write(path string, data []byte) error
	Create(path string, data []byte) (owned bool, err error)
	Remove(path string) error
}

const (
	markerPath    = "/ci-trust/root-marker"
	marker        = "ci-trust owned-smoke-v1 root marker\n"
	changedMarker = "ci-trust owned-smoke-v1 root marker write check\n"
	scratchPath   = "/tmp/ci-trust-owned-smoke-v1"
	scratchData   = "owned-smoke-v1 scratch\n"
	help          = "Usage: owned-smoke --fixture=smoke-v1 --config=owned-v1\nOwned-fixture-only partial observations; not isolation proof.\n"
	observations  = "owned-smoke: partial owned-fixture-only\nobserved: uid65534 caps0 no-new-privileges seccomp-filter root-marker-erofs scratch-write-cleanup cgroup-v2-profile\nnot-proven: resource-enforcement network credentials wall-time output-limit full-isolation\n"
)

// Run accepts only the two literal arguments in either order, or --help alone.
// Unavailable observations never produce partial success or raw system errors.
func Run(args []string, system System, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == "--help" {
		if _, err := io.WriteString(stdout, help); err != nil {
			return 1
		}
		return 0
	}
	if len(args) != 2 || !((args[0] == "--fixture=smoke-v1" && args[1] == "--config=owned-v1") ||
		(args[1] == "--fixture=smoke-v1" && args[0] == "--config=owned-v1")) {
		io.WriteString(stderr, "owned-smoke: invalid arguments\n")
		return 2
	}
	if system == nil || !observe(system) {
		io.WriteString(stderr, "owned-smoke: unavailable\n")
		return 1
	}
	if _, err := io.WriteString(stdout, observations); err != nil {
		return 1
	}
	return 0
}

func observe(s System) bool {
	if s.OS() != "linux" || s.UID() != 65534 {
		return false
	}
	status, err := s.Read("/proc/self/status", 8192)
	if err != nil || !safeStatus(string(status)) {
		return false
	}
	for _, c := range []struct {
		path  string
		bound int
		valid func(string) bool
	}{
		{"/proc/self/cgroup", 4096, func(v string) bool { return v == "0::/\n" }},
		{"/proc/self/mounts", 16384, safeMount},
		{"/sys/fs/cgroup/cpu.max", 128, func(v string) bool { return v == "100000 100000\n" }},
		{"/sys/fs/cgroup/memory.max", 128, func(v string) bool { return v == "134217728\n" }},
		{"/sys/fs/cgroup/memory.swap.max", 128, func(v string) bool { return v == "0\n" }},
		{"/sys/fs/cgroup/pids.max", 128, func(v string) bool { return v == "32\n" }},
		{markerPath, 128, func(v string) bool { return v == marker }},
	} {
		data, err := s.Read(c.path, c.bound)
		if err != nil || !c.valid(string(data)) {
			return false
		}
	}
	if err := s.Write(markerPath, []byte(changedMarker)); !errors.Is(err, syscall.EROFS) {
		if err == nil {
			s.Write(markerPath, []byte(marker))
		}
		return false
	}
	owned, writeErr := s.Create(scratchPath, []byte(scratchData))
	var cleanupErr error
	if owned {
		cleanupErr = s.Remove(scratchPath)
	}
	return owned && writeErr == nil && cleanupErr == nil
}

func safeStatus(text string) bool {
	want := map[string]string{
		"Uid:": "65534 65534 65534 65534", "NoNewPrivs:": "1", "Seccomp:": "2",
		"CapInh:": "0000000000000000", "CapPrm:": "0000000000000000",
		"CapEff:": "0000000000000000", "CapBnd:": "0000000000000000", "CapAmb:": "0000000000000000",
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if value, required := want[fields[0]]; required {
			if seen[fields[0]] || strings.Join(fields[1:], " ") != value {
				return false
			}
			seen[fields[0]] = true
		}
	}
	return len(seen) == len(want)
}

func safeMount(text string) bool {
	found := false
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[1] != "/tmp" {
			continue
		}
		if found || len(f) != 6 || f[0] != "tmpfs" || f[2] != "tmpfs" {
			return false
		}
		options := map[string]bool{}
		for _, o := range strings.Split(f[3], ",") {
			if options[o] {
				return false
			}
			options[o] = true
		}
		if !options["rw"] || !options["nosuid"] || !options["nodev"] || !options["noexec"] || !options["size=8192k"] || options["ro"] {
			return false
		}
		for o := range options {
			if strings.HasPrefix(o, "size=") && o != "size=8192k" {
				return false
			}
		}
		found = true
	}
	return found
}
