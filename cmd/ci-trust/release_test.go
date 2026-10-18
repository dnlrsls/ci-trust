package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

// Existing native Go CI runs this build-metadata fixture, not an ESLint runtime.
// Its digest lets a distributor match the exact tested bytes after cross-build.
func TestVersionedBinary(t *testing.T) {
	name := "ci-trust"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	command := exec.Command("go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-buildid= -X main.version=0.1.0", "-o", binary, ".")
	fixed := map[string]string{
		"CGO_ENABLED": "0", "GOENV": "off", "GOFLAGS": "", "GOWORK": "off",
		"GOTOOLCHAIN": "local", "GOOS": runtime.GOOS, "GOARCH": runtime.GOARCH,
		"GOAMD64": "v1", "GOARM64": "v8.0", "GOEXPERIMENT": "",
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		_, replaced := fixed[key]
		if !replaced && key != "GH_TOKEN" && !strings.HasPrefix(key, "GITHUB_") && !strings.HasPrefix(key, "DOCKER_") {
			command.Env = append(command.Env, entry)
		}
	}
	for key, value := range fixed {
		command.Env = append(command.Env, key+"="+value)
	}
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("versioned owned binary build: %v\n%s", err, output)
	}
	dev := buildPublicBinary(t)
	root := t.TempDir()
	for name, content := range map[string]string{"go.mod": "module example.test/release\n\ngo 1.26\n", "counter": "0\n"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	before := fixtureBytes(t, root)
	code, out, errOut := invokePublic(t, binary, nil, "--version")
	assertPublic(t, code, out, errOut, 0, "ci-trust \"0.1.0\"\n", "")
	code, out, errOut = invokePublic(t, binary, nil, "--help")
	assertPublic(t, code, out, errOut, 0, publicHelp, "")
	for _, args := range [][]string{
		{"audit", root, "--format", "json"},
		{"bootstrap", root, "--dry-run", "--format", "json"},
		{"verify", root, "--check", "lint", "--format", "json"},
		{"audit", root, "--format", "xml"},
	} {
		wantCode, wantOut, wantErr := invokePublic(t, dev, nil, args...)
		code, out, errOut = invokePublic(t, binary, nil, args...)
		assertPublic(t, code, out, errOut, wantCode, string(wantOut), string(wantErr))
		unchangedFixture(t, root, before)
	}
	code, audit, errOut := invokePublic(t, binary, nil, "audit", root, "--format", "json")
	if code != 0 || len(evidence.Validate(audit)) != 0 {
		t.Fatalf("invalid versioned audit: exit=%d stderr=%q", code, errOut)
	}
	wantCode, wantOut, wantErr := invokePublic(t, dev, audit, "report", "--format", "json")
	code, out, errOut = invokePublic(t, binary, audit, "report", "--format", "json")
	assertPublic(t, code, out, errOut, wantCode, string(wantOut), string(wantErr))
	if !bytes.Equal(audit, out) {
		t.Fatal("report changed versioned audit bytes")
	}
	unchangedFixture(t, root, before)
	data, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := json.Marshal(map[string]string{
		"version": "0.1.0", "go_version": runtime.Version(),
		"goos": runtime.GOOS, "goarch": runtime.GOARCH,
		"sha256": fmt.Sprintf("%x", sha256.Sum256(data)),
	})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("CI_TRUST_VERSIONED_BINARY %s\n", receipt)
}
