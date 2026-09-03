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
//
// width is this session's own effective terminal width, the same value
// a caller already resolves through paging.EffectiveTerminalWidth for
// DetailedHelp, run through effectiveManPageWidth here too, so a zero
// or otherwise unusable value, a piped "help\n" with no real terminal
// behind it for instance, still falls back to the ordinary 80 column
// default rather than an unwrapped line. A description too long to fit
// next to its own command name on one line continues on the next,
// indented to line up under the description column rather than back at
// the left margin, so a session's own terminal never has to hard wrap
// it there itself. This mirrors wrapAndIndent's own hanging indent,
// only with the indent width set by this listing's own longest command
// name rather than a fixed section margin.
func HelpText(tree map[string]*Command, t *i18n.Translator, opts ListOptions, width int) string {
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

	// "  " before the name, the name itself padded to longest, then
	// "  " again before the description, matching the Fprintf format
	// string below exactly, is how wide a continuation line's own
	// leading indent needs to be to land right under where the
	// description column itself starts.
	indent := strings.Repeat(" ", 2+longest+2)
	descWidth := effectiveManPageWidth(width) - len(indent)
	if descWidth < 20 {
		descWidth = 20
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
		descLines := wrapText(e.desc, descWidth)
		fmt.Fprintf(&b, "  %-*s  %s\n", longest, e.path, descLines[0])
		for _, line := range descLines[1:] {
			fmt.Fprintf(&b, "%s%s\n", indent, line)
		}
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
// width is threaded straight through to HelpText, for the full help
// case below, see that function's own doc comment for what it does
// with it; the word help and single command cases print no column
// aligned listing of their own, so width has nothing to affect there.
//
// This returns an empty string if the tokens do not resolve to anything
// at all, such as "bogus-command ?". A caller treats that as nothing to
// show.
func HelpForPath(tree map[string]*Command, tokens []string, t *i18n.Translator, opts ListOptions, width int) string {
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
			text := HelpText(res.AmbiguousTree, t, opts, width)
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
		text := HelpText(res.Command.Subcommands, t, opts, width)
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

// defaultManPageWidth is the column width DetailedHelp's own header
// line and wrapped body text fall back to whenever the width a caller
// passes in is not a real, usable terminal width, the same 80 column
// convention classic Unix man pages themselves default to, both nroff
// and groff, when neither MANWIDTH nor a real terminal is available to
// size against. minManPageWidth is the floor below which a passed in
// width is treated as unusable rather than honored literally, so an
// oddly small or zero value, a piped "help <command>" with no real
// terminal behind it for instance, still produces a readable page
// instead of wrapping every word onto its own line.
const (
	defaultManPageWidth = 80
	minManPageWidth     = 40
)

// effectiveManPageWidth - This function returns width itself when it
// is at least minManPageWidth, and defaultManPageWidth otherwise. Both
// buildManPageHeader and the wrapping helpers below call this once on
// whatever width DetailedHelp itself was given, so a caller with no
// real terminal to measure, or one that simply passes zero, never
// needs its own fallback logic.
func effectiveManPageWidth(width int) int {
	if width < minManPageWidth {
		return defaultManPageWidth
	}
	return width
}

// manSectionIndent is how far a section's own body text is indented
// under its all caps header. Four columns, narrower than a real man
// page's own seven column tab stop, matches this project's own
// preference, see the project instructions this whole codebase
// follows, for compact, easy to read output over faithfully
// reproducing groff's exact metrics.
const manSectionIndent = "    "

// buildManPageHeader - This function builds DetailedHelp's own first
// line, name in all capitals mirrored on both the left and right
// margins with a centered title in between, matching a real Unix man
// page's own header line, its section number omitted since RouterCLI
// has no man page "section" concept of its own to show. name is the
// command's own full resolved path, "show running-config" for
// instance, already space joined the same way DetailedHelp's own body
// joins it. productName is the deployment's own configured display
// name, see config.SystemConfig.ProductName and
// AppContext.ProductName, falling back to "RouterCLI" itself, the
// same default config.DefaultSystemConfig already ships, whenever the
// caller passes an empty string, which every test in this package
// that does not care about branding does, and which a hand built
// AppContext in a cmd/core test, never setting ProductName itself,
// also does by leaving the field at its own zero value. width is run
// through effectiveManPageWidth first, so this never needs its own
// fallback for an unusable value.
//
// The title, productName followed by "Help Information", is centered
// in whatever space is left over after both copies of name, an odd
// remaining column going to the right hand side, so "HOSTNAME"
// centered against "RouterCLI Help Information" inside width reads
// exactly the way the classic, mirrored man page header this project
// modeled its own after does. A name long enough that both copies
// together with the title would not fit inside width falls back to a
// single space of padding on each side instead of a negative,
// nonsensical gap, so an unusually long command path never panics or
// produces a mangled line, only a header wider than width itself.
func buildManPageHeader(name, productName string, width int) string {
	if productName == "" {
		productName = "RouterCLI"
	}
	width = effectiveManPageWidth(width)
	upper := strings.ToUpper(name)
	title := productName + " Help Information"

	pad := width - len(upper)*2 - len(title)
	if pad < 2 {
		return upper + " " + title + " " + upper
	}
	leftPad := pad / 2
	rightPad := pad - leftPad
	return upper + strings.Repeat(" ", leftPad) + title + strings.Repeat(" ", rightPad) + upper
}

// indentManBody - This function prefixes every line of body with
// indent, with no rewrapping of its own, so the SUBCOMMANDS section's
// own column aligned listing, see subcommandDetailLines, reads as a
// properly indented block under its own all caps section header
// without disturbing the column alignment a rewrap would break. A
// blank line is left blank rather than padded with trailing
// whitespace of its own. body's own trailing newline, if it has one,
// is trimmed first so this never doubles it.
//
// NAME, SYNOPSIS, and DESCRIPTION, prose rather than a table, use
// wrapAndIndent and wrapAndIndentParagraphs below instead, which
// rewrap their own text to a given width before indenting it.
func indentManBody(body, indent string) string {
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	var b strings.Builder
	for _, line := range lines {
		if line == "" {
			fmt.Fprintln(&b)
			continue
		}
		fmt.Fprintf(&b, "%s%s\n", indent, line)
	}
	return b.String()
}

// wrapText - This function breaks text, a single logical run of
// words with no paragraph structure of its own, into lines no wider
// than width, breaking only at whitespace the way any ordinary word
// wrap does, never hyphenating or otherwise splitting a word itself.
// A single word longer than width is still placed on its own line
// rather than truncated or forced to overflow onto a second one, so
// this never panics or loses content, only occasionally produces one
// line wider than requested. Field splitting through strings.Fields
// also collapses any run of whitespace in text, including an embedded
// newline, into the single space between words a rewrapped line
// needs, so a caller does not need to normalize text first.
func wrapText(text string, width int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return nil
	}

	lines := []string{words[0]}
	for _, word := range words[1:] {
		last := lines[len(lines)-1]
		if len(last)+1+len(word) > width {
			lines = append(lines, word)
			continue
		}
		lines[len(lines)-1] = last + " " + word
	}
	return lines
}

// wrapAndIndent - This function wraps text, a single prose line such
// as DetailedHelp's own NAME or SYNOPSIS line, to width, run through
// effectiveManPageWidth first the same way buildManPageHeader's own
// width is, minus indent's own length, then prefixes every wrapped
// line with indent, so a continuation line lands under the section
// body's own left margin instead of back at column zero the way an
// unwrapped, merely indented first line left it for a session's own
// terminal to wrap on its own. A remaining width narrower than 20
// columns, indent itself unusually wide relative to width, floors to
// 20 rather than wrapping one word per line.
func wrapAndIndent(text, indent string, width int) string {
	inner := effectiveManPageWidth(width) - len(indent)
	if inner < 20 {
		inner = 20
	}

	var b strings.Builder
	for _, line := range wrapText(text, inner) {
		fmt.Fprintf(&b, "%s%s\n", indent, line)
	}
	return b.String()
}

// wrapAndIndentParagraphs - This function wraps and indents body, a
// longer, possibly multi paragraph Help body, the same way
// wrapAndIndent does a single line, except that a blank line inside
// body is honored as a genuine paragraph break: each paragraph is
// rewrapped independently, through wrapAndIndent, and paragraphs are
// separated by one blank line of their own in the result. A
// paragraph's own internal line breaks, however body happened to be
// typed in its source YAML file, are collapsed first, through
// wrapText's own strings.Fields call, so every paragraph rewraps
// cleanly at this deployment's own width rather than keeping
// whatever arbitrary breaks the source file's own line length left
// it with.
func wrapAndIndentParagraphs(body, indent string, width int) string {
	paragraphs := strings.Split(strings.TrimRight(body, "\n"), "\n\n")

	var b strings.Builder
	for i, p := range paragraphs {
		if i > 0 {
			fmt.Fprintln(&b)
		}
		fmt.Fprint(&b, wrapAndIndent(p, indent, width))
	}
	return b.String()
}

// DetailedHelp - This function builds a man page style description of
// one specific command, everything RouterCLI knows about it in one
// place, rather than only the "what can I type next" answer
// HelpForPath gives. This is what `help <command>` in cmd/core's
// "help" command calls; HelpForPath, and the interactive "?" and Tab
// completion paths built on it, are unaffected by this function's
// existence and keep answering their own, different question exactly
// as before. tokens is the command path already split, "show",
// "running-config" for instance, the same shape HelpForPath itself
// takes. productName is the deployment's own configured display name,
// see AppContext.ProductName, threaded straight through to
// buildManPageHeader, which falls back to "RouterCLI" for an empty
// string; every caller in this project's own test suite that does not
// care about branding passes an empty string for exactly that reason.
// width is this session's own effective terminal width, see
// paging.EffectiveTerminalWidth, the same live detection "show
// terminal" already reports, which cmd/core/cmd_help.go's "help"
// handler computes and passes in; DetailedHelp itself stays free of
// any dependency on package paging or a real terminal file
// descriptor, and simply runs whatever it is given through
// effectiveManPageWidth, so zero, a piped "help <command>" with no
// real terminal behind it for instance, still produces a readable,
// conventionally wide page rather than a degenerate one.
//
// A command's own detail block opens with one blank line, then a man
// page style header line, see buildManPageHeader, then another blank
// line, then a series of all caps section headers with their own body
// text wrapped to width and indented beneath them, see wrapAndIndent,
// wrapAndIndentParagraphs, and manSectionIndent, matching a real Unix
// man page's own layout rather than this function's own earlier,
// flatter form, and closes with one further blank line of its own, so
// this whole block reads as one visually separated unit between the
// command that asked for it and whatever prompt or output follows.
//
// NAME always appears, and holds the command's own full resolved
// name, a hyphen, and its ResolvedDesc, or the bare name alone when no
// description is set, matching HelpText's own bare-name fallback.
// SYNOPSIS follows, only when the command actually takes an argument,
// and holds the full name followed by ResolvedArgHelp, the same
// content the older, flatter "Usage:" line held, renamed to match real
// man page convention. DESCRIPTION follows, only when ResolvedHelp is
// actually set, most commands have none today, and holds that
// command's own longer, free-form body, rewrapped to width, a blank
// line inside it honored as a real paragraph break. SUBCOMMANDS
// follows last, only when the command has subcommands, and holds
// subcommandDetailLines's own one level deep, column aligned listing,
// indented but not rewrapped, matching HelpText's own established
// convention of never recursing further than that. A command that is
// itself already runnable and also has subcommands, "totp enable" for
// example, gets both its own SYNOPSIS section and the SUBCOMMANDS
// section together.
//
// tokens resolving ambiguously, a partial or abbreviated name matching
// more than one command, prints the matching candidate names instead
// of a detail block, the same real candidates "?" would show for the
// same partial word, so a session sees what to narrow down to rather
// than a bare refusal; there is no single command to build a header
// line for here, so none is printed, and neither of the two framing
// blank lines is added. tokens resolving to nothing at all returns an
// empty string, the same "nothing to show" convention HelpForPath
// already uses, leaving it to the caller, cmd/core/cmd_help.go's
// "help" handler, to turn that into a real, translated error.
func DetailedHelp(tree map[string]*Command, tokens []string, t *i18n.Translator, opts ListOptions, productName string, width int) string {
	res := Resolve(tree, tokens)

	if len(res.Ambiguous) > 0 {
		names := SortCommandNames(res.Ambiguous, res.AmbiguousTree, opts)
		var b strings.Builder
		header := "Ambiguous; did you mean one of the following:"
		if t != nil {
			header = t.T("help.ambiguous")
		}
		fmt.Fprintln(&b, header)
		for _, name := range names {
			fmt.Fprintf(&b, "  %s\n", name)
		}
		return b.String()
	}

	if res.Command == nil {
		return ""
	}

	name := strings.Join(res.FullName, " ")
	var b strings.Builder

	nameHeader, synopsisHeader, descHeader, subcommandsHeader :=
		"NAME", "SYNOPSIS", "DESCRIPTION", "SUBCOMMANDS"
	if t != nil {
		nameHeader = t.T("help.section.name")
		synopsisHeader = t.T("help.section.synopsis")
		descHeader = t.T("help.section.description")
		subcommandsHeader = t.T("help.section.subcommands")
	}

	fmt.Fprintln(&b)
	fmt.Fprintln(&b, buildManPageHeader(name, productName, width))
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, nameHeader)
	nameLine := name
	if desc := res.Command.ResolvedDesc(t); desc != "" {
		nameLine = name + " - " + desc
	}
	fmt.Fprint(&b, wrapAndIndent(nameLine, manSectionIndent, width))

	if hint := res.Command.ResolvedArgHelp(t); hint != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, synopsisHeader)
		fmt.Fprint(&b, wrapAndIndent(name+" "+hint, manSectionIndent, width))
	}

	if help := res.Command.ResolvedHelp(t); help != "" {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, descHeader)
		fmt.Fprint(&b, wrapAndIndentParagraphs(help, manSectionIndent, width))
	}

	if len(res.Command.Subcommands) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, subcommandsHeader)
		fmt.Fprint(&b, indentManBody(subcommandDetailLines(res.Command.Subcommands, t, opts), manSectionIndent))
	}

	fmt.Fprintln(&b)

	return b.String()
}

