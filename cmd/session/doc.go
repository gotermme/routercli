// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package session implements the small set of command handlers that
genuinely never leave one CLI connection: "terminal length", "terminal
width", "terminal history size", "terminal filter-mode", "show
terminal", and "show history". Every one of them reads or writes a
field that already lives directly on *command.AppContext itself,
PageLines, TerminalWidth, HistorySize, FilterMode, HistoryFile, never
ctx.State or ctx.Levels, so nothing here is shared canonical state a
daemon could ever meaningfully own; see
claude/DAEMON_ARCHITECTURE_DESIGN.md's own "cmd/session/, a new
package for what is truly local to one connection" section for the
full reasoning.

This package used to be part of cmd/core. Before a daemon architecture
existed, the distinction that decided a handler's home was simply "is
this generic enough to belong in the reusable framework," and every
piece of AppContext lived in the same one process either way, so
folding these commands into cmd/core cost nothing. Once shared state
moves to a daemon, a second, independent distinction starts to matter,
"does this belong to one connection or to the device as a whole," and
that distinction deserves its own package rather than being answered
silently by which existing package a handler happened to already sit
in. cmd/core keeps every other reusable, framework level handler,
"enable", "configure terminal", "help", "language", password manager,
"show version", TOTP self service, "alias", "reload"/"reboot", "write
memory", and account management among them, several of which now touch
state that will move to a real daemon through command.DaemonClient
once their own turn to migrate comes; see cmd/core/doc.go and
command.DaemonClient's own doc comment in command/daemonclient.go.

Nothing here requires package core or package product, and neither of
those two requires this package. main.go wires all three in today, a
blank import of this package alongside its named import of package
core and package product, but a project is free to drop this import
entirely if it has no need for session scoped terminal settings, the
same freedom package core's and package product's own doc comments
describe for their files.

Every file here starts with "cmd_" and can be deleted individually if
a project does not want what it provides, the same convention every
other cmd/ package in this project follows. Importing package session
from main.go, even with a blank import since nothing here needs to be
referenced by name, is what loads every handler in this directory,
since each file's init function registers itself before main runs.

# Writing a New Command

The same three part shape every other cmd/ package in this project
follows: a reference name matching exactly on both the Go side and the
YAML side, a cmd_<name>.go file with an init function calling
command.Register("<name>", handlerFunc), and a "run: <name>" entry in
the appropriate var/tree/level_*.yaml file. See cmd/core/doc.go's own
"Writing a New Command" section for the full walkthrough; nothing
about that process differs here.

# Application State

No handler in this package touches ctx.State, ctx.Levels, or
ctx.DaemonClient at all. Everything registered here operates only on
AppContext fields that are, and stay, genuinely private to one
connection: PageLines and TerminalWidth ("terminal length"/"terminal
width"), HistorySize ("terminal history size"), FilterMode ("terminal
filter-mode"), and HistoryFile, read fresh from disk on every "show
history" call rather than cached, see historyLines in cmd_show.go. A
real daemon holding any of these would only add a network round trip
to something that is correctly instantaneous today and correctly
private to one terminal; that is the whole reason this package exists
separately from cmd/core rather than migrating alongside it.

# Negation

None of the commands registered here are negatable; "terminal length
0", not "no terminal length", is this package's own convention for
resetting paging, the same well known Cisco meaning package core and
package product both also follow wherever a "zero means off" setting
already exists.
*/
package session
