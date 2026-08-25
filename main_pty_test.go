// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"github.com/creack/pty"

	"github.com/chzyer/readline"
	"github.com/gologme/log"
	"github.com/gotermme/routercli/auditlog"
	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/cmd"
	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/completer"
	"github.com/gotermme/routercli/config"
)

// ----------------------------------------------------------------------
//
// pty test helpers
//
// ----------------------------------------------------------------------

// newPTY - This function opens a real pseudo terminal pair,
// github.com/creack/pty under the hood, and registers a t.Cleanup to
// close both ends once the test finishes. Everything in this file
// that reads a masked password or TOTP code needs one of these,
// rather than a plain os.Pipe or bytes.Buffer, since golang.org/x/term
// talks to the kernel through ioctl calls that only a genuine
// character device answers, a pipe is not one. master is written to
// by a test playing the part of a person typing, slave is handed to
// whichever function under test expects a real terminal file, such as
// establishSession's own stdin parameter. This is the same technique
// this project's own PROGRESS.md previously documented as only
// reachable by hand through a Python pty script, now driven directly
// from go test instead, see this project's PROGRESS.md for the
// history of why that manual step existed in the first place.
func newPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	m, s, err := pty.Open()
	if err != nil {
		t.Fatalf("failed to open a pseudo terminal: %v", err)
	}
	t.Cleanup(func() {
		m.Close()
		s.Close()
	})
	return m, s
}

// sendLine - This function writes s, followed by a newline, to w, the
// master side of a pty from newPTY, the same as a person typing s and
// pressing Enter. Errors are reported through t rather than returned,
// since every call site in this file already runs inside a *testing.T
// and has nothing useful to do with a write failure other than fail
// the test.
func sendLine(t *testing.T, w io.Writer, s string) {
	t.Helper()
	if _, err := fmt.Fprintln(w, s); err != nil {
		t.Fatalf("failed to write %q to pty master: %v", s, err)
	}
}

// discardAuditLogger - This function returns a *log.Logger that
// throws away everything written to it, for constructing a real
// *auditlog.AuditLog in a test that cares about the audit trail
// itself, not anything the AuditLog's own internal logger would
// report. Mirrors cmd/cmd_audit_test.go's own newTestAuditor helper,
// which cannot be reused directly here since it lives in a different
// package.
func discardAuditLogger() *log.Logger {
	return log.New(io.Discard, "", 0)
}

// newTestAudit - This function constructs a real, enabled
// *auditlog.AuditLog writing to a throwaway file under t.TempDir().
// establishSession takes a concrete *auditlog.AuditLog rather than
// the command.Auditor interface, see its own doc comment for why, so
// a test double implementing that narrower interface is not an
// option here the way it is for logSessionEnd's own tests below.
// Enable is called immediately, since AuditLog.Log and ForceLog both
// no-op silently until the underlying file is actually open, see
// auditlog.AuditLog's own doc comments, and these tests need to read
// real entries back to assert on them.
func newTestAudit(t *testing.T) *auditlog.AuditLog {
	t.Helper()
	path := filepath.Join(t.TempDir(), "audit.log")
	a := auditlog.New(path, discardAuditLogger())
	if err := a.Enable(); err != nil {
		t.Fatalf("failed to enable test audit log: %v", err)
	}
	t.Cleanup(func() { a.Close() })
	return a
}

// readAuditLog - This function returns everything written so far to
// a's own underlying file, by reading it back from disk, since
// *auditlog.AuditLog exposes no in-memory accessor of its own.
func readAuditLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read audit log %q: %v", path, err)
	}
	return string(data)
}

// mustHashPassword - This function hashes password with
// auth.HashPassword, failing the test immediately on error, so every
// call site in this file that just needs a working bcrypt hash for a
// fixture user does not have to repeat the same three line check.
func mustHashPassword(t *testing.T, password string) string {
	t.Helper()
	hash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword(%q) returned error: %v", password, err)
	}
	return hash
}

// establishSessionResult - This type bundles establishSession's own
// two return values so a test can send them over a channel from the
// goroutine that calls it, see runEstablishSession below.
type establishSessionResult struct {
	session *auth.Session
	err     error
}

