// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// historySizeMin and historySizeMax bound "terminal history size
// <n>", checked by parseTerminalGeometry in cmd_terminal.go, the same
// shared helper "terminal length" and "terminal width" already use.
// Real Cisco IOS accepts <0-256> for the same command, since its own
// history is a small, ephemeral ring buffer kept only in memory for
// the current session. RouterCLI's own HistoryFile is instead a
// genuine, persistent, cross-session log, appended to on disk as each
// line is submitted, see command.AppContext.HistoryFile's own doc
// comment, so a noticeably wider range, comfortably containing
// config.SystemConfig.DefaultHistorySize's own 500 line default, is a
// deliberate departure from Cisco's literal range rather than an
// oversight.
const (
	historySizeMin = 0
	historySizeMax = 1000
)

// init registers "terminal history size <n>" for config mode.
//
// This sets command.AppContext.HistorySize directly, the same session
// scoped, never persisted treatment "terminal length" and "terminal
// width" already give PageLines and TerminalWidth in cmd_terminal.go.
// It governs how many of the most recent lines "show history" prints
// back, through command.EffectiveHistorySize, see historyLines in
// cmd_show.go. Setting it to zero, "terminal history size 0", empties
// what "show history" reports, matching "terminal length 0"'s own
// "zero means none" convention, though the on-disk HistoryFile
// itself, and every line already written to it before this was typed,
// is untouched either way; see HistoryFile's own doc comment for why
// that file is never truncated by anything routercli itself does.
//
// This command deliberately has no live effect on this session's own
// Up and Down arrow recall. That limit, readline.Config.HistoryLimit,
// is fixed at whatever config.SystemConfig.DefaultHistorySize was
// when this session's own readline.Instance was constructed in
// main.go, and is never reassigned afterward: the underlying
// github.com/chzyer/readline library reads that same field from an
// unsynchronized background goroutine for the entire life of the
// Instance, so mutating it here, discovered during this project's own
// "go test -race" pass, would be a genuine data race, not merely a
// theoretical one. See command.AppContext.HistorySize's own doc
// comment for the fuller reasoning.
func init() {
	command.Register("terminal.history.size", func(ctx *command.AppContext, args []string) error {
		n, err := parseTerminalGeometry(ctx, args[0], historySizeMin, historySizeMax)
		if err != nil {
			return err
		}
		ctx.HistorySize = &n
		ctx.Logger.Debugln("DEBUG: terminal history size set to", n)
		fmt.Println(ctx.Translator.T("terminal.confirm", "history size", n))
		return nil
	})
}
