package discovery

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSupportDeclarationSnapshotRootOnlyAndStatic(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{".ci-trust.json": `{"version":1,"platforms":["linux","windows","macos"]}`, "sub/.ci-trust.json": `{"version":1,"platforms":["made-up"]}`, "counter": "0\n"}
	for name, content := range files {
		seamWrite(t, root, name, content)
	}
	got := ReadSnapshot(root, Options{})
	if len(got.Documents) != 1 || got.Documents[0].Path != ".ci-trust.json" || string(got.Documents[0].Content) != files[".ci-trust.json"] {
		t.Fatalf("support declaration snapshot: %+v", got)
	}
	inventory := Analyze(got, Options{})
	empty := Analyze(Snapshot{}, Options{})
	if !reflect.DeepEqual(inventory, empty) {
		t.Fatalf("declaration became manifest/tool candidate: %+v", inventory)
	}
	for name, want := range files {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || string(data) != want {
			t.Fatalf("source/counter changed %s: %v", name, err)
		}
	}
}
