package lintverify

import (
	"context"
	"encoding/json"
	"errors"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/dnlrsls/ci-trust/internal/audit"
	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/evidence"
	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

type record = map[string]any

const scopeLimit = "Owned TOOL_FIXTURE sensitivity only; no project configuration efficacy, historical CI execution or enforcement proof."

// Scan is the cold local route: bounded recognized files, no live services.
// The separate GitHub workflow composition remains behind the compiled gate.
func Scan(root string) ([]byte, error) {
	s := discovery.ReadSnapshot(root, discovery.Options{})
	for _, d := range s.Diagnostics {
		if d.Code == "ROOT_UNAVAILABLE" || d.Code == "UNSUPPORTED_HOST" || d.Code == "INVALID_OPTIONS" {
			return nil, errors.New("lint root unavailable")
		}
	}
	return analyze(s, func(snapshot string) outcome {
		return evaluate(context.Background(), snapshot, dependencies{catalog: productionCatalog()})
	})
}

func analyze(s discovery.Snapshot, invoke func(string) outcome) ([]byte, error) {
	base, err := audit.Analyze(s, audit.Options{})
	if err != nil {
		return nil, err
	}
	var r record
	if json.Unmarshal(base, &r) != nil {
		return nil, errors.New("invalid base report")
	}
	snapshot := strings.TrimPrefix(r["snapshot"].(map[string]any)["id"].(string), "snapshot-")
	source := lintAnchor(s)
	out := unresolved(productionCatalog().blocked())
	if unsupported(s) {
		out = unresolved("UNSUPPORTED_LINT_CONFIGURATION")
	} else if source != nil && invoke != nil {
		out = invoke(snapshot)
	}
	positive := out.decision.Outcome == ls.Sensitive || out.decision.Outcome == ls.Insensitive
	if positive {
		valid := out.decision == ls.Classify(out.observations)
		for _, o := range out.observations {
			valid = valid && o.Identity.Snapshot == snapshot
		}
		if !valid {
			out = unresolved("LINT_CAPTURE_UNVERIFIABLE")
			positive = false
		}
	}
	reason := safeReason(out.decision.Reason)
	add(r, "diagnostics", record{"id": evidence.StableID("diagnostic", "lint", reason), "code": reason, "message": "Owned lint sensitivity: " + string(out.decision.Outcome) + ". " + scopeLimit, "source": source})
	add(r, "limitations", scopeLimit)
	if productionCatalog().blocked() != "" {
		add(r, "limitations", "Prepared ESLint 10.12.0 / Node 24.22.0 candidate is held and not admitted; no live review or runtime channel is composed in production.")
	}
	if source == nil {
		add(r, "limitations", "No genuine ESLint declaration source anchor was available; no lint CHECK or capability is fabricated.")
	} else {
		check := evidence.StableID("check", snapshot, "owned-eslint")
		scope := record{"kind": "TOOL_FIXTURE", "subject": evidence.StableID("fixture", "eslint", ls.ConfigSHA256())}
		refs := []string{}
		status := "UNVERIFIABLE"
		if positive {
			status = "PRESENT"
			if out.decision.Outcome == ls.Insensitive {
				status = "ABSENT"
			}
			for _, o := range out.observations {
				id := evidence.StableID("evidence", check, string(o.Role), o.FixtureSHA256, o.Identity.Image)
				refs = append(refs, id)
				result := "PASS"
				if o.ExitCode == 1 {
					result = "FAIL"
				}
				add(r, "evidence", record{"id": id, "check_id": check, "state": "EXECUTED", "scope": scope, "provenance": record{"kind": "CONTROLLED_RUN", "adapter": "owned-eslint-v1", "basis": "RUNNER", "sources": []any{}}, "execution": record{"phase": "VALIDATION", "outcome": result, "os": o.Identity.OS, "environment_id": o.Identity.Environment, "tool": record{"name": o.Identity.Tool, "version": o.Identity.Version, "configuration_id": o.Identity.ConfigSHA256}, "fixture_role": string(o.Role)}, "cause": nil, "limitations": []string{scopeLimit, "Fixture SHA256: " + o.FixtureSHA256, "Image identity: " + o.Identity.Image}})
			}
		} else {
			id := evidence.StableID("evidence", check, reason)
			refs = append(refs, id)
			add(r, "evidence", record{"id": id, "check_id": check, "state": "UNVERIFIABLE", "scope": scope, "provenance": record{"kind": "DIAGNOSTIC", "adapter": "owned-eslint-v1", "basis": "UNRESOLVED", "sources": []any{}}, "execution": nil, "cause": nil, "limitations": []string{scopeLimit, "No admitted, authenticated lint execution is established."}})
		}
		add(r, "inventory", record{"id": check, "kind": "CHECK", "name": "Owned ESLint fixture sensitivity (declaration anchor only)", "source": source, "evidence_refs": []string{}})
		add(r, "capabilities", record{"id": evidence.StableID("capability", check, "efficacy"), "check_id": check, "dimension": "EFFICACY", "status": status, "scope": scope, "evidence_refs": refs, "limitations": []string{scopeLimit, "The source anchors the recognized declaration document, not execution or supported project configuration."}})
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return nil, errors.New("cannot assemble lint report")
	}
	encoded, ds := evidence.Encode(raw)
	if len(ds) > 0 {
		return nil, errors.New("invalid lint evidence")
	}
	return encoded, nil
}
func add(r record, key string, value any) { r[key] = append(r[key].([]any), value) }

var sensitiveAnchor = regexp.MustCompile(`(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|sk-[A-Za-z0-9_-]+|AKIA[A-Z0-9]{16})`)

func lintAnchor(s discovery.Snapshot) record {
	inv := discovery.Analyze(s, discovery.Options{})
	for _, tool := range inv.Tools {
		if tool.Name != "eslint" || sensitiveAnchor.MatchString(tool.Source.Path) {
			continue
		}
		for _, d := range s.Documents {
			if d.Path != tool.Source.Path {
				continue
			}
			line, column := 1, 1
			for _, c := range string(d.Content) {
				if !unicode.IsSpace(c) {
					return record{"path": d.Path, "line": line, "column": column}
				}
				if c == '\n' {
					line++
					column = 1
				} else {
					column++
				}
			}
		}
	}
	return nil
}
func unsupported(s discovery.Snapshot) bool {
	for _, d := range s.Documents {
		name := path.Base(d.Path)
		if strings.HasPrefix(name, ".eslintrc") || strings.HasPrefix(name, "eslint.config.") {
			return true
		}
		if name != "package.json" {
			continue
		}
		var p map[string]json.RawMessage
		if json.Unmarshal(d.Content, &p) != nil {
			continue
		}
		if _, ok := p["eslintConfig"]; ok {
			return true
		}
		for _, key := range []string{"dependencies", "devDependencies", "peerDependencies", "optionalDependencies"} {
			var deps map[string]any
			_ = json.Unmarshal(p[key], &deps)
			for name := range deps {
				if strings.HasPrefix(name, "eslint-plugin-") || strings.Contains(name, "/eslint-plugin") {
					return true
				}
			}
		}
	}
	return false
}
func safeReason(reason string) string {
	const known = "NODE_SECURITY_HOLD LINT_IMAGE_NOT_ADMITTED LINT_IDENTITY_INVALID LINT_EXECUTION_UNAVAILABLE LINT_CANCELLED LINT_REVIEW_UNAVAILABLE LINT_REVIEW_INVALID LINT_REVIEW_EXPIRED LINT_PREFLIGHT_UNAVAILABLE LINT_SOURCE_CHANGED LINT_CLEANUP_UNVERIFIED LINT_HARNESS_FAILURE LINT_CAPTURE_UNVERIFIABLE UNSUPPORTED_LINT_CONFIGURATION EXPECTED_DIAGNOSTIC FAULT_ACCEPTED UNEXPECTED_DIAGNOSTIC CONTROL_FAILED"
	for _, code := range strings.Fields(known) {
		if code == reason {
			return code
		}
	}
	return "LINT_CAPTURE_UNVERIFIABLE"
}
