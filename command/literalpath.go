// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

// LiteralCommandPath walks tree, and every Subcommands map nested
// under it, looking for the one Command whose own Run field equals
// runKey, and returns the sequence of literal keys, in nesting order,
// that reaches it, exactly the words a person would type at the CLI
// to run it, alongside true. It returns nil, false when no command
// anywhere in tree has that Run value.
//
// This exists so a generated CLI environment can render its own
// configuration back out as real, typeable command text, discovering
// what a session actually needs to type to enter a Command Level, the
// words "configure" and "terminal" for example, by reading the exact
// same tree that already defines that command, rather than keeping a
// second, separate declaration of the same fact somewhere else that
// could quietly drift out of sync with the first the moment either
// one changes on its own. CommandLevel.EnterCommand only ever stores
// the registered handler name, "configure.terminal" for instance, not
// the words a person actually types, since more than one tree path
// could in principle register under the same handler name and a
// handler name gives no promise about how many words, or which ones,
// produced it, the "password-manager" handler behind "password
// manager" being one such example already in this project's own
// var/tree/level_config.yaml.
//
// Go's map[string]*Command carries no order of its own, so when more
// than one path in tree shares the same Run value, LiteralCommandPath
// returns whichever one its own walk happens to reach first, which is
// not guaranteed to be stable across calls. A caller that needs a
// guaranteed, single answer MUST keep the handler name it is looking
// up reachable by exactly one path in tree, which every tree shipped
// with this project already does.
func LiteralCommandPath(tree map[string]*Command, runKey string) ([]string, bool) {
	for key, cmd := range tree {
		if cmd.Run == runKey {
			return []string{key}, true
		}
		if len(cmd.Subcommands) > 0 {
			if rest, ok := LiteralCommandPath(cmd.Subcommands, runKey); ok {
				return append([]string{key}, rest...), true
			}
		}
	}
	return nil, false
}
