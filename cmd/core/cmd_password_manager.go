// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"
	"os"

	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"
)

// init - This function registers "password manager" and "password
// manager hash" for config mode, matching a real HP ProCurve switch's
// own "password manager" directive, plus this project's own
// hash-accepting form of it. Both set the PasswordHash on the
// command.CommandLevel matching ctx.Session.CommandLevel, that is,
// whichever Command Level the session is currently inside. Config
// mode is only ever reachable from within some non-base level, see
// var/tree/level_config.yaml being merged into level_exec.yaml. The
// stored hash is exactly the secret checked the next time someone
// tries to re-enter this same level from its parent, see
// command.EnterCommandLevel.
//
// "password manager", with no argument, always prompts for the
// secret using auth.PromptSecret, the same mechanism every other
// password prompt in this project uses, rather than accepting it as a
// same-line argument. A same-line argument would flow through
// main.go's runLoop exactly like any other typed line, which both the
// audit log and readline's own history file record verbatim, leaving
// the plaintext secret written to disk. Prompting keeps this
// consistent with every other secret in this project, none of which
// ever appear as a same-line argument.
//
// "password manager hash <hash>" is different in kind, not just in
// spelling. It never sees a plaintext password at all; the argument
// it takes is already a hashed value, in this project's own
// "$id$encoded" storage form, see auth.HashPassword. This exists for
// exactly one job: restoring a Command Level's secret from previously
// saved configuration text, "show running-config" and
// "copy running-config startup-config" among them, see
// cmd/product/cmd_show.go, so a saved configuration can be pasted
// back in and reproduce the exact same access it had before, without
// ever needing to know, type, or reveal what the real, live password
// actually was. Recording only what a stored secret equals, never
// treating a presented hash as proof that whoever pasted it actually
// knows the password, is the same distinction
// command.CommandLevel.LastAuthenticatedAt's own doc comment draws
// out at length; see that field, and admin, this project's
// dedicated real, live authenticated Command Level for actually
// replaying a whole saved configuration, for the rest of that
// reasoning. A hash typed this way is never itself treated as a
// credential that grants entry on its own, it only ever sets what
// PasswordHash equals for the next time someone does present a real,
// live password.
//
// Since a project can define any number of Command Levels in
// var/tree/tree_structure.yaml, both of these commands can exist in
// more than one level's own config mode tree, and each occurrence
// sets that level's own secret independently, simply by running while
// ctx.Session.CommandLevel is different each time.
//
// "password manager", negated, as "no password manager", clears the
// secret back to an empty string, restoring the default behavior of
// entering this level without a prompt. "password manager hash" is
// not itself negatable, "no password manager" already covers clearing
// regardless of which form last set it.
//
// Neither form is allowed at all when
// command.CommandLevel.UserSettablePassword reports false, which is
// always the case once the level carries its own
// VendorDefinedPasswordHash. Refusing outright here, rather than
// silently letting the command succeed against a PasswordHash that
// EffectivePasswordHash would then go on to ignore, matters:
// otherwise an end user could type either form, believe they just
// changed this level's access, and never learn that the vendor
// defined secret is still the one actually gating entry.
func init() {
	command.Register("password-manager", func(ctx *command.AppContext, args []string) error {
		level, err := currentUserSettableLevel(ctx)
		if err != nil {
			return err
		}

		if ctx.Negated {
			level.PasswordHash = ""
			ctx.Logger.Debugln("DEBUG: password cleared for Command Level", level.Name)
			fmt.Println(ctx.Translator.T("password_manager.cleared"))
			return nil
		}

		password, err := auth.PromptSecret(os.Stdout, int(os.Stdin.Fd()), ctx.Translator)
		if err != nil {
			return err
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			return err
		}
		level.PasswordHash = hash
		ctx.Logger.Debugln("DEBUG: password set for Command Level", level.Name)
		fmt.Println(ctx.Translator.T("password_manager.confirm"))
		return nil
	})

	command.Register("password-manager.hash", func(ctx *command.AppContext, args []string) error {
		level, err := currentUserSettableLevel(ctx)
		if err != nil {
			return err
		}

		hash := args[0]
		if !auth.IsRecognizedHash(hash) {
			return fmt.Errorf("%s", ctx.Translator.T("password_manager.hash.not_recognized"))
		}

		level.PasswordHash = hash
		ctx.Logger.Debugln("DEBUG: password hash set directly for Command Level", level.Name)
		fmt.Println(ctx.Translator.T("password_manager.confirm"))
		return nil
	})
}

// currentUserSettableLevel - This function is the lookup and
// permission check "password manager" and "password manager hash"
// both start with: resolve ctx.Session.CommandLevel to its real
// *command.CommandLevel, then refuse before doing anything else if
// that level's own UserSettablePassword reports false. Factored out
// once here rather than duplicated in both handlers above, the same
// reasoning cmd_totp.go's own shared helper between "totp enable" and
// "totp enable qr" follows.
func currentUserSettableLevel(ctx *command.AppContext) (*command.CommandLevel, error) {
	level, ok := ctx.Levels.ByName[ctx.Session.CommandLevel]
	if !ok {
		// Session.CommandLevel is always kept in sync with an actual,
		// currently loaded CommandLevel by whichever cmd/cmd_*.go file
		// last called command.EnterCommandLevel or ExitCommandLevel,
		// so this should not happen. Checked rather than assumed.
		return nil, fmt.Errorf("%s", ctx.Translator.T("password_manager.no_current_level"))
	}
	if !level.UserSettablePassword() {
		return nil, fmt.Errorf("%s", ctx.Translator.T("password_manager.not_user_settable"))
	}
	return level, nil
}
