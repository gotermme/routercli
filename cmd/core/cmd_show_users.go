// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"

	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"
)

// init registers "show users", the last of the daemon aware commands
// this project's own claude/DAEMON_ARCHITECTURE_DESIGN.md describes:
// every currently attached session, real daemon required, see
// var/tree/level_exec.yaml's own "requires: daemon" on this command
// and main.go's own featureFlags, which prunes this command out of
// the tree entirely for a deployment with no daemon configured, so
// ctx.DaemonClient.ListUsers reporting command.ErrDaemonNotConfigured
// here is defense in depth, not a path an ordinary deployment ever
// actually reaches.
func init() {
	command.Register("show.users", func(ctx *command.AppContext, args []string) error {
		sessions, err := ctx.DaemonClient.ListUsers()
		if err != nil {
			return fmt.Errorf("%s", ctx.Translator.T("show.users.failed", err))
		}
		for _, line := range usersLines(ctx, sessions) {
			fmt.Println(line)
		}
		return nil
	})
}

// usersLines builds "show users"' own output, one header row naming
// every column, then one row per currently attached session, ordered
// exactly as ctx.DaemonClient.ListUsers already returned them, oldest
// connection first, see daemon.SessionDirectory.List's own doc
// comment. Every column is padded to the longest value it actually
// holds, this call included, rather than a fixed width, the same
// "measure first, then pad" approach command.HelpText already uses
// for its own description column; unlike that listing's own
// translated line templates, these five column headers are left as
// plain, untranslated English, matching real Cisco and HP convention,
// where a "show users" style table's own column headers are part of
// the command's fixed output shape, not prose meant to read naturally
// in a deployment's own configured language.
func usersLines(ctx *command.AppContext, sessions []command.SessionInfo) []string {
	if len(sessions) == 0 {
		return []string{ctx.Translator.T("show.users.empty")}
	}

	const (
		sessionHeader  = "Session"
		usernameHeader = "Username"
		levelHeader    = "Level"
		connectedWidth = len("2006-01-02 15:04:05")
	)
	widthSession, widthUsername, widthLevel := len(sessionHeader), len(usernameHeader), len(levelHeader)
	for _, s := range sessions {
		if len(s.ID) > widthSession {
			widthSession = len(s.ID)
		}
		if len(s.Username) > widthUsername {
			widthUsername = len(s.Username)
		}
		if len(s.CommandLevel) > widthLevel {
			widthLevel = len(s.CommandLevel)
		}
	}

	lines := make([]string, 0, len(sessions)+1)
	lines = append(lines, fmt.Sprintf("%-*s  %-*s  %-*s  %-*s  %s",
		widthSession, sessionHeader, widthUsername, usernameHeader, widthLevel, levelHeader, connectedWidth, "Connected", "Idle"))
	for _, s := range sessions {
		lines = append(lines, fmt.Sprintf("%-*s  %-*s  %-*s  %-*s  %s",
			widthSession, s.ID, widthUsername, s.Username, widthLevel, s.CommandLevel,
			connectedWidth, s.ConnectedAt.Format("2006-01-02 15:04:05"), auth.RoundForDisplay(s.IdleFor)))
	}
	return lines
}
