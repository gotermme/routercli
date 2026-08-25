// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"
)

// init - This function registers "password change", reachable only
// from inside the user Command Level, see cmd_user.go and
// var/tree/level_user.yaml. It never accepts anything as a same-line
// argument, the same reasoning cmd_password_manager.go and
// cmd_totp.go already document for their own secrets: a same-line
// argument flows through main.go's runLoop exactly like any other
// typed line, which both the audit log and readline's own history
// file record verbatim, leaving a plaintext password written to disk.
//
// The handler splits its work the same way totp enable and totp
// disable already do, in cmd_totp.go. runPasswordChange drives the
// interactive prompts, reading a password or a code from the real
// terminal, then hands the already read plain strings off to
// verifyReauth and finishPasswordChange, which do the actual
// verify-and-save work. That split exists so the decision logic can
// be unit tested directly with known inputs instead of needing a real
// terminal to type them into.
//
// A password change goes through two phases, re-authentication then
// a new password, both drawing on the same shared attempt budget,
// ctx.PasswordChangeMaxAttempts, rather than each phase getting its
// own separate ceiling. A session that gets its current password
// right on the first try still only has the attempts left over to
// spend getting its new password right, and a session that fails
// re-authentication a few times has correspondingly fewer chances
// left for the new password. This mirrors how
// config.SystemConfig.PasswordChangeMaxAttempts is documented, a flat
// cap on the whole command, not a per-phase allowance. Once
// re-authentication succeeds, a later failure, a mismatch between the
// new password and its confirmation, or a policy violation, only
// re-prompts for the new password and its confirmation, never asks
// for the current password or a second factor code again, matching
// the workflow asked for: getting the new password wrong is not the
// same mistake as getting the current one wrong.
func init() {
	command.Register("password.change", runPasswordChange)
}

// runPasswordChange - This function drives the interactive body of
// "password change". See this file's init doc comment for the shape
// of the whole flow and why the attempt budget is shared across both
// of its phases.
func runPasswordChange(ctx *command.AppContext, args []string) error {
	if err := requireLoggedIn(ctx); err != nil {
		return err
	}
	user, ok := currentUser(ctx)
	if !ok {
		return fmt.Errorf("%s", ctx.Translator.T("user.no_current_user"))
	}

	maxAttempts := passwordChangeMaxAttempts(ctx)
	reauthenticated := false

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if !reauthenticated {
			password, err := auth.PromptSecret(os.Stdout, int(os.Stdin.Fd()), ctx.Translator)
			if err != nil {
				return err
			}
			var code string
			if auth.SecondFactorRequired(user) {
				code, err = auth.PromptTOTPCode(os.Stdout, int(os.Stdin.Fd()), ctx.Translator)
				if err != nil {
					return err
				}
			}
			if !verifyReauth(ctx.AuthProvider, user, password, code, time.Now()) {
				fmt.Println(ctx.Translator.T("password.change.denied"))
				printPasswordChangeRetry(ctx, maxAttempts, attempt)
				continue
			}
			reauthenticated = true
		}

		newPassword, err := auth.PromptNewPassword(os.Stdout, int(os.Stdin.Fd()), ctx.Translator)
		if err != nil {
			return err
		}
		confirmPassword, err := auth.PromptPasswordConfirmation(os.Stdout, int(os.Stdin.Fd()), ctx.Translator)
		if err != nil {
			return err
		}

		changed, err := finishPasswordChange(ctx, user, newPassword, confirmPassword)
		if err != nil {
			return err
		}
		if changed {
			return nil
		}
		printPasswordChangeRetry(ctx, maxAttempts, attempt)
	}

	fmt.Println(ctx.Translator.T("password.change.attempts_exhausted"))
	return nil
}

// passwordChangeMaxAttempts - This function returns
// ctx.PasswordChangeMaxAttempts, or 1 if it is zero or negative, the
// minimum a retry loop needs in order to offer even the first
// attempt, the same fallback totpMaxAttempts in cmd_totp.go already
// uses for the same reason. A real ctx wired up through main.go
// always carries a positive value here, config.SystemConfig.validate
// enforces that at startup, so this fallback only matters for a hand
// built *command.AppContext in a test that does not set the field.
func passwordChangeMaxAttempts(ctx *command.AppContext) int {
	if ctx.PasswordChangeMaxAttempts < 1 {
		return 1
	}
	return ctx.PasswordChangeMaxAttempts
}

// printPasswordChangeRetry - This function prints how many attempts
// remain after a failed phase of runPasswordChange, current password,
// second factor code, or new password alike, or nothing at all once
// attempt was the last one, since runPasswordChange's own closing
// "attempts_exhausted" message covers that case instead.
func printPasswordChangeRetry(ctx *command.AppContext, maxAttempts, attempt int) {
	if remaining := maxAttempts - attempt; remaining > 0 {
		fmt.Println(ctx.Translator.T("password.change.retry", remaining))
	}
}

