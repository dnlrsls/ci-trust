package ownedprobe

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"reflect"
	"syscall"
	"testing"
	"time"
)

const safeKernel = "Uid:\t65534\t65534\t65534\t65534\nCapInh:\t0000000000000000\nCapPrm:\t0000000000000000\nCapEff:\t0000000000000000\nCapBnd:\t0000000000000000\nCapAmb:\t0000000000000000\nNoNewPrivs:\t1\nSeccomp:\t2\n"

// The adapter tests use only these in-memory descriptors, on every host OS.
type memoryFile struct {
	data           []byte
	pos            int64
	mode           fs.FileMode
	writes, closes int
	shortFirst     bool
	failWriteAt    int
	writeErr       error
	closeErr       error
}

func (f *memoryFile) Read(p []byte) (int, error) {
	if f.pos >= int64(len(f.data)) {
		return 0, io.EOF
	}
	n := copy(p, f.data[f.pos:])
	f.pos += int64(n)
	return n, nil
}
func (f *memoryFile) Write(p []byte) (int, error) {
	f.writes++
	n := len(p)
	if (f.shortFirst && f.writes == 1) || f.writes == f.failWriteAt {
		n = 1
	}
	end := int(f.pos) + n
	if end > len(f.data) {
		f.data = append(f.data, make([]byte, end-len(f.data))...)
	}
	copy(f.data[f.pos:], p[:n])
	f.pos += int64(n)
	if f.writes == f.failWriteAt {
		return n, f.writeErr
	}
	return n, nil
}
func (f *memoryFile) Seek(pos int64, whence int) (int64, error) {
	if whence != io.SeekStart || pos < 0 {
		return 0, errors.New("bad seek")
	}
	f.pos = pos
	return pos, nil
}
func (f *memoryFile) Truncate(n int64) error     { f.data = f.data[:n]; return nil }
func (f *memoryFile) Close() error               { f.closes++; return f.closeErr }
func (f *memoryFile) Stat() (fs.FileInfo, error) { return f, nil }
func (f *memoryFile) Name() string               { return "fake" }
func (f *memoryFile) Size() int64                { return int64(len(f.data)) }
func (f *memoryFile) Mode() fs.FileMode          { return f.mode }
func (f *memoryFile) ModTime() time.Time         { return time.Time{} }
func (f *memoryFile) IsDir() bool                { return false }
func (f *memoryFile) Sys() any                   { return nil }

type effect struct {
	op, path string
	flags    int
	mode     fs.FileMode
}
type memoryOps struct {
	files   map[string]*memoryFile
	calls   []effect
	openErr map[string]error
	created *memoryFile
}

func adapter() (*nativeSystem, *memoryOps) {
	m := &memoryOps{files: map[string]*memoryFile{
		"/proc/self/status":     {data: []byte(safeKernel)},
		"/proc/self/mounts":     {data: []byte("tmpfs /tmp tmpfs rw,nosuid,nodev,noexec,size=8192k 0 0\n")},
		"/ci-trust/root-marker": {data: []byte("ci-trust owned-smoke-v1 root marker\n")},
	}, openErr: map[string]error{}, created: &memoryFile{}}
	s := &nativeSystem{osName: "linux", uid: func() int { return 65534 }}
	s.ops = fileOps{
		open: func(path string, flags int, mode fs.FileMode) (file, error) {
			m.calls = append(m.calls, effect{"open", path, flags, mode})
			if err := m.openErr[path]; err != nil {
				return nil, err
			}
			if flags&os.O_EXCL != 0 {
				if m.files[path] != nil {
					return nil, syscall.EEXIST
				}
				m.files[path] = m.created
			}
			f := m.files[path]
			if f == nil {
				return nil, syscall.ENOENT
			}
			f.pos = 0
			return f, nil
		},
		lstat: func(path string) (fs.FileInfo, error) {
			m.calls = append(m.calls, effect{op: "lstat", path: path})
			f := m.files[path]
			if f == nil {
				return nil, syscall.ENOENT
			}
			return f, nil
		},
		same: func(a, b fs.FileInfo) bool {
			m.calls = append(m.calls, effect{op: "same"})
			return a == b
		},
		remove: func(path string) error {
			m.calls = append(m.calls, effect{op: "remove", path: path})
			delete(m.files, path)
			return nil
		},
	}
	return s, m
}

