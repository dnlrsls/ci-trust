package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

const publicHelp = "ci-trust audit PATH [--format human|json] [--platform linux|windows|macos ...]\nci-trust report [--input FILE|-] [--format human|json]\nci-trust bootstrap PATH --dry-run [--format human|json] [--platform linux|windows|macos ...]\nDry-run only: no apply behavior; conditional drafts are not executed or enforced.\n" + verifyHelp + "ci-trust --help\nci-trust --version\nOffline/read-only: no execution, network, hidden storage, or writes.\nExit 0: valid report (including FAIL/UNKNOWN), help/version; 1: operational error; 2: invalid invocation.\n"

func buildPublicBinary(t *testing.T) string {
	t.Helper()
	name := "ci-trust"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	command := exec.Command("go", "build", "-o", binary, ".")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("owned binary build: %v\n%s", err, output)
	}
	return binary
}

func invokePublic(t *testing.T, binary string, input []byte, args ...string) (int, []byte, []byte) {
	t.Helper()
	command := exec.Command(binary, args...)
	command.Stdin = bytes.NewReader(input)
	var out, errOut bytes.Buffer
	command.Stdout = &out
	command.Stderr = &errOut
	code := 0
	if err := command.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	return code, out.Bytes(), errOut.Bytes()
}

func assertPublic(t *testing.T, code int, out, errOut []byte, wantCode int, wantOut, wantErr string) {
	t.Helper()
	if code != wantCode || string(out) != wantOut || string(errOut) != wantErr {
		t.Fatalf("exit=%d stdout=%q stderr=%q; want exit=%d stdout=%q stderr=%q", code, out, errOut, wantCode, wantOut, wantErr)
	}
}

func TestPublicCLIReportAndInvocationContract(t *testing.T) {
	binary := buildPublicBinary(t)
	input, err := os.ReadFile(filepath.Join("..", "..", "internal", "evidence", "testdata", "golden", "empty-report.json"))
	if err != nil {
		t.Fatal(err)
	}
	expected, ds := evidence.Encode(input)
	if len(ds) != 0 {
		t.Fatalf("owned independent report fixture invalid: %v", ds)
	}
	root := filepath.Join(t.TempDir(), "source with spaces and ñ")
	if err = os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	source := map[string]string{"go.mod": "module example.test/sentinel\n\ngo 1.26\n", "counter": "0\n", "never_test.go": "package sentinel\nimport(\"os\";\"testing\")\nfunc TestForbidden(t *testing.T){os.WriteFile(\"counter\",[]byte(\"executed\"),0600);t.Fatal(\"must not execute\")}\n"}
	for name, content := range source {
		if err = os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	saved := filepath.Join(t.TempDir(), "saved report ñ.json")
	if err = os.WriteFile(saved, input, 0600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name        string
		args        []string
		input       []byte
		exit        int
		out, stderr string
	}{
		{"report-stdin", []string{"report", "--format", "json"}, input, 0, string(expected), ""},
		{"report-file", []string{"report", "--input", saved, "--format", "json"}, nil, 0, string(expected), ""},
		{"help", []string{"--help"}, nil, 0, publicHelp, ""},
		{"version", []string{"--version"}, nil, 0, "ci-trust \"dev\"\n", ""},
		{"invalid-format", []string{"audit", root, "--format", "xml"}, nil, 2, "", "ci-trust: invalid arguments\n" + publicHelp},
		{"invalid-platform", []string{"audit", root, "--platform", "arm64"}, nil, 2, "", "ci-trust: invalid arguments\n" + publicHelp},
		{"duplicate-platform", []string{"audit", root, "--platform", "linux", "--platform", "linux"}, nil, 2, "", "ci-trust: invalid arguments\n" + publicHelp},
		{"missing-target", []string{"audit", "--format", "json"}, nil, 2, "", "ci-trust: invalid arguments\n" + publicHelp},
		{"empty-stdin", []string{"report", "--format", "json"}, nil, 1, "", "ci-trust: invalid report\n"},
		{"invalid-report", []string{"report", "--format", "json"}, []byte("{}"), 1, "", "ci-trust: invalid report\n"},
		{"missing-file", []string{"report", "--input", filepath.Join(root, "absent.json")}, nil, 1, "", "ci-trust: cannot open report input\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, out, errOut := invokePublic(t, binary, tc.input, tc.args...)
			assertPublic(t, code, out, errOut, tc.exit, tc.out, tc.stderr)
			for name, want := range source {
				data, e := os.ReadFile(filepath.Join(root, name))
				if e != nil || string(data) != want {
					t.Fatalf("source/counter changed %s: %v", name, e)
				}
			}
			data, e := os.ReadFile(saved)
			if e != nil || !bytes.Equal(data, input) {
				t.Fatal("report source changed")
			}
		})
	}
}
