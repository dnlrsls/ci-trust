package lintpreparation

import (
	"context"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const registryAuthority = "registry.npmjs.org:443"
const registryDialTimeout = 30 * time.Second
const registryLifetime = 180 * time.Second

// RegistryProxy is a credential-free, fixed-origin CONNECT handler. It owns
// accepted tunnels, including hijacked connections not owned by http.Server.
// The caller must Close it when done; construction performs no network I/O.
// Unsupported methods receive 501, like Python BaseHTTPRequestHandler's default,
// but response formatting and general HTTP byte parity are not promised.
type RegistryProxy struct {
	ctx      context.Context
	cancel   context.CancelFunc
	dial     func(context.Context, string, string) (net.Conn, error)
	denied   atomic.Bool
	mu       sync.Mutex
	closed   bool
	handlers sync.WaitGroup
}

// NewRegistryProxy fixes the destination and uses a fresh, proxy-free TCP dialer.
// Neither incoming headers nor ambient proxy/credential settings are forwarded.
func NewRegistryProxy(ctx context.Context) *RegistryProxy {
	return newRegistryProxy(ctx, (&net.Dialer{Timeout: registryDialTimeout}).DialContext)
}

// Only package tests replace dialing; the production authority is not a knob.
func newRegistryProxy(ctx context.Context, dial func(context.Context, string, string) (net.Conn, error)) *RegistryProxy {
	ctx, cancel := context.WithCancel(ctx)
	return &RegistryProxy{ctx: ctx, cancel: cancel, dial: dial}
}

// Denied remains true after any mismatched CONNECT target, including after Close.
// Callers can prioritize origin-denied over a subprocess failure.
func (p *RegistryProxy) Denied() bool { return p.denied.Load() }

// Close cancels dialing and closes tunnels, then waits for both relay directions
// and all admitted handlers. It must not be called from an admitted handler.
func (p *RegistryProxy) Close() error {
	p.mu.Lock()
	p.closed = true
	p.cancel()
	p.mu.Unlock()
	p.handlers.Wait()
	return nil
}

func (p *RegistryProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodConnect {
		w.WriteHeader(http.StatusNotImplemented)
		return
	}
	// RequestURI is the original request-target, not a normalized URL or Host.
	if r.RequestURI != registryAuthority {
		p.denied.Store(true)
		w.WriteHeader(http.StatusForbidden)
		return
	}
	p.mu.Lock()
	if p.closed || p.ctx.Err() != nil {
		p.mu.Unlock()
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	p.handlers.Add(1)
	p.mu.Unlock()
	defer p.handlers.Done()
	upstream, err := p.dial(p.ctx, "tcp", registryAuthority)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	defer upstream.Close()
	// Cancellation owns the upstream even while Hijack is in progress.
	stopUpstream := context.AfterFunc(p.ctx, func() { upstream.Close() })
	defer stopUpstream()
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	client, rw, err := hijacker.Hijack()
	if err != nil {
		return
	}
	defer client.Close()
	stopClient := context.AfterFunc(p.ctx, func() { client.Close() })
	defer stopClient()
	// One absolute lifetime, never refreshed by traffic. The deadline covers the
	// handshake too; no request headers or authorization bytes are serialized.
	deadline := time.Now().Add(registryLifetime)
	if client.SetDeadline(deadline) != nil || upstream.SetDeadline(deadline) != nil {
		return
	}
	if _, err = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	if rw.Flush() != nil {
		return
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Preserve TLS bytes net/http may already have buffered after CONNECT.
		_, _ = io.CopyBuffer(struct{ io.Writer }{upstream}, struct{ io.Reader }{rw.Reader}, make([]byte, 65536))
		client.Close()
		upstream.Close()
	}()
	_, _ = io.CopyBuffer(struct{ io.Writer }{client}, struct{ io.Reader }{upstream}, make([]byte, 65536))
	client.Close()
	upstream.Close()
	<-done
}
