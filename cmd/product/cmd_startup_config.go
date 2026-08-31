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

	"github.com/gotermme/routercli/command"
)

// init - This function registers "copy running-config startup-config"
// and "erase startup-config", both reachable only from inside
// su-config, see var/tree/level_su_config.yaml. Both operate on the
// same file, ctx.StartupConfigFile, config.SystemConfig.StartupConfigFile
// copied onto AppContext once at startup by main.go, see
// command.AppContext's own doc comment for why this package reaches
// that path through ctx rather than importing package config
// directly.
//
// "copy running-config startup-config" writes runningConfigLines'
// own output, the exact same text "show running-config" prints,
// joined with newlines and a trailing one, to that file, creating its
// parent directory if this is the first time anything has been
// saved. This is deliberately the same single, classic-command-text
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
// RevealVendorDefinedSecrets set, is exactly what gets written to
// disk. A vendor defined secret written into startup-config this way
// can never actually be restored from it later, since
// "password manager hash <HIDDEN>" is refused, not a recognized hash,
// see auth.IsRecognizedHash, and cmd_password_manager.go's own
// UserSettablePassword check would refuse it regardless. That is
// deliberate, not an oversight: a vendor defined secret was never
// meant to survive a copy out to a file an ordinary end user, even
// one with su-config access but no real reason to be looking at
// startup-config's raw text, might read.
//
// RouterCLI replays this file back in automatically, once, at process
// startup, before establishSession or the interactive loop begins,
// see loadStartupConfig in main.go and cmd_su_config.go's own doc
// comment for the full reasoning behind why that is safe with no
// password prompting at all.
//
// What actually gets written here is not quite byte for byte
// runningConfigLines' own output. execEnterWords, see cmd_show.go, is
// prepended first, when the shipped tree resolves it, "enable" in
// this project's own tree. "show running-config" itself is only ever
// reachable from inside exec in the first place, so a live session
// viewing it, or pasting its own output back in by hand, is already
// assumed to have elevated to exec first, the same real Cisco and HP
// convention. A fresh process replaying this file at boot has nobody
// to have typed "enable" first, starting completely cold at base
// instead, so this one extra line is what makes the saved file
// self-contained and replayable starting from base, not only from
// inside an already-elevated session.
func init() {
	command.Register("copy.running-config.startup-config", func(ctx *command.AppContext, args []string) error {
		state := ctx.State.(*ProductState)
		var lines []string
		if enter, ok := execEnterWords(ctx); ok {
			lines = append(lines, strings.Join(enter, " "))
		}
		lines = append(lines, runningConfigLines(ctx, state)...)
		text := strings.Join(lines, "\n") + "\n"

		if dir := filepath.Dir(ctx.StartupConfigFile); dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0750); err != nil {
				return fmt.Errorf("%s", ctx.Translator.T("copy_running_config_startup_config.failed", err))
			}
		}
		if err := os.WriteFile(ctx.StartupConfigFile, []byte(text), 0640); err != nil {
			return fmt.Errorf("%s", ctx.Translator.T("copy_running_config_startup_config.failed", err))
		}

		ctx.Logger.Debugln("DEBUG: running-config copied to startup-config at", ctx.StartupConfigFile)
		fmt.Println(ctx.Translator.T("copy_running_config_startup_config.confirm"))
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
