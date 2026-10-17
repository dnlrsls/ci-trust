package lintpreparation

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const nodeBundlePrefix = "node-v" + NodeVersion + "-linux-x64"

// NodeBundle is opaque preparation data, never filesystem authority by default.
// T4C2 must require sealed; zero values and nil pointers are not validated.
type NodeBundle struct {
	entries []nodeBundleEntry
	sealed  bool
}
type nodeBundleEntry struct {
	name            string
	kind            byte // directory, regular file, or symlink; hardlinks become file bytes
	data            []byte
	target          string
	ownerExecutable bool
}

// DecodeNodeBundle accepts only the fixed compressed Node artifact.
func DecodeNodeBundle(ctx context.Context, compressed []byte) (*NodeBundle, error) {
	raw, err := ingestNodeBundle(ctx, compressed)
	if err != nil {
		return nil, err
	}
	return validateNodeGraph(ctx, raw)
}

// Work measures conservative byte/slot visits, not elapsed time. SafePath
// permits 4096 UTF-8 runes: paths/targets cap at 16KiB; pending tokens plus
// stack cap at 32 such paths (512KiB weighted bytes, including string slots).
// Total graph work is 32 times the existing 512MiB metadata cap. This owned
// profile deliberately rejects costlier finite aliases, not just actual cycles.
// These are technical work/state bounds, not elapsed-time or peak-heap claims.
const nodeGraphPathBytes = 4 * 4096
const nodeGraphState = 32 * nodeGraphPathBytes
const nodeGraphWork int64 = 32 * MaxBytes

type nodeGraphBudget struct {
	left        int64
	state, path int
}

func (b *nodeGraphBudget) take(n int64) bool {
	if n < 0 || n > b.left {
		return false
	}
	b.left -= n
	return true
}

// One reservation covers bounded path scans/joins/hash operations per record.
// Target length is checked before nodeHostTarget scans it. No payload copying.
func (b *nodeGraphBudget) record(name, target string) bool {
	return len(name) <= b.path && len(target) <= b.path && b.take(256+64*int64(len(name))+4*int64(len(target)))
}
func validateNodeGraph(ctx context.Context, raw []rawNodeRecord) (*NodeBundle, error) {
	return validateNodeGraphWith(ctx, raw, &nodeGraphBudget{nodeGraphWork, nodeGraphState, nodeGraphPathBytes})
}

