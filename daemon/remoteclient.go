// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/gotermme/routercli/armorchan"
	"github.com/gotermme/routercli/command"
)

// ConnectionLostText is the farewell text a RemoteClient reports on
// its own FarewellChannel when the connection to the daemon ends with
// no explicit KindFarewell push at all, the daemon killed outright,
// kill -9, being the case claude/DAEMON_ARCHITECTURE_DESIGN.md names
// directly under "Settled: what happens when the daemon is killed
// outright": "A CLI process that loses its connection to the daemon
// without a proper Goodbye or reboot farewell message prints an
// honest error, something in the shape of 'Connection to routercli
// daemon lost'." This is the one text this package sends for every
// such case, whatever actually caused the connection to fail, a
// crashed daemon, a network level error on the socket, or a
// structurally malformed message this client itself could not
// decode; every one of those is, from a CLI session's own point of
// view, the same "the daemon is simply gone" situation, and none of
// them earns its own, separately worded message.
const ConnectionLostText = "Connection to routercli daemon lost"

// rawMessage is one decoded, but not yet unmarshaled, frame this
// package's own readLoop handed off to whichever call is waiting for
// it: a MessageKind and the JSON body that followed it.
type rawMessage struct {
	kind MessageKind
	body []byte
}

// RemoteClient is a CLI process's own live connection to a real
// RouterCLI daemon, wrapping an already handshaken *armorchan.Channel.
// A single background goroutine, started by NewRemoteClient, is the
// only caller of Channel.Receive for the life of this RemoteClient;
// every exported method above sends through Channel.Send instead,
// serialized by this type's own sendMu, and, for a request expecting
// an answer, waits for that background goroutine to hand the response
// back. This project's own CLI design sends at most one request at a
// time, an ordinary command dispatch loop, never more than one
// command actually running at once, so this type deliberately
// correlates a response to whatever call is currently waiting rather
// than tagging every message with its own request ID; see call's own
// doc comment.
//
// A zero RemoteClient is not ready to use; construct one with
// NewRemoteClient. RemoteClient satisfies command.Auditor directly,
// Log, WouldLog, and ForceLog, forwarding each as a KindAuditEvent
// message rather than writing to a local file the way auditlog.AuditLog
// does, matching claude/DAEMON_ARCHITECTURE_DESIGN.md's own "Daemon
// owned audit logging" section. It also satisfies the four,
// non-Mutate methods command.DaemonClient gained in phase five,
// ListUsers, DisconnectUser, Reboot, and FarewellChannel; see
// ConnectedClient in connectedclient.go for how a real deployment
// combines this with a StandaloneClient to satisfy the whole
// interface, Mutate methods included, that this type alone does not.
type RemoteClient struct {
	ch   *armorchan.Channel
	conn net.Conn

	// sendMu serializes every call to ch.Send, both an ordinary call
	// waiting on a response and a one way send with none expected,
	// audit forwarding and Goodbye among them, so two goroutines can
	// never interleave two messages into one another on the wire; see
	// Channel's own doc comment on why Send itself allows only one
	// caller at a time per direction.
	sendMu sync.Mutex

	// respCh receives exactly one rawMessage per call awaiting a
	// response, handed off by readLoop; buffered by one so readLoop
	// never blocks handing one off even if call itself has not reached
	// its own select yet.
	respCh chan rawMessage

	// farewell is the channel FarewellChannel returns, buffered by one,
	// the same non-blocking, single slot convention
	// PendingReload.FireChannel's own doc comment already establishes
	// for a different signal.
	farewell chan string
	// done closes exactly once, the moment this connection is known to
	// be over, whether that is a genuine KindFarewell push or an
	// ordinary Receive error; see fail and readLoop.
	done     chan struct{}
	doneOnce sync.Once

	closeOnce sync.Once

	sessionID string

	// auditMu guards auditEnabled and level together, both small,
	// audit adjacent pieces of state a caller updates from outside
	// this type's own background goroutine; see Enable, Disable, and
	// SetLevel.
	auditMu      sync.Mutex
	auditEnabled bool
	level        string
}

