package runner

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"
)

type admissionClient struct {
	base    ownedClient
	calls   atomic.Int32
	entered chan struct{}
	release chan struct{}
}

func (f *admissionClient) call(ctx context.Context, w io.Writer, args ...string) (string, error) {
	if f.calls.Add(1) == 1 {
		close(f.entered)
		<-f.release
	}
	return f.base.call(ctx, w, args...)
}

func TestDockerCanceledConcurrentAdmission(t *testing.T) {
	f := &admissionClient{entered: make(chan struct{}), release: make(chan struct{})}
	d := &LocalDocker{host: "linux", client: f}
	holder := make(chan error, 1)
	go func() { holder <- d.Preflight(context.Background(), owned()) }()
	<-f.entered
	ctx, cancel := context.WithCancel(context.Background())
	waiter := make(chan error, 1)
	invoked := make(chan struct{})
	go func() { close(invoked); waiter <- d.Preflight(ctx, owned()) }()
	<-invoked
	cancel()
	prompt := false
	var waiterErr error
	select {
	case waiterErr = <-waiter:
		prompt = true
	case <-time.After(150 * time.Millisecond):
	}
	// Always release and join both operations before reporting a regression.
	// Thus the RED case cannot strand the holder or hang the package test.
	callsBeforeRelease := f.calls.Load()
	close(f.release)
	if err := <-holder; err != nil {
		t.Fatalf("holder failed: %v", err)
	}
	if !prompt {
		waiterErr = <-waiter
	}
	if !prompt || waiterErr == nil || callsBeforeRelease != 1 || f.calls.Load() != 2 {
		t.Fatalf("prompt=%v waiter=%v calls before release=%d total=%d", prompt, waiterErr, callsBeforeRelease, f.calls.Load())
	}
	// The successful holder only inspected info/image; no container needs cleanup.
	for _, args := range f.base.calls {
		if args[0] != "info" && args[0] != "image" {
			t.Fatalf("unexpected client start: %q", args)
		}
	}
}
