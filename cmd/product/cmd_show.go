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
	"strconv"
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
// have actually been set. This is also, unchanged, exactly what
// "write memory" writes to StartupConfigFile, see
// cmd_startup_config.go; there is deliberately one function producing
// one canonical text, never a second, slightly different version for
// what gets written to disk.
//
// Ordering matters here, not just content, since this whole output is
// meant to be typed, or pasted, into a fresh session starting cold at
// base, the same assumption command.LoadStartupConfig's own boot time
// replay makes. Every runtime defined command alias, see
// command.CommandLevel.Aliases and cmd/core/cmd_alias.go, is rendered
// somewhere in this output, positioned inside whatever this session
// actually needs to type to reach the Command Level it belongs to,
// never all in one flat block the way Phase 32's own first version of
// this feature did. base's own aliases need no wrapper at all, they
// render first, directly, since base is where this whole script
// already starts. user's own aliases render next, wrapped in "user"
// and the generic "end" that leaves it, reachable directly from base;
// see cmd/core/cmd_user.go's own doc comment for why replaying this
// block back in at boot needs no login of its own, the same trust
// command.AppContext.ReplayingStartupConfig already extends to every
// password protected Command Level along the way. Everything from
// here on is exec-rooted: "set description" is registered directly on
// the exec level, var/tree/level_exec.yaml; exec's own aliases;
// admin's and diagnostic's aliases, each wrapped in their own short
// enter, alias lines, exit block, wrappedLevelAliasLines below; and
// configModeLines' own output, hostname, password manager, and every
// interface, wrapped in whatever this session needs to type to enter
// config mode, "configure terminal" today, and "end" to leave it
// again. All of it needs "enable" typed first to actually reach exec,
// so this function prepends that one line itself, through
// execEnterWords, whenever there is any exec-rooted content at all to
// follow, rather than leaving that to whichever caller happens to be
// writing this text to disk. See configEnterWords and configModeLines
// below for how the config mode wrapper itself is produced.
func runningConfigLines(ctx *command.AppContext, state *ProductState) []string {
	lines := []string{"! (example running-config)"}

	if baseLevel, ok := ctx.Levels.ByName["base"]; ok {
		lines = append(lines, aliasLinesForLevel(baseLevel)...)
	}
	lines = append(lines, wrappedLevelAliasLines(ctx, "user")...)

	var execRootedLines []string

	if state.Description != "" {
		execRootedLines = append(execRootedLines, "set description "+tokenize.QuoteIfNeeded(state.Description))
	}
	if execLevel, ok := ctx.Levels.ByName["exec"]; ok {
		execRootedLines = append(execRootedLines, aliasLinesForLevel(execLevel)...)
	}
	execRootedLines = append(execRootedLines, wrappedLevelAliasLines(ctx, "admin")...)
	execRootedLines = append(execRootedLines, wrappedLevelAliasLines(ctx, "diagnostic")...)

	configLines := configModeLines(ctx, state)
	if len(configLines) > 0 {
		enter, ok := configEnterWords(ctx)
		if ok {
			execRootedLines = append(execRootedLines, strings.Join(enter, " "))
		}
		execRootedLines = append(execRootedLines, configLines...)
		if ok {
			execRootedLines = append(execRootedLines, "end")
		}
	}

	if len(execRootedLines) > 0 {
		if enter, ok := execEnterWords(ctx); ok {
			lines = append(lines, strings.Join(enter, " "))
		}
		lines = append(lines, execRootedLines...)
	}

	lines = append(lines, "!")
	return lines
}

