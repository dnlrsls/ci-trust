package lintpreparation

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type installProxy struct {
	port, closes, deniedCalls int
	denied                    bool
	closeErr                  error
}

func (p *installProxy) Port() int    { return p.port }
func (p *installProxy) Denied() bool { p.deniedCalls++; return p.denied }
func (p *installProxy) Close() error { p.closes++; return p.closeErr }

func TestNPMInstall(t *testing.T) {
	manifest := []byte(`{"name":"ci-trust-owned-eslint-preparation","version":"0.0.0","private":true,"dependencies":{"eslint":"10.12.0"}}`)
	bundle := []byte(`{"version":"11.19.1"}`)
	lock := []byte(`{"lockfileVersion":3,"packages":{"":{"dependencies":{"eslint":"10.12.0"}},"node_modules/eslint":{"version":"10.12.0","resolved":"https://registry.npmjs.org/eslint","integrity":"sha512-` + strings.Repeat("A", 86) + `=="}}}`)
	for _, name := range []string{"success", "precancel", "bundle", "manifest", "extra-target", "nonempty-home", "bad-port", "factory", "first-exit", "denied-success", "denied-exit", "timeout", "filesystem", "cancel", "invalid-first", "invalid-second", "frozen", "oversize", "missing", "directory", "symlink", "close", "close-and-phase", "dead-after-ci"} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			node, target, home := filepath.Join(base, "node"), filepath.Join(base, "target"), filepath.Join(base, "home")
			npmDir := filepath.Join(node, "lib", "node_modules", "npm")
			for _, d := range []string{npmDir, target, home} {
				if err := os.MkdirAll(d, 0700); err != nil {
					t.Fatal(err)
				}
			}
			put := func(p string, b []byte) {
				t.Helper()
				if err := os.WriteFile(p, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			npmPath, manifestPath := filepath.Join(npmDir, "package.json"), filepath.Join(target, "package.json")
			put(npmPath, bundle)
			put(manifestPath, manifest)
			p := &installProxy{port: 1234}
			factories := 0
			var phases []string
			wantStage, wantCat := "", ""
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch name {
			case "precancel":
				cancel()
				wantCat = "network"
			case "bundle":
				put(npmPath, []byte(`{}`))
				wantCat = "validation"
			case "manifest":
				put(manifestPath, []byte(`{}`))
				wantCat = "validation"
			case "extra-target":
				put(filepath.Join(target, ".npmrc"), []byte("keep"))
				wantCat = "validation"
			case "nonempty-home":
				put(filepath.Join(home, "global.npmrc"), []byte("keep"))
				wantCat = "validation"
			case "bad-port":
				p.port = 0
				wantCat = "network"
			case "factory":
				wantCat = "network"
			case "close":
				p.closeErr = NewValidationFailure("")
				wantCat = "network"
			case "first-exit", "close-and-phase":
				wantStage = "npm-lock"
				wantCat = "process-failed"
			case "denied-success", "denied-exit":
				p.denied = true
				wantStage = "npm-lock"
				wantCat = "origin-denied"
			case "timeout":
				p.denied = true
				wantStage = "npm-lock"
				wantCat = "timeout"
			case "filesystem":
				p.denied = true
				wantStage = "npm-lock"
				wantCat = "filesystem"
			case "cancel":
				p.denied = true
				wantStage = "npm-lock"
				wantCat = "network"
			case "invalid-first", "oversize", "directory", "symlink":
				wantStage = "npm-lock"
				wantCat = "validation"
			case "missing":
				wantStage = "npm-lock"
				wantCat = "filesystem"
			case "invalid-second", "frozen":
				wantStage = "npm-ci"
				wantCat = "validation"
			case "dead-after-ci":
				wantStage = "npm-ci"
				wantCat = "network"
			}
			beforeManifest, _ := os.ReadFile(manifestPath)
			beforeBundle, _ := os.ReadFile(npmPath)
			factory := func(context.Context) (npmInstallProxy, error) {
				factories++
				if name == "factory" {
					return nil, NewPreparationFailure("npm-setup", "network", "")
				}
				return p, nil
			}
			phase := func(_ context.Context, stage, n, tar, h string, port int) error {
				phases = append(phases, stage)
				if n != node || tar != target || h != home || port != 1234 {
					t.Fatal("phase arguments")
				}
				for _, rc := range []string{"user.npmrc", "global.npmrc"} {
					b, e := os.ReadFile(filepath.Join(home, rc))
					if e != nil || len(b) != 0 {
						t.Fatal("empty rc required")
					}
					st, _ := os.Stat(filepath.Join(home, rc))
					if runtime.GOOS != "windows" && st.Mode().Perm()&0077 != 0 {
						t.Fatal("rc permissions")
					}
				}
				switch name {
				case "first-exit", "denied-exit", "close-and-phase":
					p.closeErr = NewValidationFailure("")
					return NewPreparationFailure(stage, "process-failed", "")
				case "timeout":
					return NewPreparationFailure(stage, "timeout", "")
				case "filesystem":
					return &os.PathError{Op: "open", Path: "private", Err: os.ErrPermission}
				case "cancel":
					return context.Canceled
				}
				out := lock
				if name == "invalid-first" || name == "invalid-second" && stage == "npm-ci" {
					out = []byte(`{}`)
				}
				if name == "frozen" && stage == "npm-ci" {
					out = append([]byte("\n"), lock...)
				}
				if name == "oversize" {
					out = bytes.Repeat([]byte(" "), 1025)
				}
				path := filepath.Join(target, "package-lock.json")
				if name == "missing" {
					return nil
				}
				if name == "directory" {
					return os.Mkdir(path, 0700)
				}
				if name == "symlink" {
					if e := os.Symlink(npmPath, path); e != nil {
						t.Fatalf("required symlink filesystem proof unavailable: %v", e)
					}
					return nil
				}
				put(path, out)
				if name == "dead-after-ci" && stage == "npm-ci" {
					p.port = 0
				}
				return nil
			}
			flags, raw, err := installNPMWith(ctx, node, target, home, factory, phase, 1024)
			preflight := name == "precancel" || name == "bundle" || name == "manifest" || name == "extra-target" || name == "nonempty-home"
			if wantCat == "" {
				if err != nil || !bytes.Equal(raw, lock) || !reflect.DeepEqual(flags, npmCommonFlags(home)) || !reflect.DeepEqual(phases, []string{"npm-lock", "npm-ci"}) {
					t.Fatalf("success: %v phases=%v", err, phases)
				}
				flags[0] = "changed"
				if npmCommonFlags(home)[0] == "changed" {
					t.Fatal("shared flags")
				}
			} else {
				if wantStage == "" {
					wantStage = "npm-setup"
				}
				if err == nil || err.Error() != wantStage+":"+wantCat || flags != nil || raw != nil {
					t.Fatalf("failure: %v flags=%v raw=%v", err, flags, raw)
				}
			}
			if preflight {
				if factories != 0 || len(phases) != 0 {
					t.Fatal("preflight effects")
				}
				entries, _ := os.ReadDir(home)
				expected := 0
				if name == "nonempty-home" {
					expected = 1
				}
				if len(entries) != expected {
					t.Fatal("preflight wrote rc")
				}
			}
			wantClose := 1
			if preflight || name == "factory" {
				wantClose = 0
			}
			if p.closes != wantClose {
				t.Fatalf("close=%d", p.closes)
			}
			if wantStage == "npm-lock" && len(phases) != 1 {
				t.Fatal("ci after failed lock")
			}
			if name == "timeout" || name == "filesystem" || name == "cancel" {
				if p.deniedCalls != 0 {
					t.Fatal("exception inspected denied")
				}
			}
			afterManifest, _ := os.ReadFile(manifestPath)
			afterBundle, _ := os.ReadFile(npmPath)
			if !bytes.Equal(beforeManifest, afterManifest) || !bytes.Equal(beforeBundle, afterBundle) {
				t.Fatal("input changed")
			}
			if preflight {
				entries, e := os.ReadDir(target)
				want := 1
				if name == "extra-target" {
					want = 2
				}
				if e != nil || len(entries) != want {
					t.Fatal("preflight changed target entries")
				}
			}
			if name == "extra-target" {
				b, e := os.ReadFile(filepath.Join(target, ".npmrc"))
				if e != nil || string(b) != "keep" {
					t.Fatal("overwrote repository config")
				}
			}
			if name == "nonempty-home" {
				b, _ := os.ReadFile(filepath.Join(home, "global.npmrc"))
				if string(b) != "keep" {
					t.Fatal("overwrote config")
				}
			}
		})
	}
}
