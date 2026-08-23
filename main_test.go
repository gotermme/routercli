// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package main

import (
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/gotermme/routercli/command"
)

// TestPreventEscapeIgnoresEscapeSignals - This test sends the process
// real SIGINT, SIGTSTP, SIGQUIT, and SIGTERM signals through
// syscall.Kill(self, ...) after calling preventEscape, and simply
// checks that the test is still running afterward. If any of these
// signals were not actually ignored at the OS level, this test
// process would be killed or suspended, and the test runner itself
// would hang or report a failure, not a clean assertion failure. That
// is deliberate. This is testing an OS level guarantee, SIG_IGN, not
// application logic, so the test suite still running is the only
// meaningful signal that it worked.
//
// This mutates process-wide signal disposition, which is safe here
// since each "go test" invocation is its own throwaway process. It
// does not affect anything outside this test run.
func TestPreventEscapeIgnoresEscapeSignals(t *testing.T) {
	preventEscape()

	signals := []syscall.Signal{
		syscall.SIGINT,
		syscall.SIGTSTP,
		syscall.SIGQUIT,
		syscall.SIGTERM,
	}

	pid := os.Getpid()
	for _, sig := range signals {
		if err := syscall.Kill(pid, sig); err != nil {
			t.Fatalf("failed to send %v to self: %v", sig, err)
		}
	}

	// Give the OS a moment to actually deliver the signals before
	// declaring victory, since delivery is asynchronous.
	time.Sleep(100 * time.Millisecond)

	// Reaching this line at all is the assertion. If any of the four
	// signals above were not truly ignored, this goroutine, and the
	// whole test process, would already be dead or suspended.
	t.Log("process survived SIGINT/SIGTSTP/SIGQUIT/SIGTERM after preventEscape()")
}

// ----------------------------------------------------------------------
//
// attachPasswordRateLimiters
//
// ----------------------------------------------------------------------

// TestAttachPasswordRateLimitersOnlySetsLimiterForPasswordProtectedCommands -
// This test verifies that attachPasswordRateLimiters gives a working
// auth.RateLimiter to every command with a PasswordHash set, and
// leaves an unprotected command's PasswordRateLimiter nil. Allow and
// RecordFailure are exercised directly, not just a non-nil check,
// since a limiter that exists but was constructed with the wrong
// maxAttempts would still pass a non-nil check while doing nothing
// useful.
func TestAttachPasswordRateLimitersOnlySetsLimiterForPasswordProtectedCommands(t *testing.T) {
	protected := &command.Command{PasswordHash: "$0$secret"}
	unprotected := &command.Command{}
	levels := &command.TreeStructure{
		Order: []*command.CommandLevel{
			{Name: "exec", Tree: map[string]*command.Command{"secret-thing": protected, "plain-thing": unprotected}},
		},
	}

	attachPasswordRateLimiters(levels, 1, time.Minute, 5*time.Minute)

	if unprotected.PasswordRateLimiter != nil {
		t.Error("expected an unprotected command's PasswordRateLimiter to stay nil")
	}
	if protected.PasswordRateLimiter == nil {
		t.Fatal("expected a password-protected command to get a PasswordRateLimiter")
	}

	if ok, _ := protected.PasswordRateLimiter.Allow(); !ok {
		t.Fatal("expected a freshly attached RateLimiter to allow immediately")
	}
	protected.PasswordRateLimiter.RecordFailure() // maxAttempts is 1, so this locks it out
	if ok, _ := protected.PasswordRateLimiter.Allow(); ok {
		t.Error("expected the attached RateLimiter to actually enforce maxAttempts=1, not just exist")
	}
}

// TestAttachPasswordRateLimitersMaxAttemptsZeroLeavesRateLimitingDisabled -
// This test verifies that a configured maxAttempts of zero still gets
// every password-protected command a real RateLimiter, attachment is
// unconditional, see this function's own doc comment, but that
// RateLimiter never actually locks anything out, matching
// auth.RateLimiter's own "maxAttempts at or below zero disables"
// convention. This confirms that convention holds all the way through
// this wiring path, not only when auth.RateLimiter is used directly.
func TestAttachPasswordRateLimitersMaxAttemptsZeroLeavesRateLimitingDisabled(t *testing.T) {
	protected := &command.Command{PasswordHash: "$0$secret"}
	levels := &command.TreeStructure{
		Order: []*command.CommandLevel{
			{Name: "exec", Tree: map[string]*command.Command{"secret-thing": protected}},
		},
	}

	attachPasswordRateLimiters(levels, 0, time.Minute, 5*time.Minute)

	if protected.PasswordRateLimiter == nil {
		t.Fatal("expected a RateLimiter to be attached even when maxAttempts is 0")
	}
	for i := 0; i < 10; i++ {
		protected.PasswordRateLimiter.RecordFailure()
	}
	if ok, _ := protected.PasswordRateLimiter.Allow(); !ok {
		t.Error("expected Allow to stay true regardless of recorded failures when maxAttempts is 0")
	}
}

// TestAttachPasswordRateLimitersDoesNotRecurseForeverOnASharedCommand -
// This test verifies the actual mechanism visited exists for: a
// command reached more than once through the walk is never processed
// a second time. Ordinary YAML-loaded trees are acyclic, so this
// cannot happen from a real tree file, but a hand-built Go tree, or a
// future change to how InheritParent shares commands across levels,
// could produce a command that is reachable from itself. Without
// visited, walk would recurse into that command's own Subcommands
// forever. This is run on a background goroutine with a short timeout
// rather than allowed to actually hang the test suite if the
// regression this guards against were ever reintroduced.
func TestAttachPasswordRateLimitersDoesNotRecurseForeverOnASharedCommand(t *testing.T) {
	shared := &command.Command{PasswordHash: "$0$secret"}
	shared.Subcommands = map[string]*command.Command{"self": shared}
	levels := &command.TreeStructure{
		Order: []*command.CommandLevel{
			{Name: "exec", Tree: map[string]*command.Command{"root": shared}},
		},
	}

	done := make(chan struct{})
	go func() {
		attachPasswordRateLimiters(levels, 1, time.Minute, 5*time.Minute)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("attachPasswordRateLimiters did not return, a self-referencing command likely caused infinite recursion")
	}

	if shared.PasswordRateLimiter == nil {
		t.Error("expected the shared, self-referencing command to still get a PasswordRateLimiter attached")
	}
}
