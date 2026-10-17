package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/lintpreparation"
)

const expectedHelp = `Usage: owned-lint-prepare [-h|--help]
No operational flags. No arguments starts effectful preparation on Linux/amd64.
Required environment: GITHUB_WORKSPACE, GITHUB_SHA, GITHUB_RUN_ID, GITHUB_RUN_ATTEMPT.
Requires protected pre-created inputs and launcher; writes seven preparation artifacts.
Output: GITHUB_WORKSPACE/.git/lint-image-preparation/artifacts (must not exist).
Downloads pinned Node/base data and runs fixed Node/npm dependency setup only.
No package scripts, ESLint, launcher, Docker or target execution; not runtime admission.
Use only in a separately authorized fresh protected hosted job.
Source/run metadata are recorded, not authenticated. Success is silent.
Failures use fixed stage/category and optional redacted reason identifiers.
`
const invalidDiagnostic = "owned lint preparation failed; stage=host-inputs; category=validation; no runtime admission or ESLint execution\n"
const unexpectedDiagnostic = "owned lint preparation failed; stage=host-inputs; category=unexpected; no runtime admission or ESLint execution\n"

var requiredKeys = []string{"GITHUB_WORKSPACE", "GITHUB_SHA", "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT"}

func TestRun(t *testing.T) {
	ctx := context.WithValue(context.Background(), struct{}{}, "caller context")
	t.Run("help and rejected arguments have no effects", func(t *testing.T) {
		for _, tc := range []struct {
			args           []string
			code           int
			stdout, stderr string
		}{
			{[]string{"-h"}, 0, expectedHelp, ""}, {[]string{"--help"}, 0, expectedHelp, ""},
			{[]string{"--unknown"}, 2, "", invalidDiagnostic}, {[]string{"private\nargument"}, 2, "", invalidDiagnostic},
			{[]string{"--help", "other"}, 2, "", invalidDiagnostic}, {[]string{"-h", "--help"}, 2, "", invalidDiagnostic},
		} {
			var out, errout bytes.Buffer
			envCalls, prepCalls := 0, 0
			code := run(ctx, tc.args, &out, &errout, func(string) string { envCalls++; return "private" }, func(context.Context, string, string, string, string) error { prepCalls++; return nil })
			if code != tc.code || out.String() != tc.stdout || errout.String() != tc.stderr || envCalls != 0 || prepCalls != 0 {
				t.Fatalf("result: code=%d out=%q err=%q env=%d prep=%d", code, out.String(), errout.String(), envCalls, prepCalls)
			}
		}
	})
	t.Run("each required metadata key", func(t *testing.T) {
		for missingIndex, missing := range requiredKeys {
			var out, errout bytes.Buffer
			var keys []string
			prepCalls := 0
			code := run(ctx, nil, &out, &errout, func(k string) string {
				keys = append(keys, k)
				if k == missing {
					return ""
				}
				return "private"
			}, func(context.Context, string, string, string, string) error { prepCalls++; return nil })
			if code != 2 || out.Len() != 0 || errout.String() != invalidDiagnostic || prepCalls != 0 || !reflect.DeepEqual(keys, requiredKeys[:missingIndex+1]) {
				t.Fatalf("missing %s: code=%d out=%q err=%q prep=%d", missing, code, out.String(), errout.String(), prepCalls)
			}
			for _, k := range keys {
				found := false
				for _, want := range requiredKeys {
					if k == want {
						found = true
					}
				}
				if !found {
					t.Fatal("unexpected environment key", k)
				}
			}
		}
	})
	t.Run("unchanged metadata context and exact diagnostics", func(t *testing.T) {
		for _, tc := range []struct {
			failure error
			code    int
			stderr  string
		}{
			{nil, 0, ""},
			{errors.Join(errors.New("private outer detail"), lintpreparation.NewPreparationFailure("base-layer-diff", "validation", "layer-diff-id")), 1, "owned lint preparation failed; stage=base-layer-diff; category=validation; reason=layer-diff-id; no runtime admission or ESLint execution\n"},
			{errors.New("private URL https://private.invalid/path\nprocess output"), 1, unexpectedDiagnostic},
		} {
			values := []string{" workspace\n", " SHA ", " run id ", " attempt "}
			var keys []string
			prepCalls := 0
			var out, errout bytes.Buffer
			code := run(ctx, nil, &out, &errout, func(k string) string {
				keys = append(keys, k)
				for i, want := range requiredKeys {
					if k == want {
						return values[i]
					}
				}
				t.Fatal("unexpected env key", k)
				return ""
			}, func(got context.Context, w, sha, id, attempt string) error {
				prepCalls++
				if got != ctx || !reflect.DeepEqual([]string{w, sha, id, attempt}, values) {
					t.Fatal("caller binding changed")
				}
				return tc.failure
			})
			if code != tc.code || out.Len() != 0 || errout.String() != tc.stderr || prepCalls != 1 || !reflect.DeepEqual(keys, requiredKeys) {
				t.Fatalf("result: code=%d out=%q err=%q prep=%d keys=%v", code, out.String(), errout.String(), prepCalls, keys)
			}
		}
	})
	t.Run("stdio failures", func(t *testing.T) {
		for _, short := range []bool{false, true} {
			var errout bytes.Buffer
			envCalls, prepCalls := 0, 0
			code := run(ctx, []string{"--help"}, brokenWriter{short}, &errout, func(string) string { envCalls++; return "x" }, func(context.Context, string, string, string, string) error { prepCalls++; return nil })
			if code != 1 || errout.String() != unexpectedDiagnostic || envCalls != 0 || prepCalls != 0 {
				t.Fatalf("help write failure: %d %q %d %d", code, errout.String(), envCalls, prepCalls)
			}
			// Failed diagnostics get no second/raw fallback write.
			writer := &countingFailure{}
			code = run(ctx, []string{"private\nargument"}, io.Discard, writer, func(string) string { t.Fatal("env read"); return "" }, func(context.Context, string, string, string, string) error { t.Fatal("prepare called"); return nil })
			if code != 1 || writer.calls != 1 {
				t.Fatal("diagnostic failure", code, writer.calls)
			}
		}
	})
}

