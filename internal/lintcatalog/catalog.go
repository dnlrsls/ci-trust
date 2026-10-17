// Package lintcatalog owns one compiled lint identity and its closed gate.
// Metadata returned by Current is not approval or execution authority.
package lintcatalog

// State is a value-owned description. Mutating a returned copy cannot change
// compiled policy. Consumers must read BlockedReason, not trust a supplied State.
type State struct {
	Image, Archive, Layer, Node, ESLint string
	SecurityHold, Admitted              bool
}

const securityHold = true
const imageAdmitted = false

func Current() State {
	return State{
		Image:   "sha256:f047932f89d4798983a5c5e13ec32843c00fd44795844ea7969c51a15d8289dc",
		Archive: "sha256:371afa8ec9404f48a353782f3654975ff80349b67130d4986d44e8f3edca3ceb",
		Layer:   "sha256:97343ac30b461e4faef346c9ffe56a047efe90699c5aeda727bc987ada93cad7",
		Node:    "24.22.0", ESLint: "10.12.0",
		SecurityHold: securityHold, Admitted: imageAdmitted,
	}
}

// BlockedReason reads compiled state only. No caller flag, decoded report,
// imported plan or UI channel choice can change this gate.
func BlockedReason() string {
	if securityHold {
		return "NODE_SECURITY_HOLD"
	}
	if !imageAdmitted {
		return "LINT_IMAGE_NOT_ADMITTED"
	}
	return ""
}
