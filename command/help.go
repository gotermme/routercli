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

// SortCommandNames - This function orders names, every one of them
// assumed to be a key of tree, the way a listing of more than one
// command side by side should actually be presented, driven by opts.
// HelpText, and both of package completer's own print paths, the Tab
// completion ambiguous candidate list and "?"'s own word-help bare
// name list, funnel through this, so a listing's order is controlled
// by the same two settings no matter which of those three paths
// produced it, see ListOptions's own doc comment for what each one
// means.
//
// names is never mutated; a new, sorted slice is always returned.
//
// When opts.Alphabetical is true, the default, this sorts by name,
// except that when opts.MergeCommon is false, every command with
// IsCommonCommand true sorts after every command without it, still
// alphabetical within each of those two groups. When opts.Alphabetical
// is false, this sorts by DefIndex instead, but always with that same
// non-common-before-common grouping regardless of MergeCommon:
// DefIndex is only ever comparable between two commands that came from
// the very same source file, and MergeCommon's own "merged" form has
// no sensible meaning to give two separate files' own definition
// order, there is no true combined "definition order" across
// var/tree/level_common.yaml and a level's own tree file the way there
// is one true alphabetical order across both, so definition order
// always keeps them apart, own commands from a level's own tree file
// before common commands from level_common.yaml, the same way it
// already reads on the page for anyone comparing the two files
// directly.
//
// A name with no matching entry in tree, which should not happen in
// practice, sorts as neither common nor first by DefIndex, and simply
// compares by its own name against anything else in the same
// situation, so a caller passing a stale or mismatched tree still gets
// a stable, sensible order rather than a panic.
func SortCommandNames(names []string, tree map[string]*Command, opts ListOptions) []string {
	sorted := append([]string(nil), names...)

	sort.SliceStable(sorted, func(i, j int) bool {
		ni, nj := sorted[i], sorted[j]
		ci, cj := tree[ni], tree[nj]

		commonI := ci != nil && ci.IsCommonCommand
		commonJ := cj != nil && cj.IsCommonCommand

		// Definition order always separates the two groups, and so
		// does alphabetical order when common commands are being
		// appended rather than merged, see this function's own doc
		// comment for why. Alphabetical-and-merged, this project's own
		// default, is the one combination that never partitions here.
		if (!opts.Alphabetical || !opts.MergeCommon) && commonI != commonJ {
			return !commonI
		}

		if !opts.Alphabetical {
			di, dj := 0, 0
			if ci != nil {
				di = ci.DefIndex
			}
			if cj != nil {
				dj = cj.DefIndex
			}
			if di != dj {
				return di < dj
			}
		}

		return ni < nj
	})

	return sorted
}

// HelpText - This function builds a listing of the commands reachable
// at one level of the tree, the level passed in and nothing below it,
// skipping Hidden nodes entirely, since a hidden command must not
// appear here any more than it appears in tab completion. This lives in
// package command rather than in the "help" command's own file in
// cmd/core because it is a property of the tree data structure itself,
// not of any one command's behavior, so a different command set gets
// the same listing logic for free.
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
// and always ordered the same way between runs, since Go's map
// iteration order is randomized, driven by opts, see SortCommandNames,
// which does the actual ordering.
func HelpText(tree map[string]*Command, t *i18n.Translator, opts ListOptions) string {
	var names []string
	for name, cmd := range tree {
		if cmd.Hidden {
			continue
		}
		names = append(names, name)
	}
	names = SortCommandNames(names, tree, opts)

	entries := make([]helpEntry, len(names))
	for i, name := range names {
		entries[i] = helpEntry{name, tree[name].ResolvedDesc(t)}
	}

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
// A "<cr>" line, always last, never sorted in among any other names,
// is appended to either of the first two cases whenever res.Command
// itself, the container these candidates or subcommands belong to, is
// already directly runnable as typed so far, see
// ResolveResult.RunnableAsIs. This is the same "totp enable" versus
// "totp enable qr" situation Resolve()'s own doc comment describes:
// "totp enable ?" needs to show both "qr" and "<cr>" together, since
// pressing Enter right there already runs a complete command, but
// there is also more that could still be typed.
//
// opts controls how more than one name is ordered within either
// listing, see ListOptions's own doc comment and SortCommandNames,
// which does the actual ordering; "<cr>" itself is never subject to
// opts, it is always last.
//
// This returns an empty string if the tokens do not resolve to anything
// at all, such as "bogus-command ?". A caller treats that as nothing to
// show.
func HelpForPath(tree map[string]*Command, tokens []string, t *i18n.Translator, opts ListOptions) string {
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
		if ambigIdx >= 0 && ambigIdx < len(tokens) && tokens[ambigIdx] == "" && res.AmbiguousTree != nil {
			// Full help, nothing typed yet for this position: the full,
			// described HelpText of res.AmbiguousTree, the very
			// container res.Ambiguous's own candidates were drawn from,
			// see ResolveResult.AmbiguousTree's own doc comment, instead
			// of the bare names res.Ambiguous itself carries.
			text := HelpText(res.AmbiguousTree, t, opts)
			if res.RunnableAsIs {
				text += "  <cr>\n"
			}
			return text
		}
		// Word help, a genuinely partial word: bare candidate names
		// with no descriptions, matching real Cisco's own word help
		// form and this project's existing tab completion convention
		// for the same situation, see completer.go's own Ambiguous
		// branch.
		names := SortCommandNames(res.Ambiguous, res.AmbiguousTree, opts)
		var b strings.Builder
		for _, c := range names {
			fmt.Fprintf(&b, " %s\n", c)
		}
		if res.RunnableAsIs {
			fmt.Fprintf(&b, " <cr>\n")
		}
		return b.String()
	case res.Command != nil && len(res.Command.Subcommands) > 0:
		text := HelpText(res.Command.Subcommands, t, opts)
		if res.RunnableAsIs {
			text += "  <cr>\n"
		}
		return text
	case res.Command != nil:
		hint := res.Command.ResolvedArgHelp(t)
		switch {
		case hint != "" && res.RunnableAsIs:
			// Optional argument: the command is already complete as
			// typed, but more could still follow, so both the hint and
			// "<cr>" are shown together, hint first, "<cr>" last,
			// matching real Cisco.
			return " " + hint + "\n <cr>\n"
		case hint != "":
			return " " + hint + "\n"
		case res.RunnableAsIs:
			return " <cr>\n"
		default:
			// MinArgs is not satisfied, and this command defines no
			// ArgHelp or ArgHelpKey of its own to hint with. Showing
			// "<cr>" here, the old behavior, would be misleading, since
			// pressing Enter right now is not actually a complete
			// command, so this shows nothing instead.
			return ""
		}
	default:
		return ""
	}
}
