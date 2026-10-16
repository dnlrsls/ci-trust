package controlledplan

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/ownedimage"
)

func proofFixture(t *testing.T) (Envelope, time.Time) {
	t.Helper()
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	meta := ownedimage.Metadata{ImageID: "sha256:" + strings.Repeat("b", 64), ArchiveSHA256: "sha256:" + strings.Repeat("d", 64), BinarySHA256: "sha256:" + strings.Repeat("e", 64), MarkerSHA256: "sha256:" + strings.Repeat("f", 64), LayerDiffID: "sha256:" + strings.Repeat("1", 64), Scope: "preparation-only"}
	e, err := PrepareProof(strings.Repeat("a", 64), meta, strings.Repeat("c", 40), "123", "1", now)
	if err != nil {
		t.Fatal(err)
	}
	return e, now
}

func TestProofPlanBindsExactCasesAndImage(t *testing.T) {
	e, now := proofFixture(t)
	if e.Scope != "owned-smoke-execution" || e.Plan.Config != "owned-proof-v1" || e.Cases == nil {
		t.Fatalf("wrong proof profile: %+v", e)
	}
	if e.PlanDigest != e.Plan.Digest() || e.Cases.Normal != e.Plan {
		t.Fatal("normal plan differs")
	}
	if e.Cases.Timeout.Limits.Wall != time.Second || e.Cases.ActiveCancel != e.Plan || e.Cases.OutputLimit.Limits.OutputBytes != 32 {
		t.Fatal("wrong fixed cases")
	}
	for _, p := range []struct {
		name              string
		planImage, config string
	}{{"timeout", e.Cases.Timeout.Image, e.Cases.Timeout.Config}, {"cancel", e.Cases.ActiveCancel.Image, e.Cases.ActiveCancel.Config}, {"output", e.Cases.OutputLimit.Image, e.Cases.OutputLimit.Config}} {
		if p.planImage != e.Plan.Image || p.config != "owned-proof-v1" {
			t.Fatalf("unbound case %s", p.name)
		}
	}
	if e.IssuedAt != now.Format(time.RFC3339) || e.ExpiresAt != now.Add(time.Hour).Format(time.RFC3339) || e.PreparationEffects == "" {
		t.Fatal("missing validity/effects")
	}
	got, err := CheckProof(bytes.NewReader(e.JSON()), e.BindingDigest, e.Revision, e.RunID, e.RunAttempt, now.Add(time.Minute))
	if err != nil || !bytes.Equal(got.JSON(), e.JSON()) {
		t.Fatalf("roundtrip: %v", err)
	}
	if _, err := Check(bytes.NewReader(e.JSON()), e.BindingDigest, e.Revision, e.RunID, e.RunAttempt); err == nil {
		t.Fatal("execution plan accepted by preparation-only boundary")
	}
}

func TestProofPlanRejectsExpiredChangedAndForgedInputs(t *testing.T) {
	e, now := proofFixture(t)
	for name, mutate := range map[string]func(*Envelope){
		"snapshot": func(p *Envelope) { p.Plan.Snapshot = strings.Repeat("2", 64) },
		"image":    func(p *Envelope) { p.Plan.Image = "sha256:" + strings.Repeat("2", 64) },
		"archive":  func(p *Envelope) { p.ArchiveSHA256 = "sha256:" + strings.Repeat("2", 64) },
		"binary":   func(p *Envelope) { p.BinarySHA256 = "sha256:" + strings.Repeat("2", 64) },
		"marker":   func(p *Envelope) { p.MarkerSHA256 = "sha256:" + strings.Repeat("2", 64) },
		"layer":    func(p *Envelope) { p.LayerDiffID = "sha256:" + strings.Repeat("2", 64) },
		"effects":  func(p *Envelope) { p.PreparationEffects = "other" },
		"expiry":   func(p *Envelope) { p.ExpiresAt = now.Add(2 * time.Hour).Format(time.RFC3339) },
		"limits":   func(p *Envelope) { p.Cases.Timeout.Limits.MemoryBytes++ },
		"scope":    func(p *Envelope) { p.Scope = "preparation-only" },
		"attempt":  func(p *Envelope) { p.RunAttempt = "2" },
	} {
		t.Run(name, func(t *testing.T) {
			p, _ := proofFixture(t)
			mutate(&p)
			if _, err := CheckProof(bytes.NewReader(p.JSON()), e.BindingDigest, e.Revision, e.RunID, e.RunAttempt, now.Add(time.Minute)); err == nil {
				t.Fatal("accepted changed plan")
			}
		})
	}
	for _, clock := range []time.Time{now.Add(-time.Second), now.Add(time.Hour), now.Add(2 * time.Hour)} {
		if _, err := CheckProof(bytes.NewReader(e.JSON()), e.BindingDigest, e.Revision, e.RunID, e.RunAttempt, clock); err == nil {
			t.Fatal("accepted future/expired plan")
		}
	}
	bad := strings.Replace(string(e.JSON()), `"scope":"owned-smoke-execution"`, `"Scope":"owned-smoke-execution"`, 1)
	if _, err := CheckProof(strings.NewReader(bad), e.BindingDigest, e.Revision, e.RunID, e.RunAttempt, now); err == nil {
		t.Fatal("accepted case alias")
	}
	var raw map[string]any
	if err := json.Unmarshal(e.JSON(), &raw); err != nil {
		t.Fatal(err)
	}
	raw["approved"] = true
	data, _ := json.Marshal(raw)
	if _, err := CheckProof(bytes.NewReader(data), e.BindingDigest, e.Revision, e.RunID, e.RunAttempt, now); err == nil {
		t.Fatal("accepted agent approval boolean")
	}
}
