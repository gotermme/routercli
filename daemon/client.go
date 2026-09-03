// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package daemon

import (
	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"
)

// DaemonClient is the small interface claude/DAEMON_ARCHITECTURE_DESIGN.md
// describes as what a handler calls into instead of touching
// AppContext.State or AppContext.Levels directly: "only the line or
// two that currently reads or writes ctx.State changes to a call
// against a small daemon client interface instead." StandaloneClient
// below is the implementation a CLI process not configured with a
// daemon uses, wired into a real AppContext.DaemonClient by main.go,
// hostname the first handler actually calling it, see
// command.DaemonClient in command/daemonclient.go for that
// command-facing half of this same shape and why it is a second,
// separate interface rather than this one reused directly.
//
// Every method here takes a plain function to run against the current
// value of one piece of canonical state, deliberately mirroring
// Store.Do's own shape rather than inventing a second pattern.
// StandaloneClient below satisfies this directly, its own
// implementation of each method a thin call into a *Store[State] it
// owns privately, in process, no socket involved. A future remote
// implementation, backed by armorchan and a real daemon on the other
// end of a socket, cannot satisfy this interface the same way: a Go
// closure cannot be serialized across a socket, so a real remote
// DaemonClient needs each of these reshaped into a named, wire
// compatible request, ProductState's own case being the harder one
// specifically because this package deliberately keeps ProductState
// itself opaque, an any, see State's own doc comment, and a generic
// daemon package cannot decide on its own how to serialize a type it
// is never told the shape of. That reshaping is deliberately left to
// whichever later phase picks the first real handler to wire, hostname
// most likely, once there is a real, concrete request to design
// around rather than a speculative one.
type DaemonClient interface {
	// MutateProductState runs fn against the current ProductState and
	// returns whatever fn itself returned.
	MutateProductState(fn func(any) (any, error)) (any, error)

	// MutateLevels runs fn against the current *command.TreeStructure
	// and returns whatever fn itself returned.
	MutateLevels(fn func(*command.TreeStructure) (any, error)) (any, error)

	// MutateUsers runs fn against the current auth.Users and returns
	// whatever fn itself returned.
	MutateUsers(fn func(auth.Users) (any, error)) (any, error)

	// MutateRoles runs fn against the current *command.RoleSet and
	// returns whatever fn itself returned.
	MutateRoles(fn func(*command.RoleSet) (any, error)) (any, error)

	// ReplaceState replaces the entire canonical state wholesale with
	// newState, matching claude/DAEMON_ARCHITECTURE_DESIGN.md's own
	// "What reboot means once a real daemon exists" section: rereading
	// StartupConfigFile, UsersFile, and RolesFile from disk and
	// replacing canonical state outright, not merging field by field.
	// Loading newState from disk is the caller's own responsibility;
	// ReplaceState only performs the replacement itself. This is
	// deliberately a different name from command.DaemonClient's own
	// Reboot method, a bare, argument free "please reboot" trigger, see
	// that method's own doc comment in command/daemonclient.go: Go does
	// not allow one type to declare two methods sharing a name with
	// different signatures, and StandaloneClient below satisfies both
	// interfaces at once, so this method needed a name of its own,
	// ReplaceState chosen as the more literal description of what it
	// actually does, leaving Reboot itself as the name a handler
	// actually types against.
	ReplaceState(newState State) error

	// Close releases whatever resources this DaemonClient holds; a
	// StandaloneClient closes its own private Store. A caller should
	// call Close exactly once, when a session ends.
	Close() error
}

// StandaloneClient is the DaemonClient a CLI process uses when no
// daemon is configured, config.SystemConfig.DaemonSocketPath empty,
// exactly today's behavior: state lives only in this one process, for
// the life of this one connection, with no daemon involved at all. It
// wraps a private *Store[State], reusing this package's own single
// writer safety even though nothing outside this one CLI process ever
// touches it, rather than inventing a second, unsynchronized code path
// solely for the no-daemon case.
//
// A zero StandaloneClient is not ready to use; construct one with
// NewStandaloneClient.
type StandaloneClient struct {
	store *Store[State]
}

