// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package daemon

import (
	"github.com/gotermme/routercli/command"
)

// ConnectedClient is the command.DaemonClient a CLI process configured
// with a real daemon, config.SystemConfig.DaemonSocketPath non-empty,
// is wired up to, once a later phase actually dials one from main.go.
// It embeds a *StandaloneClient for the four Mutate methods and a
// *RemoteClient for the four this package's own phase five added,
// ListUsers, DisconnectUser, Reboot, and FarewellChannel, so together
// the two halves satisfy command.DaemonClient in full.
//
// This split is deliberate, not an oversight: no cmd/core or
// cmd/product handler beyond hostname has been reshaped into a wire
// compatible request yet, see command.DaemonClient's own doc comment
// in command/daemonclient.go, so ProductState, Levels, Users, and
// Roles genuinely still live only in this one CLI process's own
// memory even once a real daemon is configured, exactly what
// StandaloneClient already gives, a known, disclosed limitation this
// phase does not close. Session and connection tracking, the actual
// subject of phase five, ListUsers, DisconnectUser, Reboot, and
// FarewellChannel, are genuinely wired across a real socket instead,
// through the embedded RemoteClient. A later phase reshaping another
// handler, hostname's own eventual real remote wiring among them, is
// expected to replace the embedded StandaloneClient's role here
// piece by piece, not to replace this type outright.
//
// A zero ConnectedClient is not ready to use; construct one with
// NewConnectedClient.
type ConnectedClient struct {
	*StandaloneClient
	*RemoteClient
}

// NewConnectedClient combines an already constructed StandaloneClient
// and RemoteClient into one command.DaemonClient. Neither is
// constructed here; a caller builds each its own way, a
// StandaloneClient from whatever local state this CLI process loaded
// at its own startup exactly as it always has, a RemoteClient from an
// already dialed and handshaken connection to the real daemon, see
// NewRemoteClient, then hands both to this function once, the same
// division main.go's own startup sequence already makes between
// loading local state and wiring a DaemonClient around it.
func NewConnectedClient(standalone *StandaloneClient, remote *RemoteClient) *ConnectedClient {
	return &ConnectedClient{StandaloneClient: standalone, RemoteClient: remote}
}

// Close closes both halves of this ConnectedClient, the embedded
// StandaloneClient's own private Store and the embedded RemoteClient's
// own connection to the daemon. This method, and the four immediately
// below it, ListUsers, DisconnectUser, Reboot, and FarewellChannel,
// are all declared explicitly here, rather than left to Go's own
// embedding promotion, because StandaloneClient's own defensive
// implementations of those same four methods, see client.go, share
// their exact names and signatures with RemoteClient's real ones; Go
// does not promote a method name shared by more than one embedded
// type at the same depth at all, an ambiguous selector, so every one
// of the five needs its own explicit, one line forwarding method here
// regardless of whether a caller would ever have reached
// StandaloneClient's own version by accident.
func (c *ConnectedClient) Close() error {
	standaloneErr := c.StandaloneClient.Close()
	remoteErr := c.RemoteClient.Close()
	if standaloneErr != nil {
		return standaloneErr
	}
	return remoteErr
}

// ListUsers forwards to the embedded RemoteClient, the real, wire
// connected implementation; see this type's own doc comment on Close
// for why this cannot simply be left to Go's own embedding promotion.
func (c *ConnectedClient) ListUsers() ([]command.SessionInfo, error) {
	return c.RemoteClient.ListUsers()
}

// DisconnectUser forwards to the embedded RemoteClient; see ListUsers.
func (c *ConnectedClient) DisconnectUser(username, sessionID string) error {
	return c.RemoteClient.DisconnectUser(username, sessionID)
}

// Reboot forwards to the embedded RemoteClient; see ListUsers.
func (c *ConnectedClient) Reboot() error {
	return c.RemoteClient.Reboot()
}

// FarewellChannel forwards to the embedded RemoteClient; see
// ListUsers.
func (c *ConnectedClient) FarewellChannel() <-chan string {
	return c.RemoteClient.FarewellChannel()
}

var _ command.DaemonClient = (*ConnectedClient)(nil)
