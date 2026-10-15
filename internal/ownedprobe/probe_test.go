package ownedprobe_test

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/ownedprobe"
)

// These literals are the oracle, not snapshots produced by implementation.
const status = "Uid:\t65534\t65534\t65534\t65534\nCapInh:\t0000000000000000\nCapPrm:\t0000000000000000\nCapEff:\t0000000000000000\nCapBnd:\t0000000000000000\nCapAmb:\t0000000000000000\nNoNewPrivs:\t1\nSeccomp:\t2\n"
const marker = "ci-trust owned-smoke-v1 root marker\n"
const changedMarker = "ci-trust owned-smoke-v1 root marker write check\n"
const scratch = "/tmp/ci-trust-owned-smoke-v1"
const help = "Usage: owned-smoke --fixture=smoke-v1 --config=owned-v1\nOwned-fixture-only partial observations; not isolation proof.\n"
const success = "owned-smoke: partial owned-fixture-only\nobserved: uid65534 caps0 no-new-privileges seccomp-filter root-marker-erofs scratch-write-cleanup cgroup-v2-profile\nnot-proven: resource-enforcement network credentials wall-time output-limit full-isolation\n"

var args = []string{"--fixture=smoke-v1", "--config=owned-v1"}

type call struct {
	op, path, data string
	limit          int
}

type fake struct {
	os          string
	uid         int
	files       map[string]string
	calls       []call
	readErr     map[string]error
	writeErr    error
	createErr   error
	createOwned bool
	removeErr   error
}

func fresh() *fake {
	return &fake{
		os: "linux", uid: 65534, writeErr: syscall.EROFS, createOwned: true,
		readErr: map[string]error{},
		files: map[string]string{
			"/proc/self/status":              status,
			"/proc/self/cgroup":              "0::/\n",
			"/proc/self/mounts":                   "tmpfs /tmp tmpfs rw,nosuid,nodev,noexec,size=8192k 0 0\n",
			"/sys/fs/cgroup/cpu.max":         "100000 100000\n",
			"/sys/fs/cgroup/memory.max":      "134217728\n",
			"/sys/fs/cgroup/memory.swap.max": "0\n",
			"/sys/fs/cgroup/pids.max":        "32\n",
			"/ci-trust/root-marker":          marker,
			"/unrelated-sentinel":            "unchanged",
		},
	}
}

func (f *fake) OS() string {
	f.calls = append(f.calls, call{op: "os"})
	return f.os
}
func (f *fake) UID() int {
	f.calls = append(f.calls, call{op: "uid"})
	return f.uid
}
func (f *fake) Read(path string, limit int) ([]byte, error) {
	f.calls = append(f.calls, call{op: "read", path: path, limit: limit})
	if err := f.readErr[path]; err != nil {
		return nil, err
	}
	value, ok := f.files[path]
	if !ok {
		return nil, syscall.ENOENT
	}
	if len(value) > limit {
		return nil, errors.New("bounded read exceeded")
	}
	return []byte(value), nil
}
func (f *fake) Write(path string, data []byte) error {
	f.calls = append(f.calls, call{op: "write", path: path, data: string(data)})
	if f.writeErr != nil {
		return f.writeErr
	}
	f.files[path] = string(data)
	return nil
}
func (f *fake) Create(path string, data []byte) (bool, error) {
	f.calls = append(f.calls, call{op: "create", path: path, data: string(data)})
	if _, exists := f.files[path]; exists {
		return false, syscall.EEXIST
	}
	if f.createOwned {
		f.files[path] = string(data)
	}
	return f.createOwned, f.createErr
}
func (f *fake) Remove(path string) error {
	f.calls = append(f.calls, call{op: "remove", path: path})
	if f.removeErr != nil {
		return f.removeErr
	}
	delete(f.files, path)
	return nil
}

func clone(files map[string]string) map[string]string {
	copy := make(map[string]string, len(files))
	for path, data := range files {
		copy[path] = data
	}
	return copy
}

