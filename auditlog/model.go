// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auditlog

import (
	"os"
	"sync"

	"github.com/gologme/log"
)

// ----------------------------------------------------------------------
// Define Object Model
// ----------------------------------------------------------------------

// AuditLog - This type implements the audit trail. The file handle is
// opened lazily, so the constructor never touches the filesystem when
// auditing is disabled at startup.
type AuditLog struct {
	mu      sync.Mutex
	path    string
	file    *os.File
	enabled bool
	logger  *log.Logger
}

// ----------------------------------------------------------------------
// Initialization Functions
// ----------------------------------------------------------------------

// New - This function creates an AuditLog at the given path. It does
// not open the file yet, that happens on the first call to Enable.
// logger is used to report write failures that occur later, after the
// file has been opened, see writeLocked. A nil logger is replaced with
// a default logger writing to stderr, so a caller can never crash this
// constructor just by omitting one.
func New(path string, logger *log.Logger) *AuditLog {
	if logger == nil {
		logger = log.New(os.Stderr, "", log.LstdFlags)
	}
	return &AuditLog{path: path, logger: logger}
}
