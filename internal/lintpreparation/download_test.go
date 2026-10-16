package lintpreparation

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type downloadRT func(*http.Request) (*http.Response, error)

func (f downloadRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func downloadResponse(code int, body string) *http.Response {
	return &http.Response{StatusCode: code, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: 1}
}
func downloadFailure(t *testing.T, data []byte, err error, stage, category, reason string) {
	t.Helper()
	var f *PreparationFailure
	if data != nil || !errors.As(err, &f) || *f != (PreparationFailure{stage, category, reason}) {
		t.Fatalf("data=%q err=%v failure=%+v", data, err, f)
	}
	want := "owned lint preparation failed; stage=" + stage + "; category=" + category
	if reason != "" {
		want += "; reason=" + reason
	}
	want += "; no runtime admission or ESLint execution"
	if Diagnostic(err) != want {
		t.Fatal(Diagnostic(err))
	}
	if Diagnostic(InStage("base-image", func() error { return err })) != want {
		t.Fatal("lost nested stage")
	}
}
func TestDownload(t *testing.T) {
	const origin = "https://nodejs.org/file"
	t.Run("verified", func(t *testing.T) {
		calls := 0
		factory := func() http.RoundTripper {
			return downloadRT(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "GET" || r.Header.Get("Accept") != downloadAccept || len(r.Header) != 1 {
					t.Fatal("unexpected request")
				}
				d, ok := r.Context().Deadline()
				if !ok || time.Until(d) > 45*time.Second {
					t.Fatal("missing deadline")
				}
				return downloadResponse(200, "verified"), nil
			})
		}
		for i, u := range []string{origin, "https://NODEJS.ORG/x", "https://GcR.Io:443/x", "https://Storage.GoogleApis.Com/x"} {
			data, err := downloadWith(context.Background(), "node-download", u, Digest([]byte("verified")), factory, 8)
			if err != nil || string(data) != "verified" || calls != i+1 {
				t.Fatalf("%q %v calls=%d", data, err, calls)
			}
		}
	})
	t.Run("preflight", func(t *testing.T) {
		calls := 0
		factory := func() http.RoundTripper { calls++; return nil }
		for _, u := range []string{"http://nodejs.org/x", "https://private.test/x", "https://u:p@nodejs.org/x", "https://nodejs.org:444/x", "https://nodejs.org/x#secret", "https://nodejſ.org/x", "https://ſtorage.googleapis.com/x", "https://nоdejs.org/x", "https://nodejs.org./x", "https://nodejs.org:/x", "https://nodejs.org/x#", "https://nodejs.org/%zz"} {
			data, err := downloadWith(context.Background(), "node-download", u, Digest(nil), factory, 8)
			downloadFailure(t, data, err, "node-download", "validation", "")
		}
		for _, pin := range []string{"", "secret", strings.TrimPrefix(Digest(nil), "sha256:"), strings.ToUpper(Digest(nil))} {
			data, err := downloadWith(context.Background(), "node-download", origin, pin, factory, 8)
			downloadFailure(t, data, err, "node-download", "validation", "")
		}
		data, err := downloadWith(context.Background(), "secret", origin, Digest(nil), factory, 8)
		downloadFailure(t, data, err, "unknown", "unexpected", "")
		if calls != 0 {
			t.Fatal(calls)
		}
	})
	t.Run("failures", func(t *testing.T) {
		cases := []struct {
			name, body, category, reason string
			code                         int
			err                          error
			reader                       io.ReadCloser
		}{
			{name: "mismatch", body: "changed", code: 200, category: "validation", reason: "download-identity"},
			{name: "cap", body: "123456789", code: 200, category: "validation", reason: "download-identity"},
			{name: "status", code: 403, category: "http"},
			{name: "network", err: errors.New("secret URL credentials"), category: "network"},
			{name: "timeout", err: context.DeadlineExceeded, category: "timeout"},
			{name: "read", code: 200, reader: io.NopCloser(downloadBadReader{}), category: "network"},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				factory := func() http.RoundTripper {
					return downloadRT(func(*http.Request) (*http.Response, error) {
						if tc.err != nil {
							return nil, tc.err
						}
						r := downloadResponse(tc.code, tc.body)
						if tc.reader != nil {
							r.Body = tc.reader
						}
						return r, nil
					})
				}
				data, err := downloadWith(context.Background(), "base-config", origin, Digest([]byte("verified")), factory, 8)
				downloadFailure(t, data, err, "base-config", tc.category, tc.reason)
			})
		}
	})
	t.Run("redirects", func(t *testing.T) {
		for _, tc := range []struct {
			target, category string
			loop             bool
			calls            int
		}{
			{target: "https://storage.googleapis.com/object?X-Goog-Signature=secret", calls: 2},
			{target: "https://GcR.Io:443/object", calls: 2},
			{target: "https://nodejs.org/next#", category: "validation", calls: 1},
			{target: "/next#", category: "validation", calls: 1},
			{target: "/next", calls: 2},
			{target: "/next%23part?value=%23", calls: 2},
			{target: "https://ſtorage.googleapis.com/x", category: "validation", calls: 1},
			{target: "http://nodejs.org/x", category: "validation", calls: 1},
			{target: "https://evil.test/secret", category: "validation", calls: 1},
			{target: "https://user:secret@gcr.io/x", category: "validation", calls: 1},
			{target: origin, category: "http", loop: true, calls: 11},
		} {
			t.Run(tc.target, func(t *testing.T) {
				calls := 0
				factory := func() http.RoundTripper {
					return downloadRT(func(r *http.Request) (*http.Response, error) {
						calls++
						if len(r.Header) != 1 || r.Header.Get("Accept") != downloadAccept || r.URL.User != nil {
							t.Fatal("ambient headers/credentials")
						}
						if calls == 1 || tc.loop {
							resp := downloadResponse(302, "")
							resp.Header.Set("Location", tc.target)
							resp.Header.Set("Set-Cookie", "secret=value")
							return resp, nil
						}
						return downloadResponse(200, "ok"), nil
					})
				}
				data, err := downloadWith(context.Background(), "base-manifest", origin, Digest([]byte("ok")), factory, 8)
				if tc.category != "" {
					downloadFailure(t, data, err, "base-manifest", tc.category, "")
				} else if err != nil || string(data) != "ok" {
					t.Fatalf("%q %v", data, err)
				}
				if calls != tc.calls {
					t.Fatalf("calls=%d want=%d", calls, tc.calls)
				}
			})
		}
	})
	t.Run("cancellation", func(t *testing.T) {
		for _, deadline := range []bool{false, true} {
			var ctx context.Context
			var cancel context.CancelFunc
			category := "network"
			if deadline {
				ctx, cancel = context.WithDeadline(context.Background(), time.Unix(0, 0))
				category = "timeout"
			} else {
				ctx, cancel = context.WithCancel(context.Background())
				cancel()
			}
			defer cancel()
			calls := 0
			factory := func() http.RoundTripper { calls++; return nil }
			data, err := downloadWith(ctx, "node-download", origin, Digest(nil), factory, 8)
			downloadFailure(t, data, err, "node-download", category, "")
			if calls != 0 {
				t.Fatal(calls)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		factory := func() http.RoundTripper {
			return downloadRT(func(r *http.Request) (*http.Response, error) { cancel(); return nil, r.Context().Err() })
		}
		data, err := downloadWith(ctx, "node-download", origin, Digest(nil), factory, 8)
		downloadFailure(t, data, err, "node-download", "network", "")
	})
	t.Run("fixed transport", func(t *testing.T) {
		a := newDownloadTransport().(*http.Transport)
		b := newDownloadTransport().(*http.Transport)
		if a == b || a == http.DefaultTransport || a.Proxy != nil || !a.DisableCompression || a.TLSClientConfig != nil || a.DialTLSContext != nil || a.DialContext == nil {
			t.Fatal("ambient transport")
		}
		if a.TLSHandshakeTimeout != downloadTimeout || a.ResponseHeaderTimeout != downloadTimeout || MaxBytes != 512<<20 {
			t.Fatal("bounds")
		}
	})
	t.Run("bounded synthetic reader", func(t *testing.T) {
		reader := &downloadCountingReader{}
		factory := func() http.RoundTripper {
			return downloadRT(func(*http.Request) (*http.Response, error) {
				r := downloadResponse(200, "")
				r.Body = io.NopCloser(reader)
				return r, nil
			})
		}
		data, err := downloadWith(context.Background(), "node-download", origin, Digest(nil), factory, 8)
		downloadFailure(t, data, err, "node-download", "validation", "download-identity")
		if reader.bytes != 9 {
			t.Fatal(reader.bytes)
		}
	})
}

type downloadBadReader struct{}

func (downloadBadReader) Read(p []byte) (int, error) {
	copy(p, "x")
	return 1, errors.New("secret read path")
}

type downloadCountingReader struct{ bytes int }

func (r *downloadCountingReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 'x'
	}
	r.bytes += len(p)
	return len(p), nil
}
