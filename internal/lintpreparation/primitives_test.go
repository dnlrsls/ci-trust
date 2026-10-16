package lintpreparation

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestPrimitives(t *testing.T) {
	if Digest(nil) != "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Error("digest")
	}
	v, err := UniqueJSON([]byte(`{"z":9007199254740993,"a":"é😀<>&\u007f","n":null}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := Canonical(v)
	if err != nil || string(b) != `{"a":"\u00e9\ud83d\ude00<>&\u007f","n":null,"z":9007199254740993}` {
		t.Errorf("canonical: %s %v", b, err)
	}
	for _, s := range []string{`{"a":1,"a":2}`, `{"x":[{"a":1,"\u0061":2}]}`} {
		_, err := UniqueJSON([]byte(s))
		var vf *ValidationFailure
		if !errors.As(err, &vf) || vf.Reason != "json-duplicate-key" {
			t.Errorf("duplicate: %v", err)
		}
	}
	for _, s := range []string{`{} {}`, `[]x`, `{"a":}`, ``, `[1,]`} {
		if _, err := UniqueJSON([]byte(s)); err == nil {
			t.Errorf("accepted %s", s)
		}
	}
	for _, v := range []any{1.5, json.Number("1e2"), json.Number("NaN")} {
		if _, err := Canonical(v); err == nil {
			t.Errorf("accepted unsupported number %v", v)
		}
	}
}

func TestJSONUnicodeBoundary(t *testing.T) {
	for _, s := range []string{`"\ud800"`, `"\udfff"`, `"\ud800\ud800"`, `"\ud800\u0041"`, `"\ud800x"`, `"\ud800\\udc00"`, `{"\ud800":1,"\ufffd":2}`} {
		input := []byte(s)
		v, err := UniqueJSON(input)
		var failure *ValidationFailure
		if v != nil || !errors.As(err, &failure) || failure.Reason != "" {
			t.Errorf("malformed Unicode accepted or wrong failure: %s", s)
		}
		if string(input) != s {
			t.Fatal("input mutated")
		}
	}
	for _, tc := range []struct{ input, want string }{
		{`"\ud83d\ude00"`, "😀"}, {`"\ufffd"`, "�"}, {`"�"`, "�"}, {`"\\ud800"`, `\ud800`},
	} {
		v, err := UniqueJSON([]byte(tc.input))
		if err != nil || v != tc.want {
			t.Errorf("valid Unicode rejected: %s", tc.input)
		}
	}
}

func TestJSONUnicodeKeys(t *testing.T) {
	// Raw Go strings preserve exactly one slash for escapes, two for literal slashes.
	v, err := UniqueJSON([]byte(`{"\ud83d\ude00":1,"\ufffd":2,"\\ud800":3}`))
	if err != nil {
		t.Fatal(err)
	}
	m, ok := v.(map[string]any)
	if !ok || len(m) != 3 || m["😀"] != json.Number("1") || m["�"] != json.Number("2") || m[`\ud800`] != json.Number("3") {
		t.Fatal("key identity changed")
	}
}

func TestJSONDepthBoundary(t *testing.T) {
	if MaxJSONDepth != 1000 {
		t.Fatal("unexpected budget")
	}
	// Small inputs exercise the technical budget, not Python's exact threshold.
	for _, depth := range []int{999, 1000, 1001} {
		input := strings.Repeat("[", depth) + "0" + strings.Repeat("]", depth)
		v, err := UniqueJSON([]byte(input))
		if depth <= 1000 {
			if err != nil {
				t.Fatalf("depth %d rejected: %v", depth, err)
			}
			b, err := Canonical(v)
			if err != nil || string(b) != input {
				t.Errorf("canonical depth %d", depth)
			}
		} else {
			var failure *ValidationFailure
			if v != nil || !errors.As(err, &failure) || failure.Reason != "" {
				t.Error("parser accepted excessive depth")
			}
		}
		var direct any = 0
		for i := 0; i < depth; i++ {
			direct = map[string]any{"x": direct}
		}
		b, err := Canonical(direct)
		if depth <= 1000 && err != nil {
			t.Errorf("direct depth %d rejected", depth)
		}
		if depth > 1000 {
			var failure *ValidationFailure
			if b != nil || !errors.As(err, &failure) || failure.Reason != "" {
				t.Error("canonical accepted excessive depth")
			}
		}
	}
}

func TestDiagnostics(t *testing.T) {
	secret := "secret\n::error::injected"
	for _, reason := range strings.Fields(reasons) {
		err := InStage("base-image", func() error {
			return InStage("base-layer-import", func() error { return NewValidationFailure(reason) })
		})
		var f *PreparationFailure
		if !errors.As(err, &f) || f.Stage != "base-layer-import" || f.Reason != reason {
			t.Fatalf("nested: %v", err)
		}
		if !strings.Contains(Diagnostic(err), "; reason="+reason+";") {
			t.Fatal("reason missing")
		}
	}
	for _, stage := range strings.Fields(stages) {
		err := InStage(stage, func() error { return NewValidationFailure(secret) })
		if err.Error() != stage+":validation" || strings.Contains(Diagnostic(err), secret) || strings.Contains(Diagnostic(err), "reason=") {
			t.Fatal("redaction")
		}
	}
	called := false
	_ = InStage(secret, func() error { called = true; return nil })
	if called {
		t.Fatal("invalid stage executed")
	}
	f := &PreparationFailure{Stage: secret, Category: secret, Reason: secret}
	if Diagnostic(f) != "owned lint preparation failed; stage=unknown; category=unexpected; no runtime admission or ESLint execution" {
		t.Fatal("revalidation")
	}
	cases := []struct {
		err      error
		category string
	}{
		{&url.Error{Op: secret, URL: secret, Err: context.DeadlineExceeded}, "timeout"},
		{&url.Error{Err: os.ErrNotExist}, "network"}, {os.ErrNotExist, "filesystem"},
		{context.DeadlineExceeded, "timeout"}, {NewValidationFailure(secret), "validation"}, {errors.New(secret), "unexpected"},
	}
	for _, c := range cases {
		if got := ErrorCategory(c.err); got != c.category {
			t.Errorf("category %s != %s", got, c.category)
		}
	}
	if InStage("rootfs", func() error { return nil }) != nil {
		t.Fatal("success")
	}
}
