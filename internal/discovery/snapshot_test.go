package discovery

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
)

func snapshotFixture(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
func snapshotDiagnostic(code, p, message string) Diagnostic {
	return Diagnostic{Code: code, Path: p, Message: message, State: "UNKNOWN"}
}
func emptySnapshot() Snapshot {
	return Snapshot{Documents: []Document{}, Diagnostics: []Diagnostic{}, Limitations: []string{}}
}
func checkSnapshot(t *testing.T, got, want Snapshot) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot mismatch\ngot: %#v\nwant: %#v", got, want)
	}
}
func supportedSnapshotHost(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "js" || runtime.GOOS == "plan9" {
		t.Skip("os.Root confinement unsupported on this host")
	}
}

func TestSnapshotInventory(t *testing.T) {
	supportedSnapshotHost(t)
	root := t.TempDir()
	snapshotFixture(t, root, "package.json", `{"scripts":{"test":"touch sentinel"}}`)
	snapshotFixture(t, root, "nested/go.mod", "module example.test/nested")
	snapshotFixture(t, root, "ignored.txt", "ignored")
	snapshotFixture(t, root, "eslint.config.js", "throw new Error('must not execute')")
	want := emptySnapshot()
	want.Documents = []Document{{Path: "eslint.config.js", Content: []byte("throw new Error('must not execute')")}, {Path: "nested/go.mod", Content: []byte("module example.test/nested")}, {Path: "package.json", Content: []byte(`{"scripts":{"test":"touch sentinel"}}`)}}
	got := ReadSnapshot(root, Options{})
	checkSnapshot(t, got, want)
	checkSnapshot(t, ReadSnapshot(root, DefaultOptions()), want)
	got.Documents[0].Content[0] = 'X'
	checkSnapshot(t, ReadSnapshot(root, Options{}), want)
	for _, d := range want.Documents {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(d.Path)))
		if err != nil || string(b) != string(d.Content) {
			t.Fatal("input mutated")
		}
	}
	if _, err := os.Stat(filepath.Join(root, "sentinel")); !os.IsNotExist(err) {
		t.Fatal("hostile script executed")
	}
}

func TestSnapshotRootFailuresAndOptions(t *testing.T) {
	supportedSnapshotHost(t)
	root := t.TempDir()
	snapshotFixture(t, root, "file", "x")
	for _, name := range []string{"missing", "file"} {
		t.Run(name, func(t *testing.T) {
			want := emptySnapshot()
			want.Diagnostics = append(want.Diagnostics, snapshotDiagnostic("ROOT_UNAVAILABLE", ".", "Root cannot be opened."))
			checkSnapshot(t, ReadSnapshot(filepath.Join(root, name), Options{}), want)
		})
	}
	cases := []Options{{MaxDepth: -1}, {MaxDepth: 65}, {MaxEntries: -1}, {MaxEntries: 100001}, {MaxEntries: int(^uint(0) >> 1)}, {MaxFileBytes: -1}, {MaxFileBytes: 1 << 62}, {MaxTotalBytes: -1}, {MaxTotalBytes: 1 << 62}, {MaxReferences: -1}, {MaxReferences: 100001}, {MaxScriptDepth: -1}, {MaxScriptDepth: 65}}
	for i, o := range cases {
		t.Run(fmt.Sprintf("invalid-%d", i), func(t *testing.T) {
			want := emptySnapshot()
			want.Diagnostics = append(want.Diagnostics, snapshotDiagnostic("INVALID_OPTIONS", ".", "Options exceed supported bounds."))
			checkSnapshot(t, ReadSnapshot(filepath.Join(root, "missing"), o), want)
		})
	}
}

func TestSnapshotBounds(t *testing.T) {
	supportedSnapshotHost(t)
	tests := []struct {
		name        string
		files       map[string]string
		options     Options
		docs        []Document
		diagnostics []Diagnostic
	}{
		{"file", map[string]string{"go.mod": "abcd"}, Options{MaxFileBytes: 3}, []Document{}, []Diagnostic{snapshotDiagnostic("FILE_LIMIT", "go.mod", "File exceeds byte limit.")}},
		{"aggregate", map[string]string{"go.mod": "abc", "package.json": "de"}, Options{MaxTotalBytes: 4}, []Document{{Path: "go.mod", Content: []byte("abc")}}, []Diagnostic{snapshotDiagnostic("TOTAL_LIMIT", "package.json", "Aggregate byte limit reached.")}},
		{"depth", map[string]string{"a/b/go.mod": "abc"}, Options{MaxDepth: 1}, []Document{}, []Diagnostic{snapshotDiagnostic("DEPTH_LIMIT", "a/b", "Directory exceeds depth limit.")}},
		{"entries", map[string]string{"go.mod": "abc", "a.txt": "a", "b.txt": "b"}, Options{MaxEntries: 2}, []Document{}, []Diagnostic{snapshotDiagnostic("ENTRY_LIMIT", ".", "Directory omitted: entry limit reached.")}},
		{"nested entries", map[string]string{"a/go.mod": "a", "a/z": "z", "go.mod": "root"}, Options{MaxEntries: 3}, []Document{{Path: "go.mod", Content: []byte("root")}}, []Diagnostic{snapshotDiagnostic("ENTRY_LIMIT", "a", "Directory omitted: entry limit reached.")}},
		{"exact limits", map[string]string{"a/go.mod": "abc"}, Options{MaxDepth: 1, MaxEntries: 2, MaxFileBytes: 3, MaxTotalBytes: 3}, []Document{{Path: "a/go.mod", Content: []byte("abc")}}, []Diagnostic{}},
		{"excluded entries count", map[string]string{"node_modules/go.mod": "x", "go.mod": "x"}, Options{MaxEntries: 1}, []Document{}, []Diagnostic{snapshotDiagnostic("ENTRY_LIMIT", ".", "Directory omitted: entry limit reached.")}},
		{"rejected reads charged", map[string]string{"go.mod": "abcd", "package.json": "x"}, Options{MaxFileBytes: 3, MaxTotalBytes: 4}, []Document{}, []Diagnostic{snapshotDiagnostic("FILE_LIMIT", "go.mod", "File exceeds byte limit."), snapshotDiagnostic("TOTAL_LIMIT", "package.json", "Aggregate byte limit reached.")}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for p, c := range tt.files {
				snapshotFixture(t, root, p, c)
			}
			want := emptySnapshot()
			want.Documents = tt.docs
			want.Diagnostics = tt.diagnostics
			checkSnapshot(t, ReadSnapshot(root, tt.options), want)
			checkSnapshot(t, ReadSnapshot(root, tt.options), want)
		})
	}
}

