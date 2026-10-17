package lintpreparation

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// MaterializeNodeBundle writes only sealed data into a new child of destination.
// The caller must supply an existing, private, empty, owned directory on a trusted
// host. OpenRoot follows the root pathname: acquisition is not protection against
// malicious concurrent root replacement. Root does not provide mount/device/host
// isolation. No process is started. POSIX owner-mode guarantees require Linux;
// Windows fixture success establishes neither ACL parity nor runtime admission.
func MaterializeNodeBundle(ctx context.Context, bundle *NodeBundle, destination string) (string, error) {
	invalid := func() (string, error) { return "", NewPreparationFailure("node-extract", "validation", "") }
	if bundle == nil || !bundle.sealed || len(bundle.entries) == 0 || len(bundle.entries) > MaxFiles {
		return invalid()
	}
	if err := baseContextFailure(ctx, "node-extract"); err != nil {
		return "", err
	}
	if runtime.GOOS == "js" || runtime.GOOS == "plan9" {
		return "", nodeGraphFailure("platform")
	}
	// Defense in depth, not a second graph validator or authority for raw data.
	for _, e := range bundle.entries {
		if err := baseContextFailure(ctx, "node-extract"); err != nil {
			return "", err
		}
		name, err := SafePath(e.name)
		if err != nil || name != e.name || strings.Contains(name, ":") || (name != nodeBundlePrefix && !strings.HasPrefix(name, nodeBundlePrefix+"/")) {
			return invalid()
		}
		if (name == nodeBundlePrefix && e.kind != tar.TypeDir) || (e.kind != tar.TypeDir && e.kind != tar.TypeReg && e.kind != tar.TypeSymlink) {
			return invalid()
		}
	}
	st, err := os.Lstat(destination)
	if err != nil {
		return "", nodeMaterializeFilesystem()
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return invalid()
	}
	parent, err := os.OpenRoot(destination)
	if err != nil {
		return "", nodeMaterializeFilesystem()
	}
	defer parent.Close()
	dir, err := parent.Open(".")
	if err != nil {
		return "", nodeMaterializeFilesystem()
	}
	entries, readErr := dir.ReadDir(1)
	closeErr := dir.Close()
	if closeErr != nil || (readErr != nil && readErr != io.EOF) {
		return "", nodeMaterializeFilesystem()
	}
	if len(entries) != 0 {
		return invalid()
	}
	if err := baseContextFailure(ctx, "node-extract"); err != nil {
		return "", err
	}
	// Mkdir is the ownership claim. A collision never authorizes cleanup.
	if err := parent.Mkdir(nodeBundlePrefix, 0700); err != nil {
		return "", nodeMaterializeFilesystem()
	}
	child, err := parent.OpenRoot(nodeBundlePrefix)
	if err == nil {
		err = writeNodeBundle(ctx, child, bundle)
		if closeErr := child.Close(); err == nil && closeErr != nil {
			err = nodeMaterializeFilesystem()
		}
	} else {
		err = nodeMaterializeFilesystem()
	}
	if err != nil {
		// Never delete destination, ancestors, or a preexisting child. A cleanup
		// failure remains failure, with no successful path or unchanged claim.
		if cleanupErr := parent.RemoveAll(nodeBundlePrefix); cleanupErr != nil {
			return "", nodeMaterializeFilesystem()
		}
		return "", err
	}
	return filepath.Join(destination, nodeBundlePrefix), nil
}

func nodeMaterializeFilesystem() error {
	return NewPreparationFailure("node-extract", "filesystem", "")
}

func writeNodeBundle(ctx context.Context, root *os.Root, bundle *NodeBundle) error {
	// Physical parent directories precede children in the sealed sorted layout.
	// Symlinks are last: they cannot redirect directory/file construction.
	for _, kind := range []byte{tar.TypeDir, tar.TypeReg, tar.TypeSymlink} {
		for _, e := range bundle.entries {
			if err := baseContextFailure(ctx, "node-extract"); err != nil {
				return err
			}
			if e.kind != kind || e.name == nodeBundlePrefix {
				continue
			}
			name := strings.TrimPrefix(e.name, nodeBundlePrefix+"/")
			switch kind {
			case tar.TypeDir:
				if err := root.Mkdir(name, 0700); err != nil {
					return nodeMaterializeFilesystem()
				}
			case tar.TypeReg:
				if err := writeNodeFile(ctx, root, name, e); err != nil {
					return err
				}
			case tar.TypeSymlink:
				// Native separators preserve component semantics; do not Clean the
				// target. Linux retains the original TAR spelling verbatim.
				if err := root.Symlink(filepath.FromSlash(e.target), name); err != nil {
					return nodeMaterializeFilesystem()
				}
			}
		}
	}
	return baseContextFailure(ctx, "node-extract")
}

func writeNodeFile(ctx context.Context, root *os.Root, name string, e nodeBundleEntry) (err error) {
	file, openErr := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if openErr != nil {
		return nodeMaterializeFilesystem()
	}
	defer func() {
		if closeErr := file.Close(); err == nil && closeErr != nil {
			err = nodeMaterializeFilesystem()
		}
	}()
	for data := e.data; len(data) > 0; {
		if err := baseContextFailure(ctx, "node-extract"); err != nil {
			return err
		}
		n := min(len(data), 32<<10)
		written, writeErr := file.Write(data[:n])
		if writeErr != nil || written != n {
			return nodeMaterializeFilesystem()
		}
		data = data[n:]
	}
	if err := baseContextFailure(ctx, "node-extract"); err != nil {
		return err
	}
	mode := os.FileMode(0600)
	if e.ownerExecutable {
		mode = 0700
	}
	// File-handle Chmod avoids path-based Unix symlink races.
	if chmodErr := file.Chmod(mode); chmodErr != nil {
		return nodeMaterializeFilesystem()
	}
	return nil
}
