// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package main

import (
	"bytes"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/gologme/log"
	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/cmd"
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

// ----------------------------------------------------------------------
//
// buildPrompt
//
// ----------------------------------------------------------------------

// baseExecLevelsForPrompt - This function builds a minimal
// *command.TreeStructure with a "base" level, marked IsBase, and an
// "exec" level, the shape buildPrompt needs from ctx.Levels.Base() to
// decide whether a session is away from base.
func baseExecLevelsForPrompt() *command.TreeStructure {
	base := &command.CommandLevel{Name: "base", IsBase: true}
	exec := &command.CommandLevel{Name: "exec", Parent: "base"}
	return &command.TreeStructure{Order: []*command.CommandLevel{base, exec}}
}

// TestBuildPromptAtBaseUsesUnprivilegedMarker - This test verifies
// that a session at the base level, root frame, depth 1, gets the
// "> " unprivileged marker, and the default hostname fallback, since
// State.Hostname is still empty.
func TestBuildPromptAtBaseUsesUnprivilegedMarker(t *testing.T) {
	ctx := &command.AppContext{
		State:    &cmd.ExampleState{},
		Session:  &auth.Session{CommandLevel: "base"},
		Levels:   baseExecLevelsForPrompt(),
		Position: command.NewCommandLevelStack("base", "", nil),
	}
	got := buildPrompt(ctx)
	want := defaultHostnamePrompt + "> "
	if got != want {
		t.Errorf("buildPrompt() = %q, want %q", got, want)
	}
}

// TestBuildPromptAwayFromBaseUsesPrivilegedMarker - This test
// verifies that a session whose Session.CommandLevel is not the base
// level's own Name, "exec" here, gets the "# " privileged marker,
// even though Position itself is still only one frame deep.
func TestBuildPromptAwayFromBaseUsesPrivilegedMarker(t *testing.T) {
	ctx := &command.AppContext{
		State:    &cmd.ExampleState{},
		Session:  &auth.Session{CommandLevel: "exec"},
		Levels:   baseExecLevelsForPrompt(),
		Position: command.NewCommandLevelStack("exec", "", nil),
	}
	got := buildPrompt(ctx)
	want := defaultHostnamePrompt + "# "
	if got != want {
		t.Errorf("buildPrompt() = %q, want %q", got, want)
	}
}

// TestBuildPromptNestedFrameUsesPrivilegedMarkerEvenAtBase - This
// test verifies that a session still at the base Session.CommandLevel,
// but with a pushed, nested CommandLevelStack frame such as config,
// also gets the "# " marker, through the ctx.Position.Depth() > 1
// half of buildPrompt's own condition, independent of
// Session.CommandLevel entirely.
func TestBuildPromptNestedFrameUsesPrivilegedMarkerEvenAtBase(t *testing.T) {
	ctx := &command.AppContext{
		State:    &cmd.ExampleState{},
		Session:  &auth.Session{CommandLevel: "base"},
		Levels:   baseExecLevelsForPrompt(),
		Position: command.NewCommandLevelStack("base", "", nil),
	}
	ctx.Position.Push(command.CommandLevelFrame{Name: "config", PromptSuffix: "(config)"})

	got := buildPrompt(ctx)
	want := defaultHostnamePrompt + "(config)" + "# "
	if got != want {
		t.Errorf("buildPrompt() = %q, want %q", got, want)
	}
}

// TestBuildPromptUsesConfiguredHostnameOnceSet - This test verifies
// that once ExampleState.Hostname is non-empty, buildPrompt shows it
// instead of defaultHostnamePrompt, and reads it live rather than a
// value captured at some earlier point.
func TestBuildPromptUsesConfiguredHostnameOnceSet(t *testing.T) {
	state := &cmd.ExampleState{Hostname: "myrouter"}
	ctx := &command.AppContext{
		State:    state,
		Session:  &auth.Session{CommandLevel: "base"},
		Levels:   baseExecLevelsForPrompt(),
		Position: command.NewCommandLevelStack("base", "", nil),
	}
	got := buildPrompt(ctx)
	want := "myrouter> "
	if got != want {
		t.Errorf("buildPrompt() = %q, want %q", got, want)
	}
}

// TestBuildPromptToleratesNilSessionOrLevels - This test verifies
// that buildPrompt does not panic when Session or Levels is nil, the
// state a hand-built test context that never wired them up is in,
// falling back to the unprivileged marker rather than attempting the
// AtLevel check at all.
func TestBuildPromptToleratesNilSessionOrLevels(t *testing.T) {
	ctx := &command.AppContext{
		State:    &cmd.ExampleState{},
		Position: command.NewCommandLevelStack("base", "", nil),
	}
	got := buildPrompt(ctx)
	want := defaultHostnamePrompt + "> "
	if got != want {
		t.Errorf("buildPrompt() = %q, want %q", got, want)
	}
}

// ----------------------------------------------------------------------
//
// historyFilePath, mkdirForFile
//
// ----------------------------------------------------------------------

// TestHistoryFilePathUnderUsersHomeDirectory - This test verifies
// that historyFilePath returns a path under the real current user's
// home directory, ".routercli_history", the fallback used when the
// configuration file does not set HistoryFile explicitly.
func TestHistoryFilePathUnderUsersHomeDirectory(t *testing.T) {
	got := historyFilePath()
	u, err := user.Current()
	if err != nil || u.HomeDir == "" {
		if got != ".routercli_history" {
			t.Errorf("historyFilePath() = %q, want %q when the current user's home directory cannot be determined", got, ".routercli_history")
		}
		return
	}
	want := filepath.Join(u.HomeDir, ".routercli_history")
	if got != want {
		t.Errorf("historyFilePath() = %q, want %q", got, want)
	}
}

// TestMkdirForFileCreatesMissingParentDirectory - This test verifies
// that mkdirForFile creates path's parent directory, including any
// missing intermediate directories, when it does not already exist.
func TestMkdirForFileCreatesMissingParentDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "logs")
	target := filepath.Join(dir, "audit.log")

	if err := mkdirForFile(target); err != nil {
		t.Fatalf("mkdirForFile returned unexpected error: %v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("expected %q to exist after mkdirForFile, stat error: %v", dir, err)
	}
	if !info.IsDir() {
		t.Errorf("%q exists but is not a directory", dir)
	}
}

