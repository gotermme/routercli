// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

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

	command.Register("show.history", func(ctx *command.AppContext, args []string) error {
		lines, err := historyLines(ctx)
		if err != nil {
			return fmt.Errorf("%s", ctx.Translator.T("show.history.read_failed", err))
		}
		if len(lines) == 0 {
			fmt.Println(ctx.Translator.T("show.history.empty"))
			return nil
		}
		for _, line := range lines {
			fmt.Println(line)
		}
		return nil
	})

	command.Register("show.aliases", func(ctx *command.AppContext, args []string) error {
		lines := aliasesLines(ctx)
		if len(lines) == 0 {
			fmt.Println(ctx.Translator.T("show.aliases.empty"))
			return nil
		}
		for _, line := range lines {
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
// paging.EffectiveTerminalWidth the same way, auto-detected from the
// real terminal behind os.Stdin, live on every call, when no "terminal
// width <n>" has been typed this session, falling back to
// ctx.DefaultTerminalWidth, zero unless a persisted "line" mode
// "width <n>" setting has replayed one, only when that live detection
// itself fails. Filter mode is a
// RouterCLI specific addition beyond what a real Cisco or HP device
// reports here, see var/tree/README.md, included since it directly
// affects how "| include", "| exclude", and "| begin" behave for the
// rest of this session.
func terminalStatusLines(ctx *command.AppContext) []string {
	length := paging.EffectivePageLines(int(os.Stdin.Fd()), ctx.PageLines, ctx.DefaultPageLines)
	width := paging.EffectiveTerminalWidth(int(os.Stdin.Fd()), ctx.TerminalWidth, ctx.DefaultTerminalWidth)

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

// historyLines - This function reads ctx.HistoryFile fresh from disk
// and returns its last command.EffectiveHistorySize lines, oldest
// first, the same order a real Cisco or HP "show history" prints in.
// readline's own opHistory.Update appends each submitted line to this
// exact file immediately upon submission, see
// command.AppContext.HistoryFile's own doc comment, so reading it
// here, rather than keeping a second, separate in-memory copy,
// always reflects exactly what this session, and any earlier one
// sharing the same HistoryFile, has actually typed. This is the same
// "read the live file fresh on every call" approach
// cmd/product/cmd_show.go's own "show startup-config" already takes
// for its own file.
//
// A missing file, nothing typed yet against this HistoryFile, or an
// empty one, returns an empty slice, not an error, the same treatment
// "show startup-config" gives its own missing file case. An
// EffectiveHistorySize of zero, "terminal history size 0", also
// returns an empty slice with no file read at all, since there is
// nothing to show either way and no reason to pay for reading a file
// whose content will not be used.
func historyLines(ctx *command.AppContext) ([]string, error) {
	size := command.EffectiveHistorySize(ctx)
	if size <= 0 {
		return nil, nil
	}

	data, err := os.ReadFile(ctx.HistoryFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimRight(string(data), "\n")
	if trimmed == "" {
		return nil, nil
	}

	all := strings.Split(trimmed, "\n")
	if len(all) > size {
		all = all[len(all)-size:]
	}
	return all, nil
}

// aliasesLines - This function builds "show aliases"' output, one
// header line per Command Level that has any runtime defined alias
// at all, see cmd/core/cmd_alias.go and
// command.CommandLevel.Aliases, followed by one indented line per
// alias belonging to that level, alias name then the real command it
// expands to. ctx.Levels.Order, load order from the manifest, decides
// which level's own block comes first, the same order
// cmd/product/cmd_show.go's own configModeLines already walks
// password hashes in; within one level, alias names are sorted
// alphabetically, so this listing, unlike Aliases itself, a plain Go
// map with no ordering guarantee of its own, is stable from one call
// to the next. A level with no aliases defined contributes nothing
// here, not even its own header, the same "nothing configured, nothing
// shown" convention configModeLines already follows for an interface
// nobody has touched.
func aliasesLines(ctx *command.AppContext) []string {
	var lines []string
	if ctx.Levels == nil {
		return lines
	}

	for _, level := range ctx.Levels.Order {
		if len(level.Aliases) == 0 {
			continue
		}

		names := make([]string, 0, len(level.Aliases))
		for name := range level.Aliases {
			names = append(names, name)
		}
		sort.Strings(names)

		lines = append(lines, ctx.Translator.T("show.aliases.level_header", level.Name))
		for _, name := range names {
			lines = append(lines, ctx.Translator.T("show.aliases.line", name, strings.Join(level.Aliases[name], " ")))
		}
	}

	return lines
}
