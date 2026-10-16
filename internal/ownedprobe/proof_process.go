package ownedprobe

import "context"

func childArguments() []string { return []string{"--fixture=smoke-v1", "--config=owned-proof-v1"} }
func childEnvironment(mode string) []string {
	if mode != "memory" && mode != "pids" {
		return nil
	}
	return []string{childKey + "=" + mode, "GOMAXPROCS=1", "GOMEMLIMIT=off", "GOTRACEBACK=none"}
}

// discardWriter holds no output and imposes no child-provided allocation.
// exec drains to it until the fixed deadline; WaitDelay bounds pipe closure.
type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }

// childProcess is a lifecycle seam, not a general process execution API.
type childProcess interface {
	Start() error
	Wait() error
	Kill() error
}

// waitOwned always waits once after successful Start, including cancellation.
// Context is private and fixed in production; tests use only inert processes.
func waitOwned(ctx context.Context, p childProcess) childResult {
	if err := p.Start(); err != nil {
		return childResult{}
	}
	result := childResult{started: true}
	done := make(chan error, 1)
	go func() { done <- p.Wait() }()
	select {
	case err := <-done:
		result.waited = true
		result.failed = err != nil
	case <-ctx.Done():
		result.timedOut = true
		_ = p.Kill()
		err := <-done
		result.waited = true
		result.failed = err != nil
	}
	return result
}
