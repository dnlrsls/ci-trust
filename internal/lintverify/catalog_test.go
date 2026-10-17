package lintverify

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	ls "github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

func TestProductionGatePrecedesEveryExecutionDependency(t *testing.T) {
	for _, reason := range []string{"NODE_SECURITY_HOLD", "LINT_IMAGE_NOT_ADMITTED"} {
		t.Run(reason, func(t *testing.T) {
			c := productionCatalog()
			if reason == "LINT_IMAGE_NOT_ADMITTED" {
				c.securityHold = false
			}
			calls := 0
			d := dependencies{catalog: c,
				review:    func(context.Context, plan) (approval, error) { calls++; return approval{}, nil },
				preflight: func(context.Context, plan) error { calls++; return nil },
				capture:   func(context.Context, plan, ownedCase, string) captureResult { calls++; return captureResult{} },
				snapshot:  func() (string, error) { calls++; return strings.Repeat("a", 64), nil },
				now:       func() time.Time { calls++; return time.Unix(0, 0) },
			}
			before := c
			got := evaluate(context.Background(), strings.Repeat("a", 64), d)
			if got.decision.Outcome != ls.Unverifiable || got.decision.Reason != reason || got.decision.Scope != "TOOL_FIXTURE" || len(got.observations) != 0 || calls != 0 || c != before {
				t.Fatalf("gate crossed: decision=%+v observations=%d calls=%d", got.decision, len(got.observations), calls)
			}
		})
	}
}

func TestOwnedPlanBindsOnlyFixedFixtureData(t *testing.T) {
	c := productionCatalog()
	p, ok := planFor(c, strings.Repeat("a", 64))
	if !ok {
		t.Fatal("owned descriptor unavailable")
	}
	if p.Identity.Image != c.image || p.Identity.Tool != "eslint" || p.Identity.Version != "10.12.0" || p.Identity.OS != "LINUX" || p.Identity.Snapshot != strings.Repeat("a", 64) || p.Identity.ConfigSHA256 != ls.ConfigSHA256() || p.NodeVersion != "24.22.0" {
		t.Fatal("identity lost", p)
	}
	for i, role := range []ls.Role{ls.Clean, ls.Fault, ls.Restored} {
		inv, _ := ls.OwnedESLintInvocation(role)
		fixture, _ := ls.FixtureFor(role)
		item := p.Cases[i]
		if item.Role != role || item.Stdin != fixture.Source || item.FixtureSHA256 != fixture.SHA256 || !reflect.DeepEqual(item.Args[:], inv.Args) || p.ModuleSHA256 != inv.ConfigSHA256 {
			t.Fatal("owned role/config/arguments changed", role)
		}
	}
	if p.WallSeconds != 10 || p.OutputBytes != 64<<10 || p.MemoryBytes != 256<<20 || p.CPUs != 1 || p.PIDs != 16 || p.ScratchBytes != 16<<20 {
		t.Fatal("limits missing")
	}
	q, _ := planFor(c, strings.Repeat("a", 64))
	if q.digest() != p.digest() || len(p.digest()) != 64 {
		t.Fatal("unstable binding")
	}
	for _, mutate := range []func(*plan){
		func(p *plan) { p.Identity.Snapshot = strings.Repeat("b", 64) },
		func(p *plan) { p.Identity.Image = "sha256:" + strings.Repeat("b", 64) },
		func(p *plan) { p.ModuleSHA256 = strings.Repeat("b", 64) },
		func(p *plan) { p.Cases[1].Stdin = p.Cases[0].Stdin },
		func(p *plan) { p.Cases[0].Args[0] = "--fix" },
		func(p *plan) { p.OutputBytes++ },
	} {
		changed := p
		mutate(&changed)
		if changed.digest() == p.digest() {
			t.Fatal("unbound plan mutation")
		}
	}
	if p != q {
		t.Fatal("copy mutation changed original plan")
	}
}

func TestInvalidSnapshotHasNoPlan(t *testing.T) {
	for _, snapshot := range []string{"", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("a", 64) + "\n"} {
		if p, ok := planFor(productionCatalog(), snapshot); ok || p != (plan{}) {
			t.Fatalf("invalid snapshot accepted %q", snapshot)
		}
	}
}
