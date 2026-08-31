// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/tokenize"
)

func init() {
	command.Register("show.running-config", func(ctx *command.AppContext, args []string) error {
		ctx.Logger.Debugln("DEBUG: generating running-config output")
		state := ctx.State.(*ProductState)
		for _, line := range runningConfigLines(ctx, state) {
			fmt.Println(line)
		}
		return nil
	})

	// interface and startup-config exist mainly to demonstrate, and
	// give a real test case for, a command tree with several siblings
	// sharing no forced prefix collision, the double-Tab design.
	// "show <TAB>" here has candidates including interface,
	// running-config, and startup-config, and none of them should ever
	// get silently auto-picked. See
	// command/node_test.go's TestResolveMultipleSubtreeOptionsNeverAutoPicksOne
	// for the same principle locked down as a unit test.
	command.Register("show.interface", func(ctx *command.AppContext, args []string) error {
		state := ctx.State.(*ProductState)
		if len(state.Interfaces) == 0 {
			fmt.Println(ctx.Translator.T("show.interface.text"))
			return nil
		}
		for _, name := range sortedInterfaceNames(state) {
			iface := state.Interfaces[name]
			status := ctx.Translator.T("show.interface.up")
			if iface.Shutdown {
				status = ctx.Translator.T("show.interface.admin_down")
			}
			fmt.Printf("%s: %s\n", name, status)
			if iface.Description != "" {
				fmt.Println("  " + ctx.Translator.T("show.interface.description_label", iface.Description))
			}
		}
		return nil
	})

	command.Register("show.startup-config", func(ctx *command.AppContext, args []string) error {
		data, err := os.ReadFile(ctx.StartupConfigFile)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println(ctx.Translator.T("show.startup_config.text"))
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s", ctx.Translator.T("show.startup_config.read_failed", err))
		}
		fmt.Print(string(data))
		return nil
	})
}

// runningConfigLines - This function builds "show running-config"'s
// output as an ordered slice of lines, one command per line, using
// tokenize.QuoteIfNeeded on every value so each line can be copied out
// of the terminal and pasted straight back in, at the same Command
// Level shape a real session already understands, to reproduce the
// exact same state, covering hostname, per-interface state, and the
// top-level description. An empty or unset value is omitted entirely,
// matching how a real router only shows config lines for values that
// have actually been set.
//
// Ordering matters here, not just content, since these lines are
// meant to be typed, or pasted, into a real session in this exact
// order. "set description" is registered directly on the exec level,
// var/tree/level_exec.yaml, so it is printed first, valid to run
// immediately. Everything else here, hostname, password manager, and
// every interface, only exists inside config mode,
// var/tree/level_config.yaml and var/tree/level_config_if.yaml, so
// configModeLines's own output is wrapped in whatever this session
// actually needs to type to enter config mode, "configure terminal"
// today, and "end" to leave it again, rather than assuming a caller is
// already sitting in config mode the way this function's own earlier
// version once did. See configEnterWords and configModeLines below for
// how each of those two pieces is produced.
func runningConfigLines(ctx *command.AppContext, state *ProductState) []string {
	lines := []string{"! (example running-config)"}

	if state.Description != "" {
		lines = append(lines, "set description "+tokenize.QuoteIfNeeded(state.Description))
	}

	configLines := configModeLines(ctx, state)
	if len(configLines) > 0 {
		enter, ok := configEnterWords(ctx)
		if ok {
			lines = append(lines, strings.Join(enter, " "))
		}
		lines = append(lines, configLines...)
		if ok {
			lines = append(lines, "end")
		}
	}

	lines = append(lines, "!")
	return lines
}

