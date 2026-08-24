// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"testing"

	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"
)

// TestPasswordManagerNegatedClearsPasswordHash - This test verifies
// that running "password manager" with ctx.Negated set, the "no
// password manager" path, clears the current Command Level's own
// PasswordHash back to empty, restoring the default of entering that
// level without a prompt. The positive path, which prompts for a new
// secret through auth.PromptSecret, needs a real terminal file
// descriptor and is covered only by the pty based interactive smoke
// test, the same accepted limitation every other secret prompt in
// this project has.
func TestPasswordManagerNegatedClearsPasswordHash(t *testing.T) {
	ctx := newTestContext()
	level := &command.CommandLevel{Name: "exec", PasswordHash: "$0$already-set"}
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{"exec": level}}
	ctx.Session = &auth.Session{CommandLevel: "exec"}
	ctx.Negated = true
	cmd := loadTestCommand(t, "password-manager")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Fatalf("password-manager handler returned unexpected error: %v", err)
	}
	if level.PasswordHash != "" {
		t.Errorf("PasswordHash = %q after \"no password manager\", want empty", level.PasswordHash)
	}
}

// TestPasswordManagerErrorsWhenCurrentLevelNotFound - This test
// verifies that "password manager" refuses, rather than panicking on
// a nil CommandLevel, when ctx.Session.CommandLevel names a level not
// present in ctx.Levels.ByName at all, a state the handler's own doc
// comment says should not happen in practice but is checked for
// anyway.
func TestPasswordManagerErrorsWhenCurrentLevelNotFound(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{}}
	ctx.Session = &auth.Session{CommandLevel: "nonexistent"}
	cmd := loadTestCommand(t, "password-manager")

	if err := cmd.RunFunc(ctx, nil); err == nil {
		t.Fatal("expected an error when ctx.Session.CommandLevel names an unknown level, got nil")
	}
}
