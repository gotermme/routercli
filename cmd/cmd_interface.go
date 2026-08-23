// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"github.com/gotermme/routercli/command"
)

// init - This function registers "interface <name>", a config mode
// command that pushes the config-if sub-mode, the second level of
// nesting on top of config mode. Context on the pushed frame is the
// interface name itself, args[0]. cmd_description_if.go and
// cmd_shutdown.go read it back through ctx.Position.Current().Context
// to know which interface they are editing, without this file or
// command.CommandLevelStack needing to know anything about what an
// interface even is. That meaning lives entirely in this package, per
// CommandLevelFrame.Context's doc comment in
// command/commandlevelstack.go. The tree and prompt suffix come from
// ctx.Levels.ByName["config-if"], a CommandLevel loaded from
// var/tree/tree_structure.yaml at startup, rather than hardcoded here.
// See cmd_configure.go's doc comment for why "config-if" itself
// staying a literal in this file is fine while main.go no longer knows
// that name at all.
//
// command.RequireCurrentCommandLevel(ctx, "config-if", level.Parent)
// enforces config-if's manifest entry, parent: config, the same way
// cmd_configure.go enforces config's own, parent: exec. This command
// only ever appears inside config's own tree in the first place, see
// level_config.yaml, so this is defense in depth, not the only thing
// preventing "interface eth0" from working outside config mode.
func init() {
	command.Register("interface", func(ctx *command.AppContext, args []string) error {
		// MinArgs/MaxArgs (both 1, see var/tree/level_config.yaml)
		// guarantee args[0] exists here.
		name := args[0]
		level := ctx.Levels.ByName["config-if"]
		if err := command.RequireCurrentCommandLevel(ctx, "config-if", level.Parent); err != nil {
			return err
		}
		ctx.Logger.Debugln("DEBUG: entering interface configuration for", name)
		ctx.Position.Push(command.CommandLevelFrame{
			Name:         "config-if",
			PromptSuffix: level.PromptSuffix,
			Tree:         level.Tree,
			Context:      name,
		})
		return nil
	})
}
