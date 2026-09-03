// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package daemon

import (
	"crypto/ecdh"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/gotermme/routercli/armorchan"
)

// socketPermissions is the file mode a real daemon's own Unix domain
// socket is created with, restrictive by default, matching
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own "Transport" section: "The
// daemon creates it with restrictive permissions, 0600 by default,
// owned by whatever account the daemon itself runs as." The peer
// credential check in Accept is a second, independent layer above
// this, never a substitute for it; see PeerCredentialChecker's own doc
// comment.
const socketPermissions = 0o600

// Listener is a RouterCLI daemon's own Unix domain socket, opened by
// Listen, every accepted connection checked against a
// PeerCredentialChecker before a single byte of any application
// protocol above it is read. A zero Listener is not ready to use;
// construct one with Listen.
type Listener struct {
	path     string
	listener *net.UnixListener
	checker  PeerCredentialChecker
}

// Listen opens a Unix domain socket at path, this daemon's own
// config.SystemConfig.DaemonSocketPath, with permissions restricted to
// socketPermissions, and returns a Listener ready to Accept
// connections checked against checker. A stale socket file left
// behind by a daemon that exited without cleaning up after itself,
// killed outright for instance, see
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own "Settled: what happens
// when the daemon is killed outright" section, is removed before
// binding, since net.ListenUnix itself refuses to bind a path that
// already exists; Listen does not attempt to tell a genuinely stale
// file apart from a socket some other, still-live process is actually
// using, the same limitation an operator starting any other daemon
// twice by mistake against the same socket path already has to avoid
// on their own.
func Listen(path string, checker PeerCredentialChecker) (*Listener, error) {
	if checker == nil {
		return nil, errors.New("daemon: Listen requires a non-nil PeerCredentialChecker")
	}

	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("daemon: removing a stale socket file at %s: %w", path, err)
	}

	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, fmt.Errorf("daemon: resolving socket path %s: %w", path, err)
	}
	ln, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, fmt.Errorf("daemon: listening on %s: %w", path, err)
	}
	if err := os.Chmod(path, socketPermissions); err != nil {
		ln.Close()
		return nil, fmt.Errorf("daemon: setting permissions on %s: %w", path, err)
	}

	return &Listener{path: path, listener: ln, checker: checker}, nil
}

// Accept blocks until a new connection arrives, checks its peer
// credential against this Listener's own PeerCredentialChecker before
// returning it, and refuses, closing outright, any connection whose
// peer the checker does not allow; a refused connection is never
// returned to the caller, and Accept simply waits for the next one
// instead of returning an error for it, the same way a single bad
// connection attempt should not be mistaken for the Listener itself
// having failed. Accept returns an error only when the Listener's own
// underlying accept fails, most commonly because Close was called.
func (l *Listener) Accept() (net.Conn, error) {
	for {
		conn, err := l.listener.AcceptUnix()
		if err != nil {
			return nil, fmt.Errorf("daemon: accept: %w", err)
		}
		if err := l.checker.CheckPeer(conn); err != nil {
			// A refused peer is not this Listener's own failure; log
			// the reason at the caller's own discretion by simply
			// moving on, exactly the "keep listening" behavior a real
			// daemon needs so one hostile or misconfigured connection
			// attempt can never take the whole socket down.
			conn.Close()
			continue
		}
		return conn, nil
	}
}

// Close stops accepting new connections on this Listener and removes
// its own socket file from disk, so a later Listen against the same
// path does not need to treat this Listener's own, cleanly closed
// socket as a stale file left behind by a crash.
func (l *Listener) Close() error {
	err := l.listener.Close()
	if rmErr := os.Remove(l.path); rmErr != nil && !os.IsNotExist(rmErr) {
		if err == nil {
			err = rmErr
		}
	}
	return err
}

// AcceptAndHandshake calls Accept, then immediately runs
// armorchan.ServerHandshake over the accepted connection using
// daemonStaticPrivate as this daemon's own persisted static identity,
// returning a ready to use *armorchan.Channel. A connection that fails
// its own peer credential check inside Accept never reaches the
// handshake at all; a connection that passes the peer credential check
// but then fails the handshake itself, see armorchan's own
// ErrHandshakeFailed, is closed here and reported as this call's own
// error, rather than returned to the caller half-connected.
func AcceptAndHandshake(l *Listener, daemonStaticPrivate *ecdh.PrivateKey) (*armorchan.Channel, net.Conn, error) {
	conn, err := l.Accept()
	if err != nil {
		return nil, nil, err
	}
	ch, err := armorchan.ServerHandshake(conn, daemonStaticPrivate)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("daemon: handshake with accepted connection: %w", err)
	}
	return ch, conn, nil
}

// Dial connects to a RouterCLI daemon's own Unix domain socket at
// path, this CLI process's own config.SystemConfig.DaemonSocketPath,
// and runs armorchan.ClientHandshake against expectedDaemonStaticPublic,
// this daemon's own known static public key, most simply read from the
// world readable key file the daemon itself writes at startup; see
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own "The handshake" section.
// Dial returns armorchan.ErrHandshakeFailed, wrapped, rather than any
// Channel, if whatever answered on path could not prove it holds the
// private key matching expectedDaemonStaticPublic.
func Dial(path string, expectedDaemonStaticPublic *ecdh.PublicKey) (*armorchan.Channel, net.Conn, error) {
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, nil, fmt.Errorf("daemon: resolving socket path %s: %w", path, err)
	}
	conn, err := net.DialUnix("unix", nil, addr)
	if err != nil {
		return nil, nil, fmt.Errorf("daemon: dialing %s: %w", path, err)
	}
	ch, err := armorchan.ClientHandshake(conn, expectedDaemonStaticPublic)
	if err != nil {
		conn.Close()
		return nil, nil, fmt.Errorf("daemon: handshake with %s: %w", path, err)
	}
	return ch, conn, nil
}
