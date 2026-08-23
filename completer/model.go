// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package completer

import (
	"github.com/chzyer/readline"
	"github.com/gologme/log"
	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/i18n"
)

// ----------------------------------------------------------------------
// Define Object Model
// ----------------------------------------------------------------------

// NoopCompleter - This type satisfies readline's requirement that AutoComplete be non-nil,
// since Tab otherwise just bells if AutoComplete is nil. It
// deliberately does none of the actual completion work itself. See
// TreeListener below for why.
type NoopCompleter struct{}

// TreeListener - This type implements readline.Listener rather than readline's own
// AutoComplete interface, because chzyer/readline's AutoComplete contract
// only supports inserting a suffix at the cursor, a candidate continues
// the word already typed, and cannot rewrite earlier tokens on the line.
// Abbreviation expansion needs to rewrite every token, for example "sh
// run" becoming "show running-config", so this listens on every keypress
// instead, which fires with the buffer state after readline's own key
// handling ran and lets this type return a full replacement buffer.
//
// position holds a *command.CommandLevelStack rather than a static
// tree, and every OnChange call reads position.Current().Tree fresh.
// This is what makes tab completion Command Level aware: entering
// "configure terminal" changes what Tab completes without this type
// ever needing to be told directly, since it reads the same
// CommandLevelStack the dispatch loop mutates.
//
// lastAmbiguousInput and tapCount implement double Tab behavior matching
// real Cisco and HP devices: the first Tab press on an ambiguous prefix
// confirms or expands anything already resolved but stays quiet, and a
// second Tab press in a row on that exact same input is what prints the
// option list. Any other key, or a Tab on different input, resets the
// counter, so a sequence such as Tab, x, backspace, Tab is never misread
// as a double-tap on unrelated states.
type TreeListener struct {
	position   *command.CommandLevelStack
	instance   *readline.Instance
	logger     *log.Logger
	translator *i18n.Translator

	currentPrompt      string
	lastAmbiguousInput string
	tapCount           int
}

// ----------------------------------------------------------------------
// Initialization Functions
// ----------------------------------------------------------------------

// New - This function constructs a TreeListener bound to a given
// CommandLevelStack, a readline instance, needed to print the option
// list in the right place, a logger, for debug-level tracing of
// completion decisions, and a translator, which resolves a Command's
// ArgHelpKey, see the ArgHelp hint in OnChange.
//
// translator may be nil. Command.ResolvedArgHelp already handles a nil
// *i18n.Translator the same way ResolvedDesc and ResolvedHelp do, falling
// back to the literal ArgHelp field.
func New(position *command.CommandLevelStack, instance *readline.Instance, logger *log.Logger, translator *i18n.Translator) *TreeListener {
	return &TreeListener{position: position, instance: instance, logger: logger, translator: translator, tapCount: 0}
}
