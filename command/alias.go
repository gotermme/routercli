// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

// ExpandAlias - This function checks tokens[0] against every runtime
// defined command alias currently registered for the session's own
// current Command Level, ctx.Position.Current().Name, see
// CommandLevel.Aliases, and, when a match is found, returns a new
// token slice with tokens[0] replaced by that alias's own expansion,
// the rest of tokens, whatever trailing arguments the alias itself
// was typed with, left in place and appended after it. tokens is
// returned unchanged, the same slice, when there is nothing to
// expand, tokens is empty, ctx.Levels or ctx.Position is nil, the
// current level has never had an alias defined against it, or
// tokens[0] does not name one.
//
// Expansion is deliberately a single pass, never recursive. An alias
// whose own expansion happens to start with another alias's own name
// is resolved only one level deep, the same restraint real Cisco's
// own "alias exec" applies, so a session can never define a cycle
// that hangs command dispatch. This is checked in main.go's runLoop,
// before command.Resolve ever runs, not woven into Resolve itself,
// since an alias is a purely textual substitution on what a session
// typed, a short name standing in for some longer, real command, not
// a new kind of node Resolve itself needs to know how to walk.
func ExpandAlias(ctx *AppContext, tokens []string) []string {
	if len(tokens) == 0 || ctx.Levels == nil || ctx.Position == nil {
		return tokens
	}
	level, ok := ctx.Levels.ByName[ctx.Position.Current().Name]
	if !ok || len(level.Aliases) == 0 {
		return tokens
	}
	expansion, ok := level.Aliases[tokens[0]]
	if !ok {
		return tokens
	}
	expanded := make([]string, 0, len(expansion)+len(tokens)-1)
	expanded = append(expanded, expansion...)
	expanded = append(expanded, tokens[1:]...)
	return expanded
}
