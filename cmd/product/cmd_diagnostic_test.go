// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import "testing"

// TestDiagnosticSelfTestHandlerReturnsNoError - This test verifies
// that the registered "diagnostic.self-test" handler runs without
// error. It has no state to mutate and nothing else observable
// outside its own printed line, so this confirms it is wired up and
// callable at all, the same minimum guarantee every other handler in
// this package gets.
func TestDiagnosticSelfTestHandlerReturnsNoError(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "diagnostic.self-test")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Errorf("diagnostic.self-test handler returned unexpected error: %v", err)
	}
}
