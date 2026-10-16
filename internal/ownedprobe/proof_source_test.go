package ownedprobe_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/ownedprobe"
)

type proofFake struct {
	*fake
	dynamicCalls int
	effects      ownedprobe.ProofObservations
}

func (f *proofFake) ObserveProof() ownedprobe.ProofObservations {
	f.dynamicCalls++
	return f.effects
}

func completeProof() *proofFake {
	c := ownedprobe.CounterObservation{Supported: true, Before: 3, After: 4}
	return &proofFake{fake: fresh(), effects: ownedprobe.ProofObservations{
		ScratchLimit: true, NetworkUnreachable: true, HostCredentialsAbsent: true,
		CPU: c, Memory: c, Pids: c,
	}}
}

func TestProofSourceSuccessfulWireAndLegacyBypass(t *testing.T) {
	for _, argv := range [][]string{proofArgs, {"--config=owned-proof-v1", "--fixture=smoke-v1"}, args, {"--help"}} {
		f := completeProof()
		before := clone(f.files)
		savedArgs := append([]string(nil), argv...)
		var out, errout bytes.Buffer
		code := ownedprobe.Run(argv, f, &out, &errout)
		wantOut, wantErr, wantCalls := success, "", 0
		if argv[0] == "--help" {
			wantOut = help
		} else if strings.Contains(strings.Join(argv, " "), "owned-proof-v1") {
			wantCalls = 1
			wantErr = "owned-smoke: owned stderr\n"
			wantOut = "owned-smoke: proof-started\n" + successfulProofJSON + "\n"
		}
		if code != 0 || out.String() != wantOut || errout.String() != wantErr || f.dynamicCalls != wantCalls {
			t.Fatalf("exit/stdout/stderr/calls = %d/%q/%q/%d", code, out.String(), errout.String(), f.dynamicCalls)
		}
		if !reflect.DeepEqual(argv, savedArgs) || !reflect.DeepEqual(before, f.files) {
			t.Fatal("arguments/inventory changed")
		}
	}
}

const successfulProofJSON = `{"version":1,"scope":"owned-smoke-v1","checks":[{"name":"nonroot","state":"OBSERVED","detail":"uid65534","before":null,"after":null},{"name":"capabilities","state":"OBSERVED","detail":"all capability sets zero","before":null,"after":null},{"name":"no_new_privileges","state":"OBSERVED","detail":"NoNewPrivs enabled","before":null,"after":null},{"name":"seccomp","state":"OBSERVED","detail":"filter mode observed","before":null,"after":null},{"name":"root_readonly","state":"OBSERVED","detail":"owned marker open rejected with EROFS","before":null,"after":null},{"name":"scratch_write_cleanup","state":"OBSERVED","detail":"exclusive owned scratch written and removed","before":null,"after":null},{"name":"scratch_limit","state":"OBSERVED","detail":"owned bounded write reached ENOSPC and cleanup completed","before":null,"after":null},{"name":"network_unreachable","state":"OBSERVED","detail":"no-send route lookup and local network checks agreed","before":null,"after":null},{"name":"host_credentials_absent","state":"OBSERVED","detail":"named credential keys and harmless host sentinels absent","before":null,"after":null},{"name":"cpu_limit","state":"OBSERVED","detail":"bounded demand increased cpu.stat throttling counter","before":3,"after":4},{"name":"memory_limit","state":"OBSERVED","detail":"bounded owned child wait and memory.events limit effect","before":3,"after":4},{"name":"pids_limit","state":"OBSERVED","detail":"bounded owned child wait and pids.events limit effect","before":3,"after":4}],"limitations":["Owned fixture observations only; no absolute escape or security guarantee.","Network checks send no packets; credential checks cover named keys and harmless sentinels, not all host credential locations.","Timeout, cancellation, combined output limits and container cleanup require independent runner evidence."]}`