// NewRemoteClient takes an already handshaken *armorchan.Channel, see
// Dial in socket.go, and its underlying net.Conn, starts this type's
// own background read loop, and performs the KindHello exchange every
// connection begins with: username, this process's own PID, and
// terminal, whatever a terminal identifier looks like on this
// platform, left to the caller to determine rather than guessed here,
// keeping this package free of any one platform's own concept of a
// terminal identifier. NewRemoteClient blocks until the daemon answers
// with a session ID, see SessionID, or returns an error, closing conn
// itself before returning one.
func NewRemoteClient(ch *armorchan.Channel, conn net.Conn, username, terminal string) (*RemoteClient, error) {
	c := &RemoteClient{
		ch:       ch,
		conn:     conn,
		respCh:   make(chan rawMessage, 1),
		farewell: make(chan string, 1),
		done:     make(chan struct{}),
	}
	go c.readLoop()

	kind, body, err := c.call(KindHello, HelloPayload{Username: username, PID: os.Getpid(), Terminal: terminal})
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("daemon: sending Hello: %w", err)
	}
	if kind != KindHelloResponse {
		c.Close()
		return nil, fmt.Errorf("daemon: Hello: unexpected response kind %s", kind)
	}
	var resp HelloResponsePayload
	if err := json.Unmarshal(body, &resp); err != nil {
		c.Close()
		return nil, fmt.Errorf("daemon: decoding HelloResponse: %w", err)
	}
	c.sessionID = resp.SessionID
	return c, nil
}

// SessionID returns the session ID the daemon assigned this
// connection in its own HelloResponse, the same short identifier
// ListUsers reports for this session.
func (c *RemoteClient) SessionID() string {
	return c.sessionID
}

// readLoop is the one goroutine that ever calls c.ch.Receive, started
// by NewRemoteClient and running for the life of this RemoteClient.
// Every frame it decodes is either a KindFarewell push, handled
// entirely here, or handed off to whichever call is currently
// waiting on respCh; this connection is expected to carry at most one
// outstanding call at a time, see this type's own doc comment, so a
// frame that is not a farewell push is always the answer to whatever
// call is currently blocked in its own select.
func (c *RemoteClient) readLoop() {
	for {
		raw, err := c.ch.Receive()
		if err != nil {
			c.fail(ConnectionLostText)
			return
		}

		kind, body, err := DecodeMessage(raw)
		if err != nil {
			c.fail(ConnectionLostText)
			return
		}

		if kind == KindFarewell {
			text := FarewellDisconnected
			var p FarewellPayload
			if jsonErr := json.Unmarshal(body, &p); jsonErr == nil && p.Text != "" {
				text = p.Text
			}
			c.fail(text)
			return
		}

		select {
		case c.respCh <- rawMessage{kind: kind, body: body}:
		case <-c.done:
			return
		}
	}
}

// fail marks this connection over, delivering text on farewell,
// exactly once no matter how many times or from which goroutine it is
// called; both readLoop's own two exit paths, a genuine farewell push
// and an ordinary connection failure, converge on this one method, so
// FarewellChannel always receives exactly one value for the one way
// this connection can end, never zero and never more than one.
func (c *RemoteClient) fail(text string) {
	c.doneOnce.Do(func() {
		select {
		case c.farewell <- text:
		default:
			// farewell is buffered by exactly one; this can only be
			// reached if something already delivered a value that
			// nothing has read yet, which doneOnce itself already
			// guarantees never happens twice.
		}
		close(c.done)
	})
}

// call sends kind and payload, encoded through EncodeMessage, and
// blocks until either readLoop hands back the response it produced or
// this connection ends first, whichever happens first. See this
// type's own doc comment for why this connection is never expected to
// carry more than one outstanding call at once, and sendMu for what
// enforces that in practice.
func (c *RemoteClient) call(kind MessageKind, payload any) (MessageKind, []byte, error) {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	raw, err := EncodeMessage(kind, payload)
	if err != nil {
		return 0, nil, err
	}
	if err := c.ch.Send(raw); err != nil {
		c.fail(ConnectionLostText)
		return 0, nil, fmt.Errorf("daemon: sending a %s request: %w", kind, err)
	}

	select {
	case resp := <-c.respCh:
		return resp.kind, resp.body, nil
	case <-c.done:
		return 0, nil, errors.New("daemon: connection to the daemon ended before a response arrived")
	}
}

// send sends kind and payload the same way call does, for a message
// this package's own catalog documents as a one way notification, no
// response ever expected, KindGoodbye and KindAuditEvent among them,
// so this returns as soon as the write itself succeeds rather than
// waiting on anything readLoop might hand back.
func (c *RemoteClient) send(kind MessageKind, payload any) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	raw, err := EncodeMessage(kind, payload)
	if err != nil {
		return err
	}
	if err := c.ch.Send(raw); err != nil {
		c.fail(ConnectionLostText)
		return fmt.Errorf("daemon: sending a %s message: %w", kind, err)
	}
	return nil
}

