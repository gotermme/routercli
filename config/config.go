// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// ----------------------------------------------------------------------
// Public Methods - Duration
// ----------------------------------------------------------------------

// AsDuration - This method converts the configuration duration back to a
// time.Duration for use with the standard library.
func (d Duration) AsDuration() time.Duration {
	return time.Duration(d)
}

// IsZero - This method reports whether the duration is disabled, meaning
// zero.
func (d Duration) IsZero() bool {
	return d == 0
}

// String - This method implements fmt.Stringer. It prints the duration
// using Go's own duration syntax, for example "10m0s".
func (d Duration) String() string {
	return time.Duration(d).String()
}

// durationKindName - This function names a yaml.Kind for use in
// UnmarshalYAML's error message, so a wrongly shaped duration value
// points at what was found, instead of just a generic parse failure.
func durationKindName(k yaml.Kind) string {
	switch k {
	case yaml.DocumentNode:
		return "document"
	case yaml.MappingNode:
		return "mapping"
	case yaml.SequenceNode:
		return "sequence"
	case yaml.ScalarNode:
		return "scalar"
	case yaml.AliasNode:
		return "alias"
	default:
		return fmt.Sprintf("kind %d", int(k))
	}
}

// UnmarshalYAML - This method implements yaml.Unmarshaler. It parses a
// scalar value such as "10m" or "30s" with time.ParseDuration. An empty
// value, a plain "0", or an explicit YAML null all mean disabled, and
// unmarshal to zero.
func (d *Duration) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("duration must be a scalar, got %s node", durationKindName(node.Kind))
	}

	if node.Tag == "!!null" {
		*d = 0
		return nil
	}

	s := node.Value

	if s == "" || s == "0" {
		*d = 0
		return nil
	}

	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", s, err)
	}

	if parsed < 0 {
		return fmt.Errorf("duration must be zero or positive, got %q", s)
	}

	*d = Duration(parsed)
	return nil
}

// MarshalYAML - This method implements yaml.Marshaler. It writes the
// duration back out using Go's own duration syntax, so a generated
// configuration file reads the same way a hand-written one would.
func (d Duration) MarshalYAML() (interface{}, error) {
	if d == 0 {
		return "0s", nil
	}
	return time.Duration(d).String(), nil
}

// ----------------------------------------------------------------------
// Public Functions - System Config
// ----------------------------------------------------------------------

// DefaultSystemConfig - This function returns the settings routercli
// uses when no configuration file is found, so a project can run
// before it has written one of its own. See SystemConfig's own doc
// comment for what each field means and LoadSystemConfig for how a
// real file overrides these values.
func DefaultSystemConfig() SystemConfig {
	return SystemConfig{
		ProductName:                 "RouterCLI",
		PreventEscape:               false,
		LogLevel:                    0,
		HistoryFile:                 "var/log/history.log",
		AuditLogFile:                "var/log/audit.log",
		AuditLogEnabled:             false,
		CurrentLanguage:             "en",
		DefaultLanguage:             "en",
		LanguageDir:                 "var/lang",
		TreeStructure:               "var/tree/tree_structure.yaml",
		CommonTreeFile:              "var/tree/level_common.yaml",
		RolesFile:                   "var/tree/roles.yaml",
		DefaultsDir:                 "etc/defaults",
		StartupConfigFile:           "var/startup-config/startup-config",
		AuthRequired:                false,
		UsersFile:                   "etc/users.yaml",
		LoginMaxAttempts:            3,
		TOTPIssuer:                  "RouterCLI",
		TOTPMaxAttempts:             3,
		PasswordMinLength:           10,
		PasswordRequireUppercase:    false,
		PasswordRequireNumbers:      false,
		PasswordRequireSpecialChars: false,
		PasswordChangeMaxAttempts:   3,
		EnableHostAuthentication:    false,
		EnableCLIAuthentication:     true,
		EnableTOTPAuthentication:    true,
		AuthProviders:               []AuthProviderConfig{{Name: "local", Type: "local"}},
		CLIAuthProvider:             "local",
		AlphabeticalCommandOrder:    true,
		MergeCommonCommands:         true,
		PagingEnabled:               true,
		DefaultPageLines:            24,
		DefaultHistorySize:          500,
		FilterMatchMode:             "substring",
		MaxFilterChainDepth:         2,
		// Unlike SessionIdleTimeout, ElevationTimeout, and
		// ReauthGracePeriod above, all of which default to zero,
		// disabled, this one defaults to a real, nonzero value. Those
		// three are each a narrowing of otherwise-already-working
		// behavior, opt in for a project that wants the extra
		// restriction or convenience. SuConfigTrustWindow is the
		// opposite: zero would leave su-config, a level built
		// specifically to let a saved configuration paste back in
		// without hitting a prompt at every gated level along the way,
		// unable to actually do that job out of the box. Five minutes
		// is long enough to paste even a sizeable configuration by
		// hand, short enough that it is never mistaken for a standing
		// bypass. See CommandLevel.GrantsReplayTrust and
		// AppContext.SuConfigTrustWindow's own doc comments in
		// command/model.go, and etc/README.md, for the full reasoning.
		SuConfigTrustWindow: Duration(5 * time.Minute),
	}
}

