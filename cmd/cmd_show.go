// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package cmd

import (
	"fmt"
	"sort"

	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/tokenize"
)

func init() {
	command.Register("show.version", func(ctx *command.AppContext, args []string) error {
		fmt.Println(ctx.Translator.T("show.version.text"))
		return nil
	})

	command.Register("show.running-config", func(ctx *command.AppContext, args []string) error {
		ctx.Logger.Debugln("DEBUG: generating running-config output")
		state := ctx.State.(*ExampleState)
		printRunningConfig(ctx, state)
		return nil
	})

	// interface and startup-config exist mainly to demonstrate, and
	// give a real test case for, a command tree with several siblings
	// sharing no forced prefix collision, the double-Tab design.
	// "show <TAB>" here has four candidates: interface, running-config,
	// startup-config, and version, and none of them should ever get
	// silently auto-picked. See
	// command/node_test.go's TestResolveMultipleSubtreeOptionsNeverAutoPicksOne
	// for the same principle locked down as a unit test.
	command.Register("show.interface", func(ctx *command.AppContext, args []string) error {
		state := ctx.State.(*ExampleState)
		if len(state.Interfaces) == 0 {
			fmt.Println(ctx.Translator.T("show.interface.text"))
			return nil
		}
		for _, name := range sortedInterfaceNames(state) {
			iface := state.Interfaces[name]
			status := ctx.Translator.T("show.interface.up")
			if iface.Shutdown {
				status = ctx.Translator.T("show.interface.admin_down")
			}
			fmt.Printf("%s: %s\n", name, status)
			if iface.Description != "" {
				fmt.Println("  " + ctx.Translator.T("show.interface.description_label", iface.Description))
			}
		}
		return nil
	})

	command.Register("show.startup-config", func(ctx *command.AppContext, args []string) error {
		fmt.Println(ctx.Translator.T("show.startup_config.text"))
		return nil
	})
}

// printRunningConfig - This function prints the current session state
// back out as a sequence of CLI commands, using
// tokenize.QuoteIfNeeded on every value so the output can be copied
// out of the terminal and pasted straight back in to reproduce the
// exact same state, covering hostname, per-interface state, and the
// top-level description. An empty or unset value is omitted at every
// level, matching how a real router only shows config lines for
// values that have actually been set. Interfaces are printed in
// sorted order for stable, deterministic output, since Go map
// iteration is randomized.
//
// The "!" comment lines and the indentation as nesting convention are
// deliberately not translated. They are Cisco config file syntax, not
// UI text, the same way a shell script's "#" shebang line is never
// translated either.
func printRunningConfig(ctx *command.AppContext, state *ExampleState) {
	fmt.Println("! (example running-config)")
	if state.Hostname != "" {
		fmt.Println("hostname", tokenize.QuoteIfNeeded(state.Hostname))
	}
	// Printed in its already hashed form, exactly like a real device's
	// "show running-config". The plaintext given to "password manager
	// <secret>" is never retained anywhere to print back out. See
	// cmd/cmd_password_manager.go. This iterates every Command Level,
	// not just one global secret, since a project can define any
	// number of privilege tiers, each with its own independently
	// configurable secret. See command.TreeStructure. A nested mode
	// such as config or config-if simply never has PasswordHash set,
	// so it falls through this loop without needing to be specially
	// excluded.
	for _, level := range ctx.Levels.Order {
		if level.PasswordHash != "" {
			fmt.Println("password manager", level.PasswordHash)
		}
	}
	if state.TerminalLength != 0 {
		fmt.Println("terminal length", state.TerminalLength)
	}
	if state.TerminalWidth != 0 {
		fmt.Println("terminal width", state.TerminalWidth)
	}
	if state.Description != "" {
		fmt.Println("set description", tokenize.QuoteIfNeeded(state.Description))
	}
	for _, name := range sortedInterfaceNames(state) {
		iface := state.Interfaces[name]
		if iface.Description == "" && !iface.Shutdown {
			// Nothing was ever actually configured on this interface,
			// so an empty block is not printed for it, the same
			// reasoning as omitting an unset Description above.
			continue
		}
		fmt.Println("interface", name)
		if iface.Description != "" {
			fmt.Println(" description", tokenize.QuoteIfNeeded(iface.Description))
		}
		if iface.Shutdown {
			fmt.Println(" shutdown")
		}
	}
	fmt.Println("!")
}

func sortedInterfaceNames(state *ExampleState) []string {
	names := make([]string, 0, len(state.Interfaces))
	for name := range state.Interfaces {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
