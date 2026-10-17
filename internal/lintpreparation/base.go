package lintpreparation

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
)

type baseFetch func(context.Context, string, string, string) ([]byte, error)

// LoadBase retrieves the fixed digest-pinned linux/amd64 base without host
// extraction. Manifest maps retain all descriptor fields and JSON numbers.
func LoadBase(ctx context.Context) (Tree, map[string]any, error) {
	return loadBaseWith(ctx, Download, MaxBytes)
}

// boundedGzip reads at most limit+1 inflated bytes; callers decide how to
// diagnose overflow. No allocation is based on gzip's declared size. The
// private callers supply nonnegative owned limits, never public configuration.
// Go gzip header, CRC and truncation errors all become archive failures at the
// caller's stage. This is a deliberate safe equivalent, not Python exception
// category parity (Python distinguishes BadGzipFile/EOFError/zlib.error).
func boundedGzip(ctx context.Context, compressed []byte, limit int64) ([]byte, error) {
	stream, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer stream.Close()
	return io.ReadAll(io.LimitReader(baseContextReader{ctx, stream}, limit+1))
}

type baseContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r baseContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
func baseContextFailure(ctx context.Context, stage string) error {
	if err := ctx.Err(); err != nil {
		category := "network"
		if errors.Is(err, context.DeadlineExceeded) {
			category = "timeout"
		}
		return NewPreparationFailure(stage, category, "")
	}
	return nil
}

// Only tests inject a fetch function/limit. Production always uses Download,
// Base, BaseManifest and MaxBytes, with no public backend or origin overrides.
func loadBaseWith(ctx context.Context, fetch baseFetch, limit int64) (Tree, map[string]any, error) {
	var manifest, config map[string]any
	readObject := func(stage, url, digest string, out *map[string]any) error {
		return InStage(stage, func() error {
			if err := baseContextFailure(ctx, stage); err != nil {
				return err
			}
			data, err := fetch(ctx, stage, url, digest)
			if err != nil {
				return err
			}
			value, err := UniqueJSON(data)
			if err != nil {
				return err
			}
			object, ok := value.(map[string]any)
			if !ok {
				return NewValidationFailure("")
			}
			*out = object
			return nil
		})
	}
	if err := readObject("base-manifest", Base+"manifests/"+BaseManifest, BaseManifest, &manifest); err != nil {
		return nil, nil, err
	}
	descriptor, _ := manifest["config"].(map[string]any)
	configDigest, _ := descriptor["digest"].(string)
	if !ValidDigest(configDigest) {
		return nil, nil, NewPreparationFailure("base-config", "validation", "")
	}
	if err := readObject("base-config", Base+"blobs/"+configDigest, configDigest, &config); err != nil {
		return nil, nil, err
	}
	if config["architecture"] != "amd64" || config["os"] != "linux" {
		return nil, nil, NewPreparationFailure("base-platform", "validation", "platform")
	}
	layers, ok := manifest["layers"].([]any)
	rootfs, _ := config["rootfs"].(map[string]any)
	diffs, diffOK := rootfs["diff_ids"].([]any)
	if !ok || !diffOK || len(layers) != len(diffs) || len(layers) > MaxLayers {
		return nil, nil, NewPreparationFailure("base-layers", "validation", "layer-count")
	}
	tree := Tree{}
	for i, value := range layers {
		descriptor, _ := value.(map[string]any)
		if descriptor["mediaType"] != "application/vnd.oci.image.layer.v1.tar+gzip" {
			return nil, nil, NewPreparationFailure("base-layers", "validation", "layer-media-type")
		}
		digest, _ := descriptor["digest"].(string)
		var compressed, data []byte
		err := InStage("base-layer-download", func() error {
			if err := baseContextFailure(ctx, "base-layer-download"); err != nil {
				return err
			}
			if !ValidDigest(digest) {
				return NewValidationFailure("")
			}
			var err error
			compressed, err = fetch(ctx, "base-layer-download", Base+"blobs/"+digest, digest)
			return err
		})
		if err != nil {
			return nil, nil, err
		}
		err = InStage("base-layer-decompress", func() error {
			if err := baseContextFailure(ctx, "base-layer-decompress"); err != nil {
				return err
			}
			var err error
			data, err = boundedGzip(ctx, compressed, limit)
			if err != nil {
				if cancelled := baseContextFailure(ctx, "base-layer-decompress"); cancelled != nil {
					return cancelled
				}
				return NewPreparationFailure("base-layer-decompress", "archive", "")
			}
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
		diff, _ := diffs[i].(string)
		if int64(len(data)) > limit || Digest(data) != diff {
			return nil, nil, NewPreparationFailure("base-layer-diff", "validation", "layer-diff-id")
		}
		err = InStage("base-layer-import", func() error {
			if err := baseContextFailure(ctx, "base-layer-import"); err != nil {
				return err
			}
			return Overlay(tree, data)
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return tree, manifest, nil
}
