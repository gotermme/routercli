// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package completer

import (
	"fmt"
	"strings"

	"github.com/gotermme/routercli/command"

	"github.com/chzyer/readline"
)

// ----------------------------------------------------------------------
// Public Methods - NoopCompleter
// ----------------------------------------------------------------------

func (NoopCompleter) Do(line []rune, pos int) (newLine [][]rune, length int) {
	return nil, 0
}

// ----------------------------------------------------------------------
// Public Methods - TreeListener
// ----------------------------------------------------------------------

// SetPrompt - This method records the current prompt so TreeListener can
// prefix its own output with it, the help listing, the ambiguous
// candidate list, and the argument hint, so those lines read as
// following the real prompt rather than appearing detached from it.
// TreeListener keeps its own copy rather than reading it back from the
// readline.Instance because readline exposes no getter for the prompt it
// was last given.
//
// The caller is responsible for calling this every time it also calls
// rl.SetPrompt(), main.go does so right alongside it after every
// dispatch, since a command may have changed the mode or elevation state
// and left the two out of sync otherwise.
func (l *TreeListener) SetPrompt(prompt string) {
	l.currentPrompt = prompt
}

// OnChange - This method implements readline.Listener, called on every
// keypress with the buffer state as readline itself already handled it.
// TreeListener uses this instead of readline's own AutoComplete
// interface for the reasons explained in the type's own doc comment
// above, only OnChange can return a full replacement buffer rather than
// an inserted suffix, which abbreviation expansion needs.
//
// A '?' key delegates to handleHelp. Any other non-Tab key resets the
// double Tab state below and returns ok as false. A Tab key resolves the
// tokens typed so far against the current Command Level's tree, then
// either rewrites the buffer to the resolved command, adding a trailing
// space and an argument hint where appropriate, or, when the resolution
// is ambiguous, tracks repeated Tab presses on the same input so the
// candidate list only prints on the second consecutive press, matching
// Cisco and HP ProCurve behavior. When ok is true, readline replaces its
// buffer with newLine and moves the cursor to newPos.
func (l *TreeListener) OnChange(line []rune, pos int, key rune) (newLine []rune, newPos int, ok bool) {
	if key == '?' {
		return l.handleHelp(line, pos)
	}
	if key != readline.CharTab {
		// Any non-Tab key breaks a double Tab sequence in progress.
		l.tapCount = 0
		l.lastAmbiguousInput = ""
		return nil, 0, false
	}

	typed := string(line[:pos])
	trailingSpace := strings.HasSuffix(typed, " ")
	tokens := strings.Fields(typed)
	if len(tokens) == 0 {
		// An empty line, Tab at a bare prompt, lists every top-level
		// command. This is always shown immediately, since there is
		// nothing ambiguous about it needing a confirmation tap.
		tokens = []string{""}
	} else if trailingSpace {
		// strings.Fields silently drops trailing whitespace, but a
		// trailing space is meaningful here: it means list what can
		// come next.
		tokens = append(tokens, "")
	}

	res := command.Resolve(l.position.Current().Tree, tokens)

	// "no " is preserved as a literal prefix on the rewritten line in
	// both branches below. Resolve() strips it internally to find the
	// real command, but the user should still see it on screen exactly
	// as typed, with the rest of the line completing normally after
	// it. See command.Resolve's own doc comment for the full "no"
	// mechanism this is rendering the result of.
	noPrefix := ""
	if res.Negated {
		noPrefix = "no "
	}

	if len(res.Ambiguous) > 0 {
		newBuf := ambiguousRewriteBuffer(tokens, res, noPrefix)

		if typed == l.lastAmbiguousInput {
			l.tapCount++
		} else {
			l.tapCount = 1
			l.lastAmbiguousInput = typed
		}
		l.logger.Debugln("DEBUG: ambiguous completion for", tokens, "tap", l.tapCount)

		// Nothing typed yet for this token, a bare prompt or right
		// after a trailing space, lists immediately, no confirmation
		// tap needed, see ambiguousTokenIsEmpty's own doc comment. A
		// genuinely ambiguous partial word still needs the second Tab.
		if ambiguousTokenIsEmpty(tokens, res) || l.tapCount >= 2 {
			var list strings.Builder
			list.WriteString(l.currentPrompt)
			list.WriteString(typed)
			list.WriteString("\n")
			for _, candidate := range res.Ambiguous {
				list.WriteString(" ")
				list.WriteString(candidate)
				list.WriteString("\n")
			}
			fmt.Fprint(l.instance.Stdout(), list.String())
		}

		if newBuf == typed {
			// Nothing to expand, so do not fight the cursor on repeat taps.
			return nil, 0, false
		}
		return []rune(newBuf), len([]rune(newBuf)), true
	}

	// Not ambiguous, so any double Tab sequence in progress is no
	// longer relevant.
	l.tapCount = 0
	l.lastAmbiguousInput = ""

	resolvedLine := noPrefix + strings.Join(res.FullName, " ")
	if len(res.Args) > 0 {
		if resolvedLine != "" {
			resolvedLine += " "
		}
		resolvedLine += strings.Join(res.Args, " ")
	}

	// Once every token on the line has been fully and unambiguously
	// resolved, meaning there is nothing left in res.Args and the user
	// is not mid-way through typing anything else, add a trailing
	// space regardless of whether the user had typed one yet. This
	// lets the next Tab press immediately show what comes next.
	if len(res.Args) == 0 && resolvedLine != "" && !strings.HasSuffix(resolvedLine, " ") {
		resolvedLine += " "
	}
	rest := string(line[pos:])
	full := resolvedLine + rest

	// Argument help hint
	if trailingSpace && len(res.Args) > 0 && res.Args[len(res.Args)-1] == "" &&
		res.Command != nil && res.Command.RunFunc != nil && len(res.Command.Subcommands) == 0 && res.Command.MinArgs != nil {
		hint := res.Command.ResolvedArgHelp(l.translator)
		if hint != "" {
			fmt.Fprint(l.instance.Stdout(), l.currentPrompt+typed+"\n "+hint+"\n")
		}
	}

	if full == string(line) {
		return nil, 0, false
	}

	l.logger.Debugln("DEBUG: expanding", typed, "to", resolvedLine)
	return []rune(full), len([]rune(resolvedLine)), true
}

