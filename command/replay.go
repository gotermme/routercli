// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"fmt"
	"strings"

	"github.com/gotermme/routercli/tokenize"
)

// ReplayLines runs each of lines through the same tokenize, resolve,
// validate, run sequence a live session already uses for anything
// typed at the prompt, against whatever Command Level
// ctx.Position.Current() currently is, advancing ctx.Position and
// ctx.Session.CommandLevel exactly as a session actually typing the
// same lines would. This is the general purpose piece a caller wanting
// to apply a whole block of saved configuration text builds on;
// main.go's own boot time StartupConfigFile load, see
// AppContext.ReplayingStartupConfig's own doc comment, is its first
// real caller.
//
// An empty line, or a Cisco style "!" comment line, exactly what
// runningConfigLines' own header and trailing separator are, is
// skipped rather than sent through resolution, the same way a real
// terminal treats a blank or comment line as nothing to run. The
// first line that fails to tokenize, fails to resolve to a real,
// runnable command, fails argument validation, or whose RunFunc
// itself returns an error, stops replay immediately and returns that
// error, wrapped with the offending line, rather than skipping it and
// continuing; a caller applying a whole block of configuration this
// way needs to know it did not fully apply, not silently end up in a
// partially applied state.
//
// trusted controls whether entering a password protected Command
// Level along the way is waved through without prompting.
// ReplayLines never prompts for a password itself either way; the
// difference trusted makes is entirely inside EnterCommandLevel, see
// AppContext.ReplayingStartupConfig's own doc comment for the full
// reasoning. When trusted is true, ctx.ReplayingStartupConfig is set
// for the duration of this call and always restored to false again
// before returning, success or error, through a deferred reset, so
// this trust window is never accidentally left open for anything
// called after ReplayLines returns. When trusted is false, lines are
// replayed exactly as an ordinary interactive paste would be, any
// password protected Command Level along the way still needs a real,
// live password typed at the terminal, through EnterCommandLevel's
// own ordinary default case, reading from the real os.Stdin exactly
// as it does for anything else.
//
// ReplayLines deliberately does not enforce any individual command's
// own PasswordHash gate, the one runLoop applies per command in
// main.go, regardless of trusted. This is a deliberate scope
// boundary, not an oversight: ReplayLines exists to replay
// configuration text, runningConfigLines' own output, which by
// construction only ever contains Command Level navigation and state
// setting lines, hostname, "password manager hash", and per-interface
// configuration among them, never a line naming a command that itself
// carries its own, separate PasswordHash. A caller feeding this
// arbitrary interactive command lines rather than genuine
// configuration text is outside what this function was built for.
func ReplayLines(ctx *AppContext, lines []string, trusted bool) error {
	if trusted {
		ctx.ReplayingStartupConfig = true
		defer func() { ctx.ReplayingStartupConfig = false }()
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}

		tokens, terr := tokenize.Tokenize(line)
		if terr != nil {
			return fmt.Errorf("replaying %q: %w", line, terr)
		}

		res := Resolve(ctx.Position.Current().Tree, tokens)
		if res.Command == nil || res.Command.RunFunc == nil {
			return fmt.Errorf("replaying %q: did not resolve to a runnable command at Command Level %q", line, ctx.Position.Current().Name)
		}
		if !res.Negated {
			if verr := ValidateArgs(res.Command, res.Args); verr != nil {
				return fmt.Errorf("replaying %q: %w", line, verr)
			}
		}

		ctx.Negated = res.Negated
		runErr := res.Command.RunFunc(ctx, res.Args)
		ctx.Negated = false
		if runErr != nil {
			return fmt.Errorf("replaying %q: %w", line, runErr)
		}
	}
	return nil
}
