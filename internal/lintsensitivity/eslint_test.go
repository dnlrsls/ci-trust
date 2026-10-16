package lintsensitivity

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Authored formatter-shaped data, never captured from executing ESLint.
func eslintJSON(fault bool) []byte {
	messages := `[]`
	count := `0`
	if fault {
		messages = `[{"ruleId":"no-debugger","severity":2,"message":"Unexpected 'debugger' statement.","line":1,"column":1,"messageId":"unexpected","endLine":1,"endColumn":10}]`
		count = `1`
	}
	return []byte(`[{"filePath":"/work/fixture.js","messages":` + messages + `,"suppressedMessages":[],"errorCount":` + count + `,"fatalErrorCount":0,"warningCount":0,"fixableErrorCount":0,"fixableWarningCount":0,"usedDeprecatedRules":[]}]`)
}

func TestESLintTriplet(t *testing.T) {
	var got []Observation
	for _, want := range observations(t) {
		capture := ESLintCapture{Status: Completed, ExitCode: want.ExitCode, Stdout: eslintJSON(want.Role == Fault)}
		before := bytes.Clone(capture.Stdout)
		o := DecodeESLint(want.Role, want.Identity, capture)
		if !reflect.DeepEqual(o, want) || !bytes.Equal(before, capture.Stdout) {
			t.Fatalf("unexpected observation: %+v", o)
		}
		got = append(got, o)
	}
	if Classify(got).Outcome != Sensitive {
		t.Fatal("triplet not sensitive")
	}
	got[1] = DecodeESLint(Fault, got[1].Identity, ESLintCapture{Status: Completed, Stdout: eslintJSON(false)})
	if Classify(got).Outcome != Insensitive {
		t.Fatal("no-op not insensitive")
	}
}

func TestOwnedESLintInvocation(t *testing.T) {
	for _, role := range []Role{Clean, Fault, Restored} {
		got, ok := OwnedESLintInvocation(role)
		f, _ := FixtureFor(role)
		wantArgs := []string{"--no-config-lookup", "--no-ignore", "--config", "/owned/eslint.config.mjs", "--format", "json", "--stdin", "--stdin-filename", "/work/fixture.js"}
		wantConfig := "export default [{languageOptions:{ecmaVersion:2022,sourceType:\"module\"},linterOptions:{noInlineConfig:true,reportUnusedDisableDirectives:\"off\"},rules:{\"no-debugger\":\"error\"}}];\n"
		if !ok || !reflect.DeepEqual(got.Args, wantArgs) || got.Stdin != f.Source || got.Config != wantConfig || got.ConfigSHA256 != digest(wantConfig) {
			t.Fatalf("unexpected invocation: %+v", got)
		}
		got.Args[0] = "--fix"
		again, _ := OwnedESLintInvocation(role)
		if again.Args[0] != wantArgs[0] {
			t.Fatal("mutable invocation")
		}
	}
	if _, ok := OwnedESLintInvocation("repository.js"); ok {
		t.Fatal("accepted arbitrary source")
	}
}

