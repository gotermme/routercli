// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// init - This function registers "enable" and "disable", the exec
// Command Level's own entry and exit commands. See
// var/tree/tree_structure.yaml's "exec" entry. This is an entirely
// ordinary command file, registered the normal way, like every other
// file in this package. "exec" is not special cased anywhere in
// package command. This file could just as easily have been named
// cmd_operator.go entering a level called "operator", and nothing
// beyond the string literals below would change.
//
// command.EnterCommandLevel and command.ExitCommandLevel do the
// generic mechanical work, the parent check, the password check,
// updating Session.CommandLevel, and swapping the root
// CommandLevelStack frame, and deliberately report only what
// happened, without printing anything themselves. See their own doc
// comments. Every word printed
// below, and whether to print anything at all, is this file's
// decision alone. A different project might log this to the audit
// trail instead, or say nothing.
func init() {
	command.Register("enable", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["exec"]
		entered, err := command.EnterCommandLevel(ctx, level, ctx.Levels.ByName[level.Parent])
		if err != nil {
			return err
		}
		if !entered {
			fmt.Println(ctx.Translator.T("enable.already_here"))
			return nil
		}
		fmt.Println(ctx.Translator.T("enable.entered"))
		return nil
	})

	command.Register("disable", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["exec"]
		exited, err := command.ExitCommandLevel(ctx, level, ctx.Levels.ByName[level.Parent])
		if err != nil {
			return err
		}
		if !exited {
			fmt.Println(ctx.Translator.T("disable.not_here"))
			return nil
		}
		fmt.Println(ctx.Translator.T("disable.left"))
		return nil
	})
}
