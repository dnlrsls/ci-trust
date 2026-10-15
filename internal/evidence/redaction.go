package evidence

import (
	"encoding/json"
	"sort"
	"strings"
)

// Redact replaces explicit sensitive strings; it is not a secret detector.
func Redact(data []byte, sensitive []string) ([]byte, []Diagnostic) {
	v, ds := decode(data)
	if len(ds) > 0 {
		return nil, ds
	}
	secrets := []string{}
	seen := map[string]bool{}
	for _, s := range sensitive {
		if s != "" && !seen[s] {
			secrets = append(secrets, s)
			seen[s] = true
		}
	}
	sort.Slice(secrets, func(i, j int) bool {
		if len(secrets[i]) == len(secrets[j]) {
			return secrets[i] < secrets[j]
		}
		return len(secrets[i]) > len(secrets[j])
	})
	pairs := []string{}
	for _, s := range secrets {
		pairs = append(pairs, s, "[REDACTED]")
	}
	replace := strings.NewReplacer(pairs...)
	var walk func(any) (any, bool)
	walk = func(v any) (any, bool) {
		switch x := v.(type) {
		case string:
			return replace.Replace(x), true
		case []any:
			for i, a := range x {
				n, ok := walk(a)
				if !ok {
					return nil, false
				}
				x[i] = n
			}
		case map[string]any:
			n := map[string]any{}
			for k, a := range x {
				key := replace.Replace(k)
				if _, exists := n[key]; exists {
					return nil, false
				}
				b, ok := walk(a)
				if !ok {
					return nil, false
				}
				n[key] = b
			}
			return n, true
		}
		return v, true
	}
	out, ok := walk(v)
	if !ok {
		return nil, []Diagnostic{{"REDACTION_COLLISION", "", "Redaction would merge object keys."}}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return nil, []Diagnostic{{"INVALID_JSON", "", "Cannot serialize document."}}
	}
	return append(b, '\n'), nil
}
