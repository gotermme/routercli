// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// init - This function registers "help", present in every mode
// through var/tree/level_common.yaml. The listing itself is built by
// command.HelpText walking ctx.Position.Current().Tree, not ctx.Levels,
// which is every Command Level in the tree regardless of what mode a
// session is actually in. Using the current mode's tree here is what
// makes "help" show config mode commands while in config mode, and
// config-if commands while in that mode, matching how "?" behaves on
// a real Cisco device. It is always scoped to where the session
// currently is, not the top level. Adding a command to any
// tree*.yaml file makes it appear in "help" automatically whenever
// that mode is current, with no change needed here.
func init() {
	command.Register("help", func(ctx *command.AppContext, args []string) error {
		fmt.Print(command.HelpText(ctx.Position.Current().Tree, ctx.Translator, ctx.ListOptions))
		return nil
	})
}
