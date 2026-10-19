package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestNativeArchiveExport(t *testing.T) {
	if testing.Short() {
		t.Skip("native binary integration check")
	}
	dir := t.TempDir()
	command := exec.Command("go", "test", "-count=1", "-v", "-run", "^TestVersionedBinary$", ".")
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if key != "CI_TRUST_NATIVE_ARTIFACT_DIR" && key != "GH_TOKEN" && !strings.HasPrefix(key, "GITHUB_") && !strings.HasPrefix(key, "DOCKER_") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, "CI_TRUST_NATIVE_ARTIFACT_DIR="+dir)
	var out, stderr bytes.Buffer
	command.Stdout, command.Stderr = &out, &stderr
	if err := command.Run(); err != nil || stderr.Len() != 0 {
		t.Fatalf("owned native test exit: %v stdout=%q stderr=%q", err, out.Bytes(), stderr.Bytes())
	}
	name := "ci-trust_0.1.1_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
	z, err := zip.OpenReader(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("native archive was not retained: %v", err)
	}
	defer z.Close()
	files := map[string][]byte{}
	modes := map[string]os.FileMode{}
	for _, f := range z.File {
		if _, exists := files[f.Name]; exists {
			t.Fatal("duplicate archive member")
		}
		r, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name], err = io.ReadAll(r)
		r.Close()
		if err != nil {
			t.Fatal(err)
		}
		modes[f.Name] = f.Mode()
	}
	binary := "ci-trust"
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	var meta map[string]string
	if err := json.Unmarshal(files["RELEASE.json"], &meta); err != nil {
		t.Fatal(err)
	}
	if meta["version"] != "0.1.1" || meta["goos"] != runtime.GOOS || meta["goarch"] != runtime.GOARCH || meta["sha256"] != fmt.Sprintf("%x", sha256.Sum256(files[binary])) || len(files[binary]) == 0 || modes[binary].Perm() != 0755 {
		t.Fatal("native binary metadata/mode mismatch")
	}
	for _, p := range []string{"LICENSE", "THIRD_PARTY_NOTICES.md"} {
		want, err := os.ReadFile(filepath.Join("..", "..", p))
		if err != nil || !bytes.Equal(files[p], want) {
			t.Fatalf("distribution text mismatch: %s", p)
		}
	}
	entries, err := os.ReadDir(filepath.Join("..", "..", "LICENSES"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		p := "LICENSES/" + e.Name()
		want, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(p)))
		if err != nil || !bytes.Equal(files[p], want) {
			t.Fatalf("upstream license mismatch: %s", p)
		}
	}
	if len(files) != len(entries)+4 {
		t.Fatal("unexpected archive surfaces")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "native.json"))
	if err != nil {
		t.Fatal(err)
	}
	var receipt map[string]string
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	archive, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	if receipt["archive"] != name || receipt["archive_sha256"] != fmt.Sprintf("%x", sha256.Sum256(archive)) || receipt["sha256"] != meta["sha256"] || !strings.Contains(out.String(), "CI_TRUST_VERSIONED_BINARY ") {
		t.Fatal("archive receipt not bound to tested bytes")
	}
}

func exportNativeArchive(t *testing.T, dir, binaryName string, binary []byte, meta map[string]string) {
	t.Helper()
	if !filepath.IsAbs(dir) {
		t.Fatal("native artifact directory must be absolute")
	}
	files := map[string][]byte{binaryName: binary}
	for _, p := range []string{"LICENSE", "THIRD_PARTY_NOTICES.md"} {
		b, err := os.ReadFile(filepath.Join("..", "..", p))
		if err != nil {
			t.Fatal(err)
		}
		files[p] = b
	}
	entries, err := os.ReadDir(filepath.Join("..", "..", "LICENSES"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if !e.Type().IsRegular() {
			t.Fatal("nonregular license entry")
		}
		p := "LICENSES/" + e.Name()
		files[p], err = os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(p)))
		if err != nil {
			t.Fatal(err)
		}
	}
	meta["revision"] = os.Getenv("GITHUB_SHA")
	meta["run_id"] = os.Getenv("GITHUB_RUN_ID")
	meta["run_attempt"] = os.Getenv("GITHUB_RUN_ATTEMPT")
	files["RELEASE.json"], err = json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	files["RELEASE.json"] = append(files["RELEASE.json"], '\n')
	var names []string
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	for _, name := range names {
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC))
		header.SetMode(0644)
		if name == binaryName {
			header.SetMode(0755)
		}
		entry, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = entry.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	name := "ci-trust_0.1.1_" + runtime.GOOS + "_" + runtime.GOARCH + ".zip"
	meta["archive"] = name
	meta["archive_sha256"] = fmt.Sprintf("%x", sha256.Sum256(archive.Bytes()))
	receipt, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{name: archive.Bytes(), "native.json": append(receipt, '\n')} {
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.Write(data)
		closeErr := f.Close()
		if writeErr != nil {
			t.Fatal(writeErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
	fmt.Printf("CI_TRUST_NATIVE_ARCHIVE %s\n", receipt)
}
