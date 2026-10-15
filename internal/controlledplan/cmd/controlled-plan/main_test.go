package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
)

func process(t *testing.T, args []string, out, stderr *bytes.Buffer) int {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestMainProcessHelper$", "--"}, args...)...)
	command.Env = append(os.Environ(), "CONTROLLED_PLAN_TEST_HELPER=1")
	command.Stdout, command.Stderr = out, stderr
	err = command.Run()
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	t.Fatal(err)
	return -1
}

func TestCommand(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.json")
	sentinel := filepath.Join(dir, "counter")
	invalidInput := filepath.Join(dir, "invalid.json")
	invalidBytes := []byte(`{"version":1,"version":1}`)
	if err := os.WriteFile(invalidInput, invalidBytes, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sentinel, []byte("0"), 0600); err != nil {
		t.Fatal(err)
	}
	revision := strings.Repeat("c", 40)
	e, err := controlledplan.Prepare(strings.Repeat("a", 64), "sha256:"+strings.Repeat("b", 64), revision, "123", "2")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(input, e.JSON(), 0600); err != nil {
		t.Fatal(err)
	}
	prepare := []string{"prepare", "--snapshot", e.Plan.Snapshot, "--image", e.Plan.Image, "--revision", revision, "--run-id", "123", "--run-attempt", "2"}
	check := []string{"check", "--input", input, "--binding", e.BindingDigest, "--revision", revision, "--run-id", "123", "--run-attempt", "2"}
	cases := []struct {
		name     string
		args     []string
		code     int
		out, err string
	}{
		{"prepare", prepare, 0, string(e.JSON()), ""}, {"check", check, 0, string(e.JSON()), ""},
		{"prepare help", []string{"prepare", "--help"}, 0, help, ""},
		{"check help", []string{"check", "--help"}, 0, help, ""},
		{"help", []string{"--help"}, 0, help, ""}, {"missing", nil, 2, "", "controlled-plan: invalid arguments\n"},
		{"unknown", []string{"execute"}, 2, "", "controlled-plan: invalid arguments\n"},
		{"duplicate", append(append([]string(nil), prepare...), "--run-id", "123"), 2, "", "controlled-plan: invalid arguments\n"},
		{"bad value", []string{"prepare", "--snapshot", "bad"}, 2, "", "controlled-plan: invalid arguments\n"},
		{"stale", append(append([]string(nil), check[:len(check)-1]...), "3"), 2, "", "controlled-plan: validation failed\n"},
		{"invalid input", []string{"check", "--input", invalidInput, "--binding", e.BindingDigest, "--revision", revision, "--run-id", "123", "--run-attempt", "2"}, 2, "", "controlled-plan: validation failed\n"},
		{"missing file", []string{"check", "--input", filepath.Join(dir, "absent"), "--binding", e.BindingDigest, "--revision", revision, "--run-id", "123", "--run-attempt", "2"}, 2, "", "controlled-plan: validation failed\n"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := process(t, tt.args, &out, &stderr)
			if code != tt.code || out.String() != tt.out || stderr.String() != tt.err {
				t.Fatalf("got (%d,%q,%q), want (%d,%q,%q)", code, out.String(), stderr.String(), tt.code, tt.out, tt.err)
			}
			b, err := os.ReadFile(input)
			if err != nil || !bytes.Equal(b, e.JSON()) {
				t.Fatal("input changed")
			}
			b, err = os.ReadFile(sentinel)
			if err != nil || string(b) != "0" {
				t.Fatal("counter changed")
			}
			b, err = os.ReadFile(invalidInput)
			if err != nil || !bytes.Equal(b, invalidBytes) {
				t.Fatal("invalid input changed")
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 3 {
				t.Fatal("inventory changed")
			}
		})
	}
}
