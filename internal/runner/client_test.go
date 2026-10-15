package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as an OWNED fake executable. No Docker binary,
// socket or target code is accessed by these tests.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--config" {
		if len(os.Args) != 6 && !(len(os.Args) == 8 && os.Args[5] == "start" && os.Args[6] == "--attach" && os.Args[7] == fakeID) {
			os.Exit(92)
		}
		if os.Args[3] != "--host" || os.Args[4] != "unix:///var/run/docker.sock" || os.Getenv("HOME") != os.Args[2] || os.Getenv("DOCKER_CONFIG") != os.Args[2] || os.Getenv("DOCKER_HOST") != "" || os.Getenv("DOCKER_CONTEXT") != "" || os.Getenv("AWS_SECRET_ACCESS_KEY") != "" {
			os.Exit(90)
		}
		switch os.Args[5] {
		case "start":
			time.Sleep(2300 * time.Millisecond)
			fmt.Fprint(os.Stdout, "owned attach stdout\n")
			fmt.Fprint(os.Stderr, "owned attach stderr\n")
		case "owned-environment":
			_ = json.NewEncoder(os.Stdout).Encode(os.Environ())
		case "owned-streams":
			fmt.Fprint(os.Stdout, "owned stdout\n")
			fmt.Fprint(os.Stderr, "owned stderr\n")
		case "owned-overflow":
			fmt.Fprint(os.Stdout, strings.Repeat("x", 100000))
			fmt.Fprint(os.Stderr, strings.Repeat("y", 100000))
		case "owned-deadline":
			time.Sleep(5 * time.Second)
		case "owned-tool-exit":
			os.Exit(7)
		case "owned-transport-error":
			fmt.Fprint(os.Stderr, "owned attach transport interrupted\n")
			os.Exit(1)
		case "owned-failure":
			fmt.Fprint(os.Stderr, "startup failed")
			os.Exit(125)
		default:
			os.Exit(91)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func TestExactChildEnvironment(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	config := t.TempDir()
	for _, key := range []string{"OWNED_DUMMY_SECRET", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "SSH_AUTH_SOCK", "PATH"} {
		t.Setenv(key, "owned-parent-sentinel")
	}
	c := &executableDocker{executable, config}
	s, err := c.call(context.Background(), nil, "owned-environment")
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err = json.Unmarshal([]byte(s), &got); err != nil {
		t.Fatal(err)
	}
	expected := []string{"HOME=" + config, "DOCKER_CONFIG=" + config, "LANG=C", "LC_ALL=C"}
	if runtime.GOOS == "windows" {
		expected = append(expected, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"))
		for i, entry := range got {
			key, value, ok := strings.Cut(entry, "=")
			if !ok {
				t.Fatalf("invalid child environment %q", entry)
			}
			got[i] = strings.ToUpper(key) + "=" + value
		}
	}
	sort.Strings(got)
	sort.Strings(expected)
	if !reflect.DeepEqual(got, expected) {
		t.Fatalf("child environment=%q want exact=%q", got, expected)
	}
}

func TestOwnedExecutableClient(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	config := t.TempDir()
	t.Setenv("DOCKER_HOST", "tcp://untrusted.invalid")
	t.Setenv("DOCKER_CONTEXT", "foreign")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "owned-test-sentinel")
	c := &executableDocker{executable, config}
	for _, scenario := range []string{"owned-streams", "owned-overflow", "owned-deadline", "owned-failure"} {
		t.Run(scenario, func(t *testing.T) {
			start := time.Now()
			s, err := c.call(context.Background(), nil, scenario)
			switch scenario {
			case "owned-streams":
				if err != nil || !strings.Contains(s, "owned stdout\n") || !strings.Contains(s, "owned stderr\n") {
					t.Fatalf("%q %v", s, err)
				}
			case "owned-overflow":
				if err == nil || len(s) > 64<<10 {
					t.Fatalf("bytes=%d err=%v", len(s), err)
				}
			case "owned-deadline":
				if err == nil || s != "" || time.Since(start) < 1900*time.Millisecond || time.Since(start) > 3*time.Second {
					t.Fatalf("elapsed=%v err=%v", time.Since(start), err)
				}
			case "owned-failure":
				if err == nil || s != "startup failed" {
					t.Fatalf("%q %v", s, err)
				}
			}
		})
	}
	var output strings.Builder
	if _, err = c.call(context.Background(), &output, "owned-streams"); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"owned stdout\n", "owned stderr\n"} {
		if !strings.Contains(output.String(), line) {
			t.Fatal("missing stream")
		}
	}
	// Explicitly avoid inheriting any command input stream.
	var _ io.Writer = &output
}