func check(t *testing.T, f *fake, argv []string, exit int, out, errout string, calls []call, files map[string]string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	got := ownedprobe.Run(argv, f, &stdout, &stderr)
	if got != exit || stdout.String() != out || stderr.String() != errout {
		t.Errorf("exit/stdout/stderr = %d/%q/%q; want %d/%q/%q", got, stdout.String(), stderr.String(), exit, out, errout)
	}
	if !reflect.DeepEqual(f.calls, calls) {
		t.Errorf("provider calls = %#v; want %#v", f.calls, calls)
	}
	if !reflect.DeepEqual(f.files, files) {
		t.Errorf("provider storage changed unexpectedly: got %#v; want %#v", f.files, files)
	}
}

func TestHelpWithoutProviderEffects(t *testing.T) {
	f := fresh()
	check(t, f, []string{"--help"}, 0, help, "", nil, clone(f.files))
}

func TestRejectArgumentsWithoutProviderEffects(t *testing.T) {
	for _, tc := range []struct {
		name string
		argv []string
	}{
		{"missing", nil},
		{"fixture-only", []string{"--fixture=smoke-v1"}},
		{"config-only", []string{"--config=owned-v1"}},
		{"unknown-fixture", []string{"--fixture=other", "--config=owned-v1"}},
		{"unknown-config", []string{"--fixture=smoke-v1", "--config=other"}},
		{"unknown-option", []string{"--fixture=smoke-v1", "--config=owned-v1", "--unsafe"}},
		{"duplicate", []string{"--fixture=smoke-v1", "--config=owned-v1", "--config=owned-v1"}},
		{"positional", []string{"--fixture=smoke-v1", "--config=owned-v1", "target"}},
		{"help-does-not-hide-invalid", []string{"--help", "--unsafe"}},
		{"split-values", []string{"--fixture", "smoke-v1", "--config", "owned-v1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh()
			check(t, f, tc.argv, 2, "", "owned-smoke: invalid arguments\n", nil, clone(f.files))
		})
	}
}

// Every control path and read bound is independently specified. No fixture
// accesses real procfs, sockets, credentials, root or temporary directories.
func controlReads() []call {
	return []call{
		{op: "os"}, {op: "uid"},
		{op: "read", path: "/proc/self/status", limit: 8192},
		{op: "read", path: "/proc/self/cgroup", limit: 4096},
		{op: "read", path: "/proc/self/mounts", limit: 16384},
		{op: "read", path: "/sys/fs/cgroup/cpu.max", limit: 128},
		{op: "read", path: "/sys/fs/cgroup/memory.max", limit: 128},
		{op: "read", path: "/sys/fs/cgroup/memory.swap.max", limit: 128},
		{op: "read", path: "/sys/fs/cgroup/pids.max", limit: 128},
		{op: "read", path: "/ci-trust/root-marker", limit: 128},
	}
}

func TestOwnedPartialObservationsAndCleanup(t *testing.T) {
	for _, argv := range [][]string{args, {"--config=owned-v1", "--fixture=smoke-v1"}} {
		f := fresh()
		calls := append(controlReads(),
			call{op: "write", path: "/ci-trust/root-marker", data: changedMarker},
			call{op: "create", path: scratch, data: "owned-smoke-v1 scratch\n"},
			call{op: "remove", path: scratch})
		check(t, f, argv, 0, success, "", calls, clone(f.files))
	}
}

