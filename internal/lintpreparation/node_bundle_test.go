package lintpreparation

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
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
