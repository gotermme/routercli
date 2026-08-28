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
