package runner

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestCapturedTransport(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := &executableDocker{executable, t.TempDir()}
	source := ownedTree(t)
	before := source()
	for _, input := range []string{"", "owned fixture\n\x00tail", strings.Repeat("f", 64<<10)} {
		stdout, stderr, err := c.capture(context.Background(), input, 64<<10, time.Second, "owned-input")
		if err != nil || stdout != input || stderr != "" {
			t.Fatalf("input=%d stdout=%d stderr=%q err=%v", len(input), len(stdout), stderr, err)
		}
	}
	stdout, stderr, err := c.capture(context.Background(), "", 64<<10, time.Second, "owned-streams")
	if err != nil || stdout != "owned stdout\n" || stderr != "owned stderr\n" {
		t.Fatalf("stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}
	stdout, stderr, err = c.capture(context.Background(), "", 64<<10, time.Second, "owned-failure")
	if err == nil || stdout != "" || stderr != "startup failed" {
		t.Fatalf("failed process lost: stdout=%q stderr=%q err=%v", stdout, stderr, err)
	}
	if source() != before {
		t.Fatal("owned source or counter changed")
	}
}

func TestCapturedTransportBounds(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	c := &executableDocker{executable, t.TempDir()}
	for _, scenario := range []string{"owned-split-exact", "owned-split-overflow", "owned-overflow", "owned-stderr-overflow"} {
		stdout, stderr, err := c.capture(context.Background(), "", 64<<10, time.Second, scenario)
		if len(stdout)+len(stderr) > 64<<10 {
			t.Fatal("combined retained bytes exceeded output budget")
		}
		if scenario == "owned-split-exact" {
			if err != nil || stdout != strings.Repeat("x", 32<<10) || stderr != strings.Repeat("y", 32<<10) {
				t.Fatalf("exact boundary: stdout=%d stderr=%d err=%v", len(stdout), len(stderr), err)
			}
		} else if err == nil {
			t.Fatal("output overflow reported success")
		}
	}
	// Invalid bounds and input must fail before attempting even this missing binary.
	missing := &executableDocker{t.TempDir() + "/missing-owned-executable", t.TempDir()}
	for _, tc := range []struct {
		input string
		max   int
		wall  time.Duration
	}{
		{strings.Repeat("x", (64<<10)+1), 64 << 10, time.Second},
		{"", 0, time.Second}, {"", -1, time.Second}, {"", (64 << 10) + 1, time.Second},
		{"", 1, 0}, {"", 1, -1}, {"", 1, time.Minute + 1},
	} {
		stdout, stderr, err := missing.capture(context.Background(), tc.input, tc.max, tc.wall, "owned-input")
		if !errors.Is(err, invalidTransport) || stdout != "" || stderr != "" {
			t.Fatalf("invalid input reached executable: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := missing.capture(ctx, "", 1, time.Second, "owned-input"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled call reached executable: %v", err)
	}
	start := time.Now()
	// A child that does not read cannot strand the bounded stdin copy on timeout.
	stdout, stderr, err := c.capture(context.Background(), strings.Repeat("x", 64<<10), 64<<10, 100*time.Millisecond, "owned-deadline")
	if !errors.Is(err, context.DeadlineExceeded) || stdout != "" || stderr != "" || time.Since(start) > 2*time.Second {
		t.Fatalf("deadline not enforced: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	timer := time.AfterFunc(100*time.Millisecond, cancel)
	defer timer.Stop()
	defer cancel()
	start = time.Now()
	_, _, err = c.capture(ctx, "", 64<<10, time.Minute, "owned-deadline")
	if !errors.Is(err, context.Canceled) || time.Since(start) > 2*time.Second {
		t.Fatalf("caller cancellation not enforced: %v", err)
	}
}

func TestSplitWriterPartialCount(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &splitOutput{max: 4, cancel: cancel}
	if n, err := (streamOutput{b, false}).Write([]byte("abc")); n != 3 || err != nil {
		t.Fatalf("first write: %d %v", n, err)
	}
	if n, err := (streamOutput{b, true}).Write([]byte("de")); n != 1 || err == nil {
		t.Fatalf("partial write: %d %v", n, err)
	}
	stdout, stderr, overflow := b.state()
	if stdout != "abc" || stderr != "d" || !overflow || ctx.Err() != context.Canceled {
		t.Fatalf("stdout=%q stderr=%q overflow=%v", stdout, stderr, overflow)
	}
}

func TestSharedStreamBudgetConcurrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &splitOutput{max: 1024, cancel: cancel}
	var wg sync.WaitGroup
	for _, stderr := range []bool{false, true} {
		wg.Add(1)
		go func(stderr bool) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_, _ = (streamOutput{b, stderr}).Write([]byte(strings.Repeat("x", 16)))
			}
		}(stderr)
	}
	wg.Wait()
	stdout, stderr, overflow := b.state()
	if !overflow || len(stdout)+len(stderr) != 1024 || ctx.Err() != context.Canceled {
		t.Fatal("shared budget/cancellation failed")
	}
	if n, err := (streamOutput{b, false}).Write([]byte("after")); n != 0 || err == nil {
		t.Fatal("overflow did not remain sticky")
	}
}

func TestTransportDoesNotAdmitLint(t *testing.T) {
	p := owned()
	p.Adapter, p.Tool, p.Fixture, p.Config = "owned-eslint-v1", "eslint-10.12.0", "clean", "owned-eslint-v1"
	f := &fake{}
	if _, err := NewOwned(f, p, func(context.Context) (string, error) { return p.Snapshot, nil }); err == nil {
		t.Fatal("unreviewed lint plan admitted")
	}
	if f.probes != 0 || f.starts != 0 || f.removes != 0 {
		t.Fatal("lint rejection touched runtime")
	}
}
