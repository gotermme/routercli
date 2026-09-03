// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"errors"
	"time"

	"github.com/gotermme/routercli/auth"
)

// ErrDaemonNotConfigured is returned by ListUsers, DisconnectUser, and
// Reboot below when whatever backs AppContext.DaemonClient has no real
// daemon on the other end of it, config.SystemConfig.DaemonSocketPath
// empty; see claude/DAEMON_ARCHITECTURE_DESIGN.md's own settled
// reading of that case: "no daemon involved, no show users, no
// disconnect user, reboot behaving exactly as reload does now." A
// deployment with no daemon configured never reaches this error in
// practice, since show.users and disconnect.user are pruned out of
// the tree entirely in that case, see main.go's own featureFlags, and
// cmd/core's own reboot handler checks for this specific error to
// fall back to today's local reload behavior rather than surfacing it
// to whoever typed "reboot"; ErrDaemonNotConfigured still exists, and
// StandaloneClient still returns it from all three methods, as
// defense in depth, so a bug in that pruning or that fallback fails
// with a clear, named error rather than a nil pointer dereference or
// silent no-op.
var ErrDaemonNotConfigured = errors.New("command: no daemon is configured for this deployment")

// SessionInfo is everything "show users" reports about one currently
// attached session: a daemon assigned session ID, the username that
// session logged in as, the Command Level it was most recently seen
// at, when it connected, and how long it has been idle since,
// matching claude/DAEMON_ARCHITECTURE_DESIGN.md's own ListUsers
// description exactly. This is declared here, in package command,
// rather than reusing daemon.SessionInfo directly, for the same
// import cycle reason DaemonClient itself is declared here rather
// than in package daemon; see this file's own top of file doc
// comment. daemon.StandaloneClient and any real remote implementation
// both already import package command, so both can freely build a
// SessionInfo from whatever their own, richer, package private
// session type holds.
type SessionInfo struct {
	ID           string
	Username     string
	CommandLevel string
	ConnectedAt  time.Time
	IdleFor      time.Duration
}

// DaemonClient is the small interface a handler that touches shared
// canonical state, hostname first, calls into instead of reading or
// writing AppContext.State or AppContext.Levels directly; see
// claude/DAEMON_ARCHITECTURE_DESIGN.md for the design this
// implements. Every method takes a plain function to run against the
// current value of one piece of canonical state and returns whatever
// that function returns, mirroring package daemon's own Store.Do,
// see daemon/store.go, rather than inventing a second pattern for the
// same idea.
//
// This interface is declared here, in package command, rather than in
// package daemon, where an earlier phase's first draft placed it.
// Package daemon already imports package command, for TreeStructure
// and RoleSet below, so package command importing package daemon back
// for this same interface would be a direct import cycle. Go's own
// interface satisfaction is structural, so daemon.StandaloneClient,
// whose four Mutate methods already match this exact shape, satisfies
// DaemonClient too, without package daemon needing to import this
// type at all; a compile time assertion confirming this,
// "var _ command.DaemonClient = (*StandaloneClient)(nil)", lives in
// daemon/client.go itself, which already imports package command.
// Only main.go, which already imports both packages, ties a real
// AppContext.DaemonClient to a real daemon.StandaloneClient, or, once
// one exists, to a real remote implementation talking to an actual
// daemon over a socket.
//
// Close, present on daemon.DaemonClient, is deliberately absent here;
// process lifecycle management stays owned by whichever code actually
// constructs a DaemonClient in the first place, main.go today, never
// by anything reachable through AppContext.
//
// ListUsers, DisconnectUser, Reboot, and FarewellChannel below are a
// second, later addition to this same interface, phase five of
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own suggested implementation
// order: "adds show users, disconnect user, and the new reboot
// behavior, along with daemon owned audit logging." Unlike the four
// Mutate methods above, none of these four take a closure, so unlike
// those four, nothing about them stands in the way of a real, wire
// connected implementation; each is a plain request a real
// RemoteClient, see daemon/remoteclient.go, sends across armorchan
// directly. Reboot in particular is deliberately a bare trigger, no
// newState parameter the way daemon.DaemonClient.ReplaceState takes
// one, since a real daemon already knows its own UsersFile, RolesFile,
// and StartupConfigFile paths from its own startup configuration and
// rereads them itself; this CLI-facing Reboot needs no path of its
// own to hand across, which is exactly why, unlike ReplaceState, nothing
// here needs package config after all. Standalone mode, no daemon
// configured, keeps reboot behaving exactly like today's reload
// instead of calling this method at all, see cmd/core/cmd_admin.go's
// own runReload, falling back the moment this method reports
// ErrDaemonNotConfigured.
type DaemonClient interface {
	// MutateProductState runs fn against the current State, whatever a
	// project built on this framework stores there, ProductState in
	// this project's own shipped example, and returns whatever fn
	// itself returned.
	MutateProductState(fn func(any) (any, error)) (any, error)

	// MutateLevels runs fn against the current *TreeStructure and
	// returns whatever fn itself returned.
	MutateLevels(fn func(*TreeStructure) (any, error)) (any, error)

	// MutateUsers runs fn against the current auth.Users and returns
	// whatever fn itself returned.
	MutateUsers(fn func(auth.Users) (any, error)) (any, error)

	// MutateRoles runs fn against the current *RoleSet and returns
	// whatever fn itself returned.
	MutateRoles(fn func(*RoleSet) (any, error)) (any, error)

	// ListUsers returns one SessionInfo per currently attached
	// session, everything "show users" prints, or ErrDaemonNotConfigured
	// when no real daemon backs this DaemonClient.
	ListUsers() ([]SessionInfo, error)

	// DisconnectUser asks the daemon to end one session belonging to
	// username. sessionID empty means "the one session belonging to
	// username", refusing with an error listing the candidates if more
	// than one matches; sessionID non-empty names one exactly, the
	// same short identifier ListUsers already reports per session, so
	// a person can always resolve the ambiguity by looking there
	// first. Returns ErrDaemonNotConfigured when no real daemon backs
	// this DaemonClient.
	DisconnectUser(username, sessionID string) error

	// Reboot asks the daemon to reread its own canonical state from
	// disk and end every attached session, including this one, with a
	// "Device is rebooting" farewell; see this interface's own doc
	// comment above for why this takes no newState of its own. A nil
	// return means the daemon accepted the request and this session's
	// own ending now arrives asynchronously, through FarewellChannel,
	// not through this call returning; a non-nil return, including
	// ErrDaemonNotConfigured, means no reboot was triggered at all.
	Reboot() error

	// FarewellChannel returns the channel a session receives exactly
	// one push on, the human readable reason text, the moment the
	// daemon ends this connection on purpose, DisconnectUser or Reboot
	// elsewhere having targeted it, or the connection to the daemon is
	// lost with no explicit farewell at all. A DaemonClient with no
	// real daemon behind it, StandaloneClient among them, returns a
	// nil channel here, which a select statement simply never chooses,
	// the same nil channel convention AppContext.ReloadScheduler's own
	// FireChannel already establishes for "not wired up in this
	// context."
	FarewellChannel() <-chan string
}
