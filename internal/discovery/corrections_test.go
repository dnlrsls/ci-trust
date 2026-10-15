package discovery

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func reviewRejection(t *testing.T, input Snapshot, options Options, code string) {
	t.Helper()
	before := seamBytes(t, input)
	got := Analyze(input, options)
	if len(got.Roots) != 0 || len(got.Tools) != 0 || len(got.Candidates) != 0 || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != code || got.Diagnostics[0].Path != "" || got.Diagnostics[0].State != "UNKNOWN" {
		t.Fatalf("unbounded or partial analysis: roots=%d tools=%d candidates=%d diagnostics=%+v", len(got.Roots), len(got.Tools), len(got.Candidates), got.Diagnostics)
	}
	if !bytes.Equal(before, seamBytes(t, input)) {
		t.Fatal("rejection changed supplied snapshot")
	}
	if repeated := Analyze(input, options); !reflect.DeepEqual(got, repeated) {
		t.Fatal("budget rejection is not deterministic")
	}
}

func TestReviewAnalyzeCollectionBounds(t *testing.T) {
	for _, tc := range []struct {
		name string
		data Snapshot
	}{
		{"unsafe documents", Snapshot{Documents: []Document{{Path: "../package.json"}, {Path: "../package.json"}, {Path: "../package.json"}}}},
		{"diagnostics", Snapshot{Diagnostics: []Diagnostic{{Code: "a"}, {Code: "b"}, {Code: "c"}}}},
		{"combined records", Snapshot{Documents: []Document{{Path: "package.json", Content: []byte(`{}`)}}, Diagnostics: []Diagnostic{{Code: "a"}}}},
		{"limitations", Snapshot{Limitations: []string{"a", "b", "c"}}},
	} {
		t.Run(tc.name, func(t *testing.T) { reviewRejection(t, tc.data, Options{MaxEntries: 1}, "entry_limit") })
	}
	// A document exactly at the collection boundary remains analyzable.
	got := Analyze(Snapshot{Documents: []Document{{Path: "package.json", Content: []byte(`{}`)}}}, Options{MaxEntries: 1})
	if len(got.Roots) != 1 || len(got.Diagnostics) != 0 {
		t.Fatalf("exact collection boundary rejected: %+v", got)
	}
}

func TestReviewAnalyzeOutputBound(t *testing.T) {
	scripts := map[string]string{}
	for i := 0; i < 2500; i++ {
		scripts[fmt.Sprintf("test%d", i)] = "jest"
	}
	data, err := json.Marshal(map[string]any{"scripts": scripts})
	if err != nil {
		t.Fatal(err)
	}
	reviewRejection(t, Snapshot{Documents: []Document{{Path: "package.json", Content: data}}}, Options{}, "output_limit")
}

func TestReviewUnsupportedTokenSeparators(t *testing.T) {
	for _, separator := range []string{"\u00a0", "\u2003", "\u0085", "\v", "\f"} {
		t.Run(fmt.Sprintf("%x", separator), func(t *testing.T) {
			data := seamBytes(t, map[string]any{"scripts": map[string]string{"test": "go" + separator + "test ./..."}})
			input := Snapshot{Documents: []Document{{Path: "package.json", Content: data}}}
			before := seamBytes(t, input)
			got := Analyze(input, Options{})
			if len(got.Candidates) != 1 || got.Candidates[0].State != "UNKNOWN" || got.Candidates[0].Command != "" || len(got.Diagnostics) != 1 || got.Diagnostics[0].Code != "unsupported_script" {
				t.Fatalf("unsupported separator fabricated a command: %+v", got)
			}
			if !bytes.Equal(before, seamBytes(t, input)) {
				t.Fatal("token rejection mutated input")
			}
		})
	}
	for _, command := range []string{"go test ./...", "go\ttest ./..."} {
		data := seamBytes(t, map[string]any{"scripts": map[string]string{"test": command}})
		got := Analyze(Snapshot{Documents: []Document{{Path: "package.json", Content: data}}}, Options{})
		if len(got.Candidates) != 1 || got.Candidates[0].Command != "go test ./..." || got.Candidates[0].State != "CONFIGURED" {
			t.Fatalf("ASCII shell separators regressed: %+v", got)
		}
	}
}

func TestReviewCompositeLeafProvenance(t *testing.T) {
	input := Snapshot{Documents: []Document{{Path: "package.json", Content: []byte(`{"scripts":{"check":"npm run unit && npm run build","unit":"node --test","build":"tsc --noEmit","nested":"npm run check"}}`)}}}
	before := seamBytes(t, input)
	got := Analyze(input, Options{})
	seen := 0
	for _, candidate := range got.Candidates {
		if candidate.Source.Symbol != "check" && candidate.Source.Symbol != "nested" {
			continue
		}
		want := []Source{{Path: "package.json", Symbol: candidate.Source.Symbol}}
		if candidate.Source.Symbol == "nested" {
			want = append(want, Source{Path: "package.json", Symbol: "check"})
		}
		leaf := "unit"
		if candidate.Command == "tsc --noEmit" {
			leaf = "build"
		} else if candidate.Command != "node --test" {
			t.Fatalf("unexpected composite command: %+v", candidate)
		}
		want = append(want, Source{Path: "package.json", Symbol: leaf})
		if candidate.State != "CONFIGURED" || !reflect.DeepEqual(candidate.Chain, want) {
			t.Fatalf("leaf provenance includes siblings: got %+v want %+v", candidate.Chain, want)
		}
		seen++
	}
	if seen != 4 || !bytes.Equal(before, seamBytes(t, input)) {
		t.Fatal("missing composite leaves or input mutation")
	}
}

func TestReviewUniformOptionCaps(t *testing.T) {
	root := t.TempDir()
	seamWrite(t, root, "go.mod", "module example.test/caps\n")
	for _, options := range []Options{{MaxEntries: 65536}, {MaxReferences: 4096}, {MaxEntries: 65537}, {MaxReferences: 4097}} {
		before := options
		snapshot := ReadSnapshot(root, options)
		analysis := Analyze(snapshot, options)
		scan := Scan(root, options)
		invalid := options.MaxEntries > 65536 || options.MaxReferences > 4096
		if invalid {
			if len(snapshot.Documents) != 0 || len(snapshot.Diagnostics) != 1 || snapshot.Diagnostics[0].Code != "INVALID_OPTIONS" || len(analysis.Roots) != 0 || len(analysis.Diagnostics) != 1 || analysis.Diagnostics[0].Code != "invalid_options" || !reflect.DeepEqual(analysis, scan) {
				t.Fatalf("reader/analyzer disagree on rejection for %+v: %+v %+v", options, snapshot, analysis)
			}
		} else if len(snapshot.Documents) != 1 || len(snapshot.Diagnostics) != 0 || len(analysis.Roots) != 1 || len(analysis.Diagnostics) != 0 || !reflect.DeepEqual(analysis, scan) {
			t.Fatalf("reader/analyzer disagree on valid boundary: %+v %+v", snapshot, analysis)
		}
		if options != before {
			t.Fatal("limits mutated")
		}
	}
}
