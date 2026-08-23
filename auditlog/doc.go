// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package auditlog implements a timestamped, append-only record of what
every user did in the CLI, so a deployment with real users can answer
who ran what and whether it succeeded. See AuditLog and its Log method.

Audit logging can be turned on at startup or toggled at runtime, and
the underlying file is opened lazily, only on the first Enable call, so
a project that never enables auditing never touches the filesystem for
it at all.
*/
package auditlog
