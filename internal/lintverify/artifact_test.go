package lintverify

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	"os"
	"strings"
	"testing"
	"time"
)

var artifactNames = []string{"image.tar", "inventory.json", "owned-inputs.json", "package-lock.json", "package.json", "preparation.json", "SHA256SUMS"}

func artifactFixture(t *testing.T, change func(map[string][]byte), duplicate, symlink bool) ([]byte, lintcatalog.Artifact) {
	t.Helper()
	files := map[string][]byte{}
	sum := ""
	for _, n := range artifactNames[:6] {
		files[n] = []byte(n)
		sum += fmt.Sprintf("%x  %s\n", sha256.Sum256(files[n]), n)
	}
	files["SHA256SUMS"] = []byte(sum)
	if change != nil {
		change(files)
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, n := range artifactNames {
		data, ok := files[n]
		if !ok {
			continue
		}
		h := &zip.FileHeader{Name: n, Method: zip.Deflate}
		h.SetMode(0644)
		if symlink && n == "image.tar" {
			h.SetMode(os.ModeSymlink | 0777)
		}
		f, e := w.CreateHeader(h)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = f.Write(data); e != nil {
			t.Fatal(e)
		}
	}
	for n, data := range files {
		found := false
		for _, known := range artifactNames {
			if n == known {
				found = true
			}
		}
		if !found {
			f, e := w.Create(n)
			if e != nil {
				t.Fatal(e)
			}
			f.Write(data)
		}
	}
	if duplicate {
		f, e := w.Create("image.tar")
		if e != nil {
			t.Fatal(e)
		}
		f.Write([]byte("duplicate"))
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	raw := b.Bytes()
	return raw, lintcatalog.Artifact{Size: int64(len(raw)), ZipSHA256: fmt.Sprintf("sha256:%x", sha256.Sum256(raw))}
}
func TestLintArtifactSevenFileConsistency(t *testing.T) {
	raw, want := artifactFixture(t, nil, false, false)
	got, e := decodeLintArtifact(raw, want)
	if e != nil || string(got.archive) != "image.tar" || string(got.inventory) != "inventory.json" {
		t.Error("seven-file data rejected", e)
	}
	for _, kind := range []string{"zip-hash", "zip-size", "missing", "extra", "traversal", "duplicate", "symlink", "checksum", "uppercase", "repeated-checksum", "oversize-metadata"} {
		t.Run(kind, func(t *testing.T) {
			change := func(f map[string][]byte) {
				switch kind {
				case "missing":
					delete(f, "package.json")
				case "extra":
					f["unowned"] = []byte("x")
				case "traversal":
					f["../image.tar"] = f["image.tar"]
					delete(f, "image.tar")
				case "checksum":
					f["inventory.json"] = []byte("changed")
				case "uppercase":
					f["SHA256SUMS"] = bytes.ToUpper(f["SHA256SUMS"])
				case "repeated-checksum":
					rows := strings.Split(strings.TrimSuffix(string(f["SHA256SUMS"]), "\n"), "\n")
					rows[1] = rows[0]
					f["SHA256SUMS"] = []byte(strings.Join(rows, "\n") + "\n")
				case "oversize-metadata":
					f["package.json"] = make([]byte, (16<<20)+1)
				}
			}
			b, w := artifactFixture(t, change, kind == "duplicate", kind == "symlink")
			if kind == "zip-hash" {
				w.ZipSHA256 = "sha256:" + strings.Repeat("0", 64)
			}
			if kind == "zip-size" {
				w.Size++
			}
			out, e := decodeLintArtifact(b, w)
			if e == nil || len(out.archive) != 0 || len(out.inventory) != 0 {
				t.Fatal("rejected artifact returned data")
			}
		})
	}
}
func TestLintReviewBindsArtifactProvenance(t *testing.T) {
	now := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	p, _ := planFor(productionCatalog(), strings.Repeat("a", 64))
	r, e := makeGitHubReviewPlan(p, strings.Repeat("b", 40), 1, now)
	if e != nil {
		t.Fatal(e)
	}
	if r.Artifact != lintcatalog.CurrentArtifact() || r.Artifact.ID == 0 {
		t.Fatal("artifact origin not bound")
	}
	q := r
	q.Artifact.ID++
	q.BindingDigest = q.digest()
	if q.BindingDigest == r.BindingDigest {
		t.Fatal("artifact origin did not affect digest")
	}
	if _, e = readGitHubReviewPlan(bytes.NewReader(q.JSON()), now); e == nil {
		t.Fatal("caller replaced compiled provenance")
	}
}
