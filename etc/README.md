# RouterCLI Configuration Files

This directory holds the RouterCLI configuration file (`routercli.yaml`) and the multi-user login database (`users.yaml`).


## Properties for the routercli.yaml YAML file

This is the main configuration file for RouterCLI, loaded from the path given via `--config`/`-c` (default `etc/routercli.yaml`). A missing file is not an error as the values found in `DefaultSystemConfig()` are used instead. Unknown keys are an error, so a typo in this file fails loudly at startup rather than being silently ignored.

All fields are optional; every field not set falls back to the default shown below.

### Security Settings

#### `PreventEscape` 

This property attempts to prevent a user from being able to escape the CLI. The default is `false`. When set to `true` RouterCLI will ignore **SIGINT**, **SIGTSTP**, **SIGQUIT**, and **SIGTERM** at the OS level, so `Ctrl-C`, `Ctrl-Z` (suspend), `Ctrl-\`, and a plain `kill <pid>` cannot stop or background the process. It also changes the read loop so `Ctrl-C` does nothing observable and `Ctrl-D` no longer ends the session; it instead prints "Use 'exit' to leave." and keeps reading. The only way out becomes the `exit` command. NOTE: **SIGKILL** and **SIGSTOP** cannot be blocked or ignored by any process, on any OS, in any language. That is a kernel guarantee, not a gap in this implementation.

### Logging Support

#### `LogLevel`

This property defines the following logging levels, of `0`, `1`, `3`, or `5`. The default is `0`. A value of `1` enables `error` and `info`, a value of `3` also enables `warn`, and a value of `5` also enables `debug`. The `ROUTERCLI_DEBUG` environment variable enables `debug` logging for a one time run regardless of this setting.

#### `LogFile`

This is the path and filename for the main RouterCLI system log file. The default is **STDERR**. Set this property to send system log messages to a file instead of the terminal. In a production system this matters, especially once `PreventEscape` and `AuthRequired` are in use, as it is unlikely that an operator will be monitoring **STDERR**. This file is opened in append only mode. If for some reason it cannot be opened, due to a bad path or permissions issue, RouterCLI will fall back to **STDERR** and will print a warning to **STDERR** explaining why, rather than failing to start.

#### `HistoryFile`

This is the path and filename to the readline history file. The default is `var/log/history.log`.

#### `AuditLogFile`

This is the path and filename to the audit log file. The default is `var/log/audit.log`.

#### `AuditLogEnabled`

This property enables the audit log at startup. The default is `false`. This only controls the starting state as audit logging can be toggled at runtime regardless of this setting, but depending on your implementation, with something like `audit-log enable` and `audit-log disable`.

### Internationalization Support

#### `CurrentLanguage`

This property defines the current and active language that is being used by the CLI. The default is `en`. The value of this property is this language code (e.g., `en` for English and `fr` for French). It is critical that these code match the language catalog's filename in the `LanguageDir`. If the requested code has no matching catalog it falls back to the `DefaultLanguage`.

#### `DefaultLanguage`

This property defines the default or fallback language that should be used when the current language's catalog is missing a string. The default is `en`. This is deliberately a separate setting from `CurrentLanguage`, since an operator running the CLI in `fr` with an incomplete French catalog should be able to fall back to specific and defined language (e.g., `en`).

#### `LanguageDir`

The location of the language catalog (`*.yaml`) files. The default is `var/lang`. Empty or missing is fine, since the language catalog is entirely optional, and any translated text simply falls back to its raw key, shown bracketed as `[[show.desc]]`, if no catalogs are loaded.

### Timeouts

#### `SessionIdleTimeout`

This property defines how long the read loop needs to wait for a line of input before giving up and ending the current session entirely (e.g. `10m` or `30s`), using the Go duration syntax. Zero disables it. The default is `0` (disabled).

#### `ElevationTimeout`

This property defines how long a session stays at a non-base Command Level before automatically reverting to the base level (e.g., `5m`), using the Go duration syntax. Checked once per read loop iteration, so a session sitting idle past this timeout is demoted the next time any line is entered, not through a background timer, since there is nothing useful to do before the user interacts with the CLI again anyway. The default is `0` (disabled).

### Command Tree Settings

#### `TreeStructure`

This is the path and filename to the tree structure manifest file. The default is `var/tree/tree_structure.yaml`. See `var/tree/README.md` for its full schema.

#### `CommonTreeFile`

This is the path and filename to the file defining the commands common to every Command Level (i.e., `help`, `exit`, `end`). These will be merged into every level at load time unless that level sets `skip_common`. The default is `var/tree/level_common.yaml`.

### Authentication Settings

#### `AuthRequired`

This property determines whether a login prompt runs before the command loop starts. The default is `false`. When set to `false` the initial session login prompt never runs, and every session simply stays unauthenticated. This is independent of any Command Level or Command specific authentication requirements defined in the `password_hash` property for those Command Levels and Commands (see `var/tree/README.md`). A Command Level's own `password_hash`, or an individual Command's own `password_hash`, works exactly the same whether `AuthRequired` is `true` or `false`. Setting `AuthRequired` to `true` while both `EnableHostAuthentication` and `EnableCLIAuthentication` below are `false` is a hard error at startup, since there would then be no way to establish a session's identity.

#### `EnableHostAuthentication`

This property will use the the identity of the user that is running RouterCLI as the authenticated user in RouterCLI. RouterCLI gets this information from the standard library's os/user.Current. This does not invoke a password check and that took place via the Operating System. The default is `false`. This is primarily used for a deployment that reached over an SSH session, where the user's shell is set to run RouterCLI and the authentication is handled by `sshd`.

#### `EnableCLIAuthentication`

This property will use RouterCLI's own interactive username and password login prompt, checked against whichever provider `CLIAuthProvider` is named below. The default is `true`. `EnableHostAuthentication` and `EnableCLIAuthentication` are not mutually exclusive. Both can be `true` and would be used in a scenario where RouterCLI is reached via SSH using a shared Unix account. In this scenario the OS identity alone does not tell RouterCLI who is really logging in and the username capture through the CLI authentication process becomes the session's identity. The account used via SSH is only logged as how the connection was established.

#### `EnableTOTPAuthentication`

This property enables TOTP token codes in RouterCLI. The default is `true`. TOTP authentication can be used with both `EnableHostAuthentication` and `EnableCLIAuthentication`, and one of those MUST be enabled first, otherwise it is a hard error at startup.

#### `AuthProviders`

This property defines all of the backend authentication systems in use by RouterCLI. Each entry requires a `name` and a `type`. The only current option is `local` and represents a check against a bcrypt hash in the `UsersFile`. The default is a single entry named `local` of type `local`.

```yaml
AuthProviders:
  - name: local
    type: local
