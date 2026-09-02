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
//
// Line, item 11 of the Framework Gap Roadmap, is a different thing
// entirely, and does belong here: not a session's own live override,
// but the deployment wide default a fresh session with no override of
// its own falls back to, "line length" standing in for what real
// Cisco and HP configure through "line vty" and "line console"
// instead. See LineDefaults's own doc comment and
// cmd/product/cmd_line.go for how a value set here reaches
// command.AppContext.DefaultPageLines, DefaultTerminalWidth, and
// PagingEnabled, both immediately and, once saved, again at every
// future boot.
type ProductState struct {
	Description string
	Hostname    string
	Interfaces  map[string]*InterfaceState

	// BannerMOTD and BannerLogin hold "banner motd <text>" and
	// "banner login <text>", see cmd_banner.go. Both are shown before
	// authentication, BannerMOTD first, matching real Cisco's own
	// ordering, "message of the day" being about the connection
	// itself, shown regardless of whether a login prompt follows at
	// all, while BannerLogin is shown only immediately before a real
	// login prompt actually runs, see main.go's printBanner and
	// establishSession. An empty string, the default, prints nothing
	// at all, exactly like an interface with no Description ever set.
	BannerMOTD  string
	BannerLogin string

	// Line holds "line" mode's own persisted defaults, see
	// LineDefaults and cmd_line.go. Its zero value, every field nil,
	// is the correct state for a deployment where nobody has ever
	// entered "line" mode at all, leaving every one of
	// etc/routercli.yaml's own config file driven defaults completely
	// untouched.
	Line LineDefaults
}

// LineDefaults - This type holds "line" mode's own three settings,
// item 11 of the Framework Gap Roadmap: Length and Width, the
// deployment wide fallback command.AppContext.DefaultPageLines and
// DefaultTerminalWidth fall back to, see paging.EffectivePageLines and
// paging.EffectiveTerminalWidth, only when a session's own live
// override, "terminal length" or "terminal width" typed this session,
// is unset and the real terminal's own size cannot be auto-detected
// either; and Paging, the deployment wide switch for whether the
// interactive pager runs at all, command.AppContext.PagingEnabled.
//
// Each field is a pointer, nil meaning "line length", "line width",
// or "line paging" has never actually been typed and saved, so
// main.go leaves this deployment's own config file driven default
// completely alone rather than every fresh boot silently overwriting
// it with some zero value nobody actually chose. This is the same
// nil-means-unset convention command.AppContext.PageLines and
// TerminalWidth themselves already use for a session's own live
// override, applied here one level up, to a deployment's own default
// rather than one session's own choice.
type LineDefaults struct {
	Length *int
	Width  *int
	Paging *bool
}

// InterfaceState - This type holds the per-interface values config-if
// mode commands mutate. See cmd_interface.go, cmd_description_if.go,
// and cmd_shutdown.go.
type InterfaceState struct {
	Description string
	Shutdown    bool
}
