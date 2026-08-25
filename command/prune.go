// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import "fmt"

// ----------------------------------------------------------------------
// Public Functions - Tree Pruning
// ----------------------------------------------------------------------

// PruneDisabledCommands - This function removes every command in tree
// whose Requires names a flag that is false in enabled, along with
// that command's own Subcommands, recursively. This exists so a
// project can turn a whole feature, password change or TOTP
// enrollment for instance, off through configuration and have the
// matching command actually disappear from the tree, rather than
// staying reachable and failing, or worse, silently doing nothing,
// once run. See config.SystemConfig.EnableCLIAuthentication and
// EnableTOTPAuthentication, and main.go, which builds enabled from
// those settings and calls this once per Command Level right after
// command.LoadTreeStructure returns.
//
// A Requires naming a flag that is not a key in enabled at all is a
// hard error, the same fail loudly convention every other malformed
// piece of configuration in this project follows, see LoadTree's own
// doc comment for the same reasoning applied to an unresolved "run:"
// handler name. A tree file with "requires: totp_typo" would
// otherwise leave that command reachable forever, its author
// believing it was gated on a flag that does not exist. path is used
// only to build that error message, naming the actual command that
// referenced the bad flag, the same pathSoFar convention
// resolveHandlers already uses in loader.go.
//
// A command whose own Requires is satisfied, true, empty, or absent,
// still has its Subcommands walked, so a feature can gate one
// container command, totp for instance, while a nested command
// beneath it carries its own, independent Requires if a project ever
// needs that.
//
// This mutates tree in place, deleting a disabled command's own entry
// from tree directly, which is safe for a level's own top-level Tree
// map, freshly built per level by LoadTreeStructure, see MergeTrees.
// It is not safe to call this more than once against the same nested
// Subcommands map reached through more than one Command Level's Tree
// at once, for example a node inherited by more than one level
// through InheritParent, since MergeTrees only rebuilds the top-level
// map at each merge and reuses the same *Command pointers, and so the
// same underlying Subcommands map, beneath it. A project wanting
// Requires on a command reached that way should prune once against
// whichever level actually owns that command's tree file, before it
// is ever merged into another level, rather than calling this
// separately against every level that ends up inheriting it. Every
// command shipped with this project that sets Requires, see
// var/tree/level_user.yaml, is defined directly in a level with
// inherit_parent: false, and reached by no other level, so this
// caveat does not apply to them.
func PruneDisabledCommands(tree map[string]*Command, enabled map[string]bool, path string) error {
	for name, c := range tree {
		commandPath := name
		if path != "" {
			commandPath = path + " " + name
		}

		if c.Requires != "" {
			on, known := enabled[c.Requires]
			if !known {
				return fmt.Errorf("command %q sets requires %q, but that is not a recognized flag name", commandPath, c.Requires)
			}
			if !on {
				delete(tree, name)
				continue
			}
		}

		if err := PruneDisabledCommands(c.Subcommands, enabled, commandPath); err != nil {
			return err
		}
	}
	return nil
}
