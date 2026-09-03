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
	// ProductName is the deployment's own display name, shown in the
	// centered title of command.DetailedHelp's own man page style
	// header line, "<ProductName> Help Information", by way of
	// command.AppContext.ProductName, the same wiring pattern
	// TOTPIssuer below already uses for its own, narrower purpose. The
	// default is "RouterCLI" itself, see DefaultSystemConfig, so a
	// deployment that never sets this still gets a real, sensible
	// title rather than a blank one.
	ProductName         string   `yaml:"ProductName"`
	PreventEscape       bool     `yaml:"PreventEscape"`
	LogLevel            int      `yaml:"LogLevel"`
	LogFile             string   `yaml:"LogFile"`
	HistoryFile         string   `yaml:"HistoryFile"`
	AuditLogFile        string   `yaml:"AuditLogFile"`
	AuditLogEnabled     bool     `yaml:"AuditLogEnabled"`
	CurrentLanguage     string   `yaml:"CurrentLanguage"`
	DefaultLanguage     string   `yaml:"DefaultLanguage"`
	LanguageDir         string   `yaml:"LanguageDir"`
	SessionIdleTimeout  Duration `yaml:"SessionIdleTimeout"`
	ElevationTimeout    Duration `yaml:"ElevationTimeout"`
	ReauthGracePeriod   Duration `yaml:"ReauthGracePeriod"`
	SuConfigTrustWindow Duration `yaml:"SuConfigTrustWindow"`
	TreeStructure       string   `yaml:"TreeStructure"`
	CommonTreeFile      string   `yaml:"CommonTreeFile"`
	StartupConfigFile   string   `yaml:"StartupConfigFile"`
	AuthRequired        bool     `yaml:"AuthRequired"`
	UsersFile           string   `yaml:"UsersFile"`

	// DaemonSocketPath is the Unix domain socket a real RouterCLI
	// daemon listens on, and a CLI client connects to, for shared
	// state and multi session awareness; see
	// claude/DAEMON_ARCHITECTURE_DESIGN.md for the full design. Empty,
	// its default, means exactly what routercli has always done: the
	// CLI runs standalone, one process per connection, its own state
	// freshly loaded at boot, no daemon involved at all. A deployment
	// sets this field, and runs the daemon binary pointed at the same
	// path, only when it actually wants shared state across sessions;
	// the same opt-in shape every other property in this project's own
	// configuration already follows.
	DaemonSocketPath               string   `yaml:"DaemonSocketPath"`
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

	// AlphabeticalCommandOrder, true by default, sorts a listing of
	// more than one command name, "help", "?", and Tab completion's own
	// candidate list, by name. Setting this false instead shows
	// commands in the order their own tree file defines them in, own
	// commands before common commands regardless of MergeCommonCommands
	// below, since there is no one true combined definition order
	// across two separate files the way there is one true alphabetical
	// order across both. Real Cisco and HP devices both sort
	// alphabetically, which is why this defaults to true. MUST be true
	// whenever MergeCommonCommands is true, see that field's own doc
	// comment for the startup error the opposite combination produces.
	// See command.ListOptions and command.SortCommandNames for where
	// this is actually applied.
	AlphabeticalCommandOrder bool `yaml:"AlphabeticalCommandOrder"`

	// MergeCommonCommands, true by default, sorts a common command,
	// help, "?", exit, end, merged into every Command Level's Tree
	// from var/tree/level_common.yaml unless a level sets skip_common,
	// into its normal alphabetical position among every other command
	// in a listing, matching real Cisco and HP, which do the same.
	// Setting this false instead appends every common command after
	// every other command, alphabetical among themselves. This
	// property MUST NOT be true while AlphabeticalCommandOrder above
	// is false; validate rejects that combination as a hard error at
	// startup, since there is no single combined definition order
	// across two separate files the way there is one true alphabetical
	// order across both, so there is nothing coherent left for this
	// setting to merge into.
	MergeCommonCommands bool `yaml:"MergeCommonCommands"`

	// PagingEnabled, true by default, is the deployment wide switch
	// for the interactive "--More--" pause a Pageable command's
	// output goes through once it is longer than one screen, see
	// command.Command.Pageable and package paging. Setting this false
	// never blocks a session on a keypress, printing every line
	// straight through instead, while "| include", "| exclude", and
	// "| begin" filtering, an entirely separate concern, stays
	// available regardless of this setting. Real Cisco and HP always
	// pause by default, which is why this defaults to true.
	PagingEnabled bool `yaml:"PagingEnabled"`

	// DefaultPageLines, 24 by default, matching the classic terminal
	// height most devices have always defaulted to, is how many
	// lines a Pageable command's output shows before pausing when no
	// session has ever typed "terminal length" and the real
	// terminal's own height cannot be read, piped or redirected
	// stdin for instance. A session sitting at a real terminal, and
	// one that has typed "terminal length" explicitly, both use a
	// different value instead, see paging.EffectivePageLines.
	DefaultPageLines int `yaml:"DefaultPageLines"`

	// DefaultHistorySize, 500 by default, matching the underlying
	// readline library's own built-in default, formerly invisible to
	// anyone reading this file, fixes how many past commands this
	// session's own Up and Down arrow recall remembers for the whole
	// life of the session, and, unless a session types "terminal
	// history size <n>" itself, how many lines "show history" prints
	// back. Only the second of those two can change after startup, a
	// session that types "terminal history size" affecting "show
	// history" alone, never the Up and Down arrow recall limit,
	// already fixed by this value the moment the session began; see
	// command.AppContext.HistorySize's own doc comment for why.
	DefaultHistorySize int `yaml:"DefaultHistorySize"`

	// FilterMatchMode, "substring" by default, chooses how a
	// "| include", "| exclude", or "| begin" pattern is matched
	// against a line of output at startup. "substring" keeps a line
	// whenever it literally contains the typed text, predictable for
	// an operator who never wants to think about a metacharacter
	// hiding inside an ordinary word. "regex" compiles the pattern as
	// a Go RE2 regular expression instead, matching real Cisco and
	// HP exactly. validate rejects any other value. A session can
	// change this at runtime with "terminal filter-mode
	// <substring|regex>", see command.AppContext.FilterMode.
	FilterMatchMode string `yaml:"FilterMatchMode"`

	// MaxFilterChainDepth, 2 by default, is the most "| ..." stages
	// one typed command line may chain together, "show running-config
	// | include interface | exclude shutdown" being a chain of two.
	// A line asking for more than this is refused outright with a
	// real error naming the configured maximum, never silently
	// truncated to it. Zero disables output filtering entirely for
	// this deployment, a real hardening option for a project that
	// wants Pageable output but no pipe filtering exposed at the
	// command line at all. validate rejects a negative value.
	MaxFilterChainDepth int `yaml:"MaxFilterChainDepth"`

	// RolesFile is where a deployment declares which role names exist
	// for use with a Command or CommandLevel's own allowed_roles list,
	// see command.LoadRoles. A missing file is not an error, and
	// simply means this deployment never uses AllowedRoles at all, see
	// RoleSet's own doc comment.
	RolesFile string `yaml:"RolesFile"`

	// DefaultsDir is the directory holding this deployment's own
	// factory default files, at minimum a skeleton UsersFile seeded
	// with one bootstrap account holding the bypass role, see
	// var/tree/README.md's own roles section. "erase users" and
	// "restore-factory-defaults", both in the new admin Command Level,
	// see cmd/core/cmd_admin.go, copy from here, matched to the live
	// file's own base name, rather than deleting a file to nothing the
	// way "erase startup-config" always has.
	DefaultsDir string `yaml:"DefaultsDir"`
}

// AuthProviderConfig - This type is one entry in SystemConfig's own
// AuthProviders list, naming one authentication backend and which
// kind it is. See AuthProviders' own doc comment for what Type values
// are recognized today.
type AuthProviderConfig struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"`
}
