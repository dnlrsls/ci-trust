package main

import (
	"bytes"
	"testing"
)

func TestCLI(t *testing.T) {
	for _, tt := range []struct {
		args     []string
		code     int
		out, err string
	}{{[]string{"--help"}, 0, usage, ""}, {[]string{"pack", "--help"}, 0, usage, ""}, {nil, 2, "", "usage: owned-image pack --binary FILE --marker FILE | inspect --archive FILE --binary FILE --marker FILE\n"}, {[]string{"pack", "--binary", "missing", "--marker", "missing"}, 1, "", "owned-image: invalid input\n"}} {
		var out, err bytes.Buffer
		code := run(tt.args, &out, &err)
		if code != tt.code || out.String() != tt.out || err.String() != tt.err {
			t.Fatalf("%v: %d %q %q", tt.args, code, out.String(), err.String())
		}
	}
}
