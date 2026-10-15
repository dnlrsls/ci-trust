package evidence

import (
	"bytes"
	"testing"
)

func TestAmbiguousJSON(t *testing.T) {
	if out, ds := Redact([]byte(`{"key":1,"key":2}`), nil); len(ds) == 0 || out != nil {
		t.Fatal("duplicate input keys accepted")
	}
}
func TestRedactionBoundary(t *testing.T) {
	input := []byte(`{"secret-long":"secret-long /host/root/file","n":9007199254740993}`)
	saved := bytes.Clone(input)
	out, ds := Redact(input, []string{"secret", "secret-long", "/host/root", ""})
	if len(ds) > 0 || bytes.Contains(out, []byte("secret")) || bytes.Contains(out, []byte("/host/root")) {
		t.Fatal("sensitive strings remain")
	}
	if !bytes.Equal(input, saved) || !bytes.Contains(out, []byte("9007199254740993")) {
		t.Fatal("input or number changed")
	}
	if out, ds := Redact([]byte(`{"a":1,"b":2}`), []string{"a", "b"}); len(ds) == 0 || out != nil {
		t.Fatal("redacted key collision accepted")
	}
}
