package lintpreparation

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRegistryBoundary(t *testing.T) {
	calls := 0
	p := newRegistryProxy(context.Background(), func(context.Context, string, string) (net.Conn, error) { calls++; return nil, errors.New("private") })
	defer p.Close()
	for _, target := range []string{"REGISTRY.npmjs.org:443", "registry.npmjs.org", "registry.npmjs.org:0443", "registry.npmjs.org:443/", "user@registry.npmjs.org:443", "evil:443", "https://registry.npmjs.org:443"} {
		r := &http.Request{Method: "CONNECT", RequestURI: target}
		w := httptest.NewRecorder()
		p.ServeHTTP(w, r)
		if w.Code != 403 || !p.Denied() || calls != 0 {
			t.Fatalf("boundary: status=%d denied=%v calls=%d", w.Code, p.Denied(), calls)
		}
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, &http.Request{Method: "GET", RequestURI: registryAuthority})
	if w.Code != 501 || calls != 0 || !p.Denied() {
		t.Fatal("unsupported method changed boundary")
	}
}

func TestRegistryFixedConstruction(t *testing.T) {
	p := NewRegistryProxy(context.Background())
	defer p.Close()
	if p.Denied() || registryDialTimeout != 30*time.Second || registryLifetime != 180*time.Second {
		t.Fatal("fixed policy")
	}
	w := httptest.NewRecorder()
	p.ServeHTTP(w, &http.Request{Method: "CONNECT", RequestURI: "other:443"})
	if w.Code != 403 {
		t.Fatal("public constructor boundary")
	}
}

type proxyHijack struct {
	*httptest.ResponseRecorder
	conn   net.Conn
	reader *bufio.Reader
}

func (h proxyHijack) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return h.conn, bufio.NewReadWriter(h.reader, bufio.NewWriter(h.conn)), nil
}

type deadlineConn struct {
	net.Conn
	deadlines chan time.Time
}

func (c deadlineConn) SetDeadline(d time.Time) error { c.deadlines <- d; return c.Conn.SetDeadline(d) }

func TestRegistryRelay(t *testing.T) {
	for _, end := range []string{"close", "cancel", "eof"} {
		t.Run(end, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			client, handler := net.Pipe()
			upstream, peer := net.Pipe()
			defer client.Close()
			defer peer.Close()
			deadlines := make(chan time.Time, 2)
			dialed := make(chan struct{}, 1)
			p := newRegistryProxy(ctx, func(ctx context.Context, network, address string) (net.Conn, error) {
				if network != "tcp" || address != registryAuthority {
					t.Error("nonfixed dial")
				}
				if ctx.Err() != nil {
					t.Error("premature cancellation")
				}
				dialed <- struct{}{}
				return deadlineConn{upstream, deadlines}, nil
			})
			defer p.Close()
			// Model TLS bytes already consumed by net/http's buffered reader.
			reader := bufio.NewReader(io.MultiReader(strings.NewReader("TLS"), handler))
			done := make(chan struct{})
			start := time.Now()
			go func() {
				defer close(done)
				p.ServeHTTP(proxyHijack{httptest.NewRecorder(), deadlineConn{handler, deadlines}, reader}, &http.Request{Method: "CONNECT", RequestURI: registryAuthority, Header: http.Header{"Proxy-Authorization": {"secret"}, "Authorization": {"secret"}}})
			}()
			select {
			case <-dialed:
			case <-done:
				t.Fatal("handler returned without dialing")
			}
			br := bufio.NewReader(client)
			line, err := br.ReadString('\n')
			if err != nil || line != "HTTP/1.1 200 Connection Established\r\n" {
				t.Fatalf("handshake %q %v", line, err)
			}
			line, err = br.ReadString('\n')
			if err != nil || line != "\r\n" {
				t.Fatal("handshake includes headers")
			}
			b := make([]byte, 3)
			if _, err = io.ReadFull(peer, b); err != nil || string(b) != "TLS" {
				t.Fatal("buffered bytes lost or headers forwarded")
			}
			for i := 0; i < 2; i++ {
				d := <-deadlines
				if d.Before(start.Add(registryLifetime)) || d.After(time.Now().Add(registryLifetime)) {
					t.Fatal("absolute lifetime")
				}
			}
			sent := make(chan error, 1)
			go func() { _, e := peer.Write([]byte("reply")); sent <- e }()
			b = make([]byte, 5)
			if _, err = io.ReadFull(br, b); err != nil || string(b) != "reply" {
				t.Fatal("upstream relay")
			}
			if err = <-sent; err != nil {
				t.Fatal(err)
			}
			go func() { _, e := client.Write([]byte("next")); sent <- e }()
			b = make([]byte, 4)
			if _, err = io.ReadFull(peer, b); err != nil || string(b) != "next" {
				t.Fatal("client relay")
			}
			if err = <-sent; err != nil {
				t.Fatal(err)
			}
			switch end {
			case "close":
				p.Close()
			case "cancel":
				cancel()
			case "eof":
				peer.Close()
			}
			<-done
			if _, err = client.Write([]byte("x")); err == nil {
				t.Fatal("client not closed")
			}
			if p.Denied() {
				t.Fatal("accepted relay set denied")
			}
		})
	}
}

func TestRegistryCloseBeforeDial(t *testing.T) {
	calls := 0
	p := newRegistryProxy(context.Background(), func(context.Context, string, string) (net.Conn, error) { calls++; return nil, errors.New("private") })
	p.Close()
	p.Close()
	w := httptest.NewRecorder()
	p.ServeHTTP(w, &http.Request{Method: "CONNECT", RequestURI: registryAuthority})
	if calls != 0 {
		t.Fatal("dial after close")
	}
}
