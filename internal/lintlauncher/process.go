package lintlauncher

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

// Leave ample space for the fixed receipt header within the 64 KiB wire cap.
const toolOutputLimit = 60 << 10

var captureUnavailable = errors.New("owned tool capture incomplete")

func captureTool(ctx context.Context, cmd *exec.Cmd, input string, cancel context.CancelFunc, drain time.Duration) (lintsensitivity.ESLintCapture, error) {
	failed := func() (lintsensitivity.ESLintCapture, error) {
		return lintsensitivity.ESLintCapture{}, captureUnavailable
	}
	if ctx.Err() != nil || cmd == nil || len(input) > 64<<10 {
		return failed()
	}
	var pipes [6]*os.File
	defer func() {
		for _, p := range pipes {
			if p != nil {
				_ = p.Close()
			}
		}
	}()
	for i := 0; i < len(pipes); i += 2 {
		r, w, err := os.Pipe()
		if err != nil {
			return failed()
		}
		pipes[i], pipes[i+1] = r, w
	}
	stdinR, stdinW, stdoutR, stdoutW, stderrR, stderrW := pipes[0], pipes[1], pipes[2], pipes[3], pipes[4], pipes[5]
	// Real *os.File endpoints avoid exec's implicit copy goroutines and error
	// prioritization. We own and verify all three I/O completions independently.
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdinR, stdoutW, stderrW
	if err := cmd.Start(); err != nil {
		return failed()
	}
	for _, p := range []*os.File{stdinR, stdoutW, stderrW} {
		if err := p.Close(); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return failed()
		}
	}
	b := &toolOutput{cancel: cancel}
	done := make(chan error, 3)
	go func() {
		n, err := io.WriteString(stdinW, input)
		closeErr := stdinW.Close()
		if err == nil {
			err = closeErr
		}
		if n != len(input) {
			err = io.ErrShortWrite
		}
		done <- err
	}()
	go func() { _, err := io.Copy(toolStream{b, false}, stdoutR); done <- err }()
	go func() { _, err := io.Copy(toolStream{b, true}, stderrR); done <- err }()
	waitErr := cmd.Wait()
	// A nonzero child exit must not mask missing EOF, write errors or held pipes.
	// Bound the drain after every exit, not only successful process exits.
	if !completeIO(ctx, done, drain, func() { _ = stdinW.Close(); _ = stdoutR.Close(); _ = stderrR.Close() }) {
		return failed()
	}
	if ctx.Err() != nil || b.overflow {
		return failed()
	}
	exit := 0
	if waitErr != nil {
		var status *exec.ExitError
		if !errors.As(waitErr, &status) || status.ExitCode() != 1 {
			return failed()
		}
		exit = 1
	}
	// This asserts local tool capture completion only. Outer trusted execution,
	// cleanup and source preservation are still required by the receipt consumer.
	return lintsensitivity.ESLintCapture{Status: lintsensitivity.Completed, ExitCode: exit, Stdout: b.stdout, Stderr: b.stderr}, nil
}

func completeIO(ctx context.Context, done <-chan error, drain time.Duration, closePipes func()) bool {
	timer := time.NewTimer(drain)
	defer timer.Stop()
	ok := true
	for remaining := 3; remaining > 0; remaining-- {
		select {
		case err := <-done:
			if err != nil {
				ok = false
			}
		case <-ctx.Done():
			closePipes()
			for ; remaining > 0; remaining-- {
				<-done
			}
			return false
		case <-timer.C:
			closePipes()
			for ; remaining > 0; remaining-- {
				<-done
			}
			return false
		}
	}
	return ok
}

type toolOutput struct {
	sync.Mutex
	stdout, stderr []byte
	overflow       bool
	cancel         context.CancelFunc
}
type toolStream struct {
	output *toolOutput
	stderr bool
}

func (w toolStream) Write(p []byte) (int, error) {
	b := w.output
	b.Lock()
	defer b.Unlock()
	n := len(p)
	remaining := toolOutputLimit - len(b.stdout) - len(b.stderr)
	if n > remaining {
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
		return len(p), captureUnavailable
	}
	return n, nil
}
