// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"

	"github.com/gotermme/routercli/auditlog"
	"github.com/gotermme/routercli/command"
)

// init - This function registers the "audit-log enable", "audit-log
// disable", and "audit-log status" commands. "enable" and "disable"
// are only reachable once a session has moved past the base Command
// Level. That is a property of the Tree Structure itself. These two
// are listed in var/tree/level_exec.yaml, never in level_base.yaml or
// level_common.yaml, so a base level session simply cannot type or
// resolve them at all. See command.LoadTreeStructure. This file does
// not enforce that placement itself. Being able to turn audit logging
// off is exactly the kind of control that should never be available
// to an unelevated session, since it is what someone covering their
// tracks would reach for first, and Tree Structure placement alone
// already guarantees that.
//
// ctx.Audit is typed as command.Auditor, an interface with one Log
// method, rather than *auditlog.AuditLog, so these handlers type
// assert it back to the concrete type to reach Enable, Disable, and
// Enabled. This is a small tradeoff for keeping command.AppContext
// decoupled from package auditlog specifically.
func init() {
	command.Register("audit-log.enable", func(ctx *command.AppContext, args []string) error {
		a, ok := ctx.Audit.(*auditlog.AuditLog)
		if !ok || a == nil {
			fmt.Println("%", ctx.Translator.T("audit_log.not_configured"))
			return nil
		}
		if err := a.Enable(); err != nil {
			return err
		}
		fmt.Println(ctx.Translator.T("audit_log.enabled"))
		return nil
	})

	command.Register("audit-log.disable", func(ctx *command.AppContext, args []string) error {
		a, ok := ctx.Audit.(*auditlog.AuditLog)
		if !ok || a == nil {
			fmt.Println("%", ctx.Translator.T("audit_log.not_configured"))
			return nil
		}
		a.Disable()
		fmt.Println(ctx.Translator.T("audit_log.disabled"))
		return nil
	})

	command.Register("audit-log.status", func(ctx *command.AppContext, args []string) error {
		a, ok := ctx.Audit.(*auditlog.AuditLog)
		if !ok || a == nil {
			fmt.Println(ctx.Translator.T("audit_log.not_configured_period"))
			return nil
		}
		if a.Enabled() {
			fmt.Println(ctx.Translator.T("audit_log.status_enabled"))
		} else {
			fmt.Println(ctx.Translator.T("audit_log.status_disabled"))
		}
		return nil
	})
}