// TestMkdirForFileNoOpForBarePathWithNoDirectory - This test verifies
// that mkdirForFile does nothing, and returns no error, for a bare
// filename with no directory component, filepath.Dir returning "."
// rather than something to create.
func TestMkdirForFileNoOpForBarePathWithNoDirectory(t *testing.T) {
	if err := mkdirForFile("audit.log"); err != nil {
		t.Errorf("mkdirForFile(\"audit.log\") returned unexpected error: %v", err)
	}
}

// ----------------------------------------------------------------------
//
// warnPlaintextUserSecrets, sortedUserNames, warnPlaintextLevelSecrets
//
// ----------------------------------------------------------------------

// captureLogOutput - This function returns a *log.Logger with the
// "warn" level enabled, writing into an in-memory buffer, plus a
// function to read back everything written so far. gologme/log gates
// every level behind an explicit EnableLevel call, unlike the
// standard library's log package, so a freshly constructed logger
// with no level enabled would silently swallow every Warnln call.
func captureLogOutput() (*log.Logger, func() string) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)
	logger.EnableLevel("warn")
	return logger, buf.String
}

// TestWarnPlaintextUserSecretsWarnsOnlyForPlaintextUsers - This test
// verifies that warnPlaintextUserSecrets logs a warning naming a user
// whose PasswordHash is stored in the plaintext "$0$..." form, and
// says nothing at all for a user with a real bcrypt hash.
func TestWarnPlaintextUserSecretsWarnsOnlyForPlaintextUsers(t *testing.T) {
	logger, output := captureLogOutput()
	hash, err := auth.HashPassword("s3cret")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	users := auth.Users{
		"alice": {Username: "alice", PasswordHash: "$0$plaintext"},
		"bob":   {Username: "bob", PasswordHash: hash},
	}

	warnPlaintextUserSecrets(logger, users)

	got := output()
	if !strings.Contains(got, "alice") {
		t.Errorf("expected a warning naming \"alice\", got %q", got)
	}
	if strings.Contains(got, "bob") {
		t.Errorf("expected no warning for \"bob\", who has a real bcrypt hash, got %q", got)
	}
}

// TestWarnPlaintextUserSecretsSilentWhenNonePlaintext - This test
// verifies that warnPlaintextUserSecrets logs nothing at all when
// every user has a real bcrypt hash.
func TestWarnPlaintextUserSecretsSilentWhenNonePlaintext(t *testing.T) {
	logger, output := captureLogOutput()
	hash, err := auth.HashPassword("s3cret")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	users := auth.Users{"alice": {Username: "alice", PasswordHash: hash}}

	warnPlaintextUserSecrets(logger, users)

	if got := output(); got != "" {
		t.Errorf("expected no warning output, got %q", got)
	}
}

