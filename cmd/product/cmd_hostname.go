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
//
// This is the first handler in this project migrated to
// ctx.DaemonClient rather than reading and writing ctx.State
// directly, the "hostname first as the smallest useful proof" phase
// four names in claude/DAEMON_ARCHITECTURE_DESIGN.md's own suggested
// implementation order. The mutation itself, and every message this
// handler prints, are unchanged from before that migration; only
// where the mutation runs changed, inside the closure
// MutateProductState hands to whatever currently owns canonical
// state, a private, in process Store today, a real daemon over a
// socket once one exists, with this handler never needing to know or
// care which.
func init() {
	command.Register("hostname", func(ctx *command.AppContext, args []string) error {
		_, err := ctx.DaemonClient.MutateProductState(func(productState any) (any, error) {
			state := productState.(*ProductState)

			if ctx.Negated {
				state.Hostname = ""
				ctx.Logger.Debugln("DEBUG: hostname reset to default")
				fmt.Println(ctx.Translator.T("hostname.reset", defaultHostname))
				return nil, nil
			}

			// MinArgs/MaxArgs (both 1, see var/tree/level_config.yaml)
			// guarantee args[0] exists here on the non-negated path.
			state.Hostname = args[0]
			ctx.Logger.Debugln("DEBUG: hostname set to", args[0])
			fmt.Println(ctx.Translator.T("hostname.confirm", args[0]))
			return nil, nil
		})
		return err
	})
}
