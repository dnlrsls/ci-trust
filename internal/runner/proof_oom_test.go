package runner

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

type proofOOMClient struct {
	ownedClient
	output        string
	code          int
	attachFailure bool
}

func (f *proofOOMClient) call(ctx context.Context, w io.Writer, args ...string) (string, error) {
	if args[0] == "start" {
		f.calls = append(f.calls, append([]string(nil), args...))
		_, err := io.WriteString(w, f.output)
		if f.attachFailure {
			return "", errors.New("owned transport failure")
		}
		return "", err
	}
	s, err := f.ownedClient.call(ctx, w, args...)
	if args[0] == "inspect" && err == nil {
		var rows []map[string]any
		if json.Unmarshal([]byte(s), &rows) != nil {
			return "", errors.New("owned fixture decode")
		}
		rows[0]["State"].(map[string]any)["ExitCode"] = f.code
		b, err := json.Marshal(rows)
		return string(b), err
	}
	return s, err
}
func completedProofFixture(t *testing.T) string {
	t.Helper()
	checks := []map[string]any{}
	for _, name := range []string{"nonroot", "capabilities", "no_new_privileges", "seccomp", "root_readonly", "scratch_write_cleanup", "scratch_limit", "network_unreachable", "host_credentials_absent", "cpu_limit", "memory_limit", "pids_limit"} {
		c := map[string]any{"name": name, "state": "OBSERVED", "detail": "owned bounded fixture observation", "before": nil, "after": nil}
		if name == "cpu_limit" || name == "memory_limit" || name == "pids_limit" {
			c["before"] = 1
			c["after"] = 2
		}
		checks = append(checks, c)
	}
	b, err := json.Marshal(map[string]any{"version": 1, "scope": "owned-smoke-v1", "checks": checks, "limitations": []string{"owned observations only"}})
	if err != nil {
		t.Fatal(err)
	}
	return "owned-smoke: proof-started\nowned-smoke: owned stderr\n" + string(b) + "\n"
}
func TestExpectedOwnedChildOOMRequiresCompletedParentEvidence(t *testing.T) {
	for _, name := range []string{"owned-child", "legacy", "parent-killed", "absent-report", "unavailable-memory", "unchanged-counter", "duplicate-check", "attach-failure"} {
		t.Run(name, func(t *testing.T) {
			p := owned()
			p.Config = "owned-proof-v1"
			p.Limits.OutputBytes = 64 << 10
			f := &proofOOMClient{ownedClient: ownedClient{oom: true}, output: completedProofFixture(t)}
			switch name {
			case "legacy":
				p.Config = "owned-v1"
			case "parent-killed":
				f.code = 137
			case "absent-report":
				f.output = "owned-smoke: proof-started\n"
			case "unavailable-memory":
				f.output = strings.Replace(f.output, `"name":"memory_limit","state":"OBSERVED"`, `"name":"memory_limit","state":"UNVERIFIABLE"`, 1)
			case "unchanged-counter":
				f.output = strings.ReplaceAll(f.output, `"after":2`, `"after":1`)
			case "duplicate-check":
				f.output = strings.Replace(f.output, `"name":"cpu_limit"`, `"name":"memory_limit"`, 1)
			case "attach-failure":
				f.attachFailure = true
			}
			d := &LocalDocker{client: f, host: "linux"}
			r, err := NewOwned(d, p, func(context.Context) (string, error) { return p.Snapshot, nil })
			if err != nil {
				t.Fatal(err)
			}
			result := r.Execute(context.Background(), p, Consent{Approved: true, Digest: p.Digest()})
			if result.OOMEvent == nil || !*result.OOMEvent {
				t.Fatal("independently inspected cgroup event was lost")
			}
			if name == "owned-child" {
				if result.Outcome != Completed || result.ExitCode != 0 || !result.CleanupVerified {
					t.Fatalf("expected child event rejected: %+v", result)
				}
			} else if result.Outcome != Unverifiable || !result.CleanupVerified {
				t.Fatalf("ambiguous/failed parent accepted: %+v", result)
			}
		})
	}
}
