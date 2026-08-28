// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auth

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gotermme/routercli/i18n"

	"golang.org/x/term"
)

// ----------------------------------------------------------------------
// Public Functions - MFA
// ----------------------------------------------------------------------

// SecondFactorRequired - This function reports whether u has any
// second factor configured, checked at login, see login.go's
// PromptLogin, right after the password verifies. This is the one
// place that needs to know about every second factor method that
// exists. Adding a new method later, such as FIDO2 or U2F, means
// adding its own "is it configured" check here and its own branch in
// VerifySecondFactor below, without touching PromptLogin or any other
// call site at all.
func SecondFactorRequired(u *User) bool {
	return u.TOTPSecret != ""
}

// VerifySecondFactor - This function prompts for and checks whichever
// second factor u actually has configured. Only TOTP exists today.
// This function is the seam a future method, most likely FIDO2 or
// U2F, plugs into. See SecondFactorRequired's doc comment. It returns
// false, never true, if SecondFactorRequired(u) was false, which
// callers are expected to check first. This function does not
// re-derive whether the user needs a second factor at all, only
// whether the one they have configured checks out.
//
// reader is the same *bufio.Reader that PromptLogin already wraps
// stdin in for reading the username. It is deliberately not a fresh
// io.Reader passed in separately, since wrapping the same underlying
// stream in a second, independent bufio.Reader risks losing bytes the
// first one already buffered ahead. fd is used only for the masked
// input path, since term.ReadPassword needs a real terminal file
// descriptor, not a Reader.
func VerifySecondFactor(w io.Writer, reader *bufio.Reader, fd int, u *User, t *i18n.Translator) bool {
	if u.TOTPSecret != "" {
		return promptAndVerifyTOTP(w, reader, fd, u, t)
	}
	return false
}

// VerifySecondFactorCode - This function checks code against
// whichever second factor u actually has configured, given a code
// already read from wherever the caller got it, a masked terminal
// prompt, a test fixture, or otherwise. It performs no I/O of its
// own, the pure counterpart to VerifySecondFactor above, for a caller
// such as cmd/core/cmd_password.go's password change command that already
// runs its own retry loop around a masked prompt and only needs to
// check an already-read code, not have this function prompt for one
// itself. now is threaded through as a parameter rather than read
// with time.Now() internally, the same reason VerifyTOTPCode takes
// it, so a test can pass a fixed instant alongside a code generated
// for that same instant. It returns false, never true, if
// SecondFactorRequired(u) was false, mirroring VerifySecondFactor's
// own contract. See SecondFactorRequired's doc comment for how a
// future second factor method plugs into this same dispatch.
func VerifySecondFactorCode(u *User, code string, now time.Time) bool {
	if u.TOTPSecret != "" {
		return VerifyTOTPCode(u.TOTPSecret, code, now)
	}
	return false
}

// PromptTOTPCode - This function reads a single six-digit TOTP code,
// masked the same as a password, with no *User association of its
// own and no verification, leaving that to the caller. This is the
// standalone counterpart to promptAndVerifyTOTP below, used by
// anything that already knows which secret to check a code against
// outside the login flow, such as the totp enable and totp disable
// commands in package core (cmd/core), rather than deriving that
// secret from a matched login attempt the way PromptLogin does. Unlike
// promptAndVerifyTOTP, this has no bufio.Reader fallback for a
// non-terminal fd, since every real caller of this function already
// runs inside main.go's interactive runLoop with a genuine terminal
// file descriptor, the same assumption PromptSecret in login.go
// already makes.
func PromptTOTPCode(w io.Writer, fd int, t *i18n.Translator) (string, error) {
	fmt.Fprint(w, promptText(t, "auth.totp_prompt", "TOTP code: "))
	codeBytes, err := term.ReadPassword(fd)
	fmt.Fprintln(w) // ReadPassword does not echo the newline the user typed.
	if err != nil {
		return "", err
	}
	return string(codeBytes), nil
}

// ----------------------------------------------------------------------
// Private Functions - MFA
// ----------------------------------------------------------------------

// promptAndVerifyTOTP - This function reads a six-digit code, masked
// the same as a password, since there is no reason to show it over a
// shoulder either, and checks it against u.TOTPSecret with the
// standard clock-skew tolerance. See VerifyTOTPCode.
func promptAndVerifyTOTP(w io.Writer, reader *bufio.Reader, fd int, u *User, t *i18n.Translator) bool {
	fmt.Fprint(w, promptText(t, "auth.totp_prompt", "TOTP code: "))

	// term.ReadPassword needs a real terminal file descriptor. When
	// stdin is not one, such as with piped input or some test
	// harnesses, this falls back to reading a plain line from the
	// same bufio.Reader the rest of the login flow already uses,
	// rather than crashing outside an interactive session. Interactive
	// callers, such as main.go's real login flow, always pass a
	// genuine terminal file descriptor, so this fallback is a safety
	// net, not the normal path.
	var code string
	if codeBytes, err := term.ReadPassword(fd); err == nil {
		fmt.Fprintln(w)
		code = string(codeBytes)
	} else {
		line, _ := reader.ReadString('\n')
		code = strings.TrimSpace(line)
	}

	return VerifyTOTPCode(u.TOTPSecret, code, time.Now())
}