// configModeLines - This function returns every line that only means
// something once a session has actually entered config mode: hostname,
// each Command Level's own "password manager" secret, and one block
// per interface that has ever actually been touched. It does not
// itself decide how a caller gets into config mode to run these, see
// runningConfigLines above for that.
//
// "terminal length" and "terminal width" are deliberately never
// reproduced here. command.AppContext.PageLines and
// command.AppContext.TerminalWidth, their real, functional
// counterparts, are both session scoped, exactly matching real Cisco
// and HP, neither of which ever writes a plain, EXEC level "terminal
// length" or "terminal width" to running-config or startup-config
// either. See cmd/core/cmd_terminal.go's own doc comment, and "show
// terminal" in cmd/core/cmd_show.go for the one place a session
// actually reports either value back.
//
// A second and later interface block is preceded by "exit" rather than
// nothing, since cmd_interface.go only accepts "interface" while a
// session is sitting exactly in config mode, not still nested inside a
// previous interface's own config interface mode, see
// command.RequireCurrentCommandLevel. The very last block does not
// need this: runningConfigLines' own trailing "end" pops back to exec
// directly from any nesting depth, see
// command.CommandLevelStack.PopToRoot, so nothing needs to back out of
// the last interface block first.
//
// The "password manager hash <hash>" line reproduced here for each
// level's own ordinary, user settable PasswordHash relies on
// cmd/core/cmd_password_manager.go's "password manager hash" command,
// which accepts an already-hashed value directly rather than
// prompting, exactly so this line can be pasted back in and restore
// the same secret without ever exposing, or needing to know, the real
// plaintext password. A level's own VendorDefinedPasswordHash renders
// this same way only while the current session is somewhere with
// RevealVendorDefinedSecrets set, su-config being the one level in
// this project's own shipped tree that sets it; everywhere else it
// renders as the "<HIDDEN>" placeholder instead. See
// currentLevelRevealsVendorDefinedSecrets below.
//
// This block is still a known, separate limitation, one su-config
// does not solve either: every level's own line is emitted together,
// in one flat list, right after "configure terminal", rather than
// each positioned inside a paste sequence that actually reenters that
// specific level first. "password manager hash" only ever sets the
// secret for whichever level ctx.Session.CommandLevel happens to be
// while it runs, see that command's own doc comment, so this block,
// pasted back in as is, would only ever affect the level a session is
// actually sitting in at the time, not each named level individually.
// su-config's own GrantsReplayTrust solves a different problem,
// whether entering a gated level during a paste needs a fresh prompt
// at all, not which level a "password manager hash" line inside that
// paste actually targets. Getting every line positioned correctly
// remains open, left for whichever future phase actually builds a
// real paste or replay command, rather than solved by coincidence
// here.
func configModeLines(ctx *command.AppContext, state *ProductState) []string {
	var lines []string

	if state.Hostname != "" {
		lines = append(lines, "hostname "+tokenize.QuoteIfNeeded(state.Hostname))
	}
	if state.BannerMOTD != "" {
		lines = append(lines, "banner motd "+tokenize.QuoteIfNeeded(state.BannerMOTD))
	}
	if state.BannerLogin != "" {
		lines = append(lines, "banner login "+tokenize.QuoteIfNeeded(state.BannerLogin))
	}
	// See this function's own doc comment for why a vendor defined
	// secret renders as a "<HIDDEN>" placeholder here, never its real
	// value, while an ordinary, user settable secret renders in full.
	// reveal is decided once, by which Command Level this session is
	// actually sitting in right now, not per rendered level, since
	// su-config's whole point is a session that already proved a
	// real, live credential there gets to see everything, not a
	// selective peek at one level at a time; see
	// currentLevelRevealsVendorDefinedSecrets and
	// command.CommandLevel.RevealVendorDefinedSecrets's own doc
	// comment.
	reveal := currentLevelRevealsVendorDefinedSecrets(ctx)
	for _, level := range ctx.Levels.Order {
		switch {
		case level.VendorDefinedPasswordHash != "" && reveal:
			lines = append(lines, "password manager hash "+level.VendorDefinedPasswordHash)
		case level.VendorDefinedPasswordHash != "":
			// "<HIDDEN>" is not itself a recognized hash, see
			// auth.IsRecognizedHash, so pasting this exact line back in
			// is refused rather than silently corrupting or clearing
			// the real secret, on top of
			// command.CommandLevel.UserSettablePassword already
			// refusing "password manager hash" outright for a vendor
			// defined level regardless of what value follows it.
			lines = append(lines, "password manager hash <HIDDEN>")
		case level.PasswordHash != "":
			lines = append(lines, "password manager hash "+level.PasswordHash)
		}
	}

	var configuredNames []string
	for _, name := range sortedInterfaceNames(state) {
		iface := state.Interfaces[name]
		if iface.Description == "" && !iface.Shutdown {
			// Nothing was ever actually configured on this interface,
			// so an empty block is not printed for it, the same
			// reasoning as omitting an unset Description above.
			continue
		}
		configuredNames = append(configuredNames, name)
	}
	for i, name := range configuredNames {
		iface := state.Interfaces[name]
		if enter, ok := interfaceEnterWords(ctx, name); ok {
			lines = append(lines, strings.Join(enter, " "))
		} else {
			lines = append(lines, "interface "+name)
		}
		if iface.Description != "" {
			lines = append(lines, " description "+tokenize.QuoteIfNeeded(iface.Description))
		}
		if iface.Shutdown {
			lines = append(lines, " shutdown")
		}
		if i != len(configuredNames)-1 {
			lines = append(lines, "exit")
		}
	}

	return lines
}

