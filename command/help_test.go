// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"strings"
	"testing"
)

// TestHelpTextOneLevelOnly - This test verifies that HelpText lists
// only the level it is called from. A container command like "show"
// appears as one line, using its own Desc, and none of its children,
// "version" and "secret", appear at all. The line count is checked
// directly, by name, rather than by substring, since "show" is a
// prefix of "show version", and a naive substring check on that
// longer path could pass even if the recursive listing regression
// came back.
func TestHelpTextOneLevelOnly(t *testing.T) {
	tree := map[string]*Command{
		"show": {
			Desc: "Show things",
			Subcommands: map[string]*Command{
				"version": {Desc: "Show version", RunFunc: func(*AppContext, []string) error { return nil }},
				"secret":  {Desc: "Should never appear", Hidden: true, RunFunc: func(*AppContext, []string) error { return nil }},
			},
		},
		"exit": {Desc: "Exit the CLI", RunFunc: func(*AppContext, []string) error { return nil }},
	}

	text := HelpText(tree, nil)
	lines := strings.Split(strings.TrimSpace(text), "\n")

	if !strings.Contains(text, "show") || !strings.Contains(text, "Show things") {
		t.Errorf("expected help text to include \"show\" with its own description, got:\n%s", text)
	}
	if strings.Contains(text, "version") {
		t.Errorf("help text recursed in to \"show\"'s children - it should list \"show\" only, got:\n%s", text)
	}
	if strings.Contains(text, "secret") {
		t.Errorf("hidden command leaked in to help text:\n%s", text)
	}
	// header + show + exit, nothing more.
	if len(lines) != 3 {
		t.Errorf("expected exactly 3 lines (header, show, exit), got %d:\n%s", len(lines), text)
	}
}

// TestHelpTextExcludesHiddenAtTopLevel - This test verifies that a Hidden node at
// the level actually being listed is excluded, the direct case rather
// than TestHelpTextOneLevelOnly's indirect one, where a hidden node
// several levels down never gets a chance to matter at all because
// HelpText does not recurse.
func TestHelpTextExcludesHiddenAtTopLevel(t *testing.T) {
	tree := map[string]*Command{
		"help":  {Desc: "Display available commands", RunFunc: func(*AppContext, []string) error { return nil }},
		"exit":  {Desc: "Exit the CLI", RunFunc: func(*AppContext, []string) error { return nil }},
		"\"?\"": {Desc: "Display available commands", Alias: "help", Hidden: true},
	}

	text := HelpText(tree, nil)
	if !strings.Contains(text, "help") || !strings.Contains(text, "exit") {
		t.Errorf("expected help text to include \"help\" and \"exit\", got:\n%s", text)
	}
	if strings.Contains(text, "?") {
		t.Errorf("hidden alias leaked in to help text:\n%s", text)
	}
}

// TestHelpTextEmptyTree - This test verifies that an empty command tree still
// produces the header line, rather than an empty string.
func TestHelpTextEmptyTree(t *testing.T) {
	text := HelpText(map[string]*Command{}, nil)
	if !strings.Contains(text, "Available commands:") {
		t.Errorf("expected a header even for an empty tree, got:\n%s", text)
	}
}

// ----------------------------------------------------------------------
//
// HelpForPath, the "show ?" and "?" contextual help lookup
//
// ----------------------------------------------------------------------

func helpForPathTestTree() map[string]*Command {
	return map[string]*Command{
		"show": {
			Desc: "Show things",
			Subcommands: map[string]*Command{
				"version":        {Desc: "Show version", RunFunc: func(*AppContext, []string) error { return nil }},
				"running-config": {Desc: "Show running config", RunFunc: func(*AppContext, []string) error { return nil }},
				"startup-config": {Desc: "Show startup config", RunFunc: func(*AppContext, []string) error { return nil }},
			},
		},
		"terminal": {
			Subcommands: map[string]*Command{
				"length": {
					Desc:    "Set terminal length",
					RunFunc: func(*AppContext, []string) error { return nil },
					MinArgs: intPtr(1),
					ArgHelp: "<2-1000>  Enter a number for the 'length' command/parameter.",
				},
			},
		},
	}
}

// intPtr is defined in node.go (package-level helper, already used
// throughout this package's own tests).

// TestHelpForPathContainerListsSubcommandsWithDescriptions - This test verifies
// that "show ?" lists every subcommand of "show" along with its
// description, the real Cisco and HP "full help" form, rather than
// the bare names only "word help" form.
func TestHelpForPathContainerListsSubcommandsWithDescriptions(t *testing.T) {
	tree := helpForPathTestTree()
	got := HelpForPath(tree, []string{"show", ""}, nil)

	for _, want := range []string{"version", "running-config", "startup-config", "Show version"} {
		if !strings.Contains(got, want) {
			t.Errorf("HelpForPath(show ?) = %q, expected it to contain %q", got, want)
		}
	}
}

// TestHelpForPathBarePromptListsTopLevelNames - This test verifies that "?"
// pressed at a bare prompt, or the equivalent piped "?" line, lists
// every top-level command name.
func TestHelpForPathBarePromptListsTopLevelNames(t *testing.T) {
	tree := helpForPathTestTree()
	got := HelpForPath(tree, nil, nil)

	for _, want := range []string{"show", "terminal"} {
		if !strings.Contains(got, want) {
			t.Errorf("HelpForPath(?) = %q, expected it to contain %q", got, want)
		}
	}
}

// TestHelpForPathLeafWithArgumentShowsArgHelpHint - This test verifies that
// "terminal length ?", a leaf command that takes an argument, shows
// its ArgHelp hint. It has no subcommand listing to fall back to, and
// pressing Enter here is not valid since an argument is required, so
// neither of those forms should appear instead.
func TestHelpForPathLeafWithArgumentShowsArgHelpHint(t *testing.T) {
	tree := helpForPathTestTree()
	got := HelpForPath(tree, []string{"terminal", "length", ""}, nil)

	if !strings.Contains(got, "<2-1000>") {
		t.Errorf("HelpForPath(terminal length ?) = %q, expected the ArgHelp hint", got)
	}
}

// TestHelpForPathUnknownPathReturnsEmpty - This test verifies that "bogus ?"
// returns an empty string, since nothing resolves at all and there is
// nothing meaningful to show. Callers treat an empty string as print
// nothing rather than guessing at an error message.
func TestHelpForPathUnknownPathReturnsEmpty(t *testing.T) {
	tree := helpForPathTestTree()
	got := HelpForPath(tree, []string{"bogus"}, nil)
	if got != "" {
		t.Errorf("HelpForPath(bogus ?) = %q, want empty string", got)
	}
}

// TestHelpForPathAmbiguousListsPlainNames - This test verifies that a partial word
// matching more than one candidate, for example "s?" when both "show"
// and "set" exist, is listed as bare names, matching Tab completion's
// own ambiguous listing convention, rather than the full HelpText form
// with descriptions.
func TestHelpForPathAmbiguousListsPlainNames(t *testing.T) {
	tree := map[string]*Command{
		"show": {Desc: "Show things", RunFunc: func(*AppContext, []string) error { return nil }},
		"set":  {Desc: "Set things", RunFunc: func(*AppContext, []string) error { return nil }},
	}
	got := HelpForPath(tree, []string{"s"}, nil)

	if !strings.Contains(got, "show") || !strings.Contains(got, "set") {
		t.Errorf("HelpForPath(s?) = %q, expected both \"show\" and \"set\" listed", got)
	}
	if strings.Contains(got, "Show things") {
		t.Errorf("HelpForPath(s?) = %q, expected bare names only (word help), not descriptions", got)
	}
}