// ----------------------------------------------------------------------
// Private Methods - TreeListener
// ----------------------------------------------------------------------

// handleHelp - This method implements real Cisco and HP "?" behavior: pressing "?"
// immediately shows contextual help, with no Enter needed, and the "?"
// character itself never actually ends up part of the command line, the
// same as on a real device. By the time this is called, readline has
// already inserted the "?" rune into line at pos-1. Unlike Tab, which
// readline never inserts a literal character for, "?" is an ordinary
// printable keypress from readline's point of view, so there is a real
// "?" sitting in the buffer that has to be stripped back out before
// returning it, otherwise every "?" press would leave a literal question
// mark behind on the line. See helpTokensAndRestoredBuffer for that part.
//
// This reuses command.HelpForPath, the same contextual help logic the
// non-interactive runLoop fallback uses for a piped "show ?\n", see main.go's
// runLoop. That path exists separately because readline only calls this
// Listener for a real terminal, since piped or scripted input never
// generates keypress events at all.
//
// Only the full help, trailing space, and bare prompt forms are
// implemented currently, matching the literal "show ?" case. "sh?", a
// partial word with no space before "?", falls through to whatever
// HelpForPath resolves the current, still partial tokens to, which in
// practice behaves like full help one level too high rather than real
// Cisco's distinct word help, bare matching names with no descriptions.
// That is a reasonable stand-in for now, not a deliberate design choice,
// and is worth revisiting if that distinction matters in practice.
func (l *TreeListener) handleHelp(line []rune, pos int) (newLine []rune, newPos int, ok bool) {
	// Any double Tab sequence in progress is no longer relevant, the
	// same reasoning as the non-Tab branch below.
	l.tapCount = 0
	l.lastAmbiguousInput = ""

	tokens, restored, restoredPos := helpTokensAndRestoredBuffer(line, pos)

	help := command.HelpForPath(l.position.Current().Tree, tokens, l.translator)
	if help != "" {
		fmt.Fprint(l.instance.Stdout(), l.currentPrompt+string(line[:pos-1])+"?\n"+help)
	}

	return restored, restoredPos, true
}

