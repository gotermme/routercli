// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gotermme/routercli/command"
)

// show.terminal and show.history, and the terminalStatusLines and
// historyLines functions that built their own output, moved to
// cmd/session/cmd_show.go: both report state genuinely local to one
// connection, never shared canonical state a daemon will one day own.
// See cmd/session/doc.go and claude/DAEMON_ARCHITECTURE_DESIGN.md for
// the full reasoning behind that boundary.
func init() {
	command.Register("show.version", func(ctx *command.AppContext, args []string) error {
		fmt.Println(ctx.Translator.T("show.version.text"))
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
