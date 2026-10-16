package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/controlledplan"
	"github.com/dnlrsls/ci-trust/internal/controlledverify"
	"github.com/dnlrsls/ci-trust/internal/ownedimage"
)

func TestHelpAndInvalidArgumentsNeverReadOrExecute(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, nil, {"execute", "--approved", "true"}, {"prepare", "--source", "missing"}, {"arbitrary"}} {
		var out, stderr bytes.Buffer
		code := run(context.Background(), args, &out, &stderr)
		isHelp := len(args) == 1 && (args[0] == "--help" || args[0] == "-h")
		if isHelp {
			if code != 0 || !strings.Contains(out.String(), "no approval arguments") || stderr.Len() != 0 {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
		} else if code != 2 || out.Len() != 0 || stderr.String() != "controlled-verify: invalid arguments\n" {
			t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
		}
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("owned output unavailable") }
func TestHelpOutputFailure(t *testing.T) {
	if run(context.Background(), []string{"--help"}, brokenWriter{}, io.Discard) != 1 {
		t.Fatal("output error hidden")
	}
}

func cliPlan(t *testing.T) (controlledplan.Envelope, string) {
	t.Helper()
	m := ownedimage.Metadata{ImageID: "sha256:" + strings.Repeat("a", 64), ArchiveSHA256: "sha256:" + strings.Repeat("b", 64), BinarySHA256: "sha256:" + strings.Repeat("c", 64), MarkerSHA256: "sha256:" + strings.Repeat("d", 64), LayerDiffID: "sha256:" + strings.Repeat("e", 64), Scope: "preparation-only"}
	p, err := controlledplan.PrepareProof(strings.Repeat("f", 64), m, strings.Repeat("a", 40), "123", "1", time.Now().Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "plan.json")
	if err := os.WriteFile(path, p.JSON(), 0600); err != nil {
		t.Fatal(err)
	}
	return p, path
}
func executionArgs(p controlledplan.Envelope, path string) []string {
	// A relative source always fails the production host boundary before any
	// API or runtime access, including when these tests run on Linux.
	return []string{"execute", "--source", "relative-fixture", "--archive", "missing-archive", "--binary", "missing-binary", "--marker", "missing-marker", "--input", path, "--binding", p.BindingDigest}
}

func TestExecutionPinsCurrentWorkflowAndDoesNotLeakToken(t *testing.T) {
	for _, name := range []string{"valid-context-unsupported-source", "missing-context", "different-run", "rerun", "different-revision", "wrong-binding", "unknown-field", "duplicate-flag"} {
		t.Run(name, func(t *testing.T) {
			p, path := cliPlan(t)
			t.Setenv("GITHUB_SHA", p.Revision)
			t.Setenv("GITHUB_RUN_ID", p.RunID)
			t.Setenv("GITHUB_RUN_ATTEMPT", "1")
			t.Setenv("GITHUB_TOKEN", "owned-test-token-do-not-print")
			args := executionArgs(p, path)
			switch name {
			case "missing-context":
				t.Setenv("GITHUB_RUN_ID", "")
			case "different-run":
				t.Setenv("GITHUB_RUN_ID", "124")
			case "rerun":
				t.Setenv("GITHUB_RUN_ATTEMPT", "2")
			case "different-revision":
				t.Setenv("GITHUB_SHA", strings.Repeat("0", 40))
			case "wrong-binding":
				args[len(args)-1] = strings.Repeat("0", 64)
			case "unknown-field":
				if err := os.WriteFile(path, append(bytes.TrimSpace(p.JSON())[:len(bytes.TrimSpace(p.JSON()))-1], []byte(`,"Approved":true}`)...), 0600); err != nil {
					t.Fatal(err)
				}
			case "duplicate-flag":
				args[3] = "--source"
			}
			var out, stderr bytes.Buffer
			code := run(context.Background(), args, &out, &stderr)
			if code != 2 || strings.Contains(out.String()+stderr.String(), "owned-test-token-do-not-print") {
				t.Fatalf("code=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
			if name == "valid-context-unsupported-source" {
				var r controlledverify.ExecutionRecord
				if json.Unmarshal(out.Bytes(), &r) != nil || r.State != "UNVERIFIABLE" || r.ImageDisposition != "NOT_REQUESTED" || stderr.Len() != 0 {
					t.Fatalf("record=%+v stderr=%q", r, stderr.String())
				}
			} else if out.Len() != 0 || stderr.Len() == 0 {
				t.Fatal("invalid context or input reached composition")
			}
		})
	}
}

func TestPrepareRejectsUnknownArchiveWithoutAPIOrDocker(t *testing.T) {
	root := t.TempDir()
	archive := filepath.Join(root, "archive")
	if err := os.WriteFile(archive, []byte("inert unknown archive"), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"prepare", "--source", root, "--archive", archive, "--binary", "absent", "--marker", "absent", "--revision", strings.Repeat("a", 40), "--run-id", "123", "--run-attempt", "1"}
	var out, stderr bytes.Buffer
	if run(context.Background(), args, &out, &stderr) != 2 || out.Len() != 0 || stderr.String() != "controlled-verify: review preparation unavailable\n" {
		t.Fatalf("stdout=%q stderr=%q", out.String(), stderr.String())
	}
}
