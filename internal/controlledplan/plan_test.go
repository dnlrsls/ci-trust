package controlledplan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/runner"
)

func fixture(t *testing.T) Envelope {
	t.Helper()
	e, err := Prepare(strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64), strings.Repeat("c", 40), "123", "2")
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func TestProfileAndRoundtrip(t *testing.T) {
	e := fixture(t)
	want := runner.Plan{Snapshot: strings.Repeat("a", 64), Adapter: "owned-smoke-v1", Tool: "owned-smoke-v1", Image: "sha256:" + strings.Repeat("b", 64), Fixture: "smoke-v1", Config: "owned-v1", Limits: runner.Limits{Wall: 10 * time.Second, MemoryBytes: 134217728, CPUs: 1, PIDs: 32, OutputBytes: 65536, ScratchBytes: 8388608}, Effects: "owned fixture only; no host mounts; no network"}
	if e.Plan != want || e.PlanDigest != want.Digest() || e.Repository != "dnlrsls/ci-trust" || e.Version != 1 || e.Scope != "preparation-only" {
		t.Fatalf("wrong profile: %+v", e)
	}
	if e.Display() != want.Display() {
		t.Fatal("display diverged")
	}
	b := e.JSON()
	got, err := Check(bytes.NewReader(b), e.BindingDigest, e.Revision, e.RunID, e.RunAttempt)
	if err != nil || !bytes.Equal(got.JSON(), b) {
		t.Fatalf("roundtrip: %v", err)
	}
}
func TestPrepareRejectsInvalidContext(t *testing.T) {
	base := []string{strings.Repeat("a", 64), "sha256:" + strings.Repeat("b", 64), strings.Repeat("c", 40), "123", "2"}
	for i := range base {
		for _, bad := range []string{"", "0", "01", "-1", " 1", "1\n", strings.Repeat("A", 64)} {
			t.Run(string(rune('a'+i))+bad, func(t *testing.T) {
				args := append([]string(nil), base...)
				args[i] = bad
				if _, err := Prepare(args[0], args[1], args[2], args[3], args[4]); err == nil {
					t.Fatal("accepted invalid field")
				}
			})
		}
	}
}
func TestRejectEveryEnvelopeMutation(t *testing.T) {
	mutations := map[string]func(*Envelope){
		"snapshot": func(e *Envelope) { e.Plan.Snapshot = strings.Repeat("d", 64) }, "image": func(e *Envelope) { e.Plan.Image = "sha256:" + strings.Repeat("d", 64) },
		"adapter": func(e *Envelope) { e.Plan.Adapter = "other" }, "tool": func(e *Envelope) { e.Plan.Tool = "other" }, "fixture": func(e *Envelope) { e.Plan.Fixture = "other" }, "config": func(e *Envelope) { e.Plan.Config = "other" }, "effects": func(e *Envelope) { e.Plan.Effects = "other" },
		"transport": func(e *Envelope) { e.Plan.Transport = "split-stdin-v1" },
		"wall":      func(e *Envelope) { e.Plan.Limits.Wall++ }, "memory": func(e *Envelope) { e.Plan.Limits.MemoryBytes++ }, "cpus": func(e *Envelope) { e.Plan.Limits.CPUs++ }, "pids": func(e *Envelope) { e.Plan.Limits.PIDs++ }, "output": func(e *Envelope) { e.Plan.Limits.OutputBytes++ }, "scratch": func(e *Envelope) { e.Plan.Limits.ScratchBytes++ },
		"revision": func(e *Envelope) { e.Revision = strings.Repeat("d", 40) }, "run": func(e *Envelope) { e.RunID = "124" }, "attempt": func(e *Envelope) { e.RunAttempt = "3" }, "repository": func(e *Envelope) { e.Repository = "other" }, "version": func(e *Envelope) { e.Version = 2 }, "scope": func(e *Envelope) { e.Scope = "execution" }, "plan digest": func(e *Envelope) { e.PlanDigest = strings.Repeat("d", 64) }, "binding": func(e *Envelope) { e.BindingDigest = strings.Repeat("d", 64) },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			e := fixture(t)
			changed := e
			mutate(&changed)
			b, _ := json.Marshal(changed)
			if _, err := Check(bytes.NewReader(b), e.BindingDigest, e.Revision, e.RunID, e.RunAttempt); err == nil {
				t.Fatal("accepted mutation")
			}
		})
	}
}

func TestRejectReboundSplitTransport(t *testing.T) {
	e := fixture(t)
	e.Plan.Transport = "split-stdin-v1"
	e.PlanDigest = e.Plan.Digest()
	e.BindingDigest = ""
	sum := sha256.Sum256(bytes.TrimSuffix(e.JSON(), []byte("\n")))
	e.BindingDigest = hex.EncodeToString(sum[:])
	// A caller can calculate every digest, but cannot expand the fixed public
	// preparation profile. A new transport needs its own reviewed composition.
	if _, err := Check(bytes.NewReader(e.JSON()), e.BindingDigest, e.Revision, e.RunID, e.RunAttempt); err == nil {
		t.Fatal("self-rebound transport expanded public profile")
	}
}
func TestStrictJSONAndExternalContext(t *testing.T) {
	e := fixture(t)
	b := e.JSON()
	for name, bad := range map[string][]byte{"extra": []byte(strings.Replace(string(b), `"version":1`, `"extra":0,"version":1`, 1)), "duplicate": []byte(strings.Replace(string(b), `"version":1`, `"version":1,"version":1`, 1)), "nested duplicate": []byte(strings.Replace(string(b), `"CPUs":1`, `"CPUs":1,"CPUs":1`, 1)), "case alias": []byte(strings.Replace(string(b), `"version":1`, `"Version":1`, 1)), "trailing": append(append([]byte(nil), b...), []byte("{}")...), "oversize": bytes.Repeat([]byte(" "), MaxInput+1), "null": []byte("null")} {
		t.Run(name, func(t *testing.T) {
			if _, err := Check(bytes.NewReader(bad), e.BindingDigest, e.Revision, e.RunID, e.RunAttempt); err == nil {
				t.Fatal("accepted malformed JSON")
			}
		})
	}
	for i := 0; i < 4; i++ {
		args := []string{e.BindingDigest, e.Revision, e.RunID, e.RunAttempt}
		args[i] = "1"
		if _, err := Check(bytes.NewReader(b), args[0], args[1], args[2], args[3]); err == nil {
			t.Fatal("accepted stale external context")
		}
	}
}
