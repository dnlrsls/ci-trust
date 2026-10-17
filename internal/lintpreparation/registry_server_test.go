package lintpreparation

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type registryTestListener struct {
	conns chan net.Conn
	done  chan struct{}
	once  sync.Once
	addr  net.Addr
	fail  error
}

func testRegistryListener() *registryTestListener {
	return &registryTestListener{conns: make(chan net.Conn), done: make(chan struct{}), addr: &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 12345}}
}
func (l *registryTestListener) Accept() (net.Conn, error) {
	if l.fail != nil {
		return nil, l.fail
	}
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}
func (l *registryTestListener) Close() error   { l.once.Do(func() { close(l.done) }); return nil }
func (l *registryTestListener) Addr() net.Addr { return l.addr }
func startTestRegistry(t *testing.T, ctx context.Context, l *registryTestListener, factory func(context.Context) *RegistryProxy) *registryServer {
	t.Helper()
	s, err := newRegistryServerWith(ctx, func(network, address string) (net.Listener, error) {
		if network != "tcp4" || address != "127.0.0.1:0" {
			t.Errorf("bind %q %q", network, address)
		}
		return l, nil
	}, factory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}
func TestRegistryServerPolicy(t *testing.T) {
	l := testRegistryListener()
	s := startTestRegistry(t, context.Background(), l, NewRegistryProxy)
	if s.Port() != 12345 {
		t.Fatal("port")
	}
	h := s.http
	if h.ReadTimeout != 30*time.Second || h.ReadHeaderTimeout != 30*time.Second || h.WriteTimeout != 30*time.Second || h.IdleTimeout != 30*time.Second || h.MaxHeaderBytes != 16<<10 || h.ErrorLog == nil {
		t.Fatal("unbounded HTTP policy")
	}
}
func TestRegistryServerRejectBeforeOwnership(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	listen := func(string, string) (net.Listener, error) { calls++; return nil, errors.New("private address") }
	if s, err := newRegistryServerWith(ctx, listen, NewRegistryProxy); s != nil || err == nil || calls != 0 {
		t.Fatal("canceled constructor listened")
	}
	if s, err := newRegistryServerWith(context.Background(), listen, NewRegistryProxy); s != nil || err == nil || strings.Contains(err.Error(), "private") {
		t.Fatal("listen failure leaked")
	}
	for _, addr := range []net.Addr{&net.TCPAddr{IP: net.IPv4(0, 0, 0, 0), Port: 123}, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}, &net.UnixAddr{Name: "private"}} {
		l := testRegistryListener()
		l.addr = addr
		s, err := newRegistryServerWith(context.Background(), func(string, string) (net.Listener, error) { return l, nil }, NewRegistryProxy)
		if s != nil || err == nil {
			t.Fatal("invalid address accepted")
		}
		select {
		case <-l.done:
		default:
			t.Fatal("listener not closed")
		}
	}
}
func registryRequest(t *testing.T, l *registryTestListener, request string) int {
	t.Helper()
	server, client := net.Pipe()
	defer client.Close()
	l.conns <- server
	wrote := make(chan struct{})
	go func() { defer close(wrote); _, _ = io.WriteString(client, request) }()
	r, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	<-wrote
	r.Body.Close()
	return r.StatusCode
}
func TestRegistryServerHTTP(t *testing.T) {
	l := testRegistryListener()
	s := startTestRegistry(t, context.Background(), l, NewRegistryProxy)
	if got := registryRequest(t, l, "CONNECT forbidden:443 HTTP/1.1\r\nHost: forbidden:443\r\n\r\n"); got != 403 || !s.Denied() {
		t.Fatal(got)
	}
	// net/http adds a read-buffer allowance; this deliberately exceeds both.
	if got := registryRequest(t, l, "GET / HTTP/1.1\r\nHost: localhost\r\nX-Large: "+strings.Repeat("x", 24<<10)+"\r\n\r\n"); got != 431 {
		t.Fatal(got)
	}
}
func TestRegistryServerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := testRegistryListener()
	s := startTestRegistry(t, ctx, l, NewRegistryProxy)
	server, client := net.Pipe()
	defer client.Close()
	l.conns <- server
	cancel()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection survived")
	}
	if s.Port() != 0 {
		t.Fatal("closed port exposed")
	}
}
func TestRegistryServerStartCancelAndConcurrentClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	l := testRegistryListener()
	factories := 0
	s, err := newRegistryServerWith(ctx, func(string, string) (net.Listener, error) { cancel(); return l, nil }, func(ctx context.Context) *RegistryProxy { factories++; return NewRegistryProxy(ctx) })
	if s != nil || err == nil || factories != 0 {
		t.Fatal("cancellation after listen admitted proxy")
	}
	select {
	case <-l.done:
	default:
		t.Fatal("listener leaked")
	}

	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	l = testRegistryListener()
	s = startTestRegistry(t, ctx, l, NewRegistryProxy)
	var joined sync.WaitGroup
	joined.Add(3)
	for i := 0; i < 3; i++ {
		go func() {
			defer joined.Done()
			if err := s.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	cancel()
	joined.Wait()
	select {
	case <-l.done:
	default:
		t.Fatal("listener survived")
	}
}

func TestRegistryServerServeFailure(t *testing.T) {
	l := testRegistryListener()
	l.fail = errors.New("private address")
	s, err := newRegistryServerWith(context.Background(), func(string, string) (net.Listener, error) { return l, nil }, NewRegistryProxy)
	if err != nil {
		return
	} // Immediate failure may be observed during construction.
	<-s.done
	for i := 0; i < 2; i++ {
		if err := s.Close(); err == nil || strings.Contains(err.Error(), "private") {
			t.Fatal("serve error lost or leaked")
		}
	}
}
func TestRegistryServerHijackClose(t *testing.T) {
	l := testRegistryListener()
	upstream, peer := net.Pipe()
	defer peer.Close()
	s := startTestRegistry(t, context.Background(), l, func(ctx context.Context) *RegistryProxy {
		return newRegistryProxy(ctx, func(context.Context, string, string) (net.Conn, error) { return upstream, nil })
	})
	server, client := net.Pipe()
	defer client.Close()
	l.conns <- server
	written := make(chan struct{})
	go func() {
		defer close(written)
		_, _ = io.WriteString(client, "CONNECT registry.npmjs.org:443 HTTP/1.1\r\nHost: registry.npmjs.org:443\r\n\r\n")
	}()
	r, err := http.ReadResponse(bufio.NewReader(client), nil)
	if err != nil || r.StatusCode != 200 {
		t.Fatalf("hijack: %v", err)
	}
	<-written
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []net.Conn{client, peer} {
		if _, err := c.Read(make([]byte, 1)); err == nil {
			t.Fatal("tunnel survived")
		}
	}
}
