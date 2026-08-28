// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// defaultHostname - This constant defines what "no hostname" resets
// to, matching real Cisco's behavior of "no hostname" reverting to a
// default device name rather than leaving the hostname genuinely
// unset.
const defaultHostname = "router"

// init - This function registers "hostname <name>" for config mode,
// negatable, see var/tree/level_config.yaml, so "no hostname" resets
// to defaultHostname.
func init() {
	command.Register("hostname", func(ctx *command.AppContext, args []string) error {
		state := ctx.State.(*ProductState)

		if ctx.Negated {
			state.Hostname = ""
			ctx.Logger.Debugln("DEBUG: hostname reset to default")
			fmt.Println(ctx.Translator.T("hostname.reset", defaultHostname))
			return nil
		}

		// MinArgs/MaxArgs (both 1, see var/tree/level_config.yaml) guarantee
		// args[0] exists here on the non-negated path.
		state.Hostname = args[0]
		ctx.Logger.Debugln("DEBUG: hostname set to", args[0])
		fmt.Println(ctx.Translator.T("hostname.confirm", args[0]))
		return nil
	})
}
