// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// MessageKind identifies which of this package's own wire message
// shapes a byte slice carries, matching
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own "Message framing over
// the encrypted channel" section: "each logical message is a one byte
// type tag followed by a length prefixed JSON payload." The length
// prefixing itself is armorchan.Channel's own doing, one call to Send
// writes exactly one frame and one call to Receive reads exactly one
// frame back, so this package's own EncodeMessage and DecodeMessage
// below need only produce and consume the one byte tag plus the JSON
// payload that frame carries, never a length of their own.
type MessageKind byte

const (
	// KindHello is sent once, by a CLI client, immediately after the
	// armorchan handshake completes: this session's own username, once
	// locally authenticated, its process ID, and whatever a terminal
	// identifier looks like on this platform, HelloPayload's own three
	// fields. The daemon answers with KindHelloResponse, assigning this
	// session's own daemon side session ID, everything ListUsers will
	// later report about this one session.
	KindHello MessageKind = iota + 1
	KindHelloResponse

	// KindGoodbye is sent by a CLI client when its own session ends
	// normally, "exit" at the base Command Level for instance. This is
	// a one way notification; the daemon does not answer it, simply
	// removing this session from its own registry and closing the
	// connection, the same cleanup it performs when a connection closes
	// without one ever arriving at all, kill -9 on the CLI process
	// itself for instance.
	KindGoodbye

	// KindAuditEvent is sent once per dispatched command, matching
	// claude/DAEMON_ARCHITECTURE_DESIGN.md's own "Daemon owned audit
	// logging" section: the acting username, the command line as
	// typed, the Command Level it ran at, a timestamp, and its
	// outcome. This is a one way notification, no response expected,
	// and does not block the CLI's own prompt from returning.
	KindAuditEvent

	// KindListUsersRequest carries no payload of its own; a CLI client
	// sends this to ask the daemon for one entry per currently attached
	// session. The daemon answers with KindListUsersResponse.
	KindListUsersRequest
	KindListUsersResponse

	// KindDisconnectUserRequest carries a username and an optional
	// session ID, sent only by a session whose own role clears the same
	// gate reboot itself carries; see DisconnectUserRequestPayload's
	// own doc comment for the disambiguation rule this implements. The
	// daemon answers with KindDisconnectUserResponse.
	KindDisconnectUserRequest
	KindDisconnectUserResponse

	// KindRebootRequest carries no payload of its own; a sufficiently
	// privileged CLI session sends this to ask the daemon to reread its
	// own canonical state from disk and end every attached session,
	// including the one that asked, with a "Device is rebooting"
	// farewell. The daemon answers with KindRebootResponse once the
	// reboot has been accepted and is underway; the actual session
	// ending arrives separately, as a KindFarewell push, not as this
	// response, since every attached session, not only the requester,
	// needs one.
	KindRebootRequest
	KindRebootResponse

	// KindFarewell is a push, sent by the daemon to one CLI client
	// without that client ever having asked for it, immediately before
	// the daemon closes that client's own connection: a distinct farewell
	// message, "Device is rebooting" for a reboot, "Line closed by
	// remote host" for a targeted disconnect user, so the person
	// watching their own terminal understands why the connection ended.
	// See FarewellPayload.
	KindFarewell
)

// String reports a MessageKind's own name, for logging and for a test
// failure message naming which kind of message a decode actually
// produced, rather than only its raw numeric value.
func (k MessageKind) String() string {
	switch k {
	case KindHello:
		return "Hello"
	case KindHelloResponse:
		return "HelloResponse"
	case KindGoodbye:
		return "Goodbye"
	case KindAuditEvent:
		return "AuditEvent"
	case KindListUsersRequest:
		return "ListUsersRequest"
	case KindListUsersResponse:
		return "ListUsersResponse"
	case KindDisconnectUserRequest:
		return "DisconnectUserRequest"
	case KindDisconnectUserResponse:
		return "DisconnectUserResponse"
	case KindRebootRequest:
		return "RebootRequest"
	case KindRebootResponse:
		return "RebootResponse"
	case KindFarewell:
		return "Farewell"
	default:
		return fmt.Sprintf("MessageKind(%d)", byte(k))
	}
}

// ErrMalformedMessage is returned by DecodeMessage when raw is too
// short to even carry a one byte type tag, or by a caller's own
// per-kind json.Unmarshal call failing against the payload that
// followed it; every caller of DecodeMessage MUST treat this as an
// ordinary, recoverable protocol error, closing the one connection
// that produced it rather than the whole daemon, matching how this
// project already treats a faulted armorchan.Channel. See
// FuzzDecodeMessage in protocol_test.go, exercising exactly this
// against truncated, oversized, and structurally malformed input,
// confirming none of it ever panics.
var ErrMalformedMessage = errors.New("daemon: malformed protocol message")

// HelloPayload is KindHello's own payload, sent once by a CLI client
// right after the handshake completes, everything the daemon needs to
// register this session and everything ListUsers will later report
// about it beyond what the daemon itself assigns.
type HelloPayload struct {
	Username string
	PID      int
	Terminal string
}

