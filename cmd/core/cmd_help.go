// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"
	"os"
	"strings"

	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/paging"
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
// Real Cisco and HP have no literal "help <command>" form of their
// own; the actual per-command help comes from typing the command
// itself followed by "?", command.HelpForPath, which stays exactly as
// it was, answering exactly what it always has, "what can I type
// next." Typed with a command name after it, "help" here answers a
// different question, "what does this command do," through
// command.DetailedHelp instead: "help alias" prints alias's own
// description, its usage line, and, for a command with subcommands,
// "help show" for instance, a listing of them and their own usage,
// one level deep, all of it formatted as a real man page, a mirrored
// header line followed by NAME, SYNOPSIS, DESCRIPTION, and
// SUBCOMMANDS sections as applicable, that header's own centered
// title built from ctx.ProductName, this deployment's own configured
// display name, "RouterCLI" by default. See DetailedHelp's own doc
// comment in command/help.go for the exact format and ordering. A name that
// resolves ambiguously prints the matching candidate names; a name
// that resolves to nothing at all is refused with an error rather than
// printing nothing silently, so a typo is visible as a typo.
//
// width, paging.EffectiveTerminalWidth's own live detection, the same
// call terminalStatusLines in cmd_show.go already makes for "show
// terminal", is resolved fresh on every call rather than cached, so a
// mid session terminal resize is reflected the next time "help" runs,
// and passed straight through to command.DetailedHelp, which wraps
// its own NAME, SYNOPSIS, and DESCRIPTION text to it. "help" itself is
// marked pageable in var/tree/level_common.yaml, so a detail block
// longer than one screen pauses with the same "--More--" prompt every
// other report style command already uses, rather than scrolling
// straight past the top of the terminal.
func init() {
	command.Register("help", func(ctx *command.AppContext, args []string) error {
		if len(args) == 0 {
			fmt.Print(command.HelpText(ctx.Position.Current().Tree, ctx.Translator, ctx.ListOptions))
			return nil
		}
		width := paging.EffectiveTerminalWidth(int(os.Stdin.Fd()), ctx.TerminalWidth, ctx.DefaultTerminalWidth)
		text := command.DetailedHelp(ctx.Position.Current().Tree, args, ctx.Translator, ctx.ListOptions, ctx.ProductName, width)
		if text == "" {
			return fmt.Errorf("%s", ctx.Translator.T("help.unknown_command", strings.Join(args, " ")))
		}
		fmt.Print(text)
		return nil
	})
}
