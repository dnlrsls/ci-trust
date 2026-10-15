package cli

import (
	"bytes"
	"testing"
)

func TestBootstrapRecognizedBeforeService(t *testing.T) {
	var out, err bytes.Buffer
	code := Run([]string{"bootstrap", "target", "--dry-run"}, nil, &out, &err, Services{})
	if code != 1 || out.Len() != 0 || err.String() != "ci-trust: bootstrap service unavailable\n" {
		t.Fatalf("exit=%d stdout=%q stderr=%q", code, out.String(), err.String())
	}
}
