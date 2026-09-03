// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package daemon implements a real RouterCLI daemon: the persistent
process a deployment runs alongside its CLI clients, config.SystemConfig.DaemonSocketPath
naming the Unix domain socket both sides agree on, so more than one
connected session genuinely shares one running configuration, one set
of accounts, and one set of roles, rather than each session holding
its own separate copy that can drift from what every other session
sees. See claude/DAEMON_ARCHITECTURE_DESIGN.md for the full design
this package implements, and cmd/routercli-daemon for the small binary
that actually runs it.

# The single writer goroutine and the canonical state itself

Store, in store.go, is this package's own concurrency primitive: one
long running goroutine owns a value of some caller supplied type
directly, in ordinary unshared Go memory, with no mutex protecting it
at all, because nothing outside that one goroutine is ever allowed to
touch it. Every read and every mutation arrives as a function
submitted through Do and is run strictly one at a time, in the order
that goroutine happens to receive them, eliminating an entire class of
race by construction rather than by careful, scattered locking
discipline across dozens of separate mutation sites.

State, in state.go, is the concrete shape of what a real daemon
actually holds: everything claude/DAEMON_ARCHITECTURE_DESIGN.md names
as genuinely shared. Field types are drawn directly from this
project's own existing, already reusable packages, command and auth,
rather than duplicated here; ProductState stays an opaque any,
matching command.AppContext.State's own existing genericity, since
this package stays as free of any one deployment's own Cisco or HP
flavored concepts as command and auth already are.

# The encrypted channel, the socket, and the wire protocol

Package armorchan, a sibling package this one depends on but does not
itself implement, is the encrypted channel every connection speaks
once handshaken: an X25519 ECDH handshake against this daemon's own
persisted static identity, identity.go, followed by an AES-256-GCM
protected channel for every message afterward. Listen and Dial, in
socket.go, open and connect to the Unix domain socket itself, peer
credential checked through PeerCredentialChecker, most commonly
AllowedUIDs; AcceptAndHandshake combines an accepted connection with
its own armorchan handshake. protocol.go is the message catalog every
connection exchanges once handshaken, one byte MessageKind tag
followed by a JSON payload: Hello and HelloResponse, the session
registration exchange every connection begins with; Goodbye, a one way
"this session is ending normally" notification; AuditEvent, one per
dispatched command; ListUsersRequest/Response; DisconnectUserRequest/Response;
RebootRequest/Response; and Farewell, a push the daemon sends a
targeted session right before closing its connection.

# Both sides of that protocol

SessionDirectory, in sessions.go, is the daemon's own session and
connection registry: Register, Touch, Unregister, List, Farewell for a
targeted disconnect, and BroadcastFarewell for a reboot or a clean
shutdown alike, all built on Store the same concurrency safe way State
itself is. Server, in server.go, is the daemon's own connection
acceptor and per connection message dispatcher, the piece that
actually answers every message protocol.go names against a real
SessionDirectory, a real canonical Store[State], and a real
*auditlog.AuditLog; TriggerReboot is the one function both a
privileged session's own reboot request and this daemon's own SIGHUP
handler converge on.

RemoteClient, in remoteclient.go, is the CLI side of one live
connection: it satisfies command.Auditor directly, forwarding every
dispatched command to the daemon as an AuditEvent rather than writing
a local file, and it satisfies the four non-Mutate methods of
command.DaemonClient, ListUsers, DisconnectUser, Reboot, and
FarewellChannel, genuinely wired across the socket. StandaloneClient,
in client.go, is the no daemon configured implementation of the same
interface, reading and writing an in-process Store[State] directly;
ConnectedClient, in connectedclient.go, combines the two, a
StandaloneClient still backing the four Mutate methods, ProductState
and the rest still living only in one CLI process's own memory, a
known, disclosed limitation, layered with a RemoteClient genuinely
wired for session and connection tracking. main.go, at the root of
this repository, is what actually dials one and assigns it to
command.AppContext.DaemonClient once config.SystemConfig.DaemonSocketPath
is set; a deployment that leaves it empty keeps command.AppContext
backed by a plain StandaloneClient instead, unchanged from how this
project always worked.
*/
package daemon
