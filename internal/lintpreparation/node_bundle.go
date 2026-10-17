package lintpreparation

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"strconv"
	"strings"
)

const nodeBundlePrefix = "node-v" + NodeVersion + "-linux-x64"

// rawNodeRecord is NOT safe to materialize: links and ancestors remain unchecked.
// kind retains TAR types (including contiguous regular files). No ownership or
// privileged permissions can be restored from this record. Only the original
// owner-execute bit matters for later owner-only regular-file permissions.
// pax retains effective local metadata for explicit review by the next stage.
type rawNodeRecord struct {
	name            string
	kind            byte
	data            []byte
	target          string
	ownerExecutable bool
	pax             map[string]string
}
type nodeIngestLimits struct {
	bytes, metadata int64
	files           int
}

func ingestNodeBundle(ctx context.Context, compressed []byte) ([]rawNodeRecord, error) {
	return ingestNodeBundleWith(ctx, compressed, "sha256:"+NodeSHA256, nodeIngestLimits{MaxBytes, MaxBytes, MaxFiles})
}

// Only synthetic fixtures supply pins/limits. This is ingestion, NOT a public
// bundle decoder or Python TAR parity claim. No filesystem objects are touched.
func ingestNodeBundleWith(ctx context.Context, compressed []byte, pin string, limits nodeIngestLimits) ([]rawNodeRecord, error) {
	fail := func(category, reason string) ([]rawNodeRecord, error) {
		return nil, NewPreparationFailure("node-extract", category, reason)
	}
	// Identity deliberately precedes even gzip parsing (including malformed data).
	if Digest(compressed) != pin {
		return fail("validation", "download-identity")
	}
	if err := baseContextFailure(ctx, "node-extract"); err != nil {
		return nil, err
	}
	data, err := boundedGzip(ctx, compressed, limits.bytes)
	if err != nil {
		if cancelled := baseContextFailure(ctx, "node-extract"); cancelled != nil {
			return nil, cancelled
		}
		return fail("archive", "")
	}
	if int64(len(data)) > limits.bytes {
		return fail("validation", "")
	}
	if !nodeRawFraming(data, limits) {
		return fail("archive", "")
	}
	r := tar.NewReader(bytes.NewReader(data))
	var records []rawNodeRecord
	seen := map[string]bool{}
	var expanded int64
	for {
		if err := baseContextFailure(ctx, "node-extract"); err != nil {
			return nil, err
		}
		h, err := r.Next()
		if err == io.EOF {
			return records, nil
		}
		if err != nil {
			return fail("archive", "")
		}
		name, err := SafePath(h.Name)
		if err != nil || strings.Contains(name, ":") || (name != nodeBundlePrefix && !strings.HasPrefix(name, nodeBundlePrefix+"/")) {
			return fail("validation", "path-unsafe")
		}
		if seen[name] || len(records) >= limits.files {
			return fail("validation", "layer-duplicate-or-limit")
		}
		seen[name] = true
		for key := range h.PAXRecords {
			// Go can interpret sparse metadata as synthesized bytes, not raw payload.
			if strings.HasPrefix(key, "GNU.sparse.") {
				return fail("archive", "")
			}
		}
		record := rawNodeRecord{name: name, kind: h.Typeflag, target: h.Linkname, ownerExecutable: h.Mode&0100 != 0, pax: h.PAXRecords}
		switch h.Typeflag {
		case tar.TypeReg, tar.TypeRegA, tar.TypeCont:
			if h.Size < 0 || h.Size > limits.bytes-expanded || h.Linkname != "" {
				return fail("validation", "")
			}
			expanded += h.Size
			record.data, err = io.ReadAll(io.LimitReader(r, h.Size+1))
			if err != nil || int64(len(record.data)) != h.Size {
				return fail("archive", "")
			}
		case tar.TypeDir:
			if h.Size != 0 || h.Linkname != "" {
				return fail("archive", "")
			}
		case tar.TypeSymlink, tar.TypeLink:
			if h.Size != 0 {
				return fail("archive", "")
			}
			// Targets are intentionally raw data. Host confinement, cycles, ancestor
			// types and hardlink resolution belong to T4C1b, never image LinkPath.
		default:
			return fail("archive", "")
		}
		records = append(records, record)
	}
}

// Bound raw metadata BEFORE archive/tar consumes hidden PAX/GNU headers. Every
// header costs 512 bytes; extension bodies cost their full declared length.
// Ordinary payload is bounded separately. Go checks checksums and effective
// headers afterward. Global PAX is explicitly unsupported (S6).
func nodeRawFraming(data []byte, limits nodeIngestLimits) bool {
	if len(data) < 1024 || len(data)%512 != 0 {
		return false
	}
	var metadata int64
	headers := 0
	for pos := int64(0); pos < int64(len(data)); {
		block := data[pos : pos+512]
		if nodeZero(block) {
			return int64(len(data))-pos >= 1024 && nodeZero(data[pos:])
		}
		headers++
		metadata += 512
		if headers > limits.files*3 || metadata > limits.metadata {
			return false
		}
		size, ok := nodeRawSize(block[124:136])
		if !ok {
			return false
		}
		kind := block[156]
		switch kind {
		case tar.TypeReg, tar.TypeRegA, tar.TypeCont:
		case tar.TypeDir, tar.TypeSymlink, tar.TypeLink:
			if size != 0 {
				return false
			}
		case tar.TypeXHeader, tar.TypeGNULongName, tar.TypeGNULongLink:
			if size > limits.metadata-metadata {
				return false
			}
			metadata += size
		default:
			return false
		}
		remaining := int64(len(data)) - pos - 512
		if size > remaining {
			return false
		}
		padded := (size + 511) / 512 * 512
		if padded > remaining {
			return false
		}
		pos += 512 + padded
	}
	return false
}

func nodeZero(data []byte) bool {
	for _, b := range data {
		if b != 0 {
			return false
		}
	}
	return true
}

func nodeRawSize(field []byte) (int64, bool) {
	if field[0]&0x80 != 0 {
		// Positive GNU base-256 values only, bounded before arithmetic can overflow.
		value := int64(field[0] & 0x7f)
		for _, b := range field[1:] {
			if value > (1<<63-1-int64(b))/256 {
				return 0, false
			}
			value = value*256 + int64(b)
		}
		return value, true
	}
	text := strings.Trim(string(field), " \x00")
	if text == "" {
		return 0, true
	}
	n, err := strconv.ParseInt(text, 8, 64)
	return n, err == nil && n >= 0
}
