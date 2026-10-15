package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"
)

var program, dockerFixture string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "ci-preflight-build-")
	if err != nil {
		panic(err)
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	program = filepath.Join(dir, "preflight"+ext)
	dockerFixture = filepath.Join(dir, "docker"+ext)
	build := func(out, src string) {
		c := exec.Command("go", "build", "-o", out, src)
		if b, e := c.CombinedOutput(); e != nil {
			panic(fmt.Sprintf("fixture build: %s %v", b, e))
		}
	}
	build(program, ".")
	build(dockerFixture, "./testdata/fakedocker.go")
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

const suffix = `,"missing":["production backend and immutable adapter catalog","authenticated current-plan consent boundary","real isolation, resource, cancellation and cleanup proof"]}` + "\n"
const runtimeJSON = `{"version":"28.3.3","os":"linux","architecture":"amd64","reported_cgroup_version":"2","reported_cpus":4,"reported_memory_bytes":8589934592,"reported_memory_limit":true,"reported_swap_limit":true,"reported_pids_limit":true,"reported_cpu_quota":true}`

func TestPublicPreparation(t *testing.T) {
	cases := []struct {
		name, mode, reason  string
		args                []string
		code, calls         int
		metadata, badConfig bool
		stdout, stderr      string
	}{
		{name: "metadata-is-not-proof", mode: "metadata", metadata: true, calls: 2, reason: "metadata only; enforcement and daemon host scope not attested"},
		{name: "unavailable", mode: "unavailable", calls: 1, reason: "version metadata unavailable"},
		{name: "malformed", mode: "malformed", calls: 1, reason: "invalid Docker metadata"},
		{name: "bounded-output", mode: "oversize", calls: 1, reason: "version metadata unavailable"},
		{name: "bounded-time", mode: "timeout", calls: 1, reason: "version metadata unavailable"},
		{name: "descendant-output-handles", mode: "pipes", calls: 1, reason: "version metadata unavailable"},
		{name: "stderr-budget", mode: "stderr-budget", calls: 1, reason: "version metadata unavailable"},
		{name: "nonempty-config", mode: "metadata", badConfig: true, reason: "client configuration is not an empty directory"},
		{name: "invalid-argument", mode: "metadata", args: []string{"--approve"}, code: 2, stderr: "usage: cipreflight [--help]\nMetadata only; never starts containers or requests consent.\n"},
		{name: "help", mode: "metadata", args: []string{"--help"}, stdout: "usage: cipreflight [--help]\nMetadata only; never starts containers or requests consent.\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			bin := filepath.Join(root, "bin")
			cfg := filepath.Join(root, ".git", "ci-trust-preflight-empty")
			if e := os.MkdirAll(bin, 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.MkdirAll(cfg, 0700); e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(dockerFixture)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(bin, filepath.Base(dockerFixture)), b, 0700); e != nil {
				t.Fatal(e)
			}
			if e = os.WriteFile(filepath.Join(root, "scenario"), []byte(tc.mode), 0600); e != nil {
				t.Fatal(e)
			}
			if tc.badConfig {
				if e = os.WriteFile(filepath.Join(cfg, "credential"), []byte("SECRET-MARKER"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if tc.mode == "pipes" {
				done := filepath.Join(filepath.Dir(dockerFixture), "descendant-done")
				spec, e := json.Marshal(struct{ Executable, Done string }{dockerFixture, done})
				if e != nil {
					t.Fatal(e)
				}
				if e = os.WriteFile(filepath.Join(root, "child-spec"), spec, 0600); e != nil {
					t.Fatal(e)
				}
				t.Cleanup(func() {
					deadline := time.Now().Add(3 * time.Second)
					for {
						if _, e := os.Stat(done); e == nil {
							return
						}
						if time.Now().After(deadline) {
							t.Error("owned descendant cleanup not observed")
							return
						}
						time.Sleep(20 * time.Millisecond)
					}
				})
			}
			before := snapshot(t, root)
			c := exec.Command(program, tc.args...)
			c.Dir = root
			// A hostile inherited Docker context, credentials and proxy must not reach the client.
			c.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"), "GITHUB_SHA="+strings.Repeat("a", 40), "GITHUB_TOKEN=SECRET-MARKER", "DOCKER_HOST=tcp://SECRET-MARKER", "DOCKER_CONTEXT=SECRET-MARKER", "HTTP_PROXY=SECRET-MARKER")
			var out, err bytes.Buffer
			c.Stdout = &out
			c.Stderr = &err
			e = c.Run()
			code := 0
			if e != nil {
				var ok bool
				var x *exec.ExitError
				x, ok = e.(*exec.ExitError)
				if !ok {
					t.Fatal(e)
				}
				code = x.ExitCode()
			}
			want := tc.stdout
			if len(tc.args) == 0 {
				r := "null"
				if tc.metadata {
					r = runtimeJSON
				}
				want = fmt.Sprintf(`{"status":"UNVERIFIABLE","isolation_proof":false,"executable_plan":false,"metadata_captured":%t,"source_revision":"%s","endpoint":"unix:///var/run/docker.sock","runtime":%s,"reason":%q`, tc.metadata, strings.Repeat("a", 40), r, tc.reason) + suffix
			}
			if code != tc.code || out.String() != want || err.String() != tc.stderr {
				t.Fatalf("exit=%d want=%d\nstdout=%q\nwant=%q\nstderr=%q want=%q", code, tc.code, out.String(), want, err.String(), tc.stderr)
			}
			raw, e := os.ReadFile(filepath.Join(root, "calls"))
			if os.IsNotExist(e) {
				raw = nil
			} else if e != nil {
				t.Fatal(e)
			}
			lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
			if len(raw) == 0 {
				lines = nil
			}
			if len(lines) != tc.calls {
				t.Fatalf("calls=%d want=%d: %s", len(lines), tc.calls, raw)
			}
			wantEnv := []string{"PATH=" + bin + string(os.PathListSeparator) + os.Getenv("PATH"), "LANG=C"}
			if runtime.GOOS == "windows" {
				wantEnv = append(wantEnv, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"))
			}
			sort.Strings(wantEnv)
			for i, line := range lines {
				var call struct{ Args, Env []string }
				if e = json.Unmarshal(line, &call); e != nil {
					t.Fatal(e)
				}
				command, format := "version", "{{json .Server}}"
				if i == 1 {
					command, format = "info", "{{json .}}"
				}
				wantArgs := []string{"--host=unix:///var/run/docker.sock", "--config=.git/ci-trust-preflight-empty", command, "--format", format}
				if fmt.Sprint(call.Args) != fmt.Sprint(wantArgs) {
					t.Fatalf("unsafe arguments: %q", call.Args)
				}
				sort.Strings(call.Env)
				if fmt.Sprint(call.Env) != fmt.Sprint(wantEnv) {
					t.Fatalf("client environment differs from complete allowlist: got %q want %q", call.Env, wantEnv)
				}
			}
			if strings.Contains(out.String()+err.String(), "SECRET-MARKER") {
				t.Fatal("unallowlisted metadata leaked")
			}
			after := snapshot(t, root)
			if fmt.Sprint(before) != fmt.Sprint(after) {
				t.Fatalf("source/config changed: %v => %v", before, after)
			}
		})
	}
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	e := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		if rel == "calls" {
			return nil
		}
		if d.IsDir() {
			m[rel] = "directory"
			return nil
		}
		b, e := os.ReadFile(p)
		if e == nil {
			m[rel] = string(b)
		}
		return e
	})
	if e != nil {
		t.Fatal(e)
	}
	return m
}
