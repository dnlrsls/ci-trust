package lintpreparation

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const downloadAccept = "application/vnd.oci.image.manifest.v1+json"
const downloadTimeout = 45 * time.Second
const downloadRedirects = 10

// Download fetches only anonymous approved HTTPS origins and returns verified
// transfer bytes. The explicit stage survives nesting in a larger preparation.
func Download(ctx context.Context, stage, rawURL, expectedDigest string) ([]byte, error) {
	return downloadWith(ctx, stage, rawURL, expectedDigest, newDownloadTransport, MaxBytes)
}

// Construct rather than clone ambient defaults: no environment proxy, jar,
// credentials, custom TLS roots or automatic content decompression.
func newDownloadTransport() http.RoundTripper {
	return &http.Transport{
		DialContext:           (&net.Dialer{Timeout: downloadTimeout}).DialContext,
		DisableCompression:    true,
		TLSHandshakeTimeout:   downloadTimeout,
		ResponseHeaderTimeout: downloadTimeout,
	}
}

func checkedDownloadURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" || strings.Contains(raw, "#") {
		return false
	}
	// Reject non-ASCII before folding: Unicode case mappings must never turn
	// an IDNA-distinct authority into an approved ASCII origin.
	for i := 0; i < len(u.Host); i++ {
		if u.Host[i] >= 0x80 {
			return false
		}
	}
	switch strings.ToLower(u.Host) {
	case "nodejs.org", "gcr.io", "storage.googleapis.com", "nodejs.org:443", "gcr.io:443", "storage.googleapis.com:443":
		return true
	}
	return false
}

// The factory and byte budget are private deterministic test seams, never public
// configuration. Production always uses the fixed transport and MaxBytes.
func downloadWith(ctx context.Context, stage, rawURL, expected string, transport func() http.RoundTripper, limit int64) ([]byte, error) {
	var data []byte
	err := InStage(stage, func() error {
		if !checkedDownloadURL(rawURL) || !ValidDigest(expected) {
			return NewValidationFailure("")
		}
		ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
		defer cancel()
		failure := func(err error) error {
			category := "network"
			var n net.Error
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || (errors.As(err, &n) && n.Timeout()) {
				category = "timeout"
			}
			return NewPreparationFailure(stage, category, "")
		}
		if ctx.Err() != nil {
			return failure(ctx.Err())
		}
		rt := transport()
		if closer, ok := rt.(interface{ CloseIdleConnections() }); ok {
			defer closer.CloseIdleConnections()
		}
		client := &http.Client{Transport: rt, Timeout: downloadTimeout, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if !checkedDownloadURL(req.URL.String()) {
				return NewPreparationFailure(stage, "validation", "")
			}
			if len(via) > downloadRedirects {
				return NewPreparationFailure(stage, "http", "")
			}
			// Never forward response cookies or future inherited request headers.
			req.Header = make(http.Header)
			req.Header.Set("Accept", downloadAccept)
			return nil
		}}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return NewValidationFailure("")
		}
		req.Header.Set("Accept", downloadAccept)
		response, err := client.Do(req)
		if err != nil {
			var f *PreparationFailure
			if errors.As(err, &f) {
				return f
			}
			return failure(err)
		}
		defer response.Body.Close()
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			return NewPreparationFailure(stage, "http", "")
		}
		bounded, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
		if err != nil {
			return failure(err)
		}
		if ctx.Err() != nil {
			return failure(ctx.Err())
		}
		if int64(len(bounded)) > limit || Digest(bounded) != expected {
			return NewValidationFailure("download-identity")
		}
		data = bounded
		return nil
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}