// configModeLines - This function returns every line that only means
// something once a session has actually entered config mode: hostname,
// each Command Level's own "password manager" secret, every runtime
// defined command alias, one block per interface that has ever
// actually been touched, and, last, one "line" mode block for this
// deployment's own persisted terminal length, width, and paging
// defaults. It does not itself decide how a caller gets into config
// mode to run these, see runningConfigLines above for that.
//
// "terminal length" and "terminal width" are deliberately never
// reproduced here. command.AppContext.PageLines and
// command.AppContext.TerminalWidth, their real, functional
// counterparts, are both session scoped, exactly matching real Cisco
// and HP, neither of which ever writes a plain, EXEC level "terminal
// length" or "terminal width" to running-config or startup-config
// either. See cmd/core/cmd_terminal.go's own doc comment, and "show
// terminal" in cmd/core/cmd_show.go for the one place a session
// actually reports either value back. "line length", "line width",
// and "line paging" below are a genuinely different thing, not a
// second way of writing the same session scoped value: they are this
// deployment's own default, see ProductState.Line's own doc comment
// and cmd/product/cmd_line.go, item 11 of the Framework Gap Roadmap,
// which is exactly the kind of value real Cisco and HP do persist,
// through "line vty" and "line console".
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
// RevealVendorDefinedSecrets set, admin being the one level in
// this project's own shipped tree that sets it; everywhere else it
// renders as the "<HIDDEN>" placeholder instead. See
// currentLevelRevealsVendorDefinedSecrets below.
//
// This block is still a known, separate limitation, one admin
// does not solve either: every level's own line is emitted together,
// in one flat list, right after "configure terminal", rather than
// each positioned inside a paste sequence that actually reenters that
// specific level first. "password manager hash" only ever sets the
// secret for whichever level ctx.Session.CommandLevel happens to be
// while it runs, see that command's own doc comment, so this block,
// pasted back in as is, would only ever affect the level a session is
// actually sitting in at the time, not each named level individually.
// admin's own GrantsReplayTrust solves a different problem,
// whether entering a gated level during a paste needs a fresh prompt
// at all, not which level a "password manager hash" line inside that
// paste actually targets. Getting every line positioned correctly
// remains open, left for whichever future phase actually builds a
// real paste or replay command, rather than solved by coincidence
// here. This is unrelated to how a runtime defined command alias is
// rendered, just below; "alias" itself, unlike "password manager
// hash", always applies to whichever Command Level a session is
// actually standing in when it runs, see cmd/core/cmd_alias.go, so an
// alias line genuinely can, and does, get positioned inside a paste
// sequence that reenters its own specific level first.
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
	// admin's whole point is a session that already proved a
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

	// Every runtime defined command alias belonging to config itself,
	// see command.CommandLevel.Aliases and cmd/core/cmd_alias.go,
	// renders here as one "alias <name> <word...>" line per alias, with
	// no wrapper of its own needed, since this whole function already
	// runs inside a "configure terminal" paste. config-if and
	// config-line aliases are handled separately, further down, since
	// each needs its own short enter, exit block first.
	if configLevel, ok := ctx.Levels.ByName["config"]; ok {
		lines = append(lines, aliasLinesForLevel(configLevel)...)
	}

	// hasConfigIfAliasBlock and hasLineBlock decide, before the
	// interface loop below ever runs, whether either trailing block
	// further down, the config-if alias block or the "line" mode
	// block, will be appended after every interface block. This is
	// needed there, not just here, since the very last interface block
	// only omits its own trailing "exit" when nothing else follows it
	// in this flat, sequential paste: with either block coming next,
	// that last interface block needs to leave config-if mode first,
	// the same way every interior interface block already does.
	configIfLevel := ctx.Levels.ByName["config-if"]
	hasConfigIfAliasBlock := configIfLevel != nil && len(configIfLevel.Aliases) > 0
	configLineLevel := ctx.Levels.ByName["config-line"]
	hasLineBlock := state.Line.Length != nil || state.Line.Width != nil || state.Line.Paging != nil || (configLineLevel != nil && len(configLineLevel.Aliases) > 0)
	hasTrailingBlock := hasConfigIfAliasBlock || hasLineBlock

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
		if i != len(configuredNames)-1 || hasTrailingBlock {
			lines = append(lines, "exit")
		}
	}

	// A command alias defined while actually standing inside "interface
	// <name>" mode, config-if, is scoped to the whole config-if
	// Command Level, not to that one interface, exactly the same way
	// "password manager hash" is scoped to a whole level rather than one
	// instance of it, see command.CommandLevel.Aliases's own doc
	// comment. Replaying it back in therefore only needs some interface
	// name to walk through, any interface name, not necessarily one a
	// session ever actually configured anything else on, so this block
	// uses configIfAliasReplayInterfaceName, a name reserved for this
	// purpose alone, rather than reusing one of configuredNames above.
	// Entering and leaving config-if this way never touches
	// state.Interfaces at all, see cmd_interface.go's own doc comment,
	// so this never creates a real, visible interface as a side effect.
	if hasConfigIfAliasBlock {
		if enter, ok := interfaceEnterWords(ctx, configIfAliasReplayInterfaceName); ok {
			lines = append(lines, "! the interface named below exists only to replay config-if command aliases; it carries no configuration of its own")
			lines = append(lines, strings.Join(enter, " "))
			lines = append(lines, aliasLinesForLevel(configIfLevel)...)
			if hasLineBlock {
				lines = append(lines, "exit")
			}
		}
	}

	// "line" mode, item 11 of the Framework Gap Roadmap, renders last,
	// matching where a real Cisco or HP device's own "line vty" or
	// "line console" block typically appears in running-config, after
	// every interface. A field left nil here, "line length" for
	// instance never having been typed, is simply left out of this
	// block, the same "nothing configured, nothing shown" convention
	// every other optional value in this function already follows.
	// Every config-line alias, see aliasLinesForLevel, renders inside
	// this same block too, right alongside length, width, and paging,
	// rather than in a block of its own, since config-line, unlike
	// config-if, is only ever one single, deployment wide instance, so
	// there is no reason to enter and leave it twice. hasLineBlock
	// above is false, and this whole block is skipped, only when all
	// three fields are nil and config-line has no aliases either.
	if hasLineBlock {
		if enter, ok := lineEnterWords(ctx); ok {
			lines = append(lines, strings.Join(enter, " "))
		} else {
			lines = append(lines, "line")
		}
		if state.Line.Length != nil {
			lines = append(lines, " length "+strconv.Itoa(*state.Line.Length))
		}
		if state.Line.Width != nil {
			lines = append(lines, " width "+strconv.Itoa(*state.Line.Width))
		}
		if state.Line.Paging != nil {
			if *state.Line.Paging {
				lines = append(lines, " paging")
			} else {
				lines = append(lines, " no paging")
			}
		}
		for _, line := range aliasLinesForLevel(configLineLevel) {
			lines = append(lines, " "+line)
		}
	}

	return lines
}

