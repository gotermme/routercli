// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"fmt"
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
		fmt.Println(ctx.Translator.T("show.startup_config.text"))
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
// The "password manager <hash>" line reproduced here is a known,
// separate limitation, not something this function set out to fix.
// cmd_password_manager.go, in cmd/core, always prompts interactively
// for a new secret and ignores any argument on its own command line,
// so pasting this exact line back in starts a fresh prompt rather than
// restoring the hash directly. It is left exactly as the previous
// version of this function already printed it, recorded here rather
// than quietly changed, since deciding what that line should actually
// do once config persistence itself is built is its own separate
// question.
func configModeLines(ctx *command.AppContext, state *ProductState) []string {
	var lines []string

	if state.Hostname != "" {
		lines = append(lines, "hostname "+tokenize.QuoteIfNeeded(state.Hostname))
	}
	// See this function's own doc comment on why "password manager
	// <hash>" is reproduced here unchanged rather than fixed.
	for _, level := range ctx.Levels.Order {
		if level.PasswordHash != "" {
			lines = append(lines, "password manager "+level.PasswordHash)
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