func effects(t *testing.T, m *memoryOps, want []effect) {
	t.Helper()
	if !reflect.DeepEqual(m.calls, want) {
		t.Errorf("effects = %#v; want %#v", m.calls, want)
	}
}

// Exercise public Run through nativeSystem, not a substitute System. Linux's
// /proc/mounts final-component symlink must not be followed to make this pass.
func TestRunAdapterUsesRegularSelfMounts(t *testing.T) {
	for _, unavailableMounts := range []bool{false, true} {
		name := "controlled-success"
		if unavailableMounts {
			name = "required-control-unavailable"
		}
		t.Run(name, func(t *testing.T) {
			s, m := adapter()
			uidCalls := 0
			s.uid = func() int { uidCalls++; return 65534 }
			m.openErr["/proc/mounts"] = syscall.ELOOP
			m.openErr["/ci-trust/root-marker"] = syscall.EROFS
			// The initial read of the marker is allowed; only its write-open
			// observes EROFS. Preserve the existing injected descriptor seam.
			open := s.ops.open
			s.ops.open = func(path string, flags int, mode fs.FileMode) (file, error) {
				if path == "/ci-trust/root-marker" && flags == os.O_RDONLY {
					delete(m.openErr, path)
					f, err := open(path, flags, mode)
					m.openErr[path] = syscall.EROFS
					return f, err
				}
				return open(path, flags, mode)
			}
			for path, data := range map[string]string{
				"/proc/self/cgroup":              "0::/\n",
				"/proc/self/mounts":              "tmpfs /tmp tmpfs rw,nosuid,nodev,noexec,size=8192k 0 0\n",
				"/sys/fs/cgroup/cpu.max":         "100000 100000\n",
				"/sys/fs/cgroup/memory.max":      "134217728\n",
				"/sys/fs/cgroup/memory.swap.max": "0\n",
				"/sys/fs/cgroup/pids.max":        "32\n",
				"/unrelated-sentinel":            "unchanged",
			} {
				m.files[path] = &memoryFile{data: []byte(data)}
			}
			if unavailableMounts {
				m.openErr["/proc/self/mounts"] = syscall.EACCES
			}
			before := map[string][]byte{}
			for path, f := range m.files {
				before[path] = bytes.Clone(f.data)
			}
			wantCalls := []effect{
				{op: "open", path: "/proc/self/status"},
				{op: "open", path: "/proc/self/cgroup"},
				{op: "open", path: "/proc/self/mounts"},
			}
			wantCloses := map[string]int{"/proc/self/status": 1, "/proc/self/cgroup": 1}
			wantExit, wantOut, wantErr, wantUID := 1, "", "owned-smoke: unavailable\n", 1
			if !unavailableMounts {
				wantExit, wantErr, wantUID = 0, "", 3
				wantOut = "owned-smoke: partial owned-fixture-only\nobserved: uid65534 caps0 no-new-privileges seccomp-filter root-marker-erofs scratch-write-cleanup cgroup-v2-profile\nnot-proven: resource-enforcement network credentials wall-time output-limit full-isolation\n"
				wantCalls = append(wantCalls,
					effect{op: "open", path: "/sys/fs/cgroup/cpu.max"},
					effect{op: "open", path: "/sys/fs/cgroup/memory.max"},
					effect{op: "open", path: "/sys/fs/cgroup/memory.swap.max"},
					effect{op: "open", path: "/sys/fs/cgroup/pids.max"},
					effect{op: "open", path: "/ci-trust/root-marker"},
					effect{op: "open", path: "/proc/self/status"},
					effect{op: "open", path: "/ci-trust/root-marker", flags: os.O_RDWR},
					effect{op: "open", path: "/proc/self/status"},
					effect{op: "open", path: "/proc/self/mounts"},
					effect{op: "open", path: "/tmp/ci-trust-owned-smoke-v1", flags: os.O_WRONLY | os.O_CREATE | os.O_EXCL, mode: 0600},
					effect{op: "lstat", path: "/tmp/ci-trust-owned-smoke-v1"},
					effect{op: "same"},
					effect{op: "remove", path: "/tmp/ci-trust-owned-smoke-v1"})
				wantCloses["/proc/self/status"] = 3
				wantCloses["/proc/self/mounts"] = 2
				for _, path := range []string{"/sys/fs/cgroup/cpu.max", "/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory.swap.max", "/sys/fs/cgroup/pids.max", "/ci-trust/root-marker"} {
					wantCloses[path] = 1
				}
			}
			var stdout, stderr bytes.Buffer
			exit := Run([]string{"--fixture=smoke-v1", "--config=owned-v1"}, s, &stdout, &stderr)
			if exit != wantExit || stdout.String() != wantOut || stderr.String() != wantErr {
				t.Errorf("exit/stdout/stderr = %d/%q/%q; want %d/%q/%q", exit, stdout.String(), stderr.String(), wantExit, wantOut, wantErr)
			}
			effects(t, m, wantCalls)
			if uidCalls != wantUID || s.owned != nil || len(m.files) != len(before) {
				t.Errorf("uid calls/ownership/storage size = %d/%v/%d", uidCalls, s.owned, len(m.files))
			}
			for path, data := range before {
				f := m.files[path]
				if f == nil || !bytes.Equal(f.data, data) || f.writes != 0 || f.closes != wantCloses[path] {
					t.Errorf("unexpected descriptor/storage effects for %s: %#v", path, f)
				}
			}
			wantScratch := 0
			if !unavailableMounts {
				wantScratch = 1
			}
			if m.created.writes != wantScratch || m.created.closes != wantScratch || (!unavailableMounts && string(m.created.data) != "owned-smoke-v1 scratch\n") || (unavailableMounts && len(m.created.data) != 0) {
				t.Errorf("scratch write/close/data = %d/%d/%q", m.created.writes, m.created.closes, m.created.data)
			}
		})
	}
}

