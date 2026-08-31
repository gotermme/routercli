// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package auth implements password hashing and verification, a multi-user password
store loaded from a YAML file, the login prompts, multi-factor authentication
via TOTP, and the Session type that tracks one connection's state for the rest
of the program's life. It has no dependency on the command package, meaning a
Command Level can exist and run without auth ever being wired in. This package
only knows "is this the right password for this user" and "what does this
session currently know about itself".

# Three Separate, Deliberately Decoupled Layers

It is easy to conflate "logging in" with "having elevated access". This
framework deliberately keeps them separate, as three independent layers a
project can use in any combination: a login prompt at the start of a
session, a password on a Command Level, and a password on one specific
command. A project can use any of these, all of them, or none of them, and
none of the three requires the others to be configured.

Session.Authenticated tracks the first (login) system; Session.CommandLevel
tracks the second.

# Password Storage

HashPassword and VerifyPassword are the only two functions that should ever
touch the stored form of the password. Both work against the same `$id$encoded`
format, and neither is written against any one hashing algorithm directly.
See PasswordHasher, in auth.go, for the interface a project implements to
add or swap the algorithm HashPassword and VerifyPassword actually use.
RouterCLI ships bcrypt as the default, registered and made active at
package init, but a project wanting a different algorithm, one already
required by an existing deployment's own compliance rules, or one believed
to hold up against a future, quantum capable attacker, calls
RegisterPasswordHasher with its own implementation and, if it should be
used for every new hash from then on, SetDefaultPasswordHasher, without
touching HashPassword, VerifyPassword, or any of their own call sites
anywhere else in this project. Every password already hashed under a
different algorithm keeps verifying correctly regardless of which one is
currently the default, since VerifyPassword always dispatches on the id
already embedded in the stored value.

Passwords are never stored, logged, or passed around as plaintext longer than
the one call that needs them. PromptSecret reads a masked password directly in
to a string that is handed straight to VerifyPassword and then allowed to go out
of scope; there is no intermediate "logged in user's plaintext password" field
anywhere in this package.

# Users and the User Database

A User entry in `etc/users.yaml` contains a username, a password hash, and
optionally a TOTP secret for multi-factor login. LoadUsers parses and validates
the whole file at startup (a user with no password hash at all is a hard error,
not a silently-unusable account). SaveUsers is the inverse, writing the whole
database back to disk under the same "users:" shape LoadUsers reads, so a
running session, such as the totp enable and totp disable commands in package
cmd, can persist a change made mid-session instead of requiring an
administrator to hand edit the file and restart.

# Logging In

PromptLogin is the whole interactive flow. It reads a username, reads a masked
password, verifies it, and, if the matched user has a second factor configured,
immediately follows up with VerifySecondFactor before considering the session
authenticated. It retries up to maxAttempts times, calling back auditFail after
each wrong attempt so the caller can log it, and returns a fresh *Session on
success.

# Sessions

Session is deliberately small, just enough state for the rest of the program to
answer "who is this" and "what can they do right now" without re-deriving it on
every command. See Session's own doc comment for exactly what each field means
and who is responsible for keeping it current. The short version is that this
package only ever sets Username/Authenticated, CommandLevel /
CommandLevelEnteredAt are set by whichever hand-written cmd_*.go file calls
command.EnterCommandLevel / ExitCommandLevel instead, which is why NewSession
leaves CommandLevel as the zero value rather than trying to guess it.

# Two-Factor Authentication (TOTP)

totp.go implements TOTP (RFC 6238) from scratch. There are no external
dependencies because the whole algorithm is small, well-specified, and worth
being able to read end to end in one file rather than trusting a black box for
something this security-sensitive. GenerateTOTPSecret creates a new random
secret for a user being enrolled; TOTPProvisioningURI turns that in to the
otpauth:// URI a phone authenticator app scans (as a QR code), and
FormatTOTPSecretForDisplay groups that same secret for manual entry.
VerifyTOTPCode checks a submitted code with a small clock-skew tolerance, the
same way every real TOTP implementation does, since no two clocks agree to the
second forever.

Enrollment itself is entirely self service, from inside a running,
already logged in session. The user Command Level and its totp enable
and totp disable commands, both in package core (cmd/core), drive
GenerateTOTPSecret, TOTPProvisioningURI, and VerifyTOTPCode, through
PromptTOTPCode below for reading the confirmation code and SaveUsers
above for persisting the result. A user with no totp_secret set yet
logs in with a password alone, then runs totp enable to add a second
factor to their own account, with nothing to stop and relaunch. An
earlier command line flag, --mfa, drove the identical functions from
outside a running session, before this in-session path existed; it is
removed as of Phase 29, now that totp enable fully covers what it was
for.
*/
package auth
