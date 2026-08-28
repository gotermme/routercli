// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package core implements a set of command handlers broadly useful to
almost any project built on routercli, not tied to any one vendor's
command set or any one project's own application state. Login and
session management, elevating a session to a more privileged Command
Level, moving into and back out of a configuration mode, terminal
paging and output filtering settings, and password and TOTP self
service all fall into this category. See package product, in
cmd/product, for the separate, openly optional set of Cisco and HP
flavored demonstration commands, hostname, interface configuration,
and show running-config among them, built on top of what this package
provides.

Nothing in this package requires package product, and nothing in
package product is required by this package. main.go wires both in
today, a named import of package product and a blank import of this
package, but a project is free to drop either import, replace package
product with its own command set entirely, or add a third package of
its own commands alongside both. Package command, the reusable
framework underneath both, has no idea package core or package product
exist at all. See "Encouraged, Not Required" below for the same point
made about one specific, common convention this package follows,
elevating a session through something named "enable".

A project built on routercli needs a real place to put its own command
logic, separate from the reusable framework in package command. This
package is one such place, useful largely unchanged across many
projects. Every file here starts with "cmd_" and can be deleted if a
project does not want what it provides, the same freedom package
product's own doc comment describes for that package's files.

Importing package core from main.go, even with a blank import since
nothing here needs to be referenced by name, is what loads every
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
for an example. cmd_mode_control.go is one of the simplest complete
ones.

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
State, a project's own data, entirely unused by every handler in this
package, see Application State below, Logger, Session for login and
elevation state, Levels for every Command Level the project defines,
see command.TreeStructure and command.CommandLevel, Position for the
current Command Level stack, Translator for i18n, Negated, see
Negation below, and Audit. A handler reads what it needs off ctx. It
never constructs or replaces any of these itself.

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
actually want to track, nor should it. No handler in this package
touches ctx.State at all; everything here operates only on the
framework level values already listed under The Handler Signature
above, terminal geometry, filter mode, the current Command Level, the
logged in Session's own user record, and so on, precisely so this
package stays useful to a project whose own application state looks
nothing like package product's ProductState. A project defining its
own state type follows package product's own lead, see that package's
model.go, not anything in this one.

# Negation

Cisco- and HP-style CLIs let most configuration commands be undone with
a leading "no", for example "no shutdown", instead of defining a
separate "no-shutdown" command. Each entry in the YAML command tree can
opt in to this by setting "negatable: true", see
cmd_password_manager.go's own registered entry for a working example in
this package, and ctx.Negated is true for exactly the duration of the
one RunFunc call when the command was reached that way. A handler that
supports negation checks it first, before doing anything else:

	if ctx.Negated {
	        // Undo whatever this command normally does.
	        return nil
	}
	// Normal, non-negated behavior.

See cmd_password_manager.go for a working example: it clears a stored
secret rather than flipping a bool, since nothing in this package's own
state is a simple on or off value the way package product's Shutdown
field is.

# Entering a New Command Level

Some commands do more than perform an action. They change what commands
are reachable next, the way "configure terminal" moves a session into
config mode. That is command.CommandLevelStack.Push, not anything
specific to this package:

	ctx.Position.Push(command.CommandLevelFrame{
	        Name:         "config",
	        PromptSuffix: level.PromptSuffix, // From ctx.Levels.ByName["config"].
	        Tree:         level.Tree,
	})

See cmd_configure.go for a working example. package product's own
cmd_interface.go builds on this same mechanism one level deeper,
nesting config-if inside config, and additionally shows Context in
use, stashing the interface name being edited onto the pushed frame,
something this package has no equivalent need for.

# Encouraged, Not Required: enable, exec, and Privileged Mode

This package registers "enable" and "disable", elevating a session
into a Command Level named "exec", following the two step, unprivileged
then privileged login model real Cisco and HP devices both use. This is
an encouraged convention, not a requirement package command or main.go
imposes. See cmd_enable.go's own doc comment for the full reasoning:
"exec" is not special cased anywhere in package command, nothing beyond
the string literals inside that one file's own handlers would need to
change if a project renamed it, and command.EnterCommandLevel,
command.ExitCommandLevel, and command.VerifyCommandLevels all work from
whatever names var/tree/tree_structure.yaml declares, never from any
name hardcoded here or in package command.

A project building on routercli should not assume "enable", "exec", or
even the two tier unprivileged then privileged shape itself will always
be there. A project is free to rename this Command Level to something
else entirely, to add a third, fourth, or further privilege tier beyond
plain exec, layering additional cmd_<name>.go files the same way
cmd_user.go layers a fifth Command Level alongside exec, config, and
diagnostic, or to drop the whole idea of a privileged mode and run
every command directly out of base. Nothing downstream of
var/tree/tree_structure.yaml's own declared entries, command.VerifyCommandLevels
included, cares which of these a project actually chose.

# Command Levels: enable, configure, exit and end

Every command described above, including these, is registered by a
hand-written cmd_<name>.go file in this package, with no exceptions.
See cmd_enable.go, cmd_configure.go, and cmd_mode_control.go. A project
can rename "enable" to anything it wants, or add a third, fourth, or
further Command Level, by writing one small new file following the
same pattern, never by editing package command or main.go, which know
nothing about "enable", "exec", "config", or any other level's name.

cmd_enable.go, a root swap level where only one of base or exec can be
active at a time, calls command.EnterCommandLevel and
command.ExitCommandLevel, which does the generic mechanical work, the
parent check, the password check, the Session.CommandLevel update, and
swapping the root CommandLevelStack frame, and reports what happened
without printing anything itself. See that function's own doc comment
for why. cmd_configure.go, a nested, stacking mode where config and,
through package product's own config-if, a level built on top of it can
both be active at once, layered, calls command.RequireCurrentCommandLevel
directly and pushes its own CommandLevelStack frame, since SetRootTree
cannot express more than one of these being active at once.

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
declared name really was registered by some file in this package or
package product, catching a typo or a forgotten file at startup instead
of the first time a user actually types the command. Neither
LoadTreeStructure nor anything else in package command ever calls
Register dynamically from that data.

cmd_mode_control.go registers "exit" and "end", the two commands almost
every nested Command Level needs to leave it, back to its own parent or
all the way back to the root, respectively. These are ordinary commands
in this package, calling command.CommandLevelStack.Pop and PopToRoot,
with no special casing anywhere for which level happened to be current
when either one ran.

# Command Level: user

cmd_user.go registers a Command Level named user, a nested, stacking
mode like config, reached with command.RequireCurrentCommandLevel and
a manual ctx.Position.Push rather than command.EnterCommandLevel, but
with its parent set to base rather than exec, so it is reachable
directly after logging in, without first running "enable". It also
adds a check neither config nor any other level in this package needs:
requireLoggedIn, also in cmd_user.go, refuses entry unless
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
package core
