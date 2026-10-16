package ownedprobe

import (
	"bytes"
	"os"
	"syscall"
	"testing"
)

func proofAdapter() (*nativeSystem, *memoryOps) {
	s, m := adapter()
	for path, value := range map[string]string{
		"/proc/self/cgroup": "0::/\n", "/sys/fs/cgroup/cpu.max": "100000 100000\n",
		"/sys/fs/cgroup/memory.max": "134217728\n", "/sys/fs/cgroup/memory.swap.max": "0\n", "/sys/fs/cgroup/pids.max": "32\n",
	} {
		m.files[path] = &memoryFile{data: []byte(value)}
	}
	// Read succeeds, but opening the owned marker for write returns EROFS.
	open := s.ops.open
	s.ops.open = func(path string, flags int, mode os.FileMode) (file, error) {
		if path == markerPath && flags == os.O_RDWR {
			return nil, syscall.EROFS
		}
		if flags&os.O_EXCL != 0 {
			m.created = &memoryFile{}
		}
		return open(path, flags, mode)
	}
	return s, m
}

func TestNativeProofDispatchAfterGuards(t *testing.T) {
	s, m := proofAdapter()
	var out, errout bytes.Buffer
	code := Run([]string{"--fixture=smoke-v1", "--config=owned-proof-v1"}, s, &out, &errout)
	found := false
	for _, c := range m.calls {
		if c.path == "/tmp/ci-trust-owned-proof-limit-v1" && c.flags&os.O_EXCL != 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("native scratch operation never dispatched: exit=%d stdout=%q stderr=%q", code, out.String(), errout.String())
	}
}
