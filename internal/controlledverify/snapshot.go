package controlledverify

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

var sourceUnavailable = errors.New("owned source snapshot unavailable")

// sourceSnapshot measures live owned source bytes, paths and modes. Git-owned
// caches/artifacts are excluded; links, special files and unstable reads fail
// closed. It is not a scanner for arbitrary audited repositories.
func sourceSnapshot(root string) (string, error) {
	info, err := os.Lstat(root)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", sourceUnavailable
	}
	h := sha256.New()
	var files int
	var total int64
	put := func(n uint64) { var b [8]byte; binary.BigEndian.PutUint64(b[:], n); _, _ = h.Write(b[:]) }
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return sourceUnavailable
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return sourceUnavailable
		}
		if rel == "." {
			return nil
		}
		if rel == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		files++
		if files > 10000 {
			return sourceUnavailable
		}
		if d.Type()&os.ModeSymlink != 0 {
			return sourceUnavailable
		}
		before, err := d.Info()
		if err != nil {
			return sourceUnavailable
		}
		name := filepath.ToSlash(rel)
		put(uint64(len(name)))
		_, _ = h.Write([]byte(name))
		put(uint64(before.Mode()))
		if d.IsDir() {
			put(0)
			return nil
		}
		if !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > 8<<20 {
			return sourceUnavailable
		}
		total += before.Size()
		if total > 128<<20 {
			return sourceUnavailable
		}
		put(uint64(before.Size()))
		f, err := os.Open(path)
		if err != nil {
			return sourceUnavailable
		}
		opened, statErr := f.Stat()
		if statErr != nil || !os.SameFile(before, opened) {
			f.Close()
			return sourceUnavailable
		}
		n, copyErr := io.Copy(h, io.LimitReader(f, (8<<20)+1))
		after, statErr := f.Stat()
		closeErr := f.Close()
		if copyErr != nil || statErr != nil || closeErr != nil || n != before.Size() || !os.SameFile(before, after) || before.Mode() != after.Mode() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			return sourceUnavailable
		}
		return nil
	})
	if err != nil {
		return "", sourceUnavailable
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
