package evidence

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func report() map[string]any {
	return map[string]any{"schema_version": "1", "snapshot": map[string]any{"id": "s", "repository_id": nil, "commit_sha": nil}, "inventory": []any{map[string]any{"id": "check", "kind": "CHECK", "name": "test", "source": source(), "evidence_refs": []any{}}}, "capabilities": []any{}, "workflows": []any{}, "evidence": []any{}, "findings": []any{}, "diagnostics": []any{}, "limitations": []any{}}
}
func source() map[string]any { return map[string]any{"path": "ci.yml", "line": 1, "column": 1} }
func scope(kind, subject string) map[string]any {
	return map[string]any{"kind": kind, "subject": subject}
}
func observation(state, kind string) map[string]any {
	return map[string]any{"id": "e", "check_id": "check", "state": state, "scope": scope("TOOL_FIXTURE", "fixture"), "provenance": map[string]any{"kind": kind, "basis": "SOURCE", "adapter": "adapter", "sources": []any{source()}}, "execution": nil, "cause": nil, "limitations": []any{}}
}
func invocation() map[string]any {
	return map[string]any{"phase": "VALIDATION", "outcome": "FAIL", "os": "LINUX", "environment_id": "env", "tool": map[string]any{"name": "test", "version": "1", "configuration_id": "config"}, "fixture_role": nil}
}
func capability(dimension, status string, e map[string]any) map[string]any {
	return map[string]any{"id": "c", "check_id": "check", "dimension": dimension, "status": status, "scope": e["scope"], "evidence_refs": []any{e["id"]}, "limitations": []any{}}
}
func encoded(t *testing.T, r map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func assertVerdict(t *testing.T, r map[string]any, valid bool, code string) {
	t.Helper()
	b := encoded(t, r)
	saved := bytes.Clone(b)
	ds := ValidateSemantics(b)
	if (len(ds) == 0) != valid {
		t.Fatalf("acceptance mismatch: %v", ds)
	}
	if !bytes.Equal(saved, b) {
		t.Fatal("input mutated")
	}
	found := code == ""
	for _, d := range ds {
		if d.Code == code {
			found = true
		}
		if d.Code == "" || !strings.HasPrefix(d.Path, "/") {
			t.Fatalf("unattributed diagnostic: %v", d)
		}
		if strings.Contains(d.Message, "private-value") || strings.Contains(d.Path, "private-value") {
			t.Fatal("diagnostic leaked supplied value")
		}
	}
	if !found {
		t.Fatalf("missing diagnostic %s: %v", code, ds)
	}
	if !valid {
		out, cd := Canonicalize(b)
		if out != nil || len(cd) == 0 {
			t.Fatal("canonical output after semantic rejection")
		}
	}
}
func TestSemanticStates(t *testing.T) {
	for _, tc := range []struct {
		state, kind string
		valid       bool
	}{{"CONFIGURED", "CONFIGURATION", true}, {"UNKNOWN", "DIAGNOSTIC", true}, {"UNVERIFIABLE", "DIAGNOSTIC", true}, {"NOT_EXECUTED", "CONFIGURATION", true}, {"SKIPPED", "HISTORICAL_RUN", true}, {"SKIPPED", "CONFIGURATION", false}, {"EXECUTED", "CONFIGURATION", false}} {
		t.Run(tc.state+tc.kind, func(t *testing.T) {
			r := report()
			r["evidence"] = []any{observation(tc.state, tc.kind)}
			assertVerdict(t, r, tc.valid, "")
		})
	}
}
func TestCauseMatrix(t *testing.T) {
	for _, pair := range []struct{ category, basis string }{{"PRODUCT", "ASSERTION"}, {"TEST_FIXTURE", "FIXTURE"}, {"HARNESS", "RUNNER"}, {"ENVIRONMENT_INFRASTRUCTURE", "INFRASTRUCTURE"}} {
		for _, scenario := range []string{"supported", "exit only", "unrelated check", "missing limitations", "no refs", "not failed", "startup"} {
			t.Run(pair.category+"/"+scenario, func(t *testing.T) {
				r := report()
				e := observation("EXECUTED", "CONTROLLED_RUN")
				ex := invocation()
				e["execution"] = ex
				support := observation("UNKNOWN", "DIAGNOSTIC")
				support["id"] = "support"
				support["provenance"].(map[string]any)["basis"] = pair.basis
				cause := map[string]any{"category": pair.category, "evidence_refs": []any{"support"}, "limitations": []any{"bounded attribution"}}
				e["cause"] = cause
				valid := scenario == "supported" || scenario == "startup" && pair.category != "PRODUCT"
				switch scenario {
				case "exit only":
					support["provenance"].(map[string]any)["basis"] = "EXIT_STATUS"
				case "unrelated check":
					r["inventory"] = append(r["inventory"].([]any), map[string]any{"id": "other", "kind": "CHECK", "name": "other", "source": source(), "evidence_refs": []any{}})
					support["check_id"] = "other"
				case "missing limitations":
					cause["limitations"] = []any{}
				case "no refs":
					cause["evidence_refs"] = []any{}
				case "not failed":
					ex["outcome"] = "PASS"
				case "startup":
					e["state"] = "NOT_EXECUTED"
					ex["phase"] = "STARTUP"
				}
				r["evidence"] = []any{e, support}
				code := ""
				if !valid {
					code = "UNSUPPORTED_CAUSE"
				}
				assertVerdict(t, r, valid, code)
			})
		}
	}
	for _, refs := range []bool{false, true} {
		t.Run(map[bool]string{false: "unknown no refs", true: "unknown exit status"}[refs], func(t *testing.T) {
			r := report()
			e := observation("EXECUTED", "CONTROLLED_RUN")
			e["execution"] = invocation()
			e["provenance"].(map[string]any)["basis"] = "EXIT_STATUS"
			ids := []any{}
			if refs {
				ids = append(ids, "e")
			}
			e["cause"] = map[string]any{"category": "UNKNOWN", "evidence_refs": ids, "limitations": []any{"ambiguous"}}
			r["evidence"] = []any{e}
			assertVerdict(t, r, true, "")
		})
	}
}
func TestCapabilityMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, kind, state, dimension, status string
		promote, valid                       bool
	}{
		{"remote policy", "REMOTE_POLICY", "UNKNOWN", "ENFORCEMENT", "PRESENT", false, true},
		{"yaml enforcement", "CONFIGURATION", "CONFIGURED", "ENFORCEMENT", "PRESENT", false, false},
		{"controlled enforcement", "CONTROLLED_RUN", "EXECUTED", "ENFORCEMENT", "PRESENT", false, false},
		{"historical same scope", "HISTORICAL_RUN", "EXECUTED", "EXECUTION", "PRESENT", false, true},
		{"historical promotion", "HISTORICAL_RUN", "EXECUTED", "EXECUTION", "PRESENT", true, false},
		{"controlled promotion", "CONTROLLED_RUN", "EXECUTED", "EXECUTION", "PRESENT", true, false},
		{"historical skip", "HISTORICAL_RUN", "SKIPPED", "EXECUTION", "ABSENT", false, true},
		{"known omission", "CONFIGURATION", "NOT_EXECUTED", "EXECUTION", "ABSENT", false, true},
		{"unknown execution", "DIAGNOSTIC", "UNKNOWN", "EXECUTION", "PRESENT", false, false},
		{"unverifiable execution", "DIAGNOSTIC", "UNVERIFIABLE", "EXECUTION", "PRESENT", false, false},
		{"configuration", "CONFIGURATION", "CONFIGURED", "CONFIGURATION", "PRESENT", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := report()
			e := observation(tc.state, tc.kind)
			if tc.state == "EXECUTED" {
				e["execution"] = invocation()
			}
			c := capability(tc.dimension, tc.status, e)
			if tc.promote {
				c["scope"] = scope("REPOSITORY", "repo")
			}
			r["evidence"] = []any{e}
			r["capabilities"] = []any{c}
			assertVerdict(t, r, tc.valid, "")
		})
	}
}
func efficacyReport() (map[string]any, []map[string]any, map[string]any) {
	r := report()
	es := []map[string]any{}
	items := []any{}
	ids := []any{}
	for _, role := range []string{"CLEAN", "FAULT", "RESTORED"} {
		e := observation("EXECUTED", "CONTROLLED_RUN")
		e["id"] = role
		ex := invocation()
		ex["fixture_role"] = role
		if role != "FAULT" {
			ex["outcome"] = "PASS"
		}
		e["execution"] = ex
		es = append(es, e)
		items = append(items, e)
		ids = append(ids, role)
	}
	c := capability("EFFICACY", "PRESENT", es[0])
	c["evidence_refs"] = ids
	r["evidence"] = items
	r["capabilities"] = []any{c}
	return r, es, c
}
func TestEfficacyMatrix(t *testing.T) {
	for _, scenario := range []string{"present", "absent", "reversed", "duplicate role", "duplicate reference", "missing role", "wrong outcome", "wrong check", "wrong scope", "wrong tool", "wrong version", "wrong config", "wrong os", "wrong environment", "unknown tool", "unknown version", "unknown config", "unknown os", "unknown environment", "malformed tool", "malformed os", "malformed environment"} {
		t.Run(scenario, func(t *testing.T) {
			r, es, c := efficacyReport()
			ex := es[1]["execution"].(map[string]any)
			tool := ex["tool"].(map[string]any)
			valid := scenario == "present" || scenario == "absent" || scenario == "reversed"
			switch scenario {
			case "absent":
				c["status"] = "ABSENT"
				ex["outcome"] = "PASS"
			case "reversed":
				c["evidence_refs"] = []any{"RESTORED", "FAULT", "CLEAN"}
			case "duplicate role":
				ex["fixture_role"] = "CLEAN"
			case "duplicate reference":
				c["evidence_refs"] = []any{"CLEAN", "FAULT", "FAULT"}
			case "missing role":
				c["evidence_refs"] = []any{"CLEAN", "FAULT"}
			case "wrong outcome":
				ex["outcome"] = "PASS"
			case "wrong check":
				r["inventory"] = append(r["inventory"].([]any), map[string]any{"id": "other", "kind": "CHECK", "name": "other", "source": source(), "evidence_refs": []any{}})
				es[1]["check_id"] = "other"
			case "wrong scope":
				es[1]["scope"] = scope("TOOL_FIXTURE", "other")
			case "wrong tool":
				tool["name"] = "other"
			case "wrong version":
				tool["version"] = "2"
			case "wrong config":
				tool["configuration_id"] = "other"
			case "wrong os":
				ex["os"] = "WINDOWS"
			case "wrong environment":
				ex["environment_id"] = "other"
			case "unknown tool":
				for _, e := range es {
					e["execution"].(map[string]any)["tool"].(map[string]any)["name"] = nil
				}
			case "unknown version":
				for _, e := range es {
					e["execution"].(map[string]any)["tool"].(map[string]any)["version"] = nil
				}
			case "unknown config":
				for _, e := range es {
					e["execution"].(map[string]any)["tool"].(map[string]any)["configuration_id"] = nil
				}
			case "unknown os":
				for _, e := range es {
					e["execution"].(map[string]any)["os"] = nil
				}
			case "unknown environment":
				for _, e := range es {
					e["execution"].(map[string]any)["environment_id"] = nil
				}
			case "malformed tool":
				ex["tool"] = []any{"private-value"}
			case "malformed os":
				ex["os"] = map[string]any{"private-value": true}
			case "malformed environment":
				ex["environment_id"] = []any{"private-value"}
			}
			assertVerdict(t, r, valid, "")
		})
	}
}
func TestNestedMalformed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"provenance", func(e map[string]any) { e["provenance"] = []any{"private-value"} }},
		{"execution", func(e map[string]any) { e["execution"] = []any{"private-value"} }},
		{"cause", func(e map[string]any) { e["cause"] = "private-value" }},
		{"scope", func(e map[string]any) { e["scope"] = []any{"private-value"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := report()
			e := observation("EXECUTED", "CONTROLLED_RUN")
			e["execution"] = invocation()
			tc.mutate(e)
			r["evidence"] = []any{e}
			assertVerdict(t, r, false, "")
		})
	}
}
func TestGraphAndReferenceFailures(t *testing.T) {
	for _, scenario := range []string{"dangling finding", "unknown check", "duplicate id", "duplicate events", "cycle", "dangling needs"} {
		t.Run(scenario, func(t *testing.T) {
			r := report()
			e := observation("UNKNOWN", "DIAGNOSTIC")
			r["evidence"] = []any{e}
			switch scenario {
			case "dangling finding":
				r["findings"] = []any{map[string]any{"id": "f", "evidence_refs": []any{"missing"}}}
			case "unknown check":
				e["check_id"] = "missing"
			case "duplicate id":
				r["evidence"] = []any{e, e}
			default:
				events := []any{"push"}
				needs := []any{"j"}
				if scenario == "duplicate events" {
					events = append(events, "push")
					needs = []any{}
				}
				if scenario == "dangling needs" {
					needs = []any{"missing"}
				}
				r["workflows"] = []any{map[string]any{"id": "w", "source": source(), "events": events, "jobs": []any{map[string]any{"id": "j", "needs": needs, "evidence_refs": []any{}}}}}
			}
			assertVerdict(t, r, false, "")
		})
	}
}
func TestUnknownFindingAccepted(t *testing.T) {
	r := report()
	e := observation("UNKNOWN", "DIAGNOSTIC")
	r["evidence"] = []any{e}
	r["findings"] = []any{map[string]any{"id": "f", "rule_id": "rule", "evidence_refs": []any{"e"}, "scope": e["scope"], "explanation": "unknown history", "certainty": "UNKNOWN", "certainty_rule": "unresolved", "limitations": []any{}}}
	assertVerdict(t, r, true, "")
}
func TestSemanticMalformed(t *testing.T) {
	for _, s := range []string{`null`, `[]`, `{`, `{"inventory":[null]}`} {
		if len(ValidateSemantics([]byte(s))) == 0 {
			t.Fatal("malformed document accepted")
		}
	}
}
