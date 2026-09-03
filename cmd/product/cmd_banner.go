// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// init - This function registers "banner motd <text>" and "banner
// login <text>" for config mode, both negatable, see
// var/tree/level_config.yaml, so "no banner motd" and "no banner
// login" clear whichever one is named rather than setting it.
//
// Real Cisco and HP both print a MOTD banner to every connection
// before authentication even begins, "message of the day" being
// about the connection itself, not about who is about to log in, then
// a second, separate login banner immediately before the actual
// username prompt, only when a login prompt is actually about to run
// at all. This project follows the same two banner, two moment
// design: BannerMOTD is shown unconditionally at the start of every
// session, BannerLogin only immediately before establishSession's own
// login prompt, see main.go's printBanner and its two call sites for
// exactly where each fires. Neither banner is shown again once a
// session is already past that point; there is no "banner exec"
// equivalent here.
//
// Both handlers below reach ProductState through
// ctx.DaemonClient.MutateProductState rather than a direct type
// assertion on ctx.State, following cmd_hostname.go's own "hostname"
// handler, the first in this project migrated this way; see that
// file's own doc comment for the full reasoning.
func init() {
	command.Register("banner.motd", func(ctx *command.AppContext, args []string) error {
		_, err := ctx.DaemonClient.MutateProductState(func(productState any) (any, error) {
			state := productState.(*ProductState)

			if ctx.Negated {
				state.BannerMOTD = ""
				ctx.Logger.Debugln("DEBUG: banner motd cleared")
				fmt.Println(ctx.Translator.T("banner.motd.cleared"))
				return nil, nil
			}

			// MinArgs/MaxArgs (both 1, see var/tree/level_config.yaml)
			// guarantee args[0] exists here on the non-negated path. A
			// banner containing spaces or newlines is typed as one quoted
			// token, the same convention "hostname" and "set description"
			// already establish for free text arguments, see
			// tokenize.Tokenize's own doc comment.
			state.BannerMOTD = args[0]
			ctx.Logger.Debugln("DEBUG: banner motd set")
			fmt.Println(ctx.Translator.T("banner.motd.confirm"))
			return nil, nil
		})
		return err
	})

	command.Register("banner.login", func(ctx *command.AppContext, args []string) error {
		_, err := ctx.DaemonClient.MutateProductState(func(productState any) (any, error) {
			state := productState.(*ProductState)

			if ctx.Negated {
				state.BannerLogin = ""
				ctx.Logger.Debugln("DEBUG: banner login cleared")
				fmt.Println(ctx.Translator.T("banner.login.cleared"))
				return nil, nil
			}

			state.BannerLogin = args[0]
			ctx.Logger.Debugln("DEBUG: banner login set")
			fmt.Println(ctx.Translator.T("banner.login.confirm"))
			return nil, nil
		})
		return err
	})
}