func TestSnapshotExcludedAndMarkers(t *testing.T) {
	supportedSnapshotHost(t)
	root := t.TempDir()
	want := emptySnapshot()
	for _, name := range []string{".git", "node_modules", "vendor", ".venv", "dist", "build", ".next"} {
		snapshotFixture(t, root, name+"/go.mod", "ignored")
		want.Diagnostics = append(want.Diagnostics, snapshotDiagnostic("EXCLUDED_DIRECTORY", name, "Directory excluded from discovery."))
	}
	for _, name := range []string{"go.mod", "package.json", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml", ".eslintrc", ".eslintrc.json", "eslint.config.js", "eslint.config.mjs", "eslint.config.cjs", ".golangci.yml", ".golangci.yaml", "pyproject.toml", "requirements.txt", "Cargo.toml", "pom.xml"} {
		snapshotFixture(t, root, name, "x")
		want.Documents = append(want.Documents, Document{Path: name, Content: []byte("x")})
	}
	snapshotFixture(t, root, "go.sum", "not a recognized marker")
	snapshotFixture(t, root, "tsconfig.json", "not a recognized marker")
	sort.Slice(want.Documents, func(i, j int) bool { return want.Documents[i].Path < want.Documents[j].Path })
	sort.Slice(want.Diagnostics, func(i, j int) bool { return want.Diagnostics[i].Path < want.Diagnostics[j].Path })
	checkSnapshot(t, ReadSnapshot(root, Options{}), want)
}

func TestSnapshotSymlinks(t *testing.T) {
	supportedSnapshotHost(t)
	root := t.TempDir()
	outside := t.TempDir()
	snapshotFixture(t, root, "go.mod", "safe")
	snapshotFixture(t, outside, "package.json", "private")
	for _, s := range []struct{ name, target string }{{"internal", filepath.Join(root, "go.mod")}, {"package.json", filepath.Join(outside, "package.json")}, {"directory", outside}} {
		if err := os.Symlink(s.target, filepath.Join(root, s.name)); err != nil {
			t.Skip("symlink creation unavailable on this host")
		}
	}
	want := emptySnapshot()
	want.Documents = append(want.Documents, Document{Path: "go.mod", Content: []byte("safe")})
	for _, p := range []string{"directory", "internal", "package.json"} {
		want.Diagnostics = append(want.Diagnostics, snapshotDiagnostic("SYMLINK_SKIPPED", p, "Symbolic link omitted."))
	}
	got := ReadSnapshot(root, Options{})
	checkSnapshot(t, got, want)
	encoded := fmt.Sprintf("%#v", got)
	if strings.Contains(encoded, outside) || strings.Contains(encoded, root) {
		t.Fatal("host path leaked")
	}
	// A root that is itself a symlink is not treated as a new authority boundary.
	want = emptySnapshot()
	want.Diagnostics = append(want.Diagnostics, snapshotDiagnostic("ROOT_UNAVAILABLE", ".", "Root cannot be opened."))
	checkSnapshot(t, ReadSnapshot(filepath.Join(root, "directory"), Options{}), want)
}

func TestSnapshotIncrementalDirectory(t *testing.T) {
	supportedSnapshotHost(t)
	root := t.TempDir()
	for i := 0; i < 140; i++ {
		snapshotFixture(t, root, fmt.Sprintf("unrecognized-%03d", i), "x")
	}
	snapshotFixture(t, root, "go.mod", "module bounded")
	want := emptySnapshot()
	want.Diagnostics = append(want.Diagnostics, snapshotDiagnostic("ENTRY_LIMIT", ".", "Directory omitted: entry limit reached."))
	options := Options{MaxEntries: 129}
	original := options
	checkSnapshot(t, ReadSnapshot(root, options), want)
	if options != original {
		t.Fatal("options mutated")
	}
	checkSnapshot(t, ReadSnapshot(root, options), want)
	want = emptySnapshot()
	want.Documents = append(want.Documents, Document{Path: "go.mod", Content: []byte("module bounded")})
	checkSnapshot(t, ReadSnapshot(root, Options{MaxEntries: 141}), want)
}

func TestSnapshotEmpty(t *testing.T) {
	supportedSnapshotHost(t)
	checkSnapshot(t, ReadSnapshot(t.TempDir(), Options{}), emptySnapshot())
}
