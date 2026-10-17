package lintpreparation

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"path/filepath"
)

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