// NewStandaloneClient returns a ready to use StandaloneClient, its own
// private Store already running, starting from initial as the current
// state.
func NewStandaloneClient(initial State) *StandaloneClient {
	return &StandaloneClient{store: NewStore(initial)}
}

// MutateProductState implements DaemonClient.
func (c *StandaloneClient) MutateProductState(fn func(any) (any, error)) (any, error) {
	return c.store.Do(func(s *State) (any, error) {
		return fn(s.ProductState)
	})
}

// MutateLevels implements DaemonClient.
func (c *StandaloneClient) MutateLevels(fn func(*command.TreeStructure) (any, error)) (any, error) {
	return c.store.Do(func(s *State) (any, error) {
		return fn(s.Levels)
	})
}

// MutateUsers implements DaemonClient.
func (c *StandaloneClient) MutateUsers(fn func(auth.Users) (any, error)) (any, error) {
	return c.store.Do(func(s *State) (any, error) {
		return fn(s.Users)
	})
}

// MutateRoles implements DaemonClient.
func (c *StandaloneClient) MutateRoles(fn func(*command.RoleSet) (any, error)) (any, error) {
	return c.store.Do(func(s *State) (any, error) {
		return fn(s.Roles)
	})
}

// ReplaceState implements DaemonClient.
func (c *StandaloneClient) ReplaceState(newState State) error {
	_, err := c.store.Do(func(s *State) (any, error) {
		*s = newState
		return nil, nil
	})
	return err
}

// Close implements DaemonClient.
func (c *StandaloneClient) Close() error {
	c.store.Close()
	return nil
}

// ListUsers implements command.DaemonClient. A CLI process running
// standalone has no session registry at all, only ever knowing about
// its own one connection, so this always reports
// command.ErrDaemonNotConfigured; see that error's own doc comment
// for why a real deployment never actually reaches this in practice.
func (c *StandaloneClient) ListUsers() ([]command.SessionInfo, error) {
	return nil, command.ErrDaemonNotConfigured
}

// DisconnectUser implements command.DaemonClient, always reporting
// command.ErrDaemonNotConfigured for the same reason ListUsers does.
func (c *StandaloneClient) DisconnectUser(username, sessionID string) error {
	return command.ErrDaemonNotConfigured
}

// Reboot implements command.DaemonClient, always reporting
// command.ErrDaemonNotConfigured; standalone mode's own reboot
// behavior, rereading files and ending this one connection directly,
// runs entirely inside cmd/core/cmd_admin.go instead, falling back to
// exactly that the moment this method reports this same error. This
// is a different method from ReplaceState above, see DaemonClient's
// own doc comment on this file for why the two could not share one
// name.
func (c *StandaloneClient) Reboot() error {
	return command.ErrDaemonNotConfigured
}

// FarewellChannel implements command.DaemonClient. Standalone mode has
// no daemon that could ever push a farewell, so this always returns
// nil, a channel a select statement simply never chooses, matching
// this method's own doc comment in command/daemonclient.go.
func (c *StandaloneClient) FarewellChannel() <-chan string {
	return nil
}

var _ DaemonClient = (*StandaloneClient)(nil)

// StandaloneClient also satisfies command.DaemonClient, the narrower,
// Reboot- and Close-free interface AppContext.DaemonClient actually
// holds, declared in package command itself rather than here since
// package command cannot import package daemon back without a cycle;
// see command.DaemonClient's own doc comment in
// command/daemonclient.go for the full reasoning. Go's own interface
// satisfaction is structural, so this assertion needs no new code on
// StandaloneClient itself, only this line confirming the match at
// compile time.
var _ command.DaemonClient = (*StandaloneClient)(nil)
