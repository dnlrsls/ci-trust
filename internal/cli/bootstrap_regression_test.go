package cli

import (
	"bytes"
	"os"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/bootstrap"
)

func TestBootstrapHelpAndExistingCommandHelp(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"audit", "--help"}, auditUsage + effects},
		{[]string{"report", "--help"}, reportUsage + effects},
		{[]string{"bootstrap", "--help"}, bootstrapUsage + effects},
		{[]string{"help", "bootstrap"}, bootstrapUsage + effects},
		{[]string{"--version"}, "ci-trust \"test\"\n"},
	} {
		var out, stderr bytes.Buffer
		code := Run(tc.args, nil, &out, &stderr, Services{Version: "test"})
		if code != 0 || out.String() != tc.want || stderr.Len() != 0 {
			t.Fatalf("%v: exit=%d stdout=%q stderr=%q", tc.args, code, out.String(), stderr.String())
		}
	}
}

func TestBootstrapLimitedPlanIsSuccess(t *testing.T) {
	data, err := os.ReadFile("../evidence/testdata/golden/empty-report.json")
	if err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	code := Run([]string{"bootstrap", "--dry-run", "--format", "json", "--", "-explicit-target"}, nil, &out, &stderr, Services{Bootstrap: func(p string, platforms []string) (bootstrap.Plan, error) {
		if p != "-explicit-target" || len(platforms) != 0 {
			t.Fatal(p, platforms)
		}
		return bootstrap.Plan{Version: bootstrap.Version, DryRun: true, SourceReport: data, Limitations: []string{"Unsupported stack; no invented setup"}}, nil
	}})
	want := "{\"version\":\"ci-trust-bootstrap-plan/v1\",\"dry_run\":true,\"source_report\":" + string(bytes.TrimSpace(data)) + ",\"recommendations\":null,\"limitations\":[\"Unsupported stack; no invented setup\"]}\n"
	if code != 0 || out.String() != want || stderr.Len() != 0 {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), stderr.String())
	}
}