// ListUsers implements command.DaemonClient, sending a
// KindListUsersRequest and converting the daemon's own
// ListUsersResponsePayload into command.SessionInfo, computing each
// session's own IdleFor against time.Now at the moment this response
// is actually handled, rather than at whatever earlier moment the
// daemon itself built it, so a slow round trip never makes a session
// look less idle than it really is.
func (c *RemoteClient) ListUsers() ([]command.SessionInfo, error) {
	kind, body, err := c.call(KindListUsersRequest, struct{}{})
	if err != nil {
		return nil, err
	}
	if kind != KindListUsersResponse {
		return nil, fmt.Errorf("daemon: ListUsers: unexpected response kind %s", kind)
	}
	var resp ListUsersResponsePayload
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("daemon: decoding ListUsersResponse: %w", err)
	}

	now := time.Now()
	out := make([]command.SessionInfo, 0, len(resp.Sessions))
	for _, s := range resp.Sessions {
		out = append(out, command.SessionInfo{
			ID:           s.ID,
			Username:     s.Username,
			CommandLevel: s.CommandLevel,
			ConnectedAt:  s.ConnectedAt,
			IdleFor:      now.Sub(s.LastActivity),
		})
	}
	return out, nil
}

// DisconnectUser implements command.DaemonClient, sending a
// KindDisconnectUserRequest and turning a non-empty
// DisconnectUserResponsePayload.Error into a plain error value, the
// ambiguous session case, ErrAmbiguousSession on the daemon's own
// side, included.
func (c *RemoteClient) DisconnectUser(username, sessionID string) error {
	kind, body, err := c.call(KindDisconnectUserRequest, DisconnectUserRequestPayload{Username: username, SessionID: sessionID})
	if err != nil {
		return err
	}
	if kind != KindDisconnectUserResponse {
		return fmt.Errorf("daemon: DisconnectUser: unexpected response kind %s", kind)
	}
	var resp DisconnectUserResponsePayload
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("daemon: decoding DisconnectUserResponse: %w", err)
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

// Reboot implements command.DaemonClient, sending a KindRebootRequest
// and reporting only whether the daemon accepted it; this session's
// own ending, along with every other attached session's, arrives
// separately, as a KindFarewell push readLoop turns into
// FarewellChannel firing, not as this call's own return. See
// command.DaemonClient's own doc comment on Reboot for why this takes
// no state of its own to hand across.
func (c *RemoteClient) Reboot() error {
	kind, body, err := c.call(KindRebootRequest, struct{}{})
	if err != nil {
		return err
	}
	if kind != KindRebootResponse {
		return fmt.Errorf("daemon: Reboot: unexpected response kind %s", kind)
	}
	var resp RebootResponsePayload
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("daemon: decoding RebootResponse: %w", err)
	}
	if resp.Error != "" {
		return errors.New(resp.Error)
	}
	return nil
}

// FarewellChannel implements command.DaemonClient; see fail for the
// one path, a genuine push from the daemon or an ordinary connection
// failure, that ever delivers a value here, always exactly once for
// the life of this RemoteClient.
func (c *RemoteClient) FarewellChannel() <-chan string {
	return c.farewell
}

// SetLevel records the Command Level this session is currently at,
// included in every AuditEvent message Log and ForceLog send from
// this point on, matching claude/DAEMON_ARCHITECTURE_DESIGN.md's own
// AuditEvent description: "the acting username, the command line as
// typed, the Command Level it ran at, a timestamp, and its outcome."
// command.Auditor's own Log and ForceLog methods carry no level
// parameter of their own, so whatever wires a RemoteClient into a
// real AppContext calls SetLevel wherever ctx.Session.CommandLevel
// itself changes, main.go's own doing, a later phase's own work; a
// RemoteClient that never has SetLevel called on it simply reports an
// empty Level, exactly as good as no level at all rather than a wrong
// one.
func (c *RemoteClient) SetLevel(level string) {
	c.auditMu.Lock()
	defer c.auditMu.Unlock()
	c.level = level
}

