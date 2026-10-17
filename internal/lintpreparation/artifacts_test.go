package lintpreparation

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func artifactFixture(t *testing.T) artifactRequest {
	t.Helper()
	module, launcher := []byte("module"), []byte("launcher")
	return artifactRequest{
		tree:     Tree{"ci-trust": {Kind: "dir", Mode: 0555}, "ci-trust/owned-eslint": {Kind: "file", Mode: 0555, Data: launcher}, "owned": {Kind: "dir", Mode: 0555}, "owned/eslint.config.mjs": {Kind: "file", Mode: 0444, Data: module}},
		manifest: []byte(" \n" + ownedManifest + "\n"), lock: append(npmBytes(t, npmLock()), '\n'), launcher: launcher,
		ownedInputs:  map[string]any{"Module": string(module), "ModuleSHA256": Digest(module)[7:], "RecipeSHA256": "recipe", "Args": []any{"arg"}},
		baseManifest: map[string]any{"layers": []any{map[string]any{"size": 123, "digest": Digest([]byte("layer"))}}},
		npmFlags:     []string{"--ignore-scripts"}, sourceRevision: "revision", runID: "42", runAttempt: "2",
	}
}

func TestArtifactsPayloads(t *testing.T) {
	r := artifactFixture(t)
	snapshot := npmBytes(t, map[string]any{"tree": r.tree, "owned": r.ownedInputs, "base": r.baseManifest, "flags": r.npmFlags})
	manifest, lock := bytes.Clone(r.manifest), bytes.Clone(r.lock)
	out, err := buildArtifacts(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"image.tar", "inventory.json", "owned-inputs.json", "package-lock.json", "package.json", "preparation.json"}
	if len(out) != 7 {
		t.Fatalf("keys: %v", out)
	}
	var sums strings.Builder
	for _, name := range names {
		b, ok := out[name]
		if !ok {
			t.Fatal(name)
		}
		sums.WriteString(Digest(b)[7:] + "  " + name + "\n")
	}
	if string(out["SHA256SUMS"]) != sums.String() {
		t.Fatal("checksum identities/order")
	}
	if !bytes.Equal(out["package.json"], manifest) || !bytes.Equal(out["package-lock.json"], lock) {
		t.Fatal("original bytes")
	}
	for _, name := range []string{"inventory.json", "owned-inputs.json", "preparation.json"} {
		b := out[name]
		v, e := UniqueJSON(b)
		if e != nil {
			t.Fatal(e)
		}
		canonical, e := Canonical(v)
		if e != nil || !bytes.Equal(b, append(canonical, '\n')) {
			t.Fatal("canonical LF", name)
		}
	}
	records, e := Inventory(r.tree)
	if e != nil {
		t.Fatal(e)
	}
	want, _ := Canonical(records)
	if !bytes.Equal(out["inventory.json"], append(want, '\n')) {
		t.Fatal("inventory")
	}
	metaValue, e := UniqueJSON(out["preparation.json"])
	if e != nil {
		t.Fatal(e)
	}
	meta := metaValue.(map[string]any)
	wantMeta := map[string]any{
		"status": "PREPARATION_ONLY_NOT_ADMITTED", "node_security_hold": true,
		"node_version": NodeVersion, "npm_version": NPMVersion, "eslint_version": ESLintVersion,
		"node_archive_sha256": "sha256:" + NodeSHA256, "base_manifest": BaseManifest,
		"lock_sha256": Digest(lock), "manifest_sha256": Digest(manifest), "launcher_sha256": Digest(r.launcher),
		"source_revision": r.sourceRevision, "run_id": r.runID, "run_attempt": r.runAttempt, "go_version": runtime.Version(),
		"serialization_check":          "same-process repeat; not independent offline reconstruction",
		"independent_policy_admission": false, "real_tool_execution": false,
		"elf_library_closure": "NOT_VERIFIED; static and runtime compatibility remain separate gates",
		"limits":              "15 minute hosted job; 180 seconds per npm phase",
	}
	for k, v := range wantMeta {
		if !reflect.DeepEqual(meta[k], v) {
			t.Fatalf("metadata %s: %v", k, meta[k])
		}
	}
	if _, ok := meta["python_version"]; ok {
		t.Fatal("stale producer")
	}
	for k, v := range map[string]any{"npm_flags": r.npmFlags, "base_layers": r.baseManifest["layers"]} {
		a, _ := Canonical(meta[k])
		normalized, err := UniqueJSON(npmBytes(t, v))
		if err != nil {
			t.Fatal(err)
		}
		b, _ := Canonical(normalized)
		if !bytes.Equal(a, b) {
			t.Fatal(k)
		}
	}
	_, ids, e := PackImage(r.tree)
	if e != nil {
		t.Fatal(e)
	}
	if meta["image_id"] != ids.ImageID || meta["layer_diff_id"] != ids.LayerDiffID || meta["archive_sha256"] != Digest(out["image.tar"]) {
		t.Fatal("image identities")
	}
	if e := InspectArchive(out["image.tar"], records, ids); e != nil {
		t.Fatal(e)
	}
	again, e := buildArtifacts(context.Background(), r)
	if e != nil || !reflect.DeepEqual(out, again) {
		t.Fatal("repeat", e)
	}
	after := npmBytes(t, map[string]any{"tree": r.tree, "owned": r.ownedInputs, "base": r.baseManifest, "flags": r.npmFlags})
	if !bytes.Equal(snapshot, after) || !bytes.Equal(manifest, r.manifest) || !bytes.Equal(lock, r.lock) {
		t.Fatal("input mutation")
	}
}

