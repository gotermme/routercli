# RouterCLI Configuration Files

This directory holds the RouterCLI configuration file (`routercli.yaml`) and
the multi-user login database (`users.yaml`).


## routercli.yaml

This is the main configuration file for RouterCLI itself, loaded from the
path given via `--config`/`-c` (default `etc/routercli.yaml`). A missing
file is not an error. `DefaultSystemConfig()` is used instead. Unknown
keys are an error, so a typo in this file fails loudly at startup rather
than being silently ignored.

### Properties for YAML File

All fields are optional; every field not set falls back to the default
shown below.

#### `PreventEscape` 

This property attempts to prevent a user from being able to escape the CLI. The
default is `false`. When `true` RouterCLI ignores SIGINT, SIGTSTP, SIGQUIT, and
SIGTERM at the OS level, so Ctrl-C, Ctrl-Z (suspend), Ctrl-\, and a plain `kill
<pid>` cannot stop or background the process. It also changes the read loop so
Ctrl-C does nothing observable and Ctrl-D no longer ends the session; it
instead prints "Use 'exit' to leave." and keeps reading. The only way out
becomes the `exit` command itself. NOTE: SIGKILL and SIGSTOP cannot be blocked
or ignored by any process, on any OS, in any language. That is a kernel
guarantee, not a gap in this implementation.

#### `LogLevel`

This property defines the logging level, on of `0`, `1`, `3`, or `5`, and any
other value is a hard error at startup. The default is `0`. `1` enables `error`
and `info`. `3` also enables `warn`. `5` also enables `debug`. The
`ROUTERCLI_DEBUG` environment variable enables `debug` logging for a one time
run regardless of this setting.

#### `LogFile`

This is the path and filename for system logs. The default logging location is
STDERR.  Set this property to send system log messages to a file instead of the
terminal. In production this matters, especially once PreventEscape and
AuthRequired are in real use, since STDERR is not necessarily something an
operator is monitoring in real-time. The file is opened in append mode. If it
cannot be opened, due to a bad path or permissions, RouterCLI falls back to
STDERR and prints a warning there explaining why, rather than failing to start
over a logging setting. See main.go.

#### `HistoryFile`

This is the path and filename to the readline history file. The default is
`var/log/history.log`.

#### `AuditLogFile`

This is the path and filename to the audit log file. The default is
`var/log/audit.log`.

#### `AuditLogEnabled`

This property enables the audit log at startup. The default is `false`.
This only controls the starting state. It can be toggled at runtime, depending
on your implementation, with something like `audit-log enable` and `audit-log
disable` regardless of this setting (in the example these are only reachable
once a session has moved past the base Command Level, see
`var/tree/README.md`).

#### `CurrentLanguage`

This property defines the current and active language used in the CLI. The
default is `en`. The value of this property is this language code
(e.g., `en` for English and `fr` for French). It is critical that these code
match the language catalog's filename in the `LanguageDir`. If the requested code has no matching catalog it falls back to the default language.

#### `DefaultLanguage`

This property defines the default or fallback language that should be used when
the current language's catalog is missing a string. The default is `en`.
This is deliberately a separate setting from `CurrentLanguage`, since an
operator running the CLI in `fr` with an incomplete French catalog should fall
back to specific and defined language (e.g., `en`).

#### `LanguageDir`

The location of the language catalog(`*.yaml`) files. The default is `var/lang`.
Empty or missing is fine, since the language catalog is entirely optional, and
any translated text simply falls back to its raw key, shown bracketed as 
`[[show.desc]]`, if no catalogs are loaded at all.

#### `SessionIdleTimeout`

This property defines how long the read loop needs to wait for a line of input
before giving up and ending the current session entirely (e.g. `10m` or `30s`)
using Go duration syntax. Zero disables it. The default is `0` (disabled).

#### `ElevationTimeout`

This property defines how long a session stays at a non-base Command Level
before automatically reverting to the base level (e.g., `5m`) using Go duration
syntax. Checked once per read loop iteration, so a session sitting idle past
this timeout is demoted the next time any line is entered, not through a
background timer, since there is nothing useful to do about it before the user
interacts again anyway. The default is `0` (disabled).

#### `TreeStructure`

This is the path and filename to the Tree Structure manifest file. The default
is `var/tree/tree_structure.yaml`. See `var/tree/README.md` for its full
schema.

#### `CommonTreeFile`

This is the path and filename to the file defining the commands common to every
Command Level (i.e., `help`, `exit`, `end`) that will be merged into every
level at load time unless that level sets `skip_common`. The default is
`var/tree/level_common.yaml`.

#### `AuthRequired`

This property determines whether a login prompt runs before the command loop
starts. The default is `false`. `false` means the initial session login prompt
never runs, and every session simply stays unauthenticated. This is independent
of any Command Level or Command specific authentication requirements defined in
the `password_hash` property for those Command Levels and Commands
(see `var/tree/README.md`). A Command Level's own `password_hash`, or an
individual Command's own `password_hash`, works exactly the same whether
`AuthRequired` is `true` or `false`.

#### `UsersFile`

This is the path and filename for the multi-user login database. The default is
`etc/users.yaml`. This is only read when `AuthRequired` is `true`.

#### `LoginMaxAttempts`