// runEstablishSession - This function calls establishSession on its
// own goroutine and returns a channel carrying its result. A pty
// backed call must run this way, not inline, since establishSession
// blocks reading from slave until a full line, or a masked password,
// arrives, and the test itself is what writes that input to master
// right after starting this goroutine. Calling establishSession
// directly on the test's own goroutine before writing anything to
// master would deadlock, each side waiting on the other.
func runEstablishSession(cfg config.SystemConfig, users auth.Users, provider auth.AuthProvider, audit *auditlog.AuditLog, stdin, stdout *os.File) <-chan establishSessionResult {
	ch := make(chan establishSessionResult, 1)
	go func() {
		session, err := establishSession(cfg, users, provider, nil, audit, stdin, stdout)
		ch <- establishSessionResult{session, err}
	}()
	return ch
}

// awaitEstablishSession - This function waits for res, failing the
// test rather than hanging forever if establishSession never returns
// within timeout, the sign of a real deadlock somewhere above rather
// than a slow but working test.
func awaitEstablishSession(t *testing.T, res <-chan establishSessionResult, timeout time.Duration) establishSessionResult {
	t.Helper()
	select {
	case r := <-res:
		return r
	case <-time.After(timeout):
		t.Fatal("establishSession did not return in time, likely deadlocked waiting on pty input")
		return establishSessionResult{}
	}
}

// ----------------------------------------------------------------------
//
// establishSession
//
// ----------------------------------------------------------------------

// TestEstablishSessionHostOnly - This test verifies the
// EnableHostAuthentication-only combination: the returned session
// carries the real current OS account on both Username and
// HostUsername, with no password or TOTP prompt at all, so stdin and
// stdout are never touched and nil is passed for both rather than a
// pty, confirming this path truly performs no I/O of its own.
func TestEstablishSessionHostOnly(t *testing.T) {
	cfg := config.SystemConfig{EnableHostAuthentication: true}
	audit := newTestAudit(t)

	session, err := establishSession(cfg, auth.Users{}, nil, nil, audit, nil, nil)
	if err != nil {
		t.Fatalf("establishSession returned unexpected error: %v", err)
	}

	u, uerr := user.Current()
	if uerr != nil {
		t.Skipf("cannot determine the current OS user in this environment: %v", uerr)
	}
	if session.Username != u.Username || session.HostUsername != u.Username {
		t.Errorf("session = %+v, want Username and HostUsername both %q", session, u.Username)
	}
	if !session.Authenticated {
		t.Error("expected a host-authenticated session to be marked Authenticated")
	}
}

// TestEstablishSessionCLIOnlySuccess - This test verifies the
// EnableCLIAuthentication-only combination succeeds end to end
// through a real pseudo terminal, a correct username and password
// typed in return an authenticated session for that username, with no
// HostUsername set at all, since EnableHostAuthentication is off.
func TestEstablishSessionCLIOnlySuccess(t *testing.T) {
	master, slave := newPTY(t)
	users := auth.Users{"alice": {Username: "alice", PasswordHash: mustHashPassword(t, "s3cret")}}
	provider := auth.NewLocalAuthProvider(users)
	cfg := config.SystemConfig{EnableCLIAuthentication: true, LoginMaxAttempts: 3}
	audit := newTestAudit(t)

	resCh := runEstablishSession(cfg, users, provider, audit, slave, slave)
	sendLine(t, master, "alice")
	sendLine(t, master, "s3cret")
	res := awaitEstablishSession(t, resCh, 5*time.Second)

	if res.err != nil {
		t.Fatalf("establishSession returned unexpected error: %v", res.err)
	}
	if res.session.Username != "alice" || !res.session.Authenticated {
		t.Errorf("unexpected session: %+v", res.session)
	}
	if res.session.HostUsername != "" {
		t.Errorf("expected no HostUsername for a CLI-only session, got %q", res.session.HostUsername)
	}
}