// currentLevelRevealsVendorDefinedSecrets - This function reports
// whether ctx.Session.CommandLevel currently names a Command Level
// whose own RevealVendorDefinedSecrets is true, su-config being the
// one level in this project's own shipped tree that sets it, see
// var/tree/tree_structure.yaml. This reads a property off the current
// level rather than comparing ctx.Session.CommandLevel against a
// hardcoded "su-config" literal, the same generic,
// read-a-property-rather-than-hardcode-a-level-name approach
// command.CommandLevel.RevealVendorDefinedSecrets's own doc comment
// describes, so a project renaming or restructuring its own version
// of this level never needs to touch this function. false, safely,
// whenever ctx.Levels or ctx.Session is nil, or ctx.Session.CommandLevel
// does not resolve to a real, loaded level at all, the same
// unconfigured-context safety cmd_password_manager.go's own current
// level lookup follows.
func currentLevelRevealsVendorDefinedSecrets(ctx *command.AppContext) bool {
	if ctx.Levels == nil || ctx.Session == nil {
		return false
	}
	level, ok := ctx.Levels.ByName[ctx.Session.CommandLevel]
	if !ok {
		return false
	}
	return level.RevealVendorDefinedSecrets
}

// configEnterWords - This function returns the literal words that
// move a session from exec into config mode, "configure terminal" in
// this project today, discovered from the exec level's own tree
// through command.LiteralCommandPath rather than repeated here as a
// second, separate literal that could quietly drift out of sync with
// var/tree/level_exec.yaml. false means ctx.Levels does not have a
// fully loaded "config" entry with a resolvable Parent, the case in a
// handful of narrow unit tests in this package that only care about
// the values inside the config block itself, not the literal wrapper
// words around it; runningConfigLines falls back to leaving those
// wrapper lines out entirely rather than guessing at them.
func configEnterWords(ctx *command.AppContext) ([]string, bool) {
	return levelEnterWords(ctx, "config")
}

// execEnterWords - This function does the same thing as
// configEnterWords, for entering exec from base, "enable" in this
// project's own shipped tree. "show running-config" itself never
// needs this: it is only ever reachable from inside exec in the first
// place, matching real Cisco and HP, so a live session viewing it, or
// pasting its output back in by hand, is already assumed to have
// elevated to exec first. cmd_startup_config.go's own "copy
// running-config startup-config" is the one caller that does need
// this, prepending it to what actually gets written to
// StartupConfigFile, since a fresh process replaying that file back
// in at boot, see main.go's own loadStartupConfig, starts completely
// cold, at base, with nobody having typed "enable" by hand first the
// way an interactive paste always assumes.
func execEnterWords(ctx *command.AppContext) ([]string, bool) {
	return levelEnterWords(ctx, "exec")
}

// interfaceEnterWords - This function does the same thing as
// configEnterWords, for entering config interface mode for one
// specific interface, appending name as the argument
// cmd_interface.go's own "interface" command expects, since that value
// comes from this state, not from anything command.CommandLevel
// itself knows about.
func interfaceEnterWords(ctx *command.AppContext, name string) ([]string, bool) {
	words, ok := levelEnterWords(ctx, "config-if")
	if !ok {
		return nil, false
	}
	return append(words, name), true
}

// levelEnterWords - This function looks up levelName in ctx.Levels,
// then its own Parent, and returns the literal words, found in the
// parent's own tree through command.LiteralCommandPath, that a
// session sitting in that parent level types to reach levelName. See
// configEnterWords and interfaceEnterWords above for both of this
// project's own callers.
func levelEnterWords(ctx *command.AppContext, levelName string) ([]string, bool) {
	if ctx.Levels == nil {
		return nil, false
	}
	level, ok := ctx.Levels.ByName[levelName]
	if !ok {
		return nil, false
	}
	parent, ok := ctx.Levels.ByName[level.Parent]
	if !ok {
		return nil, false
	}
	return command.LiteralCommandPath(parent.Tree, level.EnterCommand)
}

func sortedInterfaceNames(state *ProductState) []string {
	names := make([]string, 0, len(state.Interfaces))
	for name := range state.Interfaces {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
