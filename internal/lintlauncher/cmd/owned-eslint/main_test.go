package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--owned-command" {
		os.Args = append(os.Args[:1], os.Args[2:]...)
		main()
		os.Exit(99)
	}
	os.Exit(m.Run())
}

func TestPublicHelpAndRejection(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--help"}, {"--tool", "unowned"}, {"--role", "OTHER", "--binding", strings.Repeat("c", 64)}} {
		cmd := exec.Command(executable, append([]string{"--owned-command"}, args...)...)
		cmd.Env = []string{"LANG=C", "LC_ALL=C"}
		if testing.CoverMode() != "" {
			cmd.Env = append(cmd.Env, "GOCOVERDIR="+t.TempDir())
		}
		var out, diag bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &diag
		err := cmd.Run()
		if args[0] == "--help" {
			if err != nil || diag.Len() != 0 || !strings.Contains(out.String(), "No host fallback") {
				t.Fatalf("help: %q %q %v", out.String(), diag.String(), err)
			}
		} else {
			exit, ok := err.(*exec.ExitError)
			if !ok || exit.ExitCode() != 2 || out.Len() != 0 || diag.String() != "owned-eslint: invalid arguments\n" {
				t.Fatalf("rejection: %q %q %v", out.String(), diag.String(), err)
			}
		}
	}
}
