// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"
	"strings"

	"github.com/gotermme/routercli/command"
)

// init - This function registers "help", present in every mode
// through var/tree/level_common.yaml. Typed with nothing after it,
// the listing itself is built by command.HelpText walking
// ctx.Position.Current().Tree, not ctx.Levels, which is every Command
// Level in the tree regardless of what mode a session is actually in.
// Using the current mode's tree here is what makes "help" show config
// mode commands while in config mode, and config-if commands while in
// that mode, matching how "?" behaves on a real Cisco device. It is
// always scoped to where the session currently is, not the top level.
// Adding a command to any tree*.yaml file makes it appear in "help"
// automatically whenever that mode is current, with no change needed
// here.
//
// Real Cisco and HP have no literal "help <command>" form; the actual
// per-command help comes from typing the command itself followed by
// "?". Typed with a command name after it, "help" here is a second,
// typed entry point to that exact same mechanism, command.HelpForPath,
// rather than a separate implementation: "help alias" prints exactly
// what "alias ?" already prints, useful anywhere the raw "?" keypress
// itself is inconvenient to send, a non-interactive pipe or a copied
// transcript for instance. A name that resolves to nothing at all is
// refused with an error rather than printing nothing silently, so a
// typo is visible as a typo.
func init() {
	command.Register("help", func(ctx *command.AppContext, args []string) error {
		if len(args) == 0 {
			fmt.Print(command.HelpText(ctx.Position.Current().Tree, ctx.Translator, ctx.ListOptions))
			return nil
		}
		text := command.HelpForPath(ctx.Position.Current().Tree, args, ctx.Translator, ctx.ListOptions)
		if text == "" {
			return fmt.Errorf("%s", ctx.Translator.T("help.unknown_command", strings.Join(args, " ")))
		}
		fmt.Print(text)
		return nil
	})
}
