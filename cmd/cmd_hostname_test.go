// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

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
// startup machinery. An implementer writing their own cmd/cmd_*.go
// file can copy this same pattern for their own handler tests.
func newTestContext() *command.AppContext {
	return &command.AppContext{
		State:  &ExampleState{},
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
// cmd/cmd_*.go file's own init() does automatically once package cmd
// is imported, exactly as it is here.
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

// TestHostnameHandlerSetsHostname - This test verifies that running
// the registered "hostname" handler with one argument sets
// ExampleState.Hostname to that argument, the ordinary, non-negated
// path a real "hostname myrouter" command line takes.
func TestHostnameHandlerSetsHostname(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "hostname")

	if err := cmd.RunFunc(ctx, []string{"myrouter"}); err != nil {
		t.Fatalf("hostname handler returned error: %v", err)
	}

	state := ctx.State.(*ExampleState)
	if state.Hostname != "myrouter" {
		t.Errorf("Hostname = %q, want %q", state.Hostname, "myrouter")
	}
}

// TestHostnameHandlerNegatedResetsToEmpty - This test verifies that
// running the "hostname" handler with ctx.Negated set, the "no
// hostname" path, clears Hostname back to empty rather than to
// defaultHostname itself. defaultHostname is only what the prompt
// falls back to displaying when Hostname is empty, see main.go's
// buildPrompt, not a value this handler actually stores back onto
// ExampleState.
func TestHostnameHandlerNegatedResetsToEmpty(t *testing.T) {
	ctx := newTestContext()
	ctx.State.(*ExampleState).Hostname = "myrouter"
	ctx.Negated = true
	cmd := loadTestCommand(t, "hostname")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Fatalf("hostname handler returned error: %v", err)
	}

	state := ctx.State.(*ExampleState)
	if state.Hostname != "" {
		t.Errorf("Hostname = %q after \"no hostname\", want empty", state.Hostname)
	}
}
