package lintsensitivity

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// These observations are authored inputs, not captured ESLint output or proof.
func observations(t *testing.T) []Observation {
	t.Helper()
	id := Identity{Tool: "eslint", Version: "9.0.0", ConfigSHA256: ConfigSHA256(),
		Image: "sha256:" + strings.Repeat("a", 64), Environment: "owned-test",
		OS: "LINUX", Snapshot: strings.Repeat("b", 64)}
	var out []Observation
	for _, role := range []Role{Clean, Fault, Restored} {
		f, ok := FixtureFor(role)
		if !ok {
			t.Fatal("missing owned fixture")
		}
		out = append(out, Observation{Role: role, Identity: id, FixtureSHA256: f.SHA256,
			Status: Completed, ExitCode: 0})
	}
	out[1].ExitCode = 1
	out[1].Diagnostics = []Diagnostic{{File: "fixture.js", Rule: "no-debugger", Severity: 2, Line: 1, Column: 1}}
	return out
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name   string
		edit   func([]Observation) []Observation
		want   Outcome
		reason string
	}{
		{"sensitive", func(o []Observation) []Observation { return o }, Sensitive, "EXPECTED_DIAGNOSTIC"},
		{"empty", func(o []Observation) []Observation { return nil }, Unverifiable, "INVALID_SEQUENCE"},
		{"noop", func(o []Observation) []Observation { o[1].ExitCode = 0; o[1].Diagnostics = nil; return o }, Insensitive, "FAULT_ACCEPTED"},
		{"missing role", func(o []Observation) []Observation { return o[:2] }, Unverifiable, "INVALID_SEQUENCE"},
		{"duplicate role", func(o []Observation) []Observation { o[2].Role = Fault; return o }, Unverifiable, "INVALID_SEQUENCE"},
		{"reordered", func(o []Observation) []Observation { o[0], o[1] = o[1], o[0]; return o }, Unverifiable, "INVALID_SEQUENCE"},
		{"extra role", func(o []Observation) []Observation { return append(o, o[2]) }, Unverifiable, "INVALID_SEQUENCE"},
		{"wrong fixture", func(o []Observation) []Observation { o[1].FixtureSHA256 = o[0].FixtureSHA256; return o }, Unverifiable, "FIXTURE_MISMATCH"},
		{"wrong rule", func(o []Observation) []Observation { o[1].Diagnostics[0].Rule = "no-unused-vars"; return o }, Unverifiable, "UNEXPECTED_DIAGNOSTIC"},
		{"warning", func(o []Observation) []Observation { o[1].Diagnostics[0].Severity = 1; return o }, Unverifiable, "UNEXPECTED_DIAGNOSTIC"},
		{"fatal parse", func(o []Observation) []Observation { o[1].Diagnostics[0].Fatal = true; return o }, Unverifiable, "UNEXPECTED_DIAGNOSTIC"},
		{"wrong location", func(o []Observation) []Observation { o[1].Diagnostics[0].Line = 2; return o }, Unverifiable, "UNEXPECTED_DIAGNOSTIC"},
		{"wrong column", func(o []Observation) []Observation { o[1].Diagnostics[0].Column = 2; return o }, Unverifiable, "UNEXPECTED_DIAGNOSTIC"},
		{"wrong file", func(o []Observation) []Observation { o[1].Diagnostics[0].File = "target.js"; return o }, Unverifiable, "UNEXPECTED_DIAGNOSTIC"},
		{"exit only", func(o []Observation) []Observation { o[1].Diagnostics = nil; return o }, Unverifiable, "UNEXPECTED_DIAGNOSTIC"},
		{"extra diagnostic", func(o []Observation) []Observation {
			o[1].Diagnostics = append(o[1].Diagnostics, o[1].Diagnostics[0])
			return o
		}, Unverifiable, "UNEXPECTED_DIAGNOSTIC"},
		{"exit zero with error", func(o []Observation) []Observation { o[1].ExitCode = 0; return o }, Unverifiable, "UNEXPECTED_DIAGNOSTIC"},
		{"crash exit", func(o []Observation) []Observation { o[1].ExitCode = 2; return o }, Unverifiable, "HARNESS_FAILURE"},
		{"signal exit", func(o []Observation) []Observation { o[1].ExitCode = -1; return o }, Unverifiable, "HARNESS_FAILURE"},
		{"clean fails", func(o []Observation) []Observation { o[0].ExitCode = 1; return o }, Unverifiable, "CONTROL_FAILED"},
		{"restoration fails", func(o []Observation) []Observation { o[2].ExitCode = 1; return o }, Unverifiable, "CONTROL_FAILED"},
		{"dirty control", func(o []Observation) []Observation { o[2].Diagnostics = o[1].Diagnostics; return o }, Unverifiable, "CONTROL_FAILED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := tc.edit(observations(t))
			before := clone(o)
			got := Classify(o)
			if got.Outcome != tc.want || got.Reason != tc.reason || got.Scope != "TOOL_FIXTURE" {
				t.Fatalf("got %+v, want %s/%s/TOOL_FIXTURE", got, tc.want, tc.reason)
			}
			if !reflect.DeepEqual(o, before) {
				t.Fatal("input mutated")
			}
		})
	}
}

