// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auth

import (
	"sync"
	"time"
)

// ----------------------------------------------------------------------
// Define Object Model
// ----------------------------------------------------------------------

// RateLimiter - This type implements a sliding window attempt limiter
// with a lockout. After maxAttempts failures within window, further
// attempts are refused for lockout. This matches real Cisco's own
// "login block-for lockout-seconds attempts maxAttempts within
// window-seconds" directive, deliberately following that same shape,
// three numbers with the same relationship between them, rather than
// inventing new terminology, since it is the one most operators
// coming from real network gear will already recognize.
//
// RateLimiter is safe for concurrent use, guarded by mu, since a
// CommandLevel's or Command's RateLimiter is shared state that could in
// principle be touched from more than one place, and package auth
// otherwise makes no assumptions about single-threaded callers.
//
// Rate limiting is disabled entirely when maxAttempts is at or below
// zero. Allow then always returns true, and RecordFailure and
// RecordSuccess do nothing. This matches this project's existing
// convention for optional numeric settings, see config.SystemConfig's
// SessionIdleTimeout and ElevationTimeout, both disabled by zero, and
// means a project that does not set the *MaxAttempts configuration
// fields at all gets today's actual behavior, unlimited attempts,
// with no code change required to opt out.
//
// now is an injectable clock, defaulting to time.Now through
// NewRateLimiter, so tests can advance time deterministically instead
// of calling time.Sleep for real. See ratelimit_test.go, which
// exercises window expiry and lockout expiry behavior in milliseconds
// of real wall time rather than minutes.
type RateLimiter struct {
	maxAttempts int
	window      time.Duration
	lockout     time.Duration
	now         func() time.Time

	mu          sync.Mutex
	failures    []time.Time
	lockedUntil time.Time
}

// KeyedRateLimiter - This type is a RateLimiter per key, created
// lazily on first use. This exists for the one place rate limiting in
// this project needs to be scoped per identity rather than to one
// single shared resource, login, where locking out "alice" must not
// also lock out "bob". See PromptLogin. A single, shared RateLimiter
// across every username would let anyone lock out an arbitrary other
// user just by deliberately failing that user's password a few times,
// its own denial-of-service vector. Command Level and per-command
// password rate limiting, see command.EnterCommandLevel and
// main.go's runLoop, do not need this. A Command Level's or a
// command's own secret is a single shared resource, not per-user, so
// a plain *RateLimiter is enough there.
type KeyedRateLimiter struct {
	maxAttempts int
	window      time.Duration
	lockout     time.Duration

	mu       sync.Mutex
	limiters map[string]*RateLimiter
}

// Session - This type tracks one CLI session's authentication state.
//
// Username is empty until a successful login. It is used for audit
// log entries only. See Authenticated below for why nothing in this
// project gates command reachability on identity or login state.
//
// Authenticated is true once the login prompt, or an equivalent
// caller, has verified a password. When AuthRequired is false in the
// tool configuration, main.go never runs the login prompt, and the
// session simply stays with Authenticated false for the whole
// session. This field is informational, used for audit log entries
// and for telling a real login apart from never having logged in. It
// does not, by itself, gate which commands a session can run. Command
// reachability is entirely a property of the Tree Structure, meaning
// which commands exist in which Command Level's own tree, and any
// password_hash a project chooses to set on a Command Level or an
// individual command, both completely decoupled from this field.
//
// CommandLevel is the name of the command.CommandLevel this session
// is currently in. See command.TreeStructure and command.CommandLevel.
// It is set to the base level's Name at startup by main.go, since
// NewSession itself does not know the base level's name and so cannot
// set this. See NewSession's own doc comment. It is then updated by
// whichever hand-written cmd/cmd_*.go file calls
// command.EnterCommandLevel or command.ExitCommandLevel as a session
// moves between levels, for example cmd/cmd_enable.go. This field is
// only meaningful for root swap levels reached that way. A plain,
// nested mode such as config or config-if does not touch this field
// at all, and is tracked purely through Position, the
// CommandLevelStack, instead. See
// command.RequireCurrentCommandLevel's own doc comment for why the
// two are genuinely different axes. This
// field lives in package auth, not package command, so that package
// command, which already imports auth for other reasons, can depend
// on it without an import cycle. Session itself does not need to know
// what a CommandLevel actually is, only which one it is in, by name.
//
// CommandLevelEnteredAt records when CommandLevel last changed.
// main.go's runLoop uses it together with the
// config.SystemConfig.ElevationTimeout setting to automatically
// revert to the base level once that much time has passed, the CLI
// equivalent of a privileged mode timeout. It is meaningless, and not
// read, while the session is at the base level.
//
// HostUsername and HostConnectedAt are set only when
// config.SystemConfig.EnableHostAuthentication trusted an operating
// system account identity to reach this session, see
// SessionFromHostIdentity. HostUsername is that account's own name,
// which is not necessarily Username: when EnableCLIAuthentication is
// also on, reached over a shared account for instance, Username ends
// up being whichever identity the CLI login itself resolved to, while
// HostUsername stays the underlying OS account the connection
// actually arrived as. HostConnectedAt is when that OS identity was
// established, which can meaningfully predate Username being set at
// all, if a slow or repeatedly failed CLI login followed it. Both are
// their zero values, an empty string and a zero time.Time, when
// EnableHostAuthentication was never in play for this session, and
// main.go's audit log entry for a new session includes both only in
// that case. See main.go's establishSession.
type Session struct {
	Username              string
	Authenticated         bool
	CommandLevel          string
	CommandLevelEnteredAt time.Time
	HostUsername          string
	HostConnectedAt       time.Time
}