func TestArtifactsFailures(t *testing.T) {
	for _, kind := range []string{"first-error", "corrupt-first", "second-error", "different-bytes", "different-ids", "cancel-after-first", "cancel-after-second"} {
		t.Run(kind, func(t *testing.T) {
			r := artifactFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			pack := func(tree Tree) ([]byte, ImageIdentities, error) {
				calls++
				b, ids, e := PackImage(tree)
				if (kind == "first-error" && calls == 1) || (kind == "second-error" && calls == 2) {
					return nil, ids, errors.New("private sentinel")
				}
				if kind == "corrupt-first" && calls == 1 {
					b = bytes.Clone(b)
					b[0] ^= 1
				}
				if kind == "different-bytes" && calls == 2 {
					b = append(b, 0)
				}
				if kind == "different-ids" && calls == 2 {
					ids.ImageID = Digest([]byte("other"))
				}
				if (kind == "cancel-after-first" && calls == 1) || (kind == "cancel-after-second" && calls == 2) {
					cancel()
				}
				return b, ids, e
			}
			out, e := buildArtifactsWith(ctx, r, pack)
			stage := "image-validation"
			if kind == "cancel-after-second" {
				stage = "image-validation"
			}
			artifactFailure(t, out, e, stage)
			wantCalls := 2
			if kind == "first-error" || kind == "corrupt-first" || kind == "cancel-after-first" {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatalf("calls %d want %d", calls, wantCalls)
			}
		})
	}
	for _, kind := range []string{"manifest", "lock", "module-hash", "module-binding", "launcher-binding", "layers", "canonical", "pre-cancel"} {
		t.Run(kind, func(t *testing.T) {
			r := artifactFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch kind {
			case "manifest":
				r.manifest = []byte(`{}`)
			case "lock":
				r.lock = []byte(`{}`)
			case "module-hash":
				r.ownedInputs["ModuleSHA256"] = "bad"
			case "module-binding":
				r.tree["owned/eslint.config.mjs"] = Node{Kind: "file", Mode: 0444, Data: []byte("different")}
			case "launcher-binding":
				r.launcher = []byte("different")
			case "layers":
				delete(r.baseManifest, "layers")
			case "canonical":
				r.ownedInputs["unsupported"] = 1.5
			case "pre-cancel":
				cancel()
			}
			out, e := buildArtifacts(ctx, r)
			stage := "artifact-write"
			if kind == "pre-cancel" {
				stage = "image-validation"
			}
			artifactFailure(t, out, e, stage)
		})
	}
}

func artifactFailure(t *testing.T, out map[string][]byte, e error, stage string) {
	t.Helper()
	var f *PreparationFailure
	if out != nil || !errors.As(e, &f) || f.Stage != stage || strings.Contains(e.Error(), "private") {
		t.Fatalf("partial/unsafe failure: %v %v", out, e)
	}
	if !member(categories, f.Category) || !member(stages, f.Stage) {
		t.Fatal("vocabulary", f)
	}
}

// Keep the expected checksum order visible and independently checked.
func TestArtifactChecksumNamesSorted(t *testing.T) {
	names := []string{"image.tar", "inventory.json", "owned-inputs.json", "package-lock.json", "package.json", "preparation.json"}
	if !sort.StringsAreSorted(names) {
		t.Fatal(names)
	}
}
