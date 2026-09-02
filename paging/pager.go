// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package paging

import (
	"fmt"
	"io"
	"os"

	"github.com/gotermme/routercli/i18n"

	"golang.org/x/term"
)

// EffectivePageLines returns how many lines of output Display shows
// before pausing. override is AppContext.PageLines, nil when no
// "terminal length" has ever been typed this session, in which case
// the real, live terminal height behind fd is used instead, one line
// less than what term.GetSize reports, reserved for the "--More--"
// prompt itself so a full page of content plus that prompt never
// together exceeds one real screen. fallback,
// config.SystemConfig.DefaultPageLines, is used only when fd is not a
// real terminal, or its size cannot be read for any other reason, the
// same case Display itself treats as "print everything, do not try
// to pause."
//
// A non-nil override is returned exactly as given, including zero,
// "terminal length 0", the real Cisco convention for "never pause,"
// with no further adjustment. An operator who explicitly asked for a
// specific page size gets exactly that many lines, not that many
// minus one for a prompt they did not ask this function to reserve
// room for.
func EffectivePageLines(fd int, override *int, fallback int) int {
	if override != nil {
		return *override
	}
	if _, h, err := term.GetSize(fd); err == nil && h > 1 {
		return h - 1
	}
	return fallback
}

// EffectiveTerminalWidth returns how wide this session's own terminal
// currently is, the same override, live-detected, fallback shape
// EffectivePageLines above already gives page height. override is
// AppContext.TerminalWidth, nil when no "terminal width" has ever been
// typed this session, in which case the real, live width behind fd is
// read fresh with term.GetSize, exactly the way EffectivePageLines
// itself reads a real terminal's height fresh on every call, so this
// function's own result already reflects a mid-session resize with no
// staleness of its own to correct, before "terminal width" has ever
// been typed. fallback, AppContext.DefaultTerminalWidth, is used only
// when fd is not a real terminal, piped or redirected stdin for
// instance, or its width genuinely cannot be read, the same case
// EffectivePageLines itself falls back for.
//
// A non-nil override is returned exactly as given, with no further
// adjustment, the same treatment EffectivePageLines gives a non-nil
// PageLines override.
func EffectiveTerminalWidth(fd int, override *int, fallback int) int {
	if override != nil {
		return *override
	}
	if w, _, err := term.GetSize(fd); err == nil && w > 0 {
		return w
	}
	return fallback
}

// Display writes lines to stdout, pausing with a translated
// "--More--" prompt, see the "pager.more_prompt" catalog key, once
// every pageLines lines, exactly the way a real Cisco or HP device's
// own pager behaves. Space shows the next full page. Enter or Return
// shows exactly one more line, then pauses again. "q", "Q", or Ctrl-C
// stops immediately, discarding whatever is left. Any other key is
// treated the same as space, matching real device behavior where
// almost any keypress simply advances a page.
//
// Pausing is skipped entirely, the whole of lines is written straight
// through with no prompt at all, in three cases: pagingEnabled is
// false, config.SystemConfig.PagingEnabled's own deployment wide
// switch; pageLines is zero or less, "terminal length 0" for this
// session, see EffectivePageLines; or fd is not a real terminal,
// piped or redirected stdin for instance, where there is no keyboard
// to read a keypress from and pausing forever would simply hang the
// session. len(lines) at or below pageLines also skips the prompt,
// with no special flag needed, since a real device shows no
// "--More--" at all for output that already fits on one screen.
func Display(stdout io.Writer, fd int, translator *i18n.Translator, lines []string, pageLines int, pagingEnabled bool) error {
	if !pagingEnabled || pageLines <= 0 || !term.IsTerminal(fd) || len(lines) <= pageLines {
		writeAll(stdout, lines)
		return nil
	}

	prompt := translator.T("pager.more_prompt")
	step := pageLines
	i := 0
	for i < len(lines) {
		end := i + step
		if end > len(lines) {
			end = len(lines)
		}
		writeAll(stdout, lines[i:end])
		i = end
		if i >= len(lines) {
			return nil
		}

		fmt.Fprint(stdout, prompt)
		action, err := readKeypress(fd)
		clearPrompt(stdout)
		if err != nil {
			// No further keypress could be read, for example the
			// terminal went away mid-pause. Print whatever is left
			// rather than leaving the session stuck at a prompt
			// nothing will ever answer.
			writeAll(stdout, lines[i:])
			return nil
		}

		switch action {
		case actionQuit:
			return nil
		case actionLine:
			step = 1
		default:
			step = pageLines
		}
	}
	return nil
}

// keyAction - This type is what one raw keypress read by
// readKeypress resolves to, one of actionPage, actionLine, or
// actionQuit.
type keyAction int

const (
	actionPage keyAction = iota
	actionLine
	actionQuit
)

// writeAll - This function prints every entry of lines to w, one per
// line. It is the plain, no pause fallback Display itself uses, and
// the shared building block its own paused loop prints each page
// through as well.
func writeAll(w io.Writer, lines []string) {
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
}

// clearPrompt - This function erases whatever prompt Display just
// printed, before the next page or line is written in its place.
// "\r" returns the cursor to the start of the current line, and the
// standard VT100/ANSI "\x1b[K" erase-to-end-of-line sequence clears
// everything after it, regardless of the real prompt's own length,
// the same escape sequence family cmd/core/cmd_totp.go's own
// ansiClearScreen already relies on for the same reason, honored by
// every mainstream terminal emulator in current wide use.
func clearPrompt(w io.Writer) {
	fmt.Fprint(w, "\r\x1b[K")
}

// readKeypress reads exactly one raw byte from fd, with the terminal
// placed into raw mode for just this one read and restored to
// whatever it was immediately after, regardless of which branch below
// is taken. This is done explicitly here, rather than assumed from
// whatever mode the surrounding session's own readline.Instance may
// already have left the terminal in between one command's dispatch
// and the next, so this function's own correctness never depends on
// that assumption holding.
//
// os.NewFile wraps fd without duplicating it, so the real, single
// underlying operating system file descriptor is what is actually
// read from. The wrapper itself is deliberately never closed; doing
// so would close the real stdin file descriptor out from under the
// rest of this session, since Close on a file built this way closes
// the descriptor it wraps, not just this one Go level handle to it.
func readKeypress(fd int) (keyAction, error) {
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return actionPage, err
	}
	defer term.Restore(fd, oldState)

	f := os.NewFile(uintptr(fd), "")
	buf := make([]byte, 1)
	if _, err := f.Read(buf); err != nil {
		return actionPage, err
	}

	switch buf[0] {
	case ' ':
		return actionPage, nil
	case '\r', '\n':
		return actionLine, nil
	case 'q', 'Q', 0x03:
		return actionQuit, nil
	default:
		return actionPage, nil
	}
}
