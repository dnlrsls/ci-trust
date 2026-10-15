//go:build !linux

package ownedprobe

import "io/fs"

func openNative(string, int, fs.FileMode) (file, error) {
	return nil, unavailable
}