// TestEstablishSessionCLIOnlyWrongPasswordExhaustsAttempts - This
// test verifies that a wrong password, retried until LoginMaxAttempts
// is used up, returns auth.ErrLoginFailed, exercising PromptLogin's
// own attempt loop through establishSession rather than calling
// PromptLogin directly.
func TestEstablishSessionCLIOnlyWrongPasswordExhaustsAttempts(t *testing.T) {
	master, slave := newPTY(t)
	users := auth.Users{"alice": {Username: "alice", PasswordHash: mustHashPassword(t, "s3cret")}}
	provider := auth.NewLocalAuthProvider(users)
	cfg := config.SystemConfig{EnableCLIAuthentication: true, LoginMaxAttempts: 2}
	audit := newTestAudit(t)

	resCh := runEstablishSession(cfg, users, provider, audit, slave, slave)
	for i := 0; i < cfg.LoginMaxAttempts; i++ {
		sendLine(t, master, "alice")
		sendLine(t, master, "wrong-password")
	}
	res := awaitEstablishSession(t, resCh, 5*time.Second)

	if res.err != auth.ErrLoginFailed {
		t.Errorf("establishSession error = %v, want auth.ErrLoginFailed", res.err)
	}
	if res.session != nil {
		t.Errorf("expected a nil session after exhausting every attempt, got %+v", res.session)
	}
}

// TestEstablishSessionHostAndCLICombinedCarriesHostUsername - This
// test verifies the shared-account combination described on
// Session.HostUsername's own doc comment: with both
// EnableHostAuthentication and EnableCLIAuthentication on, the
// returned session's Username is whichever identity the CLI login
// actually resolved to, "alice" here, while HostUsername and
// HostConnectedAt still carry the real OS account's own identity,
// established first through auth.SessionFromHostIdentity.
func TestEstablishSessionHostAndCLICombinedCarriesHostUsername(t *testing.T) {
	master, slave := newPTY(t)
	users := auth.Users{"alice": {Username: "alice", PasswordHash: mustHashPassword(t, "s3cret")}}
	provider := auth.NewLocalAuthProvider(users)
	cfg := config.SystemConfig{EnableHostAuthentication: true, EnableCLIAuthentication: true, LoginMaxAttempts: 3}
	audit := newTestAudit(t)

	before := time.Now()
	resCh := runEstablishSession(cfg, users, provider, audit, slave, slave)
	sendLine(t, master, "alice")
	sendLine(t, master, "s3cret")
	res := awaitEstablishSession(t, resCh, 5*time.Second)
	after := time.Now()

	if res.err != nil {
		t.Fatalf("establishSession returned unexpected error: %v", res.err)
	}
	if res.session.Username != "alice" {
		t.Errorf("session.Username = %q, want %q, the CLI-resolved identity", res.session.Username, "alice")
	}
	u, uerr := user.Current()
	if uerr != nil {
		t.Skipf("cannot determine the current OS user in this environment: %v", uerr)
	}
	if res.session.HostUsername != u.Username {
		t.Errorf("session.HostUsername = %q, want %q, the real OS account", res.session.HostUsername, u.Username)
	}
	if res.session.HostConnectedAt.Before(before) || res.session.HostConnectedAt.After(after) {
		t.Errorf("session.HostConnectedAt = %v, want a time between %v and %v", res.session.HostConnectedAt, before, after)
	}
}

// TestEstablishSessionHostAndTOTPVerifiedSecondFactor - This test
// verifies the host-plus-TOTP standalone step up: with only
// EnableHostAuthentication and EnableTOTPAuthentication on, no CLI
// login at all, a resolved host identity whose own users.yaml entry
// has a TOTPSecret set is still challenged for a code before the
// session is handed back, and a correct one succeeds.
func TestEstablishSessionHostAndTOTPVerifiedSecondFactor(t *testing.T) {
	u, uerr := user.Current()
	if uerr != nil {
		t.Skipf("cannot determine the current OS user in this environment: %v", uerr)
	}
	secret, serr := auth.GenerateTOTPSecret()
	if serr != nil {
		t.Fatalf("GenerateTOTPSecret returned error: %v", serr)
	}
	code, cerr := auth.GenerateTOTPCode(secret, time.Now())
	if cerr != nil {
		t.Fatalf("GenerateTOTPCode returned error: %v", cerr)
	}

	master, slave := newPTY(t)
	users := auth.Users{u.Username: {Username: u.Username, TOTPSecret: secret}}
	cfg := config.SystemConfig{EnableHostAuthentication: true, EnableTOTPAuthentication: true, LoginMaxAttempts: 3}
	audit := newTestAudit(t)

	resCh := runEstablishSession(cfg, users, nil, audit, slave, slave)
	sendLine(t, master, code)
	res := awaitEstablishSession(t, resCh, 5*time.Second)

	if res.err != nil {
		t.Fatalf("establishSession returned unexpected error: %v", res.err)
	}
	if res.session.Username != u.Username {
		t.Errorf("session.Username = %q, want %q", res.session.Username, u.Username)
	}
}