// LoadSystemConfig - This function reads routercli's YAML
// configuration file and decodes it into a SystemConfig. An empty
// configFile, or a file that does not exist, is not an error.
// DefaultSystemConfig is returned instead, so a project can run
// before it has written a configuration file of its own. An unknown
// YAML key is treated as an error, so a typo in a property name is
// caught at startup rather than silently ignored. The decoded
// configuration is passed through validate before it is returned.
func LoadSystemConfig(configFile string) (SystemConfig, error) {
	cfg := DefaultSystemConfig()

	if configFile == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(configFile)
	if errors.Is(err, os.ErrNotExist) {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("error opening configuration file %q: %v", configFile, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			// The file is empty, or contains comments only.
			return cfg, nil
		}
		return cfg, fmt.Errorf("error parsing configuration file %q: %v", configFile, err)
	}

	// A configuration file is expected to be a single top-level YAML
	// mapping, so decoding a second document here means the file
	// contains more than one, and that extra document is rejected.
	var extra SystemConfig
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return cfg, fmt.Errorf("configuration file %q contains multiple YAML documents", configFile)
		}
		return cfg, fmt.Errorf("error parsing configuration file %q: %v", configFile, err)
	}

	if err := cfg.validate(); err != nil {
		return cfg, fmt.Errorf("invalid configuration file %q: %v", configFile, err)
	}

	return cfg, nil
}

// ----------------------------------------------------------------------
// Private Methods - System Config
// ----------------------------------------------------------------------

// validate - This method checks the key fields of a SystemConfig after
// it has been decoded from YAML, so a mistake such as an out of range
// LogLevel or a half-configured rate limiter is caught at startup
// rather than causing confusing behavior later. See LoadSystemConfig,
// which calls this after decoding.
func (c SystemConfig) validate() error {
	switch c.LogLevel {
	case 0, 1, 3, 5:
	default:
		return fmt.Errorf("LogLevel must be one of 0, 1, 3, or 5, got %d", c.LogLevel)
	}

	if c.LoginMaxAttempts < 1 {
		return fmt.Errorf("LoginMaxAttempts must be a positive integer, got %d", c.LoginMaxAttempts)
	}

	if c.TOTPMaxAttempts < 1 {
		return fmt.Errorf("TOTPMaxAttempts must be a positive integer, got %d", c.TOTPMaxAttempts)
	}

	if c.PasswordMinLength < 1 {
		return fmt.Errorf("PasswordMinLength must be a positive integer, got %d", c.PasswordMinLength)
	}

	if c.PasswordChangeMaxAttempts < 1 {
		return fmt.Errorf("PasswordChangeMaxAttempts must be a positive integer, got %d", c.PasswordChangeMaxAttempts)
	}

	// MergeCommonCommands true means "merge every common command into
	// its normal alphabetical position", which only means something
	// once AlphabeticalCommandOrder has actually put every command
	// into one true alphabetical order to merge into. With
	// AlphabeticalCommandOrder false there is no single combined
	// definition order across a level's own tree file and
	// CommonTreeFile the way there is one true alphabetical order
	// across both, see command.SortCommandNames's own doc comment, so
	// this combination is rejected here rather than silently falling
	// back to appended order the way an earlier phase of this project
	// once let it.
	if !c.AlphabeticalCommandOrder && c.MergeCommonCommands {
		return fmt.Errorf("MergeCommonCommands cannot be true while AlphabeticalCommandOrder is false, there is no single combined definition order to merge common commands into")
	}

	if c.DefaultPageLines < 1 {
		return fmt.Errorf("DefaultPageLines must be a positive integer, got %d", c.DefaultPageLines)
	}

	if c.DefaultHistorySize < 0 {
		return fmt.Errorf("DefaultHistorySize must be zero or positive, got %d", c.DefaultHistorySize)
	}

	switch c.FilterMatchMode {
	case "substring", "regex":
	default:
		return fmt.Errorf("FilterMatchMode must be 'substring' or 'regex', got %q", c.FilterMatchMode)
	}

	if c.MaxFilterChainDepth < 0 {
		return fmt.Errorf("MaxFilterChainDepth must be zero or positive, got %d", c.MaxFilterChainDepth)
	}

	if c.SessionIdleTimeout < 0 {
		return fmt.Errorf("SessionIdleTimeout must be zero or positive, got %s", c.SessionIdleTimeout)
	}

	if c.ElevationTimeout < 0 {
		return fmt.Errorf("ElevationTimeout must be zero or positive, got %s", c.ElevationTimeout)
	}

	if c.ReauthGracePeriod < 0 {
		return fmt.Errorf("ReauthGracePeriod must be zero or positive, got %s", c.ReauthGracePeriod)
	}

	if c.SuConfigTrustWindow < 0 {
		return fmt.Errorf("SuConfigTrustWindow must be zero or positive, got %s", c.SuConfigTrustWindow)
	}

	// A rate limiter's AttemptWindow and LockoutDuration are only
	// meaningful as a pair. Setting one without the other produces a
	// configuration that looks like it opted in to rate limiting but
	// does not actually do anything. A nonzero window with a zero
	// lockout means every lockout expires instantly, since Allow()
	// never actually refuses, and a nonzero lockout with a zero window
	// is simply never triggered at all. See auth.RateLimiter's own doc
	// comment for how the two combine. Rejecting the half-configured
	// state here, at startup, follows this project's usual convention
	// of failing loudly instead of silently doing nothing. See
	// LoadSystemConfig's own doc comment.
	if err := validateAttemptWindowPair("Login", c.LoginAttemptWindow, c.LoginLockoutDuration); err != nil {
		return err
	}
	if err := validateAttemptWindowPair("CommandLevel", c.CommandLevelAttemptWindow, c.CommandLevelLockoutDuration); err != nil {
		return err
	}
	if err := validateAttemptWindowPair("CommandPassword", c.CommandPasswordAttemptWindow, c.CommandPasswordLockoutDuration); err != nil {
		return err
	}

	if err := c.validateAuthSources(); err != nil {
		return err
	}

	return nil
}

