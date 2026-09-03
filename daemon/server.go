// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package daemon

import (
	"crypto/ecdh"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/gologme/log"
	"github.com/gotermme/routercli/armorchan"
	"github.com/gotermme/routercli/auditlog"
)

// Server is a real RouterCLI daemon's own connection acceptor and per
// connection message dispatcher, the piece of code that actually
// implements claude/DAEMON_ARCHITECTURE_DESIGN.md's own "Message
// framing over the encrypted channel" section against real, accepted
// connections, reusing every other piece this package already built:
// a Listener, this daemon's own canonical *Store[State], a
// SessionDirectory, and an *auditlog.AuditLog to append AuditEvent
// messages to. A zero Server is not ready to use; construct one with
// NewServer.
type Server struct {
	listener      *Listener
	staticPrivate *ecdh.PrivateKey
	store         *Store[State]
	sessions      *SessionDirectory
	audit         *auditlog.AuditLog
	logger        *log.Logger

	// reload rebuilds a fresh State from whatever this daemon's own
	// StartupConfigFile, UsersFile, and RolesFile currently hold on
	// disk, the same replay reload already performs today, now
	// happening once, daemon side; see TriggerReboot. This is supplied
	// by the caller, routercli-daemon's own main package, rather than
	// implemented in this package directly, since building a fresh
	// ProductState needs command.LoadStartupConfig and a concrete
	// product package this generic package deliberately stays free of,
	// the same reasoning State.ProductState's own doc comment already
	// gives for keeping that field an opaque any.
	reload func() (State, error)
}

// NewServer returns a ready to use Server. listener and staticPrivate
// are what AcceptAndHandshake needs for every incoming connection;
// store is this daemon's own already running canonical state; sessions
// is this daemon's own already running session registry; audit is
// this daemon's own audit log, already Enabled if
// config.SystemConfig.AuditLogEnabled is on, standing in for what each
// CLI process opens for itself in standalone mode, see
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own "Daemon owned audit
// logging" section; reload is called by TriggerReboot to rebuild a
// fresh State from disk, see that method's own doc comment; logger
// receives a small amount of Debugln and Errorf tracing for connection
// lifecycle and reboot events, matching this project's own existing
// logging convention elsewhere.
func NewServer(listener *Listener, staticPrivate *ecdh.PrivateKey, store *Store[State], sessions *SessionDirectory, audit *auditlog.AuditLog, reload func() (State, error), logger *log.Logger) *Server {
	return &Server{
		listener:      listener,
		staticPrivate: staticPrivate,
		store:         store,
		sessions:      sessions,
		audit:         audit,
		reload:        reload,
		logger:        logger,
	}
}

// Serve accepts connections until this Server's own Listener stops
// accepting them, most commonly because Shutdown or the Listener's own
// Close was called, spawning one goroutine per accepted, already
// handshaken connection, see serveConnection. Serve returns once
// AcceptAndHandshake's own underlying Accept call fails; a connection
// that fails only its own armorchan handshake, the peer credential
// check already having passed, is simply not served, and Serve keeps
// accepting the next one, matching Listener.Accept's own "one bad
// connection attempt must never take the whole socket down" doc
// comment.
func (s *Server) Serve() {
	for {
		ch, conn, err := AcceptAndHandshake(s.listener, s.staticPrivate)
		if err != nil {
			return
		}
		go s.serveConnection(ch, conn)
	}
}

// Shutdown broadcasts a farewell with no text of its own to every
// currently attached session, matching a clean SIGTERM drain rather
// than a reboot, then closes this Server's own Listener, so Serve's
// own Accept loop returns and no new connection is accepted while
// every already attached one drains. A receiving RemoteClient reports
// an empty farewell text as FarewellDisconnected, "Line closed by
// remote host," see RemoteClient.readLoop, the same wording an
// ordinary targeted disconnect already uses, since nothing more
// specific than that applies to an orderly daemon shutdown than to any
// other daemon initiated close. This does not itself wait for every
// connection to actually finish closing; a caller wanting that waits
// on Serve itself returning, or its own equivalent bookkeeping,
// separately.
func (s *Server) Shutdown() error {
	_ = s.sessions.BroadcastFarewell("")
	return s.listener.Close()
}

// TriggerReboot rebuilds a fresh State through this Server's own
// reload function, replaces this daemon's own canonical state with it
// wholesale, then sends every currently attached session a "Device is
// rebooting" farewell, matching
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own "What reboot means once a
// real daemon exists" section. This is the one function both a
// privileged session's own KindRebootRequest, through serveConnection,
// and this daemon's own SIGHUP handler converge on, exactly as that
// section describes; SIGHUP calls this directly, with no requester of
// its own to answer first.
func (s *Server) TriggerReboot() error {
	newState, err := s.reload()
	if err != nil {
		return err
	}
	if err := s.replaceState(newState); err != nil {
		return err
	}
	return s.sessions.BroadcastFarewell(FarewellRebooting)
}

// replaceState swaps this daemon's own canonical state for newState
// wholesale, not merged field by field, the same "one function
// producing one canonical text" discipline State.Config's own doc
// comment already describes for this exact operation.
func (s *Server) replaceState(newState State) error {
	_, err := s.store.Do(func(st *State) (any, error) {
		*st = newState
		return nil, nil
	})
	return err
}

// sendMessage encodes kind and payload through EncodeMessage and
// writes it to ch, the small helper every response this file sends
// funnels through.
func (s *Server) sendMessage(ch *armorchan.Channel, kind MessageKind, payload any) error {
	raw, err := EncodeMessage(kind, payload)
	if err != nil {
		return err
	}
	return ch.Send(raw)
}