// verifyReauth - This function checks password, through provider, see
// auth.AuthProvider, and, when auth.SecondFactorRequired(user) is
// true, code, against user's already stored credentials. Routing the
// password check through provider rather than calling
// auth.VerifyPassword against user.PasswordHash directly, the way
// this function worked before AuthProvider existed, means a password
// change re-authenticates against whichever backend actually owns
// this account, local, or an LDAP or a RADIUS directory once one
// exists, the same backend that checked the session's own original
// login. The second factor step below is deliberately left
// unconditional on which provider is in play: a TOTP secret, when one
// is enrolled, always lives in this project's own users.yaml and is
// always checked the same way regardless of where the password itself
// was verified, see auth.PromptLogin's own doc comment for the same
// reasoning. now is threaded through as a parameter rather than read
// with time.Now() here, the same reason finishTOTPEnable and
// finishTOTPDisable in cmd_totp.go take it, so a test can pass a
// fixed instant alongside a code generated for that same instant. A
// right password with a wrong or missing second factor code is
// reported and treated identically to an outright wrong password, the
// same reasoning auth.PromptLogin already documents for its own login
// attempt, so an attacker watching the response cannot tell a correct
// password with a wrong TOTP code from a wrong password.
func verifyReauth(provider auth.AuthProvider, user *auth.User, password, code string, now time.Time) bool {
	ok, err := provider.Authenticate(user.Username, password)
	if err != nil || !ok {
		return false
	}
	if auth.SecondFactorRequired(user) {
		return auth.VerifySecondFactorCode(user, code, now)
	}
	return true
}

// finishPasswordChange - This function is the tty-independent second
// half of "password change", the same split finishTOTPEnable and
// finishTOTPDisable in cmd_totp.go use, for the same reason:
// newPassword and confirmPassword arrive as already read plain
// strings so this can be unit tested directly against known inputs.
//
// Checks run in this order, stopping at the first that fails, so a
// session sees one problem at a time rather than every downstream
// check's own message at once: newPassword must equal
// confirmPassword, must satisfy ctx.PasswordPolicy per
// auth.ValidatePassword, with every violation of that reported
// together since ValidatePassword itself already checks every rule
// rather than stopping at the first, and must differ from the
// account's current password, checked with auth.VerifyPassword
// against user's existing
// PasswordHash, since a password that is not actually changing at all
// provides no security benefit and most likely means the session
// retyped its own current password by mistake.
//
// On success, user.PasswordHash is replaced with a freshly bcrypt
// hashed value and persisted with auth.SaveUsers immediately, using
// ctx.UsersFile, the same path ctx.Users was originally loaded from.
// If that save fails, the in-memory change is rolled back, the same
// rollback finishTOTPEnable and finishTOTPDisable already perform for
// their own save failures, so this session's own state does not claim
// the password changed when the users file on disk still says
// otherwise.
//
// The returned bool tells runPasswordChange whether the change
// actually completed, the same reason finishTOTPEnable and
// finishTOTPDisable each return one of their own.
func finishPasswordChange(ctx *command.AppContext, user *auth.User, newPassword, confirmPassword string) (bool, error) {
	if newPassword != confirmPassword {
		fmt.Println(ctx.Translator.T("password.change.mismatch"))
		return false, nil
	}

	if violations := auth.ValidatePassword(newPassword, ctx.PasswordPolicy); len(violations) > 0 {
		printPasswordViolations(ctx, violations)
		return false, nil
	}

	if auth.VerifyPassword(user.PasswordHash, newPassword) {
		fmt.Println(ctx.Translator.T("password.change.same_as_current"))
		return false, nil
	}

	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return false, err
	}

	previous := user.PasswordHash
	user.PasswordHash = hash
	if err := auth.SaveUsers(ctx.UsersFile, ctx.Users); err != nil {
		user.PasswordHash = previous
		return false, err
	}

	ctx.Logger.Debugln("DEBUG: password changed for user", ctx.Session.Username)
	fmt.Println(ctx.Translator.T("password.change.confirm"))
	return true, nil
}

// printPasswordViolations - This function prints one translated
// message per entry in violations, the i18n-key-mapping
// responsibility auth.PasswordViolation's own doc comment says
// belongs in package cmd, since package auth has no i18n awareness of
// its own anywhere, see auth/login.go's promptText. TooShort and
// TooLong carry the actual configured minimum length and the fixed
// auth.MaxPasswordLength as a formatting argument, so the message
// tells a session the real number to satisfy rather than a generic
// complaint. An unrecognized violation, which ValidatePassword never
// actually returns today, still prints its own raw value rather than
// being silently dropped, so a future violation type added to package
// auth without a matching case here fails loudly instead of going
// unreported.
func printPasswordViolations(ctx *command.AppContext, violations []auth.PasswordViolation) {
	for _, v := range violations {
		switch v {
		case auth.PasswordViolationTooShort:
			fmt.Println(ctx.Translator.T("password.change.violation_too_short", ctx.PasswordPolicy.MinLength))
		case auth.PasswordViolationTooLong:
			fmt.Println(ctx.Translator.T("password.change.violation_too_long", auth.MaxPasswordLength))
		case auth.PasswordViolationNeedsUppercase:
			fmt.Println(ctx.Translator.T("password.change.violation_needs_uppercase"))
		case auth.PasswordViolationNeedsNumber:
			fmt.Println(ctx.Translator.T("password.change.violation_needs_number"))
		case auth.PasswordViolationNeedsSpecialChar:
			fmt.Println(ctx.Translator.T("password.change.violation_needs_special_char"))
		default:
			fmt.Println(string(v))
		}
	}
}
