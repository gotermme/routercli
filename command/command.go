// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"fmt"
	"sort"
	"strings"

	"github.com/gotermme/routercli/i18n"
)

// ----------------------------------------------------------------------
// Public Methods - Command
// ----------------------------------------------------------------------

// ResolvedDesc - This method returns this command's description,
// resolved through the translator if DescKey is set, otherwise the
// literal Desc field. A nil translator is handled the same as an
// empty DescKey, falling back to Desc, since this package works
// perfectly well with i18n never wired in at all, translation being
// additive rather than required.
func (c *Command) ResolvedDesc(t *i18n.Translator) string {
	if c.DescKey != "" && t != nil {
		return t.T(c.DescKey)
	}
	return c.Desc
}

// ResolvedHelp - This method does the same thing as ResolvedDesc, for
// Help and HelpKey.
func (c *Command) ResolvedHelp(t *i18n.Translator) string {
	if c.HelpKey != "" && t != nil {
		return t.T(c.HelpKey)
	}
	return c.Help
}

// ResolvedArgHelp - This method does the same thing as ResolvedDesc,
// for ArgHelp and ArgHelpKey.
func (c *Command) ResolvedArgHelp(t *i18n.Translator) string {
	if c.ArgHelpKey != "" && t != nil {
		return t.T(c.ArgHelpKey)
	}
	return c.ArgHelp
}

// ----------------------------------------------------------------------
// Public Functions - Command
// ----------------------------------------------------------------------

// Resolve - This function answers the one question both command
// execution and tab completion need answered, given these tokens,
// what command do they refer to, and is the answer unambiguous. It
// walks tokens against tree using exact match first, then unique
// prefix, or abbreviation, match, then chases any alias, then
// descends into Subcommands. The first token that matches nothing,
// and is not ambiguous, and everything after it, becomes Args. An
// empty string token is legitimate input here, not skipped, since the
// completer deliberately passes one to mean nothing typed yet for
// this position, which naturally lists every child at that level,
// because strings.HasPrefix(name, "") is always true.
//
// A leading "no" token is handled as a special case before any of
// that. It is stripped off, the remaining tokens are resolved against
// the same tree, since "no" is not a real command with its own
// children, and the result comes back with Negated set to true and
// FullName, Command, Args, and Ambiguous all describing the real
// command that was found, never including "no" itself. Whether the
// resolved command is actually allowed to be negated,
// Command.Negatable, is not checked here. That is main.go's job at
// dispatch time, the same separation of concerns ValidateArgs already
// has from Resolve itself, resolution answers which command was
// meant, not whether it may be run this way.
//
// "no" must be typed exactly. It is never abbreviation matched the
// way ordinary tree commands are, since real Cisco does not abbreviate
// it either. It is a two-letter reserved word, not a real subtree to
// fuzzy match against.
//
// Every return point funnels through the deferred call below, which
// sets RunnableAsIs from whatever result.Command and result.Args
// actually ended up being, so every caller sees the same "<cr>" signal
// regardless of which of Resolve()'s several return points was
// actually taken. See runnableAsIs's own doc comment for exactly what
// it checks.
func Resolve(tree map[string]*Command, tokens []string) (result ResolveResult) {
	defer func() {
		result.RunnableAsIs = runnableAsIs(result.Command, result.Args)
	}()

	if len(tokens) > 0 && tokens[0] == "no" {
		inner := Resolve(tree, tokens[1:])
		inner.Negated = true
		result = inner
		return result
	}

	current := tree
	var directives *Command

	for i, tok := range tokens {
		cmd, exact := current[tok]
		if !exact {
			// Not an exact match, so look for a unique, non-hidden
			// prefix match. A hidden command is still reachable by
			// exact match above, it just cannot be abbreviated to.
			var candidates []string
			for name, c := range current {
				if strings.HasPrefix(name, tok) && !c.Hidden {
					candidates = append(candidates, name)
				}
			}
			sort.Strings(candidates)

			switch len(candidates) {
			case 1:
				// Nothing typed yet for this position, but directives,
				// the command matched so far, is already valid to run
				// on its own, for example "totp enable" is a complete
				// command even though "totp enable qr" also exists
				// below it. Auto-descending into the sole remaining
				// subcommand here, "qr", would silently hide that
				// "enable" itself is already a complete command,
				// which real Cisco and HP never do; both show "qr"
				// alongside "<cr>" instead. This surfaces that the
				// same way genuine ambiguity is surfaced, through the
				// empty-token branch every caller already special-
				// cases, see completer.ambiguousTokenIsEmpty and this
				// function's own callers in package completer and
				// HelpForPath.
				//
				// This never fires for a genuinely partial word, only
				// for tok == "", since abbreviating "sh" to "show" when
				// "show" is the only match is exactly the completion
				// behavior real Cisco and HP already do, and this
				// project's own established convention, not something
				// this changes.
				if tok == "" && runnableAsIs(directives, nil) {
					result.Ambiguous = candidates
					result.AmbiguousTree = current
					result.AmbigAt = i
					result.Command = directives
					return result
				}
				tok = candidates[0]
				cmd = current[tok]
			case 0:
				// Nothing matches, so the remaining tokens are arguments.
				result.Args = append(result.Args, tokens[i:]...)
				result.Command = directives
				return result
			default:
				// Ambiguous: more than one command could match this token.
				result.Ambiguous = candidates
				result.AmbiguousTree = current
				result.AmbigAt = i
				result.Command = directives
				return result
			}
		}

		// Chase any alias to the real command it points at.
		for cmd.Alias != "" {
			cmd = current[cmd.Alias]
		}

		directives = cmd
		result.FullName = append(result.FullName, tok)

		if len(cmd.Subcommands) > 0 {
			current = cmd.Subcommands
		} else {
			// No children below this command, so command matching
			// stops here. Anything left over is arguments, for
			// example "show version extra".
			result.Args = append(result.Args, tokens[i+1:]...)
			result.Command = directives
			return result
		}
	}

	result.Command = directives
	return result
}

