// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"github.com/gotermme/routercli/command"
)

// init - This function registers "exit" and "end", present in every
// mode through var/tree/level_common.yaml rather than being
// duplicated into each mode's own tree file. This matches real Cisco
// IOS. "exit" goes up exactly one level, and only quits the whole
// program if already at the root, exec, mode. "end" jumps straight
// back to exec from anywhere, regardless of nesting depth.
//
// "exit" signals that the program should quit by returning
// command.ErrQuit, which main.go's runLoop checks for after every
// RunFunc call. runLoop itself has no idea a command named "exit" exists. It
// only knows what to do when a handler returns this specific
// sentinel, which is what lets "exit" behave differently depending on
// which mode it is run from.
func init() {
	command.Register("exit", func(ctx *command.AppContext, args []string) error {
		if ctx.Position.AtRoot() {
			return command.ErrQuit
		}
		ctx.Position.Pop()
		return nil
	})

	command.Register("end", func(ctx *command.AppContext, args []string) error {
		ctx.Position.PopToRoot()
		return nil
	})
}