func TestESLintInvalidOutput(t *testing.T) {
	base := string(eslintJSON(true))
	cases := map[string]string{
		"empty": "", "null": "null", "no file": "[]", "two files": "[" + base[1:len(base)-1] + "," + base[1:len(base)-1] + "]",
		"trailing": base + ` {}`, "truncated": base[:len(base)-1], "banner": "banner\n" + base,
		"duplicate":         strings.Replace(base, `"errorCount":1`, `"errorCount":0,"errorCount":1`, 1),
		"escaped duplicate": strings.Replace(base, `"errorCount":1`, `"errorCount":0,"\u0065rrorCount":1`, 1),
		"unknown":           strings.Replace(base, `"errorCount":1`, `"extra":0,"errorCount":1`, 1),
		"case alias":        strings.Replace(base, `"errorCount"`, `"ErrorCount"`, 1),
		"missing count":     strings.Replace(base, `"errorCount":1,`, ``, 1),
		"null messages":     strings.Replace(base, `"messages":[`, `"messages":null,"unused":[`, 1),
		"null field":        strings.Replace(base, `"fatalErrorCount":0`, `"fatalErrorCount":null`, 1),
		"wrong path":        strings.Replace(base, `/work/fixture.js`, `/target/fixture.js`, 1),
		"count mismatch":    strings.Replace(base, `"errorCount":1`, `"errorCount":0`, 1),
		"fraction":          strings.Replace(base, `"errorCount":1`, `"errorCount":1.5`, 1),
		"negative":          strings.Replace(base, `"warningCount":0`, `"warningCount":-1`, 1),
		"fix":               strings.Replace(base, `"fixableErrorCount":0`, `"fixableErrorCount":1`, 1),
		"source mismatch":   strings.Replace(base, `"usedDeprecatedRules":[]`, `"source":"other"`, 1),
		"suppressed":        strings.Replace(base, `"suppressedMessages":[]`, `"suppressedMessages":[{}]`, 1),
		"deprecated":        strings.Replace(base, `"usedDeprecatedRules":[]`, `"usedDeprecatedRules":[{}]`, 1),
		"output fix":        strings.Replace(base, `"usedDeprecatedRules":[]`, `"output":"changed"`, 1),
		"missing rule":      strings.Replace(base, `"ruleId":"no-debugger",`, ``, 1),
		"missing column":    strings.Replace(base, `"column":1,`, ``, 1),
		"bad severity":      strings.Replace(base, `"severity":2`, `"severity":3`, 1),
		"bad position":      strings.Replace(base, `"column":1`, `"column":0`, 1),
		"deep":              strings.Repeat("[", 20) + strings.Repeat("]", 20),
		"oversized":         strings.Repeat(" ", 65537),
		"invalid utf8":      base + string([]byte{0xff}),
		"nested duplicate":  strings.Replace(base, `"severity":2`, `"severity":1,"severity":2`, 1),
		"wrong count type":  strings.Replace(base, `"errorCount":1`, `"errorCount":"1"`, 1),
		"missing messages":  strings.Replace(base, `"messages":`, `"unknown":`, 1),
		"null array":        strings.Replace(base, `"suppressedMessages":[]`, `"suppressedMessages":null`, 1),
		"missing end":       strings.Replace(base, `,"endColumn":10`, ``, 1),
		"backward end":      strings.Replace(base, `"endLine":1`, `"endLine":0`, 1),
		"zero end column":   strings.Replace(base, `"endColumn":10`, `"endColumn":0`, 1),
		"unknown message":   strings.Replace(base, `"messageId":"unexpected"`, `"fix":{}`, 1),
		"invalid fatal":     strings.Replace(base, `"severity":2`, `"fatal":true,"severity":1`, 1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			o := DecodeESLint(Fault, observations(t)[0].Identity, ESLintCapture{Status: Completed, ExitCode: 1, Stdout: []byte(input)})
			if o.Status != InvalidOutput || len(o.Diagnostics) != 0 {
				t.Fatalf("accepted %s: %+v", name, o)
			}
		})
	}
}

func TestESLintBoundsAndDiagnosticKinds(t *testing.T) {
	id := observations(t)[0].Identity
	base := eslintJSON(true)
	atLimit := append(bytes.Clone(base), bytes.Repeat([]byte(" "), eslintOutputLimit-len(base))...)
	if DecodeESLint(Fault, id, ESLintCapture{Status: Completed, ExitCode: 1, Stdout: atLimit}).Status != Completed {
		t.Fatal("rejected exact byte limit")
	}
	if DecodeESLint(Fault, id, ESLintCapture{Status: Completed, ExitCode: 1, Stdout: append(atLimit, ' ')}).Status != InvalidOutput {
		t.Fatal("accepted byte overrun")
	}
	for name, body := range map[string]string{
		"warning": strings.ReplaceAll(strings.ReplaceAll(strings.Replace(string(base), `"severity":2`, `"severity":1`, 1), `"errorCount":1`, `"errorCount":0`), `"warningCount":0`, `"warningCount":1`),
		"fatal":   strings.Replace(strings.Replace(string(base), `"severity":2`, `"fatal":true,"severity":2`, 1), `"fatalErrorCount":0`, `"fatalErrorCount":1`, 1),
	} {
		exit := 1
		if name == "warning" {
			exit = 0
		}
		o := DecodeESLint(Fault, id, ESLintCapture{Status: Completed, ExitCode: exit, Stdout: []byte(body)})
		triplet := observations(t)
		triplet[1] = o
		if o.Status != Completed || Classify(triplet).Outcome != Unverifiable {
			t.Fatalf("unexpected %s: %+v", name, o)
		}
	}
	var files []map[string]any
	if err := json.Unmarshal(base, &files); err != nil {
		t.Fatal(err)
	}
	message := files[0]["messages"].([]any)[0]
	for _, count := range []int{2, 17} {
		messages := make([]any, count)
		for i := range messages {
			messages[i] = message
		}
		files[0]["messages"], files[0]["errorCount"] = messages, count
		body, _ := json.Marshal(files)
		o := DecodeESLint(Fault, id, ESLintCapture{Status: Completed, ExitCode: 1, Stdout: body})
		if count == 17 && o.Status != InvalidOutput {
			t.Fatal("accepted diagnostic overrun")
		}
		triplet := observations(t)
		triplet[1] = o
		if Classify(triplet).Outcome != Unverifiable {
			t.Fatal("extra diagnostic became sensitive")
		}
	}
}