// TestEstablishSessionHostAndTOTPWrongCodeExhaustsAttempts - This
// test verifies the failure half of the standalone TOTP step up: a
// wrong code, retried until LoginMaxAttempts is used up, returns
// auth.ErrLoginFailed rather than handing back a session anyway.
func TestEstablishSessionHostAndTOTPWrongCodeExhaustsAttempts(t *testing.T) {
	u, uerr := user.Current()
	if uerr != nil {
		t.Skipf("cannot determine the current OS user in this environment: %v", uerr)
	}
	secret, serr := auth.GenerateTOTPSecret()
	if serr != nil {
		t.Fatalf("GenerateTOTPSecret returned error: %v", serr)
	}

	master, slave := newPTY(t)
	users := auth.Users{u.Username: {Username: u.Username, TOTPSecret: secret}}
	cfg := config.SystemConfig{EnableHostAuthentication: true, EnableTOTPAuthentication: true, LoginMaxAttempts: 2}
	audit := newTestAudit(t)

	resCh := runEstablishSession(cfg, users, nil, audit, slave, slave)
	for i := 0; i < cfg.LoginMaxAttempts; i++ {
		sendLine(t, master, "000000")
	}
	res := awaitEstablishSession(t, resCh, 5*time.Second)

	if res.err != auth.ErrLoginFailed {
		t.Errorf("establishSession error = %v, want auth.ErrLoginFailed", res.err)
	}
}

// TestEstablishSessionHostAndTOTPSkipsUserWithNoSecret - This test
// verifies that EnableTOTPAuthentication being on does not challenge
// a resolved host identity that has no TOTPSecret configured at all,
// auth.SecondFactorRequired's own false case, the session is simply
// handed back without ever touching stdin.
func TestEstablishSessionHostAndTOTPSkipsUserWithNoSecret(t *testing.T) {
	u, uerr := user.Current()
	if uerr != nil {
		t.Skipf("cannot determine the current OS user in this environment: %v", uerr)
	}
	cfg := config.SystemConfig{EnableHostAuthentication: true, EnableTOTPAuthentication: true, LoginMaxAttempts: 3}
	audit := newTestAudit(t)

	session, err := establishSession(cfg, auth.Users{}, nil, nil, audit, nil, nil)
	if err != nil {
		t.Fatalf("establishSession returned unexpected error: %v", err)
	}
	if session.Username != u.Username {
		t.Errorf("session.Username = %q, want %q", session.Username, u.Username)
	}
}

// TestEstablishSessionLogsLoginEntries - This test verifies that
// establishSession writes a LOGIN entry to the audit log for both a
// failed and a subsequent successful attempt, not only for the final
// outcome, the same audit trail this project has always produced for
// its CLI login path.
func TestEstablishSessionLogsLoginEntries(t *testing.T) {
	master, slave := newPTY(t)
	users := auth.Users{"alice": {Username: "alice", PasswordHash: mustHashPassword(t, "s3cret")}}
	provider := auth.NewLocalAuthProvider(users)
	cfg := config.SystemConfig{EnableCLIAuthentication: true, LoginMaxAttempts: 3}
	auditPath := filepath.Join(t.TempDir(), "audit.log")
	audit := auditlog.New(auditPath, discardAuditLogger())
	if err := audit.Enable(); err != nil {
		t.Fatalf("failed to enable audit log: %v", err)
	}
	defer audit.Close()

	resCh := runEstablishSession(cfg, users, provider, audit, slave, slave)
	sendLine(t, master, "alice")
	sendLine(t, master, "wrong-password")
	sendLine(t, master, "alice")
	sendLine(t, master, "s3cret")
	res := awaitEstablishSession(t, resCh, 5*time.Second)
	if res.err != nil {
		t.Fatalf("establishSession returned unexpected error: %v", res.err)
	}

	got := readAuditLog(t, auditPath)
	if !bytes.Contains([]byte(got), []byte("FAIL\tLOGIN")) {
		t.Errorf("expected a failed LOGIN entry in the audit log, got %q", got)
	}
	if !bytes.Contains([]byte(got), []byte("OK\tLOGIN")) {
		t.Errorf("expected a successful LOGIN entry in the audit log, got %q", got)
	}
}