// subcommandDetailLines - This function renders the one level deep,
// name-and-description-and-usage-hint listing DetailedHelp's own
// Subcommands section uses, a skipped-Hidden, column aligned on name
// listing in the same spirit as HelpText, but with each subcommand's
// own ResolvedArgHelp appended in parentheses when it has one, which
// HelpText's own plain listing never shows. This lives as its own
// function, rather than inline in DetailedHelp, purely for that
// function's own readability; nothing else calls this today.
//
// Each line carries no left margin of its own. DetailedHelp's own
// caller applies manSectionIndent through indentManBody, the same
// section margin NAME, SYNOPSIS, and DESCRIPTION already get, so a
// margin built in here as well would double it, six columns instead
// of four.
func subcommandDetailLines(subcommands map[string]*Command, t *i18n.Translator, opts ListOptions) string {
	var names []string
	for name, cmd := range subcommands {
		if cmd.Hidden {
			continue
		}
		names = append(names, name)
	}
	names = SortCommandNames(names, subcommands, opts)

	longest := 0
	for _, name := range names {
		if len(name) > longest {
			longest = len(name)
		}
	}

	var b strings.Builder
	for _, name := range names {
		cmd := subcommands[name]
		desc := cmd.ResolvedDesc(t)
		hint := cmd.ResolvedArgHelp(t)
		switch {
		case desc == "" && hint == "":
			fmt.Fprintf(&b, "%s\n", name)
		case hint == "":
			fmt.Fprintf(&b, "%-*s  %s\n", longest, name, desc)
		case desc == "":
			fmt.Fprintf(&b, "%-*s  (%s)\n", longest, name, hint)
		default:
			fmt.Fprintf(&b, "%-*s  %s  (%s)\n", longest, name, desc, hint)
		}
	}
	return b.String()
}
