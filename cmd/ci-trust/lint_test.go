package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/evidence"
)

const verifyHelp = "ci-trust verify PATH --check lint [--format human|json] [--workflow-request -]\nOwned ESLint 10.12.0 / Node 24.22.0: currently UNVERIFIABLE (security hold; runtime not admitted).\nTOOL_FIXTURE only: CLEAN 0 -> FAULT 1 (no-debugger error at 1:1) -> RESTORED 0; exact human consent and preflight required; project configs/plugins are never loaded.\n"

func expectedHeldEmptyReport(t *testing.T) ([]byte, string) {
	t.Helper()
	sum := sha256.Sum256(nil)
	scope := "Owned TOOL_FIXTURE sensitivity only; no project configuration efficacy, historical CI execution or enforcement proof."
	// Authored expectation, not captured from lintverify.Scan or the public binary.
	r := map[string]any{"schema_version": "1", "snapshot": map[string]any{"id": "snapshot-" + hex.EncodeToString(sum[:]), "repository_id": nil, "commit_sha": nil}}
	for _, k := range []string{"inventory", "capabilities", "workflows", "evidence", "findings"} {
		r[k] = []any{}
	}
	r["diagnostics"] = []any{
		map[string]any{"id": evidence.StableID("diagnostic", "NO_SOURCE_ANCHOR", "", "0 0"), "code": "NO_SOURCE_ANCHOR", "message": "Static analysis could not resolve this input; no execution or enforcement is inferred.", "source": nil},
		map[string]any{"id": evidence.StableID("diagnostic", "lint", "NODE_SECURITY_HOLD"), "code": "NODE_SECURITY_HOLD", "message": "Owned lint sensitivity: UNVERIFIABLE. " + scope, "source": nil},
	}
	r["limitations"] = []string{
		"Offline bounded configuration only. Historical execution, efficacy and required-check enforcement are UNKNOWN. Runner OS is not a cross-compilation target. Generic TEST intent is not unit, integration or E2E completeness.",
		scope, "Prepared ESLint 10.12.0 / Node 24.22.0 candidate is held and not admitted; no live review or runtime channel is composed in production.",
		"No genuine ESLint declaration source anchor was available; no lint CHECK or capability is fabricated.",
	}
	raw, _ := json.Marshal(r)
	canonical, ds := evidence.Encode(raw)
	if len(ds) > 0 {
		t.Fatal(ds)
	}
	var expected struct {
		Diagnostics []struct{ Code, Message string }
	}
	if json.Unmarshal(canonical, &expected) != nil {
		t.Fatal("expectation decode")
	}
	stderr := ""
	for _, d := range expected.Diagnostics {
		stderr += fmt.Sprintf("%s: %s\n", d.Code, d.Message)
	}
	return canonical, stderr
}

func TestPublicWorkflowRequestHeldAndNoEffects(t *testing.T) {
	binary := buildPublicBinary(t)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "counter"), []byte("7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := fixtureBytes(t, root)
	code, out, stderr := invokePublic(t, binary, []byte("not an approval or valid request"), "verify", root, "--check", "lint", "--workflow-request", "-", "--format", "json")
	assertPublic(t, code, out, stderr, 1, "", "ci-trust: lint verification failed\n")
	unchangedFixture(t, root, before)
}

func TestPublicVerifyHeldReportAndNoEffects(t *testing.T) {
	binary := buildPublicBinary(t)
	root := t.TempDir()
	marker := filepath.Join(root, "counter")
	if err := os.WriteFile(marker, []byte("7\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := fixtureBytes(t, root)
	want, wantErr := expectedHeldEmptyReport(t)
	code, out, stderr := invokePublic(t, binary, nil, "verify", root, "--check", "lint", "--format", "json")
	assertPublic(t, code, out, stderr, 0, string(want), wantErr)
	unchangedFixture(t, root, before)
	code, out, stderr = invokePublic(t, binary, nil, "verify", "--check", "lint", "--format", "json", "--", root)
	assertPublic(t, code, out, stderr, 0, string(want), wantErr)
	unchangedFixture(t, root, before)
	code, out, stderr = invokePublic(t, binary, want, "report", "--format", "json")
	assertPublic(t, code, out, stderr, 0, string(want), wantErr)
	code, out, stderr = invokePublic(t, binary, nil, "help", "verify")
	assertPublic(t, code, out, stderr, 0, verifyHelp+publicWorkflowEffects, "")
	for _, args := range [][]string{
		{"verify", root}, {"verify", root, "--check", "other"}, {"verify", root, "--check", "lint", "--check", "lint"},
		{"verify", root, "--check", "lint", "--approve"}, {"verify", root, "--check", "lint", "--platform", "linux"},
		{"audit", root, "--check", "lint"}, {"bootstrap", root, "--dry-run", "--check", "lint"}, {"report", "--check", "lint"},
	} {
		code, out, stderr = invokePublic(t, binary, nil, args...)
		assertPublic(t, code, out, stderr, 2, "", "ci-trust: invalid arguments\n"+publicHelp)
		unchangedFixture(t, root, before)
	}
	code, out, stderr = invokePublic(t, binary, nil, "verify", filepath.Join(root, "absent"), "--check", "lint")
	assertPublic(t, code, out, stderr, 1, "", "ci-trust: lint verification failed\n")
	unchangedFixture(t, root, before)
}
