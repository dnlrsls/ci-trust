package main

import (
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/dnlrsls/ci-trust/internal/lintsensitivity"
)

func TestEmittedOwnedInputsAreSelfConsistent(t *testing.T) {
	clean, _ := lintsensitivity.OwnedESLintInvocation(lintsensitivity.Clean)
	if fmt.Sprintf("%x", sha256.Sum256([]byte(clean.Config))) != clean.ConfigSHA256 {
		t.Fatal("module digest")
	}
	for _, role := range []lintsensitivity.Role{lintsensitivity.Clean, lintsensitivity.Fault, lintsensitivity.Restored} {
		inv, ok := lintsensitivity.OwnedESLintInvocation(role)
		fixture, _ := lintsensitivity.FixtureFor(role)
		if !ok || inv.Config != clean.Config || inv.Stdin != fixture.Source || fmt.Sprintf("%x", sha256.Sum256([]byte(fixture.Source))) != fixture.SHA256 {
			t.Fatal("owned role mismatch")
		}
	}
}
