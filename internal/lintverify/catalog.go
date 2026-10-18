// Package lintverify integrates fixed owned lint workflow services behind a
// compiled catalog. Planning data and receipts never grant admission.
package lintverify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"time"

	"github.com/dnlrsls/ci-trust/internal/lintcatalog"
	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

// Catalog and dependency seams are private: input files/CLI arguments cannot
// supply an admitted runtime, an approval boolean or a process callback.
type catalog struct {
	image, archive, layer, node, eslint string
	securityHold, admitted              bool
}

func productionCatalog() catalog {
	c := lintcatalog.Current()
	return catalog{image: c.Image, archive: c.Archive, layer: c.Layer, node: c.Node,
		eslint: c.ESLint, securityHold: c.SecurityHold, admitted: c.Admitted}
}
func (c catalog) blocked() string {
	if c.securityHold {
		return "NODE_SECURITY_HOLD"
	}
	if !c.admitted {
		return "LINT_IMAGE_NOT_ADMITTED"
	}
	return ""
}

type ownedCase struct {
	Role                 ls.Role
	FixtureSHA256, Stdin string
	Args                 [9]string
}
type plan struct {
	Identity                                                              ls.Identity
	ArchiveSHA256, LayerDiffID, ModuleSHA256, ParserRevision, NodeVersion string
	Cases                                                                 [3]ownedCase
	WallSeconds, OutputBytes, MemoryBytes, CPUs, PIDs, ScratchBytes       int
}

var sourceHash = regexp.MustCompile(`^[a-f0-9]{64}$`)

func planFor(c catalog, snapshot string) (plan, bool) {
	if !sourceHash.MatchString(snapshot) || !sourceHash.MatchString(trimDigest(c.image)) || !sourceHash.MatchString(trimDigest(c.archive)) || !sourceHash.MatchString(trimDigest(c.layer)) {
		return plan{}, false
	}
	p := plan{Identity: ls.Identity{Tool: "eslint", Version: c.eslint, ConfigSHA256: ls.ConfigSHA256(), Image: c.image, Environment: "owned-lint-v1", OS: "LINUX", Snapshot: snapshot}, ArchiveSHA256: c.archive, LayerDiffID: c.layer, ParserRevision: "eslint-receipt-json-v1", NodeVersion: c.node, WallSeconds: 10, OutputBytes: 64 << 10, MemoryBytes: 256 << 20, CPUs: 1, PIDs: 16, ScratchBytes: 16 << 20}
	for i, role := range []ls.Role{ls.Clean, ls.Fault, ls.Restored} {
		invocation, ok := ls.OwnedESLintInvocation(role)
		if !ok || len(invocation.Args) != 9 {
			return plan{}, false
		}
		fixture, _ := ls.FixtureFor(role)
		p.Cases[i] = ownedCase{Role: role, FixtureSHA256: fixture.SHA256, Stdin: fixture.Source}
		copy(p.Cases[i].Args[:], invocation.Args)
		p.ModuleSHA256 = invocation.ConfigSHA256
	}
	return p, true
}
func trimDigest(s string) string {
	if len(s) != 71 || s[:7] != "sha256:" {
		return ""
	}
	return s[7:]
}
func (p plan) digest() string {
	b, _ := json.Marshal(p)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Only gated authenticated review can mint a production decision. Tests use
// synthetic private decisions; a framed tool receipt is never such a decision.
type approval struct {
	binding string
	expires time.Time
}
type captureResult struct {
	capture         ls.ESLintCapture
	cleanupVerified bool
}
type dependencies struct {
	catalog   catalog
	review    func(context.Context, plan) (approval, error)
	preflight func(context.Context, plan) error
	capture   func(context.Context, plan, ownedCase, string) captureResult
	snapshot  func() (string, error)
	now       func() time.Time
}
type outcome struct {
	decision     ls.Result
	observations []ls.Observation
}

func unresolved(reason string) outcome {
	return outcome{decision: ls.Result{Outcome: ls.Unverifiable, Reason: reason, Scope: "TOOL_FIXTURE"}}
}
