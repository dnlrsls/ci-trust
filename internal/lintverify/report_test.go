package lintverify

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/discovery"
	"github.com/dnlrsls/ci-trust/internal/evidence"
	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

func decodeReport(t *testing.T, b []byte) map[string]any {
	t.Helper()
	if ds := evidence.Validate(b); len(ds) > 0 {
		t.Fatal(ds)
	}
	var r map[string]any
	if json.Unmarshal(b, &r) != nil {
		t.Fatal("invalid JSON")
	}
	return r
}
func TestScanClosedAndUnsafeConfigurationsAreReadOnly(t *testing.T) {
	for _, tc := range []struct {
		name, code       string
		declared, unsafe bool
	}{
		{"empty", "NODE_SECURITY_HOLD", false, false}, {"declared", "NODE_SECURITY_HOLD", true, false}, {"unsafe", "UNSUPPORTED_LINT_CONFIGURATION", true, true},
		{"plugin", "UNSUPPORTED_LINT_CONFIGURATION", true, true}, {"embedded-config", "UNSUPPORTED_LINT_CONFIGURATION", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			files := map[string][]byte{"sentinel": []byte("7")}
			if tc.declared {
				files["package.json"] = []byte(`{"scripts":{"lint":"eslint ."}}`)
			}
			if tc.unsafe {
				switch tc.name {
				case "plugin":
					files["package.json"] = []byte(`{"scripts":{"lint":"eslint ."},"devDependencies":{"eslint-plugin-example":"1.0.0"}}`)
				case "embedded-config":
					files["package.json"] = []byte(`{"scripts":{"lint":"eslint ."},"eslintConfig":{"extends":"untrusted"}}`)
				default:
					files["eslint.config.js"] = []byte(`throw new Error("must never execute");`)
				}
			}
			for name, data := range files {
				if err := os.WriteFile(filepath.Join(root, name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := Scan(root)
			if err != nil {
				t.Fatal(err)
			}
			r := decodeReport(t, got)
			found := false
			for _, v := range r["diagnostics"].([]any) {
				d := v.(map[string]any)
				if d["code"] == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing explicit gate diagnostic %s", tc.code)
			}
			for _, v := range r["evidence"].([]any) {
				if v.(map[string]any)["state"] == "EXECUTED" {
					t.Fatal("closed service invented execution")
				}
			}
			for _, v := range r["capabilities"].([]any) {
				c := v.(map[string]any)
				if c["dimension"] == "EFFICACY" && (c["status"] == "PRESENT" || c["status"] == "ABSENT") {
					t.Fatal("closed service invented efficacy")
				}
			}
			if !tc.declared && len(r["inventory"].([]any)) != 0 {
				t.Fatal("invented source-anchored CHECK")
			}
			repeated, err := Scan(root)
			if err != nil || !bytes.Equal(got, repeated) {
				t.Fatal("nondeterministic closed report", err)
			}
			for name, data := range files {
				after, err := os.ReadFile(filepath.Join(root, name))
				if err != nil || !bytes.Equal(data, after) {
					t.Fatal("source/counter changed", name, err)
				}
			}
		})
	}
	if b, err := Scan(filepath.Join(t.TempDir(), "absent")); err == nil || b != nil {
		t.Fatal("unavailable root became a successful report")
	}
}

func TestPrivateFixtureMappingNeverPromotesRepositoryEfficacy(t *testing.T) {
	s := discovery.Snapshot{Documents: []discovery.Document{{Path: "package.json", Content: []byte(`{"scripts":{"lint":"eslint ."}}`)}}}
	for _, tc := range []struct {
		mode, status string
		executed     int
	}{
		{"sensitive", "PRESENT", 3}, {"noop", "ABSENT", 3}, {"wrong", "UNVERIFIABLE", 0}, {"missing", "UNVERIFIABLE", 0},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			b, err := analyze(s, func(hash string) outcome {
				d, _, ctx := simulation(t, tc.mode)
				d.snapshot = func() (string, error) { return hash, nil }
				return evaluate(ctx, hash, d)
			})
			if err != nil {
				t.Fatal(err)
			}
			r := decodeReport(t, b)
			found := false
			executed := 0
			for _, v := range r["capabilities"].([]any) {
				c := v.(map[string]any)
				if c["dimension"] != "EFFICACY" {
					continue
				}
				scope := c["scope"].(map[string]any)
				if scope["kind"] == "TOOL_FIXTURE" {
					if c["status"] != tc.status {
						t.Fatal("wrong fixture decision", c)
					}
					found = true
				} else if c["status"] != "UNKNOWN" {
					t.Fatal("fixture upgraded project efficacy", c)
				}
			}
			for _, v := range r["evidence"].([]any) {
				e := v.(map[string]any)
				if e["state"] == "EXECUTED" {
					executed++
					if e["scope"].(map[string]any)["kind"] != "TOOL_FIXTURE" {
						t.Fatal("broadened execution scope")
					}
				}
			}
			if !found || executed != tc.executed {
				t.Fatalf("capability=%v executed=%d", found, executed)
			}
		})
	}
}

func TestNoAnchorOrUnsafeInputCannotPublishSuppliedTriplet(t *testing.T) {
	for _, s := range []discovery.Snapshot{{}, {Documents: []discovery.Document{{Path: "eslint.config.js", Content: []byte("throw 7;")}}}} {
		calls := 0
		b, err := analyze(s, func(string) outcome { calls++; return outcome{decision: ls.Result{Outcome: ls.Sensitive}} })
		if err != nil {
			t.Fatal(err)
		}
		r := decodeReport(t, b)
		for _, v := range r["evidence"].([]any) {
			if v.(map[string]any)["state"] == "EXECUTED" {
				t.Fatal("unbound observations became facts")
			}
		}
		if calls > 0 {
			t.Fatal("unanchored/unsupported inputs reached execution dependency")
		}
	}
}
