// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package daemon

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"time"
)

// ErrNoMatchingSession is returned by SessionDirectory.Farewell when
// username, or username together with sessionID, matches no currently
// attached session.
var ErrNoMatchingSession = errors.New("daemon: no matching session")

// ErrAmbiguousSession is returned by SessionDirectory.Farewell when
// sessionID is empty and more than one currently attached session
// belongs to username, matching claude/DAEMON_ARCHITECTURE_DESIGN.md's
// own disambiguation rule: "the daemon answers with an error listing
// the matching session IDs rather than guessing which one was meant."
// The matching session IDs themselves are not carried on this error
// value; a caller builds its own message from the SessionInfo values
// ListUsers or Farewell's own candidates return alongside it.
var ErrAmbiguousSession = errors.New("daemon: more than one session for this user, specify a session ID")

// ErrSessionNotFound is returned by SessionDirectory.Touch and
// SessionDirectory.Unregister when id names no currently registered
// session, most commonly a session that has already ended, a second
// Unregister call for the same connection for instance.
var ErrSessionNotFound = errors.New("daemon: session not found")

// SessionInfo is everything ListUsers reports about one currently
// attached session, and everything this package sends across the wire
// in a ListUsersResponsePayload: a daemon assigned session ID, the
// username that sent Hello, the Command Level that session most
// recently reported itself at, when the connection was accepted, and
// when this session was last seen doing anything, an AuditEvent most
// recently, matching claude/DAEMON_ARCHITECTURE_DESIGN.md's own
// ListUsers description: "a daemon assigned session ID, username,
// current Command Level, connected since timestamp, and idle time."
// Idle time itself is not stored here; it is however long ago
// LastActivity was, computed by whoever builds a ListUsersResponsePayload
// against time.Now() at response time rather than carried as its own,
// separately staling field.
//
// SessionInfo is deliberately the only session shaped type this
// package sends across the wire; trackedSession below adds exactly
// one more field, a private channel used to push a farewell to this
// one session, that has no business ever being serialized or handed
// to a caller outside this package.
type SessionInfo struct {
	ID           string
	Username     string
	CommandLevel string
	ConnectedAt  time.Time
	LastActivity time.Time
}

// trackedSession is this package's own, private bookkeeping for one
// attached session: everything SessionInfo already carries, plus
// farewell, the one channel SessionDirectory.Farewell and
// SessionDirectory.BroadcastFarewell use to push a KindFarewell
// message at whatever goroutine is actually serving this session's
// own connection, see routercli-daemon's own per connection serve
// loop, a later phase's own work. farewell is buffered by exactly
// one, see Register, so a push here can never block this
// SessionDirectory's own single writer goroutine, the same
// non-blocking discipline PendingReload.FireChannel's own doc comment
// already documents for a different, single slot channel.
type trackedSession struct {
	SessionInfo
	farewell chan<- string
}

// SessionRegistry is the raw, in memory state one SessionDirectory
// owns: every currently attached session, keyed by its own daemon
// assigned session ID. A zero SessionRegistry, an unallocated map, is
// not valid state to construct a Store around directly; see
// NewSessionDirectory, which allocates one.
type SessionRegistry struct {
	sessions map[string]*trackedSession
}

// SessionDirectory is a real daemon's own session and connection
// registry, reusing this package's own generic Store[S any] directly
// as its concurrency safety, the identical pattern StandaloneClient
// already established for canonical state: one long running goroutine
// owning a SessionRegistry in ordinary unshared Go memory, every read
// and every mutation arriving as a function submitted through Do,
// applied strictly one at a time. A zero SessionDirectory is not ready
// to use; construct one with NewSessionDirectory.
type SessionDirectory struct {
	store *Store[SessionRegistry]
}

// NewSessionDirectory returns a ready to use SessionDirectory, its own
// private Store already running, starting from an empty registry, no
// sessions attached.
func NewSessionDirectory() *SessionDirectory {
	return &SessionDirectory{
		store: NewStore(SessionRegistry{sessions: make(map[string]*trackedSession)}),
	}
}

