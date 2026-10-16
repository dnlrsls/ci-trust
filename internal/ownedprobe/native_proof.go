package ownedprobe

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"syscall"
)

const limitScratchPath = "/tmp/ci-trust-owned-proof-limit-v1"
const hostSentinelPath = "/home/runner/work/ci-trust/ci-trust/.git/controlled-proof/host-sentinel"
const childKey = "CI_TRUST_OWNED_PROOF_CHILD"

// Fixed operations only; nil operations fail closed. Tests inject every dynamic
// boundary and never invoke the defaults or host procfs.
type nativeProofOps struct {
	lookup  func(string) (string, bool)
	connect func() error
	cpu     func()
	child   func(string) childResult
	work    func(string)
}
type childResult struct{ started, waited, failed, timedOut bool }

func proofReadBound(path string) int {
	switch path {
	case "/sys/fs/cgroup/memory.oom.group":
		return 16
	case "/sys/fs/cgroup/cpu.stat", "/sys/fs/cgroup/memory.events", "/sys/fs/cgroup/pids.events":
		return 4096
	case "/proc/net/route", "/proc/net/ipv6_route", "/proc/net/dev":
		return 16384
	}
	return 0
}

// Separate exact allowlist: the legacy Read boundary is deliberately unchanged.
func (s *nativeSystem) proofRead(path string) ([]byte, error) {
	limit := proofReadBound(path)
	if limit == 0 || !observe(s) {
		return nil, unavailable
	}
	f, err := s.ops.open(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, unavailable
	}
	data, err := bounded(f, limit)
	return data, errors.Join(err, f.Close())
}

func counter(data []byte, key string) (uint64, bool) {
	found := false
	var value uint64
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if fields[0] != key {
			continue
		}
		if found || len(fields) != 2 {
			return 0, false
		}
		n, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, false
		}
		found = true
		value = n
	}
	return value, found
}

func (s *nativeSystem) event(path, key string) (uint64, bool) {
	data, err := s.proofRead(path)
	if err != nil {
		return 0, false
	}
	return counter(data, key)
}

func (s *nativeSystem) ObserveProof() ProofObservations {
	var result ProofObservations
	if !observe(s) {
		return result
	}
	// Env is consulted only after the same full guards as the public proof. The
	// child uses the same public fixed argv, never caller-supplied executable args.
	if s.proof.lookup != nil {
		mode, present := s.proof.lookup(childKey)
		if present {
			if (mode == "memory" || mode == "pids") && s.proof.work != nil && observe(s) && (mode != "memory" || s.memoryChildSafe()) {
				s.proof.work(mode)
			}
			return result
		}
	}
	result.ScratchLimit = s.scratchLimit()
	if !result.ScratchLimit {
		return result
	}
	result.NetworkUnreachable = s.networkAbsent()
	if !result.NetworkUnreachable {
		return result
	}
	result.HostCredentialsAbsent = s.credentialsAbsent()
	if !result.HostCredentialsAbsent {
		return result
	}
	before, ok := s.event("/sys/fs/cgroup/cpu.stat", "nr_throttled")
	if !ok || s.proof.cpu == nil || !observe(s) {
		return result
	}
	s.proof.cpu()
	after, ok := s.event("/sys/fs/cgroup/cpu.stat", "nr_throttled")
	result.CPU = CounterObservation{Supported: ok && after > before, Before: before, After: after}
	if !result.CPU.Supported {
		return result
	}
	result.Memory = s.childEffect("memory", "/sys/fs/cgroup/memory.events", "max", "oom_kill")
	if !result.Memory.Supported {
		return result
	}
	result.Pids = s.childEffect("pids", "/sys/fs/cgroup/pids.events", "max", "")
	return result
}

func (s *nativeSystem) memoryChildSafe() bool {
	data, err := s.proofRead("/sys/fs/cgroup/memory.oom.group")
	return err == nil && string(data) == "0\n"
}

func (s *nativeSystem) childEffect(mode, path, key, alternate string) CounterObservation {
	var result CounterObservation
	if mode == "memory" && !s.memoryChildSafe() {
		return result
	}
	data, err := s.proofRead(path)
	if err != nil {
		return result
	}
	before, ok := counter(data, key)
	if !ok {
		return result
	}
	var altBefore uint64
	if alternate != "" {
		altBefore, ok = counter(data, alternate)
		if !ok {
			return result
		}
	}
	if s.proof.child == nil || !observe(s) {
		return result
	}
	waited := s.proof.child(mode)
	// Even failed starts/waits never become resource evidence. Native runner
	// always reaps; fake seams explicitly report each lifetime stage.
	if !waited.started || !waited.waited || !waited.failed || waited.timedOut {
		return result
	}
	data, err = s.proofRead(path)
	if err != nil {
		return result
	}
	after, ok := counter(data, key)
	if !ok {
		return result
	}
	if after > before {
		return CounterObservation{Supported: true, Before: before, After: after}
	}
	if alternate != "" {
		altAfter, ok := counter(data, alternate)
		if ok && altAfter > altBefore {
			return CounterObservation{Supported: true, Before: altBefore, After: altAfter}
		}
	}
	return result
}

