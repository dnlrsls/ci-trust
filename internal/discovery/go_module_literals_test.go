package discovery

import (
	"bytes"
	"reflect"
	"testing"
)

// Public analysis must expose metadata only as ordinary Go conventions.
func TestStaticGoModule(t *testing.T) {
	const header = "module example.com/app\ngo 1.24\ntoolchain go1.24.1\n"
	cases := []struct {
		name, text string
		accepted   bool
	}{
		{"previous module go toolchain", header, true},
		{"single require", header + "require example.com/lib v1.2.3\n", true},
		{"single tool", header + "tool github.com/a-h/templ/cmd/templ\n", true},
		{"require block", header + "require (\nexample.com/lib v1.2.3 // indirect\nexample.com/other v0.0.0-20250101123456-abcdef123456\n)\n", true},
		{"tool block", header + "tool (\ngithub.com/a-h/templ/cmd/templ\nexample.com/tool/cmd/run\n)\n", true},
		{"combined blocks", header + "require (\nexample.com/lib v1.2.3+incompatible\n)\ntool (\nexample.com/tool\n)\n", true},
		{"empty blocks", header + "require (\n)\ntool (\n)\n", true},
		{"missing module", "go 1.24\nrequire example.com/lib v1.2.3", false},
		{"duplicate module", header + "module example.com/other", false},
		{"bad go", "module example.com/app\ngo latest", false},
		{"bad toolchain", "module example.com/app\ntoolchain auto", false},
		{"replace", header + "replace example.com/lib => ../lib", false},
		{"exclude", header + "exclude example.com/lib v1.2.3", false},
		{"retract", header + "retract v1.2.3", false},
		{"unclosed", header + "require (\nexample.com/lib v1.2.3", false},
		{"nested", header + "require (\ntool (\n)\n)", false},
		{"wrong block arity", header + "tool (\nexample.com/tool v1.2.3\n)", false},
		{"extra close", header + "tool (\n)\n)", false},
		{"inline block", header + "require ( example.com/lib v1.2.3 )", false},
		{"missing version", header + "require example.com/lib", false},
		{"dynamic version", header + "require example.com/lib $VERSION", false},
		{"version injection", header + "require example.com/lib v1.2.3;id", false},
		{"path injection", header + "tool example.com/tool;id", false},
		{"substitution", header + "tool $(id)", false},
		{"absolute tool", header + "tool /usr/bin/tool", false},
		{"traversal tool", header + "tool ../tool", false},
		{"absolute require", header + "require C:/lib v1.2.3", false},
		{"traversal require", header + "require example.com/../lib v1.2.3", false},
		{"version path", header + "require example.com/lib ../v1.2.3", false},
		{"quoted tool", header + "tool \"example.com/tool\"", false},
		{"unicode space", header + "tool\u00a0example.com/tool", false},
	}
	analyze := func(text string) Result {
		t.Helper()
		content := []byte(text)
		before := bytes.Clone(content)
		snapshot := Snapshot{Documents: []Document{{Path: "nested/go.mod", Content: content}}}
		result := Analyze(snapshot, Options{})
		if !bytes.Equal(content, before) || len(snapshot.Documents) != 1 || len(snapshot.Diagnostics) != 0 || len(snapshot.Limitations) != 0 {
			t.Fatal("analysis mutated input bytes or snapshot counters")
		}
		if again := Analyze(snapshot, Options{}); !reflect.DeepEqual(result, again) {
			t.Fatal("analysis is not deterministic")
		}
		return result
	}
	accepted := analyze(header)
	rejected := analyze(header + "unsupported value")
	if len(accepted.Roots) != 1 || len(accepted.Tools) != 1 || len(accepted.Candidates) != 2 || len(accepted.Diagnostics) != 0 {
		t.Fatalf("unexpected baseline: %+v", accepted)
	}
	for _, candidate := range accepted.Candidates {
		if candidate.State != "UNKNOWN" || candidate.Origin != "CONVENTION" || candidate.Root != accepted.Roots[0].ID || !reflect.DeepEqual(candidate.Chain, []Source{{Path: "nested/go.mod"}}) {
			t.Fatalf("invalid convention ownership: %+v", candidate)
		}
	}
	if len(rejected.Candidates) != 0 || len(rejected.Tools) != 0 || len(rejected.Diagnostics) != 1 || rejected.Diagnostics[0].Code != "malformed_go_module" || rejected.Diagnostics[0].State != "UNKNOWN" {
		t.Fatalf("unexpected rejection: %+v", rejected)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := rejected
			if tc.accepted {
				want = accepted
			}
			if got := analyze(tc.text); !reflect.DeepEqual(got, want) {
				t.Fatalf("public result differs:\ngot %+v\nwant %+v", got, want)
			}
		})
	}
}
