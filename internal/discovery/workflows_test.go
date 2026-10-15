package discovery

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWorkflowSnapshotAdmissionAndInventoryIsolation(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"go.mod":                               "module example.test/root\n\ngo 1.26\n",
		"package.json":                         `{"scripts":{"lint":"eslint ."}}`,
		".github/workflows/ci.yml":             "on: push\njobs: {}\n",
		".github/workflows/pnpm-lock.yaml":     "on: pull_request\njobs: {}\n",
		"other.yml":                            "run: forbidden\n",
		"sub/.github/workflows/nested.yaml":    "run: forbidden\n",
		".github/workflows/nested/ignored.yml": "run: forbidden\n",
		".github/workflows/not-a-workflow.txt": "run: forbidden\n",
		"counter":                              "0\n",
	}
	for name, content := range files {
		seamWrite(t, root, name, content)
	}
	snapshot := ReadSnapshot(root, Options{})
	var paths []string
	for _, document := range snapshot.Documents {
		paths = append(paths, document.Path)
	}
	want := []string{".github/workflows/ci.yml", ".github/workflows/pnpm-lock.yaml", "go.mod", "package.json"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("root workflow admission: got %v, want %v", paths, want)
	}
	baseline := Snapshot{Documents: []Document{{Path: "go.mod", Content: []byte(files["go.mod"])}, {Path: "package.json", Content: []byte(files["package.json"])}}}
	got := Analyze(snapshot, Options{})
	expected := Analyze(baseline, Options{})
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("workflow YAML polluted ordinary manifest inventory:\n got %+v\nwant %+v", got, expected)
	}
	for name, content := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || !bytes.Equal(data, []byte(content)) {
			t.Fatalf("source/counter modified: %s (%v)", name, err)
		}
	}
}

func TestWorkflowSnapshotSharesReaderLimits(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options Options
		code    string
	}{
		{"file", Options{MaxFileBytes: 8}, "FILE_LIMIT"},
		{"total", Options{MaxTotalBytes: 8}, "TOTAL_LIMIT"},
		{"invalid", Options{MaxEntries: -1}, "INVALID_OPTIONS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			content := "on: push\njobs: {}\n"
			seamWrite(t, root, ".github/workflows/ci.yaml", content)
			seamWrite(t, root, "counter", "0\n")
			snapshot := ReadSnapshot(root, tc.options)
			if len(snapshot.Documents) != 0 || len(snapshot.Diagnostics) != 1 || snapshot.Diagnostics[0].Code != tc.code {
				t.Fatalf("bounded workflow read: %+v", snapshot)
			}
			for name, want := range map[string]string{".github/workflows/ci.yaml": content, "counter": "0\n"} {
				data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
				if err != nil || string(data) != want {
					t.Fatalf("rejected read changed %s: %v", name, err)
				}
			}
		})
	}
}
