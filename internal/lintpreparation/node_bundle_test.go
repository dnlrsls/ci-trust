package lintpreparation

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"strings"
	"testing"
)

func nodeGzip(t *testing.T, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func nodeTar(t *testing.T, headers ...*tar.Header) []byte {
	t.Helper()
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, h := range headers {
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg || h.Typeflag == tar.TypeCont {
			if _, err := w.Write(bytes.Repeat([]byte{'x'}, int(h.Size))); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func nodeHeader(name string, kind byte) *tar.Header {
	return &tar.Header{Name: nodeBundlePrefix + "/" + name, Typeflag: kind, Mode: 0755}
}

// Synthetic full-flow seam: these pins are not preimages of NodeSHA256.
func decodeSyntheticNode(t *testing.T, headers ...*tar.Header) (*NodeBundle, error) {
	t.Helper()
	compressed := nodeGzip(t, nodeTar(t, headers...))
	original := bytes.Clone(compressed)
	raw, err := ingestNodeBundleWith(context.Background(), compressed, Digest(compressed), nodeIngestLimits{1 << 16, 1 << 16, 100})
	if err != nil {
		return nil, err
	}
	bundle, err := validateNodeGraph(context.Background(), raw)
	if !bytes.Equal(compressed, original) {
		t.Fatal("compressed input mutated")
	}
	return bundle, err
}
func TestNodeGraphBudget(t *testing.T) {
	ctx := context.Background()
	family := []rawNodeRecord{}
	for i, name := range []string{"a0", "a1", "a2", "a3", "a4"} {
		target := "."
		if i > 0 {
			previous := []string{"a0", "a1", "a2", "a3"}[i-1]
			target = previous + "/" + previous
		}
		family = append(family, rawNodeRecord{name: nodeBundlePrefix + "/" + name, kind: tar.TypeSymlink, target: target})
	}
	run := func(raw []rawNodeRecord, b *nodeGraphBudget) (*NodeBundle, error) {
		return validateNodeGraphWith(ctx, raw, b)
	}
	reject := func(t *testing.T, raw []rawNodeRecord, b *nodeGraphBudget) {
		t.Helper()
		bundle, err := run(raw, b)
		if bundle != nil || Diagnostic(err) != Diagnostic(NewPreparationFailure("node-extract", "validation", "rootfs-limit")) {
			t.Fatalf("budget accepted: %v %v", bundle, err)
		}
	}
	t.Run("small doubling family adjacent total work", func(t *testing.T) {
		budget := &nodeGraphBudget{1 << 20, 4096, 256}
		bundle, err := run(family, budget)
		used := int64(1<<20) - budget.left
		if err != nil || bundle == nil || !bundle.sealed || used <= 0 {
			t.Fatalf("unmetered family: used=%d err=%v", used, err)
		}
		reject(t, family, &nodeGraphBudget{used - 1, 4096, 256})
		exact := &nodeGraphBudget{used, 4096, 256}
		if bundle, err := run(family, exact); err != nil || bundle == nil || exact.left != 0 {
			t.Fatalf("exact total: left=%d err=%v", exact.left, err)
		}
	})
	t.Run("shared across records and relocation passes", func(t *testing.T) {
		single := &nodeGraphBudget{1 << 20, 4096, 256}
		if _, err := run(family[:1], single); err != nil {
			t.Fatal(err)
		}
		cost := int64(1<<20) - single.left
		reject(t, family, &nodeGraphBudget{cost, 4096, 256})
		alias := []rawNodeRecord{{name: nodeBundlePrefix + "/alias", kind: tar.TypeSymlink, target: "dir"}, {name: nodeBundlePrefix + "/alias/file", kind: tar.TypeReg}}
		measured := &nodeGraphBudget{1 << 20, 4096, 256}
		if _, err := run(alias, measured); err != nil {
			t.Fatal(err)
		}
		reject(t, alias, &nodeGraphBudget{int64(1<<20) - measured.left - 1, 4096, 256})
	})
	t.Run("resolver calls share work and expansion is capped before split", func(t *testing.T) {
		name := nodeBundlePrefix + "/missing"
		measured := &nodeGraphBudget{1 << 20, 4096, 256}
		if _, err := nodeGraphResolve(ctx, measured, nil, name); err != nil {
			t.Fatal(err)
		}
		cost := int64(1<<20) - measured.left
		shared := &nodeGraphBudget{2*cost - 1, 4096, 256}
		if _, err := nodeGraphResolve(ctx, shared, nil, name); err != nil {
			t.Fatal(err)
		}
		if path, err := nodeGraphResolve(ctx, shared, nil, name); path != "" || Diagnostic(err) != Diagnostic(NewPreparationFailure("node-extract", "validation", "rootfs-limit")) {
			t.Fatalf("reset work: %q %v", path, err)
		}
		name = nodeBundlePrefix + "/a"
		index := map[string]rawNodeRecord{name: {name: name, kind: tar.TypeSymlink, target: strings.Repeat("x/", 32)}}
		// Initial path fits; the pending expansion's conservative slot weight
		// does not. Only a small 64-byte target is used, never a resource probe.
		if path, err := nodeGraphResolve(ctx, &nodeGraphBudget{1 << 20, 512, 256}, index, name); path != "" || Diagnostic(err) != Diagnostic(NewPreparationFailure("node-extract", "validation", "rootfs-limit")) {
			t.Fatalf("uncapped expansion: %q %v", path, err)
		}
	})
	t.Run("preallocation target path and pending caps", func(t *testing.T) {
		reject(t, family, &nodeGraphBudget{0, 4096, 256})
		reject(t, family, &nodeGraphBudget{1 << 20, 32, 256})
		reject(t, family, &nodeGraphBudget{1 << 20, 4096, 2})
		target := []rawNodeRecord{{name: nodeBundlePrefix + "/target", kind: tar.TypeSymlink, target: "ordinary-but-too-long-for-the-target-cap"}}
		reject(t, target, &nodeGraphBudget{1 << 20, 4096, 32})
	})
}

func TestNodeGraph(t *testing.T) {
	link := func(name, target string, kind byte) *tar.Header {
		h := nodeHeader(name, kind)
		h.Linkname = target
		return h
	}
	file := nodeHeader("bin/node", tar.TypeReg)
	file.Size = 3
	file.Mode = 06711
	js := nodeHeader("lib/npm/cli.js", tar.TypeReg)
	js.Size = 2
	js.Mode = 0601
	license := nodeHeader("LICENSE", tar.TypeCont)
	license.Size = 1
	license.Mode = 0644
	t.Run("forward links implicit parents dangling and alias", func(t *testing.T) {
		bundle, err := decodeSyntheticNode(t,
			link("bin/npm", "../lib/npm/cli.js", tar.TypeSymlink),
			link("copy", nodeBundlePrefix+"/forward", tar.TypeLink),
			link("forward", file.Name, tar.TypeLink),
			link("dangling", "missing/leaf", tar.TypeSymlink),
			link("alias", "lib/npm", tar.TypeSymlink),
			nodeHeader("alias/extra", tar.TypeReg), file, js, license)
		if err != nil || bundle == nil || !bundle.sealed {
			t.Fatalf("bundle %v error %v", bundle, err)
		}
		entries := map[string]nodeBundleEntry{}
		for _, e := range bundle.entries {
			entries[e.name] = e
		}
		for _, name := range []string{"bin/node", "copy", "forward"} {
			e := entries[nodeBundlePrefix+"/"+name]
			if e.kind != tar.TypeReg || string(e.data) != "xxx" || !e.ownerExecutable {
				t.Fatalf("file %s: %+v", name, e)
			}
		}
		if entries[nodeBundlePrefix+"/LICENSE"].ownerExecutable || entries[nodeBundlePrefix+"/lib/npm/cli.js"].ownerExecutable {
			t.Fatal("non-owner execute retained")
		}
		if entries[nodeBundlePrefix+"/bin/npm"].target != "../lib/npm/cli.js" || entries[nodeBundlePrefix+"/dangling"].target != "missing/leaf" {
			t.Fatal("link text lost")
		}
		if entries[nodeBundlePrefix+"/lib/npm/extra"].kind != tar.TypeReg || entries[nodeBundlePrefix+"/bin"].kind != tar.TypeDir {
			t.Fatal("alias or implicit parent missing")
		}
	})
	t.Run("finite repeated alias traversal", func(t *testing.T) {
		bundle, err := decodeSyntheticNode(t, nodeHeader("dir", tar.TypeDir), link("a", "dir", tar.TypeSymlink), link("dir/back", "../a", tar.TypeSymlink), link("finite", "a/back/back/missing", tar.TypeSymlink))
		if err != nil || bundle == nil || !bundle.sealed {
			t.Fatalf("safe finite path rejected: %v", err)
		}
	})
	t.Run("prefix must be directory", func(t *testing.T) {
		h := nodeHeader("", tar.TypeReg)
		h.Name = nodeBundlePrefix
		bundle, err := decodeSyntheticNode(t, h)
		if bundle != nil || Diagnostic(err) != Diagnostic(NewPreparationFailure("node-extract", "validation", "layer-ancestor")) {
			t.Fatalf("non-directory root accepted: %v %v", bundle, err)
		}
	})
	t.Run("wrong public pin and zero authority", func(t *testing.T) {
		compressed := nodeGzip(t, nodeTar(t, file))
		b, err := DecodeNodeBundle(context.Background(), compressed)
		if b != nil || Diagnostic(err) != Diagnostic(NewPreparationFailure("node-extract", "validation", "download-identity")) {
			t.Fatalf("%v %v", b, err)
		}
		if (NodeBundle{}).sealed {
			t.Fatal("zero value authorized")
		}
	})
	cases := map[string]struct {
		headers []*tar.Header
		reason  string
	}{}
	for label, target := range map[string]string{"absolute": "/etc/passwd", "drive": "C:/node", "backslash": "..\\node", "escape": "../other/node", "lookalike": "../" + nodeBundlePrefix + "-other/node", "empty": "", "control": "bad\x7f"} {
		cases[label] = struct {
			headers []*tar.Header
			reason  string
		}{[]*tar.Header{file, link("bad", target, tar.TypeSymlink)}, "link-invalid"}
	}
	cases["hard absolute"] = struct {
		headers []*tar.Header
		reason  string
	}{[]*tar.Header{link("bad", "/"+file.Name, tar.TypeLink)}, "link-invalid"}
	cases["self cycle"] = struct {
		headers []*tar.Header
		reason  string
	}{[]*tar.Header{link("a", "a", tar.TypeSymlink)}, "link-cycle"}
	cases["multi cycle"] = struct {
		headers []*tar.Header
		reason  string
	}{[]*tar.Header{link("a", "b", tar.TypeSymlink), link("b", "a", tar.TypeSymlink)}, "link-cycle"}
	cases["hard cycle"] = struct {
		headers []*tar.Header
		reason  string
	}{[]*tar.Header{link("a", nodeBundlePrefix+"/b", tar.TypeLink), link("b", nodeBundlePrefix+"/a", tar.TypeLink)}, "link-cycle"}
	cases["file ancestor"] = struct {
		headers []*tar.Header
		reason  string
	}{[]*tar.Header{file, nodeHeader("bin/node/child", tar.TypeReg)}, "layer-ancestor"}
	cases["hard directory"] = struct {
		headers []*tar.Header
		reason  string
	}{[]*tar.Header{nodeHeader("dir", tar.TypeDir), link("bad", nodeBundlePrefix+"/dir", tar.TypeLink)}, "layer-hardlink"}
	cases["hard missing"] = struct {
		headers []*tar.Header
		reason  string
	}{[]*tar.Header{link("bad", nodeBundlePrefix+"/missing", tar.TypeLink)}, "layer-hardlink"}
	cases["alias conflict"] = struct {
		headers []*tar.Header
		reason  string
	}{[]*tar.Header{link("alias", "lib", tar.TypeSymlink), nodeHeader("alias/a", tar.TypeReg), nodeHeader("lib/a", tar.TypeDir)}, "layer-type-replacement"}
	cases["link then dotdot"] = struct {
		headers []*tar.Header
		reason  string
	}{[]*tar.Header{file, link("bad", "bin/node/../LICENSE", tar.TypeSymlink)}, "layer-ancestor"}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			bundle, err := decodeSyntheticNode(t, tc.headers...)
			if bundle != nil || err == nil || Diagnostic(err) != Diagnostic(NewPreparationFailure("node-extract", "validation", tc.reason)) {
				t.Fatalf("bundle=%v error=%v diagnostic=%s", bundle, err, Diagnostic(err))
			}
		})
	}
	t.Run("cancelled graph", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if b, err := validateNodeGraph(ctx, nil); b != nil || err == nil || err.Error() != "node-extract:network" {
			t.Fatalf("%v %v", b, err)
		}
	})
}