// ----------------------------------------------------------------------
//
// runHashPasswordUtility
//
// ----------------------------------------------------------------------

// TestRunHashPasswordUtilityPrintsAVerifiableHash - This test
// verifies that runHashPasswordUtility reads a masked password from
// stdin and prints a bcrypt hash to stdout that auth.VerifyPassword
// actually accepts for that same password, not merely that some
// output was produced.
func TestRunHashPasswordUtilityPrintsAVerifiableHash(t *testing.T) {
	master, slave := newPTY(t)

	doneCh := make(chan string, 1)
	go func() {
		doneCh <- captureStdout(t, func() { runHashPasswordUtility(slave) })
	}()
	sendLine(t, master, "s3cret")

	var out string
	select {
	case out = <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("runHashPasswordUtility did not return in time")
	}

	hash := ""
	for _, line := range splitLines(out) {
		if line != "" {
			hash = line
		}
	}
	if hash == "" {
		t.Fatalf("runHashPasswordUtility printed no hash, output was %q", out)
	}
	if !auth.VerifyPassword(hash, "s3cret") {
		t.Errorf("printed hash %q does not verify against the password that was typed", hash)
	}
}

// splitLines - This function splits s on newlines and drops any
// trailing empty entries left by a final newline, so
// TestRunHashPasswordUtilityPrintsAVerifiableHash can find the one
// real line of output regardless of exactly how many trailing
// newlines fmt.Println left behind.
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// ----------------------------------------------------------------------
//
// runLoop
//
// ----------------------------------------------------------------------

// newPTYReadline - This function builds a real *readline.Instance
// backed by a pty pair, rather than the real process's own os.Stdin
// and os.Stdout the way main's own real call site does. chzyer/readline's
// Config accepts an explicit Stdin and Stdout, see its own doc
// comment, precisely so a caller does not have to be the real process
// terminal, which is what makes runLoop testable at all, see its own
// doc comment, "split out from main so it can be exercised without a
// real terminal attached". A plain os.Pipe still is not enough here,
// unlike a bare io.Reader elsewhere in this file, because readline
// itself puts the terminal into raw mode to read arrow keys, Ctrl-C,
// and Ctrl-D one keystroke at a time rather than line buffered, the
// same requirement golang.org/x/term has, so this still needs a real
// pty, not just something Reader shaped.
func newPTYReadline(t *testing.T, tree map[string]*command.Command) (*readline.Instance, *completer.TreeListener, *os.File) {
	t.Helper()
	master, slave := newPTY(t)
	logger := discardAuditLogger()
	position := command.NewCommandLevelStack("exec", "", tree)
	rl, err := readline.NewEx(&readline.Config{
		Stdin:           slave,
		Stdout:          slave,
		AutoComplete:    completer.NoopCompleter{},
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		t.Fatalf("failed to construct a pty backed readline.Instance: %v", err)
	}
	t.Cleanup(func() { rl.Close() })
	treeListener := completer.New(position, rl, logger, nil)
	rl.Config.Listener = treeListener
	return rl, treeListener, master
}

// runLoopTestContext - This function builds a minimal, real
// *command.AppContext for a runLoop test, wired against tree, with a
// recordingAuditor, see main_test.go, so a test can assert on which
// commands runLoop actually logged.
func runLoopTestContext(tree map[string]*command.Command) (*command.AppContext, *recordingAuditor) {
	audit := &recordingAuditor{}
	base := &command.CommandLevel{Name: "exec", IsBase: true, Tree: tree}
	ctx := &command.AppContext{
		State:      &cmd.ExampleState{},
		Logger:     discardAuditLogger(),
		Levels:     &command.TreeStructure{Order: []*command.CommandLevel{base}},
		Audit:      audit,
		Session:    &auth.Session{CommandLevel: "exec"},
		Position:   command.NewCommandLevelStack("exec", "", tree),
		Translator: nil,
	}
	return ctx, audit
}

// TestRunLoopQuitCommandLogsSessionEnd - This test verifies runLoop's
// command.ErrQuit exit path end to end through a real pty: typing
// "quit" dispatches to a handler that returns command.ErrQuit, runLoop
// returns, having logged SESSION END on its way out, see runLoop's own
// doc comment and logSessionEnd.
func TestRunLoopQuitCommandLogsSessionEnd(t *testing.T) {
	tree := map[string]*command.Command{
		"quit": {RunFunc: func(ctx *command.AppContext, args []string) error { return command.ErrQuit }},
	}
	rl, treeListener, master := newPTYReadline(t, tree)
	ctx, audit := runLoopTestContext(tree)

	doneCh := make(chan struct{})
	go func() {
		runLoop(rl, treeListener, ctx, runLoopOptions{})
		close(doneCh)
	}()
	sendLine(t, master, "quit")

	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("runLoop did not return after a command.ErrQuit handler, likely never reached the exit path")
	}

	if !audit.hasEntry("SESSION END") {
		t.Errorf("expected a SESSION END audit entry, got %+v", audit.entries)
	}
}

