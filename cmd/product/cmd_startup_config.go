// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"
)

// init - This function registers "write memory" and "erase
// startup-config", both reachable only from inside admin, see
// var/tree/level_admin.yaml. Both operate on ctx.StartupConfigFile,
// config.SystemConfig.StartupConfigFile copied onto AppContext once
// at startup by main.go, see command.AppContext's own doc comment for
// why this package reaches that path through ctx rather than
// importing package config directly.
//
// "write memory" is the one command in this project that writes
// anything to disk at all. It writes writeRunningConfigToStartupConfig's
// own output, the exact same text "show running-config" prints,
// joined with newlines and a trailing one, to StartupConfigFile,
// creating its parent directory if this is the first time anything
// has been saved, then saves every in memory change to ctx.Users
// through auth.SaveUsers, since account create, account delete,
// account roles add, account roles remove, password change, and totp
// enable and disable all deliberately leave ctx.UsersFile untouched
// on disk until this runs, see cmd/core/cmd_admin.go's own top of
// file doc comment and design-goals.md's own core "nothing survives a
// restart without an explicit save" design goal. Real Cisco and HP
// ship "write memory" as a synonym alongside a separate "copy
// running-config startup-config" that only ever covers the first half
// of this. This project ships "write memory" alone, deliberately,
// matching this project's own design goal against building two
// separate commands that would otherwise do almost, but not quite,
// the same thing; see design-goals.md's own General Design Philosophy
// section.
//
// The saved text is deliberately the same single, classic-command-text
// representation Phase 24's own design goal settled on for the whole
// project: no separate structured format, no versioning, just the
// same lines a session could type by hand.
//
// "erase startup-config" removes that file entirely. Erasing a file
// that does not exist already is not treated as an error, matching
// real Cisco and HP, both of which let "erase startup-config" run
// again harmlessly rather than refusing on an already-clean device.
//
// Neither command reveals or redacts anything itself; whatever
// runningConfigLines already decided to print, "<HIDDEN>" placeholders
// included unless the current session is somewhere with
// RevealVendorDefinedSecrets set, is exactly what "write memory"
// writes to disk. A vendor defined secret written into startup-config
// this way can never actually be restored from it later, since
// "password manager hash <HIDDEN>" is refused, not a recognized hash,
// see auth.IsRecognizedHash, and cmd_password_manager.go's own
// UserSettablePassword check would refuse it regardless. That is
// deliberate, not an oversight: a vendor defined secret was never
// meant to survive a save to a file an ordinary end user, even one
// with admin access but no real reason to be looking at
// startup-config's raw text, might read.
//
// RouterCLI replays startup-config back in automatically, once, at
// process startup, before establishSession or the interactive loop
// begins, see command.LoadStartupConfig in command/replay.go and
// cmd/core/cmd_admin.go's own doc comments for the full reasoning
// behind why that is safe with no password prompting at all.
func init() {
	command.Register("write.memory", func(ctx *command.AppContext, args []string) error {
		if err := writeRunningConfigToStartupConfig(ctx); err != nil {
			return err
		}
		if ctx.UsersFile != "" {
			if err := auth.SaveUsers(ctx.UsersFile, ctx.Users); err != nil {
				return fmt.Errorf("%s", ctx.Translator.T("write_memory.failed", err))
			}
			ctx.Logger.Debugln("DEBUG: users.yaml saved to", ctx.UsersFile)
		}
		fmt.Println(ctx.Translator.T("write_memory.confirm"))
		return nil
	})

	command.Register("erase.startup-config", func(ctx *command.AppContext, args []string) error {
		err := os.Remove(ctx.StartupConfigFile)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println(ctx.Translator.T("erase_startup_config.nothing_to_erase"))
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s", ctx.Translator.T("erase_startup_config.failed", err))
		}

		ctx.Logger.Debugln("DEBUG: startup-config erased at", ctx.StartupConfigFile)
		fmt.Println(ctx.Translator.T("erase_startup_config.confirm"))
		return nil
	})
}

// writeRunningConfigToStartupConfig - This function writes
// runningConfigLines' own output, byte for byte the same text "show
// running-config" prints, to ctx.StartupConfigFile, creating its
// parent directory if this is the first time anything has been saved.
// See this file's own init doc comment for the full reasoning behind
// what gets written and why. runningConfigLines already prepends
// "enable" itself, through execEnterWords, whenever it has any
// exec-rooted content to follow, so what this function writes is
// already self-contained and replayable starting from base, with
// nothing further needed here.
func writeRunningConfigToStartupConfig(ctx *command.AppContext) error {
	state := ctx.State.(*ProductState)
	text := strings.Join(runningConfigLines(ctx, state), "\n") + "\n"

	if dir := filepath.Dir(ctx.StartupConfigFile); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("%s", ctx.Translator.T("startup_config.save_failed", err))
		}
	}
	if err := os.WriteFile(ctx.StartupConfigFile, []byte(text), 0640); err != nil {
		return fmt.Errorf("%s", ctx.Translator.T("startup_config.save_failed", err))
	}

	ctx.Logger.Debugln("DEBUG: running-config saved to startup-config at", ctx.StartupConfigFile)
	return nil
}
