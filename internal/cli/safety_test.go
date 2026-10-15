package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type forbiddenReader struct{ t *testing.T }

func (r forbiddenReader) Read([]byte) (int, error) {
	r.t.Fatal("unexpected input read")
	return 0, io.EOF
}

func TestRejectedArgumentsHaveNoEffects(t *testing.T) {
	for _, args := range [][]string{nil, {"audit"}, {"audit", "x", "y"}, {"audit", "x", "--format", "xml"}, {"audit", "x", "--platform", "bsd"}, {"audit", "x", "--platform", "linux", "--platform", "linux"}, {"report", "--input", "missing", "--format", "bad"}, {"report", "x"}, {"report", "--format", "json", "--format", "human"}, {"verify", "x"}, {"bootstrap", "x"}, {"audit", "x", "--eval", "oops"}, {"audit", ""}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var out, err bytes.Buffer
			code := Run(args, forbiddenReader{t}, &out, &err, Services{Audit: func(string, []string) ([]byte, error) { t.Fatal("unexpected audit"); return nil, nil }})
			if code != 2 || out.Len() != 0 || err.String() != "ci-trust: invalid arguments\n"+usage {
				t.Fatalf("%d %q %q", code, out.String(), err.String())
			}
		})
	}
}

func TestReportErrors(t *testing.T) {
	for _, tt := range []struct {
		name, input, message string
		interactive          bool
	}{
		{"empty", "", "invalid report", false}, {"invalid", "{", "invalid report", false},
		{"version", `{"schema_version":"2"}`, "invalid report", false},
		{"duplicate", `{"schema_version":"1","schema_version":"1"}`, "invalid report", false},
		{"oversize", strings.Repeat(" ", maxInput+1), "report exceeds 8 MiB", false},
		{"interactive", "", "report input required (interactive stdin)", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var out, err bytes.Buffer
			var reader io.Reader = strings.NewReader(tt.input)
			if tt.interactive {
				reader = forbiddenReader{t}
			}
			code := Run([]string{"report"}, reader, &out, &err, Services{InteractiveInput: tt.interactive})
			if code != 1 || out.Len() != 0 || err.String() != "ci-trust: "+tt.message+"\n" {
				t.Fatalf("%d %q %q", code, out.String(), err.String())
			}
		})
	}
}

func TestFileAndSamePathParity(t *testing.T) {
	data, e := os.ReadFile("../evidence/testdata/golden/empty-report.json")
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "report ü space.json")
	if e = os.WriteFile(path, data, 0600); e != nil {
		t.Fatal(e)
	}
	for _, format := range []string{"json", "human"} {
		var fileOut, stdinOut, stderr bytes.Buffer
		if Run([]string{"report", "--input", path, "--format", format}, forbiddenReader{t}, &fileOut, &stderr, Services{}) != 0 {
			t.Fatal(stderr.String())
		}
		if Run([]string{"report", "--format", format}, bytes.NewReader(data), &stdinOut, &stderr, Services{}) != 0 || stdinOut.String() != fileOut.String() || stderr.Len() != 0 {
			t.Fatal("parity")
		}
	}
	var out, stderr bytes.Buffer
	if Run([]string{"audit", path, "--format", "bad"}, forbiddenReader{t}, &out, &stderr, Services{Audit: func(string, []string) ([]byte, error) { t.Fatal("audit called"); return nil, nil }}) != 2 {
		t.Fatal("exit")
	}
	after, e := os.ReadFile(path)
	if e != nil || !bytes.Equal(after, data) {
		t.Fatal("source changed")
	}
}

func TestHelpVersion(t *testing.T) {
	for _, tt := range []struct {
		args []string
		want string
	}{{[]string{"--help"}, usage}, {[]string{"help"}, usage}, {[]string{"audit", "--help"}, auditUsage + effects}, {[]string{"report", "help"}, reportUsage + effects}, {[]string{"help", "report"}, reportUsage + effects}, {[]string{"--version"}, "ci-trust \"v1\\n\"\n"}} {
		var out, err bytes.Buffer
		if Run(tt.args, forbiddenReader{t}, &out, &err, Services{Version: "v1\n"}) != 0 || out.String() != tt.want || err.Len() != 0 {
			t.Fatalf("%v %q %q", tt.args, out.String(), err.String())
		}
	}
}

func TestAuditErrorsAndOverrides(t *testing.T) {
	data, e := os.ReadFile("../evidence/testdata/golden/empty-report.json")
	if e != nil {
		t.Fatal(e)
	}
	var out, stderr bytes.Buffer
	calls := 0
	code := Run([]string{"audit", "--platform", "macos", "--platform", "windows", "--", "-ü space"}, forbiddenReader{t}, &out, &stderr, Services{Audit: func(path string, p []string) ([]byte, error) {
		calls++
		if path != "-ü space" || strings.Join(p, ",") != "macos,windows" {
			t.Fatal(path, p)
		}
		return data, nil
	}})
	if code != 0 || calls != 1 || stderr.Len() != 0 {
		t.Fatal(code, calls, stderr.String())
	}
	out.Reset()
	stderr.Reset()
	code = Run([]string{"audit", "x"}, forbiddenReader{t}, &out, &stderr, Services{Audit: func(string, []string) ([]byte, error) { return nil, errors.New("untrusted\x1b error") }})
	if code != 1 || out.Len() != 0 || stderr.String() != "ci-trust: audit failed\n" {
		t.Fatal(code, out.String(), stderr.String())
	}
}
