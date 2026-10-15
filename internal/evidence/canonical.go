package evidence

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// StableID hashes length-framed inputs. Callers must exclude secrets.
func StableID(kind string, parts ...string) string {
	h := sha256.New()
	for _, s := range append([]string{kind}, parts...) {
		var n [8]byte
		binary.BigEndian.PutUint64(n[:], uint64(len(s)))
		h.Write(n[:])
		h.Write([]byte(s))
	}
	return kind + ":" + hex.EncodeToString(h.Sum(nil))
}

// Canonicalize checks JSON and semantics, not the full structural schema.
// Encode must apply structural validation first. Source integer positions are
// normalized exactly; other numbers retain their original numeric meaning.
func Canonicalize(data []byte) ([]byte, []Diagnostic) {
	v, ds := decode(data)
	if len(ds) > 0 {
		return nil, ds
	}
	if ds = knownFields(v, "root", ""); len(ds) > 0 {
		return nil, ds
	}
	if ds = semantics(v); len(ds) > 0 {
		return nil, ds
	}
	canonical(v, "")
	out, err := json.Marshal(v)
	if err != nil {
		return nil, []Diagnostic{{"INVALID_JSON", "", "Cannot serialize document."}}
	}
	return append(out, '\n'), nil
}

// Reject unknown record fields independently of the schema implementation.
func knownFields(v any, record, path string) []Diagnostic {
	fields := map[string]string{
		"root":         "schema_version snapshot inventory capabilities workflows evidence findings diagnostics limitations",
		"snapshot":     "id repository_id commit_sha",
		"inventory":    "id kind name source evidence_refs",
		"capabilities": "id check_id dimension status evidence_refs scope limitations",
		"workflows":    "id source events jobs",
		"jobs":         "id needs evidence_refs",
		"evidence":     "id check_id state scope provenance execution cause limitations",
		"findings":     "id rule_id evidence_refs scope explanation certainty certainty_rule limitations",
		"diagnostics":  "id code message source",
		"source":       "path line column", "sources": "path line column",
		"scope": "kind subject", "provenance": "kind adapter sources basis",
		"execution": "phase outcome os environment_id tool fixture_role",
		"tool":      "name version configuration_id", "cause": "category evidence_refs limitations",
	}
	var ds []Diagnostic
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			p := path + "/" + pointer(k)
			if !strings.Contains(" "+fields[record]+" ", " "+k+" ") {
				ds = append(ds, Diagnostic{"UNKNOWN_FIELD", path, "Unknown record field."})
				continue
			}
			if _, ok := fields[k]; ok {
				ds = append(ds, knownFields(x[k], k, p)...)
			}
		}
	case []any:
		for i, a := range x {
			ds = append(ds, knownFields(a, record, path+"/"+strconv.Itoa(i))...)
		}
	}
	return ds
}

func canonical(v any, key string) {
	switch x := v.(type) {
	case map[string]any:
		for k, a := range x {
			if (key == "source" || key == "sources") && (k == "line" || k == "column") {
				if n, ok := a.(json.Number); ok {
					if r, ok := new(big.Rat).SetString(string(n)); ok && r.IsInt() {
						x[k] = json.Number(r.Num().String())
					}
				}
			}
			if k == "evidence_refs" {
				refs := list(a)
				sort.SliceStable(refs, func(i, j int) bool { return text(refs[i]) < text(refs[j]) })
			}
			canonical(a, k)
		}
	case []any:
		for _, a := range x {
			canonical(a, key)
		}
		switch key {
		case "inventory", "capabilities", "workflows", "evidence", "findings", "diagnostics", "jobs":
			sort.SliceStable(x, func(i, j int) bool { return text(obj(x[i])["id"]) < text(obj(x[j])["id"]) })
		case "needs", "events":
			sort.SliceStable(x, func(i, j int) bool { return text(x[i]) < text(x[j]) })
		}
	}
}