// configIfAliasReplayInterfaceName - This constant names the
// interface configModeLines walks through solely to replay a
// config-if scoped command alias back in, see that function's own doc
// comment. It is deliberately distinctive, so it never collides with a
// name a real deployment would actually give one of its own
// interfaces, and it never appears in state.Interfaces at all, since
// merely entering and leaving config-if touches no interface state,
// see cmd_interface.go.
const configIfAliasReplayInterfaceName = "alias-replay-placeholder"

// aliasLinesForLevel - This function returns one "alias <name>
// <word...>" line per entry in level's own Aliases map, sorted
// alphabetically by alias name, the same stability sortedInterfaceNames
// already gives interface blocks above, since Aliases itself is a
// plain Go map with no ordering guarantee of its own. nil, the same as
// an empty level, when level itself is nil, the case whenever a
// project's own tree structure never declared the level being asked
// about at all, admin or diagnostic for instance, in a deployment
// that removed either from its own tree_structure.yaml.
func aliasLinesForLevel(level *command.CommandLevel) []string {
	if level == nil || len(level.Aliases) == 0 {
		return nil
	}
	names := make([]string, 0, len(level.Aliases))
	for name := range level.Aliases {
		names = append(names, name)
	}
	sort.Strings(names)

	lines := make([]string, 0, len(names))
	for _, name := range names {
		words := make([]string, 0, len(level.Aliases[name]))
		for _, w := range level.Aliases[name] {
			words = append(words, tokenize.QuoteIfNeeded(w))
		}
		lines = append(lines, "alias "+name+" "+strings.Join(words, " "))
	}
	return lines
}

