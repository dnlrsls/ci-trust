package runner

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// immutableSource measures both owned input bytes and an execution sentinel.
func immutableSource(t *testing.T) func() string {
	t.Helper()
	dir := t.TempDir()
	names := []string{"source.js", "counter"}
	for i, name := range names {
		if err := os.WriteFile(filepath.Join(dir, name), []byte([]string{"const owned = 1;\n", "0\n"}[i]), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return func() string {
		t.Helper()
		h := sha256.New()
		for _, name := range names {
			b, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(h, "%s:%d:", name, len(b))
			_, _ = h.Write(b)
		}
		return fmt.Sprintf("%x", h.Sum(nil))
	}
}
func TestActiveCancellation(t *testing.T) {
	source := immutableSource(t)
	before := source()
	p := owned()
	p.Snapshot = before
	f := &fake{mode: "cancel", started: make(chan struct{})}
	r, err := NewOwned(f, p, func(context.Context) (string, error) { return source(), nil })
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan Result, 1)
	go func() { done <- r.Execute(ctx, p, Consent{p.Digest(), true}) }()
	select {
	case <-f.started:
	case <-time.After(time.Second):
		t.Fatal("start not observed")
	}
	cancel()
	select {
	case got := <-done:
		if got.Outcome != Unverifiable || !strings.Contains(got.Reason, "canceled") || !got.CleanupVerified || !f.cleanupLive || f.starts != 1 || f.removes != 1 {
			t.Fatalf("result=%+v starts=%d cleanup=%d independent=%v", got, f.starts, f.removes, f.cleanupLive)
		}
	case <-time.After(time.Second):
		t.Fatal("active cancellation did not return within bound")
	}
	if source() != before {
		t.Fatal("source files/counter changed")
	}
}
func TestBoundedWriterCountsAcceptedBytes(t *testing.T) {
	b := &boundedOutput{max: 2, cancel: func() {}}
	n, err := b.Write([]byte("overflow"))
	if n != 2 || err == nil || b.String() != "ov" {
		t.Fatalf("accepted=%d err=%v output=%q", n, err, b.String())
	}
}
func TestConcurrentBoundedOutput(t *testing.T) {
	b := &boundedOutput{max: 32, cancel: func() {}}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = b.Write([]byte("xxxx"))
				_, _ = b.state()
			}
		}()
	}
	wg.Wait()
	if len(b.String()) != 32 {
		t.Fatal("output bound not enforced")
	}
}
