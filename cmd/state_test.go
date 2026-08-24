// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import "testing"

// TestExampleStateInterfaceCreatesMapAndEntryOnFirstUse - This test
// verifies that Interface lazily creates both the Interfaces map
// itself, nil on a freshly constructed ExampleState, and the named
// entry within it, on the very first call for a given name.
func TestExampleStateInterfaceCreatesMapAndEntryOnFirstUse(t *testing.T) {
	state := &ExampleState{}
	if state.Interfaces != nil {
		t.Fatal("test setup problem: expected a freshly constructed ExampleState to have a nil Interfaces map")
	}

	iface := state.Interface("eth0")
	if iface == nil {
		t.Fatal("expected Interface to return a non-nil *InterfaceState")
	}
	if state.Interfaces == nil {
		t.Fatal("expected Interface to lazily create the Interfaces map")
	}
	if state.Interfaces["eth0"] != iface {
		t.Error("expected the returned *InterfaceState to be stored under its name in Interfaces")
	}
}

// TestExampleStateInterfaceReturnsSameEntryOnRepeatedCalls - This test
// verifies that calling Interface twice for the same name returns the
// same *InterfaceState both times, rather than overwriting whatever
// was already set on it, so an earlier "shutdown" is not silently
// lost by a later "description" call for the same interface.
func TestExampleStateInterfaceReturnsSameEntryOnRepeatedCalls(t *testing.T) {
	state := &ExampleState{}
	first := state.Interface("eth0")
	first.Shutdown = true

	second := state.Interface("eth0")
	if second != first {
		t.Fatal("expected a second call for the same name to return the same *InterfaceState")
	}
	if !second.Shutdown {
		t.Error("expected the previously set Shutdown value to survive a second Interface call")
	}
}

// TestExampleStateInterfaceKeysDistinctNamesSeparately - This test
// verifies that two different interface names get two distinct,
// independent *InterfaceState entries.
func TestExampleStateInterfaceKeysDistinctNamesSeparately(t *testing.T) {
	state := &ExampleState{}
	eth0 := state.Interface("eth0")
	eth1 := state.Interface("eth1")

	if eth0 == eth1 {
		t.Fatal("expected different interface names to get distinct *InterfaceState entries")
	}
}
