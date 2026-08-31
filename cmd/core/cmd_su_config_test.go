// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"testing"

	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"
)

// execSuConfigLevels - This function builds a minimal
// *command.TreeStructure with an "exec" level and a "su-config" level
// whose Parent is "exec", the same shape var/tree/tree_structure.yaml
// declares, for tests that need ctx.Levels.ByName["su-config"]
// populated without loading real YAML files. Neither level carries a
// PasswordHash, matching su-config's own shipped default of being
// open, so these tests exercise the mechanical enter and exit
// behavior, not the password prompt or trust window paths, which are
// already covered by command/treestructure_test.go.
func execSuConfigLevels() *command.TreeStructure {
	return &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"exec":      {Name: "exec", Tree: map[string]*command.Command{}},
		"su-config": {Name: "su-config", Parent: "exec", PromptSuffix: "(su-config)", Tree: map[string]*command.Command{}},
	}}
}

// TestSuConfigHandlerEntersFromExec - This test verifies that the
// registered "su-config" handler moves a session from exec into
// su-config, swapping the root frame and updating
// Session.CommandLevel.
func TestSuConfigHandlerEntersFromExec(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = execSuConfigLevels()
	ctx.Session = &auth.Session{CommandLevel: "exec"}
	ctx.Position = command.NewCommandLevelStack("exec", "", map[string]*command.Command{})
	cmd := loadTestCommand(t, "su-config")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Fatalf("su-config handler returned unexpected error: %v", err)
	}
	if ctx.Session.CommandLevel != "su-config" {
		t.Errorf("Session.CommandLevel = %q, want %q", ctx.Session.CommandLevel, "su-config")
	}
	if ctx.Position.Current().PromptSuffix != "(su-config)" {
		t.Errorf("Position.Current().PromptSuffix = %q, want %q", ctx.Position.Current().PromptSuffix, "(su-config)")
	}
}

// TestSuConfigHandlerRefusesFromBase - This test verifies that
// "su-config" is refused when the session is not currently at
// su-config's own parent, exec, the same RequireCurrentCommandLevel
// check every root swap Command Level enforces.
func TestSuConfigHandlerRefusesFromBase(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = execSuConfigLevels()
	ctx.Session = &auth.Session{CommandLevel: ""}
	ctx.Position = command.NewCommandLevelStack("base", "", map[string]*command.Command{})
	cmd := loadTestCommand(t, "su-config")

	if err := cmd.RunFunc(ctx, nil); err == nil {
		t.Fatal("expected an error entering su-config from base, got nil")
	}
	if ctx.Session.CommandLevel == "su-config" {
		t.Error("Session.CommandLevel should not have changed on a refused entry")
	}
}

// TestSuConfigHandlerAlreadyHereIsNoOp - This test verifies that
// running "su-config" again while already inside it leaves the
// session exactly where it was and still returns no error, the
// "already here" outcome rather than a failure.
func TestSuConfigHandlerAlreadyHereIsNoOp(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = execSuConfigLevels()
	ctx.Session = &auth.Session{CommandLevel: "su-config"}
	ctx.Position = command.NewCommandLevelStack("su-config", "(su-config)", map[string]*command.Command{})
	cmd := loadTestCommand(t, "su-config")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Fatalf("su-config handler returned unexpected error: %v", err)
	}
	if ctx.Session.CommandLevel != "su-config" {
		t.Errorf("Session.CommandLevel = %q, want %q (unchanged)", ctx.Session.CommandLevel, "su-config")
	}
}

// TestExitSuConfigHandlerReturnsToExec - This test verifies that
// "exit-su-config" moves a session back from su-config to exec.
func TestExitSuConfigHandlerReturnsToExec(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = execSuConfigLevels()
	ctx.Session = &auth.Session{CommandLevel: "su-config"}
	ctx.Position = command.NewCommandLevelStack("su-config", "(su-config)", map[string]*command.Command{})
	cmd := loadTestCommand(t, "exit-su-config")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Fatalf("exit-su-config handler returned unexpected error: %v", err)
	}
	if ctx.Session.CommandLevel != "exec" {
		t.Errorf("Session.CommandLevel = %q, want %q", ctx.Session.CommandLevel, "exec")
	}
}

// TestExitSuConfigHandlerAtExecIsNoOp - This test verifies that
// running "exit-su-config" while not actually inside su-config is a
// no-op returning no error.
func TestExitSuConfigHandlerAtExecIsNoOp(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = execSuConfigLevels()
	ctx.Session = &auth.Session{CommandLevel: "exec"}
	ctx.Position = command.NewCommandLevelStack("exec", "", map[string]*command.Command{})
	cmd := loadTestCommand(t, "exit-su-config")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Fatalf("exit-su-config handler returned unexpected error: %v", err)
	}
	if ctx.Session.CommandLevel != "exec" {
		t.Errorf("Session.CommandLevel = %q, want %q (unchanged)", ctx.Session.CommandLevel, "exec")
	}
}
