// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"github.com/gotermme/routercli/command"
)

// init - This function registers "configure terminal", which enters
// config mode by pushing a new CommandLevelFrame onto ctx.Position.
// The tree and prompt suffix for that frame come from
// ctx.Levels.ByName["config"], a CommandLevel loaded and merged with
// the common tree at startup from var/tree/tree_structure.yaml, see
// command.LoadTreeStructure, rather than loaded or hardcoded here.
// That way a broken level_config.yaml fails the program at startup
// rather than the first time someone actually types this command, and
// its prompt suffix lives in one place, the manifest, instead of
// being duplicated as a Go string literal here too. "config" is still
// a literal string in this file, deliberately. It is this command's
// own business which Command Level it enters, the same way
// "configure.terminal" itself is a literal a moment later. Nothing in
// package command or main.go names "config" anywhere.
//
// command.RequireCurrentCommandLevel(ctx, "config", level.Parent)
// below is the exact same check that cmd_enable.go and
// cmd_diagnostic_mode.go make, through command.EnterCommandLevel,
// which calls it internally.
// config's manifest entry sets parent: exec the exact same way exec's
// own entry sets parent: base. It is just called here directly rather
// than through EnterCommandLevel, since entering config also needs to
// push a CommandLevelFrame rather than swap the root tree. See
// EnterCommandLevel's own doc comment on why nested, stacking modes
// such as config and config-if cannot use it. Structurally this
// should already be unreachable, since level_base.yaml never lists
// "configure" as a command at all, see level_exec.yaml instead, so
// this is defense in depth, not the only thing standing between a
// base level session and config mode.
func init() {
	command.Register("configure.terminal", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["config"]
		if err := command.RequireCurrentCommandLevel(ctx, "config", level.Parent); err != nil {
			return err
		}
		ctx.Logger.Debugln("DEBUG: entering configuration mode")
		ctx.Position.Push(command.CommandLevelFrame{
			Name:         "config",
			PromptSuffix: level.PromptSuffix,
			Tree:         level.Tree,
		})
		return nil
	})
}