// wrappedLevelAliasLines - This function returns the full block needed
// to replay every command alias belonging to levelName back in: the
// literal words that enter it, one "alias <name> <word...>" line per
// alias through aliasLinesForLevel, and the literal words that leave
// it again, in that order. This works the same way regardless of how
// levelName is actually reached, levelExitWords below picking up
// whichever exit levelName's own manifest entry declares, or falling
// back to the generic "end" when it declares none. runningConfigLines
// above calls this for admin and diagnostic, both of which swap the
// whole root Command Level in place, see command.EnterCommandLevel and
// command.ExitCommandLevel, using their own real exit command,
// "return.admin" or "exit-diagnostic-mode" in this project's own
// shipped tree; and for user, which instead pushes a nested frame the
// way config, config-if, and config-line do, and so leaves through the
// generic "end" every pushed mode already uses.
//
// nil, meaning nothing to render at all, whenever ctx.Levels has no
// entry named levelName, that level has no aliases defined, or either
// the enter or exit words cannot be discovered through
// command.LiteralCommandPath, the same "leave the wrapper out entirely
// rather than guess at it" fallback configEnterWords and its own
// siblings already follow.
func wrappedLevelAliasLines(ctx *command.AppContext, levelName string) []string {
	if ctx.Levels == nil {
		return nil
	}
	level, ok := ctx.Levels.ByName[levelName]
	if !ok || len(level.Aliases) == 0 {
		return nil
	}
	enter, ok := levelEnterWords(ctx, levelName)
	if !ok {
		return nil
	}
	exit, ok := levelExitWords(ctx, levelName)
	if !ok {
		return nil
	}

	lines := []string{strings.Join(enter, " ")}
	lines = append(lines, aliasLinesForLevel(level)...)
	lines = append(lines, strings.Join(exit, " "))
	return lines
}

// currentLevelRevealsVendorDefinedSecrets - This function reports
// whether ctx.Session.CommandLevel currently names a Command Level
// whose own RevealVendorDefinedSecrets is true, admin being the
// one level in this project's own shipped tree that sets it, see
// var/tree/tree_structure.yaml. This reads a property off the current
// level rather than comparing ctx.Session.CommandLevel against a
// hardcoded "admin" literal, the same generic,
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
// project's own shipped tree. runningConfigLines above is the one
// caller, prepending this whenever it has any exec-rooted content to
// follow, base and user's own aliases, reached without ever running
// "enable", coming before it instead. This one line is what makes the
// whole script self-contained and replayable starting from base,
// command.LoadStartupConfig's own boot time replay included, rather
// than assuming a live session pasting it back in by hand has already
// elevated to exec first.
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

// lineEnterWords - This function does the same thing as
// configEnterWords, for entering "line" mode, item 11 of the Framework
// Gap Roadmap. Unlike interfaceEnterWords, "line" itself takes no
// argument, there is only ever one, deployment wide set of line
// defaults today, not a named collection the way interfaces are, so
// this needs nothing appended beyond what levelEnterWords already
// returns.
func lineEnterWords(ctx *command.AppContext) ([]string, bool) {
	return levelEnterWords(ctx, "config-line")
}

// levelEnterWords - This function looks up levelName in ctx.Levels,
// then its own Parent, and returns the literal words, found in the
// parent's own tree through command.LiteralCommandPath, that a
// session sitting in that parent level types to reach levelName. See
// configEnterWords, interfaceEnterWords, lineEnterWords, and
// wrappedLevelAliasLines above for this project's own callers.
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

// levelExitWords - This function looks up levelName in ctx.Levels and
// returns the literal words that leave it again. When levelName
// declares its own ExitCommand, "return.admin" for admin or
// "exit-diagnostic-mode" for diagnostic in this project's own shipped
// tree, the words are found inside levelName's own Tree, not its
// parent's, through command.LiteralCommandPath, since that is where
// EnterCommandLevel and ExitCommandLevel already expect it to be
// registered, see command/treestructure.go. A level with no
// ExitCommand at all, config, config-if, and config-line in this
// project's own shipped tree, is instead left through the shared, generic
// "end" command, see var/tree/level_common.yaml and
// command.CommandLevelStack.PopToRoot, which is always correct for
// one of those, since each is only ever reached by pushing a new frame
// on top of exec, never by swapping the root frame itself the way
// EnterCommandLevel does for admin or diagnostic. See
// wrappedLevelAliasLines above for this function's one real caller,
// only ever used for a level reached through EnterCommandLevel in the
// first place.
func levelExitWords(ctx *command.AppContext, levelName string) ([]string, bool) {
	if ctx.Levels == nil {
		return nil, false
	}
	level, ok := ctx.Levels.ByName[levelName]
	if !ok {
		return nil, false
	}
	if level.ExitCommand == "" {
		return []string{"end"}, true
	}
	return command.LiteralCommandPath(level.Tree, level.ExitCommand)
}

func sortedInterfaceNames(state *ProductState) []string {
	names := make([]string, 0, len(state.Interfaces))
	for name := range state.Interfaces {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
