package lintpreparation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

const preparationManifest = `{"name":"ci-trust-owned-eslint-preparation","version":"0.0.0","private":true,"dependencies":{"eslint":"10.12.0"}}`

func preparationFixture(t *testing.T) (string, *preparationFS) {
	t.Helper()
	w := t.TempDir()
	for _, d := range []string{".git/lint-image-preparation/tmp", ".git/lint-image-preparation/cache", "tools/lint-preparation"} {
		if e := os.MkdirAll(filepath.Join(w, d), 0700); e != nil {
			t.Fatal(e)
		}
	}
	put := func(n, s string) {
		t.Helper()
		if e := os.WriteFile(filepath.Join(w, n), []byte(s), 0600); e != nil {
			t.Fatal(e)
		}
	}
	put(".git/lint-image-preparation/inputs.json", `{"Module":"x","ModuleSHA256":"`+Digest([]byte("x"))[7:]+`","extra":{"kept":true}}`)
	put(".git/lint-image-preparation/owned-eslint", "launcher")
	put("tools/lint-preparation/package.json", preparationManifest)
	p, e := openPreparationFS(w)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := p.finish(); e != nil {
			t.Error(e)
		}
	})
	return w, p
}

// Cancellation at an explicit IO checkpoint, without timing/races or fault probes.
type preparationCheckpoint struct {
	context.Context
	cancel    context.CancelFunc
	remaining int
}

