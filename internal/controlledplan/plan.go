// Package controlledplan prepares run-bound review data, never execution consent.
package controlledplan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"time"

	"github.com/dnlrsls/ci-trust/internal/runner"
)

const MaxInput = 16 * 1024

// Envelope is preparation data. Field order defines the canonical JSON encoding.
// BindingDigest is omitted only while calculating the binding itself.
type Envelope struct {
	Plan          runner.Plan `json:"plan"`
	PlanDigest    string      `json:"plan_digest"`
	BindingDigest string      `json:"binding_digest,omitempty"`
	Revision      string      `json:"revision"`
	RunID         string      `json:"run_id"`
	RunAttempt    string      `json:"run_attempt"`
	Repository    string      `json:"repository"`
	Version       int         `json:"version"`
	Scope         string      `json:"scope"`
}

var hex64 = regexp.MustCompile(`^[a-f0-9]{64}$`)
var revision40 = regexp.MustCompile(`^[a-f0-9]{40}$`)
var image = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var decimal = regexp.MustCompile(`^[1-9][0-9]*$`)
var invalid = errors.New("invalid preparation envelope")

// Prepare constructs the sole fixed profile; it does not authorize any execution.
func Prepare(snapshot, imageID, revision, runID, attempt string) (Envelope, error) {
	if !hex64.MatchString(snapshot) || !image.MatchString(imageID) || !revision40.MatchString(revision) || !decimal.MatchString(runID) || !decimal.MatchString(attempt) || len(runID) > MaxInput/4 || len(attempt) > MaxInput/4 {
		return Envelope{}, invalid
	}
	p := runner.Plan{Snapshot: snapshot, Adapter: "owned-smoke-v1", Tool: "owned-smoke-v1", Image: imageID, Fixture: "smoke-v1", Config: "owned-v1", Limits: runner.Limits{Wall: 10 * time.Second, MemoryBytes: 128 << 20, CPUs: 1, PIDs: 32, OutputBytes: 64 << 10, ScratchBytes: 8 << 20}, Effects: "owned fixture only; no host mounts; no network"}
	e := Envelope{Plan: p, PlanDigest: p.Digest(), Revision: revision, RunID: runID, RunAttempt: attempt, Repository: "dnlrsls/ci-trust", Version: 1, Scope: "preparation-only"}
	sum := sha256.Sum256(bytes.TrimSuffix(e.JSON(), []byte("\n")))
	e.BindingDigest = hex.EncodeToString(sum[:])
	return e, nil
}

// JSON returns canonical compact JSON followed by one newline.
func (e Envelope) JSON() []byte { b, _ := json.Marshal(e); return append(b, '\n') }

// Display uses the runner's complete human-readable plan, not a second renderer.
func (e Envelope) Display() string { return e.Plan.Display() }

// Check independently reconstructs the profile and matches externally pinned context.
// Canonical structural comparison rejects aliases, duplicates, unknown and missing fields.
func Check(r io.Reader, binding, revision, runID, attempt string) (Envelope, error) {
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
	if d.Decode(&tail) != io.EOF {
		return Envelope{}, invalid
	}
	want, err := Prepare(e.Plan.Snapshot, e.Plan.Image, revision, runID, attempt)
	if err != nil || e != want || binding != want.BindingDigest {
		return Envelope{}, invalid
	}
	// Decode a second time as tokens, requiring the exact canonical key spellings,
	// multiplicity and value types. Whitespace and key ordering are insignificant.
	var actual any
	if json.Unmarshal(b, &actual) != nil {
		return Envelope{}, invalid
	}
	canonical, _ := json.Marshal(actual)
	var expected any
	_ = json.Unmarshal(want.JSON(), &expected)
	expectedJSON, _ := json.Marshal(expected)
	if !bytes.Equal(canonical, expectedJSON) || !uniqueKeys(json.NewDecoder(bytes.NewReader(b))) {
		return Envelope{}, invalid
	}
	return want, nil
}

func uniqueKeys(d *json.Decoder) bool {
	token, err := d.Token()
	if err != nil {
		return false
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return false
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return false
			}
			seen[s] = true
			if !uniqueKeys(d) {
				return false
			}
		}
	case '[':
		for d.More() {
			if !uniqueKeys(d) {
				return false
			}
		}
	default:
		return false
	}
	_, err = d.Token()
	return err == nil
}
