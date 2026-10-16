package controlledverify

import (
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
	"github.com/dnlrsls/ci-trust/internal/ownedimage"
)

func TestCatalogRejectsEveryChangedIdentity(t *testing.T) {
	for _, name := range []string{"image", "archive", "binary", "marker", "layer", "scope"} {
		t.Run(name, func(t *testing.T) {
			m := reviewedImage()
			switch name {
			case "image":
				m.ImageID = "sha256:" + strings.Repeat("0", 64)
			case "archive":
				m.ArchiveSHA256 = "sha256:" + strings.Repeat("0", 64)
			case "binary":
				m.BinarySHA256 = "sha256:" + strings.Repeat("0", 64)
			case "marker":
				m.MarkerSHA256 = "sha256:" + strings.Repeat("0", 64)
			case "layer":
				m.LayerDiffID = "sha256:" + strings.Repeat("0", 64)
			case "scope":
				m.Scope = "execution"
			}
			if admitImage(m) == nil {
				t.Fatal("changed catalog identity admitted")
			}
		})
	}
	if err := admitImage(reviewedImage()); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogPlanRejectsReboundImage(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	m := reviewedImage()
	p, err := controlledplan.PrepareProof(strings.Repeat("f", 64), m, strings.Repeat("a", 40), "123", "1", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := admitPlanImage(p); err != nil {
		t.Fatal(err)
	}
	// Even a self-consistent newly hashed plan cannot introduce another image.
	m.BinarySHA256 = "sha256:" + strings.Repeat("0", 64)
	p, err = controlledplan.PrepareProof(p.Plan.Snapshot, m, p.Revision, "123", "1", now)
	if err != nil {
		t.Fatal(err)
	}
	if err := admitPlanImage(p); err == nil {
		t.Fatal("rebound foreign image admitted")
	}
}

func TestArchiveInspectionRejectsBeforeRuntime(t *testing.T) {
	calls := 0
	inspect := func(a []byte, binary, marker string) (ownedimage.Metadata, error) {
		calls++
		return reviewedImage(), nil
	}
	if _, err := inspectCatalog([]byte("changed archive"), "binary", "marker", inspect); err == nil {
		t.Fatal("archive hash not checked independently")
	}
	if calls != 0 {
		t.Fatal("invalid archive reached inspector")
	}
}