// serveConnection is this Server's own per connection goroutine body,
// started once by Serve for every accepted, already handshaken
// connection: read the connecting session's own KindHello, register it
// with this Server's own SessionDirectory, answer with its assigned
// session ID, then loop, dispatching every message that connection
// sends until it ends, whether through an ordinary KindGoodbye, a
// targeted KindFarewell this connection's own session was pushed, or
// the connection simply failing. A malformed message, one that fails
// to even decode, ends this one connection outright, matching
// armorchan's own "treat a faulted channel as fatal, close the
// connection" convention; it never brings down this Server or any
// other connection.
func (s *Server) serveConnection(ch *armorchan.Channel, conn net.Conn) {
	defer conn.Close()

	raw, err := ch.Receive()
	if err != nil {
		return
	}
	kind, body, err := DecodeMessage(raw)
	if err != nil {
		return
	}
	if kind != KindHello {
		return
	}
	var hello HelloPayload
	if err := json.Unmarshal(body, &hello); err != nil {
		return
	}

	sessionID, farewellCh, err := s.sessions.Register(hello.Username, "")
	if err != nil {
		if s.logger != nil {
			s.logger.Errorf("daemon: registering session for %q: %v", hello.Username, err)
		}
		return
	}
	defer func() { _ = s.sessions.Unregister(sessionID) }()

	if err := s.sendMessage(ch, KindHelloResponse, HelloResponsePayload{SessionID: sessionID}); err != nil {
		return
	}
	if s.logger != nil {
		s.logger.Debugln("DEBUG: daemon accepted session", sessionID, "for user", hello.Username)
	}

	// This connection's own farewell watcher: the one goroutine that
	// ever reads farewellCh, pushed to by SessionDirectory.Farewell or
	// BroadcastFarewell elsewhere, possibly from a different
	// connection's own goroutine entirely, a targeted "disconnect
	// user" or a reboot. Closing conn here is what makes this
	// connection's own blocked Receive call below return an error,
	// ending this function the same way any other connection failure
	// already does, so no separate bookkeeping is needed for that
	// path. doneCh stops this goroutine cleanly on every other exit
	// path, an ordinary KindGoodbye among them, so it never leaks past
	// this function returning.
	doneCh := make(chan struct{})
	defer close(doneCh)
	go func() {
		select {
		case text := <-farewellCh:
			_ = s.sendMessage(ch, KindFarewell, FarewellPayload{Text: text})
			conn.Close()
		case <-doneCh:
		}
	}()

	for {
		raw, err := ch.Receive()
		if err != nil {
			return
		}
		kind, body, err := DecodeMessage(raw)
		if err != nil {
			return
		}

		switch kind {
		case KindGoodbye:
			return

		case KindAuditEvent:
			var payload AuditEventPayload
			if err := json.Unmarshal(body, &payload); err != nil {
				return
			}
			s.audit.LogAt(payload.Time, payload.Username, payload.Command, payload.Success)
			_ = s.sessions.Touch(sessionID, payload.Level)

		case KindListUsersRequest:
			infos, lerr := s.sessions.List()
			if lerr != nil {
				return
			}
			if err := s.sendMessage(ch, KindListUsersResponse, ListUsersResponsePayload{Sessions: infos}); err != nil {
				return
			}

		case KindDisconnectUserRequest:
			var req DisconnectUserRequestPayload
			if err := json.Unmarshal(body, &req); err != nil {
				return
			}
			resp := DisconnectUserResponsePayload{}
			if _, ferr := s.sessions.Farewell(req.Username, req.SessionID, FarewellDisconnected); ferr != nil {
				resp.Error = s.disconnectErrorText(req.Username, ferr)
			}
			if err := s.sendMessage(ch, KindDisconnectUserResponse, resp); err != nil {
				return
			}

		case KindRebootRequest:
			newState, rerr := s.reload()
			if rerr == nil {
				rerr = s.replaceState(newState)
			}
			if rerr != nil {
				_ = s.sendMessage(ch, KindRebootResponse, RebootResponsePayload{Error: rerr.Error()})
				continue
			}
			if err := s.sendMessage(ch, KindRebootResponse, RebootResponsePayload{}); err != nil {
				return
			}
			if s.logger != nil {
				s.logger.Debugln("DEBUG: daemon reboot triggered by session", sessionID, "for user", hello.Username)
			}
			_ = s.sessions.BroadcastFarewell(FarewellRebooting)

		default:
			// An unrecognized or out of place message kind, KindHello a
			// second time on an already established connection among
			// them, is treated the same as any other malformed input:
			// fatal to this one connection, never to this Server.
			return
		}
	}
}

// disconnectErrorText turns ferr, whatever SessionDirectory.Farewell
// returned, into the text a requesting session's own "disconnect user"
// handler prints. ErrAmbiguousSession specifically is expanded into a
// message naming the actual candidate session IDs, resolved fresh
// through s.sessions.List, matching
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own disambiguation rule: "the
// daemon answers with an error listing the matching session IDs rather
// than guessing which one was meant." Every other error, most commonly
// ErrNoMatchingSession, is reported as is.
func (s *Server) disconnectErrorText(username string, ferr error) string {
	if !errors.Is(ferr, ErrAmbiguousSession) {
		return ferr.Error()
	}
	infos, err := s.sessions.List()
	if err != nil {
		return ferr.Error()
	}
	var ids []string
	for _, info := range infos {
		if info.Username == username {
			ids = append(ids, info.ID)
		}
	}
	return fmt.Sprintf("more than one session for %q, specify a session ID: %s", username, strings.Join(ids, ", "))
}
