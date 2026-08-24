// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"bytes"
	"io"
	"os"
	"testing"
)

// captureStdout - This function runs fn with os.Stdout temporarily
// redirected to an in-memory pipe, and returns everything fn printed.
// A handful of handlers in this package, "help", "show *", and
// "language list" among them, print straight to os.Stdout rather than
// through a writer carried on *command.AppContext, the same way a
// real Cisco or HP CLI's own output is not something its command
// handlers thread a writer parameter through either. This is the
// smallest way to observe that output directly from a test, restoring
// the real os.Stdout afterward regardless of whether fn panics.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	real := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = real }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	return buf.String()
}
