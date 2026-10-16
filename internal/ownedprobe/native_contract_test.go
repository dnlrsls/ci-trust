package ownedprobe

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestChildArgumentsAndEnvironmentAreFixed(t *testing.T) {
	if !reflect.DeepEqual(childArguments(), []string{"--fixture=smoke-v1", "--config=owned-proof-v1"}) {
		t.Fatal("child arguments widened")
	}
	for _, mode := range []string{"memory", "pids"} {
		want := []string{childKey + "=" + mode, "GOMAXPROCS=1", "GOMEMLIMIT=off", "GOTRACEBACK=none"}
		if !reflect.DeepEqual(childEnvironment(mode), want) {
			t.Fatal("child environment inherited or widened")
		}
	}
	if childEnvironment("other") != nil {
		t.Fatal("arbitrary child mode")
	}
}

func TestNetworkLocalViewsRejectRoutesAndInterfacesBeforeConnect(t *testing.T) {
	for _, name := range []string{"interface", "route", "ipv6-next-hop", "unreadable", "oversize"} {
		t.Run(name, func(t *testing.T) {
			s, m, _, calls := dynamicAdapter()
			switch name {
			case "interface":
				m.files["/proc/net/dev"].data = append(m.files["/proc/net/dev"].data, []byte("eth0: 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n")...)
			case "route":
				m.files["/proc/net/route"].data = append(m.files["/proc/net/route"].data, []byte("eth0 00000000 01010101 0003\n")...)
			case "ipv6-next-hop":
				m.files["/proc/net/ipv6_route"].data = []byte(strings.Repeat("0", 32) + " 00 " + strings.Repeat("0", 32) + " 00 " + strings.Repeat("0", 31) + "1 00000000 00000000 00000000 00000000 lo\n")
			case "unreadable":
				delete(m.files, "/proc/net/dev")
			case "oversize":
				m.files["/proc/net/dev"].data = []byte(strings.Repeat("x", 16385))
			}
			if s.networkAbsent() {
				t.Fatal("network view accepted")
			}
			if len(*calls) != 0 {
				t.Fatalf("socket operation before local checks: %v", *calls)
			}
		})
	}
}

func TestChildEffectsRequireSpecificCounterDeltaAndWait(t *testing.T) {
	for _, name := range []string{"memory-max", "memory-oom", "memory-reset", "pids-max", "pids-unchanged"} {
		t.Run(name, func(t *testing.T) {
			s, m, _, _ := dynamicAdapter()
			path, key, alt, mode := "/sys/fs/cgroup/memory.events", "max", "oom_kill", "memory"
			if strings.HasPrefix(name, "pids") {
				path, key, alt, mode = "/sys/fs/cgroup/pids.events", "max", "", "pids"
			}
			s.proof.child = func(gotMode string) childResult {
				if gotMode != mode {
					t.Fatalf("mode = %q", gotMode)
				}
				switch name {
				case "memory-max":
					m.files[path].data = []byte("max 4\noom_kill 1\n")
				case "memory-oom":
					m.files[path].data = []byte("max 3\noom_kill 2\n")
				case "memory-reset":
					m.files[path].data = []byte("max 2\noom_kill 0\n")
				case "pids-max":
					m.files[path].data = []byte("max 3\n")
				}
				return childResult{true, true, true, false}
			}
			got := s.childEffect(mode, path, key, alt)
			wantSupported := name != "memory-reset" && name != "pids-unchanged"
			if got.Supported != wantSupported || (wantSupported && got.After <= got.Before) {
				t.Fatalf("effect = %+v", got)
			}
		})
	}
}

func TestChildGuardChangeBeforeWorkAndBeforeSpawn(t *testing.T) {
	s, m, _, calls := dynamicAdapter()
	s.proof.lookup = func(key string) (string, bool) {
		m.files["/sys/fs/cgroup/pids.max"].data = []byte("max\n")
		return "memory", true
	}
	if got := s.ObserveProof(); got != (ProofObservations{}) || len(*calls) != 0 {
		t.Fatal("changed child controls bypassed")
	}
	s, m, _, calls = dynamicAdapter()
	// Changing controls during the event read must prevent the subsequent spawn.
	original := s.ops.open
	s.ops.open = func(path string, flags int, mode os.FileMode) (file, error) {
		f, err := original(path, flags, mode)
		if path == "/sys/fs/cgroup/memory.events" {
			m.files["/sys/fs/cgroup/pids.max"].data = []byte("max\n")
		}
		return f, err
	}
	if got := s.childEffect("memory", "/sys/fs/cgroup/memory.events", "max", "oom_kill"); got.Supported || len(*calls) != 0 {
		t.Fatal("spawn after changed controls")
	}
}
