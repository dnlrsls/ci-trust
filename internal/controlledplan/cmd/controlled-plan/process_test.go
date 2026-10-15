package main

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

// The helper executes the real main, including its os.Exit, in the owned test
// executable. It never compiles another binary or invokes a host probe.
func TestMainProcessHelper(t *testing.T) {
	if os.Getenv("CONTROLLED_PLAN_TEST_HELPER") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			os.Args = append([]string{os.Args[0]}, os.Args[i+1:]...)
			main()
			os.Exit(99)
		}
	}
	os.Exit(98)
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) {
	return 0, errors.New("owned output failure")
}

func TestOutputFailure(t *testing.T) {
	var stderr bytes.Buffer
	code := run([]string{"--help"}, failedWriter{}, &stderr)
	if code != 1 || stderr.String() != "controlled-plan: output failed\n" {
		t.Fatalf("got (%d, %q), want (1, output failed diagnostic)", code, stderr.String())
	}
}
