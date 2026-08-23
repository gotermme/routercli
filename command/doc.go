// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package command is the reusable framework routercli is built on. A
project using this framework writes its own commands and its own
YAML command trees, but never needs to touch this package to do so.
That separation is why this package exists as its own thing: nothing
here knows about hostnames, interfaces, or any other actual routercli
command, only about the shape a command line interface like this takes.
See package cmd for the actual commands.

This package provides the command tree data structure, abbreviation
resolution, tab completion support, mode nesting such as config and
config interface, the Command Level system that covers both privilege
levels and plain nested modes, and the registration mechanism that lets
a command handler in package cmd make itself known. A project building
on this framework mostly interacts with three things: Command and the
tree shape, which is what a command is, AppContext, which is what a
handler receives, and CommandLevelStack and TreeStructure, which govern
how a session moves around. Each is covered below.

# The Tree Shape

A command tree is a plain Go map[string]*Command. The map key is the
command word itself, such as "show" or "hostname", and Command
describes everything about that word: its description, whether it is
hidden, whether it carries its own password gate, its argument
constraints, and its own subcommands. See Command's own doc comment for
the full field list.

Trees are normally built from YAML rather than written in Go by hand.
See LoadTree, LoadTreeStructure, and MergeTrees, and see
var/tree/README.md for the YAML file format itself. A project can build
a map[string]*Command directly in Go instead, since nothing requires
the YAML loader specifically, but every example in this project uses
YAML, since that lets someone add or rearrange commands without
touching Go source at all.

# Resolution and Completion

Both command execution, in main.go's runLoop, and tab completion, in
package completer, need the same answer to one question: given what has
been typed so far, what command does that actually refer to, and is the
answer unambiguous. Resolve(tree, tokens) is that single function.
Resolve implements Cisco and HP style abbreviation matching, so "sh run"
resolves to "show running-config" as long as "sh" and "run" are each
unambiguous prefixes at their position in the tree, and reports back a
ResolveResult: either a single matched Command with any leftover
argument tokens, or a list of ambiguous candidates when more than one
command could still match. See ResolveResult's own doc comment for what
each field means.

ValidateArgs is a separate step on purpose. Resolve only answers which
command was meant. ValidateArgs, called by runLoop right after and
before RunFunc is ever invoked, checks the resolved command's own MinArgs,
MaxArgs, and MaxArgLength against what is left over. A handler in
package cmd never has to check argument count itself, since by the time
its HandlerFunc runs that is already guaranteed.

# Command Level Navigation

A session moves through a nested sequence of Command Levels: the base
level, then config if "configure terminal" was run, then config
interface on top of that if "interface eth0" was run, and so on.
CommandLevelStack tracks that sequence. Push and Pop move a session in
and out of a nested Command Level. Current returns whichever frame is
active right now, which is what runLoop and the completer both resolve
commands against.

SetRootTree is different from Push and Pop. It replaces the root
frame's Name, PromptSuffix, and Tree together in place, without
changing how many frames are on the stack. This is what moving into a
different Command Level actually does, see Tree Structure below, and it
is deliberately not a Push, so that "exit" at the root keeps meaning
quit the program at every level, the same way it does on a real Cisco
or HP device. Only a dedicated exit command, such as "disable", steps
down a level without disconnecting. Updating Name alongside Tree also
matters for RequireCurrentCommandLevel, which checks that value as the
one way anything verifies that a session is currently where it needs to
be.

# Tree Structure

CommandLevel and TreeStructure describe every Command Level a project
defines, not only a privilege chain but also plain nested modes such as
config and config interface. A project can define any number of named
levels in its own var/tree/tree_structure.yaml manifest, each with its
own declared enter and exit command names and its own choice of whether
it inherits everything its parent level could already do, through
InheritParent. Every level's enter and exit command is a hand-written
cmd/cmd_*.go file, with no exceptions. See cmd/cmd_enable.go,
cmd/cmd_diagnostic_mode.go, cmd/cmd_configure.go, and
cmd/cmd_interface.go for examples. A level reached by swapping the root
frame calls EnterCommandLevel and ExitCommandLevel. A nested mode calls
RequireCurrentCommandLevel directly and pushes its own CommandLevelStack
frame. Neither generates a Register call from manifest data. Both are
ordinary, hand-written command registrations that pull their own
CommandLevel from AppContext.Levels.ByName by name.

See LoadTreeStructure for how the manifest is parsed and validated, and
VerifyCommandLevels for how a project confirms that every declared
enter_command and exit_command actually corresponds to a real,
registered command. Those manifest fields are declared metadata for that
check, not something this package acts on to register anything itself.

# Registration

Register(name, fn) is how a command handler, almost always written in
package cmd in its own cmd_<name>.go file and called from that file's
init(), makes itself known. See package cmd's own doc comment for the
full walkthrough of writing a new command. This package only owns the
registry itself and the lookup LoadTree performs against it when a YAML
tree file's "run:" directive needs to resolve to an actual function.

# AppContext

AppContext is the one value every command handler receives. See its own
doc comment for the full field by field breakdown. It is constructed
once, in main.go, and passed through the rest of the program from
there. Nothing in this package or package cmd constructs a second one.
*/
package command
