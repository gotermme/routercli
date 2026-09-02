// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"sync"
	"time"
)

// PendingReload tracks at most one scheduled, delayed reload or reboot
// at a time, backing cmd/core/cmd_admin.go's own "reload" and "reboot"
// handlers, which are full synonyms for the same underlying command
// and the same pending state, see that file's own doc comment.
// RouterCLI has no persistent daemon behind a connection, see
// AppContext.ReloadScheduler's own doc comment, so a delayed reload is
// implemented as an ordinary background timer inside this one process,
// not a request sent anywhere else.
//
// A zero PendingReload is not ready to use; construct one with
// NewPendingReload. Every method is safe to call from more than one
// goroutine at once, which matters here specifically: "reload
// <seconds>" and "no reload" both run from the interactive command
// dispatch goroutine, while a scheduled timer's own callback runs on a
// goroutine of its own, once its delay elapses, entirely independent
// of whatever the interactive session happens to be doing at that
// moment.
type PendingReload struct {
	mu     sync.Mutex
	timer  *time.Timer
	fireCh chan struct{}
}

// NewPendingReload returns a ready to use PendingReload, with nothing
// currently scheduled. main.go constructs exactly one of these, at
// startup, and stores it on AppContext.ReloadScheduler.
func NewPendingReload() *PendingReload {
	return &PendingReload{
		// Buffered by one, never more, so Schedule's own callback can
		// always deliver a fire notification without blocking on
		// whether anything is currently reading FireChannel(), and so
		// that a fire notification, once sent, can never accumulate
		// more than the one real reload it is reporting.
		fireCh: make(chan struct{}, 1),
	}
}

// FireChannel returns the channel that receives exactly one value once
// a scheduled reload actually fires, uncancelled. main.go's runLoop
// selects on this alongside reading the next typed line, so a
// scheduled reload can end the session even while the loop is
// otherwise blocked waiting on interactive input from a real terminal.
// The same channel is returned every time; it is never replaced, so a
// caller may safely select on it once and keep using that same value
// for the life of the process.
func (p *PendingReload) FireChannel() <-chan struct{} {
	return p.fireCh
}

// Schedule arms a new pending reload, delay from now, replacing
// whatever was previously scheduled, if anything; only the most
// recently typed "reload <seconds>" or "reboot <seconds>" is ever
// pending at once, matching real Cisco's own "reload" behavior, where
// typing it again with a new delay simply reschedules it.
//
// When delay elapses without an intervening Cancel or a later
// Schedule call superseding this one, exactly one value is sent,
// non-blocking, to FireChannel(), from a goroutine of Schedule's own,
// not the caller's. The identity check inside that goroutine, whether
// this specific timer is still the one PendingReload currently
// considers pending, closes a real race between time.Timer.Stop and a
// callback that has already started running by the time Stop is
// called: Go's own documentation is explicit that Stop returning false
// does not mean the callback has finished, or even started, only that
// it cannot be prevented from running, or already has. Comparing
// identity here, under the same mutex Cancel and a later Schedule both
// take, means a callback that loses that race is simply dropped,
// rather than firing a reload a session already believed it had
// cancelled.
func (p *PendingReload) Schedule(delay time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.timer != nil {
		p.timer.Stop()
	}

	var t *time.Timer
	t = time.AfterFunc(delay, func() {
		p.mu.Lock()
		if p.timer != t {
			// Superseded by Cancel or a later Schedule call before
			// this callback actually got to run; see this method's
			// own doc comment for the Stop/fire race this guards
			// against.
			p.mu.Unlock()
			return
		}
		p.timer = nil
		p.mu.Unlock()

		select {
		case p.fireCh <- struct{}{}:
		default:
			// FireChannel is buffered by exactly one and nothing has
			// drained a previous, already superseded notification
			// yet; see NewPendingReload's own doc comment for why
			// that can never represent more than the one real reload
			// actually pending.
		}
	})
	p.timer = t
}

// Cancel stops whatever reload is currently pending, if any, and
// reports whether there actually was one to stop. "no reload" and "no
// reboot" both call this; a false result means neither had anything
// scheduled to begin with, which cmd/core/cmd_admin.go reports back to
// the session as an error rather than a silent no-op, the same "fail
// loudly on a malformed request" convention this project already
// applies elsewhere.
func (p *PendingReload) Cancel() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.timer == nil {
		return false
	}
	p.timer.Stop()
	p.timer = nil
	return true
}

// Pending reports whether a reload is currently scheduled.
func (p *PendingReload) Pending() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.timer != nil
}
