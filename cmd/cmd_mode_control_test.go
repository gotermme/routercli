// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"testing"

	"github.com/gotermme/routercli/command"
)

// TestExitHandlerAtRootReturnsErrQuit - This test verifies that
// running "exit" at the root frame, AtRoot() true, returns
// command.ErrQuit rather than popping, the sentinel main.go's runLoop
// checks for to end the whole program.
func TestExitHandlerAtRootReturnsErrQuit(t *testing.T) {
	ctx := newTestContext()
	ctx.Position = command.NewCommandLevelStack("exec", "", map[string]*command.Command{})
	cmd := loadTestCommand(t, "exit")

	if err := cmd.RunFunc(ctx, nil); err != command.ErrQuit {
		t.Errorf("exit handler at root returned %v, want command.ErrQuit", err)
	}
}

// TestExitHandlerNestedPopsOneLevel - This test verifies that running
// "exit" from a pushed, non-root frame pops exactly one level and
// returns no error, rather than quitting the program.
func TestExitHandlerNestedPopsOneLevel(t *testing.T) {
	ctx := newTestContext()
	ctx.Position = command.NewCommandLevelStack("exec", "", map[string]*command.Command{})
	ctx.Position.Push(command.CommandLevelFrame{Name: "config"})
	ctx.Position.Push(command.CommandLevelFrame{Name: "config-if"})
	cmd := loadTestCommand(t, "exit")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Fatalf("exit handler returned unexpected error: %v", err)
	}
	if ctx.Position.Depth() != 2 {
		t.Errorf("Position.Depth() = %d, want 2 after exiting one nested level", ctx.Position.Depth())
	}
	if ctx.Position.Current().Name != "config" {
		t.Errorf("Current().Name = %q, want %q", ctx.Position.Current().Name, "config")
	}
}

// TestEndHandlerJumpsStraightToRootFromAnyDepth - This test verifies
// that running "end" from several levels deep returns straight to the
// root frame in one call, rather than one level at a time like exit.
func TestEndHandlerJumpsStraightToRootFromAnyDepth(t *testing.T) {
	ctx := newTestContext()
	ctx.Position = command.NewCommandLevelStack("exec", "", map[string]*command.Command{})
	ctx.Position.Push(command.CommandLevelFrame{Name: "config"})
	ctx.Position.Push(command.CommandLevelFrame{Name: "config-if"})
	cmd := loadTestCommand(t, "end")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Fatalf("end handler returned unexpected error: %v", err)
	}
	if ctx.Position.Depth() != 1 {
		t.Errorf("Position.Depth() = %d, want 1 after \"end\"", ctx.Position.Depth())
	}
	if ctx.Position.Current().Name != "exec" {
		t.Errorf("Current().Name = %q, want %q", ctx.Position.Current().Name, "exec")
	}
}

// TestEndHandlerAtRootIsNoOp - This test verifies that "end" run
// already at the root frame stays there and returns no error, rather
// than quitting the way "exit" would.
func TestEndHandlerAtRootIsNoOp(t *testing.T) {
	ctx := newTestContext()
	ctx.Position = command.NewCommandLevelStack("exec", "", map[string]*command.Command{})
	cmd := loadTestCommand(t, "end")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Fatalf("end handler returned unexpected error: %v", err)
	}
	if ctx.Position.Depth() != 1 {
		t.Errorf("Position.Depth() = %d, want 1", ctx.Position.Depth())
	}
}
