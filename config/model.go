// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package config

import "time"

// ----------------------------------------------------------------------
// Define Object Model
// ----------------------------------------------------------------------

// Duration - This type is a YAML friendly wrapper around time.Duration. A
// YAML value such as "10m" is a string, while time.Duration is an integer
// count of nanoseconds. This type lets a configuration file write a
// duration the same way Go's own duration syntax does, and handles the
// conversion on both read and write.
type Duration time.Duration

// SystemConfig - This type holds the settings that control routercli
// itself. A field left unset takes the value from DefaultSystemConfig.
// See LoadSystemConfig for how a YAML file is decoded into this type.

type SystemConfig struct {
	PreventEscape                  bool     `yaml:"PreventEscape"`
	LogLevel                       int      `yaml:"LogLevel"`
	LogFile                        string   `yaml:"LogFile"`
	HistoryFile                    string   `yaml:"HistoryFile"`
	AuditLogFile                   string   `yaml:"AuditLogFile"`
	AuditLogEnabled                bool     `yaml:"AuditLogEnabled"`
	CurrentLanguage                string   `yaml:"CurrentLanguage"`
	DefaultLanguage                string   `yaml:"DefaultLanguage"`
	LanguageDir                    string   `yaml:"LanguageDir"`
	SessionIdleTimeout             Duration `yaml:"SessionIdleTimeout"`
	ElevationTimeout               Duration `yaml:"ElevationTimeout"`
	TreeStructure                  string   `yaml:"TreeStructure"`
	CommonTreeFile                 string   `yaml:"CommonTreeFile"`
	AuthRequired                   bool     `yaml:"AuthRequired"`
	UsersFile                      string   `yaml:"UsersFile"`
	LoginMaxAttempts               int      `yaml:"LoginMaxAttempts"`
	LoginAttemptWindow             Duration `yaml:"LoginAttemptWindow"`
	LoginLockoutDuration           Duration `yaml:"LoginLockoutDuration"`
	CommandLevelMaxAttempts        int      `yaml:"CommandLevelMaxAttempts"`
	CommandLevelAttemptWindow      Duration `yaml:"CommandLevelAttemptWindow"`
	CommandLevelLockoutDuration    Duration `yaml:"CommandLevelLockoutDuration"`
	CommandPasswordMaxAttempts     int      `yaml:"CommandPasswordMaxAttempts"`
	CommandPasswordAttemptWindow   Duration `yaml:"CommandPasswordAttemptWindow"`
	CommandPasswordLockoutDuration Duration `yaml:"CommandPasswordLockoutDuration"`
	TOTPIssuer                     string   `yaml:"TOTPIssuer"`
	TOTPMaxAttempts                int      `yaml:"TOTPMaxAttempts"`
	PasswordMinLength              int      `yaml:"PasswordMinLength"`
	PasswordRequireUppercase       bool     `yaml:"PasswordRequireUppercase"`
	PasswordRequireNumbers         bool     `yaml:"PasswordRequireNumbers"`
	PasswordRequireSpecialChars    bool     `yaml:"PasswordRequireSpecialChars"`
	PasswordChangeMaxAttempts      int      `yaml:"PasswordChangeMaxAttempts"`

	// EnableHostAuthentication, when true, trusts the identity of the
	// operating system account routercli itself is running as, read
	// through os/user.Current, as proof enough of who is connecting.
	// No password is prompted for or checked on this path at all. This
	// is meant for a deployment reached over SSH, where sshd already
	// authenticated the underlying Unix account, whether routercli is
	// installed as that account's login shell or reached through a
	// ForceCommand, before routercli ever started. See
	// auth.SessionFromHostIdentity.
	EnableHostAuthentication bool `yaml:"EnableHostAuthentication"`

	// EnableCLIAuthentication, when true, runs routercli's own
	// interactive username and password login, auth.PromptLogin,
	// checked against whichever provider CLIAuthProvider names below.
	// This is today's original behavior, and stays the only enabled
	// authentication source by default, see DefaultSystemConfig.
	//
	// EnableHostAuthentication and EnableCLIAuthentication are not
	// mutually exclusive. Both true describes a shared Unix account
	// reached over SSH, where the OS identity alone does not tell
	// routercli which real person is at the keyboard. In that
	// combination, the CLI login's own username becomes
	// Session.Username, the identity used for everything from that
	// point on, while the OS account routercli was reached as is kept
	// on Session.HostUsername purely as a record of how the connection
	// arrived. See main.go's establishSession.
	EnableCLIAuthentication bool `yaml:"EnableCLIAuthentication"`

	// EnableTOTPAuthentication is the system-wide switch for whether a
	// TOTP code is ever required as part of establishing a session, on
	// top of whichever of the two flags above actually identifies the
	// account. It cannot stand alone: validate below rejects it unless
	// at least one of EnableHostAuthentication or
	// EnableCLIAuthentication is also true, since a second factor is a
	// step up on top of a primary identity, not a substitute for one.
	// When EnableCLIAuthentication alone is on, this simply governs
	// whether auth.PromptLogin's existing per-user TOTPSecret check
	// runs at all; a false value here means a session is never
	// challenged for a TOTP code at login even if a user record
	// happens to carry one. When only EnableHostAuthentication is on,
	// this is what requires a TOTP step up at all, since trusting the
	// OS identity alone otherwise needs no further check.
	EnableTOTPAuthentication bool `yaml:"EnableTOTPAuthentication"`

	// AuthProviders names every authentication backend this
	// deployment has configured, so a project can add a new kind of
	// backend, an LDAP or a RADIUS server for instance, by adding an
	// entry here rather than by changing code. Only CLIAuthProvider
	// below is actually put to use today, and only the "local" Type,
	// bcrypt hashes in UsersFile, has a real implementation, see
	// auth.NewAuthProvider. An unrecognized Type is a startup error,
	// the same fail loudly convention every other malformed setting in
	// this file already follows.
	AuthProviders []AuthProviderConfig `yaml:"AuthProviders"`

	// CLIAuthProvider names which entry in AuthProviders
	// EnableCLIAuthentication's own login prompt checks a typed
	// password against. It is ignored entirely when
	// EnableCLIAuthentication is false.
	CLIAuthProvider string `yaml:"CLIAuthProvider"`
}

// AuthProviderConfig - This type is one entry in SystemConfig's own
// AuthProviders list, naming one authentication backend and which
// kind it is. See AuthProviders' own doc comment for what Type values
// are recognized today.
type AuthProviderConfig struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}
