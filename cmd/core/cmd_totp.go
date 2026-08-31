// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"

	qrcode "github.com/skip2/go-qrcode"
)

// init - This function registers "totp enable", "totp enable qr", and
// "totp disable", all only reachable from inside the user Command
// Level, see cmd_user.go and var/tree/level_user.yaml. None of them
// accept a same-line confirmation code or password, the same
// reasoning cmd_password_manager.go already documents for its own
// "password manager" command: a same-line argument flows through
// main.go's runLoop exactly like any other typed line, which both the
// audit log and readline's own history file record verbatim, leaving
// a plaintext secret written to disk. The literal "qr" subcommand is
// the one exception, it names a display mode rather than a secret, so
// typing it plainly costs nothing.
//
// "totp enable" on its own shows only the plain, manually typed
// secret. "totp enable qr" additionally shows a scannable QR code,
// for whoever prefers to scan rather than type, or is enrolling from
// a terminal a QR code renders poorly on. Both then read a
// confirmation code and hand off to the shared runTOTPEnable, along
// with which of the two forms was used.
//
// Every handler here splits its work in two. The registered handler
// does the interactive part, printing whatever enrollment material
// applies and reading a code or password from the real terminal, then
// hands off to finishTOTPEnable or finishTOTPDisable, which take the
// already read secret material as plain arguments and do the actual
// verify-and-save work. That split exists so the decision logic, does
// this code verify, and what happens next, can be unit tested
// directly with a known code and a fixed time.Time instead of needing
// a real terminal to type one into, the same split auth.VerifyLogin
// and auth.PromptLogin already use for the same reason.
//
// Both totp enable and totp disable retry a rejected code, up to
// ctx.TOTPMaxAttempts times, rather than ejecting the session after a
// single mistake, mirroring the retry ceiling auth.PromptLogin already
// enforces for a login attempt. Once totp enable has printed its
// enrollment material, clearScreen wipes it off the terminal both on
// a successful confirmation and once every attempt has been used up
// without one, so a still-unconfirmed secret, and its QR code if
// shown, never lingers on screen either way. totp disable prints
// nothing sensitive of its own, only prompts for an already-known
// password and code, so it has nothing on screen that needs clearing.
func init() {
	command.Register("totp.enable", func(ctx *command.AppContext, args []string) error {
		return runTOTPEnable(ctx, false, int(os.Stdin.Fd()), os.Stdout)
	})

	command.Register("totp.enable.qr", func(ctx *command.AppContext, args []string) error {
		return runTOTPEnable(ctx, true, int(os.Stdin.Fd()), os.Stdout)
	})

	command.Register("totp.disable", func(ctx *command.AppContext, args []string) error {
		return runTOTPDisable(ctx, int(os.Stdin.Fd()), os.Stdout)
	})
}

// runTOTPDisable - This function is the interactive body of "totp
// disable", registered against fd and stdout rather than the real
// process's os.Stdin and os.Stdout directly, the same io.Writer and
// file descriptor injection pattern runTOTPEnable below and main.go's
// establishSession already use, and for the same reason: it lets a
// test hand this function a pty's slave file directly. The init
// registration above is the only real caller outside a test, always
// passing the real process's own stdin and stdout, so production
// behavior is unchanged.
func runTOTPDisable(ctx *command.AppContext, fd int, stdout io.Writer) error {
	if err := requireLoggedIn(ctx); err != nil {
		return err
	}
	user, ok := currentUser(ctx)
	if !ok {
		return fmt.Errorf("%s", ctx.Translator.T("user.no_current_user"))
	}
	if user.TOTPSecret == "" {
		fmt.Println(ctx.Translator.T("totp.not_enabled"))
		return nil
	}

	maxAttempts := totpMaxAttempts(ctx)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		password, err := auth.PromptSecret(stdout, fd, ctx.Translator)
		if err != nil {
			return err
		}
		code, err := auth.PromptTOTPCode(stdout, fd, ctx.Translator)
		if err != nil {
			return err
		}
		disabled, err := finishTOTPDisable(ctx, user, password, code, time.Now())
		if err != nil {
			return err
		}
		if disabled {
			return nil
		}
		if remaining := maxAttempts - attempt; remaining > 0 {
			fmt.Println(ctx.Translator.T("totp.disable_retry", remaining))
		}
	}

	fmt.Println(ctx.Translator.T("totp.disable_attempts_exhausted"))
	return nil
}