// ----------------------------------------------------------------------
// Private Functions - TreeListener
// ----------------------------------------------------------------------

// ambiguousRewriteBuffer - This function computes the rewritten line buffer for an
// ambiguous match. It must include the ambiguous token itself, exactly as
// typed, not just the already resolved prefix before it. res.FullName only
// covers tokens that resolved without ambiguity, so when the ambiguity is
// on the very first token, for example "en" matching both "enable" and
// "end", FullName is empty, and a naive implementation that only used
// FullName would produce an empty buffer, wiping out whatever the user had
// typed. res.AmbigAt indexes into the tokens Resolve() actually walked,
// which is tokens[1:] rather than tokens when Negated, since "no" itself
// was stripped before Resolve() recursed, so translating that back to an
// index into the original tokens slice needs one added in that case.
//
// This is separated out from OnChange so it can be unit tested without a
// real readline.Instance. OnChange itself needs one, to print the
// candidate list on a double-tap, but this function needs none of that.
func ambiguousRewriteBuffer(tokens []string, res command.ResolveResult, noPrefix string) string {
	resolvedPrefix := strings.Join(res.FullName, " ")

	ambigIdx := res.AmbigAt
	if res.Negated {
		ambigIdx++
	}
	ambiguousToken := ""
	if ambigIdx >= 0 && ambigIdx < len(tokens) {
		ambiguousToken = tokens[ambigIdx]
	}

	newBuf := noPrefix + resolvedPrefix
	if resolvedPrefix != "" {
		newBuf += " "
	}
	newBuf += ambiguousToken
	return newBuf
}

// ambiguousTokenIsEmpty - This function reports whether the token at the ambiguity,
// res.AmbigAt, adjusted for a leading "no" exactly like
// ambiguousRewriteBuffer does, is the empty string, meaning the user has
// not typed anything yet for this position and just hit Tab right after a
// space, or at a bare prompt. This is the boundary between two genuinely
// different situations that both surface as ambiguous in ResolveResult,
// and which a real Cisco or HP device treats differently. If nothing is
// typed yet, for example "show" followed by Tab, or Tab at a bare prompt,
// there is no partial word to be uncertain about completing further, so
// the device lists what is available on the very first Tab. If a partial
// word could grow into more than one command, for example "s" followed by
// Tab, which could become show or set, a single Tab should not immediately
// dump a list, since the user may still be about to type more of it. That
// is what the double Tab confirmation, see tapCount below, exists for.
//
// This is separated out for the same reason ambiguousRewriteBuffer is:
// pure logic, easy to unit test without a real readline.Instance.
func ambiguousTokenIsEmpty(tokens []string, res command.ResolveResult) bool {
	ambigIdx := res.AmbigAt
	if res.Negated {
		ambigIdx++
	}
	return ambigIdx >= 0 && ambigIdx < len(tokens) && tokens[ambigIdx] == ""
}

// helpTokensAndRestoredBuffer - This function computes the token path to look up help
// for, and the buffer and cursor readline should be left with, from a
// handleHelp call. before and after split the original buffer, before this
// keypress, at the point where "?" was inserted, see handleHelp's own doc
// comment for why pos-1, not pos, is that boundary.
//
// This is separated out from handleHelp itself so it can be unit tested
// without a real *readline.Instance, since chzyer/readline's Instance
// requires actual raw-mode terminal control to construct, unavailable in
// this test environment, and handleHelp needs one only to print through.
func helpTokensAndRestoredBuffer(line []rune, pos int) (tokens []string, restored []rune, restoredPos int) {
	before := line[:pos-1]
	after := line[pos:]
	typed := string(before)

	tokens = strings.Fields(typed)
	if strings.HasSuffix(typed, " ") || len(tokens) == 0 {
		tokens = append(tokens, "")
	}

	restored = make([]rune, 0, len(before)+len(after))
	restored = append(restored, before...)
	restored = append(restored, after...)
	return tokens, restored, pos - 1
}
