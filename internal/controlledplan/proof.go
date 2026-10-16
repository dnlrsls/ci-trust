package controlledplan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"time"

	"github.com/dnlrsls/ci-trust/internal/ownedimage"
	"github.com/dnlrsls/ci-trust/internal/runner"
)

// ProofCases are four fixed invocations of the same owned tool, not arbitrary commands.
type ProofCases struct {
	Normal       runner.Plan `json:"normal"`
	Timeout      runner.Plan `json:"timeout"`
	ActiveCancel runner.Plan `json:"active_cancel"`
	OutputLimit  runner.Plan `json:"combined_output_limit"`
}

const proofEffects = "load only the reviewed owned archive into local Linux Docker; inspect immutable identity/config; run four fixed owned-smoke cases; remove owned containers; retain the untagged immutable image (no image deletion authority); host artifacts only under .git/controlled-proof; no pull/build/host mounts/network/credential forwarding"

// PrepareProof extends the existing envelope without changing preparation bytes.
// It is review data: only a separately authenticated exact-plan decision can authorize it.
func PrepareProof(snapshot string, meta ownedimage.Metadata, revision, runID, attempt string, issued time.Time) (Envelope, error) {
	e, err := Prepare(snapshot, meta.ImageID, revision, runID, attempt)
	if err != nil || meta.Scope != "preparation-only" || attempt != "1" || issued.IsZero() {
		return Envelope{}, invalid
	}
	for _, value := range []string{meta.ArchiveSHA256, meta.BinarySHA256, meta.MarkerSHA256, meta.LayerDiffID} {
		if !image.MatchString(value) {
			return Envelope{}, invalid
		}
	}
	issued = issued.UTC().Truncate(time.Second)
	e.Scope = "owned-smoke-execution"
	e.Plan.Config = "owned-proof-v1"
	e.PlanDigest = e.Plan.Digest()
	e.IssuedAt = issued.Format(time.RFC3339)
	e.ExpiresAt = issued.Add(time.Hour).Format(time.RFC3339)
	e.ArchiveSHA256, e.BinarySHA256 = meta.ArchiveSHA256, meta.BinarySHA256
	e.MarkerSHA256, e.LayerDiffID = meta.MarkerSHA256, meta.LayerDiffID
	e.PreparationEffects, e.ImageActionWall = proofEffects, 20*time.Second
	e.Cases = &ProofCases{Normal: e.Plan, Timeout: e.Plan, ActiveCancel: e.Plan, OutputLimit: e.Plan}
	e.Cases.Timeout.Limits.Wall = time.Second
	e.Cases.OutputLimit.Limits.OutputBytes = 32
	e.BindingDigest = ""
	b := bytes.TrimSuffix(e.JSON(), []byte("\n"))
	sum := sha256.Sum256(b)
	e.BindingDigest = hex.EncodeToString(sum[:])
	return e, nil
}

// CheckProof validates only fixed execution envelopes. The human audit is a
// separate trusted boundary; a digest or successful check is never consent.
func CheckProof(r io.Reader, binding, revision, runID, attempt string, now time.Time) (Envelope, error) {
	if !hex64.MatchString(binding) {
		return Envelope{}, invalid
	}
	b, err := io.ReadAll(io.LimitReader(r, MaxInput+1))
	if err != nil || len(b) > MaxInput {
		return Envelope{}, invalid
	}
	var e Envelope
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&e) != nil {
		return Envelope{}, invalid
	}
	var tail any
	if d.Decode(&tail) != io.EOF || !uniqueKeys(json.NewDecoder(bytes.NewReader(b))) {
		return Envelope{}, invalid
	}
	issued, err := time.Parse(time.RFC3339, e.IssuedAt)
	if err != nil || now.Before(issued) || !now.Before(issued.Add(time.Hour)) {
		return Envelope{}, invalid
	}
	meta := ownedimage.Metadata{ImageID: e.Plan.Image, ArchiveSHA256: e.ArchiveSHA256, BinarySHA256: e.BinarySHA256, MarkerSHA256: e.MarkerSHA256, LayerDiffID: e.LayerDiffID, Scope: "preparation-only"}
	want, err := PrepareProof(e.Plan.Snapshot, meta, revision, runID, attempt, issued)
	if err != nil || binding != want.BindingDigest || e.Cases == nil || *e.Cases != *want.Cases {
		return Envelope{}, invalid
	}
	e.Cases = want.Cases // Compare values, never pointer identity.
	if e != want {
		return Envelope{}, invalid
	}
	// Preserve the existing boundary's exact spellings/types, rejecting case aliases
	// and explicit extra empty fields despite encoding/json's typed aliases.
	var actual, expected any
	if json.Unmarshal(b, &actual) != nil || json.Unmarshal(want.JSON(), &expected) != nil {
		return Envelope{}, invalid
	}
	a, _ := json.Marshal(actual)
	w, _ := json.Marshal(expected)
	if !bytes.Equal(a, w) {
		return Envelope{}, invalid
	}
	return want, nil
}
