//go:build linux

package ownedprobe

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"
)

func realProofOps() nativeProofOps {
	return nativeProofOps{lookup: os.LookupEnv, connect: connectDocumentation, cpu: cpuDemand, child: ownChild, work: childWork}
}

// UDP connect performs a kernel route lookup only: there is deliberately no
// send/write/dial API here. Closing even an unsuccessful socket is mandatory.
func connectDocumentation() error {
	if !observe(NewSystem()) {
		return unavailable
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC|syscall.SOCK_NONBLOCK, 0)
	if err != nil {
		return unavailable
	}
	connectErr := syscall.Connect(fd, &syscall.SockaddrInet4{Port: 9, Addr: [4]byte{192, 0, 2, 1}})
	closeErr := syscall.Close(fd)
	if closeErr != nil {
		return unavailable
	}
	return connectErr
}

var cpuSink uint64

func cpuDemand() {
	if !observe(NewSystem()) {
		return
	}
	previous := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(previous)
	end := time.Now().Add(1800 * time.Millisecond)
	spin := func() uint64 {
		var n uint64 = 1
		for time.Now().Before(end) {
			for i := 0; i < 16384; i++ {
				n = n*6364136223846793005 + 1
			}
		}
		return n
	}
	other := make(chan uint64, 1)
	go func() { other <- spin() }()
	cpuSink = spin() + <-other
}

type execChild struct{ cmd *exec.Cmd }

func (c execChild) Start() error { return c.cmd.Start() }
func (c execChild) Wait() error  { return c.cmd.Wait() }
func (c execChild) Kill() error {
	if c.cmd.Process == nil {
		return unavailable
	}
	return c.cmd.Process.Kill()
}

func ownChild(mode string) childResult {
	if !observe(NewSystem()) {
		return childResult{}
	}
	if mode != "memory" && mode != "pids" {
		return childResult{}
	}
	executable, err := os.Executable()
	if err != nil || executable != "/ci-trust/owned-smoke" {
		return childResult{}
	}
	info, err := os.Lstat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0555 {
		return childResult{}
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != 0 {
		return childResult{}
	}
	// Do not force the parent to create control threads while the child tests the
	// cgroup ceiling. Reserve the current thread before starting it and use one P.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	previous := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previous)
	ctx, cancel := context.WithTimeout(context.Background(), 3500*time.Millisecond)
	defer cancel()
	cmd := exec.Command(executable, childArguments()...)
	cmd.Env = childEnvironment(mode)
	cmd.Stdin = nil
	cmd.Stdout = discardWriter{}
	cmd.Stderr = discardWriter{}
	cmd.WaitDelay = 200 * time.Millisecond
	// The fixed image owns this root-owned non-writable binary. Recheck identity
	// immediately before launch; guarded UID/capabilities cannot replace it.
	current, err := os.Lstat(executable)
	if err != nil || !os.SameFile(info, current) {
		return childResult{}
	}
	return waitOwned(ctx, execChild{cmd})
}

func childWork(mode string) {
	system := NewSystem().(*nativeSystem)
	if !observe(system) || (mode == "memory" && !system.memoryChildSafe()) {
		return
	}
	switch mode {
	case "memory":
		// One capped allocation, fault every page, retain until return. The 128 MiB
		// cgroup should kill or deny this child, not the independent parent.
		data := make([]byte, 160*1024*1024)
		for i := 0; i < len(data); i += 4096 {
			data[i] = 1
		}
		runtime.KeepAlive(data)
	case "pids":
		// Each goroutine locks one OS thread. A child runtime thread-creation
		// failure should end this process. The parent establishes its control
		// thread first; scheduler survival still requires independent evidence.
		stop := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < 80; i++ {
			ready := make(chan struct{})
			wg.Add(1)
			go func() { defer wg.Done(); runtime.LockOSThread(); defer runtime.UnlockOSThread(); close(ready); <-stop }()
			<-ready
		}
		close(stop)
		wg.Wait()
	}
}
