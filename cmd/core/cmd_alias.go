// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"
	"strings"

	"github.com/gotermme/routercli/command"
)

// init - This function registers "alias <alias-name> <word...>", a
// command reachable from every Command Level, see
// var/tree/level_common.yaml, negatable as "no alias <alias-name>".
// This lives in cmd/core rather than cmd/product, unlike hostname or
// the "line" mode item 11 later adds, since a runtime defined command
// alias is a framework capability, not a Cisco or HP flavored piece of
// product state; any project built on this library gets it for free.
//
// A session never types which Command Level the alias belongs to. The
// target level is always the one the session is actually standing in
// at the moment "alias" is typed, ctx.Position.Current(), so an alias
// can only ever be defined for a level a session can actually reach,
// and there is nothing left to type wrong. alias-name is the short
// word a session will type in that level from now on; word... is the
// real command it expands to, everything after alias-name, taken
// literally, with no further parsing here beyond what
// tokenize.Tokenize already split the whole typed line into. See
// command.ExpandAlias, wired into main.go's runLoop, for where that
// expansion actually happens.
//
// Persistence is not this file's job. An alias defined here simply
// lives on the target level's own CommandLevel.Aliases map for the
// rest of this process's life, the same way hostname or interface
// state lives directly on command.AppContext.State until something
// else renders it, and, per this project's own core design goal, does
// not survive a restart on its own. See
// cmd/product/cmd_show.go's own runningConfigLines and its sibling
// alias rendering functions for how a defined alias, at any Command
// Level, is written into running-config, and from there into
// startup-config, once a session actually asks for that with "write
// memory", and design-goals.md's own "nothing survives a restart
// without an explicit save" goal for the reasoning behind requiring
// that explicit step at all.
//
// Tab completion and the interactive "?" help path, package
// completer, do not currently expand an alias the way main.go's
// runLoop does. Typing an alias's own short name still runs the real
// command it expands to, but completing partway through typing one
// offers no special help beyond what the alias's own literal name
// itself already gets as an ordinary, if invisible to the tree, typed
// word. This is a known, narrower scope than real Cisco's own
// completer, left for a later phase if it turns out to matter in
// practice.
//
// Redefining an alias name that is already defined is refused; "no
// alias <alias-name>" must remove it first, before it can be defined
// again. This is a deliberate security measure, at the user's own
// request, not simply "define again to change it" the way real
// Cisco's own "alias exec" convention works: a session, or anyone
// able to type at one, could otherwise change what an already trusted
// alias name quietly expands to, and a session that never happens to
// run "show aliases" again would have no way to notice the change. A
// visible "no alias" first, its own separate confirmation printed,
// makes that kind of change something a session has to see happen,
// not something that can slip past unnoticed inside a single command.
// This check is waived only while ctx.ReplayingStartupConfig is true,
// see AppContext.ReplayingStartupConfig's own doc comment in
// command/model.go, since replaying a saved startup-config, "reload"
// among its callers, restates every alias a session already has, not
// a live session's own typed change, and this project's own
// `reload`/`reboot` reruns that same replay against the very same
// in-memory ctx.Levels rather than a freshly loaded one, see
// command.LoadStartupConfig's own doc comment, so replay must be free
// to restate an alias already present without being refused as a
// collision with itself.
//
// Every read below, the unknown level check, the reserved word and
// command collision checks, and the already-defined check, stays a
// direct read off ctx.Levels, the same as every other framework-level
// Levels lookup in this project; command.DaemonClient exposes only
// Mutate methods, no Read method of any kind, evidence that reads were
// never meant to route through it. Only the actual write, the delete
// or the new alias assignment, runs inside
// ctx.DaemonClient.MutateLevels, and, once inside that closure,
// re-resolves level from the closure's own levels parameter rather
// than reusing the level pointer read above, the same discipline
// cmd_hostname.go's own "hostname" handler already follows: a real,
// remote daemon implementation would hand this closure a freshly
// fetched copy each call, not the pointer read before the call began.
func init() {
	command.Register("alias", func(ctx *command.AppContext, args []string) error {
		currentName := ctx.Position.Current().Name
		level, ok := ctx.Levels.ByName[currentName]
		if !ok {
			return fmt.Errorf("%s", ctx.Translator.T("alias.unknown_level", currentName))
		}

		if ctx.Negated {
			// ValidateArgs is never called for a negated command, see
			// command.ValidateArgs's own doc comment, so the minimum
			// shape "no alias <alias-name>" needs is checked by hand
			// here, before args[0] is read.
			if len(args) < 1 {
				return fmt.Errorf("%s", ctx.Translator.T("alias.negate_usage"))
			}
			aliasName := args[0]
			if _, defined := level.Aliases[aliasName]; !defined {
				return fmt.Errorf("%s", ctx.Translator.T("alias.not_defined", aliasName, currentName))
			}
			_, err := ctx.DaemonClient.MutateLevels(func(levels *command.TreeStructure) (any, error) {
				delete(levels.ByName[currentName].Aliases, aliasName)
				return nil, nil
			})
			if err != nil {
				return err
			}
			ctx.Logger.Debugln("DEBUG: alias removed:", aliasName, "from level", currentName)
			fmt.Println(ctx.Translator.T("alias.removed", aliasName, currentName))
			return nil
		}

		// MinArgs (2, see var/tree/level_common.yaml) guarantees args[0]
		// and args[1] both exist here.
		aliasName, expansion := args[0], args[1:]

		if aliasName == "no" {
			// "no" is a reserved word, see command.Resolve's own doc
			// comment, never a real command name, so it can never be a
			// real alias name either, an alias called "no" could never
			// actually be typed and reached, since every leading "no"
			// is always stripped and treated as negation first.
			return fmt.Errorf("%s", ctx.Translator.T("alias.reserved_name", aliasName))
		}
		if _, exists := ctx.Position.Current().Tree[aliasName]; exists {
			// A real command already answers to this name at this
			// Command Level, "show" for instance, and silently
			// shadowing it with an alias would surprise a session far
			// more than refusing the alias outright.
			return fmt.Errorf("%s", ctx.Translator.T("alias.collides_with_command", aliasName, currentName))
		}
		if _, defined := level.Aliases[aliasName]; defined && !ctx.ReplayingStartupConfig {
			// See this function's own doc comment for the security
			// reasoning: a session must remove an existing alias
			// before it can be redefined, rather than silently
			// overwriting it in one step.
			return fmt.Errorf("%s", ctx.Translator.T("alias.already_defined", aliasName, currentName, "no alias "+aliasName))
		}

		_, err := ctx.DaemonClient.MutateLevels(func(levels *command.TreeStructure) (any, error) {
			target := levels.ByName[currentName]
			if target.Aliases == nil {
				target.Aliases = make(map[string][]string)
			}
			target.Aliases[aliasName] = expansion
			return nil, nil
		})
		if err != nil {
			return err
		}
		ctx.Logger.Debugln("DEBUG: alias defined:", aliasName, "->", strings.Join(expansion, " "), "for level", currentName)
		fmt.Println(ctx.Translator.T("alias.confirm", aliasName, currentName, strings.Join(expansion, " ")))
		return nil
	})
}
