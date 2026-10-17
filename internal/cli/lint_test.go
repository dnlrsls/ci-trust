package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

func TestVerifyReportServiceAndInvocationBoundaries(t *testing.T) {
	data, err := os.ReadFile("../evidence/testdata/golden/empty-report.json")
	if err != nil {
		t.Fatal(err)
	}
	want, ds := evidence.Encode(data)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	for _, args := range [][]string{{"verify", "space ü", "--check", "lint", "--format", "json"}, {"verify", "--format", "json", "--check", "lint", "--", "-owned"}} {
		calls := 0
		var out, stderr bytes.Buffer
		code := Run(args, forbiddenReader{t}, &out, &stderr, Services{Audit: func(string, []string) ([]byte, error) { t.Fatal("unexpected audit"); return nil, nil }, Verify: func(root string) ([]byte, error) {
			calls++
			if root != "space ü" && root != "-owned" {
				t.Fatal(root)
			}
			return data, nil
		}})
		if code != 0 || !bytes.Equal(out.Bytes(), want) || stderr.Len() != 0 || calls != 1 {
			t.Fatalf("exit=%d stdout=%q stderr=%q calls=%d", code, out.String(), stderr.String(), calls)
		}
	}
	for _, args := range [][]string{{"verify", "x"}, {"verify", "x", "--check", "unknown"}, {"verify", "x", "--check", "lint", "--check", "lint"}, {"verify", "x", "--check", "lint", "--approve"}, {"verify", "x", "--check", "lint", "--input", "missing"}, {"verify", "x", "--check", "lint", "--platform", "linux"}, {"audit", "x", "--check", "lint"}, {"bootstrap", "x", "--dry-run", "--check", "lint"}, {"report", "--check", "lint"}} {
		calls := 0
		var out, stderr bytes.Buffer
		code := Run(args, forbiddenReader{t}, &out, &stderr, Services{Verify: func(string) ([]byte, error) { calls++; return data, nil }, Audit: func(string, []string) ([]byte, error) { calls++; return data, nil }})
		if code != 2 || out.Len() != 0 || stderr.String() != "ci-trust: invalid arguments\n"+usage || calls != 0 {
			t.Fatalf("rejection effects: %v exit=%d stdout=%q stderr=%q calls=%d", args, code, out.String(), stderr.String(), calls)
		}
	}
}

func TestVerifyServiceErrorsAreBounded(t *testing.T) {
	for _, tc := range []struct {
		name, message string
		service       func(string) ([]byte, error)
	}{
		{"missing", "lint verification service unavailable", nil},
		{"failed", "lint verification failed", func(string) ([]byte, error) { return nil, errors.New("private-path\x1b must not escape") }},
		{"invalid", "invalid report", func(string) ([]byte, error) { return []byte("{}"), nil }},
		{"oversized", "report exceeds 8 MiB", func(string) ([]byte, error) { return []byte(strings.Repeat(" ", maxInput+1)), nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			code := Run([]string{"verify", "x", "--check", "lint"}, forbiddenReader{t}, &out, &stderr, Services{Verify: tc.service})
			if code != 1 || out.Len() != 0 || stderr.String() != "ci-trust: "+tc.message+"\n" {
				t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
			}
		})
	}
}
