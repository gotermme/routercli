// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import "fmt"

// ----------------------------------------------------------------------
// Public Functions - Verify
// ----------------------------------------------------------------------

// VerifyCommandLevels - This function exists to catch a manifest that
// names a level's enter or exit command without a matching
// cmd_*.go file ever having registered it, a mistake that would
// otherwise only surface the first time a user actually typed the
// command and got "unknown command". It checks a successfully loaded
// TreeStructure against consistency rules beyond what loading itself
// requires, see LoadTreeStructure's own doc comment for that split,
// and returns every problem found rather than just the first, so a
// project can fix them all in one pass.
//
// It checks two things. Every non-base Command Level must declare a
// non-empty EnterCommand, since every level's enter command is a hand-
// written cmd_*.go file, see this package's own doc comment on
// CommandLevel, and a level with no declared EnterCommand has no way
// to be verified at all. And every declared EnterCommand, and every
// declared ExitCommand, must name a command that is actually
// registered, see registry.go, meaning some cmd_*.go file's
// init() really did call command.Register with that exact name.
//
// This is a separate pass from LoadTreeStructure so it can be invoked
// on its own. See main.go's --check-config flag, which runs this and
// nothing else, then exits, without building an AppContext or
// entering the interactive loop. It also runs unconditionally at
// ordinary startup, immediately after a successful LoadTreeStructure,
// since this project's own convention is that broken configuration
// fails loudly at startup rather than silently producing an
// unreachable Command Level.
func VerifyCommandLevels(levels *TreeStructure) []error {
	var problems []error

	for _, level := range levels.Order {
		if level.IsBase {
			continue
		}
		if level.EnterCommand == "" {
			problems = append(problems, fmt.Errorf("command level %q has no enter_command declared in tree_structure.yaml", level.Name))
			continue // nothing to look up in the registry below
		}
		if _, ok := lookupHandler(level.EnterCommand); !ok {
			problems = append(problems, fmt.Errorf("command level %q declares enter_command %q, but no cmd_*.go file has registered a command by that name", level.Name, level.EnterCommand))
		}
		if level.ExitCommand != "" {
			if _, ok := lookupHandler(level.ExitCommand); !ok {
				problems = append(problems, fmt.Errorf("command level %q declares exit_command %q, but no cmd_*.go file has registered a command by that name", level.Name, level.ExitCommand))
			}
		}
	}

	return problems
}

// VerifyVendorDefinedSecrets - This function checks a successfully
// loaded TreeStructure against the three rules var/tree/README.md
// documents for VendorDefinedPasswordHash, on both a CommandLevel and
// a Command, the same "run this once at startup, fail loudly, catch
// every problem in one pass" convention VerifyCommandLevels above
// already follows, and it is called from the same places, right after
// LoadTreeStructure at ordinary startup, and standalone under
// --check-config.
//
// A VendorDefinedPasswordHash exists to hold an implementer's own
// baked-in secret, deliberately kept out of an ordinary end user's
// reach, see that field's own doc comment in model.go for the full
// reasoning. These three rules are what keep that promise honest
// rather than aspirational:
//
//  1. PasswordHash and VendorDefinedPasswordHash MUST NOT both be set
//     on the same level or command. Nothing in this project ever
//     checks both at once, see EffectivePasswordHash, so allowing
//     both to be set would silently make PasswordHash dead
//     configuration, exactly the kind of mistake that stays invisible
//     until someone wonders why a password they set no longer works.
//  2. PasswordUserSettable MUST NOT be explicitly true alongside a
//     VendorDefinedPasswordHash. UserSettablePassword already
//     resolves this correctly at runtime regardless, VendorDefined
//     always wins, but a manifest that writes password_user_settable:
//     true right next to a vendor secret almost certainly reflects a
//     misunderstanding worth catching at startup rather than trusting
//     the resolution to quietly paper over it forever.
//  3. Hidden MUST be true alongside a VendorDefinedPasswordHash. A
//     vendor defined secret that is still openly listed in help output
//     and tab completion defeats the entire point of keeping it out of
//     an ordinary end user's reach.
func VerifyVendorDefinedSecrets(levels *TreeStructure) []error {
	var problems []error

	for _, level := range levels.Order {
		if level.VendorDefinedPasswordHash == "" {
			continue
		}
		if level.PasswordHash != "" {
			problems = append(problems, fmt.Errorf("command level %q sets both password_hash and vendor_defined_password_hash - remove one, they must never both be set on the same level", level.Name))
		}
		if level.PasswordUserSettable != nil && *level.PasswordUserSettable {
			problems = append(problems, fmt.Errorf("command level %q sets vendor_defined_password_hash and password_user_settable: true together - a vendor defined password must never be user settable", level.Name))
		}
		if !level.Hidden {
			problems = append(problems, fmt.Errorf("command level %q sets vendor_defined_password_hash but not hidden: true - a vendor defined password must always be hidden", level.Name))
		}
	}

	visited := make(map[*Command]bool)
	var walk func(path string, tree map[string]*Command)
	walk = func(path string, tree map[string]*Command) {
		for name, cmd := range tree {
			if visited[cmd] {
				continue
			}
			visited[cmd] = true
			full := name
			if path != "" {
				full = path + " " + name
			}
			if cmd.VendorDefinedPasswordHash != "" {
				if cmd.PasswordHash != "" {
					problems = append(problems, fmt.Errorf("command %q sets both password_hash and vendor_defined_password_hash - remove one, they must never both be set on the same command", full))
				}
				if cmd.PasswordUserSettable != nil && *cmd.PasswordUserSettable {
					problems = append(problems, fmt.Errorf("command %q sets vendor_defined_password_hash and password_user_settable: true together - a vendor defined password must never be user settable", full))
				}
				if !cmd.Hidden {
					problems = append(problems, fmt.Errorf("command %q sets vendor_defined_password_hash but not hidden: true - a vendor defined password must always be hidden", full))
				}
			}
			walk(full, cmd.Subcommands)
		}
	}
	for _, level := range levels.Order {
		walk("", level.Tree)
	}

	return problems
}
