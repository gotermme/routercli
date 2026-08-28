# CLI Environment

## Tree Structure

A Command Line Interface (CLI) environment contains a series of commands at
various levels forming a tree like structure. This collection of levels, and
how one navigates to them, is called a Tree Structure. Levels can exist off of
the base level, or can be nested deeply. At each level, the prompt can change
and there can be additional authentication requirements to reach that level. 

In the simplest form of a CLI environment all commands will found at the base
level, creating a flat tree structure. However, most CLI environments will have
additional levels forming a more complicated tree structure. For example,
network switches can have a `base` level, a `exec` or `enable` level, and
inside that a `configure terminal` level, and nested inside that a `interface
eth#` level and maybe even a `vlan #` level. 


### Properties for YAML File

All fields are optional except for `tree_file`. Unknown property names, in
either `tree_structure.yaml` itself or in a Command Level's own tree file, are
a hard error, so a typo in a property name fails loudly at startup rather
than being silently ignored, the same way `etc/routercli.yaml` and
`etc/users.yaml` already treat an unknown key in their own files.

#### `tree_file`

This property is the path to the YAML file that holds this Command Level's own
commands. It is the one required property on every entry, including the base
level.

#### `is_base`

This property defines the one, and only one, entry point a session starts in.
There MUST be enabled only once across the whole `tree_structure.yaml`.
RouterCLI refuses to start if this is not set at least once, or if it was set
more than once.

#### `parent`

This property defines the Command Level a session must currently be in in order
to reach this Command Level. It is set on every entry except the base level,
which MUST omit it, since the base level is where every session starts and
there can be only one base level in the Tree Structure. Every Command Level,
without exception, is reached through a handwritten `cmd_*.go` file that
enforces this parent requirement.

#### `inherit_parent`

When `true`, this Command Level's effective command set is its own commands plus
every command inherited from its parent, recursively up the entire Tree
Structure. The default is `false`. When set to `false` this Command Level
contains only the commands listed in its own YAML file.

#### `enter_command`

This property names the command that moves a session from the parent into this
Command Level. It is required for every non-base level, and
`command.VerifyCommandLevels` checks it at every startup, and standalone
through `--check-config`. A level missing this property, or naming a command
that was never registered, fails loudly at startup rather than silently
producing a Command Level nobody can ever reach.

#### `exit_command`

This property names the command that moves a session back out to the parent
without ending the session. It is optional; a level with no `exit_command` can
still be left with the generic `exit` command. When set,
`command.VerifyCommandLevels` checks `exit_command` the same way it checks
`enter_command`. It is best practice to provide one.

#### `password_hash`

This property is an optional hard coded secret, a bcrypt hash, required to use
`enter_command` successfully. Omit it, or leave it empty, to have
`enter_command` work immediately, with no password prompt at all.

#### `prompt_suffix`

This property is the text appended to the prompt while this Command Level is
currently active, for example `"(config)"`. An empty string is fine, and the
base level normally has none.

#### `skip_common`

When `true`, this Command Level does not get the standard `help`, `exit`, and
`end` commands merged in from `level_common.yaml`. The default is `false`. When
set to `false` it merges all common commands into this Command Level, so this
is an opt-out property rather than something to remember to turn on.


## Command Level

A collection of commands at a given level is called a Command Level. Each
Command Level is defined in its own YAML file.

Each YAML file contains the commands found for that Command Level. It does not
define what a command actually does, as that lives in the actual Go code. The
command name found in the `run` property below must exactly match the name
found in the `command.Register()` function that is part of the `init
()` function found in a `cmd_<something>.go` file in `cmd/core` or
`cmd/product`. This is how the information in
the YAML file and the Go code are connected. If the names do not match, then
the system refuses to start.

### Properties for YAML File

#### `desc`

This property is the one line description shown for this command in a command
listing. It is only used when `DescKey` is empty.

#### `help`

This property is the help text shown for this command. It is only used when
`HelpKey` is empty. The output of the `help` command is generated dynamically.

#### `arghelp`

This property is a one line hint describing the argument a command expects, for
example `"<2-1000>  Enter a number for the 'length' command/parameter."`, shown
by the completer when Tab is pressed with nothing yet typed for that argument.
It only means something on a leaf command, one with no subcommands, that also
sets a `minargs` value, since a command with no required argument has nothing
to hint about.

#### `desc_key`

This property is a key into the language catalog (`var/lang/`) that may be used
in place of a literal `desc`, when translation is needed or desired. If both
`desc` and `desc_key` are set for a given command, `desc_key` wins. It is best
practice to use the keyed version over the static version.

