// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auth

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gotermme/routercli/i18n"

	"golang.org/x/term"
)

// dummyBcryptHash - This constant is a fixed, valid bcrypt hash used
// only to burn the same amount of CPU time a real password comparison
// would, when LocalAuthProvider.Authenticate, see provider.go, hits a
// nonexistent username. Its plaintext, "not-a-real-password", is
// never compared against anything a real user could type. Only its
// bcrypt cost factor matters here.
const dummyBcryptHash = "$2a$10$C6UzMDM.H6dfI/f/IKcEeO4Sqzr4v4E4T0.E1TQhZY6NxN.0kQ8wa"

// ErrLoginFailed - This variable is returned by PromptLogin once the
// attempt limit is exhausted, so a caller such as main.go can
// distinguish a user typing the wrong password repeatedly from an
// actual I/O error, and choose an appropriate exit path and message
// for each.
var ErrLoginFailed = errors.New("authentication failed")

// ----------------------------------------------------------------------
// Public Functions - Login
// ----------------------------------------------------------------------

// VerifyLogin - This function is the actual login process, kept
// separate from any terminal I/O so it can be unit tested without a
// real tty. It delegates the actual credential check to provider,
// see AuthProvider, so this function itself no longer needs to know
// whether that check is against a local users.yaml, an LDAP
// directory, or anything else. A nonexistent username and a wrong
// password intentionally produce the exact same result through the
// boolean return. Anything more specific, such as distinguishing "no
// such user" from "wrong password", would tell an attacker which
// usernames are valid, a classic login error message mistake, and
// AuthProvider's own implementations, see LocalAuthProvider.Authenticate,
// are written to preserve that property.
func VerifyLogin(provider AuthProvider, username, password string) (*Session, bool) {
	ok, err := provider.Authenticate(username, password)
	if err != nil || !ok {
		return nil, false
	}
	return &Session{Username: username, Authenticated: true}, true
}

// PromptLogin - This function drives the interactive login prompts
// for a username and password, reading the password with echo
// disabled, checking the typed password with provider, see
// AuthProvider. users is still needed alongside provider, since a
// resolved identity's own second factor secret, and the TOTP prompt
// that goes with it below, is always looked up in this project's own
// users.yaml database regardless of which AuthProvider actually
// checked the password; an AuthProvider such as a future LDAP or
// RADIUS backend has no notion of a TOTP secret of its own. A
// username provider authenticates that has no matching entry in
// users, possible once a non-local AuthProvider is in play, is
// treated as a real identity with no second factor configured rather
// than an error, see the u == nil guard below.
//
// totpEnabled mirrors config.SystemConfig.EnableTOTPAuthentication.
// When false, no second factor is ever requested here, even for a
// user whose users.yaml entry has a TOTPSecret set, since that global
// switch is meant to turn step-up authentication off deployment wide,
// not merely to stop new second factors from being enrolled. See
// cmd/core/cmd_totp.go, which is removed from the tree entirely, through
// command's Requires field, when this same configuration flag is
// off.
//
// If the matched user has a second factor configured and totpEnabled
// is true, a valid code for it is also required, read from the same
// bufio.Reader created here rather than a fresh one. A right password
// with a wrong or missing second factor code counts as a failed
// attempt, the same as a wrong password. It is reported and audited
// identically, so an attacker cannot distinguish a wrong password
// from a right password with the wrong TOTP code. auditFail is
// called after every failed attempt, not just the final one, so a
// caller can record each one rather than only the attempt that ended
// the session.
//
// This function works even if the Translator is not set up.
//
// When rateLimiter is nil, PromptLogin enforces a flat cap of
// maxAttempts total tries, with no windowing, lockout, or wait. When
// a rate limiter is supplied, the outer loop's own bound becomes a
// generous safety ceiling rather than the actual limiting mechanism.
// The rate limiter's own lockout, checked with Allow right after each
// username is read so it can be scoped per username, is what
// actually stops repeated attempts, and it does so without sleeping
// inline. Once a session is locked out, this function returns
// ErrLoginFailed immediately rather than blocking for the lockout
// duration, since a real, potentially minutes-long sleep inside an
// interactive prompt, or a scripted login flow, is worse than simply
// ending the attempt and telling the caller how long to wait before
// trying again.
func PromptLogin(r io.Reader, w io.Writer, fd int, provider AuthProvider, users Users, totpEnabled bool, maxAttempts int, rateLimiter *KeyedRateLimiter, t *i18n.Translator, auditFail func(username string)) (*Session, error) {
	reader := bufio.NewReader(r)

	// With a real rate limiter, the lockout itself is what actually
	// limits how many attempts may happen. This loop bound is purely
	// a defensive safety net against an infinite loop if the rate
	// limiter is ever misconfigured to never lock out, and is not
	// expected to be reached in normal operation.
	loopBound := maxAttempts
	if rateLimiter != nil {
		loopBound = 1000
	}

	for attempt := 1; attempt <= loopBound; attempt++ {
		fmt.Fprint(w, promptText(t, "auth.username_prompt", "Username: "))
		username, err := reader.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("error reading username: %v", err)
		}
		username = strings.TrimSpace(username)

		if rateLimiter != nil {
			if ok, retryAfter := rateLimiter.Allow(username); !ok {
				fmt.Fprintln(w, "%", promptText(t, "auth.too_many_attempts", "Too many failed attempts - try again in %s", RoundForDisplay(retryAfter)))
				if auditFail != nil {
					auditFail(username)
				}
				return nil, ErrLoginFailed
			}
		}

		fmt.Fprint(w, promptText(t, "auth.password_prompt", "Password: "))
		passwordBytes, err := term.ReadPassword(fd)
		fmt.Fprintln(w) // ReadPassword does not echo the newline the user typed.
		if err != nil {
			return nil, fmt.Errorf("error reading password: %v", err)
		}

		session, ok := VerifyLogin(provider, username, string(passwordBytes))
		if ok {
			u := users[username]
			if u == nil {
				// provider authenticated username, but this project's
				// own users.yaml has no matching entry for it. This is
				// expected once a non-local AuthProvider is in play,
				// see this function's own doc comment, and simply
				// means there is no second factor secret to check.
				u = &User{Username: username}
			}
			if !totpEnabled || !SecondFactorRequired(u) {
				if rateLimiter != nil {
					rateLimiter.RecordSuccess(username)
				}
				return session, nil
			}
			if VerifySecondFactor(w, reader, fd, u, t) {
				if rateLimiter != nil {
					rateLimiter.RecordSuccess(username)
				}
				return session, nil
			}
			// A wrong or missing second factor code falls through to
			// the same failure handling as a wrong password, below.
		}

		if rateLimiter != nil {
			rateLimiter.RecordFailure(username)
		}
		if auditFail != nil {
			auditFail(username)
		}
		if attempt < loopBound {
			fmt.Fprintln(w, "%", promptText(t, "auth.login_incorrect", "Login incorrect"))
		}
	}

	return nil, ErrLoginFailed
}