// Private reduced-budget seam for small synthetic fixtures only.
func validateNodeGraphWith(ctx context.Context, raw []rawNodeRecord, budget *nodeGraphBudget) (*NodeBundle, error) {
	fail := func(reason string) (*NodeBundle, error) { return nil, nodeGraphFailure(reason) }
	if !budget.take(int64(len(raw)) * 32) {
		return fail("rootfs-limit")
	}
	index := map[string]rawNodeRecord{}
	names := make([]string, len(raw))
	for i, r := range raw {
		if !budget.record(r.name, r.target) {
			return fail("rootfs-limit")
		}
		if err := baseContextFailure(ctx, "node-extract"); err != nil {
			return nil, err
		}
		if r.name == nodeBundlePrefix && r.kind != tar.TypeDir {
			return fail("layer-ancestor")
		}
		if r.kind == tar.TypeSymlink || r.kind == tar.TypeLink {
			if !nodeHostTarget(r.target) {
				return fail("link-invalid")
			}
		}
		index[r.name] = r
		names[i] = r.name
	}
	// Relocate entries beneath symlink aliases to their physical destination.
	// Rebuild until stable so forward/nested aliases are order-independent. Each
	// pass resolves against the previous complete index, never the host filesystem.
	stable := false
	for pass := 0; pass <= len(raw); pass++ {
		next := map[string]rawNodeRecord{}
		changed := false
		for i, r := range raw {
			if !budget.record(r.name, r.target) {
				return fail("rootfs-limit")
			}
			if err := baseContextFailure(ctx, "node-extract"); err != nil {
				return nil, err
			}
			parent, err := nodeGraphResolve(ctx, budget, index, path.Dir(r.name))
			if r.name == nodeBundlePrefix {
				parent = "."
				err = nil
			}
			if err != nil {
				return nil, err
			}
			if p, ok := index[parent]; ok && p.kind != tar.TypeDir {
				return fail("layer-ancestor")
			}
			if len(parent)+1+len(path.Base(r.name)) > budget.path || !budget.take(256+64*int64(len(parent)+len(r.name))) {
				return fail("rootfs-limit")
			}
			name := path.Join(parent, path.Base(r.name))
			if _, err := SafePath(name); err != nil {
				return fail("link-invalid")
			}
			if old, ok := next[name]; ok && (old.kind != tar.TypeDir || r.kind != tar.TypeDir) {
				return fail("layer-type-replacement")
			}
			changed = changed || names[i] != name
			names[i] = name
			r.name = name
			next[name] = r
		}
		index = next
		if !changed {
			stable = true
			break
		}
	}
	if !stable {
		return fail("link-cycle")
	}
	if err := baseContextFailure(ctx, "node-extract"); err != nil {
		return nil, err
	}
	entries := map[string]nodeBundleEntry{}
	var total int64
	for name, r := range index {
		if !budget.record(name, r.target) {
			return fail("rootfs-limit")
		}
		if err := baseContextFailure(ctx, "node-extract"); err != nil {
			return nil, err
		}
		entry := nodeBundleEntry{name: name, kind: r.kind, data: r.data, target: r.target, ownerExecutable: r.ownerExecutable}
		if r.kind == tar.TypeSymlink || r.kind == tar.TypeLink {
			resolved, err := nodeGraphResolve(ctx, budget, index, name)
			if err != nil {
				return nil, err
			}
			if r.kind == tar.TypeLink {
				target, ok := index[resolved]
				if !ok || (target.kind != tar.TypeReg && target.kind != tar.TypeRegA && target.kind != tar.TypeCont) {
					return fail("layer-hardlink")
				}
				entry.data = target.data
				entry.ownerExecutable = target.ownerExecutable
				entry.target = ""
				entry.kind = tar.TypeReg
			}
		}
		if entry.kind == tar.TypeRegA || entry.kind == tar.TypeCont {
			entry.kind = tar.TypeReg
		}
		if entry.kind == tar.TypeReg {
			total += int64(len(entry.data))
		} else {
			entry.ownerExecutable = false
		}
		// Ownership, privilege bits, xattrs and timestamps are deliberately not
		// materialization metadata (matching data_filter's ownership policy).
		// Effective path/linkpath/size were already applied by archive/tar.
		if total > MaxBytes {
			return fail("rootfs-limit")
		}
		entries[name] = entry
	}
	// Missing parents are ordinary implicit directories, not dangling-link errors.
	for name := range index {
		if !budget.record(name, "") {
			return fail("rootfs-limit")
		}
		if err := baseContextFailure(ctx, "node-extract"); err != nil {
			return nil, err
		}
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if !budget.record(parent, "") {
				return fail("rootfs-limit")
			}
			if existing, ok := entries[parent]; ok && existing.kind != tar.TypeDir {
				return fail("layer-ancestor")
			}
			entries[parent] = nodeBundleEntry{name: parent, kind: tar.TypeDir}
			if len(entries) > MaxFiles {
				return fail("rootfs-limit")
			}
		}
	}
	if len(entries) == 0 || len(entries) > MaxFiles {
		return fail("rootfs-limit")
	}
	// Reserve comparisons (ceil(log2(n))+1), path copying, and assembly BEFORE
	// allocating output. String comparisons can inspect up to path bytes each.
	levels := 1
	for n := len(entries); n > 1; n = (n + 1) / 2 {
		levels++
	}
	if !budget.take(int64(len(entries)) * 32) {
		return fail("rootfs-limit")
	}
	longest := 0
	for name := range entries {
		if len(name) > longest {
			longest = len(name)
		}
	}
	if !budget.take(int64(len(entries)) * int64(longest+32) * int64(16*levels+16)) {
		return fail("rootfs-limit")
	}
	ordered := make([]string, 0, len(entries))
	for name := range entries {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	bundle := &NodeBundle{}
	for _, name := range ordered {
		bundle.entries = append(bundle.entries, entries[name])
	}
	if err := baseContextFailure(ctx, "node-extract"); err != nil {
		return nil, err
	}
	bundle.sealed = true
	return bundle, nil
}

