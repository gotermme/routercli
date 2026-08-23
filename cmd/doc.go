// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package cmd implements the actual command handlers for the commands
described in the YAML tree structures. These are the functions that run
when a user types a specific command in the CLI.

A project built on routercli needs a real place to put its own command
logic, separate from the reusable framework in package command. This
package is that place, and it also ships as a working example: every
file here starts with "cmd_" and can be deleted if a project does not
need it.

Importing package cmd from main.go is all that is needed to load every
handler in this directory, since each file's init function registers
itself before main runs.

It is best practice to have one file per command found in the CLI, or
at least to keep everything in a file closely related to one command.

# Writing a New Command

A command has three parts: a name, a handler function matching
command.HandlerFunc, and a tree entry connecting the two.

Pick a reference name for the command. It must match exactly on both
sides, the Go side and the YAML side.

Create a new cmd_<name>.go file with an init function that calls
command.Register("<name>", handlerFunc). See any existing cmd_*.go file
for an example. cmd_hostname.go is the simplest complete one.

Create an entry in the appropriate var/tree/level_*.yaml file with a
"run: <name>" directive using the exact string given to Register. See
var/tree/level_base.yaml's own header comment for the full field list,
including desc, minargs, maxargs, negatable, and hidden.

Both sides have to exist, the cmd_<name>.go file and the YAML entry,
and the name used on both sides has to match exactly. An entry in a
YAML command tree whose "run:" value was never registered is a hard
error at startup, see command.LoadTree, rather than a runtime error the
first time someone types the command. This is deliberate. A typo in
either file should stop the program from starting, not silently
produce a command that does nothing.

# The Handler Signature

	type HandlerFunc func(ctx *AppContext, args []string) error

ctx carries everything a handler might need about the running session:
State, this project's own data, see Application State below, Logger,
Session for login and elevation state, Levels for every Command Level
the project defines, see command.TreeStructure and command.CommandLevel,
Position for the current Command Level stack, Translator for i18n,
Negated, see Negation below, and Audit. A handler reads what it needs
off ctx. It never constructs or replaces any of these itself.

args is whatever tokens followed the command name on the line. By the
time RunFunc is called, args has already been validated against whatever
MinArgs, MaxArgs, or MaxArgLength the tree entry specified. See
command.ValidateArgs, called from main.go's runLoop before RunFunc is ever
reached. A handler for a command with "minargs: 1" can index args[0]
directly, with no length check of its own, since if the count were
wrong this function would never have been called at all.

A non-nil error return is what runLoop prints back to the user, with a
leading "%" matching Cisco and HP convention, and what gets recorded in
the audit log as a failure. See auditlog.Auditor. Use fmt.Errorf with a
translated message, ctx.Translator.T(...), for anything a real user
might see. A bare error from some other package is fine for something
that should never actually happen, such as a case the tree structure
itself should have made unreachable.

# Application State

ctx.State is declared as any in package command, because package
command has no idea what a project built on this framework will
actually want to track, nor should it. Every handler in this package
that touches state does the same type assertion:

	state := ctx.State.(*ExampleState)

The definition for ExampleState lives in state.go, in this package. It
is a plain struct, one field per piece of running configuration style
state, such as Hostname, Description, and TerminalLength. A project
extending this example adds fields there, not anywhere in package
command.

# Negation

Cisco- and HP-style CLIs let most configuration commands be undone with
a leading "no", for example "no shutdown", instead of defining a
separate "no-shutdown" command. Each entry in the YAML command tree can
opt in to this by setting "negatable: true", see
var/tree/level_config_if.yaml's shutdown entry, and ctx.Negated is true
for exactly the duration of the one RunFunc call when the command was
reached that way. A handler that supports negation checks it first,
before doing anything else:

	if ctx.Negated {
	        // Undo whatever this command normally does.
	        return nil
	}
	// Normal, non-negated behavior.

See cmd_shutdown.go and cmd_password_manager.go for two working
examples. One simply flips a bool, the other clears a stored secret.

# Entering a New Command Level

Some commands do more than perform an action. They change what commands
are reachable next, the way "configure terminal" moves a session into
config mode. That is command.CommandLevelStack.Push, not anything
specific to this package:

	ctx.Position.Push(command.CommandLevelFrame{
	        Name:         "config-if",
	        PromptSuffix: level.PromptSuffix, // From ctx.Levels.ByName["config-if"].
	        Tree:         level.Tree,
	        Context:      name, // Whatever this sub-mode needs to remember.
	})