// Enable, Disable, and Enabled give a RemoteClient the same three
// method shape auditlog.AuditLog already carries, so
// cmd/core/cmd_audit.go's own "audit-log enable", "audit-log
// disable", and "audit-log status" commands can operate against
// either concrete type behind ctx.Audit through one small, locally
// declared interface, rather than a type assertion naming
// *auditlog.AuditLog specifically; see that file's own doc comment
// once it is updated to do so, later phase work. Unlike
// auditlog.AuditLog, Enable here can never fail, no file to open, so
// it always returns a nil error; the signature still returns one so
// the same small interface covers both types without either needing
// an adapter.
func (c *RemoteClient) Enable() error {
	c.auditMu.Lock()
	defer c.auditMu.Unlock()
	c.auditEnabled = true
	return nil
}

func (c *RemoteClient) Disable() {
	c.auditMu.Lock()
	defer c.auditMu.Unlock()
	c.auditEnabled = false
}

func (c *RemoteClient) Enabled() bool {
	c.auditMu.Lock()
	defer c.auditMu.Unlock()
	return c.auditEnabled
}

// WouldLog implements command.Auditor, reporting whether Log would
// actually send anything; unlike auditlog.AuditLog, there is no
// separate "file open" condition to check here, so this is simply
// Enabled.
func (c *RemoteClient) WouldLog() bool {
	return c.Enabled()
}

// Log implements command.Auditor, forwarding username, command, and
// success as a KindAuditEvent message, together with this
// RemoteClient's own current level, see SetLevel, and the current
// time, the moment this call actually runs, matching
// auditlog.AuditLog.Log's own "the timestamp uses RFC3339...unambiguous
// across time zones" reasoning, just carried here as a time.Time
// rather than pre-formatted text, since the daemon, not this client,
// is what eventually calls auditlog.FormatEntry against it. Log does
// nothing when WouldLog is false, matching auditlog.AuditLog.Log's own
// behavior exactly; a transient send failure here is reported to the
// caller nowhere, matching that same method's own "an audit log
// failure must never crash the CLI" reasoning, since command.Auditor's
// own Log signature has no error return for this method to give one
// through.
func (c *RemoteClient) Log(username, cmdText string, success bool) {
	if !c.WouldLog() {
		return
	}
	c.sendAuditEvent(username, cmdText, success)
}

// ForceLog implements command.Auditor, sending unconditionally,
// skipping the WouldLog check Log performs, matching
// auditlog.AuditLog.ForceLog's own doc comment and its one real use,
// a command whose own side effect flips audit logging off, "audit-log
// disable" itself, needing its own entry sent despite that.
func (c *RemoteClient) ForceLog(username, cmdText string, success bool) {
	c.sendAuditEvent(username, cmdText, success)
}

func (c *RemoteClient) sendAuditEvent(username, cmdText string, success bool) {
	c.auditMu.Lock()
	level := c.level
	c.auditMu.Unlock()

	// A transient send failure here must never surface to Log or
	// ForceLog's own caller, neither of which has an error return to
	// give one through; see this method's own two callers' doc
	// comments. A genuine connection failure is already reported
	// through FarewellChannel by send itself, via fail, which is the
	// channel this project's own runLoop actually watches for exactly
	// this kind of fatal, unrecoverable condition.
	_ = c.send(KindAuditEvent, AuditEventPayload{
		Username: username,
		Command:  cmdText,
		Level:    level,
		Time:     time.Now(),
		Success:  success,
	})
}

// Close sends a best effort KindGoodbye, matching
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own "sent when a session ends
// normally" description, then closes the underlying connection. A
// failure sending Goodbye does not prevent the connection from being
// closed; closing conn itself is what actually matters, and it is
// what makes readLoop's own blocked Receive call return, driving fail
// and closing done, exactly the same cleanup an unexpected connection
// failure already triggers, so Close needs no separate bookkeeping of
// its own for that. Safe to call more than once; only the first call
// does anything.
//
// Close skips sending Goodbye at all when done is already closed, a
// farewell already received or a failure already observed by the time
// Close runs: Goodbye means a session ending normally, which is no
// longer true once this connection is already known to have ended
// some other way, and attempting it anyway would mean sending into a
// connection nothing may still be reading on the daemon's own side.
func (c *RemoteClient) Close() error {
	var err error
	c.closeOnce.Do(func() {
		select {
		case <-c.done:
		default:
			_ = c.send(KindGoodbye, struct{}{})
		}
		err = c.conn.Close()
	})
	return err
}

var (
	_ command.Auditor = (*RemoteClient)(nil)
)
