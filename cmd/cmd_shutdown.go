// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// init - This function registers "shutdown" for config-if mode,
// negatable, see var/tree/level_config_if.yaml's "negatable: true",
// so "no shutdown" runs this same handler with ctx.Negated set,
// rather than being a separate registration. command.Resolve
// understands "no" as a modifier on an existing command, so one
// registration covers both directions.
func init() {
	command.Register("interface.shutdown", func(ctx *command.AppContext, args []string) error {
		state := ctx.State.(*ExampleState)
		ifaceName := ctx.Position.Current().Context.(string)
		iface := state.Interface(ifaceName)

		if ctx.Negated {
			iface.Shutdown = false
			ctx.Logger.Debugln("DEBUG: interface administratively enabled", ifaceName)
			fmt.Println(ctx.Translator.T("no_shutdown.confirm"))
			return nil
		}

		iface.Shutdown = true
		ctx.Logger.Debugln("DEBUG: interface administratively shut down", ifaceName)
		fmt.Println(ctx.Translator.T("shutdown.confirm"))
		return nil
	})
}
