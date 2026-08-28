// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/gologme/log"
	"github.com/gotermme/routercli/command"
)

// newTestContext - This function builds a minimal *command.AppContext
// suitable for exercising a single command handler directly, without
// needing readline, a real login session, or any of main.go's other
// startup machinery. An implementer writing their own command handler
// tests can copy this same pattern for their own handler tests.
//
// This is deliberately the same shape as cmd/core's own
// newTestContext, one independent copy per package rather than a
// shared export, since the two disagree on what State should hold,
// ProductState here, nil in package core, and package product cannot
// export a test-only helper for package core to import without pulling
// a test file into a non-test build.
func newTestContext() *command.AppContext {
	return &command.AppContext{
		State:  &ProductState{},
		Logger: log.New(io.Discard, "", 0),
	}
}

// loadTestCommand - This function resolves handlerName into its
// *command.Command by writing a throwaway, one command tree file and
// loading it through command.LoadTree, the same loader main.go uses
// at startup. This is deliberately not a direct call into the handler
// closure itself, since command.Register stores it only in an
// unexported registry inside package command, reachable from outside
// that package through this same "run:" resolution path a real tree
// file uses. handlerName must already be registered, which every
// cmd_*.go file in this package's own init() does automatically once
// this package is imported, exactly as it is here.
func loadTestCommand(t *testing.T, handlerName string) *command.Command {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tree.yaml")
	body := "commands:\n  test:\n    run: " + handlerName + "\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("failed to write test tree file: %v", err)
	}
	tree, err := command.LoadTree(path)
	if err != nil {
		t.Fatalf("LoadTree returned error: %v", err)
	}
	return tree["test"]
}
