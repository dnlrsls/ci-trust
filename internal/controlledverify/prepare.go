package controlledverify

import (
	"time"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
	"github.com/dnlrsls/ci-trust/internal/ownedimage"
)

// PrepareReview inspects only owned data and measures live source. It performs
// no API, Docker, provisioning or execution operation. Its full JSON output is
// review data, not consent; ExecuteProof independently authenticates the review.
func PrepareReview(source, archive, binary, marker, revision, runID, attempt string, issued time.Time) (controlledplan.Envelope, error) {
	a, err := ownedimage.ReadFile(archive, ownedimage.MaxArchive)
	if err != nil {
		return controlledplan.Envelope{}, imageUnavailable
	}
	meta, err := inspectCatalog(a, binary, marker, ownedimage.Inspect)
	if err != nil {
		return controlledplan.Envelope{}, imageUnavailable
	}
	snapshot, err := sourceSnapshot(source)
	if err != nil {
		return controlledplan.Envelope{}, sourceUnavailable
	}
	return controlledplan.PrepareProof(snapshot, meta, revision, runID, attempt, issued)
}
