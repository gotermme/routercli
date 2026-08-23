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
	a.writeLocked(username, command, success)
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
	a.writeLocked(username, command, success)
}

// ----------------------------------------------------------------------
// Private Methods
// ----------------------------------------------------------------------

// writeLocked - This method does the actual write. Callers must hold
// a.mu and have already checked that a.file is not nil.
func (a *AuditLog) writeLocked(username, command string, success bool) {
	if username == "" {
		username = "-"
	}
	result := "OK"
	if !success {
		result = "FAIL"
	}

	line := fmt.Sprintf("%s\t%s\t%s\t%s\n",
		time.Now().Format(time.RFC3339), username, result, command)
	// A transient write failure here must never crash the CLI. An
	// audit log that can bring down the tool would itself be a
	// significant reliability problem, so the error is reported to the
	// general logger instead of being returned or panicked on.
	if _, err := a.file.WriteString(line); err != nil {
		a.logger.Errorf("audit log write failed: %v", err)
	}
}
