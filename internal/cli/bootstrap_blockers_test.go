package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/bootstrap"
	"github.com/dnlrsls/ci-trust/internal/discovery"
)

func TestBootstrapActionableCollectionBlockers(t *testing.T) {
	cases := []struct {
		name        string
		diagnostics []discovery.Diagnostic
		limits      []string
		want        []string
	}{
		{name: "entry limit", diagnostics: []discovery.Diagnostic{{Code: "ENTRY_LIMIT", Path: "packages/check", Message: "PRIVATE_PAYLOAD", State: "UNKNOWN"}}, want: []string{`Collection blocker ENTRY_LIMIT at "packages/check": Entry limit reached; select a smaller complete repository scope before retrying.`}},
		{name: "all blockers", diagnostics: []discovery.Diagnostic{{Code: "SYMLINK_SKIPPED", Path: "linked", State: "UNKNOWN"}, {Code: "FILE_READ", Path: "package.json", State: "UNKNOWN"}}, want: []string{`Collection blocker SYMLINK_SKIPPED at "linked": Symbolic link skipped; review the link and provide regular in-root inputs before retrying.`, `Collection blocker FILE_READ at "package.json": Input could not be collected safely; review the omitted input and retry only with a complete supported source scope.`}},
		{name: "snapshot limit", limits: []string{"PRIVATE_PAYLOAD"}, want: []string{`Collection blocker SNAPSHOT_LIMITATION at [unavailable]: Snapshot collection is limited; review the collection limits before requesting a draft.`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := discovery.Snapshot{Documents: []discovery.Document{{Path: "package.json", Content: []byte(`{"scripts":{"test":"node --test"}}`)}, {Path: ".ci-trust.json", Content: []byte(`{"version":1,"platforms":["linux"]}`)}}, Diagnostics: tc.diagnostics, Limitations: tc.limits}
			assertCollectionCLI(t, s, tc.want)
		})
	}
}

func TestBootstrapBlockerWithholdsUnsafeData(t *testing.T) {
	for _, p := range []string{"", "/private/token", "C:/private/token", "../private/token", "a/../private/token", `a\token`, "bad\nname", "bad\x1bname", "bad\u202ename", strings.Repeat("x", 513)} {
		t.Run(fmt.Sprintf("%q", p), func(t *testing.T) {
			s := discovery.Snapshot{Diagnostics: []discovery.Diagnostic{{Code: "PRIVATE_PAYLOAD", Path: p, Message: "PRIVATE_PAYLOAD", State: "UNKNOWN"}}}
			assertCollectionCLI(t, s, []string{`Collection blocker UNRESOLVED_COLLECTION at [unavailable]: Input could not be collected safely; review the omitted input and retry only with a complete supported source scope.`})
		})
	}
}

func assertCollectionCLI(t *testing.T, s discovery.Snapshot, want []string) {
	t.Helper()
	before, _ := json.Marshal(s)
	plan, err := bootstrap.Analyze(s, bootstrap.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Recommendations) != 0 || !plan.DryRun {
		t.Fatal("collection guard changed")
	}
	for _, line := range append([]string{"Partial or unsafe collection: destination and source scope are unresolved; no draft proposed."}, want...) {
		if !strings.Contains(strings.Join(plan.Limitations, "\n"), line) {
			t.Fatalf("missing %q in %v", line, plan.Limitations)
		}
	}
	var report struct {
		Diagnostics []sourceDiagnostic `json:"diagnostics"`
	}
	if err := json.Unmarshal(plan.SourceReport, &report); err != nil {
		t.Fatal(err)
	}
	var wantStderr strings.Builder
	for _, d := range report.Diagnostics {
		fmt.Fprintf(&wantStderr, "%s: %s\n", d.Code, d.Message)
	}
	for _, format := range []string{"json", "human"} {
		var previous string
		for i := 0; i < 2; i++ {
			var out, stderr bytes.Buffer
			calls := 0
			code := Run([]string{"bootstrap", "target", "--dry-run", "--format", format}, nil, &out, &stderr, Services{Bootstrap: func(string, []string) (bootstrap.Plan, error) {
				calls++
				return bootstrap.Analyze(s, bootstrap.Options{})
			}})
			if code != 0 || calls != 1 || stderr.String() != wantStderr.String() {
				t.Fatalf("exit=%d calls=%d stderr=%q want=%q", code, calls, stderr.String(), wantStderr.String())
			}
			for _, line := range want {
				encoded, _ := json.Marshal(line)
				if !bytes.Contains(out.Bytes(), encoded) {
					t.Fatalf("%s missing complete blocker %s", format, encoded)
				}
			}
			if strings.Contains(out.String(), "PRIVATE_PAYLOAD") || strings.ContainsAny(out.String(), "\x1b\u202e") {
				t.Fatal("untrusted diagnostic payload exposed")
			}
			if i > 0 && previous != out.String() {
				t.Fatal("nondeterministic CLI output")
			}
			previous = out.String()
			if format == "json" {
				var got bootstrap.Plan
				if json.Unmarshal(out.Bytes(), &got) != nil || len(got.Recommendations) != 0 || !got.DryRun {
					t.Fatal("invalid/no-draft contract changed")
				}
			}
		}
	}
	after, _ := json.Marshal(s)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("source snapshot changed")
	}
}

func TestBootstrapCleanConditionalBehaviorStillHolds(t *testing.T) {
	s := discovery.Snapshot{Documents: []discovery.Document{{Path: "package.json", Content: []byte(`{"scripts":{"test":"node --test"}}`)}, {Path: ".ci-trust.json", Content: []byte(`{"version":1,"platforms":["linux"]}`)}}}
	p, err := bootstrap.Analyze(s, bootstrap.Options{})
	if err != nil || len(p.Recommendations) != 1 || len(p.Limitations) != 1 {
		t.Fatalf("ordinary conditional plan changed: %v %+v", err, p)
	}
	s.Diagnostics = []discovery.Diagnostic{{Code: "EXCLUDED_REPOSITORY", Path: "checkout", State: "UNKNOWN"}}
	p, err = bootstrap.Analyze(s, bootstrap.Options{})
	if err != nil || len(p.Recommendations) != 1 {
		t.Fatalf("intentional outside-destination repository boundary changed: %v", err)
	}
}