func TestRunRootfsReasons(t *testing.T) {
	for _, reason := range []string{
		"rootfs-launcher", "rootfs-bundle", "rootfs-node", "rootfs-license",
		"rootfs-base-banned", "rootfs-seed-descendant", "rootfs-seed-parent", "rootfs-seed-collision",
		"rootfs-target", "rootfs-modules", "rootfs-entry-path", "rootfs-entry-type", "rootfs-entry-access",
		"rootfs-entry-collision", "rootfs-file-ancestor", "rootfs-parent-type", "rootfs-entry-read", "rootfs-timezone",
		"private/path?token=private\nrootfs-entry-type",
	} {
		t.Run(reason, func(t *testing.T) {
			var out, errout bytes.Buffer
			envCalls, prepareCalls := 0, 0
			code := run(context.Background(), nil, &out, &errout,
				func(string) string { envCalls++; return "private environment" },
				func(context.Context, string, string, string, string) error {
					prepareCalls++
					// Public error fields are revalidated; joined raw context is never printed.
					return errors.Join(errors.New("private raw URL/process/path"), &lintpreparation.PreparationFailure{Stage: "rootfs", Category: "validation", Reason: reason})
				})
			want := "owned lint preparation failed; stage=rootfs; category=validation"
			if !bytes.Contains([]byte(reason), []byte("private")) {
				want += "; reason=" + reason
			}
			want += "; no runtime admission or ESLint execution\n"
			if code != 1 || out.Len() != 0 || errout.String() != want || envCalls != 4 || prepareCalls != 1 {
				t.Fatalf("code=%d stdout=%q stderr=%q env=%d prepare=%d; want stderr=%q", code, out.String(), errout.String(), envCalls, prepareCalls, want)
			}
		})
	}
}

type brokenWriter struct{ short bool }

func (w brokenWriter) Write(p []byte) (int, error) {
	if w.short {
		return len(p) - 1, nil
	}
	return 0, errors.New("private IO detail")
}

type countingFailure struct{ calls int }

func (w *countingFailure) Write([]byte) (int, error) {
	w.calls++
	return 0, errors.New("private stderr detail")
}