// HelloResponsePayload is KindHelloResponse's own payload, the
// daemon's own answer to a Hello: the session ID this connection is
// known by for the rest of its own life, named in a later
// DisconnectUserRequest, and printed by ListUsers.
type HelloResponsePayload struct {
	SessionID string
}

// AuditEventPayload is KindAuditEvent's own payload, recording exactly
// what auditlog.AuditLog already records today, moved here so a real
// daemon deployment can be the one process actually writing
// AuditLogFile; see auditlog.FormatEntry, which a real daemon's own
// message handler calls directly against these same four fields.
type AuditEventPayload struct {
	Username string
	Command  string
	Level    string
	Time     time.Time
	Success  bool
}

// ListUsersResponsePayload is KindListUsersResponse's own payload, one
// SessionInfo per currently attached session, sorted by ConnectedAt,
// oldest first, matching SessionDirectory.List's own doc comment.
type ListUsersResponsePayload struct {
	Sessions []SessionInfo
}

// DisconnectUserRequestPayload is KindDisconnectUserRequest's own
// payload. When SessionID is empty and exactly one session matches
// Username, the daemon closes that connection outright. When more
// than one session matches Username and SessionID is empty, the
// daemon answers KindDisconnectUserResponse with its own Error field
// set, naming the candidate session IDs, rather than guessing which
// one was meant; see SessionDirectory.Farewell and ErrAmbiguousSession.
type DisconnectUserRequestPayload struct {
	Username  string
	SessionID string
}

// DisconnectUserResponsePayload is KindDisconnectUserResponse's own
// payload. Error is empty on success; a non-empty Error, the
// ambiguous session case above among them, means no session was
// disconnected.
type DisconnectUserResponsePayload struct {
	Error string
}

// RebootResponsePayload is KindRebootResponse's own payload,
// acknowledging that a reboot was accepted and is underway. Error is
// empty on success; the requester's own session still ends through
// the KindFarewell push every attached session receives, not through
// this response, so a non-empty Error here is the only way a reboot
// request can fail without also ending this session.
type RebootResponsePayload struct {
	Error string
}

// FarewellPayload is KindFarewell's own payload, the one, short,
// human readable line of text a CLI client prints before ending its
// own session because the daemon closed its connection on purpose,
// rather than the connection simply failing; see ClientFarewellText,
// FarewellDisconnected, and FarewellRebooting for the two texts this
// project actually sends.
type FarewellPayload struct {
	Text string
}

// FarewellDisconnected and FarewellRebooting are the two farewell
// texts claude/DAEMON_ARCHITECTURE_DESIGN.md names directly: "Line
// closed by remote host" for a targeted disconnect user, tracking
// real Cisco and HP wording closely, and "Device is rebooting" for a
// reboot, so a person watching their own terminal understands why the
// connection ended without needing to guess whether they were
// personally targeted. See also ConnectionLostText, in
// remoteclient.go, the third, distinct text a client reports on its
// own, for a connection that ended with no farewell push at all.
const (
	FarewellDisconnected = "Line closed by remote host"
	FarewellRebooting    = "Device is rebooting"
)

// EncodeMessage builds the plaintext this package hands to
// armorchan.Channel.Send: kind's own byte value followed by payload
// marshaled as JSON. payload should be one of this file's own
// payload types, or an empty struct{}{} for a kind that carries none,
// KindGoodbye, KindListUsersRequest, and KindRebootRequest among
// them; EncodeMessage itself does not enforce which payload type goes
// with which kind, leaving that pairing to this package's own callers,
// the same way encoding/json itself does not enforce a schema.
func EncodeMessage(kind MessageKind, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("daemon: encoding a %s message: %w", kind, err)
	}
	raw := make([]byte, 0, len(body)+1)
	raw = append(raw, byte(kind))
	raw = append(raw, body...)
	return raw, nil
}

// DecodeMessage splits raw, one plaintext frame already returned by
// armorchan.Channel.Receive, into its own MessageKind and the JSON
// body that followed it, without itself unmarshaling that body into
// any particular payload type; a caller already knows, from the kind
// returned here, which of this file's own payload types to decode the
// body into next, `var p HelloPayload; json.Unmarshal(body, &p)` for
// instance. DecodeMessage returns ErrMalformedMessage, wrapped, if raw
// is too short to even carry a one byte type tag; it does not itself
// validate that the body is well formed JSON, or that kind is one this
// package actually defines, leaving both to the caller's own
// json.Unmarshal call and switch, the same "decode what the tag says,
// let the caller's own per-kind logic reject what makes no sense"
// division FuzzDecodeMessage in protocol_test.go exercises directly.
func DecodeMessage(raw []byte) (MessageKind, []byte, error) {
	if len(raw) < 1 {
		return 0, nil, fmt.Errorf("daemon: decoding a message: %w: empty frame", ErrMalformedMessage)
	}
	return MessageKind(raw[0]), raw[1:], nil
}
