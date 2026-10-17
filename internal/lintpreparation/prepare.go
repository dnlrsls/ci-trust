package lintpreparation

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type preparationEffects struct {
	acquire   func(context.Context, string) (*acquiredPreparation, error)
	base      func(context.Context) (Tree, map[string]any, error)
	rootfs    func(context.Context, Tree, *NodeBundle, string, []byte, []byte) (Tree, error)
	artifacts func(context.Context, artifactRequest) (map[string][]byte, error)
}

func fixedPreparationEffects() preparationEffects {
	return preparationEffects{acquirePreparation, LoadBase, buildOwnedRootfs, buildArtifacts}
}

// Prepare builds preparation-only artifacts under the trusted, stable workspace's
// .git/lint-image-preparation directory. It requires Linux/amd64 and fixed pinned
// dependencies; no host credentials/configuration are inherited by npm. Source
// and run metadata are recorded as supplied, not independently authenticated.
// Success is NOT runtime admission, tool compatibility or independent reproduction.
// Owned temp cleanup is checked; filesystem writes are not an atomic job rollback.
func Prepare(ctx context.Context, workspace, sourceRevision, runID, runAttempt string) error {
	return prepareWith(ctx, workspace, sourceRevision, runID, runAttempt, fixedPreparationEffects())
}

// Private effects isolate orchestration tests; production never accepts overrides.
func prepareWith(ctx context.Context, workspace, sourceRevision, runID, runAttempt string, d preparationEffects) (err error) {
	run := func(stage string, f func() error) error {
		return InStage(stage, func() error {
			if e := baseContextFailure(ctx, stage); e != nil {
				return e
			}
			return f()
		})
	}
	var s *acquiredPreparation
	err = run("host-inputs", func() error { var e error; s, e = d.acquire(ctx, workspace); return e })
	if err != nil {
		return err
	}
	// Acquisition transfers all owned roots/temp here. Cleanup is independent of
	// cancellation; primary inner-stage failure wins diagnostic precedence.
	defer func() { err = errors.Join(err, s.finish()) }()
	var base, tree Tree
	var baseManifest map[string]any
	err = run("base-image", func() error { var e error; base, baseManifest, e = d.base(ctx); return e })
	if err != nil {
		return err
	}
	var launcher []byte
	err = run("rootfs", func() error {
		var e error
		launcher, e = s.fs.launcher(ctx, MaxBytes)
		if e != nil {
			staged := InStage("rootfs", func() error { return e })
			var failure *PreparationFailure
			if errors.As(staged, &failure) && failure.Reason == "" {
				return NewPreparationFailure(failure.Stage, failure.Category, "rootfs-launcher")
			}
			return staged
		}
		tree, e = d.rootfs(ctx, base, s.bundle, s.target, launcher, s.inputs.module)
		return e
	})
	if err != nil {
		return err
	}
	var payloads map[string][]byte
	// The accepted builder preserves its own image-validation/artifact-write inner
	// stages. Launcher bytes are exactly those consumed by assembly, never reread.
	err = run("image-validation", func() error {
		var e error
		payloads, e = d.artifacts(ctx, artifactRequest{
			tree: tree, manifest: s.manifest, lock: s.lock, launcher: launcher,
			ownedInputs: s.inputs.owned, baseManifest: baseManifest, npmFlags: s.flags,
			sourceRevision: sourceRevision, runID: runID, runAttempt: runAttempt,
		})
		return e
	})
	if err != nil {
		return err
	}
	return run("artifact-write", func() error { return s.fs.writeArtifacts(ctx, payloads) })
}

// Acquisition effects are private fixture seams, never caller configuration.
type acquisitionEffects struct {
	goos, goarch string
	open         func(string) (*preparationFS, error)
	download     func(context.Context, string, string, string) ([]byte, error)
	decode       func(context.Context, []byte) (*NodeBundle, error)
	materialize  func(context.Context, *NodeBundle, string) (string, error)
	install      func(context.Context, string, string, string) ([]string, []byte, error)
	finish       func(*preparationFS) error
}
type acquiredPreparation struct {
	fs                           *preparationFS
	inputs                       preparationInputs
	bundle                       *NodeBundle
	temp, nodeRoot, target, home string
	manifest, lock               []byte
	flags                        []string
	close                        func(*preparationFS) error
}

// Success transfers ownership to C2b, which MUST defer finish on every return.
// Failure transfers nothing. Neither success nor this state is runtime admission.
func (s *acquiredPreparation) finish() error {
	return InStage("host-inputs", func() error { return s.close(s.fs) })
}
func fixedAcquisitionEffects() acquisitionEffects {
	return acquisitionEffects{runtime.GOOS, runtime.GOARCH, openPreparationFS, Download, DecodeNodeBundle, MaterializeNodeBundle, installNPM, (*preparationFS).finish}
}
func acquirePreparation(ctx context.Context, workspace string) (*acquiredPreparation, error) {
	return acquirePreparationWith(ctx, workspace, fixedAcquisitionEffects())
}