// runTOTPEnable - This function is the shared interactive body of
// both "totp enable" and "totp enable qr", differing only in showQR,
// which decides whether printTOTPSecret or printTOTPEnrollmentQR
// shows the freshly generated secret before a confirmation code is
// read. Everything else, the login and current-user checks, the
// already-enabled refusal, the retry loop around finishTOTPEnable,
// and clearing the screen once the secret is no longer needed, is
// identical either way. fd and stdout are injected the same way
// runTOTPDisable's own doc comment above describes, so a test can
// drive this against a real pty instead of the real process's own
// stdin and stdout.
func runTOTPEnable(ctx *command.AppContext, showQR bool, fd int, stdout io.Writer) error {
	if err := requireLoggedIn(ctx); err != nil {
		return err
	}
	user, ok := currentUser(ctx)
	if !ok {
		return fmt.Errorf("%s", ctx.Translator.T("user.no_current_user"))
	}
	if user.TOTPSecret != "" {
		fmt.Println(ctx.Translator.T("totp.already_enabled"))
		return nil
	}

	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		return err
	}
	if showQR {
		if err := printTOTPEnrollmentQR(ctx, secret); err != nil {
			return err
		}
	} else {
		printTOTPSecret(ctx, secret)
	}

	maxAttempts := totpMaxAttempts(ctx)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		code, err := auth.PromptTOTPCode(stdout, fd, ctx.Translator)
		if err != nil {
			return err
		}
		enabled, err := finishTOTPEnable(ctx, user, secret, code, time.Now())
		if err != nil {
			return err
		}
		if enabled {
			clearScreen(stdout)
			return nil
		}
		if remaining := maxAttempts - attempt; remaining > 0 {
			fmt.Println(ctx.Translator.T("totp.enable_retry", remaining))
		}
	}

	clearScreen(stdout)
	fmt.Println(ctx.Translator.T("totp.enable_attempts_exhausted"))
	return nil
}

// totpMaxAttempts - This function returns ctx.TOTPMaxAttempts, or 1 if
// it is zero or negative, the minimum a retry loop needs in order to
// offer even the first attempt. A real ctx wired up through main.go
// always carries a positive value here, config.SystemConfig.validate
// enforces that at startup the same way it already does for
// LoginMaxAttempts, so this fallback only matters for a hand built
// *command.AppContext in a test that does not set the field.
func totpMaxAttempts(ctx *command.AppContext) int {
	if ctx.TOTPMaxAttempts < 1 {
		return 1
	}
	return ctx.TOTPMaxAttempts
}

// currentUser - This function looks up the auth.User record matching
// ctx.Session.Username in ctx.Users, so the totp enable and totp
// disable handlers modify the same record auth.PromptLogin already
// authenticated against. It returns ok false if ctx.Users or
// ctx.Session is nil, or the session's own username has no matching
// entry. None of that should be possible once requireLoggedIn has
// already passed, but this is checked rather than assumed, the same
// defensive style cmd_password_manager.go already uses for its own
// ctx.Levels.ByName lookup.
func currentUser(ctx *command.AppContext) (*auth.User, bool) {
	if ctx.Session == nil || ctx.Users == nil {
		return nil, false
	}
	u, ok := ctx.Users[ctx.Session.Username]
	return u, ok
}

// printTOTPSecret - This function shows a freshly generated TOTP
// secret as plain, grouped text for manual entry, the presentation
// plain "totp enable" uses for whoever would rather type a secret
// into their authenticator app than scan a QR code, or is enrolling
// from a terminal a QR code would not render well on.
func printTOTPSecret(ctx *command.AppContext, secret string) {
	fmt.Println(ctx.Translator.T("totp.enable_secret_intro"))
	fmt.Println()
	fmt.Println("  " + auth.FormatTOTPSecretForDisplay(secret))
	fmt.Println()
}

// printTOTPEnrollmentQR - This function shows a freshly generated
// TOTP secret both as a scannable QR code and as plain text for
// manual entry, the "totp enable qr" presentation.
func printTOTPEnrollmentQR(ctx *command.AppContext, secret string) error {
	uri := auth.TOTPProvisioningURI(ctx.TOTPIssuer, ctx.Session.Username, secret)
	qr, err := qrcode.New(uri, qrcode.Medium)
	if err != nil {
		return err
	}

	fmt.Println(ctx.Translator.T("totp.enable_intro"))
	fmt.Println()
	fmt.Println(qr.ToSmallString(false))
	fmt.Println(ctx.Translator.T("totp.enable_manual"))
	fmt.Println()
	fmt.Println("  " + auth.FormatTOTPSecretForDisplay(secret))
	fmt.Println()
	return nil
}