// newSessionID returns a fresh, random session ID: eight bytes read
// from crypto/rand, hex encoded, sixteen characters. This is the
// "exact daemon side session ID format" claude/DAEMON_ARCHITECTURE_DESIGN.md
// leaves as ordinary implementation detail, worked out here rather
// than left open: short enough to read and type back on a
// "disconnect user bob <session-id>" command line, long enough that
// two sessions attaching around the same moment never collide by
// chance.
func newSessionID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("daemon: generating a session ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// Register adds a new session to this directory, starting at
// commandLevel, ConnectedAt and LastActivity both set to now, and
// returns the fresh, random session ID it was assigned. farewell is
// the channel whatever goroutine is serving this one connection reads
// from to learn it has been targeted by DisconnectUser or a
// BroadcastFarewell; it is wrapped here in a channel buffered by
// exactly one, allocated internally rather than accepted as a
// caller-supplied buffered channel, so this directory's own Farewell
// and BroadcastFarewell can always push to it without blocking
// regardless of what the caller itself passed in. The caller receives
// that same channel back, unbuffered from the caller's own point of
// view, to read a farewell off of; see the returned channel's own
// use in routercli-daemon's per connection serve loop, a later
// phase's own work.
func (d *SessionDirectory) Register(username, commandLevel string) (id string, farewell <-chan string, err error) {
	sessionID, err := newSessionID()
	if err != nil {
		return "", nil, err
	}

	ch := make(chan string, 1)
	now := time.Now()
	_, doErr := d.store.Do(func(r *SessionRegistry) (any, error) {
		r.sessions[sessionID] = &trackedSession{
			SessionInfo: SessionInfo{
				ID:           sessionID,
				Username:     username,
				CommandLevel: commandLevel,
				ConnectedAt:  now,
				LastActivity: now,
			},
			farewell: ch,
		}
		return nil, nil
	})
	if doErr != nil {
		return "", nil, doErr
	}
	return sessionID, ch, nil
}

// Touch updates a session's own CommandLevel and LastActivity to now,
// called whenever that session sends an AuditEvent, its own dispatched
// command naming whichever Command Level it ran at; this is
// deliberately the only source this package uses for "idle time"
// rather than a separate heartbeat message, since a real daemon
// already hears from an active session at least once per dispatched
// command regardless. Touch returns ErrSessionNotFound if id names no
// currently registered session.
func (d *SessionDirectory) Touch(id, commandLevel string) error {
	_, err := d.store.Do(func(r *SessionRegistry) (any, error) {
		s, ok := r.sessions[id]
		if !ok {
			return nil, ErrSessionNotFound
		}
		s.CommandLevel = commandLevel
		s.LastActivity = time.Now()
		return nil, nil
	})
	return err
}

// Unregister removes a session from this directory outright, called
// once its own connection has actually closed, whether that session
// sent a KindGoodbye first or simply disappeared, kill -9 on the CLI
// process for instance; either way this directory's own bookkeeping
// must not keep reporting a session that can no longer be reached.
// Unregister returns ErrSessionNotFound if id names no currently
// registered session, most commonly a second call for the same
// connection; a caller that cannot tell whether it already called
// this once for a given id should simply ignore that specific error
// rather than treating it as a real failure.
func (d *SessionDirectory) Unregister(id string) error {
	_, err := d.store.Do(func(r *SessionRegistry) (any, error) {
		if _, ok := r.sessions[id]; !ok {
			return nil, ErrSessionNotFound
		}
		delete(r.sessions, id)
		return nil, nil
	})
	return err
}

// List returns one SessionInfo per currently attached session, sorted
// by ConnectedAt, oldest first, so "show users" reports a stable,
// predictable order from one call to the next rather than a plain Go
// map's own unordered iteration. This is everything
// ListUsersResponsePayload carries back to whichever CLI session
// asked.
func (d *SessionDirectory) List() ([]SessionInfo, error) {
	value, err := d.store.Do(func(r *SessionRegistry) (any, error) {
		infos := make([]SessionInfo, 0, len(r.sessions))
		for _, s := range r.sessions {
			infos = append(infos, s.SessionInfo)
		}
		return infos, nil
	})
	if err != nil {
		return nil, err
	}
	infos := value.([]SessionInfo)
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].ConnectedAt.Before(infos[j].ConnectedAt)
	})
	return infos, nil
}

// Farewell resolves username, and optionally sessionID, against every
// currently attached session, matching
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own DisconnectUser
// disambiguation rule exactly: sessionID non-empty names one session
// precisely, refusing with ErrNoMatchingSession if it does not belong
// to username; sessionID empty and exactly one session belongs to
// username resolves to that one session; sessionID empty and more
// than one session belongs to username refuses with
// ErrAmbiguousSession, candidates still returned so the caller can
// list them. On a single, unambiguous match, Farewell pushes text to
// that one session's own farewell channel, allocated by Register, and
// returns that session's own ID alone; a real daemon's own per
// connection serve loop, seeing this push, sends KindFarewell down
// the wire and closes the connection, see trackedSession's own doc
// comment. Farewell does not itself remove the session from this
// directory; that happens through Unregister once the connection
// this push caused to close is actually observed closing, the same
// way an ordinary session ending is handled.
func (d *SessionDirectory) Farewell(username, sessionID, text string) (matchedID string, err error) {
	value, doErr := d.store.Do(func(r *SessionRegistry) (any, error) {
		var candidates []*trackedSession
		for _, s := range r.sessions {
			if s.Username != username {
				continue
			}
			if sessionID != "" && s.ID != sessionID {
				continue
			}
			candidates = append(candidates, s)
		}

		if sessionID != "" {
			if len(candidates) == 0 {
				return "", ErrNoMatchingSession
			}
			candidates[0].farewell <- text
			return candidates[0].ID, nil
		}

		switch len(candidates) {
		case 0:
			return "", ErrNoMatchingSession
		case 1:
			candidates[0].farewell <- text
			return candidates[0].ID, nil
		default:
			return "", ErrAmbiguousSession
		}
	})
	if doErr != nil {
		return "", doErr
	}
	return value.(string), nil
}

// BroadcastFarewell pushes text to every currently attached session's
// own farewell channel, including a session that itself triggered
// whatever caused this, a reboot for instance, naturally producing
// "the requester also gets disconnected" with no special casing at
// all. This is the one mechanism behind both reboot, text
// FarewellRebooting, and a clean SIGTERM drain, text left empty,
// "closed, no special explanation" per
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own settled default for a
// daemon shutting down cleanly, reusing this exact mechanism rather
// than inventing a second one for that case.
func (d *SessionDirectory) BroadcastFarewell(text string) error {
	_, err := d.store.Do(func(r *SessionRegistry) (any, error) {
		for _, s := range r.sessions {
			s.farewell <- text
		}
		return nil, nil
	})
	return err
}

// Close stops this SessionDirectory's own single writer goroutine. A
// caller should call this exactly once, when a real daemon process
// itself is shutting down, after BroadcastFarewell and after every
// attached connection has actually been closed.
func (d *SessionDirectory) Close() {
	d.store.Close()
}