This defines how many wrong password attempts are allowed within the
`LoginAttemptWindow` before the login prompt refuses further tries for
`LoginLockoutDuration`. The default is `3`. The value MUST be a positive
integer, or it is a hard error at startup. This is tracked separately per
username, so repeatedly failing one username's password can never lock out a
different, uninvolved username. If `LoginAttemptWindow` and
`LoginLockoutDuration` are both left at their default, this reverts to
RouterCLI's original and more simpler behavior: a flat cap of the
`LoginMaxAttempts` total tries before the login prompt gives up entirely, with
no window or lockout concept at all.

#### `LoginAttemptWindow`

This defines the slide window that `LoginMaxAttempts` is counted within. The
default is `0` (disabled). If this is set then `LoginLockoutDuration` MUST also
be set.


#### `LoginLockoutDuration`

This defines how long the login prompt should then refuse further attempts
(e.g., `5m`). The default is `0` (disabled). If this is set then
`LoginAttemptWindow` MUST also be set.

#### `CommandLevelMaxAttempts` / `CommandLevelAttemptWindow` / `CommandLevelLockoutDuration`

These properties define the same things as the three login properties above, but
just for  Command Level authentication. The default for
`CommandLevelMaxAttempts`, however, is `0` (disabled). This is tracked per
Command Level, not per user and has the same validation rules as the login
properties.

#### `CommandPasswordMaxAttempts` / `CommandPasswordAttemptWindow` / `CommandPasswordLockoutDuration`

These properties define the same things as the three login properties above, but
just for a specific Command's authentication. The default for
`CommandPasswordMaxAttempts`, however, is `0` (disabled). This is tracked per
Command, not per user and has the same validation rules as the login
properties.

#### `TOTPIssuer`

This property defines the "issuer" name that is shown in a user's authenticator
app next to their account name. The default is `RouterCLI`. This is purely
cosmetic, but shown by every mainstream app, so it is worth setting to your
organization's real name in a real deployment. Used both when enrolling a user
through `./routercli --mfa <username>` and when a logged in user runs
`totp enable` from inside the running CLI.

#### `TOTPMaxAttempts`

This defines how many times in a row a session may retype a rejected code
before `totp enable` or `totp disable` gives up on that attempt and reports
failure. The default is `3`. The value MUST be a positive integer, or it is a
hard error at startup, the same validation rule `LoginMaxAttempts` already
uses. Unlike the login and Command Level attempt limits above, there is no
windowed lockout variant for this one, only a flat cap on how many codes a
single `totp enable` or `totp disable` run accepts before giving up, since
these two commands already require an authenticated session to even reach.

#### `PasswordMinLength`

This is the shortest new password the `password change` command accepts, in
characters. The default is `10`. The value MUST be a positive integer, or it
is a hard error at startup, the same validation rule every other
`*MaxAttempts` setting already uses. This project does not additionally
enforce a hard floor of its own beneath whatever value is configured here,
so an operator who sets a very small number is deliberately overriding the
shipped default, not tripping over a hidden safety net. Every password,
regardless of this setting, is also always rejected once it exceeds 72
bytes, the hard limit the bcrypt algorithm itself imposes on its input, see
`auth.MaxPasswordLength`.

#### `PasswordRequireUppercase` / `PasswordRequireNumbers` / `PasswordRequireSpecialChars`

These three properties each add one composition rule a new password must
satisfy: at least one uppercase letter, at least one digit, and at least one
punctuation or symbol character, respectively, checked with Go's own
`unicode.IsUpper`, `unicode.IsDigit`, and `unicode.IsPunct` /
`unicode.IsSymbol`. All three default to `false`. Current guidance from
NIST 800-63B actually recommends against mandatory composition rules in
favor of length, since they tend to push people toward predictable
substitutions rather than genuinely stronger passwords, which is why none of
the three are on by default here. They exist for a deployment that must
still satisfy an external composition-based policy. Each is independent, so
any combination may be turned on.

#### `PasswordChangeMaxAttempts`

This defines how many times in a row the `password change` command lets a
session retry, both re-entering its current password, and a TOTP code if one
is configured, and re-typing a new password that failed to match its own
confirmation or the policy above, before giving up on that attempt. The
default is `3`. The value MUST be a positive integer, or it is a hard error
at startup, the same validation rule `TOTPMaxAttempts` already uses. As with
`TOTPMaxAttempts`, there is no windowed lockout variant, only a flat cap,
since `password change` already requires an authenticated session to even
reach.


## The users.yaml File

The multi-user login database, only read when `AuthRequired: true` in
`routercli.yaml`. The file's top-level shape is a single `users:` key
mapping username to that user's entry, the same top-level-key convention
`tree_structure.yaml` uses, so both files read consistently if you have
both open at once.

```yaml
users:
  admin:
    password: "$6$$2a$10$..."
  bob:
    password: "$6$$2a$10$..."
```

A user with no `password` set is a hard error at load time. An account
nobody can ever log in to is almost certainly a mistake, not intent, so
this fails loudly at startup rather than producing a bug report about a
login that just does not work.

### Properties for the Users YAML File

#### `username`

There is no `username` property. The login name is the YAML key itself under
`users:`, for example `admin` or `bob` in the example above, not a field
written inside that user's own entry.

#### `password`

This is a required property and contains the bcrypt hash of a user's password
as a "$id$encoded" string. This is generated with `./routercli --hashpassword`.
No user can log in without this set, if authentication is enabled.

#### `totp_secret`

An optional base32-encoded TOTP secret. When set, this user must also provide a
valid six-digit code from their authenticator app to log in, checked right
after the password verifies. Empty means no second factor is required for this
user. Generate one with `./routercli --mfa <username>`.
