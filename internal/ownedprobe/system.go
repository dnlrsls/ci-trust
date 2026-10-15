package ownedprobe

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"runtime"
)

var unavailable = errors.New("owned fixture unavailable")

// file and fileOps keep all adapter effects injectable, including open, close,
// identity checks and unlink. Tests never substitute real host procfs or files.
type file interface {
	io.Reader
	io.Writer
	io.Seeker
	Stat() (fs.FileInfo, error)
	Truncate(int64) error
	Close() error
}

type fileOps struct {
	open   func(string, int, fs.FileMode) (file, error)
	lstat  func(string) (fs.FileInfo, error)
	same   func(fs.FileInfo, fs.FileInfo) bool
	remove func(string) error
}

type nativeSystem struct {
	osName string
	uid    func() int
	ops    fileOps
	owned  fs.FileInfo
}

// NewSystem constructs but does not invoke the real adapter. Run validates
// arguments before invoking its operations. Non-Linux hosts fail closed.
func NewSystem() System {
	return &nativeSystem{osName: runtime.GOOS, uid: os.Getuid, ops: fileOps{
		open: openNative, lstat: os.Lstat, same: os.SameFile, remove: os.Remove,
	}}
}

func (s *nativeSystem) OS() string { return s.osName }
func (s *nativeSystem) UID() int   { return s.uid() }

func readBound(path string) int {
	switch path {
	case "/proc/self/status":
		return 8192
	case "/proc/self/cgroup":
		return 4096
	case "/proc/self/mounts":
		return 16384
	case "/sys/fs/cgroup/cpu.max", "/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory.swap.max", "/sys/fs/cgroup/pids.max", markerPath:
		return 128
	}
	return 0
}

func bounded(f file, limit int) ([]byte, error) {
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return nil, unavailable
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(limit)+1))
	if err != nil || len(data) > limit {
		return nil, unavailable
	}
	return data, nil
}

func (s *nativeSystem) Read(path string, limit int) ([]byte, error) {
	if s.osName != "linux" || limit == 0 || limit != readBound(path) {
		return nil, unavailable
	}
	f, err := s.ops.open(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	data, readErr := bounded(f, limit)
	return data, errors.Join(readErr, f.Close())
}

// Recheck the security guard inside each mutating adapter method as well as Run.
func (s *nativeSystem) guarded() bool {
	if s.osName != "linux" || s.uid() != 65534 {
		return false
	}
	data, err := s.Read("/proc/self/status", 8192)
	return err == nil && safeStatus(string(data))
}

func writeExact(f file, data []byte) error {
	n, err := f.Write(data)
	if err == nil && n != len(data) {
		return io.ErrShortWrite
	}
	return err
}

func replace(f file, data []byte) error {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	if err := writeExact(f, data); err != nil {
		return err
	}
	return f.Truncate(int64(len(data)))
}

func (s *nativeSystem) Write(path string, data []byte) error {
	if path != markerPath || (string(data) != changedMarker && string(data) != marker) || !s.guarded() {
		return unavailable
	}
	// No truncation on open. O_NOFOLLOW and regular-file validation prevent a
	// marker symlink/device from converting this into another kind of write.
	f, err := s.ops.open(path, os.O_RDWR, 0)
	if err != nil {
		return err
	} // Only an open EROFS may establish read-only.
	original, readErr := bounded(f, 128)
	if readErr != nil || string(original) != marker {
		f.Close()
		return unavailable
	}
	writeErr := replace(f, data)
	// Restore even after a partial write/truncate error, on this same descriptor.
	// Run also requests restoration on unexpected success; never touch foreigners.
	restoreErr := replace(f, []byte(marker))
	closeErr := f.Close()
	if writeErr != nil || restoreErr != nil || closeErr != nil {
		return unavailable
	}
	return nil
}

func (s *nativeSystem) Create(path string, data []byte) (bool, error) {
	if path != scratchPath || string(data) != scratchData || s.owned != nil || !s.guarded() {
		return false, unavailable
	}
	mounts, err := s.Read("/proc/self/mounts", 16384)
	if err != nil || !safeMount(string(mounts)) {
		return false, unavailable
	}
	f, err := s.ops.open(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false, err
	}
	// Ownership starts at successful exclusive open, not successful write/close.
	info, statErr := f.Stat()
	if statErr != nil || !info.Mode().IsRegular() {
		f.Close()
		return true, unavailable // No identity => Remove refuses unsafe cleanup.
	}
	s.owned = info
	writeErr := writeExact(f, data)
	return true, errors.Join(writeErr, f.Close())
}

func (s *nativeSystem) Remove(path string) error {
	if path != scratchPath || s.owned == nil {
		return unavailable
	}
	info, err := s.ops.lstat(path)
	if err != nil || !info.Mode().IsRegular() || !s.ops.same(s.owned, info) {
		return unavailable
	}
	if err := s.ops.remove(path); err != nil {
		return err
	}
	s.owned = nil
	return nil
}
