package runner

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

var lintHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

// FixedLintPlan is inert owned data. binding is the complete lint plan digest,
// not an approval token; no command, image or resource override is accepted.
func FixedLintPlan(snapshot string, role ls.Role, binding string) (Plan, error) {
	inv, ok := ls.OwnedESLintInvocation(role)
	if !ok || !lintHash.MatchString(snapshot) || !lintHash.MatchString(binding) {
		return Plan{}, errors.New("invalid fixed lint identity")
	}
	c := lintcatalog.Current()
	return Plan{Snapshot: snapshot, Adapter: "owned-eslint-v1", Tool: "eslint@" + c.ESLint, Image: c.Image, Fixture: string(role), Config: "owned-eslint-module:" + inv.ConfigSHA256 + ":binding:" + binding,
		Limits: Limits{Wall: 10 * time.Second, MemoryBytes: 256 << 20, CPUs: 1, PIDs: 16, OutputBytes: 64 << 10, ScratchBytes: 16 << 20}, Effects: "owned fixture only; no host mounts; no network", Transport: splitStdin}, nil
}

// NewLintOwned is trusted application composition, never approval from input.
// The compiled gate is checked before any runtime or snapshot dependency.
func NewLintOwned(rt Runtime, p Plan, snapshot Snapshot) (*Runner, error) {
	if reason := lintcatalog.BlockedReason(); reason != "" {
		return nil, errors.New(reason)
	}
	return newLintOwned(rt, p, snapshot)
}
func newLintOwned(rt Runtime, p Plan, snapshot Snapshot) (*Runner, error) {
	if rt == nil || snapshot == nil || !validLint(p) {
		return nil, errors.New("invalid fixed lint plan")
	}
	return &Runner{runtime: rt, owned: p, snapshot: snapshot, lint: true}, nil
}
func lintInfo(p Plan) (ls.Role, string, bool) {
	if len(p.Config) < 64 {
		return "", "", false
	}
	binding := p.Config[len(p.Config)-64:]
	for _, role := range []ls.Role{ls.Clean, ls.Fault, ls.Restored} {
		q, e := FixedLintPlan(p.Snapshot, role, binding)
		if e == nil && p == q {
			return role, binding, true
		}
	}
	return "", "", false
}
func validLint(p Plan) bool { _, _, ok := lintInfo(p); return ok }
func lintDockerArgs(p Plan, name string) []string {
	role, binding, _ := lintInfo(p)
	l := p.Limits
	return []string{"run", "--interactive", "--name", name, "--pull=never", "--network=none", "--read-only", "--user=65532:65532", "--workdir=/work", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--memory", fmt.Sprint(l.MemoryBytes), "--memory-swap", fmt.Sprint(l.MemoryBytes), "--cpus", fmt.Sprint(l.CPUs), "--pids-limit", fmt.Sprint(l.PIDs), "--tmpfs", fmt.Sprintf("/tmp:rw,noexec,nosuid,nodev,size=%d", l.ScratchBytes), "--entrypoint", "/ci-trust/owned-eslint", p.Image, "--role", string(role), "--binding", binding}
}
