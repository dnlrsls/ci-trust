package lintsensitivity

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"
)

const eslintOutputLimit = 64 * 1024

// This fixed module is owned configuration data for a future reviewed image.
// It imports no code and never loads configuration from the audited repository.
const eslintConfig = "export default [{languageOptions:{ecmaVersion:2022,sourceType:\"module\"},linterOptions:{noInlineConfig:true,reportUnusedDisableDirectives:\"off\"},rules:{\"no-debugger\":\"error\"}}];\n"

// ESLintInvocation is inert owned data, not an executable plan or runtime pin.
type ESLintInvocation struct {
	Args                        []string
	Stdin, Config, ConfigSHA256 string
}

// OwnedESLintInvocation describes arguments to a future pinned ESLint entrypoint.
// Paths are fixed isolated-image paths, not host paths. No program is launched.
func OwnedESLintInvocation(role Role) (ESLintInvocation, bool) {
	f, ok := FixtureFor(role)
	if !ok {
		return ESLintInvocation{}, false
	}
	return ESLintInvocation{
		Args:  []string{"--no-config-lookup", "--no-ignore", "--config", "/owned/eslint.config.mjs", "--format", "json", "--stdin", "--stdin-filename", "/work/fixture.js"},
		Stdin: f.Source, Config: eslintConfig, ConfigSHA256: digest(eslintConfig),
	}, true
}

// ESLintCapture must eventually come from a trusted bounded runner. Completed
// asserts consent, execution, cleanup and source preservation; decoding cannot.
type ESLintCapture struct {
	Status         Status
	ExitCode       int
	Stdout, Stderr []byte
}

// DecodeESLint normalizes supplied bytes only. Identity is asserted by the caller,
// not authenticated here. It never turns a failed harness result into completion.
// The identity's ConfigSHA256 remains the existing owned recipe digest; the flat
// configuration digest above must also be bound by any future execution plan.
func DecodeESLint(role Role, id Identity, capture ESLintCapture) Observation {
	o := Observation{Role: role, Identity: id, Status: InvalidOutput, ExitCode: capture.ExitCode}
	f, ok := FixtureFor(role)
	if !ok || !valid(id) {
		return o
	}
	o.FixtureSHA256 = f.SHA256
	if capture.Status != Completed {
		switch capture.Status {
		case Denied, Unavailable, TimedOut, Crashed, CleanupUnverified, InvalidOutput:
			o.Status = capture.Status
		}
		return o
	}
	if len(capture.Stdout) > eslintOutputLimit || len(capture.Stderr) > eslintOutputLimit-len(capture.Stdout) ||
		len(capture.Stderr) != 0 || (capture.ExitCode != 0 && capture.ExitCode != 1) || !uniqueJSON(capture.Stdout) {
		return o
	}
	var files []json.RawMessage
	if json.Unmarshal(capture.Stdout, &files) != nil || len(files) != 1 {
		return o
	}
	var file eslintFile
	if !object(files[0], &file,
		[]string{"filePath", "messages", "suppressedMessages", "errorCount", "fatalErrorCount", "warningCount", "fixableErrorCount", "fixableWarningCount"},
		[]string{"source", "usedDeprecatedRules"}) || file.FilePath != "/work/fixture.js" ||
		len(file.SuppressedMessages) != 0 || len(file.UsedDeprecatedRules) != 0 ||
		(file.Source != nil && *file.Source != f.Source) || len(file.Messages) > 16 ||
		file.FixableErrorCount != 0 || file.FixableWarningCount != 0 {
		return o
	}
	var diagnostics []Diagnostic
	errors, warnings, fatals := 0, 0, 0
	for _, raw := range file.Messages {
		var m eslintMessage
		if !object(raw, &m, []string{"ruleId", "severity", "message", "line", "column"},
			[]string{"fatal", "messageId", "endLine", "endColumn"}) || m.RuleID == "" || m.Message == "" ||
			(m.Severity != 1 && m.Severity != 2) || m.Line < 1 || m.Column < 1 ||
			(m.EndLine == nil) != (m.EndColumn == nil) {
			return o
		}
		if m.EndLine != nil && (*m.EndLine < m.Line || *m.EndColumn < 1 || (*m.EndLine == m.Line && *m.EndColumn < m.Column)) {
			return o
		}
		if m.Severity == 2 {
			errors++
		} else {
			warnings++
		}
		if m.Fatal {
			fatals++
			if m.Severity != 2 {
				return o
			}
		}
		diagnostics = append(diagnostics, Diagnostic{File: "fixture.js", Rule: m.RuleID, Severity: m.Severity, Line: m.Line, Column: m.Column, Fatal: m.Fatal})
	}
	if file.ErrorCount != errors || file.WarningCount != warnings || file.FatalErrorCount != fatals ||
		(capture.ExitCode == 0 && errors != 0) || (capture.ExitCode == 1 && errors == 0) {
		return o
	}
	o.Status, o.Diagnostics = Completed, diagnostics
	return o
}

type eslintFile struct {
	FilePath            string            `json:"filePath"`
	Messages            []json.RawMessage `json:"messages"`
	SuppressedMessages  []json.RawMessage `json:"suppressedMessages"`
	ErrorCount          int               `json:"errorCount"`
	FatalErrorCount     int               `json:"fatalErrorCount"`
	WarningCount        int               `json:"warningCount"`
	FixableErrorCount   int               `json:"fixableErrorCount"`
	FixableWarningCount int               `json:"fixableWarningCount"`
	Source              *string           `json:"source"`
	UsedDeprecatedRules []json.RawMessage `json:"usedDeprecatedRules"`
}

type eslintMessage struct {
	RuleID    string `json:"ruleId"`
	Severity  int    `json:"severity"`
	Message   string `json:"message"`
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	Fatal     bool   `json:"fatal"`
	MessageID string `json:"messageId"`
	EndLine   *int   `json:"endLine"`
	EndColumn *int   `json:"endColumn"`
}

// Exact keys are checked separately: encoding/json's struct matching is otherwise
// case insensitive, and absent numeric fields otherwise silently become zero.
func object(raw []byte, dst any, required, optional []string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return false
	}
	allowed := make(map[string]bool)
	for _, key := range required {
		if _, ok := fields[key]; !ok {
			return false
		}
		allowed[key] = true
	}
	for _, key := range optional {
		allowed[key] = true
	}
	for key := range fields {
		if !allowed[key] {
			return false
		}
	}
	return json.Unmarshal(raw, dst) == nil
}

// A byte cap and depth cap bound the token walk and subsequent decodes. Reject
// duplicate decoded keys, nulls, malformed UTF-8 and trailing documents before
// Go's permissive JSON struct decoder can erase those distinctions.
func uniqueJSON(raw []byte) bool {
	if !utf8.Valid(raw) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !jsonValue(d, 0) {
		return false
	}
	_, err := d.Token()
	return err == io.EOF
}

func jsonValue(d *json.Decoder, depth int) bool {
	if depth > 8 {
		return false
	}
	token, err := d.Token()
	if err != nil || token == nil {
		return false
	}
	delim, compound := token.(json.Delim)
	if !compound {
		return true
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for d.More() {
			token, err := d.Token()
			if err != nil {
				return false
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return false
			}
			seen[key] = true
			if !jsonValue(d, depth+1) {
				return false
			}
		}
	case '[':
		for d.More() {
			if !jsonValue(d, depth+1) {
				return false
			}
		}
	default:
		return false
	}
	end, err := d.Token()
	return err == nil && ((delim == '{' && end == json.Delim('}')) || (delim == '[' && end == json.Delim(']')))
}
