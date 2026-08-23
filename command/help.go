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
// Public Functions - Help
// ----------------------------------------------------------------------

// HelpText - This function builds a listing of the commands reachable
// at one level of the tree, the level passed in and nothing below it,
// skipping Hidden nodes entirely, since a hidden command must not
// appear here any more than it appears in tab completion. This lives in
// package command rather than in the "help" command's own file in
// package cmd because it is a property of the tree data structure
// itself, not of any one command's behavior, so a different command set
// gets the same listing logic for free.
//
// This is deliberately one level deep, not a recursive walk of the
// whole tree, matching real Cisco and HP behavior. "help" at the top
// level shows "show" as a single line with its own description, not
// "show version", "show running-config", and "show interface" spelled
// out individually. A container command such as "show" is shown using
// its own Desc or DescKey. If a tree file entry defines a container
// with no description at all, the path is printed bare rather than
// silently omitted, so a forgotten description is visible as a gap in
// the output instead of a missing command entirely. To see what is
// under a container, the user descends into it, for example by typing
// "show", and calls "help" again from there. The discovery mechanism
// for a container's own children is tab completion, not "help" listing
// them recursively.
//
// t may be nil, in which case descriptions fall back to each Command's
// literal Desc field, see Command.ResolvedDesc. Passing a real Translator
// resolves DescKey based descriptions to the active language, and
// translates the "Available commands:" header itself through the
// "help.header" key.
//
// Output is column aligned on the command name so descriptions line up,
// and sorted alphabetically so the listing stays stable between runs,
// since Go's map iteration order is randomized.
func HelpText(tree map[string]*Command, t *i18n.Translator) string {
	var entries []helpEntry
	for name, cmd := range tree {
		if cmd.Hidden {
			continue
		}
		entries = append(entries, helpEntry{name, cmd.ResolvedDesc(t)})
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })

	longest := 0
	for _, e := range entries {
		if len(e.path) > longest {
			longest = len(e.path)
		}
	}

	var b strings.Builder
	header := "Available commands:"
	if t != nil {
		header = t.T("help.header")
	}
	fmt.Fprintln(&b, header)
	for _, e := range entries {
		if e.desc == "" {
			fmt.Fprintf(&b, "  %s\n", e.path)
			continue
		}
		fmt.Fprintf(&b, "  %-*s  %s\n", longest, e.path, e.desc)
	}
	return b.String()
}

// HelpForPath - This function builds the same contextual listing a real
// Cisco or HP "?" produces, see completer.TreeListener's key == '?'
// branch, as a pure function over an already split token path. This
// exists for anywhere that needs it without a live readline.Instance,
// specifically main.go's runLoop non-interactive fallback, since a
// piped "show ?\n" has no real terminal to intercept a raw '?' keypress
// on. tokens is the command line already split, without the trailing
// "?" itself. The caller strips that, since what counts as the trailing
// token being literally "?" differs between a raw keypress and a
// tokenized line.
//
// There are three cases, matching real Cisco and HP "?" behavior. If
// tokens resolve ambiguously, including the bare or empty token case,
// which is always ambiguous against every top-level name, this lists
// the plain candidate names with no descriptions, matching real Cisco's
// word help form. If tokens resolve to a container command, one with
// subcommands, this returns the full HelpText() of its subcommands with
// descriptions, matching real Cisco's full help form, the literal
// "show ?" case. If tokens resolve to a leaf command, this returns that
// command's own ArgHelp or ArgHelpKey hint if it takes an argument, or
// "<cr>" if not, matching real Cisco's own notation for "you can press
// enter here".
//
// This returns an empty string if the tokens do not resolve to anything
// at all, such as "bogus-command ?". A caller treats that as nothing to
// show.
func HelpForPath(tree map[string]*Command, tokens []string, t *i18n.Translator) string {
	if len(tokens) == 0 {
		tokens = []string{""}
	}
	res := Resolve(tree, tokens)

	switch {
	case len(res.Ambiguous) > 0:
		// Resolve() reports "show " (a trailing empty token right after
		// a real container) as ambiguous against every one of that
		// container's children, the same as a genuine half typed word
		// such as "sh". Real Cisco and HP treat these two situations
		// differently for "?", see this function's own doc comment. The
		// distinguishing signal is whether the ambiguous token itself is
		// empty, the same check completer.go's ambiguousTokenIsEmpty
		// makes for tab completion.
		ambigIdx := res.AmbigAt
		if res.Negated {
			ambigIdx++
		}
		if ambigIdx >= 0 && ambigIdx < len(tokens) && tokens[ambigIdx] == "" {
			// Full help, nothing typed yet for this position. Walk
			// res.FullName, everything already resolved and
			// unambiguous, down from tree to find the container these
			// candidates actually belong to, or tree itself if
			// FullName is empty, the bare prompt case, and list its
			// subcommands with descriptions through HelpText, instead
			// of the bare names res.Ambiguous itself carries.
			container := tree
			for _, name := range res.FullName {
				cmd, ok := container[name]
				if !ok || cmd == nil {
					container = nil
					break
				}
				container = cmd.Subcommands
			}
			if container != nil {
				return HelpText(container, t)
			}
		}
		// Word help, a genuinely partial word: bare candidate names
		// with no descriptions, matching real Cisco's own word help
		// form and this project's existing tab completion convention
		// for the same situation, see completer.go's own Ambiguous
		// branch.
		var b strings.Builder
		for _, c := range res.Ambiguous {
			fmt.Fprintf(&b, " %s\n", c)
		}
		return b.String()
	case res.Command != nil && len(res.Command.Subcommands) > 0:
		return HelpText(res.Command.Subcommands, t)
	case res.Command != nil:
		hint := res.Command.ResolvedArgHelp(t)
		if hint == "" {
			hint = "<cr>"
		}
		return " " + hint + "\n"
	default:
		return ""
	}
}
