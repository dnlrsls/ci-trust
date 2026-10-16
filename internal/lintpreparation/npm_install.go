package lintpreparation

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type npmInstallProxy interface {
	Port() int
	Denied() bool
	Close() error
}
type npmInstallPhase func(context.Context, string, string, string, string, int) error

// installNPM requires existing private absolute roots, an authenticated Node
// bundle, an owned manifest-only target and an empty home. Root acquisition is
// trusted: this is not a hostile-root race, mount or isolation boundary. The
// caller owns whole-temp cleanup; no rollback/atomic installation is promised.
func installNPM(ctx context.Context, nodeRoot, target, home string) ([]string, []byte, error) {
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return nil, nil, NewPreparationFailure("npm-setup", "validation", "")
	}
	return installNPMWith(ctx, nodeRoot, target, home, func(ctx context.Context) (npmInstallProxy, error) { return newRegistryServer(ctx) }, runNPMPhase, MaxBytes)
}

// Only the private fixture seam bypasses the platform guard. No caller-selected
// origin, executable, host configuration or listener is exposed by installNPM.
func installNPMWith(ctx context.Context, nodeRoot, target, home string, factory func(context.Context) (npmInstallProxy, error), phase npmInstallPhase, limit int64) (flags []string, raw []byte, err error) {
	err = InStage("npm-setup", func() error {
		if e := baseContextFailure(ctx, "npm-setup"); e != nil {
			return e
		}
		if !filepath.IsAbs(nodeRoot) || !filepath.IsAbs(target) || !filepath.IsAbs(home) || factory == nil || phase == nil || limit < 1 || limit > MaxBytes {
			return NewValidationFailure("")
		}
		nr, e := os.OpenRoot(nodeRoot)
		if e != nil {
			return e
		}
		defer nr.Close()
		tr, e := os.OpenRoot(target)
		if e != nil {
			return e
		}
		defer tr.Close()
		hr, e := os.OpenRoot(home)
		if e != nil {
			return e
		}
		defer hr.Close()
		bundled, e := readNPMJSON(nr, "lib/node_modules/npm/package.json", limit)
		if e != nil {
			return e
		}
		if e = validateBundledNPM(bundled); e != nil {
			return e
		}
		manifest, e := readNPMJSON(tr, "package.json", limit)
		if e != nil {
			return e
		}
		if e = ValidateOwnedManifest(manifest); e != nil {
			return e
		}
		if e = npmDirectoryOnly(tr, "package.json"); e != nil {
			return e
		}
		if e = npmDirectoryOnly(hr, ""); e != nil {
			return e
		}
		// Exclusive creation never overwrites host or repository configuration. A
		// mid-setup failure may leave only these owned new files for caller cleanup.
		for _, name := range []string{"user.npmrc", "global.npmrc"} {
			if e := baseContextFailure(ctx, "npm-setup"); e != nil {
				return e
			}
			f, e := hr.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return e
			}
			if e = f.Close(); e != nil {
				return e
			}
		}
		if e := baseContextFailure(ctx, "npm-setup"); e != nil {
			return e
		}
		proxy, e := factory(ctx)
		if e != nil {
			return e
		}
		return func() (result error) {
			// Always close outside handlers. An existing phase/read failure wins over
			// close failure; otherwise close failure revokes success, redacted network.
			defer func() {
				if e := proxy.Close(); e != nil && result == nil {
					result = NewPreparationFailure("npm-setup", "network", "")
				}
			}()
			port := proxy.Port()
			if port < 1 || port > 65535 {
				return NewPreparationFailure("npm-setup", "network", "")
			}
			var frozen []byte
			for _, stage := range []string{"npm-lock", "npm-ci"} {
				e := InStage(stage, func() error {
					if e := baseContextFailure(ctx, stage); e != nil {
						return e
					}
					e := phase(ctx, stage, nodeRoot, target, home, port)
					var failure *PreparationFailure
					completed := e == nil || errors.As(e, &failure) && failure.Category == "process-failed"
					// Returned process failures represent completed subprocesses. Timeout,
					// filesystem and cancellation-equivalent exceptions bypass Denied.
					if completed && proxy.Denied() {
						return NewPreparationFailure(stage, "origin-denied", "")
					}
					if e != nil {
						if errors.Is(e, context.Canceled) {
							return NewPreparationFailure(stage, "network", "")
						}
						return e
					}
					if proxy.Port() != port {
						return NewPreparationFailure(stage, "network", "")
					}
					lock, e := readNPMJSON(tr, "package-lock.json", limit)
					if e != nil {
						return e
					}
					if e = ValidateNPMLock(lock); e != nil {
						return e
					}
					if frozen != nil && !bytes.Equal(frozen, lock) {
						return NewValidationFailure("")
					}
					if frozen == nil {
						frozen = lock
					}
					return nil
				})
				if e != nil {
					return e
				}
			}
			raw = frozen
			return nil
		}()
	})
	if err != nil {
		return nil, nil, err
	}
	return npmCommonFlags(home), raw, nil
}

func npmDirectoryOnly(root *os.Root, name string) error {
	f, e := root.Open(".")
	if e != nil {
		return e
	}
	defer f.Close()
	entries, e := f.ReadDir(2)
	if e != nil && !errors.Is(e, io.EOF) {
		return e
	}
	if name == "" && len(entries) == 0 || name != "" && len(entries) == 1 && entries[0].Name() == name {
		return nil
	}
	return NewValidationFailure("")
}

// Root confines intermediate links. Reject final links/nonregular objects before
// opening and stat the acquired handle too; trusted private roots exclude races.
func readNPMJSON(root *os.Root, name string, limit int64) ([]byte, error) {
	st, e := root.Lstat(name)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, NewValidationFailure("")
	}
	f, e := root.Open(name)
	if e != nil {
		return nil, e
	}
	st, e = f.Stat()
	if e != nil {
		f.Close()
		return nil, e
	}
	if !st.Mode().IsRegular() {
		f.Close()
		return nil, NewValidationFailure("")
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	closeErr := f.Close()
	if e != nil {
		return nil, e
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if int64(len(b)) > limit {
		return nil, NewValidationFailure("")
	}
	return b, nil
}
