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

	if c.SessionIdleTimeout < 0 {
		return fmt.Errorf("SessionIdleTimeout must be zero or positive, got %s", c.SessionIdleTimeout)
	}

	if c.ElevationTimeout < 0 {
		return fmt.Errorf("ElevationTimeout must be zero or positive, got %s", c.ElevationTimeout)
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
