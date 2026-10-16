package runner

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

const transportLimit = 64 << 10

// splitStdin is opt-in and digest-bound. Only smoke with empty owned input is
// currently admitted; it is not a lint catalog entry or an execution approval.
const splitStdin = "split-stdin-v1"

type capturedRun struct {
	Stdout, Stderr string
	ExitCode       int
}

type capturedRuntime interface {
	startCaptured(context.Context, string, []string, string) (capturedRun, error)
}

type splitDockerClient interface {
	attach(context.Context, string, string, int, time.Duration) (string, string, error)
}

// attach never accepts a caller command vector or inherited stdin. The lifecycle
// supplies the independently inspected container ID and plan-bound resource caps.
func (c *executableDocker) attach(ctx context.Context, id, input string, max int, wall time.Duration) (string, string, error) {
	if !containerID.MatchString(id) {
		return "", "", invalidTransport
	}
	return c.capture(ctx, input, max, wall, "start", "--attach", "--interactive", id)
}

var invalidTransport = errors.New("invalid owned transport bounds")

// capture is private process plumbing, not runtime admission or a container
// attach protocol. Input must come from trusted owned composition, never target
// data. No production lint profile calls it until image/plan/consent admission
// and container input handling have been implemented and reviewed separately.
func (c *executableDocker) capture(ctx context.Context, input string, max int, wall time.Duration, args ...string) (string, string, error) {
	if len(input) > transportLimit || max < 1 || max > transportLimit || wall <= 0 || wall > time.Minute {
		return "", "", invalidTransport
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	ctx, cancel := context.WithTimeout(ctx, wall)
	defer cancel()
	b := &splitOutput{max: max, cancel: cancel}
	err := c.runProcess(ctx, strings.NewReader(input), streamOutput{b, false}, streamOutput{b, true}, args...)
	stdout, stderr, overflow := b.state()
	if overflow {
		return stdout, stderr, errors.New("combined tool output limit")
	}
	if ctx.Err() != nil {
		return stdout, stderr, ctx.Err()
	}
	// A client failure stays an error, even when a tool-like exit code is present.
	return stdout, stderr, err
}

// runProcess preserves the legacy exact client environment, fixed local socket
// vector and pipe wait bound. Callers own deadlines and stream budgets. Nil
// input is disconnected, not inherited; split capture uses an immutable reader.
func (c *executableDocker) runProcess(ctx context.Context, input io.Reader, stdout, stderr io.Writer, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	argv := append([]string{"--config", c.config, "--host", "unix:///var/run/docker.sock"}, args...)
	cmd := exec.CommandContext(ctx, c.path, argv...)
	cmd.Env = []string{"HOME=" + c.config, "DOCKER_CONFIG=" + c.config, "LANG=C", "LC_ALL=C"}
	cmd.WaitDelay = 200 * time.Millisecond
	cmd.Stdin, cmd.Stdout, cmd.Stderr = input, stdout, stderr
	return cmd.Run()
}

// splitOutput retains at most max bytes across both streams. The shared lock
// makes the budget and sticky overflow independent of concurrent pipe order.
type splitOutput struct {
	sync.Mutex
	stdout, stderr []byte
	max            int
	overflow       bool
	cancel         context.CancelFunc
}

type streamOutput struct {
	output *splitOutput
	stderr bool
}

func (w streamOutput) Write(p []byte) (int, error) {
	b := w.output
	b.Lock()
	defer b.Unlock()
	n := len(p)
	remaining := b.max - len(b.stdout) - len(b.stderr)
	if len(p) > remaining {
		p = p[:remaining]
		b.overflow = true
		b.cancel()
	}
	if w.stderr {
		b.stderr = append(b.stderr, p...)
	} else {
		b.stdout = append(b.stdout, p...)
	}
	if b.overflow {
		return len(p), errors.New("combined tool output limit")
	}
	return n, nil
}

func (b *splitOutput) state() (string, string, bool) {
	b.Lock()
	defer b.Unlock()
	return string(b.stdout), string(b.stderr), b.overflow
}