// RoundForDisplay - This function rounds a retry-after duration up to
// the nearest second before showing it to a user. The underlying
// duration often carries sub-second precision, for example
// "4m59.7s", that is meaningless noise in a "try again in %s"
// message. It is exported so every place that needs this can reach
// it.
func RoundForDisplay(d time.Duration) time.Duration {
	return d.Round(time.Second)
}

// PromptSecret - This function reads a single password, masked, with
// no username and no association with any *User, and returns it as
// plaintext for the caller to verify.
func PromptSecret(w io.Writer, fd int, t *i18n.Translator) (string, error) {
	fmt.Fprint(w, promptText(t, "auth.password_prompt", "Password: "))
	passwordBytes, err := term.ReadPassword(fd)
	fmt.Fprintln(w)
	if err != nil {
		return "", err
	}
	return string(passwordBytes), nil
}

// PromptNewPassword - This function reads a candidate new password,
// masked the same way PromptSecret reads an existing one, for
// cmd/core/cmd_password.go's password change command. It is a distinct
// function from PromptSecret, rather than PromptSecret reused as is,
// so its own prompt text ("New password: ") reads unambiguously
// different from a prompt for an already-known password.
func PromptNewPassword(w io.Writer, fd int, t *i18n.Translator) (string, error) {
	fmt.Fprint(w, promptText(t, "auth.new_password_prompt", "New password: "))
	passwordBytes, err := term.ReadPassword(fd)
	fmt.Fprintln(w)
	if err != nil {
		return "", err
	}
	return string(passwordBytes), nil
}

// PromptPasswordConfirmation - This function reads a second, masked
// copy of a candidate new password, for cmd/core/cmd_password.go's
// password change command to confirm against what PromptNewPassword
// already read, the same "type it twice" confirmation step any
// password change form uses to catch a typo before it becomes the
// only copy of a password nobody, including its own owner, can
// actually reproduce.
func PromptPasswordConfirmation(w io.Writer, fd int, t *i18n.Translator) (string, error) {
	fmt.Fprint(w, promptText(t, "auth.confirm_password_prompt", "Confirm new password: "))
	passwordBytes, err := term.ReadPassword(fd)
	fmt.Fprintln(w)
	if err != nil {
		return "", err
	}
	return string(passwordBytes), nil
}

// ----------------------------------------------------------------------
// Private Functions - Login
// ----------------------------------------------------------------------

// promptText - This function resolves a translation key through t,
// falling back to fallback if t is nil or the key resolves to i18n's
// bracketed "missing" placeholder. The second check matters because a
// nil *i18n.Translator already returns the bracketed form, but so
// does a real Translator with no catalogs loaded at all, and a login
// or password prompt showing "[[auth.username_prompt]]" instead of
// readable English would be a genuinely worse experience than just
// using the fallback. args, if given, are applied with fmt.Sprintf to
// whichever text is actually used, translated or fallback.
func promptText(t *i18n.Translator, key, fallback string, args ...any) string {
	text := t.T(key, args...)
	if text == "[["+key+"]]" {
		if len(args) > 0 {
			return fmt.Sprintf(fallback, args...)
		}
		return fallback
	}
	return text
}
