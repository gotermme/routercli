// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

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
//
// This handler reaches ProductState through
// ctx.DaemonClient.MutateProductState rather than a direct type
// assertion on ctx.State, following cmd_hostname.go's own "hostname"
// handler; see that file's own doc comment for the full reasoning.
// ifaceName itself, read from ctx.Position, is session-local, not
// shared state, so it is resolved once, outside the closure.
func init() {
	command.Register("interface.shutdown", func(ctx *command.AppContext, args []string) error {
		ifaceName := ctx.Position.Current().Context.(string)

		_, err := ctx.DaemonClient.MutateProductState(func(productState any) (any, error) {
			state := productState.(*ProductState)
			iface := state.Interface(ifaceName)

			if ctx.Negated {
				iface.Shutdown = false
				ctx.Logger.Debugln("DEBUG: interface administratively enabled", ifaceName)
				fmt.Println(ctx.Translator.T("no_shutdown.confirm"))
				return nil, nil
			}

			iface.Shutdown = true
			ctx.Logger.Debugln("DEBUG: interface administratively shut down", ifaceName)
			fmt.Println(ctx.Translator.T("shutdown.confirm"))
			return nil, nil
		})
		return err
	})
}