// validateAuthSources - This method checks the settings introduced
// for authenticating over more than one path, an interactive login,
// a trusted host identity, or both together, plus an optional TOTP
// step up on top of either. See validate's own doc comment for why a
// bad configuration is rejected here rather than left to fail in a
// more confusing way once the program is already running.
func (c SystemConfig) validateAuthSources() error {
	if c.AuthRequired && !c.EnableHostAuthentication && !c.EnableCLIAuthentication {
		return fmt.Errorf("AuthRequired is true but neither EnableHostAuthentication nor EnableCLIAuthentication is set, so there is no way to establish a session's identity")
	}

	if c.EnableTOTPAuthentication && !c.EnableHostAuthentication && !c.EnableCLIAuthentication {
		return fmt.Errorf("EnableTOTPAuthentication cannot stand alone, it requires EnableHostAuthentication or EnableCLIAuthentication to also be true")
	}

	seenNames := make(map[string]bool, len(c.AuthProviders))
	for _, p := range c.AuthProviders {
		if p.Name == "" {
			return fmt.Errorf("AuthProviders entry has an empty name")
		}
		if p.Type == "" {
			return fmt.Errorf("AuthProviders entry %q has an empty type", p.Name)
		}
		if seenNames[p.Name] {
			return fmt.Errorf("AuthProviders has more than one entry named %q", p.Name)
		}
		seenNames[p.Name] = true
	}

	if c.EnableCLIAuthentication {
		if c.CLIAuthProvider == "" {
			return fmt.Errorf("EnableCLIAuthentication is true but CLIAuthProvider is empty")
		}
		if !seenNames[c.CLIAuthProvider] {
			return fmt.Errorf("CLIAuthProvider %q does not match any entry in AuthProviders", c.CLIAuthProvider)
		}
	}

	return nil
}

// validateAttemptWindowPair - This function enforces that an
// AttemptWindow and LockoutDuration pair is either both zero, meaning
// rate limiting for that area is not opted in to the windowed behavior
// at all, or both nonzero, meaning fully configured. See validate's
// own comment for why a half-configured pair is rejected rather than
// silently doing nothing.
func validateAttemptWindowPair(prefix string, window, lockout Duration) error {
	if (window == 0) != (lockout == 0) {
		return fmt.Errorf("%sAttemptWindow and %sLockoutDuration must either both be set or both be left at zero (got window=%s, lockout=%s)", prefix, prefix, window, lockout)
	}
	return nil
}