func (c *preparationCheckpoint) Err() error {
	c.remaining--
	if c.remaining == 0 {
		c.cancel()
	}
	return c.Context.Err()
}
func TestPreparationFS(t *testing.T) {
	ctx := context.Background()
	t.Run("launcher deferred until rootfs", func(t *testing.T) {
		w, p := preparationFixture(t)
		// Replace the launcher with a nonregular terminal entry without deleting it.
		name := filepath.Join(w, ".git/lint-image-preparation/owned-eslint")
		if e := os.Rename(name, name+".saved"); e != nil {
			t.Fatal(e)
		}
		if _, e := p.inputs(ctx, 1024); e != nil {
			t.Fatal("missing launcher read in host-inputs", e)
		}
		if _, e := p.launcher(ctx, 1024); e == nil {
			t.Fatal("missing launcher accepted")
		}
		if e := os.Mkdir(name, 0700); e != nil {
			t.Fatal(e)
		}
		if _, e := p.inputs(ctx, 1024); e != nil {
			t.Fatal("invalid launcher read in host-inputs", e)
		}
		if _, e := p.launcher(ctx, 1024); e == nil {
			t.Fatal("nonregular launcher accepted")
		}
	})
	t.Run("inputs and deferred manifest", func(t *testing.T) {
		w, p := preparationFixture(t)
		in, e := p.inputs(ctx, 1024)
		if e != nil || string(in.module) != "x" || in.owned["extra"] == nil {
			t.Fatalf("inputs: %+v %v", in, e)
		}
		launcher, e := p.launcher(ctx, 1024)
		if e != nil || string(launcher) != "launcher" {
			t.Fatal("launcher payload", e)
		}
		if _, e = p.launcher(ctx, 2); e == nil {
			t.Fatal("launcher bound ignored")
		}
		b, e := p.manifest(ctx, 1024)
		if e != nil || string(b) != preparationManifest {
			t.Fatalf("manifest: %s %v", b, e)
		}
		if e = os.WriteFile(filepath.Join(w, "tools/lint-preparation/package.json"), []byte(`{}`), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e = p.inputs(ctx, 1024); e != nil {
			t.Fatal("manifest read too early", e)
		}
		if _, e = p.manifest(ctx, 1024); e == nil {
			t.Fatal("invalid manifest accepted")
		}
	})
	for _, bad := range []string{`{"Module":"x","ModuleSHA256":"bad"}`, `{"Module":1}`, `{"Module":"x","Module":"y"}`, `[]`} {
		t.Run(bad, func(t *testing.T) {
			w, p := preparationFixture(t)
			name := filepath.Join(w, ".git/lint-image-preparation/inputs.json")
			if e := os.WriteFile(name, []byte(bad), 0600); e != nil {
				t.Fatal(e)
			}
			if _, e := p.inputs(ctx, 1024); e == nil {
				t.Fatal("unsafe inputs accepted")
			}
			b, e := os.ReadFile(name)
			if e != nil || string(b) != bad {
				t.Fatal("input mutated")
			}
		})
	}
	t.Run("bounds cancellation and terminal types", func(t *testing.T) {
		w, p := preparationFixture(t)
		if _, e := p.inputs(ctx, 8); e == nil {
			t.Fatal("bound ignored")
		}
		c, cancel := context.WithCancel(ctx)
		cancel()
		if _, e := p.inputs(c, 1024); e == nil {
			t.Fatal("cancel ignored")
		}
		if _, e := p.launcher(c, 1024); e == nil {
			t.Fatal("cancelled launcher read")
		}
		if _, e := p.claimTemp(c); e == nil {
			t.Fatal("cancelled claim")
		}
		name := filepath.Join(w, ".git/lint-image-preparation/owned-eslint")
		// Required symlink proof; do not skip unsupported environments.
		link := filepath.Join(w, ".git/lint-image-preparation/link")
		if e := os.Symlink(name, link); e != nil {
			t.Fatal(e)
		}
		if _, e := readPreparationFile(ctx, p.prep, "link", 1024); e == nil {
			t.Fatal("symlink accepted")
		}
		if _, e := readPreparationFile(ctx, p.prep, "tmp", 1024); e == nil {
			t.Fatal("directory accepted")
		}
	})
	t.Run("empty exclusive temp cleanup and closed guard", func(t *testing.T) {
		w, p := preparationFixture(t)
		temp, e := p.claimTemp(ctx)
		if e != nil {
			t.Fatal(e)
		}
		entries, e := os.ReadDir(temp)
		if e != nil || len(entries) != 0 {
			t.Fatalf("not empty: %v %v", entries, e)
		}
		if _, e = p.claimTemp(ctx); e == nil {
			t.Fatal("second claim")
		}
		if e = os.WriteFile(filepath.Join(temp, "owned"), []byte("data"), 0600); e != nil {
			t.Fatal(e)
		}
		if e = p.finish(); e != nil {
			t.Fatal(e)
		}
		if _, e = os.Stat(temp); !os.IsNotExist(e) {
			t.Fatal("temp retained", e)
		}
		if _, e = os.Stat(filepath.Join(w, ".git/lint-image-preparation/tmp")); e != nil {
			t.Fatal("unowned tmp removed", e)
		}
		if _, e = p.inputs(ctx, 1024); e == nil {
			t.Fatal("closed read")
		}
		if e = p.finish(); e != nil {
			t.Fatal("idempotent close", e)
		}
	})
	t.Run("partial owned output cleanup and stage wrapping", func(t *testing.T) {
		w, p := preparationFixture(t)
		temp, e := p.claimTemp(ctx)
		if e != nil {
			t.Fatal(e)
		}
		payloads := map[string][]byte{}
		for _, n := range preparationArtifactNames {
			payloads[n] = []byte("synthetic")
		}
		c, cancel := context.WithCancel(ctx)
		defer cancel()
		checkpoint := &preparationCheckpoint{c, cancel, 3}
		e = InStage("artifact-write", func() error { return p.writeArtifacts(checkpoint, payloads) })
		var failure *PreparationFailure
		if !errors.As(e, &failure) || failure.Stage != "artifact-write" {
			t.Fatal("lost stage", e)
		}
		if _, e = os.Stat(filepath.Join(w, ".git/lint-image-preparation/artifacts")); !os.IsNotExist(e) {
			t.Fatal("partial owned artifacts retained", e)
		}
		if e = p.finish(); e != nil {
			t.Fatal(e)
		}
		if _, e = os.Stat(temp); !os.IsNotExist(e) {
			t.Fatal("failure temp retained", e)
		}
	})
	t.Run("preexisting artifacts sentinel", func(t *testing.T) {
		w, p := preparationFixture(t)
		dir := filepath.Join(w, ".git/lint-image-preparation/artifacts")
		if e := os.Mkdir(dir, 0700); e != nil {
			t.Fatal(e)
		}
		sentinel := filepath.Join(dir, "sentinel")
		if e := os.WriteFile(sentinel, []byte("untouched"), 0600); e != nil {
			t.Fatal(e)
		}
		payloads := map[string][]byte{}
		for _, n := range preparationArtifactNames {
			payloads[n] = nil
		}
		if e := p.writeArtifacts(ctx, payloads); e == nil {
			t.Fatal("existing directory claimed")
		}
		if e := p.finish(); e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(sentinel)
		if e != nil || string(b) != "untouched" {
			t.Fatal("sentinel changed", e)
		}
		entries, e := os.ReadDir(dir)
		if e != nil || len(entries) != 1 {
			t.Fatal("existing directory changed", entries, e)
		}
	})
	t.Run("artifact names bytes cancellation collision", func(t *testing.T) {
		w, p := preparationFixture(t)
		payloads := map[string][]byte{}
		for _, n := range preparationArtifactNames {
			payloads[n] = []byte(n)
		}
		before := map[string][]byte{}
		for n, b := range payloads {
			before[n] = append([]byte(nil), b...)
		}
		bad := map[string][]byte{"../outside": []byte("no")}
		if e := p.writeArtifacts(ctx, bad); e == nil {
			t.Fatal("invalid names")
		}
		c, cancel := context.WithCancel(ctx)
		cancel()
		if e := p.writeArtifacts(c, payloads); e == nil {
			t.Fatal("cancel accepted")
		}
		dir := filepath.Join(w, ".git/lint-image-preparation/artifacts")
		if _, e := os.Stat(dir); !os.IsNotExist(e) {
			t.Fatal("early artifact effects", e)
		}
		if e := p.writeArtifacts(ctx, payloads); e != nil {
			t.Fatal(e)
		}
		entries, e := os.ReadDir(dir)
		if e != nil || len(entries) != 7 {
			t.Fatal(entries, e)
		}
		for n, b := range payloads {
			got, e := os.ReadFile(filepath.Join(dir, n))
			if e != nil || !reflect.DeepEqual(got, b) {
				t.Fatal(n, e)
			}
		}
		if e = p.writeArtifacts(ctx, payloads); e == nil {
			t.Fatal("overwrite accepted")
		}
		if !reflect.DeepEqual(before, payloads) {
			t.Fatal("payload mutated")
		}
		if e = p.finish(); e != nil {
			t.Fatal(e)
		}
		for n, b := range payloads {
			got, e := os.ReadFile(filepath.Join(dir, n))
			if e != nil || !reflect.DeepEqual(got, b) {
				t.Fatal("published bytes removed", n, e)
			}
		}
	})
}
