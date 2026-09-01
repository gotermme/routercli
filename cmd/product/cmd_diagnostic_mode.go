// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// init - This function registers "diagnostic-mode" and
// "exit-diagnostic-mode", the diagnostic Command Level's own entry
// and exit commands, by their internal registered names. See
// var/tree/tree_structure.yaml's "diagnostic" entry. The word a
// session actually types to leave, "return", is decided entirely by
// var/tree/level_diagnostic.yaml's own tree, whose "return" entry
// points its own "run:" at this same registered "exit-diagnostic-mode"
// name; nothing here needed to change when that word did, see that
// file's own doc comment. This is the same shape as cmd_enable.go.
// See that file's own doc comment for why this is an entirely
// ordinary command file, with nothing special cased in package
// command about "diagnostic" as a name. cmd_diagnostic.go, note the
// different filename, registers "self-test", a command reachable from
// inside diagnostic mode, a different thing entirely from entering or
// leaving diagnostic mode, which is this file's job.
func init() {
	command.Register("diagnostic-mode", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["diagnostic"]
		entered, err := command.EnterCommandLevel(ctx, level, ctx.Levels.ByName[level.Parent])
		if err != nil {
			return err
		}
		if !entered {
			fmt.Println(ctx.Translator.T("diagnostic_mode.already_here"))
			return nil
		}
		fmt.Println(ctx.Translator.T("diagnostic_mode.entered"))
		return nil
	})

	command.Register("exit-diagnostic-mode", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["diagnostic"]
		exited, err := command.ExitCommandLevel(ctx, level, ctx.Levels.ByName[level.Parent])
		if err != nil {
			return err
		}
		if !exited {
			fmt.Println(ctx.Translator.T("exit_diagnostic_mode.not_here"))
			return nil
		}
		fmt.Println(ctx.Translator.T("exit_diagnostic_mode.left"))
		return nil
	})
}
