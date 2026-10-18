package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

// These injected dispositions test CLI transport/exit behavior, not real lint
// sensitivity. Production derives the disposition from its own fresh report.
func TestWorkflowRequestProducerAndExitDisposition(t *testing.T) {
	data, err := os.ReadFile("../evidence/testdata/golden/empty-report.json")
	if err != nil {
		t.Fatal(err)
	}
	want, ds := evidence.Encode(data)
	if len(ds) != 0 {
		t.Fatal(ds)
	}
	for _, tc := range []struct {
		name   string
		passed bool
		err    error
		exit   int
	}{
		{"sensitive", true, nil, 0}, {"unresolved", false, nil, 1},
		{"operational", false, errors.New("private error must not escape"), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			counter := filepath.Join(root, "counter")
			if err := os.WriteFile(counter, []byte("7\n"), 0600); err != nil {
				t.Fatal(err)
			}
			calls := 0
			var out, stderr bytes.Buffer
			code := Run([]string{"verify", root, "--check", "lint", "--workflow-request", "-", "--format", "json"}, bytes.NewBufferString("request data, never approval"), &out, &stderr, Services{
				Verify: func(string) ([]byte, error) { t.Fatal("workflow fell back to cold scan"); return nil, nil },
				VerifyWorkflow: func(path string, request io.Reader) ([]byte, bool, error) {
					calls++
					b, e := io.ReadAll(request)
					if e != nil || path != root || string(b) != "request data, never approval" {
						t.Fatalf("request routing: %q %q %v", path, b, e)
					}
					return data, tc.passed, tc.err
				},
			})
			wantOut, wantErr := string(want), ""
			if tc.err != nil {
				wantOut, wantErr = "", "ci-trust: lint verification failed\n"
			}
			if code != tc.exit || out.String() != wantOut || stderr.String() != wantErr || calls != 1 {
				t.Fatalf("exit%d out%q err%q calls%d", code, out.String(), stderr.String(), calls)
			}
			b, e := os.ReadFile(counter)
			if e != nil || string(b) != "7\n" {
				t.Fatal("source counter changed", e)
			}
		})
	}
}

func TestOrdinaryVerifyDoesNotUseWorkflowOrInput(t *testing.T) {
	data, err := os.ReadFile("../evidence/testdata/golden/empty-report.json")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := evidence.Encode(data)
	calls := 0
	var out, stderr bytes.Buffer
	code := Run([]string{"verify", "root", "--check", "lint", "--format", "json"}, forbiddenReader{t}, &out, &stderr, Services{
		Verify: func(path string) ([]byte, error) {
			calls++
			if path != "root" {
				t.Fatal(path)
			}
			return data, nil
		},
		VerifyWorkflow: func(string, io.Reader) ([]byte, bool, error) {
			t.Fatal("implicit workflow execution")
			return nil, false, nil
		},
	})
	if code != 0 || !bytes.Equal(out.Bytes(), want) || stderr.Len() != 0 || calls != 1 {
		t.Fatalf("exit%d out%q err%q calls%d", code, out.String(), stderr.String(), calls)
	}
}

func TestWorkflowRequestArgumentAndMissingServiceBoundaries(t *testing.T) {
	root := t.TempDir()
	counter := filepath.Join(root, "counter")
	if err := os.WriteFile(counter, []byte("7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"verify", "x", "--check", "lint", "--workflow-request", "file.json"},
		{"verify", "x", "--workflow-request", "-"},
		{"verify", "x", "--check", "lint", "--workflow-request", "-", "--workflow-request", "-"},
		{"audit", "x", "--workflow-request", "-"},
		{"report", "--workflow-request", "-"},
		{"bootstrap", "x", "--dry-run", "--workflow-request", "-"},
	} {
		for i, arg := range args {
			if arg == "x" {
				args[i] = root
			}
		}
		calls := 0
		var out, stderr bytes.Buffer
		code := Run(args, forbiddenReader{t}, &out, &stderr, Services{VerifyWorkflow: func(string, io.Reader) ([]byte, bool, error) { calls++; return nil, false, nil }})
		if code != 2 || out.Len() != 0 || stderr.String() != "ci-trust: invalid arguments\n"+usage || calls != 0 {
			t.Fatalf("args%v exit%d out%q err%q calls%d", args, code, out.String(), stderr.String(), calls)
		}
		b, err := os.ReadFile(counter)
		if err != nil || string(b) != "7\n" {
			t.Fatal("rejected input changed counter", err)
		}
	}
	var out, stderr bytes.Buffer
	code := Run([]string{"verify", "x", "--check", "lint", "--workflow-request", "-"}, forbiddenReader{t}, &out, &stderr, Services{})
	if code != 1 || out.Len() != 0 || stderr.String() != "ci-trust: lint verification service unavailable\n" {
		t.Fatalf("exit%d out%q err%q", code, out.String(), stderr.String())
	}
}
