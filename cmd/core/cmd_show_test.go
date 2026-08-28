// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"testing"
)

// TestShowVersionHandlerReturnsNoError - This test verifies that
// "show version" runs without error and prints something, the
// minimum guarantee for a command with nothing else to observe.
func TestShowVersionHandlerReturnsNoError(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "show.version")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Errorf("show.version handler returned unexpected error: %v", runErr)
	}
	if out == "" {
		t.Error("expected show.version to print something")
	}
}
