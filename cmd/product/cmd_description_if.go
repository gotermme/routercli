// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// init - This function registers "description <text>" for config-if
// mode, negatable, see var/tree/level_config_if.yaml, so "no
// description" clears the interface's description instead of setting
// it. This is registered under a different name,
// "description.interface", from the root level "set description",
// "set.description" in cmd_set.go, even though both end up setting a
// field called Description. They are scoped to different state,
// session-wide versus per-interface, and reachable from different
// modes.
//
// ValidateArgs, which normally enforces MinArgs: 1, is skipped
// entirely for negated commands. See command.ValidateArgs's doc
// comment. So args could be empty here on the "no description" path,
// and typically will be, since there is nothing meaningful to clear
// to. "no description anything", extra args on the negated form, is
// accepted and simply ignored rather than treated as an error. Real
// Cisco is similarly lenient about trailing tokens after "no" that do
// not change the outcome.
//
// This handler reaches ProductState through
// ctx.DaemonClient.MutateProductState rather than a direct type
// assertion on ctx.State, following cmd_hostname.go's own "hostname"
// handler; see that file's own doc comment for the full reasoning.
// ifaceName itself, read from ctx.Position, is session-local, not
// shared state, so it is resolved once, outside the closure.
func init() {
	command.Register("description.interface", func(ctx *command.AppContext, args []string) error {
		ifaceName := ctx.Position.Current().Context.(string)

		_, err := ctx.DaemonClient.MutateProductState(func(productState any) (any, error) {
			state := productState.(*ProductState)
			iface := state.Interface(ifaceName)

			if ctx.Negated {
				iface.Description = ""
				ctx.Logger.Debugln("DEBUG: description cleared for interface", ifaceName)
				fmt.Println(ctx.Translator.T("description_if.cleared"))
				return nil, nil
			}

			// MinArgs/MaxArgs (both 1) are enforced by the framework for
			// the non-negated path, so args[0] is guaranteed to exist
			// here.
			iface.Description = args[0]
			ctx.Logger.Debugln("DEBUG: description set for interface", ifaceName)
			fmt.Println(ctx.Translator.T("description_if.confirm"))
			return nil, nil
		})
		return err
	})
}
