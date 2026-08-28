// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"
	"os"

	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/paging"
)

func init() {
	command.Register("show.version", func(ctx *command.AppContext, args []string) error {
		fmt.Println(ctx.Translator.T("show.version.text"))
		return nil
	})

	command.Register("show.terminal", func(ctx *command.AppContext, args []string) error {
		for _, line := range terminalStatusLines(ctx) {
			fmt.Println(line)
		}
		return nil
	})
}

// terminalStatusLines - This function builds "show terminal"'s
// output: Length and Width on one line, then whether the interactive
// pager is currently active, both for this one session and for the
// deployment as a whole, matching real HP's own "(session)"/"(global)"
// split exactly, since this project already keeps that same two tier
// distinction, see command.AppContext.PageLines and PagingEnabled.
// Length reflects paging.EffectivePageLines's own live, resolved
// value, auto-detected from the real terminal behind os.Stdin when no
// "terminal length" has been typed yet this session, not merely
// whatever override, if any, is currently set. Width reflects
// command.AppContext.TerminalWidth directly, zero until "terminal
// width <n>" has been typed this session, since this package defines
// no auto-detection fallback for width the way it does for length.
// Filter mode is a RouterCLI specific addition beyond what a real
// Cisco or HP device reports here, see var/tree/README.md, included
// since it directly affects how "| include", "| exclude", and "|
// begin" behave for the rest of this session.
func terminalStatusLines(ctx *command.AppContext) []string {
	length := paging.EffectivePageLines(int(os.Stdin.Fd()), ctx.PageLines, ctx.DefaultPageLines)

	var width int
	if ctx.TerminalWidth != nil {
		width = *ctx.TerminalWidth
	}

	sessionPaging := ctx.Translator.T("show.terminal.enabled")
	if ctx.PageLines != nil && *ctx.PageLines == 0 {
		sessionPaging = ctx.Translator.T("show.terminal.disabled")
	}
	globalPaging := ctx.Translator.T("show.terminal.disabled")
	if ctx.PagingEnabled {
		globalPaging = ctx.Translator.T("show.terminal.enabled")
	}

	filterMode := ctx.Translator.T("show.terminal.filter_mode_substring")
	if ctx.FilterMode == paging.FilterModeRegex {
		filterMode = ctx.Translator.T("show.terminal.filter_mode_regex")
	}

	return []string{
		ctx.Translator.T("show.terminal.geometry_line", length, width),
		ctx.Translator.T("show.terminal.paging_line", sessionPaging, globalPaging),
		ctx.Translator.T("show.terminal.filter_mode_line", filterMode),
	}
}
