package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type unreadInput struct{}

func (unreadInput) Read([]byte) (int, error) { panic("command read input before gate") }
func TestControlledLintHelpArgsAndUntrustedScope(t *testing.T) {
	t.Setenv("GITHUB_ACTIONS", "")
	for _, tc := range []struct {
		args     []string
		code     int
		out, err string
	}{
		{[]string{"--help"}, 0, help, ""}, {[]string{"-h"}, 0, help, ""},
		{nil, 2, "", "controlled-lint: invalid arguments\n"},
		{[]string{"verify", "--approve"}, 2, "", "controlled-lint: invalid arguments\n"},
		{[]string{"prepare", "--image", "unowned"}, 2, "", "controlled-lint: invalid arguments\n"},
		{[]string{"execute"}, 2, "", "controlled-lint: invalid arguments\n"},
		{[]string{"prepare"}, 1, "", "controlled-lint: workflow unavailable\n"},
		{[]string{"verify"}, 1, "", "controlled-lint: workflow unavailable\n"},
	} {
		var out, stderr bytes.Buffer
		code := run(context.Background(), tc.args, unreadInput{}, &out, &stderr)
		if code != tc.code || out.String() != tc.out || stderr.String() != tc.err {
			t.Errorf("args%v: exit%d out%q stderr%q", tc.args, code, out.String(), stderr.String())
		}
	}
	if !strings.Contains(help, "no approval arguments") || !strings.Contains(help, "Exit 1:") {
		t.Error("missing help contract")
	}
}
func TestControlledLintReportIsExitDataNotAdmission(t *testing.T) {
	for _, status := range []string{"PRESENT", "ABSENT", "UNVERIFIABLE"} {
		r := map[string]any{"inventory": []any{map[string]any{"id": "owned-check", "kind": "CHECK", "name": "Owned ESLint fixture sensitivity (declaration anchor only)"}}, "capabilities": []any{map[string]any{"check_id": "owned-check", "dimension": "EFFICACY", "status": status, "scope": map[string]any{"kind": "TOOL_FIXTURE"}}}}
		raw, _ := json.Marshal(r)
		if reportPassed(raw) != (status == "PRESENT") {
			t.Error("wrong CI exit result", status)
		}
		r["inventory"] = []any{map[string]any{"id": "owned-check", "kind": "CHECK", "name": "other"}}
		raw, _ = json.Marshal(r)
		if reportPassed(raw) {
			t.Error("unrelated evidence accepted")
		}
	}
	if reportPassed([]byte("{")) || reportPassed([]byte("{}")) {
		t.Error("malformed report accepted")
	}
}