func clone(in []Observation) []Observation {
	out := append([]Observation(nil), in...)
	for i := range out {
		if in[i].Diagnostics != nil {
			out[i].Diagnostics = append([]Diagnostic{}, in[i].Diagnostics...)
		}
	}
	return out
}

func TestUnavailableNeverSensitive(t *testing.T) {
	for _, status := range []Status{Denied, Unavailable, TimedOut, Crashed, CleanupUnverified, InvalidOutput, "", "OTHER"} {
		for i := range 3 {
			o := observations(t)
			o[i].Status = status
			got := Classify(o)
			if got.Outcome != Unverifiable || got.Reason != "HARNESS_FAILURE" {
				t.Fatalf("role %d status %q: %+v", i, status, got)
			}
		}
	}
}

func TestIdentity(t *testing.T) {
	edits := map[string]func(*Identity){
		"tool":        func(id *Identity) { id.Tool = "noop" },
		"version":     func(id *Identity) { id.Version = "9.1.0" },
		"config":      func(id *Identity) { id.ConfigSHA256 = strings.Repeat("c", 64) },
		"image":       func(id *Identity) { id.Image = "sha256:" + strings.Repeat("c", 64) },
		"environment": func(id *Identity) { id.Environment = "other" },
		"os":          func(id *Identity) { id.OS = "WINDOWS" },
		"snapshot":    func(id *Identity) { id.Snapshot = strings.Repeat("c", 64) },
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			o := observations(t)
			edit(&o[1].Identity)
			if got := Classify(o); got.Outcome != Unverifiable || got.Reason != "IDENTITY_MISMATCH" {
				t.Fatalf("got %+v", got)
			}
		})
	}
	invalid := map[string]func(*Identity){
		"unknown tool":        func(id *Identity) { id.Tool = "" },
		"unknown version":     func(id *Identity) { id.Version = "" },
		"floating version":    func(id *Identity) { id.Version = "^9.0.0" },
		"unknown environment": func(id *Identity) { id.Environment = "" },
		"tagged image":        func(id *Identity) { id.Image = "eslint:latest" },
		"unknown snapshot":    func(id *Identity) { id.Snapshot = "" },
		"repository config":   func(id *Identity) { id.ConfigSHA256 = "eslint.config.js" },
		"other config":        func(id *Identity) { id.ConfigSHA256 = strings.Repeat("c", 64) },
		"unsupported os":      func(id *Identity) { id.OS = "WINDOWS" },
	}
	for name, edit := range invalid {
		t.Run(name, func(t *testing.T) {
			o := observations(t)
			for i := range o {
				edit(&o[i].Identity)
			}
			if got := Classify(o); got.Outcome != Unverifiable || got.Reason != "UNSUPPORTED_IDENTITY" {
				t.Fatalf("got %+v", got)
			}
		})
	}
}

func TestOwnedFixtures(t *testing.T) {
	want := map[Role]string{Clean: "const answer = 42;\nvoid answer;\n", Fault: "debugger;\nconst answer = 42;\nvoid answer;\n", Restored: "const answer = 42;\nvoid answer;\n"}
	for role, source := range want {
		f, ok := FixtureFor(role)
		if !ok || f.Source != source || f.SHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(source))) {
			t.Fatalf("unexpected %s fixture: %+v", role, f)
		}
	}
	if _, ok := FixtureFor("target.js"); ok {
		t.Fatal("accepted arbitrary fixture")
	}
	if string(Config()) != "{\"rules\":{\"no-debugger\":\"error\"}}\n" {
		t.Fatal("unexpected owned recipe")
	}
	if ConfigSHA256() != fmt.Sprintf("%x", sha256.Sum256(Config())) {
		t.Fatal("recipe digest does not bind recipe bytes")
	}
	b := Config()
	b[0] = '!'
	if Config()[0] != '{' {
		t.Fatal("recipe is mutable")
	}
}
