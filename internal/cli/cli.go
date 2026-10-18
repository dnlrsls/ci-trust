// Package cli provides an injected command and rendering boundary.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode"

	"github.com/dnlrsls/ci-trust/internal/bootstrap"
	"github.com/dnlrsls/ci-trust/internal/evidence"
)

// Services keeps parsing independent of trusted report producers. Audit and
// Verify must redact known secrets before returning schema-v1 bytes. Verify is
// not approval or runtime authority. VerifyWorkflow accepts request data only;
// its bool is a just-produced report disposition, never admission or consent.
type Services struct {
	Bootstrap        func(string, []string) (bootstrap.Plan, error)
	Audit            func(string, []string) ([]byte, error)
	Verify           func(string) ([]byte, error)
	VerifyWorkflow   func(string, io.Reader) ([]byte, bool, error)
	Version          string
	InteractiveInput bool
}

const auditUsage = "ci-trust audit PATH [--format human|json] [--platform linux|windows|macos ...]\n"
const reportUsage = "ci-trust report [--input FILE|-] [--format human|json]\n"
const effects = "Offline/read-only: no execution, network, hidden storage, or writes.\nExit 0: valid report (including FAIL/UNKNOWN), help/version; 1: operational error; 2: invalid invocation.\n"
const bootstrapUsage = "ci-trust bootstrap PATH --dry-run [--format human|json] [--platform linux|windows|macos ...]\nDry-run only: no apply behavior; conditional drafts are not executed or enforced.\n"
const verifyUsage = "ci-trust verify PATH --check lint [--format human|json] [--workflow-request -]\nOwned ESLint 10.12.0 / Node 24.22.0: source-admitted owned fixture profile; ordinary host verification remains UNVERIFIABLE.\nTOOL_FIXTURE only: CLEAN 0 -> FAULT 1 (no-debugger error at 1:1) -> RESTORED 0; exact human consent and preflight required; project configs/plugins are never loaded.\n"
const workflowEffects = "Without --workflow-request: offline/read-only valid-report exit 0, including UNVERIFIABLE.\n--workflow-request -: bounded non-authorizing request on stdin; owned protected GitHub workflow only, never host fallback.\nWorkflow exit 0: just-produced owned sensitivity; 1: unavailable, insensitive, unverifiable or operational error; 2: invalid invocation.\n"
const usage = auditUsage + reportUsage + bootstrapUsage + verifyUsage + "ci-trust --help\nci-trust --version\n" + "Audit/report/bootstrap: " + effects + workflowEffects
const maxInput = 8 << 20

type invocation struct {
	command, path, input, format string
	platforms                    []string
	dryRun, lint, workflow       bool
}

func parse(args []string) (invocation, bool) {
	v := invocation{input: "-", format: "human"}
	if len(args) == 0 {
		return v, false
	}
	v.command = args[0]
	if v.command != "audit" && v.command != "report" && v.command != "bootstrap" && v.command != "verify" {
		return v, false
	}
	seen := map[string]bool{}
	platforms := map[string]bool{}
	positional := false
	for i := 1; i < len(args); i++ {
		a := args[i]
		if !positional && a == "--" && v.command != "report" {
			positional = true
			continue
		}
		if !positional && strings.HasPrefix(a, "-") {
			if a == "--dry-run" && v.command == "bootstrap" {
				if v.dryRun {
					return v, false
				}
				v.dryRun = true
				continue
			}
			if a != "--format" && !(a == "--platform" && (v.command == "audit" || v.command == "bootstrap")) && !(a == "--input" && v.command == "report") && !((a == "--check" || a == "--workflow-request") && v.command == "verify") {
				return v, false
			}
			if i+1 >= len(args) {
				return v, false
			}
			i++
			value := args[i]
			if value == "" {
				return v, false
			}
			if a != "--platform" && seen[a] {
				return v, false
			}
			seen[a] = true
			switch a {
			case "--workflow-request":
				if value != "-" {
					return v, false
				}
				v.workflow = true
			case "--check":
				if value != "lint" {
					return v, false
				}
				v.lint = true
			case "--format":
				if value != "human" && value != "json" {
					return v, false
				}
				v.format = value
			case "--input":
				v.input = value
			case "--platform":
				if (value != "linux" && value != "windows" && value != "macos") || platforms[value] {
					return v, false
				}
				platforms[value] = true
				v.platforms = append(v.platforms, value)
			}
		} else {
			if v.command == "report" || v.path != "" || a == "" {
				return v, false
			}
			v.path = a
		}
	}
	return v, (v.command == "report" || v.path != "") && (v.command != "bootstrap" || v.dryRun) && (v.command != "verify" || v.lint)
}

