// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package daemon

import (
	"errors"
	"sync"
)

// ErrStoreClosed is returned by Do once a Store has been closed, or
// is in the process of closing, rather than letting a caller submit a
// function against state that may no longer be owned by a live
// goroutine. A Store that has ever produced this error stays closed;
// Close is not reversible and a new Store must be constructed instead.
var ErrStoreClosed = errors.New("daemon: store is closed")

// storeRequest bundles one caller's own request, a function to run
// against the current state, together with the one channel that
// call's own result is delivered back on. Every field is set once, by
// Do, and read once, by run; nothing about storeRequest itself needs
// its own synchronization beyond that single handoff through Store's
// own commands channel. Named storeRequest rather than the more
// obvious command specifically to avoid colliding with this project's
// own package command, imported by state.go right alongside this
// file.
type storeRequest[S any] struct {
	fn     func(*S) (any, error)
	result chan<- doResult
}

// doResult is the value Do's own function call to fn ultimately
// returns, carried back from the single writer goroutine to whichever
// caller's own Do call is waiting for it.
type doResult struct {
	value any
	err   error
}

// Store is this package's own concurrency primitive: a single, long
// running goroutine owning one value of type S directly, in ordinary
// unshared Go memory, with no mutex protecting it at all, because
// nothing outside that one goroutine is ever allowed to touch it. See
// this package's own doc comment for the full reasoning; see Do for
// how a caller actually reads or mutates the state a Store owns.
//
// A zero Store is not ready to use; construct one with NewStore. Every
// method is safe to call from more than one goroutine at once.
type Store[S any] struct {
	commands  chan storeRequest[S]
	stop      chan struct{}
	stopped   chan struct{}
	closeOnce sync.Once
}

// NewStore returns a ready to use Store, its single writer goroutine
// already running, starting from initial as the current state. The
// caller passes ownership of initial to the Store; nothing outside a
// function passed to Do should read or write the value initial itself
// referred to ever again, the same way nothing outside a goroutine
// holding a mutex should touch what that mutex protects without
// holding it.
func NewStore[S any](initial S) *Store[S] {
	st := &Store[S]{
		commands: make(chan storeRequest[S]),
		stop:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	go st.run(initial)
	return st
}

// run is the one goroutine that ever touches state directly, for the
// entire life of a Store. It never returns until stop is closed, and
// every request it ever receives off commands is applied to state,
// and answered, strictly one at a time, in the order run happened to
// receive them off that one channel.
func (s *Store[S]) run(state S) {
	defer close(s.stopped)
	for {
		select {
		case cmd := <-s.commands:
			value, err := cmd.fn(&state)
			// result is always buffered by exactly one, see Do, and
			// used by exactly one command value, exactly once, so this
			// send can never block.
			cmd.result <- doResult{value: value, err: err}
		case <-s.stop:
			return
		}
	}
}

// Do submits fn to be run against the Store's own current state by
// its single writer goroutine, blocks until fn has actually run, and
// returns whatever fn itself returned. fn receives a pointer to the
// live state, and may both read and mutate it directly; no other call
// to Do, from any other goroutine, ever runs concurrently with fn,
// this Store's own whole reason to exist. fn should not retain the
// pointer it was given past its own return, and should not block for
// any real length of time, since every other caller waiting on this
// same Store, including a concurrent Do call already in flight
// elsewhere, waits for fn to return before its own turn comes.
//
// Do returns ErrStoreClosed, without ever running fn, if the Store
// was already closed, or being closed, when Do was called. A command
// already in flight, already submitted, when Close is called may
// still run to completion and genuinely mutate state, this Store's
// own documented "answered exactly once, one at a time" guarantee
// never breaks, but Do itself may still report ErrStoreClosed back to
// that specific caller rather than the real result, if Close happens
// to finish at almost the exact same moment; a caller that must know
// for certain whether its own mutation actually landed should not
// rely on Do's own return value racing Close this closely, the same
// caution any other shutdown race deserves.
func (s *Store[S]) Do(fn func(*S) (any, error)) (any, error) {
	resultCh := make(chan doResult, 1)
	select {
	case s.commands <- storeRequest[S]{fn: fn, result: resultCh}:
	case <-s.stop:
		return nil, ErrStoreClosed
	}

	select {
	case r := <-resultCh:
		return r.value, r.err
	case <-s.stop:
		return nil, ErrStoreClosed
	}
}

// Close stops this Store's own single writer goroutine and waits for
// it to actually exit before returning. Close is safe to call more
// than once, from more than one goroutine at once; only the first
// call does anything, every call, including the first, blocks until
// the goroutine has genuinely stopped. Every Do call still in flight
// when Close is called either completes normally or observes
// ErrStoreClosed, per Do's own doc comment; no command is ever left
// half applied.
func (s *Store[S]) Close() {
	s.closeOnce.Do(func() { close(s.stop) })
	<-s.stopped
}