func TestRejectUnsafeControlsBeforeWrites(t *testing.T) {
	for _, tc := range []struct {
		name, path, value string
		lastRead          int
	}{
		{"uid-metadata", "/proc/self/status", strings.Replace(status, "Uid:\t65534\t65534\t65534\t65534", "Uid:\t0\t0\t0\t0", 1), 3},
		{"capabilities", "/proc/self/status", strings.Replace(status, "CapEff:\t0000000000000000", "CapEff:\t0000000000000001", 1), 3},
		{"no-new-privileges", "/proc/self/status", strings.Replace(status, "NoNewPrivs:\t1", "NoNewPrivs:\t0", 1), 3},
		{"seccomp", "/proc/self/status", strings.Replace(status, "Seccomp:\t2", "Seccomp:\t0", 1), 3},
		{"missing-capability", "/proc/self/status", strings.Replace(status, "CapAmb:\t0000000000000000\n", "", 1), 3},
		{"duplicate-control", "/proc/self/status", status + "Seccomp:\t2\n", 3},
		{"bounded-status", "/proc/self/status", strings.Repeat("x", 8193), 3},
		{"cgroup-v1", "/proc/self/cgroup", "1:cpu:/\n", 4},
		{"scratch-not-tmpfs", "/proc/self/mounts", "rootfs /tmp ext4 rw 0 0\n", 5},
		{"scratch-unbounded", "/proc/self/mounts", "tmpfs /tmp tmpfs rw 0 0\n", 5},
		{"scratch-executable", "/proc/self/mounts", "tmpfs /tmp tmpfs rw,nosuid,nodev,size=8192k 0 0\n", 5},
		{"scratch-too-large", "/proc/self/mounts", "tmpfs /tmp tmpfs rw,nosuid,nodev,noexec,size=16384k 0 0\n", 5},
		{"cpu-unbounded", "/sys/fs/cgroup/cpu.max", "max 100000\n", 6},
		{"memory-unbounded", "/sys/fs/cgroup/memory.max", "max\n", 7},
		{"swap-enabled", "/sys/fs/cgroup/memory.swap.max", "134217728\n", 8},
		{"pids-unbounded", "/sys/fs/cgroup/pids.max", "max\n", 9},
		{"foreign-marker", "/ci-trust/root-marker", "foreign\n", 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh()
			f.files[tc.path] = tc.value
			check(t, f, args, 1, "", "owned-smoke: unavailable\n", controlReads()[:tc.lastRead], clone(f.files))
		})
	}
}

func TestRejectHostBeforeKernelReads(t *testing.T) {
	for _, tc := range []struct {
		name, os string
		uid      int
		calls    []call
	}{
		{"non-linux", "windows", 65534, []call{{op: "os"}}},
		{"root", "linux", 0, []call{{op: "os"}, {op: "uid"}}},
		{"wrong-nonroot", "linux", 1000, []call{{op: "os"}, {op: "uid"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh()
			f.os, f.uid = tc.os, tc.uid
			check(t, f, args, 1, "", "owned-smoke: unavailable\n", tc.calls, clone(f.files))
		})
	}
}

func TestReadFailureStopsBeforeEffects(t *testing.T) {
	for i, c := range controlReads()[2:] {
		t.Run(c.path, func(t *testing.T) {
			f := fresh()
			f.readErr[c.path] = errors.New("private error must not escape")
			check(t, f, args, 1, "", "owned-smoke: unavailable\n", controlReads()[:i+3], clone(f.files))
		})
	}
}

func TestReadonlyRequiresEROFS(t *testing.T) {
	f := fresh()
	f.writeErr = syscall.EACCES
	calls := append(controlReads(), call{op: "write", path: "/ci-trust/root-marker", data: changedMarker})
	check(t, f, args, 1, "", "owned-smoke: unavailable\n", calls, clone(f.files))
}

func TestUnexpectedWritableMarkerRestoredWithoutScratch(t *testing.T) {
	f := fresh()
	f.writeErr = nil
	calls := append(controlReads(),
		call{op: "write", path: "/ci-trust/root-marker", data: changedMarker},
		call{op: "write", path: "/ci-trust/root-marker", data: marker})
	check(t, f, args, 1, "", "owned-smoke: unavailable\n", calls, clone(f.files))
}

func TestOwnedScratchFailureCleanup(t *testing.T) {
	for _, tc := range []struct {
		name        string
		owned       bool
		createError error
		removeError error
		foreign     bool
	}{
		{"write-failure-owned", true, errors.New("partial write"), nil, false},
		{"create-failure-unowned", false, syscall.EACCES, nil, false},
		{"missing-ownership", false, nil, nil, false},
		{"collision-unowned", false, nil, nil, true},
		{"cleanup-failure", true, nil, syscall.EACCES, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh()
			f.createOwned, f.createErr, f.removeErr = tc.owned, tc.createError, tc.removeError
			if tc.foreign {
				f.files[scratch] = "foreign must survive"
			}
			wantFiles := clone(f.files)
			calls := append(controlReads(),
				call{op: "write", path: "/ci-trust/root-marker", data: changedMarker},
				call{op: "create", path: scratch, data: "owned-smoke-v1 scratch\n"})
			if tc.owned {
				calls = append(calls, call{op: "remove", path: scratch})
				if tc.removeError != nil {
					wantFiles[scratch] = "owned-smoke-v1 scratch\n"
				}
			}
			check(t, f, args, 1, "", "owned-smoke: unavailable\n", calls, wantFiles)
		})
	}
}
