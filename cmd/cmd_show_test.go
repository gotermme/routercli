// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"strings"
	"testing"

	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/i18n"
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

// TestShowStartupConfigHandlerReturnsNoError - This test verifies
// that "show startup-config" runs without error, the same minimum
// guarantee as show.version.
func TestShowStartupConfigHandlerReturnsNoError(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "show.startup-config")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Errorf("show.startup-config handler returned unexpected error: %v", err)
	}
}

// TestShowInterfaceHandlerNoInterfacesConfigured - This test verifies
// that "show interface" with no interfaces ever touched prints the
// "nothing configured" text rather than an empty listing or an
// error.
func TestShowInterfaceHandlerNoInterfacesConfigured(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "show.interface")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Errorf("show.interface handler returned unexpected error: %v", runErr)
	}
	if out == "" {
		t.Error("expected show.interface to print something when no interfaces are configured")
	}
}

// TestShowInterfaceHandlerListsConfiguredInterfacesInSortedOrder -
// This test verifies that "show interface" lists every interface that
// has been touched, each with its shutdown status and description
// when set, in sorted order regardless of Go's randomized map
// iteration, exercising sortedInterfaceNames through the handler.
func TestShowInterfaceHandlerListsConfiguredInterfacesInSortedOrder(t *testing.T) {
	ctx := newTestContext()
	// T() on a nil Translator, newTestContext's default, drops any
	// format args rather than applying them, see i18n.Translator.T's
	// own doc comment, so a real Translator with a minimal catalog is
	// needed here to actually see the interpolated description text
	// in the captured output.
	ctx.Translator = i18n.New(map[string]i18n.Catalog{
		"en": {"show.interface.description_label": "Description: %s"},
	}, "en", "en")
	state := ctx.State.(*ExampleState)
	state.Interface("eth1").Shutdown = true
	state.Interface("eth0").Description = "uplink"
	cmd := loadTestCommand(t, "show.interface")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Errorf("show.interface handler returned unexpected error: %v", runErr)
	}

	eth0At := strings.Index(out, "eth0")
	eth1At := strings.Index(out, "eth1")
	if eth0At == -1 || eth1At == -1 {
		t.Fatalf("expected both eth0 and eth1 to appear in output, got %q", out)
	}
	if eth0At > eth1At {
		t.Errorf("expected eth0 to be listed before eth1 (sorted order), got %q", out)
	}
	if !strings.Contains(out, "uplink") {
		t.Errorf("expected eth0's description %q to appear, got %q", "uplink", out)
	}
}

// TestShowRunningConfigHandlerIncludesEveryConfiguredValue - This
// test verifies that "show running-config" reflects the hostname,
// description, terminal geometry, and per-interface state that has
// actually been set, exercising printRunningConfig end to end through
// the registered handler, the same way a real "show running-config"
// on the device would.
func TestShowRunningConfigHandlerIncludesEveryConfiguredValue(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{}
	state := ctx.State.(*ExampleState)
	state.Hostname = "myrouter"
	state.Description = "a lab router"
	state.TerminalLength = 40
	state.TerminalWidth = 120
	state.Interface("eth0").Description = "uplink"
	state.Interface("eth1").Shutdown = true
	cmd := loadTestCommand(t, "show.running-config")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Fatalf("show.running-config handler returned unexpected error: %v", runErr)
	}

	for _, want := range []string{"myrouter", "a lab router", "terminal length 40", "terminal width 120", "eth0", "uplink", "eth1", "shutdown"} {
		if !strings.Contains(out, want) {
			t.Errorf("show running-config output missing %q, got:\n%s", want, out)
		}
	}
}

// TestShowRunningConfigHandlerOmitsNeverConfiguredValues - This test
// verifies the companion case: a freshly constructed ExampleState with
// nothing ever set produces output with none of hostname, description,
// or terminal geometry lines, matching how a real device only shows
// config lines for values that have actually been configured.
func TestShowRunningConfigHandlerOmitsNeverConfiguredValues(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{}
	cmd := loadTestCommand(t, "show.running-config")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Fatalf("show.running-config handler returned unexpected error: %v", runErr)
	}
	for _, unwanted := range []string{"hostname", "terminal length", "terminal width", "set description"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("show running-config output unexpectedly contains %q for an unconfigured value, got:\n%s", unwanted, out)
		}
	}
}