// User - This type represents one entry in the user database.
type User struct {
	Username     string `yaml:"-"`
	PasswordHash string `yaml:"password"`
	TOTPSecret   string `yaml:"totp_secret,omitempty"`
}

// Users - This type is the in-memory form of the whole user database.
type Users map[string]*User

// yamlUsersFile - This type is the on-disk shape. Everything sits
// under a "users:" key, the same top-level key convention the tree
// YAML loader uses, so both configuration files read consistently if
// someone opens both at once.
type yamlUsersFile struct {
	Users map[string]User `yaml:"users"`
}

// PasswordPolicy - This type is the set of rules a new password must
// satisfy, checked by ValidatePassword. It mirrors
// config.SystemConfig's own Password* settings field for field, kept
// as a separate type here rather than importing package config
// directly, since package auth must not depend on package config,
// see the Core Library Versus Implementation split documented in
// PROGRESS.md for this project. A caller such as main.go builds one
// of these from the loaded SystemConfig and carries it on
// command.AppContext for cmd/cmd_password.go to use.
type PasswordPolicy struct {
	MinLength           int
	RequireUppercase    bool
	RequireNumbers      bool
	RequireSpecialChars bool
}

// PasswordViolation - This type names one way a candidate password
// failed to satisfy a PasswordPolicy, or MaxPasswordLength, see
// ValidatePassword. It carries no message of its own, deliberately;
// package auth has no i18n awareness anywhere else either, see
// login.go's promptText, so a caller such as cmd/cmd_password.go maps
// each violation to its own translated message.
type PasswordViolation string

// The complete set of PasswordViolation values ValidatePassword can
// return. TooShort and TooLong are checked unconditionally; the three
// composition violations only when the matching PasswordPolicy field
// requests them.
const (
	PasswordViolationTooShort         PasswordViolation = "too_short"
	PasswordViolationTooLong          PasswordViolation = "too_long"
	PasswordViolationNeedsUppercase   PasswordViolation = "needs_uppercase"
	PasswordViolationNeedsNumber      PasswordViolation = "needs_number"
	PasswordViolationNeedsSpecialChar PasswordViolation = "needs_special_char"
)

// AuthProvider - This type is the seam every backend that can check a
// typed username and password plugs into: today only LocalAuthProvider,
// bcrypt hashes in a Users database, with an LDAP or a RADIUS backend
// the kind of thing expected to implement this same interface later
// without VerifyLogin, PromptLogin, or cmd/cmd_password.go's own
// reauthentication step needing to change at all. See
// config.SystemConfig.AuthProviders for how a deployment names which
// backend, of which kind, it wants, and NewAuthProvider for how a
// name there becomes a real value of this type.
//
// Authenticate reports whether password is correct for username. A
// non-nil error means the check itself could not be completed, a
// network failure reaching a remote directory for instance, distinct
// from ok being false for a password that was actually wrong. A
// caller such as VerifyLogin treats either an error or ok being false
// as a failed login, exactly the same as it always has, since neither
// case should ever let a session through.
type AuthProvider interface {
	Authenticate(username, password string) (bool, error)
}

// LocalAuthProvider - This type is the AuthProvider backed by this
// project's own etc/users.yaml, or whatever a project renames its own
// UsersFile to, checking a candidate password against the matching
// User's own PasswordHash with VerifyPassword. This is what every
// deployment used before AuthProvider existed at all, so it stays the
// default entry in config.DefaultSystemConfig's own AuthProviders
// list, keeping every existing deployment's behavior unchanged.
type LocalAuthProvider struct {
	Users Users
}

// ----------------------------------------------------------------------
// Initialization Functions
// ----------------------------------------------------------------------

// NewLocalAuthProvider - This function constructs a LocalAuthProvider
// checking candidate passwords against users.
func NewLocalAuthProvider(users Users) *LocalAuthProvider {
	return &LocalAuthProvider{Users: users}
}

// NewRateLimiter - This function constructs a RateLimiter.
// maxAttempts at or below zero disables rate limiting entirely. See
// RateLimiter's own doc comment.
func NewRateLimiter(maxAttempts int, window, lockout time.Duration) *RateLimiter {
	return &RateLimiter{
		maxAttempts: maxAttempts,
		window:      window,
		lockout:     lockout,
		now:         time.Now,
	}
}

// NewSession - This function returns an empty, unauthenticated
// session with CommandLevel left unset. The caller, main.go, is
// responsible for setting CommandLevel to the base CommandLevel's
// Name right after construction, since this function lives in
// package auth and has no knowledge of package command's CommandLevel
// concept at all. See the CommandLevel field's own doc comment above
// for why that split exists. Leaving CommandLevel at its zero value
// here, rather than threading a base level name through this
// constructor, keeps package auth decoupled from package command
// entirely. Nothing in this package needs to import command, and this
// function's signature never has to change if the tree structure
// system itself changes shape later.
func NewSession() *Session {
	return &Session{}
}