func TestProofSourceGuardAndCleanupFailuresNeverInvokeSource(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*fake)
	}{
		{"host", func(f *fake) { f.os = "windows" }},
		{"status", func(f *fake) { f.files["/proc/self/status"] = "" }},
		{"cgroup", func(f *fake) { f.files["/proc/self/cgroup"] = "" }},
		{"mount", func(f *fake) { f.files["/proc/self/mounts"] = "" }},
		{"marker-not-erofs", func(f *fake) { f.writeErr = syscall.EACCES }},
		{"foreign-scratch", func(f *fake) { f.files[scratch] = "foreign" }},
		{"partial-create", func(f *fake) { f.createErr = errors.New("private raw error") }},
		{"cleanup", func(f *fake) { f.removeErr = errors.New("private raw error") }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := completeProof()
			tc.change(f.fake)
			var out, errout bytes.Buffer
			code := ownedprobe.Run(proofArgs, f, &out, &errout)
			if code != 1 || out.Len() != 0 || errout.String() != "owned-smoke: unavailable\n" || f.dynamicCalls != 0 {
				t.Fatalf("exit/stdout/stderr/dynamic = %d/%q/%q/%d", code, out.String(), errout.String(), f.dynamicCalls)
			}
			if tc.name == "foreign-scratch" && f.files[scratch] != "foreign" {
				t.Fatal("foreign scratch changed")
			}
			if tc.name == "partial-create" {
				if _, exists := f.files[scratch]; exists {
					t.Fatal("owned partial create leaked")
				}
			}
		})
	}
}

func TestProofEveryMissingEffectFailsClosed(t *testing.T) {
	for _, name := range []string{"scratch_limit", "network_unreachable", "host_credentials_absent", "cpu_limit", "memory_limit", "pids_limit", "cpu_reset", "cpu_unchanged"} {
		t.Run(name, func(t *testing.T) {
			f := completeProof()
			switch name {
			case "scratch_limit":
				f.effects.ScratchLimit = false
			case "network_unreachable":
				f.effects.NetworkUnreachable = false
			case "host_credentials_absent":
				f.effects.HostCredentialsAbsent = false
			case "cpu_limit":
				f.effects.CPU.Supported = false
			case "memory_limit":
				f.effects.Memory.Supported = false
			case "pids_limit":
				f.effects.Pids.Supported = false
			case "cpu_reset":
				f.effects.CPU.After = 2
			case "cpu_unchanged":
				f.effects.CPU.After = 3
			}
			var out, errout bytes.Buffer
			code := ownedprobe.Run(proofArgs, f, &out, &errout)
			if code != 1 || errout.String() != "owned-smoke: owned stderr\nowned-smoke: unavailable\n" || f.dynamicCalls != 1 {
				t.Fatalf("exit/streams/calls = %d/%q/%q/%d", code, out.String(), errout.String(), f.dynamicCalls)
			}
			var report struct {
				Checks []struct {
					Name, State   string
					Before, After *uint64
				}
			}
			if err := json.Unmarshal([]byte(strings.Split(out.String(), "\n")[1]), &report); err != nil {
				t.Fatal(err)
			}
			wanted := name
			if strings.HasPrefix(name, "cpu_") {
				wanted = "cpu_limit"
			}
			found := false
			for _, c := range report.Checks {
				if c.Name == wanted {
					found = true
					if c.State != "UNVERIFIABLE" || c.Before != nil || c.After != nil {
						t.Fatalf("unsupported check = %+v", c)
					}
				}
			}
			if !found {
				t.Fatal("missing check")
			}
		})
	}
}

func TestProofArgumentsRemainFixed(t *testing.T) {
	for _, argv := range [][]string{
		{"--fixture=other", "--config=owned-proof-v1"},
		{"--fixture=smoke-v1", "--config=owned-proof-v1", "--child=memory"},
		{"--child=memory"},
		{"--fixture=smoke-v1", "--config=owned-proof-v1", "target"},
	} {
		f := completeProof()
		before := clone(f.files)
		var out, errout bytes.Buffer
		if code := ownedprobe.Run(argv, f, &out, &errout); code != 2 || out.Len() != 0 || errout.String() != "owned-smoke: invalid arguments\n" || len(f.calls) != 0 || f.dynamicCalls != 0 || !reflect.DeepEqual(before, f.files) {
			t.Fatalf("invalid invocation affected system: %d/%q/%q/%v", code, out.String(), errout.String(), f.calls)
		}
	}
}
