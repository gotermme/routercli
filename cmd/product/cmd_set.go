// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// This handler reaches ProductState through
// ctx.DaemonClient.MutateProductState rather than a direct type
// assertion on ctx.State, following cmd_hostname.go's own "hostname"
// handler; see that file's own doc comment for the full reasoning.
func init() {
	command.Register("set.description", func(ctx *command.AppContext, args []string) error {
		// MinArgs and MaxArgs are enforced by command.ValidateArgs
		// before this ever runs, so args[0] is guaranteed to exist
		// here. See var/tree/level_base.yaml's minargs and maxargs
		// directives for "set description".
		_, err := ctx.DaemonClient.MutateProductState(func(productState any) (any, error) {
			state := productState.(*ProductState)
			ctx.Logger.Debugln("DEBUG: setting description to", args[0])
			state.Description = args[0]
			fmt.Println(ctx.Translator.T("set.description.confirm"))
			return nil, nil
		})
		return err
	})
}