// Roots are caller-trusted/stable; no hostile-root, mount or host isolation.
// Private synthetic effects are never exposed as public host/pin/process knobs.
func acquirePreparationWith(ctx context.Context, workspace string, d acquisitionEffects) (result *acquiredPreparation, err error) {
	s := &acquiredPreparation{close: d.finish}
	run := func(stage string, f func() error) error {
		return InStage(stage, func() error {
			if e := baseContextFailure(ctx, stage); e != nil {
				return e
			}
			return f()
		})
	}
	err = run("host-inputs", func() error {
		if d.goos != "linux" || d.goarch != "amd64" {
			return NewValidationFailure("platform")
		}
		if d.open == nil || d.download == nil || d.decode == nil || d.materialize == nil || d.install == nil || d.finish == nil {
			return NewValidationFailure("")
		}
		// Resolve only syntactically; no ambient credentials/configuration are read.
		workspace, e := filepath.Abs(workspace)
		if e != nil {
			return e
		}
		s.fs, e = d.open(workspace)
		return e
	})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, s.finish())
			result = nil
		}
	}()
	err = run("host-inputs", func() error {
		var e error
		s.inputs, e = s.fs.inputs(ctx, MaxBytes)
		if e != nil {
			return e
		}
		s.temp, e = s.fs.claimTemp(ctx)
		return e
	})
	if err != nil {
		return nil, err
	}
	var archive []byte
	err = run("node-download", func() error {
		var e error
		archive, e = d.download(ctx, "node-download", "https://nodejs.org/dist/v"+NodeVersion+"/"+NodeArchive, "sha256:"+NodeSHA256)
		return e
	})
	if err != nil {
		return nil, err
	}
	err = run("node-extract", func() error {
		var e error
		s.bundle, e = d.decode(ctx, archive)
		if e != nil {
			return e
		}
		if e = baseContextFailure(ctx, "node-extract"); e != nil {
			return e
		}
		s.nodeRoot, e = d.materialize(ctx, s.bundle, s.temp)
		return e
	})
	if err != nil {
		return nil, err
	}
	err = run("owned-manifest", func() (e error) {
		s.manifest, e = s.fs.manifest(ctx, MaxBytes)
		if e != nil {
			return e
		}
		r, e := s.fs.prep.OpenRoot(s.fs.temp)
		if e != nil {
			return e
		}
		defer func() { e = errors.Join(e, r.Close()) }()
		for _, name := range []string{"owned", "home"} {
			if e = baseContextFailure(ctx, "owned-manifest"); e != nil {
				return e
			}
			if e = r.Mkdir(name, 0700); e != nil {
				return e
			}
		}
		s.target = filepath.Join(s.temp, "owned")
		s.home = filepath.Join(s.temp, "home")
		return writePreparationFile(ctx, r, "owned/package.json", s.manifest)
	})
	if err != nil {
		return nil, err
	}
	err = run("npm-setup", func() error {
		var e error
		s.flags, s.lock, e = d.install(ctx, s.nodeRoot, s.target, s.home)
		return e
	})
	if err != nil {
		return nil, err
	}
	// Late cancellation must not hand off an apparently successful acquisition.
	if err = baseContextFailure(ctx, "npm-setup"); err != nil {
		return nil, err
	}
	return s, nil
}

var preparationArtifactNames = []string{"image.tar", "package.json", "package-lock.json", "inventory.json", "owned-inputs.json", "preparation.json", "SHA256SUMS"}

// Private IO only: C2 must admit Linux/amd64 and context before acquisition.
// Workspace and root acquisition are trusted/stable, not hostile rename, mount
// or isolation boundaries. Requested POSIX modes do not assert Windows ACLs.
// The owner must finish on every post-acquisition return. No concurrent use.
type preparationFS struct {
	prep, source *os.Root
	temp         string
	closed       bool
}
type preparationInputs struct {
	owned  map[string]any
	module []byte
}

func openPreparationFS(workspace string) (*preparationFS, error) {
	if !filepath.IsAbs(workspace) || filepath.Clean(workspace) != workspace {
		return nil, NewValidationFailure("")
	}
	p, e := os.OpenRoot(filepath.Join(workspace, ".git/lint-image-preparation"))
	if e != nil {
		return nil, e
	}
	s, e := os.OpenRoot(filepath.Join(workspace, "tools/lint-preparation"))
	if e != nil {
		return nil, errors.Join(e, p.Close())
	}
	return &preparationFS{prep: p, source: s}, nil
}
func (p *preparationFS) ready(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if p == nil || p.closed {
		return NewValidationFailure("")
	}
	return nil
}

