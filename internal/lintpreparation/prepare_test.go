package lintpreparation

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestPrepare(t *testing.T) {
	ctx := context.Background()
	t.Run("fixed defaults without invoking production", func(t *testing.T) {
		d := fixedPreparationEffects()
		for _, pair := range [][2]any{{d.acquire, acquirePreparation}, {d.base, LoadBase}, {d.rootfs, buildOwnedRootfs}, {d.artifacts, buildArtifacts}} {
			if reflect.ValueOf(pair[0]).Pointer() != reflect.ValueOf(pair[1]).Pointer() {
				t.Fatal("fixed completion binding")
			}
		}
	})
	t.Run("cancel before acquisition and acquisition failure", func(t *testing.T) {
		c, cancel := context.WithCancel(ctx)
		cancel()
		calls := 0
		d := preparationEffects{acquire: func(context.Context, string) (*acquiredPreparation, error) {
			calls++
			return nil, NewPreparationFailure("node-download", "network", "")
		}}
		if e := prepareWith(c, "no workspace", "rev", "run", "attempt", d); e == nil || calls != 0 {
			t.Fatal("cancel before effects", e, calls)
		}
		e := prepareWith(ctx, "no workspace", "rev", "run", "attempt", d)
		var f *PreparationFailure
		if !errors.As(e, &f) || f.Stage != "node-download" || calls != 1 {
			t.Fatal("acquisition error changed", e, calls)
		}
	})
	for _, fail := range []string{"", "base", "base-inner", "launcher", "rootfs", "artifacts", "artifacts-inner", "output", "cancel", "output-cancel", "cleanup", "primary-cleanup"} {
		t.Run("completion/"+fail, func(t *testing.T) {
			w, p := preparationFixture(t)
			prep := filepath.Join(w, ".git/lint-image-preparation")
			inputBytes, e := os.ReadFile(filepath.Join(prep, "inputs.json"))
			if e != nil {
				t.Fatal(e)
			}
			var calls []string
			var s *acquiredPreparation
			closed := 0
			c, cancel := context.WithCancel(ctx)
			defer cancel()
			base := Tree{"base": Node{Kind: "file", Mode: 0444, Data: []byte("base")}}
			baseManifest := map[string]any{"layers": []any{map[string]any{"digest": "synthetic"}}, "extra": "retained"}
			lock := []byte("frozen-lock")
			flags := []string{"fixed flags"}
			bundle := &NodeBundle{sealed: true}
			output := map[string][]byte{}
			for _, n := range preparationArtifactNames {
				output[n] = []byte("synthetic payload: " + n)
			}
			sentinel := filepath.Join(prep, "artifacts/sentinel")
			if fail == "output" {
				if e = os.Mkdir(filepath.Dir(sentinel), 0700); e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(sentinel, []byte("preserved"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			inner := NewPreparationFailure("base-layer-diff", "validation", "layer-diff-id")
			d := preparationEffects{}
			d.acquire = func(_ context.Context, workspace string) (*acquiredPreparation, error) {
				calls = append(calls, "acquire")
				if workspace != w {
					t.Fatal("workspace binding")
				}
				in, e := p.inputs(ctx, 1024)
				if e != nil {
					t.Fatal(e)
				}
				temp, e := p.claimTemp(ctx)
				if e != nil {
					t.Fatal(e)
				}
				target := filepath.Join(temp, "owned")
				if e = os.Mkdir(target, 0700); e != nil {
					t.Fatal(e)
				}
				s = &acquiredPreparation{fs: p, inputs: in, bundle: bundle, temp: temp, nodeRoot: filepath.Join(temp, nodeBundlePrefix), target: target, manifest: []byte(preparationManifest), lock: lock, flags: flags}
				s.close = func(fs *preparationFS) error {
					calls = append(calls, "finish")
					closed++
					e := fs.finish()
					if fail == "cleanup" || fail == "primary-cleanup" {
						return errors.Join(e, os.ErrPermission)
					}
					return e
				}
				return s, nil
			}
			d.base = func(context.Context) (Tree, map[string]any, error) {
				calls = append(calls, "base")
				if fail == "base" || fail == "primary-cleanup" {
					return nil, nil, errors.New("private base detail")
				}
				if fail == "base-inner" {
					return nil, nil, inner
				}
				// A launcher can become available only after base retrieval: not eager.
				name := filepath.Join(prep, "owned-eslint")
				if fail == "launcher" {
					if e := os.Rename(name, name+".saved"); e != nil {
						t.Fatal(e)
					}
				} else if e := os.WriteFile(name, []byte("rootfs launcher"), 0600); e != nil {
					t.Fatal(e)
				}
				return base, baseManifest, nil
			}
			var assembled Tree
			d.rootfs = func(_ context.Context, b Tree, node *NodeBundle, target string, launcher, module []byte) (Tree, error) {
				calls = append(calls, "rootfs")
				if !reflect.DeepEqual(b, base) || node != bundle || target != s.target || string(launcher) != "rootfs launcher" || string(module) != "x" {
					t.Fatal("rootfs provenance/binding")
				}
				if fail == "rootfs" {
					return nil, errors.New("private rootfs detail")
				}
				// If completion rereads the mutable launcher, the artifact binding will fail.
				if e := os.WriteFile(filepath.Join(prep, "owned-eslint"), []byte("changed after assembly"), 0600); e != nil {
					t.Fatal(e)
				}
				assembled = Tree{"ci-trust/owned-eslint": Node{Kind: "file", Mode: 0555, Data: launcher}, "owned/eslint.config.mjs": Node{Kind: "file", Mode: 0444, Data: module}}
				if fail == "cancel" {
					cancel()
				}
				return assembled, nil
			}
			d.artifacts = func(_ context.Context, r artifactRequest) (map[string][]byte, error) {
				calls = append(calls, "artifacts")
				if !reflect.DeepEqual(r.tree, assembled) || !reflect.DeepEqual(r.manifest, s.manifest) || !reflect.DeepEqual(r.lock, lock) || string(r.launcher) != "rootfs launcher" || Digest(r.launcher) != Digest(assembled["ci-trust/owned-eslint"].Data) || !reflect.DeepEqual(r.ownedInputs, s.inputs.owned) || !reflect.DeepEqual(r.baseManifest, baseManifest) || !reflect.DeepEqual(r.npmFlags, flags) || r.sourceRevision != "source revision" || r.runID != "run id" || r.runAttempt != "attempt" {
					t.Fatal("artifact request bindings")
				}
				if fail == "artifacts" {
					return nil, errors.New("private image detail")
				}
				if fail == "artifacts-inner" {
					return nil, NewPreparationFailure("artifact-write", "validation", "")
				}
				if fail == "output-cancel" {
					cancel()
				}
				return output, nil
			}
			e = prepareWith(c, w, "source revision", "run id", "attempt", d)
			expected := []string{"acquire", "base", "rootfs", "artifacts", "finish"}
			stage := ""
			category := "unexpected"
			switch fail {
			case "base", "primary-cleanup":
				expected = []string{"acquire", "base", "finish"}
				stage = "base-image"
			case "base-inner":
				expected = []string{"acquire", "base", "finish"}
				stage = "base-layer-diff"
				category = "validation"
			case "launcher":
				expected = []string{"acquire", "base", "finish"}
				stage = "rootfs"
				category = "filesystem"
			case "rootfs":
				expected = []string{"acquire", "base", "rootfs", "finish"}
				stage = "rootfs"
			case "cancel":
				expected = []string{"acquire", "base", "rootfs", "finish"}
				stage = "image-validation"
				category = "network"
			case "artifacts":
				stage = "image-validation"
			case "artifacts-inner":
				stage = "artifact-write"
				category = "validation"
			case "output":
				stage = "artifact-write"
				category = "filesystem"
			case "output-cancel":
				stage = "artifact-write"
				category = "network"
			case "cleanup":
				stage = "host-inputs"
				category = "filesystem"
			}
			if !reflect.DeepEqual(calls, expected) {
				t.Fatalf("order: %v want %v error=%v", calls, expected, e)
			}
			if fail == "" {
				if e != nil {
					t.Fatal(e)
				}
			} else {
				var f *PreparationFailure
				if !errors.As(e, &f) || f.Stage != stage || f.Category != category {
					t.Fatalf("stage/category: %v want %s/%s", e, stage, category)
				}
				if strings.Contains(Diagnostic(e), "private") {
					t.Fatal("raw detail leaked")
				}
			}
			if closed != 1 || !p.closed {
				t.Fatal("handoff not finished", closed)
			}
			if _, e = os.Stat(s.temp); !os.IsNotExist(e) {
				t.Fatal("temp retained", e)
			}
			for _, r := range []*os.Root{p.prep, p.source} {
				if _, e = r.Stat("."); e == nil {
					t.Fatal("root retained")
				}
			}
			after, e := os.ReadFile(filepath.Join(prep, "inputs.json"))
			if e != nil || !reflect.DeepEqual(after, inputBytes) {
				t.Fatal("owned input mutated", e)
			}
			after, e = os.ReadFile(filepath.Join(w, "tools/lint-preparation/package.json"))
			if e != nil || string(after) != preparationManifest {
				t.Fatal("source manifest mutated", e)
			}
			if !reflect.DeepEqual(lock, []byte("frozen-lock")) || !reflect.DeepEqual(flags, []string{"fixed flags"}) || string(base["base"].Data) != "base" {
				t.Fatal("borrowed data mutated")
			}
			if fail == "" || fail == "cleanup" {
				entries, e := os.ReadDir(filepath.Join(prep, "artifacts"))
				if e != nil || len(entries) != 7 {
					t.Fatal("output set", entries, e)
				}
				for name, b := range output {
					got, e := os.ReadFile(filepath.Join(prep, "artifacts", name))
					if e != nil || !reflect.DeepEqual(got, b) {
						t.Fatal("output bytes", name, e)
					}
				}
			} else if fail == "output" {
				b, e := os.ReadFile(sentinel)
				if e != nil || string(b) != "preserved" {
					t.Fatal("preexisting output changed", e)
				}
				entries, e := os.ReadDir(filepath.Dir(sentinel))
				if e != nil || len(entries) != 1 {
					t.Fatal("preexisting output modified", entries, e)
				}
			} else if _, e = os.Stat(filepath.Join(prep, "artifacts")); !os.IsNotExist(e) {
				t.Fatal("unexpected artifacts", e)
			}
		})
	}
}

func TestAcquisition(t *testing.T) {
	ctx := context.Background()
	t.Run("fixed production bindings without execution", func(t *testing.T) {
		d := fixedAcquisitionEffects()
		if d.goos != runtime.GOOS || d.goarch != runtime.GOARCH {
			t.Fatal("production platform binding")
		}
		for _, pair := range [][2]any{{d.open, openPreparationFS}, {d.download, Download}, {d.decode, DecodeNodeBundle}, {d.materialize, MaterializeNodeBundle}, {d.install, installNPM}, {d.finish, (*preparationFS).finish}} {
			if reflect.ValueOf(pair[0]).Pointer() != reflect.ValueOf(pair[1]).Pointer() {
				t.Fatal("production helper binding")
			}
		}
	})
	t.Run("guards precede acquisition", func(t *testing.T) {
		for _, platform := range [][2]string{{"windows", "amd64"}, {"linux", "arm64"}, {"linux", "amd64"}} {
			c, cancel := context.WithCancel(ctx)
			if platform[0] == "linux" && platform[1] == "amd64" {
				cancel()
			}
			defer cancel()
			called := 0
			_, e := acquirePreparationWith(c, "not-an-absolute-workspace", acquisitionEffects{goos: platform[0], goarch: platform[1], open: func(string) (*preparationFS, error) { called++; return nil, nil }})
			if e == nil || called != 0 {
				t.Fatalf("guard: %v calls=%d", e, called)
			}
			var f *PreparationFailure
			if !errors.As(e, &f) || f.Stage != "host-inputs" {
				t.Fatal("guard stage", e)
			}
			if platform[0] == "linux" && platform[1] == "amd64" {
				if f.Category != "network" {
					t.Fatal("cancellation category", e)
				}
			} else if f.Reason != "platform" {
				t.Fatal("platform admission bypassed", e)
			}
		}
	})
	for _, fail := range []string{"", "module", "download", "decode", "materialize", "manifest", "install", "cancel", "cleanup", "handoff-cleanup"} {
		t.Run("flow/"+fail, func(t *testing.T) {
			w, initial := preparationFixture(t)
			if e := initial.finish(); e != nil {
				t.Fatal(e)
			}
			prep := filepath.Join(w, ".git/lint-image-preparation")
			// Launcher deliberately absent: acquisition must defer it to C2b/rootfs.
			if e := os.Rename(filepath.Join(prep, "owned-eslint"), filepath.Join(prep, "saved-launcher")); e != nil {
				t.Fatal(e)
			}
			if fail == "module" {
				if e := os.WriteFile(filepath.Join(prep, "inputs.json"), []byte(`{}`), 0600); e != nil {
					t.Fatal(e)
				}
			}
			inputPath := filepath.Join(prep, "inputs.json")
			originalInputs, e := os.ReadFile(inputPath)
			if e != nil {
				t.Fatal(e)
			}
			manifestPath := filepath.Join(w, "tools/lint-preparation/package.json")
			// Invalid until materialization: proves the manifest is not read early.
			if e := os.WriteFile(manifestPath, []byte(`{}`), 0600); e != nil {
				t.Fatal(e)
			}
			c, cancel := context.WithCancel(ctx)
			defer cancel()
			var calls []string
			var fs *preparationFS
			var temp string
			closed := 0
			bundle := &NodeBundle{sealed: true}
			archive := []byte("small authenticated-fixture stand-in")
			lock := []byte("frozen raw lock")
			flags := []string{"accepted npm flags"}
			injected := NewPreparationFailure("npm-lock", "origin-denied", "")
			d := acquisitionEffects{goos: "linux", goarch: "amd64"}
			d.open = func(path string) (*preparationFS, error) {
				calls = append(calls, "open")
				var e error
				fs, e = openPreparationFS(path)
				return fs, e
			}
			d.finish = func(p *preparationFS) error {
				closed++
				e := p.finish()
				if fail == "cleanup" || fail == "handoff-cleanup" {
					return errors.Join(e, os.ErrPermission)
				}
				return e
			}
			d.download = func(_ context.Context, stage, url, pin string) ([]byte, error) {
				calls = append(calls, "download")
				if stage != "node-download" || url != "https://nodejs.org/dist/v24.22.0/node-v24.22.0-linux-x64.tar.gz" || pin != "sha256:28ea0a438de9a13bd7891fd758eee6c7819ee8af5442dfd8d45bd65e56d4228c" {
					t.Fatal("download binding", stage, url, pin)
				}
				temp = filepath.Join(prep, fs.temp)
				if fail == "download" || fail == "cleanup" {
					return nil, injected
				}
				return archive, nil
			}
			d.decode = func(_ context.Context, b []byte) (*NodeBundle, error) {
				calls = append(calls, "decode")
				if !reflect.DeepEqual(b, archive) {
					t.Fatal("archive changed")
				}
				if fail == "decode" {
					return nil, injected
				}
				return bundle, nil
			}
			d.materialize = func(_ context.Context, b *NodeBundle, dest string) (string, error) {
				calls = append(calls, "materialize")
				if b != bundle || dest != temp {
					t.Fatal("bundle/temp changed")
				}
				entries, e := os.ReadDir(dest)
				if e != nil || len(entries) != 0 {
					t.Fatal("materialization destination not empty", entries, e)
				}
				if fail == "materialize" {
					return "", injected
				}
				node := filepath.Join(dest, nodeBundlePrefix)
				if e = os.Mkdir(node, 0700); e != nil {
					t.Fatal(e)
				}
				if fail != "manifest" {
					if e = os.WriteFile(manifestPath, []byte(preparationManifest), 0600); e != nil {
						t.Fatal(e)
					}
				}
				if fail == "cancel" {
					cancel()
				}
				return node, nil
			}
			d.install = func(_ context.Context, node, target, home string) ([]string, []byte, error) {
				calls = append(calls, "install")
				if node != filepath.Join(temp, nodeBundlePrefix) || target != filepath.Join(temp, "owned") || home != filepath.Join(temp, "home") {
					t.Fatal("npm path bindings")
				}
				entries, e := os.ReadDir(home)
				if e != nil || len(entries) != 0 {
					t.Fatal("home not fresh", entries, e)
				}
				entries, e = os.ReadDir(target)
				if e != nil || len(entries) != 1 || entries[0].Name() != "package.json" {
					t.Fatal("target not manifest only", entries, e)
				}
				b, e := os.ReadFile(filepath.Join(target, "package.json"))
				if e != nil || string(b) != preparationManifest {
					t.Fatal("manifest byte binding", e)
				}
				if fail == "install" {
					return nil, nil, injected
				}
				return flags, lock, nil
			}
			s, e := acquirePreparationWith(c, w, d)
			expected := []string{"open", "download", "decode", "materialize", "install"}
			stage := ""
			switch fail {
			case "module":
				expected = expected[:1]
				stage = "host-inputs"
			case "download", "cleanup":
				expected = expected[:2]
				stage = "npm-lock"
			case "decode":
				expected = expected[:3]
				stage = "npm-lock"
			case "materialize":
				expected = expected[:4]
				stage = "npm-lock"
			case "manifest":
				expected = expected[:4]
				stage = "owned-manifest"
			case "cancel":
				expected = expected[:4]
				stage = "owned-manifest"
			case "install":
				stage = "npm-lock"
			}
			if !reflect.DeepEqual(calls, expected) {
				t.Fatalf("order: %v want %v error=%v", calls, expected, e)
			}
			if fail == "" || fail == "handoff-cleanup" {
				if e != nil || s == nil {
					t.Fatal("handoff", e)
				}
				if closed != 0 || s.fs != fs || s.bundle != bundle || s.temp != temp || s.nodeRoot != filepath.Join(temp, nodeBundlePrefix) || s.target != filepath.Join(temp, "owned") || s.home != filepath.Join(temp, "home") || string(s.manifest) != preparationManifest || !reflect.DeepEqual(s.lock, lock) || !reflect.DeepEqual(s.flags, flags) || string(s.inputs.module) != "x" || s.inputs.owned["extra"] == nil {
					t.Fatal("handoff bindings")
				}
				e = s.finish()
				if fail == "handoff-cleanup" {
					var f *PreparationFailure
					if !errors.As(e, &f) || f.Stage != "host-inputs" || f.Category != "filesystem" {
						t.Fatal("cleanup success incorrectly reported", e)
					}
				} else if e != nil {
					t.Fatal(e)
				}
			} else {
				var f *PreparationFailure
				if s != nil || !errors.As(e, &f) || f.Stage != stage {
					t.Fatalf("failure: state=%v error=%v wantstage=%s", s, e, stage)
				}
				if stage == "npm-lock" && f.Category != "origin-denied" {
					t.Fatal("inner category lost", e)
				}
			}
			if closed != 1 || !fs.closed {
				t.Fatal("ownership not finished", closed)
			}
			for _, r := range []*os.Root{fs.prep, fs.source} {
				if _, e := r.Stat("."); e == nil {
					t.Fatal("root handle retained")
				}
			}
			currentInputs, e := os.ReadFile(inputPath)
			if e != nil || !reflect.DeepEqual(currentInputs, originalInputs) {
				t.Fatal("caller inputs changed", e)
			}
			if temp != "" {
				if _, e = os.Stat(temp); !os.IsNotExist(e) {
					t.Fatal("owned temp retained", e)
				}
			}
			entries, e := os.ReadDir(prep)
			if e != nil {
				t.Fatal(e)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), "owned-lint-") || entry.Name() == "artifacts" {
					t.Fatal("unexpected owned output", entry.Name())
				}
			}
			b, e := os.ReadFile(filepath.Join(prep, "saved-launcher"))
			if e != nil || string(b) != "launcher" {
				t.Fatal("source launcher changed", e)
			}
			if !reflect.DeepEqual(lock, []byte("frozen raw lock")) || !reflect.DeepEqual(flags, []string{"accepted npm flags"}) {
				t.Fatal("npm result mutated")
			}
		})
	}
}

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
