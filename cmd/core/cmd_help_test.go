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