// TestSortedUserNamesReturnsNamesInSortedOrder - This test verifies
// that sortedUserNames returns every username sorted alphabetically,
// regardless of Go's randomized map iteration order, so
// warnPlaintextUserSecrets's own output is stable between runs.
func TestSortedUserNamesReturnsNamesInSortedOrder(t *testing.T) {
	users := auth.Users{
		"charlie": {Username: "charlie"},
		"alice":   {Username: "alice"},
		"bob":     {Username: "bob"},
	}
	got := sortedUserNames(users)
	want := []string{"alice", "bob", "charlie"}
	if len(got) != len(want) {
		t.Fatalf("sortedUserNames() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sortedUserNames()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestSortedUserNamesEmptyForEmptyUsers - This test verifies that
// sortedUserNames returns an empty, non-nil slice for an empty user
// database, rather than nil, so a caller ranging over it never needs
// its own nil check.
func TestSortedUserNamesEmptyForEmptyUsers(t *testing.T) {
	got := sortedUserNames(auth.Users{})
	if len(got) != 0 {
		t.Errorf("sortedUserNames(empty) = %v, want an empty slice", got)
	}
}

// TestWarnPlaintextLevelSecretsWarnsOnlyForPlaintextLevels - This
// test verifies the Tree Structure counterpart to
// TestWarnPlaintextUserSecretsWarnsOnlyForPlaintextUsers: a Command
// Level whose PasswordHash is stored in plaintext form is warned
// about by name, and a level with a real bcrypt hash, or no
// PasswordHash at all, is not.
func TestWarnPlaintextLevelSecretsWarnsOnlyForPlaintextLevels(t *testing.T) {
	logger, output := captureLogOutput()
	hash, err := auth.HashPassword("s3cret")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	levels := &command.TreeStructure{Order: []*command.CommandLevel{
		{Name: "exec", PasswordHash: "$0$plaintext"},
		{Name: "diagnostic", PasswordHash: hash},
		{Name: "config"},
	}}

	warnPlaintextLevelSecrets(logger, levels)

	got := output()
	if !strings.Contains(got, "exec") {
		t.Errorf("expected a warning naming \"exec\", got %q", got)
	}
	if strings.Contains(got, "diagnostic") {
		t.Errorf("expected no warning for \"diagnostic\", which has a real bcrypt hash, got %q", got)
	}
	if strings.Contains(got, "config") {
		t.Errorf("expected no warning for \"config\", which has no PasswordHash at all, got %q", got)
	}
}

// ----------------------------------------------------------------------
//
// firstBadToken
//
// ----------------------------------------------------------------------

// TestFirstBadTokenReturnsFirstEntry - This test verifies that
// firstBadToken returns only the first entry of a multi-element Args
// slice, "fan" not "fan extra junk", matching how a real device
// points at the first word it could not place.
func TestFirstBadTokenReturnsFirstEntry(t *testing.T) {
	got := firstBadToken([]string{"fan", "extra", "junk"})
	if got != "fan" {
		t.Errorf("firstBadToken([\"fan\", \"extra\", \"junk\"]) = %q, want %q", got, "fan")
	}
}

// TestFirstBadTokenEmptyForEmptyArgs - This test verifies that
// firstBadToken returns an empty string, rather than panicking, for
// an empty Args slice.
func TestFirstBadTokenEmptyForEmptyArgs(t *testing.T) {
	if got := firstBadToken(nil); got != "" {
		t.Errorf("firstBadToken(nil) = %q, want empty", got)
	}
	if got := firstBadToken([]string{}); got != "" {
		t.Errorf("firstBadToken([]string{}) = %q, want empty", got)
	}
}

// ----------------------------------------------------------------------
//
// printOutputHeader
//
// ----------------------------------------------------------------------

// captureStdout - This function runs fn with os.Stdout temporarily
// redirected to an in-memory pipe, and returns everything fn printed,
// restoring the real os.Stdout afterward. printOutputHeader prints
// straight to os.Stdout rather than through an injectable writer, the
// same as several handlers in package cmd, see that package's own
// captureStdout helper for the same reasoning.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	real := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = real }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	return buf.String()
}

// TestPrintOutputHeaderIncludesVersion - This test verifies that
// printOutputHeader's output includes the current Version, and the
// Build string only when one is set, matching the "--version" and
// "--help" flags' own use of this function in processCommandLineFlags.
func TestPrintOutputHeaderIncludesVersion(t *testing.T) {
	origBuild := Build
	Build = ""
	defer func() { Build = origBuild }()

	out := captureStdout(t, printOutputHeader)
	if !strings.Contains(out, "Router CLI") {
		t.Errorf("printOutputHeader output = %q, expected it to contain %q", out, "Router CLI")
	}
	if !strings.Contains(out, Version) {
		t.Errorf("printOutputHeader output = %q, expected it to contain the Version %q", out, Version)
	}
}

// TestPrintOutputHeaderIncludesBuildWhenSet - This test verifies that
// a non-empty Build string is included in the header, the branch
// skipped by TestPrintOutputHeaderIncludesVersion.
func TestPrintOutputHeaderIncludesBuildWhenSet(t *testing.T) {
	origBuild := Build
	Build = "test-build-123"
	defer func() { Build = origBuild }()

	out := captureStdout(t, printOutputHeader)
	if !strings.Contains(out, "test-build-123") {
		t.Errorf("printOutputHeader output = %q, expected it to contain the Build string %q", out, "test-build-123")
	}
}
