package ownedprobe

import (
	"context"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

type quotaFile struct {
	*memoryFile
	budget, attempted, largest int
	failure                    error
}

func (f *quotaFile) Write(p []byte) (int, error) {
	f.attempted += len(p)
	if len(p) > f.largest {
		f.largest = len(p)
	}
	if f.failure != nil {
		return 0, f.failure
	}
	if len(p) > f.budget {
		n := f.budget
		f.budget = 0
		return n, syscall.ENOSPC
	}
	f.budget -= len(p)
	return len(p), nil
}

func dynamicAdapter() (*nativeSystem, *memoryOps, *quotaFile, *[]string) {
	s, m := proofAdapter()
	q := &quotaFile{memoryFile: &memoryFile{}, budget: 8 * 1024 * 1024}
	open := s.ops.open
	s.ops.open = func(path string, flags int, mode os.FileMode) (file, error) {
		if path == limitScratchPath {
			if flags != os.O_WRONLY|os.O_CREATE|os.O_EXCL || mode != 0600 {
				return nil, unavailable
			}
			if m.files[path] != nil {
				return nil, syscall.EEXIST
			}
			m.files[path] = q.memoryFile
			return q, nil
		}
		return open(path, flags, mode)
	}
	for path, text := range map[string]string{
		"/proc/net/dev":                   "Inter-| Receive | Transmit\n face |bytes packets errs drop fifo frame compressed multicast|bytes packets errs drop fifo colls carrier compressed\n lo: 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n",
		"/proc/net/route":                 "Iface\tDestination\tGateway\tFlags\tRefCnt\tUse\tMetric\tMask\tMTU\tWindow\tIRTT\n",
		"/proc/net/ipv6_route":            "",
		"/sys/fs/cgroup/cpu.stat":         "nr_throttled 7\n",
		"/sys/fs/cgroup/memory.events":    "max 3\noom_kill 1\n",
		"/sys/fs/cgroup/memory.oom.group": "0\n",
		"/sys/fs/cgroup/pids.events":      "max 2\n",
	} {
		m.files[path] = &memoryFile{data: []byte(text)}
	}
	calls := []string{}
	s.proof = nativeProofOps{
		lookup:  func(key string) (string, bool) { calls = append(calls, "lookup:"+key); return "", false },
		connect: func() error { calls = append(calls, "udp-connect"); return syscall.ENETUNREACH },
		cpu: func() {
			calls = append(calls, "cpu")
			m.files["/sys/fs/cgroup/cpu.stat"].data = []byte("nr_throttled 8\n")
		},
		child: func(mode string) childResult {
			calls = append(calls, "child:"+mode)
			if mode == "memory" {
				m.files["/sys/fs/cgroup/memory.events"].data = []byte("max 4\noom_kill 2\n")
			} else {
				m.files["/sys/fs/cgroup/pids.events"].data = []byte("max 3\n")
			}
			return childResult{started: true, waited: true, failed: true}
		},
		work: func(mode string) { calls = append(calls, "work:"+mode) },
	}
	return s, m, q, &calls
}

func TestNativeObservationsInertSuccess(t *testing.T) {
	s, m, q, calls := dynamicAdapter()
	got := s.ObserveProof()
	want := ProofObservations{ScratchLimit: true, NetworkUnreachable: true, HostCredentialsAbsent: true, CPU: CounterObservation{true, 7, 8}, Memory: CounterObservation{true, 3, 4}, Pids: CounterObservation{true, 2, 3}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observations = %+v", got)
	}
	expected := []string{"lookup:" + childKey, "udp-connect", "lookup:CI_TRUST_HOST_CREDENTIAL_SENTINEL", "lookup:GITHUB_TOKEN", "lookup:GH_TOKEN", "lookup:AWS_ACCESS_KEY_ID", "lookup:AWS_SECRET_ACCESS_KEY", "lookup:AWS_SESSION_TOKEN", "lookup:SSH_AUTH_SOCK", "lookup:DOCKER_AUTH_CONFIG", "lookup:GOOGLE_APPLICATION_CREDENTIALS", "lookup:AZURE_CLIENT_SECRET", "cpu", "child:memory", "child:pids"}
	if !reflect.DeepEqual(*calls, expected) {
		t.Fatalf("dynamic operations = %v", *calls)
	}
	if q.attempted != 8*1024*1024+1 || q.largest > 32*1024 || q.closes != 1 {
		t.Fatalf("scratch attempts/chunk/closes = %d/%d/%d", q.attempted, q.largest, q.closes)
	}
	if m.files[limitScratchPath] != nil || m.files[scratchPath] != nil {
		t.Fatal("owned file leaked")
	}
}

func TestNativeFullGuardsPreventAllDynamicOperations(t *testing.T) {
	for _, path := range []string{"os", "uid", "/proc/self/status", "/proc/self/cgroup", "/proc/self/mounts", "/sys/fs/cgroup/cpu.max", "/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory.swap.max", "/sys/fs/cgroup/pids.max", markerPath} {
		t.Run(path, func(t *testing.T) {
			s, m, q, calls := dynamicAdapter()
			switch path {
			case "os":
				s.osName = "other"
			case "uid":
				s.uid = func() int { return 0 }
			default:
				m.files[path].data = []byte("invalid")
			}
			if got := s.ObserveProof(); got != (ProofObservations{}) || len(*calls) != 0 || q.attempted != 0 {
				t.Fatalf("unguarded effect: %+v calls=%v writes=%d", got, *calls, q.attempted)
			}
		})
	}
}

func TestNativeChildModesAreFixedAndEquallyGuarded(t *testing.T) {
	for _, mode := range []string{"memory", "pids", "arbitrary"} {
		for _, safe := range []bool{true, false} {
			s, m, q, calls := dynamicAdapter()
			s.proof.lookup = func(key string) (string, bool) {
				if key == childKey {
					return mode, true
				}
				t.Fatal("unexpected env lookup")
				return "", false
			}
			if !safe {
				m.files["/sys/fs/cgroup/pids.max"].data = []byte("max\n")
			}
			if got := s.ObserveProof(); got != (ProofObservations{}) {
				t.Fatalf("child manufactured evidence: %+v", got)
			}
			count := 0
			if safe && mode != "arbitrary" {
				count = 1
			}
			if len(*calls) != count || q.attempted != 0 {
				t.Fatalf("child mode %q safe %v operations %v", mode, safe, *calls)
			}
		}
	}
}

func TestNativeScratchCleanupAndForeignIdentity(t *testing.T) {
	for _, name := range []string{"write-error", "short-write", "close-error", "foreign", "foreign-replacement", "unlink-error", "guard-change"} {
		t.Run(name, func(t *testing.T) {
			s, m, q, _ := dynamicAdapter()
			remove := s.ops.remove
			switch name {
			case "write-error":
				q.failure = io.ErrUnexpectedEOF
			case "short-write":
				q.budget = 16
				q.failure = io.ErrShortWrite
			case "close-error":
				q.closeErr = errors.New("private")
			case "foreign":
				m.files[limitScratchPath] = &memoryFile{data: []byte("foreign")}
			case "foreign-replacement":
				s.ops.same = func(a, b os.FileInfo) bool {
					if a == q.memoryFile {
						return false
					}
					return a == b
				}
			case "unlink-error":
				s.ops.remove = func(path string) error {
					if path == limitScratchPath {
						return errors.New("private")
					}
					return remove(path)
				}
			case "guard-change":
				open := s.ops.open
				s.ops.open = func(path string, flags int, mode os.FileMode) (file, error) {
					f, err := open(path, flags, mode)
					if path == limitScratchPath {
						m.files["/sys/fs/cgroup/pids.max"].data = []byte("max\n")
					}
					return f, err
				}
			}
			if s.scratchLimit() {
				t.Fatal("failure was reported observed")
			}
			retained := name == "foreign" || name == "foreign-replacement" || name == "unlink-error"
			if (m.files[limitScratchPath] != nil) != retained {
				t.Fatalf("cleanup ownership mismatch for %s", name)
			}
			if name != "foreign" && q.closes != 1 {
				t.Fatalf("close count %d", q.closes)
			}
		})
	}
}

func TestNativeUnsupportedStopsSequence(t *testing.T) {
	for _, name := range []string{"network", "sentinel-env", "sentinel-path", "cpu-missing", "cpu-unchanged", "child-start", "child-wait", "child-timeout", "child-exit-zero", "child-counter-unchanged"} {
		t.Run(name, func(t *testing.T) {
			s, m, _, calls := dynamicAdapter()
			switch name {
			case "network":
				s.proof.connect = func() error { return nil }
			case "sentinel-env":
				s.proof.lookup = func(key string) (string, bool) { return "never print", key == "CI_TRUST_HOST_CREDENTIAL_SENTINEL" }
			case "sentinel-path":
				m.files[hostSentinelPath] = &memoryFile{}
			case "cpu-missing":
				m.files["/sys/fs/cgroup/cpu.stat"].data = []byte("usage_usec 9\n")
			case "cpu-unchanged":
				s.proof.cpu = func() {}
			default:
				s.proof.child = func(mode string) childResult {
					*calls = append(*calls, "child:"+mode)
					switch name {
					case "child-start":
						return childResult{}
					case "child-wait":
						return childResult{started: true}
					case "child-timeout":
						return childResult{true, true, true, true}
					case "child-exit-zero":
						return childResult{true, true, false, false}
					default:
						return childResult{true, true, true, false}
					}
				}
			}
			got := s.ObserveProof()
			if got.Pids.Supported || got.Memory.Supported {
				t.Fatalf("unsupported source succeeded: %+v", got)
			}
			for _, call := range *calls {
				if call == "child:pids" {
					t.Fatal("continued after unavailable control")
				}
			}
		})
	}
}

func TestNativeProofReadAllowlistAndCounterParsing(t *testing.T) {
	s, m, _, _ := dynamicAdapter()
	before := len(m.calls)
	if _, err := s.proofRead("/proc/not-allowed"); err == nil || len(m.calls) != before {
		t.Fatal("unknown read invoked operations")
	}
	if _, err := s.Read("/sys/fs/cgroup/cpu.stat", 4096); err == nil {
		t.Fatal("legacy read widened")
	}
	for _, text := range []string{"", "nr_throttled 1\nnr_throttled 2\n", "nr_throttled -1\n", "nr_throttled 18446744073709551616\n", "nr_throttled 1 extra\n"} {
		if _, ok := counter([]byte(text), "nr_throttled"); ok {
			t.Fatalf("accepted malformed counter %q", text)
		}
	}
	m.files["/sys/fs/cgroup/cpu.stat"].data = []byte(strings.Repeat("x", 4097))
	if _, err := s.proofRead("/sys/fs/cgroup/cpu.stat"); err == nil {
		t.Fatal("unbounded read")
	}
}

type inertChild struct {
	startErr, errorWait  error
	starts, waits, kills int
	release              chan struct{}
}

func (p *inertChild) Start() error { p.starts++; return p.startErr }
func (p *inertChild) Wait() error {
	p.waits++
	if p.release != nil {
		<-p.release
	}
	return p.errorWait
}
func (p *inertChild) Kill() error { p.kills++; close(p.release); return nil }
func TestOwnedChildWaitAndCancellationLifetime(t *testing.T) {
	for _, name := range []string{"normal", "failed-start", "failed-exit", "cancel"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := &inertChild{}
			switch name {
			case "failed-start":
				p.startErr = errors.New("start")
			case "failed-exit":
				p.errorWait = errors.New("exit")
			case "cancel":
				p.release = make(chan struct{})
				p.errorWait = errors.New("killed")
				cancel()
			}
			got := waitOwned(ctx, p)
			wantWait, wantKill := 1, 0
			if name == "failed-start" {
				wantWait = 0
			}
			if name == "cancel" {
				wantKill = 1
			}
			if p.starts != 1 || p.waits != wantWait || p.kills != wantKill || got.timedOut != (name == "cancel") || got.waited != (wantWait == 1) {
				t.Fatalf("lifetime result %+v calls %+v", got, p)
			}
		})
	}
}