// TestRunLoopEOFExitsAndLogsSessionEnd - This test verifies runLoop's
// other real exit path, Ctrl-D on an empty line delivering io.EOF from
// readline, by closing the master side of the pty outright, the same
// end-of-input condition a real disconnect produces. See runLoop's own
// doc comment on opts.PreventEscape for why this, unlike Ctrl-C, is
// unconditional here, opts.PreventEscape defaults to false.
func TestRunLoopEOFExitsAndLogsSessionEnd(t *testing.T) {
	tree := map[string]*command.Command{}
	rl, treeListener, master := newPTYReadline(t, tree)
	ctx, audit := runLoopTestContext(tree)

	doneCh := make(chan struct{})
	go func() {
		runLoop(rl, treeListener, ctx, runLoopOptions{})
		close(doneCh)
	}()
	master.Close()

	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("runLoop did not return after the pty's master side closed, likely never observed EOF")
	}

	if !audit.hasEntry("SESSION END") {
		t.Errorf("expected a SESSION END audit entry, got %+v", audit.entries)
	}
}

// TestRunLoopDispatchesAKnownCommandAndAudits - This test verifies
// the ordinary dispatch path, a real command typed and resolved,
// RunFunc actually called, and the audit entry runLoop's own doc
// comment describes recorded, all driven through a genuine pty rather
// than calling command.Resolve or RunFunc directly, the way this
// project's cmd/ package tests already do, since this is specifically
// exercising runLoop's own read, resolve, dispatch wiring, not the
// handler underneath it.
func TestRunLoopDispatchesAKnownCommandAndAudits(t *testing.T) {
	ran := false
	tree := map[string]*command.Command{
		"hello": {RunFunc: func(ctx *command.AppContext, args []string) error { ran = true; return nil }},
		"quit":  {RunFunc: func(ctx *command.AppContext, args []string) error { return command.ErrQuit }},
	}
	rl, treeListener, master := newPTYReadline(t, tree)
	ctx, audit := runLoopTestContext(tree)
	audit.would = true // so the dispatched "hello" command is actually recorded, see WouldLog's own contract

	doneCh := make(chan struct{})
	go func() {
		runLoop(rl, treeListener, ctx, runLoopOptions{})
		close(doneCh)
	}()
	sendLine(t, master, "hello")
	sendLine(t, master, "quit")

	select {
	case <-doneCh:
	case <-time.After(5 * time.Second):
		t.Fatal("runLoop did not return")
	}

	if !ran {
		t.Error("expected the \"hello\" command's RunFunc to have been called")
	}
	if !audit.hasEntry("hello") {
		t.Errorf("expected an audit entry for \"hello\", got %+v", audit.entries)
	}
}