func FuzzDecodeESLint(f *testing.F) {
	f.Add(eslintJSON(true), 1)
	f.Add(eslintJSON(false), 0)
	f.Add([]byte(`[{"messages":null}]`), 1)
	f.Fuzz(func(t *testing.T, body []byte, exit int) {
		id := observations(t)[0].Identity
		o := DecodeESLint(Fault, id, ESLintCapture{Status: Completed, ExitCode: exit, Stdout: body})
		if o.Status == Completed && (len(body) > eslintOutputLimit || !json.Valid(body) || (exit != 0 && exit != 1)) {
			t.Fatal("accepted invalid capture")
		}
		if o.Status != Completed && len(o.Diagnostics) != 0 {
			t.Fatal("leaked partial diagnostics")
		}
	})
}

func TestESLintCaptureFailures(t *testing.T) {
	id := observations(t)[0].Identity
	for _, status := range []Status{Denied, Unavailable, TimedOut, Crashed, CleanupUnverified, InvalidOutput, "", "OTHER"} {
		got := DecodeESLint(Fault, id, ESLintCapture{Status: status, ExitCode: 1, Stdout: eslintJSON(true)})
		if got.Status == Completed || len(got.Diagnostics) != 0 {
			t.Fatalf("upgraded status: %+v", got)
		}
		if status != "" && status != "OTHER" && got.Status != status {
			t.Fatal("lost harness status")
		}
	}
	for _, capture := range []ESLintCapture{
		{Status: Completed, ExitCode: 2, Stdout: eslintJSON(true)},
		{Status: Completed, ExitCode: -1, Stdout: eslintJSON(true)},
		{Status: Completed, ExitCode: 0, Stdout: eslintJSON(true)},
		{Status: Completed, ExitCode: 1, Stdout: eslintJSON(false)},
		{Status: Completed, ExitCode: 1, Stdout: eslintJSON(true), Stderr: []byte("warning")},
		{Status: Completed, ExitCode: 1, Stdout: eslintJSON(true), Stderr: bytes.Repeat([]byte("x"), 65536)},
	} {
		if DecodeESLint(Fault, id, capture).Status == Completed {
			t.Fatal("accepted failed capture")
		}
	}
	if DecodeESLint("OTHER", id, ESLintCapture{Status: Completed}).Status != InvalidOutput {
		t.Fatal("accepted role")
	}
	id.Tool = "other"
	if DecodeESLint(Fault, id, ESLintCapture{Status: Completed, ExitCode: 1, Stdout: eslintJSON(true)}).Status != InvalidOutput {
		t.Fatal("accepted identity")
	}
}

func TestESLintSourceAndWrongRule(t *testing.T) {
	id := observations(t)[0].Identity
	f, _ := FixtureFor(Fault)
	source, _ := json.Marshal(f.Source)
	input := strings.Replace(string(eslintJSON(true)), `"usedDeprecatedRules":[]`, `"source":`+string(source), 1)
	if DecodeESLint(Fault, id, ESLintCapture{Status: Completed, ExitCode: 1, Stdout: []byte(input)}).Status != Completed {
		t.Fatal("rejected matching source")
	}
	input = strings.Replace(string(eslintJSON(true)), "no-debugger", "other-rule", 1)
	o := DecodeESLint(Fault, id, ESLintCapture{Status: Completed, ExitCode: 1, Stdout: []byte(input)})
	triplet := observations(t)
	triplet[1] = o
	if o.Status != Completed || Classify(triplet).Reason != "UNEXPECTED_DIAGNOSTIC" {
		t.Fatal("wrong rule accepted as sensitive")
	}
}