#### `help_key`

This property works the same way as `desc_key`, but for `help` instead of
`desc`.

#### `arghelp_key`

This property works the same way as `desc_key`, but for `arghelp` instead of
`desc`.

#### `run`

This property is the name of the handler to call when this command runs. It must
exactly match the string passed to `command.Register()` in some
`cmd_<something>.go` file's `init()` function, in `cmd/core` or
`cmd/product`, or RouterCLI refuses to
start.

#### `alias`

This property creates an alias for and existing command. When set, this command
becomes a pointer to another command name at the same level. Resolve() finds
the alias's name automatically and that is executed instead.

#### `hidden`

This property enables commands to be hidden. When `true` the command is hideden
from tab completion, command listings, and the dynamically built help output.
However, it is still reachable if the full command name is typed in.

#### `negatable`

This property when `true`, allows the command to be undone by prefixing it with
a `no` operator, running the same handler with a negated flag set rather than
being a separate registration.

#### `password_hash`

This property is an optional bcrypt hash gating this specific command,
independent of Command Level. This is distinct from a Command Level's own
`password_hash`, which gates entering a whole level rather than one command
inside it. For example, `show tech` or toggling the audit log can each carry
its own `password_hash`.

#### `minargs`

This property sets the minimum number of arguments this command requires,
enforced before `run` is ever called. `nil` means no minimum, the common case
for most commands. This is not enforced when the command was reached
through `no`.

#### `maxargs`

This property sets the maximum number of arguments this command accepts,
enforced the same way as `minargs`. `nil` means no maximum. This is a \*int
rather than a plain int on purpose, since a plain int would set a `nil` / zero
value to 0, which would silently mean "this command accepts zero arguments" for
every node that does not set it. This makes `nil` unambiguous.

#### `maxarglength`

This property sets the maximum length, in runes, that is allowed for any single
argument, enforced the same way as `minargs` and `maxargs`.

#### `requires`

This property names a feature flag that must be true for this command, and
its subcommands, to exist in the tree at all. It is checked once at startup
by `PruneDisabledCommands`, called from `main.go` right after the tree
structure is loaded. The default is empty, meaning the command is always
available. This differs from `password_hash` above in kind, not just in
name. `password_hash` gates whether a reachable command's own action is
allowed to run, while `requires` gates whether the command is reachable, or
even shown in help or tab completion, at all. That distinction matters for a
command whose whole reason to exist depends on a feature being turned on,
`password change` when `EnableCLIAuthentication` is `false` in
`etc/routercli.yaml` for instance, where the right behavior is for the
command not to exist rather than to exist and refuse.

The flag names checked today are `totp`, tied to `EnableTOTPAuthentication`,
and `password_change`, tied to `EnableCLIAuthentication`. See
`var/tree/level_user.yaml` for both in real use, and `etc/README.md` for
what each of those `routercli.yaml` settings does.

#### `pageable`

This property, a boolean defaulting to `false`, marks a command as safe to
run through output paging and pipe filtering, `| include`, `| exclude`,
`| begin`, and the interactive `--More--` pager for output longer than one
screen. It is opt in on purpose, one command at a time, rather than on for
every command with an exclusion list. A command whose own handler reads
directly from the terminal partway through running, a masked password
prompt or a TOTP code for instance, must never be marked `pageable`, since a
pageable command's entire output is captured into memory before anything
reaches the real terminal, see the `paging` package's own `CaptureOutput`
function, and an interactive prompt captured that way would never actually
reach the person who needs to answer it. Every command in
`var/tree/level_base.yaml` that only prints and returns, `show version`,
`show interface`, `show running-config`, `show startup-config`, and `show
terminal` among them, sets `pageable: true`. A command line that types a
pipe filter against a command left at the default `false` is refused with
an error rather than silently ignoring the filter and running the command
anyway. See `README.md`'s own section on output paging and filtering for
the full picture, and `etc/README.md` for the related `PagingEnabled`,
`DefaultPageLines`, `FilterMatchMode`, and `MaxFilterChainDepth` settings.

#### `subcommands`

This property lists the nested subcommands reachable from this command.


## Commands

Each command may have sub commands, for that command (e.g., `show version`,
where `show` is the command and `version` is the sub command.) It is common,
though not required, that when a command has one or more sub commands, that one
of the sub commands must be provided for the command to fully execute. 


## Purpose

The purpose of this framework is to allow organizations to organize commands and
levels into whatever tree structure they need to support their desired CLI
environment. 
