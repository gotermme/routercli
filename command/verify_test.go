// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import "testing"

// TestVerifyCommandLevelsPassesWhenEverythingRegistered - This test verifies that
// a manifest whose enter_command and exit_command are both registered
// handlers produces no problems.
func TestVerifyCommandLevelsPassesWhenEverythingRegistered(t *testing.T) {
	registerTestHandlers()
	Register("test-verify-enter", func(*AppContext, []string) error { return nil })
	Register("test-verify-exit", func(*AppContext, []string) error { return nil })

	opTree := writeTree(t, "  show:\n    run: test.noop\n")
	execTree := writeTree(t, "  configure:\n    run: test.noop\n")
	common := emptyCommonTree(t)
	manifest := writeManifest(t, `
  operator:
    tree_file: `+opTree+`
    is_base: true
  exec:
    tree_file: `+execTree+`
    parent: operator
    enter_command: test-verify-enter
    exit_command: test-verify-exit
`)
	levels, err := LoadTreeStructure(manifest, common)
	if err != nil {
		t.Fatalf("LoadTreeStructure: %v", err)
	}

	if problems := VerifyCommandLevels(levels); len(problems) != 0 {
		t.Errorf("expected no problems, got %v", problems)
	}
}

// TestVerifyCommandLevelsCatchesMissingEnterCommand - This test
// verifies the rule that every non-base level must declare
// enter_command. LoadTreeStructure itself deliberately does not
// enforce this, only this separate verification pass does.
func TestVerifyCommandLevelsCatchesMissingEnterCommand(t *testing.T) {
	registerTestHandlers()
	opTree := writeTree(t, "  show:\n    run: test.noop\n")
	execTree := writeTree(t, "  configure:\n    run: test.noop\n")
	common := emptyCommonTree(t)
	manifest := writeManifest(t, `
  operator:
    tree_file: `+opTree+`
    is_base: true
  exec:
    tree_file: `+execTree+`
    parent: operator
`)
	levels, err := LoadTreeStructure(manifest, common)
	if err != nil {
		t.Fatalf("LoadTreeStructure: %v", err)
	}

	problems := VerifyCommandLevels(levels)
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem (missing enter_command), got %d: %v", len(problems), problems)
	}
}

// TestVerifyCommandLevelsCatchesUnregisteredEnterCommand - This test verifies the
// case where the manifest names a command nobody wrote. This is
// precisely the mistake this whole check exists to catch, a typo in
// the manifest or a forgotten cmd_*.go file, instead of it only
// surfacing the first time a user actually types the command.
func TestVerifyCommandLevelsCatchesUnregisteredEnterCommand(t *testing.T) {
	registerTestHandlers()
	opTree := writeTree(t, "  show:\n    run: test.noop\n")
	execTree := writeTree(t, "  configure:\n    run: test.noop\n")
	common := emptyCommonTree(t)
	manifest := writeManifest(t, `
  operator:
    tree_file: `+opTree+`
    is_base: true
  exec:
    tree_file: `+execTree+`
    parent: operator
    enter_command: this-was-never-registered-anywhere
`)
	levels, err := LoadTreeStructure(manifest, common)
	if err != nil {
		t.Fatalf("LoadTreeStructure: %v", err)
	}

	problems := VerifyCommandLevels(levels)
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem (unregistered enter_command), got %d: %v", len(problems), problems)
	}
}

// TestVerifyCommandLevelsCatchesUnregisteredExitCommand - This test verifies that
// a declared exit_command naming a handler nobody registered is
// caught, the same as an unregistered enter_command.
func TestVerifyCommandLevelsCatchesUnregisteredExitCommand(t *testing.T) {
	registerTestHandlers()
	Register("test-verify-exit-only-enter", func(*AppContext, []string) error { return nil })

	opTree := writeTree(t, "  show:\n    run: test.noop\n")
	execTree := writeTree(t, "  configure:\n    run: test.noop\n")
	common := emptyCommonTree(t)
	manifest := writeManifest(t, `
  operator:
    tree_file: `+opTree+`
    is_base: true
  exec:
    tree_file: `+execTree+`
    parent: operator
    enter_command: test-verify-exit-only-enter
    exit_command: this-was-never-registered-either
`)
	levels, err := LoadTreeStructure(manifest, common)
	if err != nil {
		t.Fatalf("LoadTreeStructure: %v", err)
	}

	problems := VerifyCommandLevels(levels)
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem (unregistered exit_command), got %d: %v", len(problems), problems)
	}
}

// TestVerifyCommandLevelsSkipsTheBaseLevel - This test verifies that a base level,
// which has no EnterCommand by definition, is never reported as a
// problem for lacking one.
func TestVerifyCommandLevelsSkipsTheBaseLevel(t *testing.T) {
	registerTestHandlers()
	opTree := writeTree(t, "  show:\n    run: test.noop\n")
	common := emptyCommonTree(t)
	manifest := writeManifest(t, `
  operator:
    tree_file: `+opTree+`
    is_base: true
`)
	levels, err := LoadTreeStructure(manifest, common)
	if err != nil {
		t.Fatalf("LoadTreeStructure: %v", err)
	}

	if problems := VerifyCommandLevels(levels); len(problems) != 0 {
		t.Errorf("expected no problems for a base only manifest, got %v", problems)
	}
}

// TestVerifyCommandLevelsExitCommandIsOptional - This test verifies that a level
// with no exit_command declared at all, left through the generic
// "exit" or "end" instead, is never reported as a problem.
func TestVerifyCommandLevelsExitCommandIsOptional(t *testing.T) {
	registerTestHandlers()
	Register("test-verify-no-exit-cmd", func(*AppContext, []string) error { return nil })

	opTree := writeTree(t, "  show:\n    run: test.noop\n")
	nestedTree := writeTree(t, "  hostname:\n    run: test.noop\n")
	common := emptyCommonTree(t)
	manifest := writeManifest(t, `
  operator:
    tree_file: `+opTree+`
    is_base: true
  config:
    tree_file: `+nestedTree+`
    parent: operator
    enter_command: test-verify-no-exit-cmd
`)
	levels, err := LoadTreeStructure(manifest, common)
	if err != nil {
		t.Fatalf("LoadTreeStructure: %v", err)
	}

	if problems := VerifyCommandLevels(levels); len(problems) != 0 {
		t.Errorf("expected no problems when exit_command is simply omitted, got %v", problems)
	}
}