// ansiClearScreen - This is the standard ANSI escape sequence for
// wiping a terminal, including whatever a viewer could otherwise
// still reach by scrolling back, and returning the cursor to the top
// left corner. \x1b[2J clears the visible screen, \x1b[3J additionally
// discards the terminal's own scrollback buffer, an xterm extension
// every mainstream terminal emulator in wide use today also honors,
// and \x1b[H moves the cursor home. Without \x1b[3J, a viewer could
// still scroll up and read an already cleared secret right back off
// the screen, defeating the entire point of clearing it. It is named
// once here rather than written inline in clearScreen below.
const ansiClearScreen = "\x1b[H\x1b[2J\x1b[3J"

// clearScreen - This function wipes w, an already printed secret and
// its QR code included, off the visible terminal and its scrollback
// once that material is no longer needed. runTOTPEnable calls this
// both right after a confirmation code is accepted and saved, and
// once every retry attempt has been used up without one, so a secret
// that was shown but never actually confirmed does not linger on
// screen either.
func clearScreen(w io.Writer) {
	fmt.Fprint(w, ansiClearScreen)
}

// finishTOTPEnable - This function is the tty-independent second half
// of "totp enable" and "totp enable qr". now is threaded through as a
// parameter rather than read with time.Now() here, so a test can pass
// a fixed time.Time alongside a code generated for that same instant,
// instead of racing a real clock across a 30 second TOTP window.
//
// A code that fails to verify against secret is reported and this
// returns false, nil rather than an error, the same way a wrong
// password at a login prompt is reported without failing the whole
// command, since getting one attempt wrong is not itself a program
// failure. The caller, runTOTPEnable, decides whether that leaves any
// attempts remaining.
//
// On success, user.TOTPSecret is set and the change is persisted with
// auth.SaveUsers immediately, using ctx.UsersFile, the same path
// ctx.Users was originally loaded from. If that save fails, the
// in-memory change is rolled back, so this session's own state does
// not claim TOTP is enabled when the users file on disk still says
// otherwise, and this reports false alongside the error.
//
// The returned bool tells the caller whether enrollment actually
// completed, the same reason auth.VerifyLogin returns a bool
// alongside its *Session. runTOTPEnable uses it both to decide
// whether to keep retrying and, on true, whether the secret and any
// QR code already printed to the terminal are now safe to clear.
func finishTOTPEnable(ctx *command.AppContext, user *auth.User, secret, code string, now time.Time) (bool, error) {
	if !auth.VerifyTOTPCode(secret, code, now) {
		fmt.Println(ctx.Translator.T("totp.enable_failed"))
		return false, nil
	}

	user.TOTPSecret = secret
	if err := auth.SaveUsers(ctx.UsersFile, ctx.Users); err != nil {
		user.TOTPSecret = ""
		return false, err
	}

	ctx.Logger.Debugln("DEBUG: TOTP enabled for user", ctx.Session.Username)
	fmt.Println(ctx.Translator.T("totp.enable_confirm"))
	return true, nil
}

// finishTOTPDisable - This function is the tty-independent second
// half of "totp disable", the same split finishTOTPEnable above uses,
// for the same reason. Both the account password and a current TOTP
// code must verify before the secret is cleared, re-authenticating
// the user rather than trusting that an already open session is
// still the right person sitting at the terminal, since removing a
// second factor is exactly the kind of action someone else at an
// unlocked session should not be able to do unchallenged.
//
// On success, user.TOTPSecret is cleared and the change is persisted
// with auth.SaveUsers, rolled back the same way finishTOTPEnable rolls
// back if that save fails.
//
// The returned bool tells the caller, the registered "totp.disable"
// handler, whether the account is now actually disabled, so it knows
// whether to keep retrying, the same reason finishTOTPEnable returns
// one of its own.
func finishTOTPDisable(ctx *command.AppContext, user *auth.User, password, code string, now time.Time) (bool, error) {
	if !auth.VerifyPassword(user.PasswordHash, password) || !auth.VerifyTOTPCode(user.TOTPSecret, code, now) {
		fmt.Println(ctx.Translator.T("totp.disable_denied"))
		return false, nil
	}

	previous := user.TOTPSecret
	user.TOTPSecret = ""
	if err := auth.SaveUsers(ctx.UsersFile, ctx.Users); err != nil {
		user.TOTPSecret = previous
		return false, err
	}

	ctx.Logger.Debugln("DEBUG: TOTP disabled for user", ctx.Session.Username)
	fmt.Println(ctx.Translator.T("totp.disable_confirm"))
	return true, nil
}
