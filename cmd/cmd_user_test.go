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

// userLevels - This function builds a minimal *command.TreeStructure
// with a "user" CommandLevel whose Parent is "base", the same shape
// var/tree/tree_structure.yaml declares, for tests that need
// ctx.Levels.ByName["user"] populated without loading real YAML
// files.
func userLevels() *command.TreeStructure {
	return &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"user": {Name: "user", Parent: "base", PromptSuffix: "(user)", Tree: map[string]*command.Command{}},
	}}
}

// TestRequireLoggedInErrorsWhenSessionNil - This test verifies that
// requireLoggedIn refuses a *command.AppContext with a nil Session,
// rather than panicking on a nil pointer dereference, the state a
// handful of throwaway test contexts, and in principle a badly
// constructed real one, could be in.
func TestRequireLoggedInErrorsWhenSessionNil(t *testing.T) {
	ctx := newTestContext()
	if err := requireLoggedIn(ctx); err == nil {
		t.Fatal("expected an error for a nil Session, got nil")
	}
}

// TestRequireLoggedInErrorsWhenNotAuthenticated - This test verifies
// that requireLoggedIn refuses a Session that exists but has never
// authenticated, the ordinary state for any session run with
// AuthRequired off.
func TestRequireLoggedInErrorsWhenNotAuthenticated(t *testing.T) {
	ctx := newTestContext()
	ctx.Session = &auth.Session{}
	if err := requireLoggedIn(ctx); err == nil {
		t.Fatal("expected an error for an unauthenticated Session, got nil")
	}
}

// TestRequireLoggedInAllowsAuthenticatedSession - This test verifies
// that requireLoggedIn returns nil once Session.Authenticated is
// true.
func TestRequireLoggedInAllowsAuthenticatedSession(t *testing.T) {
	ctx := newTestContext()
	ctx.Session = &auth.Session{Username: "alice", Authenticated: true}
	if err := requireLoggedIn(ctx); err != nil {
		t.Errorf("expected no error for an authenticated Session, got %v", err)
	}
}

// TestUserHandlerRequiresLogin - This test verifies that the "user"
// handler refuses to enter user mode for a session that is at the
// base level but never logged in, and leaves ctx.Position untouched
// rather than pushing a frame before returning the error.
func TestUserHandlerRequiresLogin(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = userLevels()
	ctx.Session = &auth.Session{}
	ctx.Position = command.NewCommandLevelStack("base", "", map[string]*command.Command{})
	cmd := loadTestCommand(t, "user")

	if err := cmd.RunFunc(ctx, nil); err == nil {
		t.Fatal("expected an error entering user mode without being logged in, got nil")
	}
	if ctx.Position.Depth() != 1 {
		t.Errorf("expected Position to stay at depth 1 after a refused entry, got %d", ctx.Position.Depth())
	}
}

// TestUserHandlerRequiresBaseLevel - This test verifies that the
// "user" handler refuses entry from anywhere other than the base
// level, the same parent check every other Command Level entry
// enforces, even for an authenticated session.
func TestUserHandlerRequiresBaseLevel(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = userLevels()
	ctx.Session = &auth.Session{Username: "alice", Authenticated: true}
	ctx.Position = command.NewCommandLevelStack("exec", "", map[string]*command.Command{})
	cmd := loadTestCommand(t, "user")

	if err := cmd.RunFunc(ctx, nil); err == nil {
		t.Fatal("expected an error entering user mode from a Command Level other than base, got nil")
	}
	if ctx.Position.Depth() != 1 {
		t.Errorf("expected Position to stay at depth 1 after a refused entry, got %d", ctx.Position.Depth())
	}
}

// TestUserHandlerPushesUserFrameWhenAuthenticated - This test
// verifies that the "user" handler pushes a CommandLevelFrame named
// "user", carrying the manifest's own PromptSuffix, once both the
// parent check and the login check pass.
func TestUserHandlerPushesUserFrameWhenAuthenticated(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = userLevels()
	ctx.Session = &auth.Session{Username: "alice", Authenticated: true}
	ctx.Position = command.NewCommandLevelStack("base", "", map[string]*command.Command{})
	cmd := loadTestCommand(t, "user")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Fatalf("user handler returned unexpected error: %v", err)
	}
	if ctx.Position.Depth() != 2 {
		t.Fatalf("expected Position depth 2 after entering user mode, got %d", ctx.Position.Depth())
	}
	if ctx.Position.Current().Name != "user" {
		t.Errorf("Current().Name = %q, want %q", ctx.Position.Current().Name, "user")
	}
	if ctx.Position.Current().PromptSuffix != "(user)" {
		t.Errorf("Current().PromptSuffix = %q, want %q", ctx.Position.Current().PromptSuffix, "(user)")
	}
}
