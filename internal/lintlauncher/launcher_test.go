package lintlauncher

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

var binding = strings.Repeat("c", 64)

// Only this test executable and its owned helper are launched, never Node.
func TestMain(m *testing.M) {
	if len(os.Args) >= 3 && os.Args[1] == "--owned-tool" {
		mode := os.Args[2]
		if mode == "hold" {
			if len(os.Args) != 4 {
				os.Exit(16)
			}
			deadline := time.Now().Add(3 * time.Second)
			for time.Now().Before(deadline) {
				if _, err := os.Stat(os.Args[3]); err == nil {
					break
				}
				time.Sleep(5 * time.Millisecond)
			}
			_ = os.WriteFile(os.Args[3]+".done", []byte("owned helper released"), 0600)
			os.Exit(0)
		}
		if mode == "sleep" {
			if len(os.Args) == 4 {
				if err := os.WriteFile(os.Args[3], []byte("owned ready"), 0600); err != nil {
					os.Exit(15)
				}
			}
			time.Sleep(10 * time.Second)
			os.Exit(0)
		}
		input, err := io.ReadAll(os.Stdin)
		if err != nil {
			os.Exit(12)
		}
		if mode == "leak0" || mode == "leak1" {
			child := exec.Command(os.Args[0], "--owned-tool", "hold", os.Args[3])
			child.Stdout, child.Stderr = os.Stdout, os.Stderr
			child.Env = []string{"LANG=C", "LC_ALL=C"}
			if err := child.Start(); err != nil {
				os.Exit(13)
			}
			_ = child.Process.Release()
			fmt.Fprint(os.Stdout, "partial owned output")
			if mode == "leak1" {
				os.Exit(1)
			}
			os.Exit(0)
		}
		if mode == "overflow" {
			fmt.Fprint(os.Stdout, strings.Repeat("x", 40000))
			fmt.Fprint(os.Stderr, strings.Repeat("y", 40000))
			os.Exit(1)
		}
		if mode == "exit2" {
			fmt.Fprint(os.Stdout, "configuration failure")
			os.Exit(2)
		}
		if mode == "echo" {
			_, _ = os.Stdout.Write(input)
			os.Exit(0)
		}
		fault := strings.HasPrefix(string(input), "debugger;\n")
		messages, count := "[]", "0"
		if fault {
			messages = `[{"ruleId":"no-debugger","severity":2,"message":"owned finding","line":1,"column":1}]`
			count = "1"
		}
		fmt.Fprint(os.Stdout, `[{"filePath":"/work/fixture.js","messages":`+messages+`,"suppressedMessages":[],"errorCount":`+count+`,"fatalErrorCount":0,"warningCount":0,"fixableErrorCount":0,"fixableWarningCount":0}]`)
		if fault {
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func testLauncher(t *testing.T, mode string) launcher {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	l := launcher{host: "linux", wall: 2 * time.Second, drain: 200 * time.Millisecond}
	l.command = func(ctx context.Context, invocation lintsensitivity.ESLintInvocation) *exec.Cmd {
		cmd := exec.CommandContext(ctx, executable, "--owned-tool", mode)
		cmd.Env = []string{"LANG=C", "LC_ALL=C"}
		// Instrumented helper exits otherwise inject a Go coverage warning into
		// stderr. Keep its coverage destination owned and test-only.
		if testing.CoverMode() != "" {
			cmd.Env = append(cmd.Env, "GOCOVERDIR="+t.TempDir())
		}
		return cmd
	}
	return l
}

func TestLauncherTriplet(t *testing.T) {
	l := testLauncher(t, "lint")
	id := lintsensitivity.Identity{Tool: "eslint", Version: "9.0.0", ConfigSHA256: lintsensitivity.ConfigSHA256(), Image: "sha256:" + strings.Repeat("a", 64), Environment: "owned-test", OS: "LINUX", Snapshot: strings.Repeat("b", 64)}
	var observations []lintsensitivity.Observation
	for _, role := range []lintsensitivity.Role{lintsensitivity.Clean, lintsensitivity.Fault, lintsensitivity.Restored} {
		var stdout, stderr bytes.Buffer
		code := l.run(context.Background(), []string{"--role", string(role), "--binding", binding}, &stdout, &stderr)
		got := lintsensitivity.DecodeESLintReceipt(role, id, binding, lintsensitivity.ESLintCapture{Status: lintsensitivity.Completed, ExitCode: code, Stdout: stdout.Bytes(), Stderr: stderr.Bytes()})
		if code != 0 || stderr.Len() != 0 || got.Status != lintsensitivity.Completed || len(stdout.Bytes()) > 65536 {
			t.Fatalf("code=%d stderr=%q result=%+v", code, stderr.String(), got)
		}
		if role == lintsensitivity.Fault && got.ExitCode != 1 {
			t.Fatal("tool failure not preserved inside successful launcher")
		}
		observations = append(observations, got)
	}
	if lintsensitivity.Classify(observations).Outcome != lintsensitivity.Sensitive {
		t.Fatal("authored test tool sequence rejected")
	}
}

func TestLauncherNoStartAndEmission(t *testing.T) {
	l := testLauncher(t, "lint")
	calls := 0
	original := l.command
	l.command = func(ctx context.Context, i lintsensitivity.ESLintInvocation) *exec.Cmd {
		calls++
		return original(ctx, i)
	}
	for _, args := range [][]string{nil, {"--role", "CLEAN"}, {"--role", "OTHER", "--binding", binding}, {"--role", "CLEAN", "--binding", "bad"}, {"--role", "CLEAN", "--binding", binding, "--tool", "other"}, {"--binding", binding, "--role", "CLEAN"}} {
		var out, diag bytes.Buffer
		if l.run(context.Background(), args, &out, &diag) != 2 || out.Len() != 0 || calls != 0 {
			t.Fatal("invalid args reached process")
		}
	}
	args := []string{"--role", "CLEAN", "--binding", binding}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, diag bytes.Buffer
	if l.run(ctx, args, &out, &diag) != 1 || calls != 0 || out.Len() != 0 {
		t.Fatal("canceled context reached process")
	}
	l.host = "windows"
	if l.run(context.Background(), args, &out, &diag) != 1 || calls != 0 {
		t.Fatal("unsupported host reached process")
	}
	l.host = "linux"
	if l.run(context.Background(), []string{"--help"}, &out, &diag) != 0 || calls != 0 {
		t.Fatal("help started tool")
	}
	if l.run(context.Background(), args, brokenWriter{}, &diag) != 1 {
		t.Fatal("failed receipt write reported success")
	}
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

type cancelWriter struct {
	cancel context.CancelFunc
	bytes.Buffer
}

func (w *cancelWriter) Write(p []byte) (int, error) {
	n, err := w.Buffer.Write(p)
	w.cancel()
	return n, err
}

func TestLauncherCancellationDuringEmission(t *testing.T) {
	l := testLauncher(t, "lint")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	out := &cancelWriter{cancel: cancel}
	var diag bytes.Buffer
	code := l.run(ctx, []string{"--role", "CLEAN", "--binding", binding}, out, &diag)
	if code != 1 || out.Len() == 0 || diag.String() != "owned-eslint: execution unavailable\n" {
		t.Fatal("complete frame masked cancellation during emission")
	}
	id := lintsensitivity.Identity{Tool: "eslint", Version: "9.0.0", ConfigSHA256: lintsensitivity.ConfigSHA256(), Image: "sha256:" + strings.Repeat("a", 64), Environment: "owned-test", OS: "LINUX", Snapshot: strings.Repeat("b", 64)}
	got := lintsensitivity.DecodeESLintReceipt(lintsensitivity.Clean, id, binding, lintsensitivity.ESLintCapture{Status: lintsensitivity.Completed, ExitCode: code, Stdout: out.Bytes(), Stderr: diag.Bytes()})
	if got.Status != lintsensitivity.InvalidOutput || len(got.Diagnostics) != 0 {
		t.Fatal("receipt consumer accepted failed outer emission")
	}
}

func TestLauncherShortWrite(t *testing.T) {
	l := testLauncher(t, "lint")
	var diag bytes.Buffer
	if l.run(context.Background(), []string{"--role", "CLEAN", "--binding", binding}, shortWriter{}, &diag) != 1 || diag.String() != "owned-eslint: output failed\n" {
		t.Fatal("short write reported success")
	}
	if l.run(context.Background(), []string{"--help"}, brokenWriter{}, &diag) != 1 {
		t.Fatal("help output failure ignored")
	}
}

func TestCompleteIORejectsErrorsAndHeldPipes(t *testing.T) {
	for _, mode := range []string{"success", "read-error", "held", "canceled"} {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 3)
		closed := false
		closePipes := func() { closed = true; done <- io.ErrClosedPipe }
		done <- nil
		done <- nil
		if mode == "canceled" {
			cancel()
		}
		if mode == "success" {
			done <- nil
		}
		if mode == "read-error" {
			done <- io.ErrUnexpectedEOF
		}
		got := completeIO(ctx, done, 10*time.Millisecond, closePipes)
		cancel()
		if got != (mode == "success") || (mode == "held" && !closed) {
			t.Fatalf("mode=%s complete=%v closed=%v", mode, got, closed)
		}
	}
}

func TestOwnedInputAndToolBudget(t *testing.T) {
	l := testLauncher(t, "echo")
	for _, n := range []int{0, toolOutputLimit, toolOutputLimit + 1} {
		// Race-instrumented owned helpers may delay their process exit.
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		i, _ := lintsensitivity.OwnedESLintInvocation(lintsensitivity.Clean)
		input := strings.Repeat("x", n)
		capture, err := captureTool(ctx, l.command(ctx, i), input, cancel, l.drain)
		cancel()
		if n <= toolOutputLimit {
			if err != nil || string(capture.Stdout) != input || len(capture.Stderr) != 0 {
				t.Fatalf("input=%d err=%v", n, err)
			}
		} else if err == nil {
			t.Fatal("tool output over limit accepted")
		}
	}
}

func TestLauncherCallerCancellation(t *testing.T) {
	l := testLauncher(t, "sleep")
	l.wall = 3 * time.Second
	marker := filepath.Join(t.TempDir(), "owned-started")
	old := l.command
	l.command = func(ctx context.Context, i lintsensitivity.ESLintInvocation) *exec.Cmd {
		cmd := old(ctx, i)
		cmd.Args = append(cmd.Args, marker)
		return cmd
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, diag bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- l.run(ctx, []string{"--role", "FAULT", "--binding", binding}, &out, &diag) }()
	deadline := time.Now().Add(2 * time.Second)
	started := false
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			started = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case code := <-done:
		if !started || code != 1 || out.Len() != 0 {
			t.Fatal("caller cancellation not verified after tool start")
		}
	case <-time.After(time.Second):
		t.Fatal("caller cancellation stranded launcher")
	}
}

func TestLauncherFailureNoReceipt(t *testing.T) {
	for _, mode := range []string{"sleep", "overflow", "exit2", "missing", "leak0", "leak1"} {
		t.Run(mode, func(t *testing.T) {
			l := testLauncher(t, mode)
			if mode == "sleep" {
				l.wall = 80 * time.Millisecond
			}
			if mode == "missing" {
				l.command = func(ctx context.Context, _ lintsensitivity.ESLintInvocation) *exec.Cmd {
					return exec.CommandContext(ctx, filepath.Join(t.TempDir(), "missing-owned-tool"))
				}
			}
			if strings.HasPrefix(mode, "leak") {
				release := filepath.Join(t.TempDir(), "owned-child-release")
				old := l.command
				l.command = func(ctx context.Context, i lintsensitivity.ESLintInvocation) *exec.Cmd {
					cmd := old(ctx, i)
					cmd.Args = append(cmd.Args, release)
					return cmd
				}
				t.Cleanup(func() {
					if err := os.WriteFile(release, []byte("release"), 0600); err != nil {
						t.Error(err)
						return
					}
					deadline := time.Now().Add(2 * time.Second)
					for time.Now().Before(deadline) {
						if _, err := os.Stat(release + ".done"); err == nil {
							return
						}
						time.Sleep(5 * time.Millisecond)
					}
					t.Error("owned held-pipe helper did not acknowledge release")
				})
			}
			var out, diag bytes.Buffer
			start := time.Now()
			code := l.run(context.Background(), []string{"--role", "FAULT", "--binding", binding}, &out, &diag)
			if code != 1 || out.Len() != 0 || diag.String() != "owned-eslint: execution unavailable\n" || time.Since(start) > 3*time.Second {
				t.Fatalf("code=%d out=%q diag=%q elapsed=%v", code, out.String(), diag.String(), time.Since(start))
			}
		})
	}
}

func TestFixedCommand(t *testing.T) {
	i, _ := lintsensitivity.OwnedESLintInvocation(lintsensitivity.Fault)
	cmd := fixedCommand(context.Background(), i)
	want := append([]string{"/owned/node/bin/node", "/owned/node_modules/eslint/bin/eslint.js"}, i.Args...)
	if cmd.Path != "/owned/node/bin/node" || cmd.Dir != "/work" || !reflect.DeepEqual(cmd.Args, want) || !reflect.DeepEqual(cmd.Env, []string{"LANG=C", "LC_ALL=C"}) {
		t.Fatal("fixed invocation changed")
	}
}

func TestToolOutputConcurrent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &toolOutput{cancel: cancel}
	var wg sync.WaitGroup
	for _, stderr := range []bool{false, true} {
		wg.Add(1)
		go func(stderr bool) {
			defer wg.Done()
			for n := 0; n < 500; n++ {
				_, _ = (toolStream{b, stderr}).Write(bytes.Repeat([]byte("x"), 100))
			}
		}(stderr)
	}
	wg.Wait()
	if !b.overflow || len(b.stdout)+len(b.stderr) != toolOutputLimit || ctx.Err() != context.Canceled {
		t.Fatal("shared tool budget/cancellation failed")
	}
	if n, err := (toolStream{b, true}).Write([]byte("after")); n != 0 || err == nil {
		t.Fatal("tool overflow not sticky")
	}
}