See cmd_configure.go and cmd_interface.go for two working examples. The
latter also shows Context in use: the interface name given to
"interface <name>" is stashed on the pushed frame, so cmd_description_if.go
and cmd_shutdown.go, running later inside that sub-mode, can read back
which interface they are editing through ctx.Position.Current().Context,
without command.CommandLevelStack needing to know anything about what an
interface actually is.

# Command Levels: enable, disable, diagnostic mode, configure, interface

Every command described above, including these, is registered by a
hand-written cmd_<name>.go file in this package, with no exceptions.
See cmd_enable.go, cmd_diagnostic_mode.go, cmd_configure.go, and
cmd_interface.go. A project can rename "enable" to anything it wants,
or add a third, fourth, or further Command Level, by writing one small
new file in this package following the same pattern, never by editing
package command or main.go, which know nothing about "enable", "exec",
"diagnostic", or any other level's name.

The four files split into two shapes, matching a real structural
difference, see command.EnterCommandLevel's own doc comment. A root
swap level, cmd_enable.go and cmd_diagnostic_mode.go, where only one
can be active at a time, calls command.EnterCommandLevel and
command.ExitCommandLevel, which does the generic mechanical work, the
parent check, the password check, the Session.CommandLevel update, and
swapping the root CommandLevelStack frame, and reports what happened
without printing anything itself. See that function's own doc comment
for why. A nested, stacking mode, cmd_configure.go and cmd_interface.go,
where config and config-if can both be active at once, layered, calls
command.RequireCurrentCommandLevel directly and pushes its own
CommandLevelStack frame, since SetRootTree cannot express more than one
of these being active at once.

Each of these files decides entirely on its own what, if anything, to
print, log, or audit for its own level's entered, already here, left,
or not here outcomes. "Entering exec mode." lives in cmd_enable.go's
own i18n keys, enable.entered in var/lang/en.yaml, not anywhere in
package command, precisely so a different project can print something
completely different, log it instead, or say nothing at all, without
touching the framework.

var/tree/tree_structure.yaml's enter_command and exit_command fields
name these commands as declared, verifiable metadata.
command.VerifyCommandLevels, in command/verify.go, confirms every
declared name really was registered by some file in this package,
catching a typo or a forgotten file at startup instead of the first
time a user actually types the command. Neither LoadTreeStructure nor
anything else in package command ever calls Register dynamically from
that data.

This does not change anything about how a command in this package
behaves once it exists. For example, cmd_password_manager.go is an
entirely ordinary command file, registered the normal way, that simply
happens to look up ctx.Levels.ByName[ctx.Session.CommandLevel] to find
out which Command Level's secret it should set.

# Command Level: user

cmd_user.go registers a fifth Command Level, user, a nested, stacking
mode like config, reached with command.RequireCurrentCommandLevel and
a manual ctx.Position.Push rather than command.EnterCommandLevel, but
with its parent set to base rather than exec, so it is reachable
directly after logging in, without first running "enable". It also
adds a check neither config nor any other level in this package
needs: requireLoggedIn, also in cmd_user.go, refuses entry unless
ctx.Session.Authenticated is true, since every command inside this
level, see cmd_totp.go and cmd_password.go, acts on the current
session's own entry in the user database, which only means something
for a session that actually logged in as somebody. This is where a
session manages its own account: its second factor through cmd_totp.go,
and its own password through cmd_password.go.

cmd_totp.go registers totp enable, totp enable qr, and totp disable,
all reachable only from inside user mode, see var/tree/level_user.yaml.
totp enable on its own shows only the plain, manually typed secret;
totp enable qr additionally shows a scannable QR code. Both share one
interactive body, runTOTPEnable, differing only in which of
printTOTPSecret or printTOTPEnrollmentQR is called first. These
commands update a User's TOTPSecret and persist that change with
auth.SaveUsers, rather than only holding it in memory for the rest of
this one session, so enrolling or removing a second factor no longer
requires stopping the program and relaunching it with the --mfa flag
the way it used to.

Every handler here splits its interactive, terminal-reading half from
its verify-and-save half, finishTOTPEnable and finishTOTPDisable, the
same split package auth's own PromptLogin and VerifyLogin already use,
so the verify-and-save decision can be unit tested with a known code
and a fixed time.Time instead of a real terminal. Both
runTOTPEnable and the registered totp.disable handler retry a
rejected code, up to ctx.TOTPMaxAttempts times, the same retry
ceiling auth.PromptLogin already enforces for a login attempt, rather
than ejecting the session after a single mistake.

Once totp enable's confirmation code has been accepted and saved, or
once every retry attempt has been used up without one, runTOTPEnable
calls clearScreen, wiping the freshly printed secret and its QR code,
and the terminal's own scrollback, off the screen so neither one is
still readable by whoever looks at that terminal next, including by
scrolling back.
*/
package cmd
