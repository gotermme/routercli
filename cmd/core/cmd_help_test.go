// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"strings"
	"testing"

	"github.com/gotermme/routercli/command"
)

// TestHelpHandlerListsCurrentPositionTree - This test verifies that
// the registered "help" handler builds its listing from
// ctx.Position.Current().Tree, the session's current mode, not from
// ctx.Levels or any other tree, by giving it a tree with one known
// command and confirming command.HelpText actually reflects it in the
// printed output. This is what makes "help" show different output
// depending on which mode a session is currently in.
func TestHelpHandlerListsCurrentPositionTree(t *testing.T) {
	ctx := newTestContext()
	tree := map[string]*command.Command{
		"show-me-in-help": {Desc: "A marker command only this test's tree defines"},
	}
	ctx.Position = command.NewCommandLevelStack("exec", "", tree)
	cmd := loadTestCommand(t, "help")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Errorf("help handler returned unexpected error: %v", runErr)
	}
	if !strings.Contains(out, "show-me-in-help") {
		t.Errorf("help output = %q, expected it to contain %q", out, "show-me-in-help")
	}
}

// TestHelpHandlerWithCommandNameShowsHelpForPath - This test verifies
// that "help" typed with a command name after it prints exactly what
// command.HelpForPath itself returns for that same path, the same
// text "<command> ?" already shows, rather than falling back to the
// full current-level listing the way it did before this behavior was
// added.
func TestHelpHandlerWithCommandNameShowsHelpForPath(t *testing.T) {
	ctx := newTestContext()
	minArgs := 2
	tree := map[string]*command.Command{
		"greet": {ArgHelp: "<name> <greeting>  Say something to someone.", MinArgs: &minArgs},
	}
	ctx.Position = command.NewCommandLevelStack("exec", "", tree)
	cmd := loadTestCommand(t, "help")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, []string{"greet"}) })
	if runErr != nil {
		t.Fatalf("help handler returned unexpected error: %v", runErr)
	}
	want := command.HelpForPath(tree, []string{"greet"}, ctx.Translator, ctx.ListOptions)
	if out != want {
		t.Errorf("help greet output = %q, want %q (command.HelpForPath's own output)", out, want)
	}
}

// TestHelpHandlerWithUnknownCommandNameReturnsError - This test
// verifies that "help" typed with a command name that resolves to
// nothing at all is refused with an error, rather than printing
// nothing silently, so a typo is visible as a typo instead of an
// empty, unexplained line.
func TestHelpHandlerWithUnknownCommandNameReturnsError(t *testing.T) {
	ctx := newTestContext()
	tree := map[string]*command.Command{
		"show-me-in-help": {Desc: "A marker command only this test's tree defines"},
	}
	ctx.Position = command.NewCommandLevelStack("exec", "", tree)
	cmd := loadTestCommand(t, "help")

	if err := cmd.RunFunc(ctx, []string{"bogus"}); err == nil {
		t.Fatal("expected an error for a command name that does not resolve to anything, got nil")
	}
}
