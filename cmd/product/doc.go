// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package product implements a working set of Cisco and HP flavored
demonstration commands: hostname, interface configuration, a trivial
diagnostic self test, and the "set description" and "show *" commands
that report this package's own state back out. Nothing here is part of
the reusable framework, and nothing here is required by it. See
package core, in cmd/core, for the separate set of command handlers,
login and session elevation, configuration mode entry, and password
and TOTP self service among them, broadly useful across projects that
have nothing to do with network gear at all, and package session, in
cmd/session, for the narrower set of settings genuinely local to one
connection, terminal paging and filtering among them, see that
package's own doc comment for the boundary that separates it from both
of the other two.

This package exists to be replaced. A project built on routercli that
is not modeling network equipment, or that simply wants a different
command set, deletes this package's import from main.go, deletes this
directory, and writes its own instead, following the same patterns
documented below. Nothing in package command or package core imports
this package or knows it exists; the dependency runs one way only,
this package on top of both of those, never the reverse. main.go
imports this package only because the working demonstration this
repository ships needs somewhere to keep hostname, interface state,
and the rest of what "show running-config" reports, not because
routercli itself requires a hostname, an interface, or any of the
other ideas modeled here.

Every file here starts with "cmd_" and can be deleted individually if
a project wants most of these commands but not all of them, the same
freedom package core's own doc comment describes for that package's
files. Importing package product from main.go is what loads every
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
State, this package's own data, see Application State below, Logger,
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
that reads or writes it, hostname, banner motd and banner login,
description, interface description and shutdown, and line length,
width, and paging, reaches it through ctx.DaemonClient rather than a
direct type assertion on ctx.State, so a project built on this
framework can add its own command against the identical pattern and
have it work correctly whether this deployment is standalone or backed
by a real daemon, config.SystemConfig.DaemonSocketPath set, with no
change of its own once one exists. See
claude/DAEMON_ARCHITECTURE_DESIGN.md and command.DaemonClient's own
doc comment for the full design this follows.

A write reaches ProductState through MutateProductState's own closure
argument, never through ctx.State directly:

	_, err := ctx.DaemonClient.MutateProductState(func(productState any) (any, error) {
	        state := productState.(*ProductState)
	        // Mutate state exactly as a direct ctx.State.(*ProductState)
	        // type assertion would have.
	        return nil, nil
	})
	return err

Three rules keep this correct once a real daemon exists, not only in
today's standalone deployment shape, where MutateProductState's own
closure simply runs against ctx.State's identical underlying value:

Resolve state from the closure's own productState parameter, every
time, never from a *ProductState read or captured before the closure
ran. A real, remote daemon implementation hands this closure a freshly
fetched copy on each call, not the same pointer a handler read a
moment earlier; see cmd_hostname.go's own doc comment for this same
point made at greater length.

Read anything session-local, not shared state, ctx.Position, ctx.Negated,
and args among them, outside the closure, exactly where a handler
already unmigrated would. cmd_description_if.go's own ifaceName, taken
from ctx.Position.Current().Context before its own MutateProductState
call, is a working example: nothing about which interface a command
concerns is shared state, so nothing about resolving it needs the
closure at all.

Reserve MutateProductState for an actual write. A command that only
ever reads ProductState, "show running-config" and "show interface" in
cmd_show.go for instance, keeps reading ctx.State.(*ProductState)
directly; command.DaemonClient exposes only Mutate methods, no Read
method of any kind, which is itself the evidence reads were never
meant to route through it. See runningConfigLines in cmd_show.go for a
worked example of a read that deliberately stays direct.

The definition for ProductState lives in model.go, in this package. It
is a plain struct, one field per piece of running configuration style
state, such as Hostname, Description, and per interface state. A
project extending or replacing this package adds fields to its own
equivalent type, not anywhere in package command or package core, and
reaches every one of them the same MutateProductState way described
above, whether the field already existed here or is entirely the
project's own addition.

A command a vendor adds that instead touches the tree structure's own
runtime data, a Command Level's own defined aliases or its own
PasswordHash, the user database, or the role set, reaches that
through ctx.DaemonClient.MutateLevels, MutateUsers, or MutateRoles
respectively, the same three rules above applying unchanged, only the
shared value and its own accessor method differing. None of those
three live in this package; see package core's own doc comment, under
Application State there, for the full pattern and worked examples,
since cmd_alias.go, cmd_password_manager.go, cmd_admin.go, and
cmd_totp.go, every one of them already migrated, all live there.

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

See cmd_shutdown.go for a working example: it simply flips a bool back
to false.

# Entering a New Command Level

Some commands do more than perform an action. They change what commands
are reachable next, the way "interface eth0" moves a session into
config-if mode. That is command.CommandLevelStack.Push, not anything
specific to this package:

	ctx.Position.Push(command.CommandLevelFrame{
	        Name:         "config-if",
	        PromptSuffix: level.PromptSuffix, // From ctx.Levels.ByName["config-if"].
	        Tree:         level.Tree,
	        Context:      name, // Whatever this sub-mode needs to remember.
	})

See cmd_interface.go for a working example. It also shows Context in
use: the interface name given to "interface <name>" is stashed on the
pushed frame, so cmd_description_if.go and cmd_shutdown.go, running
later inside that sub-mode, can read back which interface they are
editing through ctx.Position.Current().Context, without
command.CommandLevelStack needing to know anything about what an
interface actually is. cmd_diagnostic_mode.go is the other shape,
a root swap level built the same way cmd_enable.go in package core is,
see that package's own doc comment for the distinction between the two
shapes.

# Command Levels: diagnostic mode and interface

cmd_diagnostic_mode.go registers a Command Level named diagnostic, a
deliberately trivial example of a third, non-inheriting privilege tier
alongside exec, reached only from exec, see var/tree/level_exec.yaml,
holding one command of its own, cmd_diagnostic.go's "self-test", also
deliberately trivial. A real project's diagnostic mode would presumably
do something considerably more interesting; this exists to be something
to actually run from inside a third-tier Command Level, and to prove
the shape works, not as a serious feature in its own right. See package
core's own doc comment, under "Encouraged, Not Required," for the
broader point this small example is meant to illustrate: an
implementation can add, remove, or reshape privilege tiers like this
one freely, since nothing in package command or package core assumes
diagnostic, or any tier beyond base and whatever this package calls
exec, will ever exist.

cmd_interface.go registers "interface", entering config-if mode for one
named interface at a time, the nested, stacking mode described under
Entering a New Command Level above. cmd_description_if.go and
cmd_shutdown.go are the two commands that only make sense once inside
that mode.

This does not change anything about how a command in this package
behaves once it exists. For example, cmd_set.go is an entirely
ordinary command file, registered the normal way, that simply happens
to be reachable directly from exec mode rather than needing config
mode first, the same as any other command whose tree entry happens to
sit at that level.
*/
package product
