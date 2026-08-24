// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import "testing"

// TestTerminalLengthHandlerSetsValidValue - This test verifies that
// "terminal length 40", a value inside the configured <2-1000> range,
// sets ExampleState.TerminalLength to the parsed number.
func TestTerminalLengthHandlerSetsValidValue(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "terminal.length")

	if err := cmd.RunFunc(ctx, []string{"40"}); err != nil {
		t.Fatalf("terminal.length handler returned unexpected error: %v", err)
	}
	if state := ctx.State.(*ExampleState); state.TerminalLength != 40 {
		t.Errorf("TerminalLength = %d, want 40", state.TerminalLength)
	}
}

// TestTerminalLengthHandlerRejectsNonNumber - This test verifies that
// a non-numeric argument is rejected with an error, rather than
// panicking in strconv.Atoi or silently leaving TerminalLength
// unset.
func TestTerminalLengthHandlerRejectsNonNumber(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "terminal.length")

	if err := cmd.RunFunc(ctx, []string{"not-a-number"}); err == nil {
		t.Fatal("expected an error for a non-numeric terminal length, got nil")
	}
	if state := ctx.State.(*ExampleState); state.TerminalLength != 0 {
		t.Errorf("TerminalLength = %d, want 0 (unchanged) after a rejected value", state.TerminalLength)
	}
}

// TestTerminalLengthHandlerRejectsOutOfRangeValues - This test
// verifies that both ends of the <2-1000> range are enforced: a value
// below the minimum and a value above the maximum are both rejected.
func TestTerminalLengthHandlerRejectsOutOfRangeValues(t *testing.T) {
	for _, raw := range []string{"1", "1001"} {
		ctx := newTestContext()
		cmd := loadTestCommand(t, "terminal.length")

		if err := cmd.RunFunc(ctx, []string{raw}); err == nil {
			t.Errorf("expected an error for out-of-range terminal length %q, got nil", raw)
		}
	}
}

// TestTerminalWidthHandlerSetsValidValue - This test verifies that
// "terminal width" sets its own field, TerminalWidth, independently
// of terminal length, confirming setTerminalGeometry's shared logic
// is wired to the right field for each of the two commands that call
// it.
func TestTerminalWidthHandlerSetsValidValue(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "terminal.width")

	if err := cmd.RunFunc(ctx, []string{"120"}); err != nil {
		t.Fatalf("terminal.width handler returned unexpected error: %v", err)
	}
	state := ctx.State.(*ExampleState)
	if state.TerminalWidth != 120 {
		t.Errorf("TerminalWidth = %d, want 120", state.TerminalWidth)
	}
	if state.TerminalLength != 0 {
		t.Errorf("TerminalLength = %d, want 0 (terminal width must not touch it)", state.TerminalLength)
	}
}

// TestTerminalWidthHandlerRejectsOutOfRangeValues - This test
// verifies the same <2-1000> boundary enforcement for terminal width
// as TestTerminalLengthHandlerRejectsOutOfRangeValues covers for
// terminal length.
func TestTerminalWidthHandlerRejectsOutOfRangeValues(t *testing.T) {
	for _, raw := range []string{"0", "5000"} {
		ctx := newTestContext()
		cmd := loadTestCommand(t, "terminal.width")

		if err := cmd.RunFunc(ctx, []string{raw}); err == nil {
			t.Errorf("expected an error for out-of-range terminal width %q, got nil", raw)
		}
	}
}
