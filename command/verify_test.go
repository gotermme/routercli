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

// ----------------------------------------------------------------------
//
// VerifyVendorDefinedSecrets
//
// ----------------------------------------------------------------------

// TestVerifyVendorDefinedSecretsPassesWhenNoneSet - This test verifies
// that an ordinary manifest, nobody using
// VendorDefinedPasswordHash at all, produces no problems, the common
// case every existing tree file in this project is in today.
func TestVerifyVendorDefinedSecretsPassesWhenNoneSet(t *testing.T) {
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

	if problems := VerifyVendorDefinedSecrets(levels); len(problems) != 0 {
		t.Errorf("expected no problems, got %v", problems)
	}
}

// TestVerifyVendorDefinedSecretsPassesLevelFullyValid - This test
// verifies that a level meeting all three rules at once, Hidden true,
// PasswordUserSettable left nil, and no PasswordHash also set,
// produces no problems.
func TestVerifyVendorDefinedSecretsPassesLevelFullyValid(t *testing.T) {
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

	levels.ByName["operator"].VendorDefinedPasswordHash = "$6$$vendorhash"
	levels.ByName["operator"].Hidden = true

	if problems := VerifyVendorDefinedSecrets(levels); len(problems) != 0 {
		t.Errorf("expected no problems for a fully valid vendor defined level, got %v", problems)
	}
}

// TestVerifyVendorDefinedSecretsCatchesLevelBothHashesSet - This test
// verifies rule 1: a level must not set both PasswordHash and
// VendorDefinedPasswordHash at once, since EffectivePasswordHash would
// then silently make PasswordHash dead configuration.
func TestVerifyVendorDefinedSecretsCatchesLevelBothHashesSet(t *testing.T) {
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

	level := levels.ByName["operator"]
	level.PasswordHash = "$6$$ordinary"
	level.VendorDefinedPasswordHash = "$6$$vendorhash"
	level.Hidden = true // isolate the "both set" rule from the "not hidden" rule

	problems := VerifyVendorDefinedSecrets(levels)
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem (both hashes set), got %d: %v", len(problems), problems)
	}
}

// TestVerifyVendorDefinedSecretsCatchesLevelUserSettableTrue - This
// test verifies rule 2: a level must not set PasswordUserSettable
// true alongside a VendorDefinedPasswordHash.
func TestVerifyVendorDefinedSecretsCatchesLevelUserSettableTrue(t *testing.T) {
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

	level := levels.ByName["operator"]
	level.VendorDefinedPasswordHash = "$6$$vendorhash"
	level.Hidden = true // isolate the "user settable true" rule from the "not hidden" rule
	yes := true
	level.PasswordUserSettable = &yes

	problems := VerifyVendorDefinedSecrets(levels)
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem (password_user_settable: true), got %d: %v", len(problems), problems)
	}
}

// TestVerifyVendorDefinedSecretsCatchesLevelNotHidden - This test
// verifies rule 3: a level must set Hidden true alongside a
// VendorDefinedPasswordHash.
func TestVerifyVendorDefinedSecretsCatchesLevelNotHidden(t *testing.T) {
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

	levels.ByName["operator"].VendorDefinedPasswordHash = "$6$$vendorhash"
	// Hidden left at its zero value, false.

	problems := VerifyVendorDefinedSecrets(levels)
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem (not hidden), got %d: %v", len(problems), problems)
	}
}

// TestVerifyVendorDefinedSecretsCatchesCommandViolations - This test
// verifies that the same three rules apply to an individual Command
// reachable inside a level's Tree, not just to the level itself,
// mirroring TestVerifyVendorDefinedSecretsCatchesLevelBothHashesSet
// and its siblings above but for Command.Hidden instead of
// CommandLevel.Hidden.
func TestVerifyVendorDefinedSecretsCatchesCommandViolations(t *testing.T) {
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

	show := levels.ByName["operator"].Tree["show"]
	show.VendorDefinedPasswordHash = "$6$$vendorhash"
	// Hidden left false, PasswordUserSettable left nil (so only the
	// "not hidden" rule fires here, matching the level level test
	// above's isolation approach).

	problems := VerifyVendorDefinedSecrets(levels)
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem (command not hidden), got %d: %v", len(problems), problems)
	}
}

// TestVerifyVendorDefinedSecretsDedupesSharedCommand - This test
// verifies that a command reachable through more than one level's
// merged tree, via InheritParent, the identical *Command pointer in
// each, see MergeTrees's own doc comment, is only ever reported once,
// not once per level that happens to inherit it.
func TestVerifyVendorDefinedSecretsDedupesSharedCommand(t *testing.T) {
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
    inherit_parent: true
`)
	levels, err := LoadTreeStructure(manifest, common)
	if err != nil {
		t.Fatalf("LoadTreeStructure: %v", err)
	}

	// "show" is defined once, in operator's own tree file, and
	// reachable from both operator and exec, since exec inherits.
	show := levels.ByName["operator"].Tree["show"]
	if levels.ByName["exec"].Tree["show"] != show {
		t.Fatal("test setup error: expected exec to inherit the identical *Command pointer for \"show\"")
	}
	show.VendorDefinedPasswordHash = "$6$$vendorhash"
	// Hidden left false, so exactly one rule fires, and the dedupe
	// this test is actually about is whether it fires once or twice.

	problems := VerifyVendorDefinedSecrets(levels)
	if len(problems) != 1 {
		t.Fatalf("expected exactly 1 problem, a shared command reported once regardless of how many levels reach it, got %d: %v", len(problems), problems)
	}
}
