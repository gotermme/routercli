// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package daemon implements the RouterCLI daemon's own canonical, shared
state, the persistent process a real deployment runs alongside its CLI
clients so more than one connected session can genuinely share one
running configuration, one set of accounts, and one set of roles,
rather than each session holding its own separate copy that can drift
from what every other session sees. See
claude/DAEMON_ARCHITECTURE_DESIGN.md for the full design this package
implements one piece of.

This is phase two of that document's own suggested implementation
order: the single writer goroutine and the in-memory canonical state
it owns, built and tested entirely on its own, driven directly in
process against a plain Go test, still with no real socket and no real
daemon binary. See armorchan, phase one, for the encrypted channel
this daemon's own socket will eventually speak; this package does not
depend on armorchan at all, and armorchan does not depend on this
package, each independently useful and independently tested before
phase three wires them together.

# The single writer goroutine

Store, in store.go, is this package's own concurrency primitive: one
long running goroutine owns a value of some caller supplied type
directly, in ordinary unshared Go memory, with no mutex protecting it
at all, because nothing outside that one goroutine is ever allowed to
touch it. Every read and every mutation, a hostname change from one
session, an account creation from another, a show running-config read
from a third, arrives as a function submitted through Do and is run
strictly one at a time, in the order that goroutine happens to receive
them. This is Go's own "share memory by communicating" principle
applied directly to the exact problem this daemon exists to solve, and
it eliminates an entire class of race by construction rather than by
careful, scattered locking discipline across dozens of separate
mutation sites; see claude/DAEMON_ARCHITECTURE_DESIGN.md's own section
on this daemon's internal concurrency model for the full reasoning,
and store_test.go for the concurrent access tests this design is
meant to make trivially true rather than merely probably true.

# The canonical state itself

State, in state.go, is the concrete shape of what this daemon actually
holds: everything claude/DAEMON_ARCHITECTURE_DESIGN.md names as
genuinely shared, moving out of command.AppContext.State and
command.AppContext.Levels and into this one place instead. Field types
are drawn directly from this project's own existing, already reusable
packages, command and auth, rather than duplicated here; ProductState
itself stays an opaque any, exactly matching
command.AppContext.State's own existing genericity, since this package
is meant to stay as free of any one deployment's own Cisco or HP
flavored concepts as command and auth already are. Wiring this State
up to real handlers, and to a real ProductState type, is later phase
work, not this one.
*/
package daemon
