// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

// ----------------------------------------------------------------------
// Define Object Model
// ----------------------------------------------------------------------

// ProductState - This type holds the values this example's commands
// mutate and "show running-config" reports back out. It lives here,
// not in package command, because it is specific to this example, not
// part of the reusable framework. A different command set would
// define its own state type instead. Handlers reach it through
// ctx.State.(*ProductState). See any handler in this package for the
// pattern.
//
// Interfaces is keyed by interface name, for example "eth0".
// "interface eth0" in config mode pushes a config-if CommandLevelFrame
// whose Context is that same name, and the description and shutdown
// handlers in config-if mode look themselves up in this map through
// that Context to know which interface they are editing. An interface
// that has never been explicitly configured gets no entry until
// something actually sets a value on it, the same principle as the
// top-level Description field only appearing in show running-config
// once it has actually been set.
//
// Neither TerminalLength nor TerminalWidth lives here. Both real,
// functional counterparts, command.AppContext.PageLines and
// command.AppContext.TerminalWidth, live on the framework's own
// AppContext instead, see cmd/core/cmd_terminal.go's own doc comment
// for why: real Cisco and HP treat both terminal length and terminal
// width as strictly session scoped, never written to running-config or
// startup-config, so neither one has any business living alongside a
// value "show running-config" reports back.
type ProductState struct {
	Description string
	Hostname    string
	Interfaces  map[string]*InterfaceState
}

// InterfaceState - This type holds the per-interface values config-if
// mode commands mutate. See cmd_interface.go, cmd_description_if.go,
// and cmd_shutdown.go.
type InterfaceState struct {
	Description string
	Shutdown    bool
}
