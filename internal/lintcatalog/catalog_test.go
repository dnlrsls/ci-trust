package lintcatalog_test

import (
	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	"testing"
)

func TestCompiledCatalogHasExactHeldIdentity(t *testing.T) {
	want := lintcatalog.State{
		Image:   "sha256:f047932f89d4798983a5c5e13ec32843c00fd44795844ea7969c51a15d8289dc",
		Archive: "sha256:371afa8ec9404f48a353782f3654975ff80349b67130d4986d44e8f3edca3ceb",
		Layer:   "sha256:97343ac30b461e4faef346c9ffe56a047efe90699c5aeda727bc987ada93cad7",
		Node:    "24.22.0", ESLint: "10.12.0", SecurityHold: true, Admitted: false,
	}
	if got := lintcatalog.Current(); got != want || lintcatalog.BlockedReason() != "NODE_SECURITY_HOLD" {
		t.Fatalf("catalog=%+v reason=%q", got, lintcatalog.BlockedReason())
	}
}
func TestCallerCopyCannotCreateAdmission(t *testing.T) {
	copy := lintcatalog.Current()
	copy.SecurityHold = false
	copy.Admitted = true
	copy.Image = "caller-supplied"
	if lintcatalog.Current() == copy || !lintcatalog.Current().SecurityHold || lintcatalog.Current().Admitted || lintcatalog.BlockedReason() != "NODE_SECURITY_HOLD" {
		t.Fatal("caller metadata changed compiled authority")
	}
}
