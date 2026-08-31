// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"

	"github.com/gotermme/routercli/command"
)

// init - This function registers "su-config" and "exit-su-config",
// the su-config Command Level's own entry and exit commands. See
// var/tree/tree_structure.yaml's "su-config" entry, and
// var/tree/README.md's own su-config section, for what this level is
// for. This is the same shape as cmd_enable.go, entirely ordinary,
// nothing special cased in package command about "su-config" as a
// name; see that file's own doc comment for why. What actually makes
// su-config different from an ordinary level is two properties set on
// it in the manifest, not this file: GrantsReplayTrust and
// RevealVendorDefinedSecrets, see command.CommandLevel's own doc
// comments for both, and command.EnterCommandLevel and
// withinSuConfigTrust in command/treestructure.go for how the first
// one is actually used.
//
// su-config exists to solve one specific, narrow problem this
// project's design conversation spent a long time on: replaying a
// whole saved configuration, "show running-config"'s own output for
// instance, back into a fresh session without stopping at a fresh
// password prompt every time the pasted text moves into another
// gated level, while never treating anything inside that pasted text
// itself as proof of a real credential. A hash recorded in
// configuration text, "password manager hash <hash>" among them, only
// ever sets what a stored secret equals; see cmd_password_manager.go.
// It can never grant entry on its own, a real "pass the hash"
// vulnerability this project's design deliberately rejected. su-
// config is the one place a real, live, freshly typed credential is
// actually required and actually checked, exactly once, by this very
// command running through command.EnterCommandLevel below, before any
// of that broader trust is extended to anything else.
//
// Whether su-config actually grants any of that trust at all depends
// entirely on whether it has a real password of its own configured.
// See var/tree/tree_structure.yaml's own su-config entry: it ships
// with neither password_hash nor vendor_defined_password_hash set, so
// entering it never actually checks a credential, see
// command.EnterCommandLevel's own "no password at all, nothing to
// check" case, which also means LastAuthenticatedAt is never set,
// which in turn means withinSuConfigTrust never finds anything to
// trust. A project shipping su-config wide open like this keeps it as
// a manual place to view and manage configuration only, never a
// shortcut past any other level's own password, exactly the
// deliberately unforced default var/tree/README.md documents: best
// practice for an implementer or an end user is to set a real
// password here first thing, but this framework does not require it.
//
// RouterCLI does automatically replay a saved startup-config file
// back in when the process restarts, main.go's own loadStartupConfig,
// called well before establishSession or the interactive loop below
// it ever run. That replay is not su-config's own doing, and needs no
// password of its own, real or vendor defined, since a fresh process
// has nobody sitting at a terminal yet to type one: the trust there
// comes from the process itself already having been allowed to run,
// and to read StartupConfigFile, by the operating system, before any
// Session even exists. See command.AppContext.ReplayingStartupConfig's
// own doc comment in command/model.go, and loadStartupConfig's own
// doc comment in main.go, for the full reasoning, and
// etc/README.md's StartupConfigFile section for how a deployment
// points this at its own saved file.
func init() {
	command.Register("su-config", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["su-config"]
		entered, err := command.EnterCommandLevel(ctx, level, ctx.Levels.ByName[level.Parent])
		if err != nil {
			return err
		}
		if !entered {
			fmt.Println(ctx.Translator.T("su_config.already_here"))
			return nil
		}
		fmt.Println(ctx.Translator.T("su_config.entered"))
		return nil
	})

	command.Register("exit-su-config", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["su-config"]
		exited, err := command.ExitCommandLevel(ctx, level, ctx.Levels.ByName[level.Parent])
		if err != nil {
			return err
		}
		if !exited {
			fmt.Println(ctx.Translator.T("exit_su_config.not_here"))
			return nil
		}
		fmt.Println(ctx.Translator.T("exit_su_config.left"))
		return nil
	})
}
