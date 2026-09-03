// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"fmt"
	"strconv"

	"github.com/gotermme/routercli/command"
)

// lineLengthMin, lineLengthMax, lineWidthMin, and lineWidthMax match
// the same <0-512> range cmd/core/cmd_terminal.go's own
// terminalLengthMin, terminalLengthMax, terminalWidthMin, and
// terminalWidthMax already enforce for the strictly session scoped
// "terminal length" and "terminal width" commands. These are kept as
// this package's own separate constants, rather than imported from
// cmd/core, since cmd/core and cmd/product are deliberately
// independent siblings, neither one importing the other, both reached
// only through main.go; duplicating four small integers here costs
// far less than inverting that boundary would.
const (
	lineLengthMin = 0
	lineLengthMax = 512
	lineWidthMin  = 0
	lineWidthMax  = 512
)

// init - This function registers "line", a config mode command that
// pushes config-line mode, and, inside it, "length <n>", "width <n>",
// and "paging", negatable as "no paging". This is item 11 of the
// Framework Gap Roadmap: a deployment wide default for page height,
// terminal width, and whether the interactive pager runs at all,
// persisted through running-config and startup-config, exactly the
// role real Cisco and HP's own "line vty" and "line console" modes
// play for the settings they configure. This lives in cmd/product,
// not cmd/core, unlike the "alias" command item 4 adds, since a
// persisted deployment default is Cisco and HP flavored product
// state, the same reasoning hostname and banner already follow, not a
// framework capability every project built on this library needs for
// free.
//
// This is deliberately a single, global set of defaults today, with
// no separate vty and console split the way a real device keeps.
// RouterCLI has no listener of its own yet, one process per
// connection, invoked however a deployment's own wrapper chooses, so
// there is no structural way for this process to know whether it was
// reached over a network connection or a local console the way a real
// device's own dedicated UART driver versus its Telnet or SSH
// listener always does. Splitting this into "line vty" and "line
// console" is left for whichever future phase gives RouterCLI a real
// listener of its own, item 10 of the Framework Gap Roadmap.
//
// "line length" and "line width" set state.Line.Length and
// state.Line.Width, then immediately apply the same value to
// ctx.DefaultPageLines and ctx.DefaultTerminalWidth, the fields
// paging.EffectivePageLines and paging.EffectiveTerminalWidth
// actually fall back to, see command.AppContext's own doc comments on
// both fields, so a change here takes effect right away for this
// process, not only after a future restart replays it back in. "line
// paging" and "no line paging" work the same way for
// state.Line.Paging and ctx.PagingEnabled.
//
// state.Line.Length, state.Line.Width, and state.Line.Paging, being
// ProductState, are reached through
// ctx.DaemonClient.MutateProductState rather than a direct type
// assertion on ctx.State, following cmd_hostname.go's own "hostname"
// handler; see that file's own doc comment for the full reasoning.
// ctx.DefaultPageLines, ctx.DefaultTerminalWidth, and
// ctx.PagingEnabled are session-local AppContext fields, not shared
// state, so they stay direct assignments, applied right after the
// mutation succeeds.
func init() {
	command.Register("line", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["config-line"]
		if err := command.RequireCurrentCommandLevel(ctx, "config-line", level.Parent); err != nil {
			return err
		}
		ctx.Logger.Debugln("DEBUG: entering line configuration")
		ctx.Position.Push(command.CommandLevelFrame{
			Name:         "config-line",
			PromptSuffix: level.PromptSuffix,
			Tree:         level.Tree,
		})
		return nil
	})

	command.Register("line.length", func(ctx *command.AppContext, args []string) error {
		n, err := parseLineGeometry(ctx, args[0], lineLengthMin, lineLengthMax)
		if err != nil {
			return err
		}
		if _, err := ctx.DaemonClient.MutateProductState(func(productState any) (any, error) {
			state := productState.(*ProductState)
			state.Line.Length = &n
			return nil, nil
		}); err != nil {
			return err
		}
		ctx.DefaultPageLines = n
		ctx.Logger.Debugln("DEBUG: line length default set to", n)
		fmt.Println(ctx.Translator.T("line.length.confirm", n))
		return nil
	})

	command.Register("line.width", func(ctx *command.AppContext, args []string) error {
		n, err := parseLineGeometry(ctx, args[0], lineWidthMin, lineWidthMax)
		if err != nil {
			return err
		}
		if _, err := ctx.DaemonClient.MutateProductState(func(productState any) (any, error) {
			state := productState.(*ProductState)
			state.Line.Width = &n
			return nil, nil
		}); err != nil {
			return err
		}
		ctx.DefaultTerminalWidth = n
		ctx.Logger.Debugln("DEBUG: line width default set to", n)
		fmt.Println(ctx.Translator.T("line.width.confirm", n))
		return nil
	})

	command.Register("line.paging", func(ctx *command.AppContext, args []string) error {
		enabled := !ctx.Negated
		if _, err := ctx.DaemonClient.MutateProductState(func(productState any) (any, error) {
			state := productState.(*ProductState)
			state.Line.Paging = &enabled
			return nil, nil
		}); err != nil {
			return err
		}
		ctx.PagingEnabled = enabled
		ctx.Logger.Debugln("DEBUG: line paging default set to", enabled)
		if enabled {
			fmt.Println(ctx.Translator.T("line.paging.confirm_enabled"))
		} else {
			fmt.Println(ctx.Translator.T("line.paging.confirm_disabled"))
		}
		return nil
	})
}

// parseLineGeometry - This function is shared by "line length" and
// "line width", the same small parse and range check logic
// cmd/session/cmd_terminal.go's own parseTerminalGeometry already
// performs for "terminal length" and "terminal width", duplicated
// here rather than imported since cmd/product and cmd/session stay
// independent siblings, see this file's own doc comment. The error
// messages reuse cmd/core's own "terminal.not_a_number" and
// "terminal.out_of_range" catalog keys directly: both are already
// written generically, "%q is not a number" and "%q is out of range
// (%d-%d)", with no mention of "terminal" baked into the English text
// itself, so reusing them here is a genuine shared message, not a
// coincidental collision with a framework specific string.
func parseLineGeometry(ctx *command.AppContext, raw string, min, max int) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s", ctx.Translator.T("terminal.not_a_number", raw))
	}
	if n < min || n > max {
		return 0, fmt.Errorf("%s", ctx.Translator.T("terminal.out_of_range", raw, min, max))
	}
	return n, nil
}
