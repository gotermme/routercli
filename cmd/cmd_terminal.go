// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"fmt"
	"strconv"

	"github.com/gotermme/routercli/command"
)

// terminalLengthMin - This constant, together with terminalLengthMax,
// terminalWidthMin, and terminalWidthMax, matches the <2-1000> range
// shown in the ArgHelp hint itself, var/lang/en.yaml's
// terminal.length.arghelp and terminal.width.arghelp. They are kept
// as named constants so the validation logic and the hint text cannot
// silently drift apart the way two independently hand-typed numbers
// eventually do.
const (
	terminalLengthMin = 2
	terminalLengthMax = 1000
	terminalWidthMin  = 2
	terminalWidthMax  = 1000
)

// init - This function registers "terminal length <n>" and "terminal
// width <n>" for config mode. These exist to give the ArgHelp tab
// completion hint, see command.Command.ArgHelp and completer.OnChange,
// a real command to demonstrate against. Typing "terminal length " and
// pressing Tab shows "<2-1000>  Enter a number for the 'length'
// command/parameter."
//
// Framework level argument validation, MinArgs, MaxArgs, and
// MaxArgLength, see command.ValidateArgs, only checks argument count
// and length, not that an argument parses as a number in a given
// range. That check is this handler's own job, the same as any other
// domain-specific validation a real command needs beyond the generic
// framework checks.
//
// Neither of these commands actually controls readline's own
// pagination, since this is an example CLI framework, not a real
// terminal driver. They are purely a stored, reportable value, the
// same spirit as Hostname and Description elsewhere in ExampleState.
func init() {
	command.Register("terminal.length", func(ctx *command.AppContext, args []string) error {
		return setTerminalGeometry(ctx, args[0], "length", terminalLengthMin, terminalLengthMax,
			func(state *ExampleState, n int) { state.TerminalLength = n })
	})
	command.Register("terminal.width", func(ctx *command.AppContext, args []string) error {
		return setTerminalGeometry(ctx, args[0], "width", terminalWidthMin, terminalWidthMax,
			func(state *ExampleState, n int) { state.TerminalWidth = n })
	})
}

// setTerminalGeometry - This function is shared by both handlers
// above. "length" and "width" differ only in which state field they
// set and which range they enforce, so the parse, validate, and
// report logic lives here once instead of being duplicated twice
// with two chances to drift.
func setTerminalGeometry(ctx *command.AppContext, raw, which string, min, max int, set func(*ExampleState, int)) error {
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fmt.Errorf("%s", ctx.Translator.T("terminal.not_a_number", raw))
	}
	if n < min || n > max {
		return fmt.Errorf("%s", ctx.Translator.T("terminal.out_of_range", raw, min, max))
	}

	state := ctx.State.(*ExampleState)
	set(state, n)
	ctx.Logger.Debugln("DEBUG: terminal", which, "set to", n)
	fmt.Println(ctx.Translator.T("terminal.confirm", which, n))
	return nil
}