func TestNodeIngest(t *testing.T) {
	limits := nodeIngestLimits{1 << 16, 1 << 16, 20}
	decode := func(data []byte, cap nodeIngestLimits) ([]rawNodeRecord, error) {
		compressed := nodeGzip(t, data)
		return ingestNodeBundleWith(context.Background(), compressed, Digest(compressed), cap)
	}
	t.Run("ordinary records retain bytes links and owner execution", func(t *testing.T) {
		dir := nodeHeader("", tar.TypeDir)
		file := nodeHeader("bin/node", tar.TypeReg)
		file.Size = 3
		file.Mode = 06711
		file.Uid = 123
		file.Gid = 456
		license := nodeHeader("LICENSE", tar.TypeCont)
		license.Size = 2
		license.Mode = 0601
		js := nodeHeader("lib/node_modules/npm/bin/npm-cli.js", tar.TypeReg)
		js.Size = 1
		js.Mode = 0644
		sym := nodeHeader("bin/npm", tar.TypeSymlink)
		sym.Linkname = "../lib/node_modules/npm/bin/npm-cli.js"
		hard := nodeHeader("node-copy", tar.TypeLink)
		hard.Linkname = file.Name
		data := nodeTar(t, dir, file, license, js, sym, hard)
		original := bytes.Clone(data)
		got, err := decode(data, limits)
		if err != nil || len(got) != 6 {
			t.Fatalf("records=%v error=%v", got, err)
		}
		if got[0].name != nodeBundlePrefix || got[1].kind != tar.TypeReg || string(got[1].data) != "xxx" || !got[1].ownerExecutable || got[2].ownerExecutable || got[3].ownerExecutable || got[4].target != sym.Linkname || got[5].target != file.Name {
			t.Fatalf("incorrect raw records: %+v", got)
		}
		if !bytes.Equal(data, original) {
			t.Fatal("input mutated")
		}
	})
	t.Run("pin before parsing", func(t *testing.T) {
		for _, data := range [][]byte{[]byte("not gzip"), nodeGzip(t, []byte("not tar"))} {
			got, err := ingestNodeBundle(context.Background(), data)
			if got != nil || Diagnostic(err) != Diagnostic(NewPreparationFailure("node-extract", "validation", "download-identity")) {
				t.Fatalf("got %v %v", got, err)
			}
		}
	})
	t.Run("rejections", func(t *testing.T) {
		valid := nodeTar(t, nodeHeader("a", tar.TypeReg))
		badChecksum := bytes.Clone(valid)
		badChecksum[0] ^= 1
		cases := map[string][]byte{"checksum": badChecksum, "short": valid[:511], "unaligned": valid[:513], "garbage": bytes.Repeat([]byte{'z'}, 1024)}
		for label, name := range map[string]string{"absolute": "/host", "escape": nodeBundlePrefix + "/../host", "outside": "other/a", "backslash": nodeBundlePrefix + "/a\\b", "drive": nodeBundlePrefix + "/C:/a", "empty component": nodeBundlePrefix + "//a"} {
			h := nodeHeader("a", tar.TypeReg)
			h.Name = name
			cases[label] = nodeTar(t, h)
		}
		for _, kind := range []byte{tar.TypeChar, tar.TypeBlock, tar.TypeFifo} {
			cases[string(kind)] = nodeTar(t, nodeHeader("special", kind))
		}
		cases["duplicate"] = nodeTar(t, nodeHeader("a", tar.TypeReg), nodeHeader("a", tar.TypeDir))
		global := &tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "global", PAXRecords: map[string]string{"comment": "metadata"}}
		cases["global pax"] = nodeTar(t, global)
		for name, data := range cases {
			t.Run(name, func(t *testing.T) {
				original := bytes.Clone(data)
				got, err := decode(data, limits)
				if got != nil || err == nil {
					t.Fatalf("accepted: %+v", got)
				}
				if !bytes.Equal(data, original) {
					t.Fatal("input mutated")
				}
				f, ok := err.(*PreparationFailure)
				if !ok || f.Stage != "node-extract" || (f.Category != "archive" && f.Category != "validation") {
					t.Fatalf("unsafe failure: %v", err)
				}
			})
		}
	})
	t.Run("gzip failures", func(t *testing.T) {
		good := nodeGzip(t, nodeTar(t, nodeHeader("a", tar.TypeReg)))
		crc := bytes.Clone(good)
		crc[len(crc)-8] ^= 1
		for _, data := range [][]byte{[]byte("bad"), good[:len(good)-4], crc} {
			got, err := ingestNodeBundleWith(context.Background(), data, Digest(data), limits)
			if got != nil || err == nil || err.Error() != "node-extract:archive" {
				t.Fatalf("got %v %v", got, err)
			}
		}
	})
	t.Run("raw sizes and hidden metadata", func(t *testing.T) {
		for _, field := range [][]byte{[]byte("00000000003\x00"), {0x80, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3}} {
			if n, ok := nodeRawSize(field); !ok || n != 3 {
				t.Fatalf("size %d %v", n, ok)
			}
		}
		for _, field := range [][]byte{[]byte("-0000000001\x00"), bytes.Repeat([]byte{255}, 12), []byte("zzzzzzzzzzz\x00")} {
			if _, ok := nodeRawSize(field); ok {
				t.Fatal("invalid size accepted")
			}
		}
		var b bytes.Buffer
		w := tar.NewWriter(&b)
		h := nodeHeader("huge", tar.TypeReg)
		h.Size = MaxBytes + 1
		if err := w.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		data := append(bytes.Clone(b.Bytes()), make([]byte, 1024)...)
		if got, err := decode(data, limits); got != nil || err == nil {
			t.Fatal("claimed payload accepted")
		}
		h = nodeHeader("pax", tar.TypeReg)
		h.PAXRecords = map[string]string{"comment": "retained"}
		h.Format = tar.FormatPAX
		data = nodeTar(t, h)
		got, err := decode(data, limits)
		if err != nil || len(got) != 1 || got[0].pax["comment"] != "retained" {
			t.Fatalf("local metadata lost: %v %v", got, err)
		}
		if got, err := decode(data, nodeIngestLimits{1 << 16, 1024, 20}); got != nil || err == nil {
			t.Fatal("hidden metadata unbounded")
		}
		two := nodeTar(t, nodeHeader("a", tar.TypeReg), nodeHeader("b", tar.TypeReg))
		if got, err := decode(two, nodeIngestLimits{1 << 16, 1 << 16, 1}); got != nil || err == nil {
			t.Fatal("late count failure leaked records")
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		compressed := nodeGzip(t, two)
		if got, err := ingestNodeBundleWith(ctx, compressed, Digest(compressed), limits); got != nil || err == nil {
			t.Fatal("cancellation accepted")
		}
	})
	t.Run("tiny bounds", func(t *testing.T) {
		h := nodeHeader("a", tar.TypeReg)
		h.Size = 3
		data := nodeTar(t, h)
		for _, cap := range []nodeIngestLimits{{int64(len(data)) - 1, 1 << 16, 20}, {1 << 16, 511, 20}, {1 << 16, 1 << 16, 0}} {
			got, err := decode(data, cap)
			if got != nil || err == nil {
				t.Fatalf("bound accepted: %+v", cap)
			}
		}
		if got, err := decode(data, nodeIngestLimits{int64(len(data)), 512, 1}); err != nil || len(got) != 1 {
			t.Fatalf("exact bounds %v %v", got, err)
		}
	})
}