```

#### `CLIAuthProvider`

This property defines which entry in the `AuthProviders` should be used by `EnableCLIAuthentication`. The default is `local`. This property is ignored when `EnableCLIAuthentication` is `false`, and MUST name an entry that actually exists in `AuthProviders` when `EnableCLIAuthentication` is `true`.

#### `UsersFile`

This is the path and filename for the multi-user login database. The default is `etc/users.yaml`. This is only used when `AuthRequired` is `true`.

#### `LoginMaxAttempts`

This property defines how many wrong password attempts are allowed within the `LoginAttemptWindow` before the login prompt refuses to an additional attempts for `LoginLockoutDuration`. The default is `3`. This is tracked separately per username, so repeatedly failing one username's password can never lock out a different, uninvolved username.

#### `LoginAttemptWindow`

This property defines a sliding window that `LoginMaxAttempts` is counted within. The default is `0` (disabled). If this is set then `LoginLockoutDuration` MUST also be set.

#### `LoginLockoutDuration`

This property defines how long the login prompt should refuse further attempts (e.g., `5m`). The default is `0` (disabled). If this is set then `LoginAttemptWindow` MUST also be set.

#### `CommandLevelMaxAttempts` / `CommandLevelAttemptWindow` / `CommandLevelLockoutDuration`

These properties define the same things as the three login properties above, but just for  Command Level authentication. The default for `CommandLevelMaxAttempts`, however, is `0` (disabled). This is tracked per Command Level, not per user and has the same validation rules as the login properties.

#### `CommandPasswordMaxAttempts` / `CommandPasswordAttemptWindow` / `CommandPasswordLockoutDuration`

These properties define the same things as the three login properties above, but just for a specific Command's authentication. The default for `CommandPasswordMaxAttempts`, however, is `0` (disabled). This is tracked per Command, not per user and has the same validation rules as the login properties.

#### `TOTPIssuer`

This property defines the "issuer" name that is shown in a user's authenticator app next to their account name. The default is `RouterCLI`. This is purely a cosmetic value, but is shown by every mainstream app, so it is worth setting to your organization's real name for production use. Used both when enrolling a user through `./routercli --mfa <username>` and when a logged in user runs `totp enable` from inside the running CLI.

#### `TOTPMaxAttempts`

This property defines how many times in a row a session may retype a rejected code before `totp enable` or `totp disable` gives up on that attempt and reports a failure. The default is `3`. Unlike the login and Command Level attempt limits above, there is no windowed lockout variant for this, only a flat cap on how many codes a single `totp enable` or `totp disable` run accepts before giving up.

#### `PasswordMinLength`

This property defines the minimum length in characters for a new password that is set via the `password change` command. The default is `10`. Every password, regardless of this setting, is rejected if it exceeds 72 bytes, the hard limit for the bcrypt algorithm.

#### `PasswordRequireUppercase` / `PasswordRequireNumbers` / `PasswordRequireSpecialChars`

These three properties each add one composition rule a new password MUST satisfy: at least one uppercase letter, at least one digit, and at least one punctuation or symbol character, respectively, checked with Go's own `unicode.IsUpper`, `unicode.IsDigit`, and `unicode.IsPunct` / `unicode.IsSymbol`. All three default to `false`. Current guidance from NIST 800-63B actually recommends against mandatory composition rules in favor of length, since they tend to push people toward predictable substitutions rather than genuinely stronger passwords, which is why none of the three are on by default. They exist for a deployment that MUST still satisfy an external composition-based policy. Each is independent, so any combination may be turned on.

#### `PasswordChangeMaxAttempts`

This property defines how many times in a row the `password change` command lets a session retry, both re-entering its current password, and a TOTP code if one is configured, and re-typing a new password that failed to match its own confirmation or the policy above, before giving up. The default is `3`.

## Properties for the users.yaml YAML file

The multi-user login database, only read when `AuthRequired: true` in `routercli.yaml`. The file's top-level configuration is a single `users:` key mapping username to that user's entry, the same top-level-key convention `tree_structure.yaml` uses, so both files read consistently if you have both open at once.

```yaml
users:
  admin:
    password: "$6$$2a$10$..."
  bob:
    password: "$6$$2a$10$..."
```

A user with no `password` set is a hard error at load time. An account nobody can ever log in to is almost certainly a mistake, not the intended setup.

### Accounts

#### `username`

There is no `username` property. The login name is the YAML key itself under `users:`, for example `admin` or `bob` in the example above, not a field written inside that user's own entry.

#### `password`

This is a required property and contains the bcrypt hash of a user's password as a "$id$encoded" string. This is generated with `./routercli --hashpassword`.

#### `totp_secret`

An optional base32-encoded TOTP secret. When set, this user must also provide a valid six-digit code from their authenticator app to log in. This is checked right after the password verifies. Empty means no second factor is required for this user. Generate one with `./routercli --mfa <username>`.
