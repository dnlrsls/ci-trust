package lintpreparation

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

type baseFixture struct {
	manifest, config map[string]any
	compressed       []byte
}

func newBaseFixture(t *testing.T, headers ...*tar.Header) *baseFixture {
	t.Helper()
	if len(headers) == 0 {
		headers = []*tar.Header{{Name: "owned", Typeflag: tar.TypeDir, Mode: 02775}, {Name: "owned/file", Typeflag: tar.TypeCont, Mode: 0666, Size: 3}}
	}
	raw := layer(t, headers...)
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &baseFixture{
		manifest: map[string]any{"schemaVersion": json.Number("2"), "annotations": map[string]any{"extra": "retained"}, "config": map[string]any{"digest": Digest([]byte("config")), "size": json.Number("123")}, "layers": []any{map[string]any{"digest": Digest(b.Bytes()), "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip", "size": json.Number("456"), "annotations": map[string]any{"extra": "descriptor"}}}},
		config:   map[string]any{"architecture": "amd64", "os": "linux", "rootfs": map[string]any{"diff_ids": []any{Digest(raw)}}}, compressed: b.Bytes(),
	}
}
func baseJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// This fake asserts forwarded pins, not a synthetic manifest preimage of
// BaseManifest. Compressed layer and uncompressed diff hashes ARE real.
func (f *baseFixture) run(t *testing.T, limit int64, replace int, data []byte, failure error) (Tree, map[string]any, error) {
	t.Helper()
	calls := 0
	fetch := func(ctx context.Context, stage, url, digest string) ([]byte, error) {
		i := calls
		calls++
		stages := []string{"base-manifest", "base-config", "base-layer-download"}
		digests := []string{BaseManifest, f.manifest["config"].(map[string]any)["digest"].(string), ""}
		if i == 2 {
			digests[2] = f.manifest["layers"].([]any)[0].(map[string]any)["digest"].(string)
		}
		urls := []string{Base + "manifests/" + digests[0], Base + "blobs/" + digests[1], Base + "blobs/" + digests[2]}
		if i >= 3 {
			t.Fatal("unexpected fetch")
		}
		if stage != stages[i] || url != urls[i] || digest != digests[i] {
			t.Fatalf("incorrect fetch %d", i)
		}
		if i == replace {
			return data, failure
		}
		return [][]byte{baseJSON(t, f.manifest), baseJSON(t, f.config), f.compressed}[i], nil
	}
	return loadBaseWith(context.Background(), fetch, limit)
}
func baseFailure(t *testing.T, err error, stage, category, reason string) {
	t.Helper()
	var f *PreparationFailure
	if !errors.As(err, &f) || f.Stage != stage || f.Category != category || f.Reason != reason {
		t.Fatalf("failure=%#v want %s/%s/%s", err, stage, category, reason)
	}
	if strings.Contains(Diagnostic(err), "secret") {
		t.Fatal("leaked input")
	}
}
func TestBaseSuccess(t *testing.T) {
	f := newBaseFixture(t)
	tree, m, err := f.run(t, MaxBytes, -1, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := Tree{"owned": {Kind: "dir", Mode: 0555}, "owned/file": {Kind: "file", Mode: 0444, Data: []byte("xxx")}}
	if !reflect.DeepEqual(tree, want) || !reflect.DeepEqual(m, f.manifest) {
		t.Fatalf("lost tree or manifest: %#v %#v", tree, m)
	}
}
func TestBaseValidation(t *testing.T) {
	cases := []struct {
		name, stage, reason string
		mutate              func(*baseFixture)
	}{
		{"platform", "base-platform", "platform", func(f *baseFixture) { f.config["architecture"] = "arm64" }},
		{"platform-type", "base-platform", "platform", func(f *baseFixture) { f.config["os"] = []any{} }},
		{"count", "base-layers", "layer-count", func(f *baseFixture) { f.config["rootfs"].(map[string]any)["diff_ids"] = []any{} }},
		{"rootfs-type", "base-layers", "layer-count", func(f *baseFixture) { f.config["rootfs"] = false }},
		{"layers-type", "base-layers", "layer-count", func(f *baseFixture) { f.manifest["layers"] = nil }},
		{"too-many", "base-layers", "layer-count", func(f *baseFixture) {
			ls := make([]any, 65)
			ds := make([]any, 65)
			for i := range ls {
				ls[i] = f.manifest["layers"].([]any)[0]
				ds[i] = "bad"
			}
			f.manifest["layers"] = ls
			f.config["rootfs"].(map[string]any)["diff_ids"] = ds
		}},
		{"media", "base-layers", "layer-media-type", func(f *baseFixture) { f.manifest["layers"].([]any)[0].(map[string]any)["mediaType"] = "secret" }},
		{"descriptor-type", "base-layers", "layer-media-type", func(f *baseFixture) { f.manifest["layers"].([]any)[0] = false }},
		{"diff", "base-layer-diff", "layer-diff-id", func(f *baseFixture) { f.config["rootfs"].(map[string]any)["diff_ids"] = []any{"secret"} }},
		{"diff-type", "base-layer-diff", "layer-diff-id", func(f *baseFixture) { f.config["rootfs"].(map[string]any)["diff_ids"] = []any{false} }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newBaseFixture(t)
			c.mutate(f)
			_, _, err := f.run(t, MaxBytes, -1, nil, nil)
			baseFailure(t, err, c.stage, "validation", c.reason)
		})
	}
}
func TestBaseParseAndDownload(t *testing.T) {
	for i, stage := range []string{"base-manifest", "base-config", "base-layer-download"} {
		t.Run(stage, func(t *testing.T) {
			f := newBaseFixture(t)
			_, _, err := f.run(t, MaxBytes, i, nil, NewPreparationFailure(stage, "validation", "download-identity"))
			baseFailure(t, err, stage, "validation", "download-identity")
		})
	}
	for i, stage := range []string{"base-manifest", "base-config"} {
		for _, data := range []string{`{"x":1,"x":2}`, `{secret`, `[]`, `null`, `42`} {
			f := newBaseFixture(t)
			_, _, err := f.run(t, MaxBytes, i, []byte(data), nil)
			reason := ""
			if strings.Contains(data, `"x"`) {
				reason = "json-duplicate-key"
			}
			baseFailure(t, err, stage, "validation", reason)
		}
	}
	f := newBaseFixture(t)
	_, _, err := f.run(t, MaxBytes, 2, nil, errors.New("secret URL/member trace"))
	baseFailure(t, err, "base-layer-download", "unexpected", "")
	// An already-staged nested failure must not be replaced by an outer stage.
	_, _, err = f.run(t, MaxBytes, 2, nil, NewPreparationFailure("base-config", "network", ""))
	baseFailure(t, err, "base-config", "network", "")
}
func TestBaseGzipAndCap(t *testing.T) {
	f := newBaseFixture(t)
	corrupt := append([]byte(nil), f.compressed...)
	corrupt[len(corrupt)-8] ^= 1
	for _, data := range [][]byte{[]byte("secret bad gzip"), f.compressed[:len(f.compressed)-4], corrupt} {
		_, _, err := f.run(t, MaxBytes, 2, data, nil)
		baseFailure(t, err, "base-layer-decompress", "archive", "")
	}
	_, _, err := f.run(t, 8, -1, nil, nil)
	baseFailure(t, err, "base-layer-diff", "validation", "layer-diff-id")
}
func TestBaseImport(t *testing.T) {
	for _, c := range []struct {
		h                *tar.Header
		category, reason string
	}{
		{&tar.Header{Name: "secret", Typeflag: tar.TypeDir, Mode: 04755}, "validation", "layer-privileged"},
		{&tar.Header{Typeflag: tar.TypeXGlobalHeader, PAXRecords: map[string]string{"comment": "secret"}}, "archive", ""},
		{&tar.Header{Name: "../secret", Typeflag: tar.TypeDir, Mode: 0755}, "validation", "path-unsafe"},
	} {
		f := newBaseFixture(t, c.h)
		tree, m, err := f.run(t, MaxBytes, -1, nil, nil)
		baseFailure(t, err, "base-layer-import", c.category, c.reason)
		if tree != nil || m != nil {
			t.Fatal("returned partial result")
		}
	}
}
func TestBaseDescriptorTypes(t *testing.T) {
	for _, manifest := range []string{`{}`, `{"config":false}`, `{"config":{"digest":42}}`, `{"config":{"digest":"secret"}}`} {
		calls := 0
		_, _, err := loadBaseWith(context.Background(), func(context.Context, string, string, string) ([]byte, error) { calls++; return []byte(manifest), nil }, MaxBytes)
		baseFailure(t, err, "base-config", "validation", "")
		if calls != 1 {
			t.Fatal("invalid descriptor fetched")
		}
	}
	f := newBaseFixture(t)
	f.manifest["layers"].([]any)[0].(map[string]any)["digest"] = false
	calls := 0
	_, _, err := loadBaseWith(context.Background(), func(_ context.Context, stage, _, _ string) ([]byte, error) {
		calls++
		if stage == "base-manifest" {
			return baseJSON(t, f.manifest), nil
		}
		if stage == "base-config" {
			return baseJSON(t, f.config), nil
		}
		t.Fatal("invalid layer digest fetched")
		return nil, nil
	}, MaxBytes)
	baseFailure(t, err, "base-layer-download", "validation", "")
	if calls != 2 {
		t.Fatal("unexpected fetch count")
	}
}
func TestBaseGzipBoundary(t *testing.T) {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write([]byte("123456789")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	for _, limit := range []int64{8, 9} {
		data, err := boundedGzip(context.Background(), b.Bytes(), limit)
		if err != nil || string(data) != "123456789" {
			t.Fatalf("bounded data=%q err=%v", data, err)
		}
		if (int64(len(data)) > limit) != (limit == 8) {
			t.Fatal("lost overflow sentinel")
		}
	}
}
func TestBaseCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := loadBaseWith(ctx, func(context.Context, string, string, string) ([]byte, error) {
		t.Fatal("fetch after cancellation")
		return nil, nil
	}, MaxBytes)
	baseFailure(t, err, "base-manifest", "network", "")
}