// Run validates the entire invocation before reading input or invoking a producer.
// Ordinary valid reports exit 0. Explicit workflow verification additionally
// requires the trusted producer's sensitivity disposition; unresolved exits 1.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer, services Services) int {
	fail := func(message string) int { fmt.Fprintln(stderr, "ci-trust: "+message); return 1 }
	write := func(data []byte) int {
		n, err := stdout.Write(data)
		if err != nil || n != len(data) {
			return fail("cannot write output")
		}
		return 0
	}
	if len(args) == 1 {
		switch args[0] {
		case "--help", "help":
			return write([]byte(usage))
		case "--version":
			return write([]byte("ci-trust " + strconv.Quote(services.Version) + "\n"))
		}
	}
	if len(args) == 2 {
		command := ""
		if args[0] == "help" {
			command = args[1]
		} else if args[1] == "--help" || args[1] == "help" {
			command = args[0]
		}
		switch command {
		case "audit":
			return write([]byte(auditUsage + effects))
		case "report":
			return write([]byte(reportUsage + effects))
		case "bootstrap":
			return write([]byte(bootstrapUsage + effects))
		case "verify":
			return write([]byte(verifyUsage + workflowEffects))
		}
	}
	v, ok := parse(args)
	if !ok {
		fmt.Fprint(stderr, "ci-trust: invalid arguments\n"+usage)
		return 2
	}
	if v.command == "bootstrap" {
		if services.Bootstrap == nil {
			return fail("bootstrap service unavailable")
		}
		plan, err := services.Bootstrap(v.path, append([]string(nil), v.platforms...))
		if err != nil {
			return fail("bootstrap failed")
		}
		output, records, err := renderPlan(plan, v.format)
		if err != nil {
			return fail("invalid bootstrap plan")
		}
		for _, record := range records {
			fmt.Fprintln(stderr, safe(record.Code)+": "+safe(record.Message))
		}
		return write(output)
	}
	var data []byte
	var err error
	workflowPassed := false
	if v.command == "verify" {
		if v.workflow {
			if services.VerifyWorkflow == nil {
				return fail("lint verification service unavailable")
			}
			data, workflowPassed, err = services.VerifyWorkflow(v.path, stdin)
		} else {
			if services.Verify == nil {
				return fail("lint verification service unavailable")
			}
			data, err = services.Verify(v.path)
		}
		if err != nil {
			return fail("lint verification failed")
		}
	} else if v.command == "audit" {
		if services.Audit == nil {
			return fail("audit service unavailable")
		}
		data, err = services.Audit(v.path, append([]string(nil), v.platforms...))
		if err != nil {
			return fail("audit failed")
		}
	} else {
		reader := stdin
		if v.input == "-" {
			if services.InteractiveInput {
				return fail("report input required (interactive stdin)")
			}
		} else {
			file, e := os.Open(v.input)
			if e != nil {
				return fail("cannot open report input")
			}
			defer file.Close()
			reader = file
		}
		if reader == nil {
			return fail("cannot read report input")
		}
		data, err = io.ReadAll(io.LimitReader(reader, maxInput+1))
		if err != nil {
			return fail("cannot read report input")
		}
	}
	if len(data) > maxInput {
		return fail("report exceeds 8 MiB")
	}
	canonical, diagnostics := evidence.Encode(data)
	if len(diagnostics) > 0 {
		return fail("invalid report")
	}
	var report map[string]json.RawMessage
	_ = json.Unmarshal(canonical, &report)
	var records []struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(report["diagnostics"], &records)
	var output []byte
	if v.format == "json" {
		output = canonical
	} else {
		output = human(report)
	}
	for _, record := range records {
		fmt.Fprintln(stderr, safe(record.Code)+": "+safe(record.Message))
	}
	if code := write(output); code != 0 {
		return code
	}
	if v.workflow && !workflowPassed {
		return 1
	}
	return 0
}

// safe escapes terminal controls, including C1 and bidi format characters.
func safe(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			if r <= 0xffff {
				fmt.Fprintf(&b, "\\u%04x", r)
			} else {
				r -= 0x10000
				fmt.Fprintf(&b, "\\u%04x\\u%04x", 0xd800+(r>>10), 0xdc00+(r&1023))
			}
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func human(report map[string]json.RawMessage) []byte {
	var b bytes.Buffer
	b.WriteString("CI Trust report (configuration is not execution or enforcement)\n")
	for _, key := range []string{"schema_version", "snapshot"} {
		fmt.Fprintf(&b, "%s: %s\n", key, safe(string(report[key])))
	}
	for _, key := range []string{"inventory", "capabilities", "workflows", "evidence", "findings", "diagnostics", "limitations"} {
		fmt.Fprintf(&b, "%s:\n", key)
		var values []json.RawMessage
		_ = json.Unmarshal(report[key], &values)
		if len(values) == 0 {
			b.WriteString("  (none)\n")
		}
		for _, value := range values {
			fmt.Fprintf(&b, "  %s\n", safe(string(value)))
		}
	}
	return b.Bytes()
}
