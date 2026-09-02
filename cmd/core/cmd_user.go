// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// init - This function registers "user", which enters the user
// Command Level, a nested mode reachable directly from the base
// level, the same push mechanism cmd_configure.go and
// cmd_interface.go use, not a root swap Command Level such as exec or
// diagnostic. See those two files' own doc comments for why a
// nested, stacking mode cannot use command.EnterCommandLevel. The
// tree and prompt suffix come from ctx.Levels.ByName["user"], loaded
// from var/tree/tree_structure.yaml at startup, rather than
// hardcoded here.
//
// Beyond the ordinary parent check every other Command Level entry
// enforces, this one adds a second requirement, enforced by
// requireLoggedIn below: the session must already be logged in. This
// matters because every command reachable only from inside this
// level, totp enable and totp disable in cmd_totp.go and password
// change in cmd_password.go, acts on the current session's own entry
// in the user database, which only makes sense once a session
// actually knows who it is.
//
// requireLoggedIn is waived while ctx.ReplayingStartupConfig is true,
// the same trust command.LoadStartupConfig already extends to every
// other Command Level's own password gate, see that field's own doc
// comment in model.go. A saved startup-config can only ever contain a
// "user" block around a runtime defined command alias, see
// cmd/product/cmd_show.go's own runningConfigLines, never a live
// action such as "totp enable" or "password change", so this waiver
// grants no more than replaying that one alias back in, and the
// trust behind it is the same, this process itself already having
// been allowed to run, and to read this file, by the operating
// system, not a credential typed at a terminal, since a boot time
// replay runs before any session has logged in at all.
func init() {
	command.Register("user", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["user"]
		if err := command.RequireCurrentCommandLevel(ctx, "user", level.Parent); err != nil {
			return err
		}
		if !ctx.ReplayingStartupConfig {
			if err := requireLoggedIn(ctx); err != nil {
				return err
			}
		}
		username := "<none, replaying startup-config>"
		if ctx.Session != nil {
			username = ctx.Session.Username
		}
		ctx.Logger.Debugln("DEBUG: entering user mode for user", username)
		ctx.Position.Push(command.CommandLevelFrame{
			Name:         "user",
			PromptSuffix: level.PromptSuffix,
			Tree:         level.Tree,
		})
		return nil
	})
}

// requireLoggedIn - This function returns an error unless ctx.Session
// is both non-nil and Authenticated. It is shared by the "user"
// command above and by every command reachable only from inside the
// user Command Level, totp enable and totp disable in cmd_totp.go and
// password change in cmd_password.go, since all of them act on the
// current session's own identity and must refuse to run at all for a
// session that never logged in.
//
// Authenticated is only ever true once auth.PromptLogin has verified
// a real username and password, which itself only happens when
// AuthRequired is set in the project's configuration, see main.go.
// Checking Authenticated alone is therefore enough to enforce both
// "AuthRequired is on" and "this session really did log in", since
// there is no other path that ever sets it true.
func requireLoggedIn(ctx *command.AppContext) error {
	if ctx.Session == nil || !ctx.Session.Authenticated {
		return fmt.Errorf("%s", ctx.Translator.T("user.login_required"))
	}
	return nil
}
