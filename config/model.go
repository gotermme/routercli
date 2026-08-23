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
}
