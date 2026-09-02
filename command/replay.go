// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/gotermme/routercli/paging"
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

// LoadStartupConfig reads path, a saved startup-config file, and, if
// it exists, replays its own text back into ctx.State the same way a
// live session pasting that exact text back in would, through
// ReplayLines above, with trusted always true: nothing has typed a
// password yet, no real credential check is being bypassed, only
// waived, the same reasoning AppContext.ReplayingStartupConfig's own
// doc comment gives in full. A path that does not exist yet is not an
// error; there is simply nothing saved to load.
//
// This is called from two places: main.go, once, at process startup,
// before establishSession or the interactive readline loop ever
// begins, and cmd/core/cmd_admin.go's "reload" command, which calls
// this again, mid-session, as part of re-reading every persistent
// file fresh from disk before ending the connection, see that
// command's own doc comment for why re-validating startup-config this
// way, even though nothing further in a dying session actually reads
// the result, still catches a startup-config a hand edit broke before
// the next real connection would otherwise be the first to discover
// it.
//
// ctx.Position and ctx.Session.CommandLevel are both reset to the
// base level before replay begins, and reset back to base again
// before this function returns, success or failure, through a
// deferred reset. The reset before replay matters just as much as the
// one after: startup-config always starts with "enable" and walks
// down from there, exactly as a session logging in from base would,
// so replay must actually begin at base too, regardless of wherever
// ctx.Position happened to be sitting when this function was called.
// At process startup that is already base, nothing has logged in
// yet, but "reload" in cmd/core/cmd_admin.go calls this function
// again mid-session, from whatever Command Level the session
// happened to be standing in, admin for instance, and replaying
// "enable" against admin's own tree would simply fail to resolve.
// The reset after replay matters for the same reason in reverse;
// replaying "enable" and "configure terminal" style lines necessarily
// walks ctx.Position deep into whatever Command Levels the saved
// configuration touched, and that must never be where a session about
// to log in, or a session about to end right after "reload", is
// actually left sitting. Every mutation this function's own replay
// makes to ctx.State itself, and to any Command Level's own
// PasswordHash through a replayed "password manager hash" line, is
// deliberately left in place; only the navigation state this function
// itself used to get there is undone.
//
// The whole replay runs inside paging.CaptureOutput, so the ordinary
// confirmation text every replayed line's own handler prints never
// reaches a terminal nobody has actually asked to see it on, whether
// that is a fresh process nobody has logged into yet, or an
// interactive session mid-command running "reload". Each captured
// line is logged at Debugln level instead, so verbose troubleshooting
// can still see exactly what was applied.
func LoadStartupConfig(ctx *AppContext, path string) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	base := ctx.Levels.Base()
	resetToBase := func() {
		ctx.Position = NewCommandLevelStack(base.Name, base.PromptSuffix, base.Tree)
		ctx.Session.CommandLevel = base.Name
	}
	resetToBase()
	defer resetToBase()

	lines := strings.Split(string(data), "\n")
	var replayErr error
	captured, cerr := paging.CaptureOutput(func() {
		replayErr = ReplayLines(ctx, lines, true)
	})
	if cerr != nil {
		return cerr
	}
	for _, line := range captured {
		ctx.Logger.Debugln("DEBUG: startup-config replay:", line)
	}
	return replayErr
}
