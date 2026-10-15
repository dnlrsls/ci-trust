//go:build linux

package ownedprobe

import (
	"io/fs"
	"os"
	"syscall"
)

func openNative(path string, flags int, mode fs.FileMode) (file, error) {
	return os.OpenFile(path, flags|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, mode)
}