// ValidateArgs - This function enforces MinArgs, MaxArgs, and
// MaxArgLength for a resolved command. This is deliberately separate
// from Resolve(), since resolution, which command was meant, and
// validation, whether the arguments to that command are acceptable,
// are different concerns with different failure messages. It is
// called after a command fully resolves and before its handler runs,
// and returns nil if the arguments are acceptable.
//
// A caller should not call this when ResolveResult.Negated is true.
// main.go's runLoop skips it entirely for negated commands on
// purpose, since "no X" often has a different valid argument shape
// than "X" itself. Real Cisco's "no description" takes zero arguments
// to clear a value, while "description <text>" requires exactly one
// to set it. Forcing one set of MinArgs and MaxArgs to describe both
// directions would mean either the positive form accepts arguments it
// should not, or the negated form rejects the arguments, usually
// none, that it needs to accept. A Negatable handler is expected to
// check len(args) itself and decide what is acceptable for its own
// negated case.
func ValidateArgs(cmd *Command, args []string) error {
	if cmd.MinArgs != nil && len(args) < *cmd.MinArgs {
		return fmt.Errorf("not enough arguments: need at least %d, got %d", *cmd.MinArgs, len(args))
	}
	if cmd.MaxArgs != nil && len(args) > *cmd.MaxArgs {
		return fmt.Errorf("too many arguments: accepts at most %d, got %d", *cmd.MaxArgs, len(args))
	}
	if cmd.MaxArgLength > 0 {
		for _, a := range args {
			if len([]rune(a)) > cmd.MaxArgLength {
				return fmt.Errorf("argument exceeds maximum length of %d characters: %q", cmd.MaxArgLength, a)
			}
		}
	}
	return nil
}

// ----------------------------------------------------------------------
// Private Functions - Command
// ----------------------------------------------------------------------

// runnableAsIs - This function reports whether cmd is both set and
// directly executable right now with args as its arguments, matching
// real Cisco and HP's own "<cr>" notation for "you can press enter
// here". cmd must have a RunFunc of its own, a pure container command
// with only Subcommands and no "run:" of its own is never runnable as
// is, and args, once a single trailing empty string is stripped, must
// satisfy both cmd.MinArgs and cmd.MaxArgs.
//
// The trailing empty string strip matters because both of this
// project's own "<cr>" callers, completer.OnChange and HelpForPath's
// caller in completer.handleHelp, follow the same convention every
// other empty-token check in this project already follows, appending
// a synthetic "" token to mean nothing typed yet for this position,
// see ambiguousTokenIsEmpty's own doc comment in package completer for
// the fuller reasoning. That placeholder is never a real argument, so
// counting it as one would report a command such as plain "exit" as
// not runnable the moment a trailing space is typed after it, purely
// because Resolve() had nowhere else to put that placeholder token
// once "exit" itself, having no Subcommands, stopped command matching
// and folded every remaining token into Args. A real argument is
// never itself the empty string, tokenize.Tokenize and strings.Fields
// alike never produce one, so stripping at most one trailing empty
// string here is always safe.
func runnableAsIs(cmd *Command, args []string) bool {
	if cmd == nil || cmd.RunFunc == nil {
		return false
	}

	effective := args
	if n := len(effective); n > 0 && effective[n-1] == "" {
		effective = effective[:n-1]
	}

	if cmd.MinArgs != nil && len(effective) < *cmd.MinArgs {
		return false
	}
	if cmd.MaxArgs != nil && len(effective) > *cmd.MaxArgs {
		return false
	}
	return true
}

// intPtr - This function returns a pointer to the int passed in. It
// exists so a test can write MinArgs: intPtr(1) inline instead of
// needing a named variable for every argument constraint, since Go
// will not take the address of a literal directly. A production tree
// definition comes from YAML, loader.go, where yaml.v3 already
// unmarshals directly into *int fields without needing this helper.
func intPtr(n int) *int {
	return &n
}
