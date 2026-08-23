// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"fmt"
	"os"

	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"
)

// init - This function registers "password manager" for config mode,
// matching a real HP ProCurve switch's own directive of the same
// name. Running it prompts for a new secret using auth.PromptSecret,
// the same mechanism every other password prompt in this project
// uses, hashes it with auth.HashPassword, the same algorithm
// etc/users.yaml's password hashes use, and stores the result as the
// PasswordHash on the command.CommandLevel matching
// ctx.Session.CommandLevel, that is, whichever Command Level the
// session is currently inside. Config mode is only ever reachable
// from within some non-base level, see var/tree/level_config.yaml
// being merged into level_exec.yaml. The stored hash is exactly the
// secret checked the next time someone tries to re-enter this same
// level from its parent, see command.EnterCommandLevel.
//
// This command always prompts for the secret rather than accepting it
// as a same-line argument. A same-line argument would flow through
// main.go's runLoop exactly like any other typed line, which both the
// audit log and readline's own history file record verbatim, leaving
// the plaintext secret written to disk. Prompting keeps this
// consistent with every other secret in this project, none of which
// ever appear as a same-line argument.
//
// Since a project can define any number of Command Levels in
// var/tree/tree_structure.yaml, this command can exist in more than
// one level's own config mode tree, and each occurrence sets that
// level's own secret independently, simply by running while
// ctx.Session.CommandLevel is different each time.
//
// Negated, as "no password manager", this clears the secret back to
// an empty string, restoring the default behavior of entering this
// level without a prompt.
func init() {
	command.Register("password-manager", func(ctx *command.AppContext, args []string) error {
		level, ok := ctx.Levels.ByName[ctx.Session.CommandLevel]
		if !ok {
			// Session.CommandLevel is always kept in sync with an actual,
			// currently loaded CommandLevel by whichever cmd/cmd_*.go file
			// last called command.EnterCommandLevel or ExitCommandLevel,
			// so this should not happen. Checked rather than assumed.
			return fmt.Errorf("%s", ctx.Translator.T("password_manager.no_current_level"))
		}

		if ctx.Negated {
			level.PasswordHash = ""
			ctx.Logger.Debugln("DEBUG: password cleared for Command Level", level.Name)
			fmt.Println(ctx.Translator.T("password_manager.cleared"))
			return nil
		}

		password, err := auth.PromptSecret(os.Stdout, int(os.Stdin.Fd()), ctx.Translator)
		if err != nil {
			return err
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}
		level.PasswordHash = hash
		ctx.Logger.Debugln("DEBUG: password set for Command Level", level.Name)
		fmt.Println(ctx.Translator.T("password_manager.confirm"))
		return nil
	})
}