func (s *nativeSystem) scratchLimit() (success bool) {
	if !observe(s) {
		return false
	}
	f, err := s.ops.open(limitScratchPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false
	}
	identity, statErr := f.Stat()
	// No acquired identity means cleanup must refuse to unlink an unknown inode.
	defer func() {
		closeErr := f.Close()
		if statErr != nil || identity == nil || !identity.Mode().IsRegular() {
			success = false
			return
		}
		current, err := s.ops.lstat(limitScratchPath)
		if err != nil || !current.Mode().IsRegular() || !s.ops.same(identity, current) {
			success = false
			return
		}
		if s.ops.remove(limitScratchPath) != nil || closeErr != nil {
			success = false
		}
	}()
	if statErr != nil || !identity.Mode().IsRegular() {
		return false
	}
	chunk := make([]byte, 32*1024)
	remaining := 8*1024*1024 + 1
	for remaining > 0 {
		size := len(chunk)
		if remaining < size {
			size = remaining
		}
		// Recheck all read-only controls without allocating another scratch file.
		// The final ENOSPC attempt must remain reachable when tmpfs is full.
		if !guardControls(s) {
			return false
		}
		n, err := f.Write(chunk[:size])
		remaining -= size
		if errors.Is(err, syscall.ENOSPC) && n >= 0 && n < size {
			return true
		}
		if err != nil || n != size {
			return false
		}
	}
	return false
}

func onlyLoopback(dev, route, ipv6 []byte) bool {
	lines := strings.Split(strings.TrimSpace(string(dev)), "\n")
	if len(lines) < 3 || !strings.Contains(lines[0], "Inter-") || !strings.Contains(lines[1], "face") {
		return false
	}
	count := 0
	for _, line := range lines[2:] {
		name, values, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(name) != "lo" {
			return false
		}
		fields := strings.Fields(values)
		if len(fields) != 16 {
			return false
		}
		for _, field := range fields {
			if _, err := strconv.ParseUint(field, 10, 64); err != nil {
				return false
			}
		}
		count++
	}
	if count != 1 {
		return false
	}
	// No IPv4 route entries at all. IPv6 may contain only loopback local/reject
	// routes, never a nonzero next hop or a non-loopback interface.
	routes := strings.Split(strings.TrimSpace(string(route)), "\n")
	if len(routes) != 1 || !strings.HasPrefix(routes[0], "Iface\t") {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(ipv6)), "\n") {
		if line == "" {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 10 || f[9] != "lo" || f[4] != strings.Repeat("0", 32) {
			return false
		}
		// Only ::/0 reject or ::1/128 local entries are safe here.
		if !((f[0] == strings.Repeat("0", 32) && f[1] == "00") || (f[0] == strings.Repeat("0", 31)+"1" && f[1] == "80")) {
			return false
		}
		if f[2] != strings.Repeat("0", 32) || f[3] != "00" {
			return false
		}
	}
	return true
}

func (s *nativeSystem) networkAbsent() bool {
	dev, e1 := s.proofRead("/proc/net/dev")
	route, e2 := s.proofRead("/proc/net/route")
	v6, e3 := s.proofRead("/proc/net/ipv6_route")
	if e1 != nil || e2 != nil || e3 != nil || !onlyLoopback(dev, route, v6) || s.proof.connect == nil || !observe(s) {
		return false
	}
	return errors.Is(s.proof.connect(), syscall.ENETUNREACH)
}

func (s *nativeSystem) credentialsAbsent() bool {
	if s.proof.lookup == nil || !observe(s) {
		return false
	}
	// Inspect presence only; values are never returned or printed.
	for _, key := range []string{"CI_TRUST_HOST_CREDENTIAL_SENTINEL", "GITHUB_TOKEN", "GH_TOKEN", "AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "SSH_AUTH_SOCK", "DOCKER_AUTH_CONFIG", "GOOGLE_APPLICATION_CREDENTIALS", "AZURE_CLIENT_SECRET"} {
		if _, exists := s.proof.lookup(key); exists {
			return false
		}
	}
	if !observe(s) {
		return false
	}
	_, err := s.ops.lstat(hostSentinelPath)
	return errors.Is(err, os.ErrNotExist)
}
