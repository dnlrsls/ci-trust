package controlledverify

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedSourceSnapshotMeasuresBytesNamesAndSentinel(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(root, "owned source ñ.txt")
	if err := os.WriteFile(file, []byte("source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	counter := filepath.Join(root, "counter")
	if err := os.WriteFile(counter, []byte("0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := sourceSnapshot(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "owned-cache"), []byte("allowed scratch"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err := sourceSnapshot(root)
	if err != nil || after != before {
		t.Fatal("owned Git scratch changed source identity")
	}
	if err := os.WriteFile(file, []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	after, err = sourceSnapshot(root)
	if err != nil || after == before {
		t.Fatal("changed source bytes not detected")
	}
	if err := os.WriteFile(file, []byte("source\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file, filepath.Join(root, "other.txt")); err != nil {
		t.Fatal(err)
	}
	after, err = sourceSnapshot(root)
	if err != nil || after == before {
		t.Fatal("changed source names not detected")
	}
	if b, err := os.ReadFile(counter); err != nil || string(b) != "0\n" {
		t.Fatal("snapshot wrote source counter")
	}
}

func TestOwnedSourceSnapshotRefusesLinksAndOversize(t *testing.T) {
	for _, kind := range []string{"symlink", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			file := filepath.Join(root, "source")
			if err := os.WriteFile(file, []byte("owned"), 0600); err != nil {
				t.Fatal(err)
			}
			if kind == "symlink" {
				if err := os.Symlink(file, filepath.Join(root, "alias")); err != nil {
					t.Fatalf("symlink proof unavailable: %v", err)
				}
			} else {
				f, err := os.OpenFile(file, os.O_WRONLY, 0600)
				if err != nil {
					t.Fatal(err)
				}
				err = f.Truncate(8<<20 + 1)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, err := sourceSnapshot(root); err == nil {
				t.Fatal("unbounded or ambiguous source accepted")
			}
		})
	}
}