func nodeGraphFailure(reason string) error {
	return NewPreparationFailure("node-extract", "validation", reason)
}
func nodeHostTarget(target string) bool {
	if target == "" || strings.HasPrefix(target, "/") || strings.ContainsAny(target, "\\\\:") || !utf8.ValidString(target) {
		return false
	}
	for _, r := range target {
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

// Walk components BEFORE processing '..': lexical Clean before following a
// symlink would incorrectly accept paths such as file/../escape. Hardlinks are
// archive-root-relative; symlinks are relative to their physical parent.
func nodeGraphResolve(ctx context.Context, budget *nodeGraphBudget, index map[string]rawNodeRecord, name string) (string, error) {
	initial := int64(len(name))*17 + 16
	if len(name) > budget.path || initial > int64(budget.state) || !budget.take(initial) {
		return "", nodeGraphFailure("rootfs-limit")
	}
	todo := strings.Split(name, "/")
	queued := len(name) + 16*len(todo) // includes conservative separator slack
	stackBytes := 0
	stack := []string{}
	followed := map[string]bool{}
	for len(todo) > 0 {
		// Pay before scanning pending tokens, joining/hashing stack paths, or
		// growing slices. Slot weights cover scans even for empty components.
		state := queued + stackBytes + 16*len(stack)
		if state > budget.state || !budget.take(16*int64(state)+256) {
			return "", nodeGraphFailure("rootfs-limit")
		}
		if err := baseContextFailure(ctx, "node-extract"); err != nil {
			return "", err
		}
		part := todo[0]
		todo = todo[1:]
		queued -= len(part) + 16
		// End-of-expansion markers distinguish a real cycle from repeated but
		// finite traversal of the same directory alias in the remaining suffix.
		if strings.HasPrefix(part, "\x00") {
			delete(followed, part[1:])
			continue
		}
		remaining := false
		for _, p := range todo {
			if !strings.HasPrefix(p, "\x00") {
				remaining = true
				break
			}
		}
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			if len(stack) <= 1 {
				return "", nodeGraphFailure("link-invalid")
			}
			stackBytes -= len(stack[len(stack)-1]) + 1
			stack = stack[:len(stack)-1]
			continue
		}
		if stackBytes+len(part) > budget.path || queued+stackBytes+len(part)+1+16*(len(stack)+1) > budget.state {
			return "", nodeGraphFailure("rootfs-limit")
		}
		stackBytes += len(part) + 1
		stack = append(stack, part)
		if stack[0] != nodeBundlePrefix {
			return "", nodeGraphFailure("link-invalid")
		}
		current := strings.Join(stack, "/")
		r, ok := index[current]
		if !ok {
			continue
		} // confined dangling symlinks are allowed by data_filter
		if r.kind == tar.TypeSymlink || r.kind == tar.TypeLink {
			if followed[current] {
				return "", nodeGraphFailure("link-cycle")
			}
			// Reserve worst-case split slots (one per byte), marker, and queue
			// copying BEFORE scanning/splitting target or allocating expansion.
			extra := int64(len(r.target))*17 + int64(len(current)) + 32
			if len(r.target) > budget.path || int64(queued+stackBytes+16*len(stack))+extra > int64(budget.state) || !budget.take(extra+int64(queued)) {
				return "", nodeGraphFailure("rootfs-limit")
			}
			followed[current] = true
			if r.kind == tar.TypeLink {
				if remaining {
					return "", nodeGraphFailure("layer-ancestor")
				}
				stack = nil
				stackBytes = 0
			} else {
				stackBytes -= len(stack[len(stack)-1]) + 1
				stack = stack[:len(stack)-1]
			}
			expansion := append(strings.Split(r.target, "/"), "\x00"+current)
			queued += len(r.target) + 16*(len(expansion)-1) + len(current) + 1 + 16
			todo = append(expansion, todo...)
		} else if r.kind != tar.TypeDir && remaining {
			return "", nodeGraphFailure("layer-ancestor")
		}
	}
	result, err := SafePath(strings.Join(stack, "/"))
	if err != nil {
		return "", nodeGraphFailure("link-invalid")
	}
	return result, nil
}

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