// Lstat precedes opening to reject terminal links and nonregular entries before
// reads (notably FIFOs). The owned stable source precondition excludes replacement
// races; the opened handle is independently checked. One detection byte only.
func readPreparationFile(ctx context.Context, r *os.Root, name string, limit int64) (data []byte, err error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if limit < 1 || limit > MaxBytes {
		return nil, NewValidationFailure("")
	}
	st, e := r.Lstat(name)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() {
		return nil, NewValidationFailure("")
	}
	f, e := r.Open(name)
	if e != nil {
		return nil, e
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	st, e = f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Size() > limit {
		return nil, NewValidationFailure("")
	}
	data, e = io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil {
		return nil, e
	}
	if int64(len(data)) > limit {
		return nil, NewValidationFailure("")
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return data, nil
}

// Caller wraps this in host-inputs; manifest and launcher reads are deferred to
// owned-manifest and rootfs respectively. Full JSON is retained; executable provenance is a caller
// precondition, not inferred from these consistency checks.
func (p *preparationFS) inputs(ctx context.Context, limit int64) (preparationInputs, error) {
	var out preparationInputs
	if e := p.ready(ctx); e != nil {
		return out, e
	}
	raw, e := readPreparationFile(ctx, p.prep, "inputs.json", limit)
	if e != nil {
		return out, e
	}
	v, e := UniqueJSON(raw)
	if e != nil {
		return out, e
	}
	m, ok := v.(map[string]any)
	if !ok {
		return out, NewValidationFailure("")
	}
	module, ok := m["Module"].(string)
	hash, hok := m["ModuleSHA256"].(string)
	if !ok || !hok || hash != Digest([]byte(module))[7:] {
		return out, NewValidationFailure("")
	}
	return preparationInputs{m, []byte(module)}, nil
}

// Caller wraps in rootfs and passes these same retained bytes to assembly and
// artifact hashing, without rereading mutable launcher data.
func (p *preparationFS) launcher(ctx context.Context, limit int64) ([]byte, error) {
	if e := p.ready(ctx); e != nil {
		return nil, e
	}
	return readPreparationFile(ctx, p.prep, "owned-eslint", limit)
}
func (p *preparationFS) manifest(ctx context.Context, limit int64) ([]byte, error) {
	if e := p.ready(ctx); e != nil {
		return nil, e
	}
	b, e := readPreparationFile(ctx, p.source, "package.json", limit)
	if e != nil {
		return nil, e
	}
	if e = ValidateOwnedManifest(b); e != nil {
		return nil, e
	}
	return b, nil
}

// Fresh and empty for MaterializeNodeBundle; target/home belong to the later
// orchestration stage. A failed exclusive claim never acquires deletion rights.
func (p *preparationFS) claimTemp(ctx context.Context) (string, error) {
	if e := p.ready(ctx); e != nil {
		return "", e
	}
	if p.temp != "" {
		return "", NewValidationFailure("")
	}
	name := "owned-lint-" + rand.Text()
	if e := p.prep.Mkdir(name, 0700); e != nil {
		return "", e
	}
	p.temp = name
	return filepath.Join(p.prep.Name(), name), nil
}

// Cleanup ignores cancellation. Failures must revoke caller success. Only the
// exclusively claimed temp is removed; completed artifacts deliberately survive.
func (p *preparationFS) finish() error {
	if p == nil || p.closed {
		return nil
	}
	var e error
	if p.temp != "" {
		e = p.prep.RemoveAll(p.temp)
	}
	e = errors.Join(e, p.source.Close(), p.prep.Close())
	p.closed = true
	return e
}
func writePreparationFile(ctx context.Context, r *os.Root, name string, b []byte) (err error) {
	if e := ctx.Err(); e != nil {
		return e
	}
	f, e := r.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	for len(b) > 0 {
		if e = ctx.Err(); e != nil {
			return e
		}
		n := len(b)
		if n > 64<<10 {
			n = 64 << 10
		}
		written, e := f.Write(b[:n])
		if e != nil {
			return e
		}
		if written != n {
			return io.ErrShortWrite
		}
		b = b[n:]
	}
	return ctx.Err()
}

// Caller supplies immutable, already validated T7B bytes. This validates names,
// not image authority. No JSON/input cap applies to the differently bounded TAR.
// Claim only at write time. On failure remove ONLY our partial artifact dir;
// rollback is best-effort, checked and not atomic. Existing data is never removed.
// Caller wraps in artifact-write and preserves inner preparation stages.
func (p *preparationFS) writeArtifacts(ctx context.Context, payloads map[string][]byte) (err error) {
	if e := p.ready(ctx); e != nil {
		return e
	}
	if len(payloads) != len(preparationArtifactNames) {
		return NewValidationFailure("")
	}
	for _, n := range preparationArtifactNames {
		if _, ok := payloads[n]; !ok {
			return NewValidationFailure("")
		}
	}
	if e := p.prep.Mkdir("artifacts", 0700); e != nil {
		return e
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, p.prep.RemoveAll("artifacts"))
		}
	}()
	r, e := p.prep.OpenRoot("artifacts")
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, r.Close()) }()
	for _, n := range preparationArtifactNames {
		if e = writePreparationFile(ctx, r, n, payloads[n]); e != nil {
			return e
		}
	}
	return ctx.Err()
}
