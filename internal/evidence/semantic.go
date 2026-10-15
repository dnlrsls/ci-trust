package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

type object = map[string]any

func obj(v any) object        { m, _ := v.(map[string]any); return m }
func text(v any) string       { s, _ := v.(string); return s }
func list(v any) []any        { a, _ := v.([]any); return a }
func pointer(s string) string { return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1") }
func decode(data []byte) (any, []Diagnostic) {
	check := json.NewDecoder(bytes.NewReader(data))
	check.UseNumber()
	if uniqueKeys(check) != nil {
		return nil, []Diagnostic{{"INVALID_JSON", "", "Invalid or ambiguous JSON document."}}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	var v any
	if d.Decode(&v) != nil {
		return nil, []Diagnostic{{"INVALID_JSON", "", "Invalid JSON document."}}
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return nil, []Diagnostic{{"INVALID_JSON", "", "Expected one JSON document."}}
	}
	return v, nil
}

// uniqueKeys rejects ambiguity before decoding into maps could discard data.
func uniqueKeys(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	switch token {
	case json.Delim('{'):
		seen := map[string]bool{}
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return err
			}
			s, ok := key.(string)
			if !ok || seen[s] {
				return fmt.Errorf("ambiguous object")
			}
			seen[s] = true
			if err := uniqueKeys(d); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	case json.Delim('['):
		for d.More() {
			if err := uniqueKeys(d); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	}
	return nil
}

// ValidateSemantics checks admissibility of supplied evidence, not its truth.
func ValidateSemantics(data []byte) []Diagnostic {
	v, ds := decode(data)
	if len(ds) > 0 {
		return ds
	}
	return semantics(v)
}
func semantics(v any) []Diagnostic {
	var ds []Diagnostic
	bad := func(code, path string) {
		ds = append(ds, Diagnostic{code, path, "Evidence contract requirement not satisfied."})
	}
	root := obj(v)
	if root == nil {
		bad("INVALID_SHAPE", "")
		return ds
	}
	collections := []string{"inventory", "capabilities", "workflows", "evidence", "findings", "diagnostics"}
	indexes := map[string]map[string]object{}
	for _, key := range collections {
		a, ok := root[key].([]any)
		if !ok {
			bad("INVALID_SHAPE", "/"+key)
		}
		index := map[string]object{}
		indexes[key] = index
		for i, x := range a {
			p := "/" + key + "/" + strconv.Itoa(i)
			m := obj(x)
			id := text(m["id"])
			if m == nil || id == "" {
				bad("INVALID_SHAPE", p)
				continue
			}
			if index[id] != nil {
				bad("DUPLICATE_ID", p+"/id")
			}
			index[id] = m
		}
	}
	refs := func(m object, p string) []object {
		var out []object
		for i, r := range list(m["evidence_refs"]) {
			e := indexes["evidence"][text(r)]
			if e == nil {
				bad("DANGLING_REFERENCE", p+"/evidence_refs/"+strconv.Itoa(i))
			} else {
				out = append(out, e)
			}
		}
		return out
	}
	for _, key := range collections {
		for i, x := range list(root[key]) {
			m := obj(x)
			p := "/" + key + "/" + strconv.Itoa(i)
			refs(m, p)
			if key == "evidence" || key == "capabilities" {
				c := indexes["inventory"][text(m["check_id"])]
				if c == nil || text(c["kind"]) != "CHECK" {
					bad("UNKNOWN_CHECK", p+"/check_id")
				}
			}
		}
	}
	for i, x := range list(root["evidence"]) {
		e := obj(x)
		p := "/evidence/" + strconv.Itoa(i)
		state := text(e["state"])
		pr := obj(e["provenance"])
		kind := text(pr["kind"])
		ex := obj(e["execution"])
		// Guard records consumed by semantic rules without duplicating schema fields.
		if obj(e["scope"]) == nil {
			bad("INVALID_SHAPE", p+"/scope")
		}
		if e["cause"] != nil && obj(e["cause"]) == nil {
			bad("INVALID_SHAPE", p+"/cause")
		}
		if e["execution"] != nil && ex == nil {
			bad("INVALID_SHAPE", p+"/execution")
		}
		if pr == nil {
			bad("INVALID_PROVENANCE", p+"/provenance")
		}
		if kind == "CONFIGURATION" && len(list(pr["sources"])) == 0 {
			bad("INVALID_PROVENANCE", p+"/provenance/sources")
		}
		switch state {
		case "EXECUTED":
			if ex == nil || text(ex["phase"]) != "VALIDATION" || (kind != "CONTROLLED_RUN" && kind != "HISTORICAL_RUN") {
				bad("INVALID_STATE", p+"/state")
			}
		case "NOT_EXECUTED":
			if ex != nil && (text(ex["phase"]) != "STARTUP" || (kind != "CONTROLLED_RUN" && kind != "HISTORICAL_RUN" && kind != "DIAGNOSTIC")) {
				bad("INVALID_STATE", p+"/execution")
			}
		case "CONFIGURED", "SKIPPED", "UNKNOWN", "UNVERIFIABLE":
			if e["execution"] != nil {
				bad("INVALID_STATE", p+"/execution")
			}
			if state == "CONFIGURED" && kind != "CONFIGURATION" || state == "SKIPPED" && kind != "HISTORICAL_RUN" {
				bad("INVALID_STATE", p+"/state")
			}
		default:
			bad("INVALID_STATE", p+"/state")
		}
		if ex != nil {
			switch text(ex["outcome"]) {
			case "PASS", "FAIL", "UNKNOWN":
			default:
				bad("INVALID_OUTCOME", p+"/execution/outcome")
			}
		}
		cause := obj(e["cause"])
		if cause != nil {
			support := refs(cause, p+"/cause")
			cat := text(cause["category"])
			if cat != "UNKNOWN" {
				basis := map[string]string{"PRODUCT": "ASSERTION", "TEST_FIXTURE": "FIXTURE", "HARNESS": "RUNNER", "ENVIRONMENT_INFRASTRUCTURE": "INFRASTRUCTURE"}[cat]
				valid := basis != "" && text(ex["outcome"]) == "FAIL" && len(list(cause["limitations"])) > 0
				if cat == "PRODUCT" && text(ex["phase"]) != "VALIDATION" {
					valid = false
				}
				backed := false
				for _, s := range support {
					if text(s["check_id"]) == text(e["check_id"]) && text(obj(s["provenance"])["basis"]) == basis {
						backed = true
					}
				}
				if !valid || !backed {
					bad("UNSUPPORTED_CAUSE", p+"/cause")
				}
			}
		}
	}
	for i, x := range list(root["capabilities"]) {
		c := obj(x)
		p := "/capabilities/" + strconv.Itoa(i)
		status := text(c["status"])
		if status == "UNKNOWN" || status == "UNVERIFIABLE" {
			continue
		}
		es := refs(c, p)
		valid := len(es) > 0
		dimension := text(c["dimension"])
		for _, e := range es {
			pr := obj(e["provenance"])
			kind := text(pr["kind"])
			if text(e["check_id"]) != text(c["check_id"]) {
				valid = false
			}
			// Evidence cannot be promoted to a broader or unrelated scope.
			if !equalJSON(e["scope"], c["scope"]) {
				valid = false
			}
			switch dimension {
			case "CONFIGURATION":
				if kind != "CONFIGURATION" || text(e["state"]) != "CONFIGURED" {
					valid = false
				}
			case "ENFORCEMENT":
				if kind != "REMOTE_POLICY" {
					valid = false
				}
			case "EXECUTION":
				s := text(e["state"])
				if status == "PRESENT" && s != "EXECUTED" || status == "ABSENT" && s != "SKIPPED" && s != "NOT_EXECUTED" {
					valid = false
				}
				if kind == "CONTROLLED_RUN" && text(obj(c["scope"])["kind"]) != "TOOL_FIXTURE" {
					valid = false
				}
			case "EFFICACY":
				if kind != "CONTROLLED_RUN" || text(e["state"]) != "EXECUTED" || text(obj(c["scope"])["kind"]) != "TOOL_FIXTURE" {
					valid = false
				}
			default:
				valid = false
			}
		}
		if dimension == "EFFICACY" {
			if len(es) != 3 {
				valid = false
			} else {
				// Reference arrays are sets. Roles, not caller order, define the
				// CLEAN / FAULT / RESTORED comparison.
				roles := map[string]object{}
				for _, e := range es {
					role := text(obj(e["execution"])["fixture_role"])
					if roles[role] != nil {
						valid = false
					}
					roles[role] = e
				}
				for _, role := range []string{"CLEAN", "FAULT", "RESTORED"} {
					if roles[role] == nil {
						valid = false
					}
				}
				first := obj(roles["CLEAN"]["execution"])
				tool := obj(first["tool"])
				if text(first["environment_id"]) == "" || text(first["os"]) == "" || text(tool["name"]) == "" || text(tool["version"]) == "" || text(tool["configuration_id"]) == "" {
					valid = false
				}
				for role, e := range roles {
					ex := obj(e["execution"])
					out := "PASS"
					if role == "FAULT" && status == "PRESENT" {
						out = "FAIL"
					}
					if text(ex["outcome"]) != out || !equalJSON(ex["tool"], first["tool"]) || !equalJSON(ex["environment_id"], first["environment_id"]) || !equalJSON(ex["os"], first["os"]) {
						valid = false
					}
				}
			}
		}
		if !valid {
			bad("UNSUPPORTED_CAPABILITY", p)
		}
	}
	for i, x := range list(root["workflows"]) {
		w := obj(x)
		p := "/workflows/" + strconv.Itoa(i)
		seenEvents := map[string]bool{}
		for j, event := range list(w["events"]) {
			s := text(event)
			if seenEvents[s] {
				bad("DUPLICATE_VALUE", p+"/events/"+strconv.Itoa(j))
			}
			seenEvents[s] = true
		}
		jobs := map[string]object{}
		for j, y := range list(w["jobs"]) {
			m := obj(y)
			id := text(m["id"])
			if id == "" || jobs[id] != nil {
				bad("DUPLICATE_ID", p+"/jobs/"+strconv.Itoa(j)+"/id")
			}
			jobs[id] = m
			refs(m, p+"/jobs/"+strconv.Itoa(j))
		}
		colors := map[string]int{}
		var visit func(string)
		visit = func(id string) {
			if colors[id] == 1 {
				bad("WORKFLOW_CYCLE", p+"/jobs")
				return
			}
			if colors[id] == 2 {
				return
			}
			colors[id] = 1
			for _, n := range list(jobs[id]["needs"]) {
				s := text(n)
				if jobs[s] == nil {
					bad("DANGLING_REFERENCE", p+"/jobs")
				} else {
					visit(s)
				}
			}
			colors[id] = 2
		}
		keys := make([]string, 0, len(jobs))
		for id := range jobs {
			keys = append(keys, id)
		}
		sort.Strings(keys)
		for _, id := range keys {
			visit(id)
		}
	}
	return ds
}
func equalJSON(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
