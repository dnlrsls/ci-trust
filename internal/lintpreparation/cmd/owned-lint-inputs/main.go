// Command owned-lint-inputs emits compiled owned data without running JavaScript.
package main

import (
	"encoding/json"
	"os"

	"github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

func main() {
	if len(os.Args) != 1 {
		os.Exit(2)
	}
	inv, _ := lintsensitivity.OwnedESLintInvocation(lintsensitivity.Clean)
	fixtures := map[lintsensitivity.Role]lintsensitivity.Fixture{}
	for _, role := range []lintsensitivity.Role{lintsensitivity.Clean, lintsensitivity.Fault, lintsensitivity.Restored} {
		fixtures[role], _ = lintsensitivity.FixtureFor(role)
	}
	if json.NewEncoder(os.Stdout).Encode(struct {
		Module, ModuleSHA256, RecipeSHA256 string
		Args                               []string
		Fixtures                           map[lintsensitivity.Role]lintsensitivity.Fixture
	}{inv.Config, inv.ConfigSHA256, lintsensitivity.ConfigSHA256(), inv.Args, fixtures}) != nil {
		os.Exit(1)
	}
}
