package ownedprobe_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/ownedprobe"
)

var proofArgs = []string{"--fixture=smoke-v1", "--config=owned-proof-v1"}

func TestProofUnsupportedIsExplicit(t *testing.T) {
	f := fresh()
	before := clone(f.files)
	var out, errout bytes.Buffer
	code := ownedprobe.Run(proofArgs, f, &out, &errout)
	if code != 1 || errout.String() != "owned-smoke: owned stderr\nowned-smoke: unavailable\n" {
		t.Fatalf("exit/stdout/stderr = %d/%q/%q", code, out.String(), errout.String())
	}
	lines := strings.Split(out.String(), "\n")
	if len(lines) != 3 || lines[0] != "owned-smoke: proof-started" || lines[2] != "" {
		t.Fatalf("wire lines = %q", out.String())
	}
	var report struct {
		Version int
		Scope   string
		Checks  []struct {
			Name, State, Detail string
			Before, After       *uint64
		}
		Limitations []string
	}
	if err := json.Unmarshal([]byte(lines[1]), &report); err != nil {
		t.Fatal(err)
	}
	names := []string{"nonroot", "capabilities", "no_new_privileges", "seccomp", "root_readonly", "scratch_write_cleanup", "scratch_limit", "network_unreachable", "host_credentials_absent", "cpu_limit", "memory_limit", "pids_limit"}
	if report.Version != 1 || report.Scope != "owned-smoke-v1" || len(report.Checks) != len(names) || len(report.Limitations) == 0 {
		t.Fatalf("report = %+v", report)
	}
	for i, c := range report.Checks {
		want := "OBSERVED"
		if i >= 6 {
			want = "UNVERIFIABLE"
		}
		if c.Name != names[i] || c.State != want || c.Detail == "" || c.Before != nil || c.After != nil {
			t.Fatalf("check %d = %+v", i, c)
		}
	}
	if !reflect.DeepEqual(before, f.files) {
		t.Fatal("inventory changed")
	}
}

func TestProofRejectsGuardsBeforeDynamicOperations(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fake)
	}{
		{"os", func(f *fake) { f.os = "windows" }},
		{"uid", func(f *fake) { f.uid = 0 }},
		{"caps", func(f *fake) {
			f.files["/proc/self/status"] = strings.Replace(status, "CapEff:\t0000000000000000", "CapEff:\t0000000000000001", 1)
		}},
		{"no-new-privileges", func(f *fake) {
			f.files["/proc/self/status"] = strings.Replace(status, "NoNewPrivs:\t1", "NoNewPrivs:\t0", 1)
		}},
		{"seccomp", func(f *fake) { f.files["/proc/self/status"] = strings.Replace(status, "Seccomp:\t2", "Seccomp:\t0", 1) }},
		{"mount", func(f *fake) { f.files["/proc/self/mounts"] = "" }},
		{"cgroup", func(f *fake) { f.files["/sys/fs/cgroup/pids.max"] = "max\n" }},
		{"marker", func(f *fake) { f.files["/ci-trust/root-marker"] = "foreign\n" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := fresh()
			tc.change(f)
			before := clone(f.files)
			var out, errout bytes.Buffer
			code := ownedprobe.Run(proofArgs, f, &out, &errout)
			if code != 1 || out.Len() != 0 || errout.String() != "owned-smoke: unavailable\n" || !reflect.DeepEqual(before, f.files) {
				t.Fatalf("exit/stdout/stderr/inventory = %d/%q/%q/%v", code, out.String(), errout.String(), f.files)
			}
			for _, c := range f.calls {
				if c.op == "write" || c.op == "create" || c.op == "remove" {
					t.Fatalf("effect before guards: %+v", c)
				}
			}
		})
	}
}
