// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package session

import (
	"fmt"
	"strconv"

	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/paging"
)

// terminalLengthMin - This constant, together with terminalLengthMax,
// terminalWidthMin, and terminalWidthMax, matches the <0-512> range
// shown in the ArgHelp hint itself, var/lang/en.yaml's
// terminal.length.arghelp and terminal.width.arghelp, and the real
// range Cisco IOS itself accepts for both commands. They are kept as
// named constants so the validation logic and the hint text cannot
// silently drift apart the way two independently hand-typed numbers
// eventually do. Zero is not merely the bottom of this range; for
// "terminal length" specifically it carries the real, well known
// meaning "never pause", see command.AppContext.PageLines's own doc
// comment and paging.Display.
const (
	terminalLengthMin = 0
	terminalLengthMax = 512
	terminalWidthMin  = 0
	terminalWidthMax  = 512
)

// init - This function registers "terminal length <n>", "terminal
// width <n>", and "terminal filter-mode <substring|regex>" for config
// mode.
//
// "terminal length" sets command.AppContext.PageLines directly, the
// real, live page size package paging's pager honors for the rest of
// this session, in place of auto-detecting the real terminal's own
// height, see paging.EffectivePageLines. This is genuine, framework
// level behavior, not a cosmetic, reported-only value: setting it to
// zero disables the pager entirely for this session, matching real
// Cisco's own well known "terminal length 0".
//
// "terminal width" sets command.AppContext.TerminalWidth directly, the
// same session scoped, never persisted treatment as "terminal length".
// Real Cisco and HP both treat both settings this way, never writing
// either one to running-config or startup-config, see "show terminal",
// cmd_show.go in this package, for the one place a session actually
// reports the current width back. Nothing in package command or this
// package paces output by column width today; TerminalWidth exists so
// an implementation that formats output to a fixed width has one place
// to find the session's own override instead of inventing its own
// field for the same idea.
//
// "terminal filter-mode" sets command.AppContext.FilterMode, changing
// how every "| include", "| exclude", and "| begin" pattern is
// matched for the rest of this session, see package paging's own
// FilterMode.
//
// Framework level argument validation, MinArgs, MaxArgs, and
// MaxArgLength, see command.ValidateArgs, only checks argument count
// and length, not that an argument parses as a number in a given
// range, or is one of a fixed set of words. That check is each of
// these handlers' own job, the same as any other domain-specific
// validation a real command needs beyond the generic framework
// checks.
func init() {
	command.Register("terminal.length", func(ctx *command.AppContext, args []string) error {
		n, err := parseTerminalGeometry(ctx, args[0], terminalLengthMin, terminalLengthMax)
		if err != nil {
			return err
		}
		ctx.PageLines = &n
		ctx.Logger.Debugln("DEBUG: terminal length set to", n)
		fmt.Println(ctx.Translator.T("terminal.confirm", "length", n))
		return nil
	})

	command.Register("terminal.width", func(ctx *command.AppContext, args []string) error {
		n, err := parseTerminalGeometry(ctx, args[0], terminalWidthMin, terminalWidthMax)
		if err != nil {
			return err
		}
		ctx.TerminalWidth = &n
		ctx.Logger.Debugln("DEBUG: terminal width set to", n)
		fmt.Println(ctx.Translator.T("terminal.confirm", "width", n))
		return nil
	})

	command.Register("terminal.filter-mode", func(ctx *command.AppContext, args []string) error {
		switch args[0] {
		case "substring":
			ctx.FilterMode = paging.FilterModeSubstring
		case "regex":
			ctx.FilterMode = paging.FilterModeRegex
		default:
			return fmt.Errorf("%s", ctx.Translator.T("terminal.filter_mode.invalid", args[0]))
		}
		ctx.Logger.Debugln("DEBUG: terminal filter-mode set to", args[0])
		fmt.Println(ctx.Translator.T("terminal.filter_mode.confirm", args[0]))
		return nil
	})
}

// parseTerminalGeometry - This function is shared by "terminal
// length" and "terminal width", the parse and range check logic
// every numeric terminal geometry setting needs, so it lives here
// once instead of being duplicated twice with two chances to drift.
// Each handler still owns its own assignment and confirmation
// message, since one sets command.AppContext.PageLines and the other
// sets command.AppContext.TerminalWidth, two distinct fields with
// nothing left in common to share past this point.
func parseTerminalGeometry(ctx *command.AppContext, raw string, min, max int) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s", ctx.Translator.T("terminal.not_a_number", raw))
	}
	if n < min || n > max {
		return 0, fmt.Errorf("%s", ctx.Translator.T("terminal.out_of_range", raw, min, max))
	}
	return n, nil
}
