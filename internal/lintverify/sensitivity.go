package lintverify

import "encoding/json"

// SensitiveReport interprets just-produced owned report data for CLI/CI exit
// status only. It never grants admission, consent or execution authority.
func SensitiveReport(data []byte) bool {
	var r struct {
		Inventory    []struct{ ID, Kind, Name string }
		Capabilities []struct {
			CheckID           string `json:"check_id"`
			Dimension, Status string
			Scope             struct{ Kind string }
		}
	}
	if json.Unmarshal(data, &r) != nil {
		return false
	}
	id := ""
	checks := 0
	for _, v := range r.Inventory {
		if v.Kind == "CHECK" && v.Name == "Owned ESLint fixture sensitivity (declaration anchor only)" {
			id = v.ID
			checks++
		}
	}
	if checks != 1 || id == "" {
		return false
	}
	count := 0
	passed := false
	for _, c := range r.Capabilities {
		if c.CheckID == id && c.Dimension == "EFFICACY" {
			count++
			passed = c.Status == "PRESENT" && c.Scope.Kind == "TOOL_FIXTURE"
		}
	}
	return count == 1 && passed
}
