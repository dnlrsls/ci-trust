package lintpreparation

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// registryServer owns the listener, HTTP connections and proxy tunnels together.
// Close must be called outside admitted handlers. No settings are inherited.
type registryServer struct {
	http  *http.Server
	proxy *RegistryProxy
	port  int
	stop  chan struct{}
	done  chan struct{}
	once  sync.Once
	err   error // published by closing done
}

func newRegistryServer(ctx context.Context) (*registryServer, error) {
	return newRegistryServerWith(ctx, net.Listen, NewRegistryProxy)
}

// Only package tests replace listening and proxy construction. Production binds
// an ephemeral IPv4 localhost port and has no address or origin options.
func newRegistryServerWith(ctx context.Context, listen func(string, string) (net.Listener, error), factory func(context.Context) *RegistryProxy) (*registryServer, error) {
	fail := func() error { return NewPreparationFailure("npm-setup", "network", "") }
	if ctx.Err() != nil {
		return nil, fail()
	}
	l, err := listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, fail()
	}
	// Ownership begins with successful listen, including every rejection below.
	addr, ok := l.Addr().(*net.TCPAddr)
	if !ok || !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) || addr.Port < 1 || addr.Port > 65535 || addr.Zone != "" || ctx.Err() != nil {
		l.Close()
		return nil, fail()
	}
	proxy := factory(ctx)
	s := &registryServer{proxy: proxy, port: addr.Port, stop: make(chan struct{}), done: make(chan struct{})}
	s.http = &http.Server{Handler: proxy, ErrorLog: log.New(io.Discard, "", 0), BaseContext: func(net.Listener) context.Context { return ctx },
		ReadTimeout: 30 * time.Second, ReadHeaderTimeout: 30 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second,
		// net/http permits additional read-buffer allowance beyond this value.
		MaxHeaderBytes: 16 << 10,
	}
	served := make(chan error, 1)
	go func() { served <- s.http.Serve(l) }()
	go func() {
		var serveErr error
		joined := false
		select {
		case serveErr = <-served:
			joined = true
		case <-ctx.Done():
		case <-s.stop:
		}
		// Server.Close excludes hijacks; proxy.Close excludes ordinary HTTP conns.
		closeErr := s.http.Close()
		l.Close() // also covers Serve failing before taking listener ownership
		proxyErr := proxy.Close()
		if !joined {
			serveErr = <-served
		}
		if (serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed)) || closeErr != nil || proxyErr != nil {
			s.err = fail()
		}
		close(s.done)
	}()
	select {
	case <-s.done:
		if s.err != nil {
			return nil, s.err
		}
		return nil, fail()
	default:
		return s, nil
	}
}

// Port is usable only while the owned server is live. A concurrent shutdown can
// still interrupt a caller; the composer must treat process failure accordingly.
func (s *registryServer) Port() int {
	select {
	case <-s.done:
		return 0
	default:
		return s.port
	}
}
func (s *registryServer) Denied() bool { return s.proxy.Denied() }
func (s *registryServer) Close() error { s.once.Do(func() { close(s.stop) }); <-s.done; return s.err }
