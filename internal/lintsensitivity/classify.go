// Package lintsensitivity classifies supplied owned-fixture observations without
// I/O or execution. Acceptance is not authentication, isolation proof, or a claim
// that ESLint or CI ran. This package is not connected to the CLI or runner.
package lintsensitivity

import (
	"crypto/sha256"
	_ "embed"
	"fmt"
	"regexp"
)

type Role string

const (
	Clean    Role = "CLEAN"
	Fault    Role = "FAULT"
	Restored Role = "RESTORED"
)

// Status must come from a future trusted harness, not from ESLint's exit code.
// Completed asserts validation was reached, bounded output was decoded and
// cleanup/source preservation were verified. This package cannot prove that.
type Status string

const (
	Completed         Status = "COMPLETED"
	Denied            Status = "DENIED"
	Unavailable       Status = "UNAVAILABLE"
	TimedOut          Status = "TIMED_OUT"
	Crashed           Status = "CRASHED"
	CleanupUnverified Status = "CLEANUP_UNVERIFIED"
	InvalidOutput     Status = "INVALID_OUTPUT"
)

// Identity binds the triplet's execution context. Shape checks do not approve a
// version/image: a future reviewed catalog must pin and authenticate both.
type Identity struct {
	Tool, Version, ConfigSHA256, Image, Environment, OS, Snapshot string
}

// Diagnostic is normalized by a future bounded adapter, not a raw log match.
type Diagnostic struct {
	File, Rule             string
	Severity, Line, Column int
	Fatal                  bool
}

// Observation contains no executable configuration, paths to load, or commands.
// Slice order is the asserted execution order; it is not independently verified.
type Observation struct {
	Role          Role
	Identity      Identity
	FixtureSHA256 string
	Status        Status
	ExitCode      int
	Diagnostics   []Diagnostic
}

type Outcome string

const (
	Sensitive    Outcome = "SENSITIVE"
	Insensitive  Outcome = "INSENSITIVE"
	Unverifiable Outcome = "UNVERIFIABLE"
)

// Result is an internal decision, not a v1 evidence report or public JSON format.
// Scope is always TOOL_FIXTURE; it cannot upgrade project configuration or CI.
type Result struct {
	Outcome Outcome
	Reason  string
	Scope   string
}

// Fixture is owned data only. The same virtual filename must be used in each run.
type Fixture struct {
	Source, SHA256 string
}

//go:embed testdata/clean.js
var clean string

//go:embed testdata/fault.js
var fault string

//go:embed testdata/restored.js
var restored string

//go:embed testdata/config.json
var config string

func digest(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

// FixtureFor returns immutable owned source, never a repository fixture.
func FixtureFor(role Role) (Fixture, bool) {
	var source string
	switch role {
	case Clean:
		source = clean
	case Fault:
		source = fault
	case Restored:
		source = restored
	default:
		return Fixture{}, false
	}
	return Fixture{source, digest(source)}, true
}

// Config returns a copy of the owned rule recipe, not a loadable ESLint config.
// A future adapter must compose reviewed configuration without target discovery.
func Config() []byte { return []byte(config) }

func ConfigSHA256() string { return digest(config) }

var (
	sha         = regexp.MustCompile(`^[a-f0-9]{64}$`)
	image       = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	version     = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	environment = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]*$`)
)

func valid(id Identity) bool {
	return id.Tool == "eslint" && version.MatchString(id.Version) &&
		id.ConfigSHA256 == ConfigSHA256() && image.MatchString(id.Image) &&
		environment.MatchString(id.Environment) && id.OS == "LINUX" && sha.MatchString(id.Snapshot)
}

// Classify checks supplied observations only; it never starts a process.
// Caller order must be CLEAN, FAULT, RESTORED. Any ambiguous or incomplete run
// fails closed. Even a positive result requires future authenticated runtime
// evidence before it can be published as observed efficacy.
func Classify(observations []Observation) Result {
	unknown := func(reason string) Result { return Result{Unverifiable, reason, "TOOL_FIXTURE"} }
	if len(observations) != 3 {
		return unknown("INVALID_SEQUENCE")
	}
	roles := [...]Role{Clean, Fault, Restored}
	for i, o := range observations {
		if o.Role != roles[i] {
			return unknown("INVALID_SEQUENCE")
		}
	}
	id := observations[0].Identity
	if !valid(id) {
		return unknown("UNSUPPORTED_IDENTITY")
	}
	for _, o := range observations {
		if o.Identity != id {
			return unknown("IDENTITY_MISMATCH")
		}
		f, _ := FixtureFor(o.Role)
		if o.FixtureSHA256 != f.SHA256 {
			return unknown("FIXTURE_MISMATCH")
		}
		if o.Status != Completed || o.ExitCode < 0 || o.ExitCode > 1 {
			return unknown("HARNESS_FAILURE")
		}
	}
	for _, i := range []int{0, 2} {
		o := observations[i]
		if o.ExitCode != 0 || len(o.Diagnostics) != 0 {
			return unknown("CONTROL_FAILED")
		}
	}
	fault := observations[1]
	if fault.ExitCode == 0 && len(fault.Diagnostics) == 0 {
		return Result{Insensitive, "FAULT_ACCEPTED", "TOOL_FIXTURE"}
	}
	expected := Diagnostic{File: "fixture.js", Rule: "no-debugger", Severity: 2, Line: 1, Column: 1}
	if fault.ExitCode != 1 || len(fault.Diagnostics) != 1 || fault.Diagnostics[0] != expected {
		return unknown("UNEXPECTED_DIAGNOSTIC")
	}
	return Result{Sensitive, "EXPECTED_DIAGNOSTIC", "TOOL_FIXTURE"}
}
