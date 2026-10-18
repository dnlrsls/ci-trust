package lintverify

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	"io"
	"strings"
)

var artifactUnavailable = errors.New("owned lint artifact unavailable")

// deliveredArtifact holds data only, never an admission or load capability.
type deliveredArtifact struct{ archive, inventory []byte }

var deliveredNames = [7]string{"image.tar", "inventory.json", "owned-inputs.json", "package-lock.json", "package.json", "preparation.json", "SHA256SUMS"}

func artifactSHA(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }

// decodeLintArtifact compares an envelope with reference DATA. Production must
// obtain that reference from the compiled catalog after exact authenticated review.
// Nothing is extracted to disk, interpreted as tool output or granted admission.
func decodeLintArtifact(raw []byte, want lintcatalog.Artifact) (deliveredArtifact, error) {
	bad := func() (deliveredArtifact, error) { return deliveredArtifact{}, artifactUnavailable }
	if len(raw) == 0 || len(raw) > 96<<20 || int64(len(raw)) != want.Size || "sha256:"+artifactSHA(raw) != want.ZipSHA256 {
		return bad()
	}
	z, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if e != nil || len(z.File) != 7 {
		return bad()
	}
	files := map[string][]byte{}
	var total uint64
	for _, f := range z.File {
		known := false
		for _, n := range deliveredNames {
			if f.Name == n {
				known = true
			}
		}
		if !known || !f.Mode().IsRegular() || f.Flags&1 != 0 || (f.Method != zip.Store && f.Method != zip.Deflate) {
			return bad()
		}
		if _, duplicate := files[f.Name]; duplicate {
			return bad()
		}
		limit := int64(16 << 20)
		if f.Name == "image.tar" {
			limit = 256 << 20
		}
		if f.Name == "SHA256SUMS" {
			limit = 4 << 10
		}
		total += f.UncompressedSize64
		if f.UncompressedSize64 > uint64(limit) || total > 352<<20 {
			return bad()
		}
		r, e := f.Open()
		if e != nil {
			return bad()
		}
		b, e := io.ReadAll(io.LimitReader(r, limit+1))
		ce := r.Close()
		if e != nil || ce != nil || int64(len(b)) > limit || uint64(len(b)) != f.UncompressedSize64 {
			return bad()
		}
		files[f.Name] = b
	}
	if len(files) != 7 {
		return bad()
	}
	var manifest strings.Builder
	for _, n := range deliveredNames[:6] {
		manifest.WriteString(artifactSHA(files[n]) + "  " + n + "\n")
	}
	if !bytes.Equal(files["SHA256SUMS"], []byte(manifest.String())) {
		return bad()
	}
	return deliveredArtifact{archive: files["image.tar"], inventory: files["inventory.json"]}, nil
}
