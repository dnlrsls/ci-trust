package controlledverify

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
	"github.com/dnlrsls/ci-trust/internal/ownedimage"
)

var imageUnavailable = errors.New("owned image outside fixed catalog")

// reviewedImage is the sole compiled candidate, built with Go 1.26.4,
// linux/amd64, CGO_ENABLED=0, -trimpath -buildvcs=false -ldflags=-buildid=.
// Pinning bytes is not human execution approval or independent security review.
// Any probe change requires rebuilding and reviewing this entire identity tuple.
func reviewedImage() ownedimage.Metadata {
	return ownedimage.Metadata{
		ImageID:       "sha256:2e926eafbfd9d90e2a697443b659adb7ea967cbd5b22691508064c0b7fe504bf",
		ArchiveSHA256: "sha256:3b313de015fd2375bac989b8a428f1eca2b929ef9127aa7ad7c1af7d23e8dd79",
		BinarySHA256:  "sha256:f48e16b92cd2975559300ae1d0be359920784c7d183fdf40590a0aab3a258d82",
		MarkerSHA256:  "sha256:a194da40aa19ba161d5357e8ee9696309981c3ccd1c0c3adece66c0ba3480553",
		LayerDiffID:   "sha256:8bfe3516a63daa0905da6ec4aec724e1a2e44528ecd2c016021e182165cddeea",
		Scope:         "preparation-only",
	}
}

func admitImage(m ownedimage.Metadata) error {
	if m != reviewedImage() {
		return imageUnavailable
	}
	return nil
}

func admitPlanImage(p controlledplan.Envelope) error {
	return admitImage(ownedimage.Metadata{
		ImageID: p.Plan.Image, ArchiveSHA256: p.ArchiveSHA256,
		BinarySHA256: p.BinarySHA256, MarkerSHA256: p.MarkerSHA256,
		LayerDiffID: p.LayerDiffID, Scope: "preparation-only",
	})
}

// The hash gate precedes the independent archive/config/content inspector.
// inspect is a private test seam; production composition supplies ownedimage.Inspect.
func inspectCatalog(a []byte, binary, marker string, inspect func([]byte, string, string) (ownedimage.Metadata, error)) (ownedimage.Metadata, error) {
	if len(a) == 0 || len(a) > ownedimage.MaxArchive || inspect == nil {
		return ownedimage.Metadata{}, imageUnavailable
	}
	sum := sha256.Sum256(a)
	if "sha256:"+hex.EncodeToString(sum[:]) != reviewedImage().ArchiveSHA256 {
		return ownedimage.Metadata{}, imageUnavailable
	}
	m, err := inspect(a, binary, marker)
	if err != nil || admitImage(m) != nil {
		return ownedimage.Metadata{}, imageUnavailable
	}
	return m, nil
}