func TestAdapterGuardBeforeMutations(t *testing.T) {
	for _, name := range []string{"non-linux", "root", "unsafe-status"} {
		t.Run(name, func(t *testing.T) {
			s, m := adapter()
			var want []effect
			switch name {
			case "non-linux":
				s.osName = "windows"
			case "root":
				s.uid = func() int { return 0 }
			case "unsafe-status":
				m.files["/proc/self/status"].data = []byte("NoNewPrivs:\t0\n")
				want = []effect{{op: "open", path: "/proc/self/status"}, {op: "open", path: "/proc/self/status"}}
			}
			if s.Write("/ci-trust/root-marker", []byte("ci-trust owned-smoke-v1 root marker write check\n")) == nil {
				t.Error("unsafe write accepted")
			}
			if owned, err := s.Create("/tmp/ci-trust-owned-smoke-v1", []byte("owned-smoke-v1 scratch\n")); owned || err == nil {
				t.Error("unsafe create accepted")
			}
			effects(t, m, want)
			if string(m.files["/ci-trust/root-marker"].data) != "ci-trust owned-smoke-v1 root marker\n" || m.created.writes != 0 {
				t.Error("unsafe mutation")
			}
		})
	}
}

func TestAdapterFixedBoundedReads(t *testing.T) {
	for _, tc := range []struct {
		name, path string
		bound      int
		content    []byte
		mode       fs.FileMode
		opens      bool
		ok         bool
	}{
		{"unknown", "/credentials-forbidden", 128, nil, 0, false, false},
		{"wrong-bound", "/proc/self/status", 9000, nil, 0, false, false},
		{"exact-bound", "/proc/self/status", 8192, bytes.Repeat([]byte("x"), 8192), 0, true, true},
		{"over-bound", "/proc/self/status", 8192, bytes.Repeat([]byte("x"), 8193), 0, true, false},
		{"not-regular", "/proc/self/status", 8192, nil, fs.ModeNamedPipe, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, m := adapter()
			f := &memoryFile{data: tc.content, mode: tc.mode}
			m.files[tc.path] = f
			data, err := s.Read(tc.path, tc.bound)
			if (err == nil) != tc.ok || (tc.ok && !bytes.Equal(data, tc.content)) || (!tc.ok && data != nil) {
				t.Errorf("read = %d bytes, %v", len(data), err)
			}
			var want []effect
			if tc.opens {
				want = []effect{{op: "open", path: tc.path}}
			}
			effects(t, m, want)
			if f.closes != boolInt(tc.opens) || f.writes != 0 {
				t.Error("descriptor effects")
			}
		})
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestAdapterMarkerRestorationAndSpecificEROFS(t *testing.T) {
	for _, name := range []string{"erofs", "eacces", "writable", "partial-write", "post-open-erofs", "restore-failure", "close-failure", "foreign"} {
		t.Run(name, func(t *testing.T) {
			s, m := adapter()
			f := m.files["/ci-trust/root-marker"]
			switch name {
			case "erofs":
				m.openErr["/ci-trust/root-marker"] = syscall.EROFS
			case "eacces":
				m.openErr["/ci-trust/root-marker"] = syscall.EACCES
			case "partial-write":
				f.shortFirst = true
			case "post-open-erofs":
				f.failWriteAt, f.writeErr = 1, syscall.EROFS
			case "restore-failure":
				f.failWriteAt, f.writeErr = 2, syscall.EIO
			case "close-failure":
				f.closeErr = syscall.EIO
			case "foreign":
				f.data = []byte("foreign")
			}
			err := s.Write("/ci-trust/root-marker", []byte("ci-trust owned-smoke-v1 root marker write check\n"))
			if errors.Is(err, syscall.EROFS) != (name == "erofs") || (err == nil) != (name == "writable") {
				t.Errorf("write error = %v", err)
			}
			effects(t, m, []effect{{op: "open", path: "/proc/self/status"}, {op: "open", path: "/ci-trust/root-marker", flags: os.O_RDWR}})
			wantData, writes, closes := "ci-trust owned-smoke-v1 root marker\n", 0, 0
			if name == "writable" || name == "partial-write" || name == "post-open-erofs" || name == "restore-failure" || name == "close-failure" {
				writes, closes = 2, 1
			}
			if name == "restore-failure" {
				wantData = "ci-trust owned-smoke-v1 root marker write check\n"
			}
			if name == "foreign" {
				wantData, closes = "foreign", 1
			}
			if string(f.data) != wantData || f.writes != writes || f.closes != closes || m.created.writes != 0 {
				t.Error("marker/storage/close effects differ")
			}
		})
	}
}

func TestAdapterExclusiveOwnershipAndCleanup(t *testing.T) {
	for _, name := range []string{"success", "partial-write", "close-failure", "foreign-collision", "replaced-before-cleanup"} {
		t.Run(name, func(t *testing.T) {
			s, m := adapter()
			switch name {
			case "partial-write":
				m.created.shortFirst = true
			case "close-failure":
				m.created.closeErr = errors.New("close failed")
			case "foreign-collision":
				m.files["/tmp/ci-trust-owned-smoke-v1"] = &memoryFile{data: []byte("foreign")}
			}
			owned, err := s.Create("/tmp/ci-trust-owned-smoke-v1", []byte("owned-smoke-v1 scratch\n"))
			if owned != (name != "foreign-collision") || (err == nil) != (name == "success" || name == "replaced-before-cleanup") {
				t.Errorf("create = %v, %v", owned, err)
			}
			want := []effect{{op: "open", path: "/proc/self/status"}, {op: "open", path: "/proc/self/mounts"}, {op: "open", path: "/tmp/ci-trust-owned-smoke-v1", flags: os.O_WRONLY | os.O_CREATE | os.O_EXCL, mode: 0600}}
			if name == "replaced-before-cleanup" {
				m.files["/tmp/ci-trust-owned-smoke-v1"] = &memoryFile{data: []byte("replacement")}
			}
			cleanup := s.Remove("/tmp/ci-trust-owned-smoke-v1")
			if (cleanup == nil) != (owned && name != "replaced-before-cleanup") {
				t.Errorf("cleanup = %v", cleanup)
			}
			if owned {
				want = append(want, effect{op: "lstat", path: "/tmp/ci-trust-owned-smoke-v1"}, effect{op: "same"})
				if name != "replaced-before-cleanup" {
					want = append(want, effect{op: "remove", path: "/tmp/ci-trust-owned-smoke-v1"})
				}
			}
			effects(t, m, want)
			remaining := m.files["/tmp/ci-trust-owned-smoke-v1"]
			if name == "foreign-collision" {
				if string(remaining.data) != "foreign" {
					t.Error("foreign collision altered")
				}
			} else if name == "replaced-before-cleanup" {
				if string(remaining.data) != "replacement" {
					t.Error("replacement removed")
				}
			} else if remaining != nil || s.owned != nil {
				t.Error("owned scratch leaked")
			}
			if m.created.writes != boolInt(owned) || m.created.closes != boolInt(owned) {
				t.Error("write/close counts")
			}
			if string(m.files["/ci-trust/root-marker"].data) != "ci-trust owned-smoke-v1 root marker\n" {
				t.Error("root marker changed")
			}
		})
	}
}
