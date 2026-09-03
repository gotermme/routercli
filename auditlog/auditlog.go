// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auditlog

import (
	"fmt"
	"os"
	"time"
)

// ----------------------------------------------------------------------
// Public Methods - AuditLog
// ----------------------------------------------------------------------

// Enable - This method turns on audit logging and opens the underlying
// file in append mode, if it is not already open. This is safe to call
// whether logging was previously on or off, and safe to call from a
// running session.
func (a *AuditLog) Enable() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file == nil {
		f, err := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
		if err != nil {
			return fmt.Errorf("error opening audit log %q: %v", a.path, err)
		}
		a.file = f
	}
	a.enabled = true
	return nil
}

// Disable - This method turns off audit logging. It deliberately
// leaves the file handle open rather than closing it. There is no
// reason to release the handle just because logging is paused, and
// leaving it open lets auditing be re-enabled later without reopening
// the file.
func (a *AuditLog) Disable() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.enabled = false
}

// Enabled - This method reports whether audit logging is currently on.
func (a *AuditLog) Enabled() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.enabled
}

// WouldLog - This method reports whether a call to Log would actually
// write anything, meaning audit logging is enabled and the file is
// open. A command such as "audit-log disable" flips enabled to false
// as its own side effect, so by the time a generic Log call ran
// afterward it would find logging already off and silently swallow
// its own entry. A caller snapshots WouldLog before running such a
// command, and uses that snapshot to decide whether to log it
// afterward instead. See main.go's runLoop for where this matters.
func (a *AuditLog) WouldLog() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.enabled && a.file != nil
}

// Close - This method closes the underlying file, if one was ever
// opened. Safe to call even if auditing was never enabled.
func (a *AuditLog) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.file != nil {
		return a.file.Close()
	}
	return nil
}

// Log - This method records a single audit entry containing a
// timestamp, who ran the command, the exact command line, and whether
// it succeeded. username is recorded as "-" when the session is not
// authenticated. It silently does nothing when auditing is disabled or
// the file has not yet been opened. Most callers do not need to check
// WouldLog first. The one case that does is a logging command that
// disables auditing as its own side effect, such as "audit-log
// disable", see ForceLog below. The timestamp uses RFC3339 rather than
// time.LstdFlags' format, since an audit record needs to be
// unambiguous across time zones and machine-readable by whatever
// tooling later parses this file.
func (a *AuditLog) Log(username, command string, success bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.enabled || a.file == nil {
		return
	}
	a.writeLocked(time.Now(), username, command, success)
}

// ForceLog - This method writes an entry unconditionally, as long as
// the file is open, skipping the enabled check that Log performs. This
// exists for exactly one situation: a caller snapshotted WouldLog as
// true before running a command whose own side effect was to flip
// enabled to false, such as "audit-log disable". Without ForceLog,
// that command would swallow its own log entry, since by the time
// anything tried to log it, enabled would already be off. Every other
// caller should use the normal Log instead.
func (a *AuditLog) ForceLog(username, command string, success bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.file == nil {
		return
	}
	a.writeLocked(time.Now(), username, command, success)
}

// LogAt - This method does exactly what Log does, using when instead
// of time.Now() as the audit entry's own recorded timestamp. A real
// RouterCLI daemon calls this for an AuditEvent message a CLI session
// sent it, when carrying the moment that session actually dispatched
// the command, not the later moment the daemon itself got around to
// writing it; see FormatEntry's own doc comment for the same
// reasoning. Every other rule Log follows, silently doing nothing when
// auditing is disabled or the file is not open, applies here
// identically.
func (a *AuditLog) LogAt(when time.Time, username, command string, success bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.enabled || a.file == nil {
		return
	}
	a.writeLocked(when, username, command, success)
}

// ----------------------------------------------------------------------
// Public Functions
// ----------------------------------------------------------------------

// FormatEntry - This function builds the exact text of one audit log
// line, a timestamp, who ran the command, OK or FAIL, and the command
// text itself, tab separated, one line, RFC3339 timestamp so a record
// stays unambiguous across time zones and machine readable by whatever
// tooling later parses this file. username is recorded as "-" when
// empty, matching an unauthenticated session's own existing
// convention.
//
// This function exists so this package's own writeLocked below, and a
// real RouterCLI daemon writing an audit line on behalf of a session
// that sent it an AuditEvent message rather than opening
// AuditLogFile itself, share one formatting function, rather than
// this project maintaining two versions of what an audit line looks
// like; see claude/DAEMON_ARCHITECTURE_DESIGN.md's own "Daemon owned
// audit logging" section, which settles this directly: "the two modes
// share that formatting function rather than maintaining two separate
// versions of what an audit line looks like." when is passed in
// rather than read from time.Now() here specifically so a daemon can
// record the moment a session actually dispatched a command, carried
// in its own AuditEvent message, rather than the later moment the
// daemon itself got around to writing it.
func FormatEntry(when time.Time, username, command string, success bool) string {
	if username == "" {
		username = "-"
	}
	result := "OK"
	if !success {
		result = "FAIL"
	}
	return fmt.Sprintf("%s\t%s\t%s\t%s\n", when.Format(time.RFC3339), username, result, command)
}

// ----------------------------------------------------------------------
// Private Methods
// ----------------------------------------------------------------------

// writeLocked - This method does the actual write. Callers must hold
// a.mu and have already checked that a.file is not nil.
func (a *AuditLog) writeLocked(when time.Time, username, command string, success bool) {
	line := FormatEntry(when, username, command, success)
	// A transient write failure here must never crash the CLI. An
	// audit log that can bring down the tool would itself be a
	// significant reliability problem, so the error is reported to the
	// general logger instead of being returned or panicked on.
	if _, err := a.file.WriteString(line); err != nil {
		a.logger.Errorf("audit log write failed: %v", err)
	}
}
