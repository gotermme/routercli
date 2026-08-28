// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"reflect"
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

	text := HelpText(tree, nil, DefaultListOptions())
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

	text := HelpText(tree, nil, DefaultListOptions())
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
	text := HelpText(map[string]*Command{}, nil, DefaultListOptions())
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
	got := HelpForPath(tree, []string{"show", ""}, nil, DefaultListOptions())

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
	got := HelpForPath(tree, nil, nil, DefaultListOptions())

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
	got := HelpForPath(tree, []string{"terminal", "length", ""}, nil, DefaultListOptions())

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
	got := HelpForPath(tree, []string{"bogus"}, nil, DefaultListOptions())
	if got != "" {
		t.Errorf("HelpForPath(bogus ?) = %q, want empty string", got)
	}
}

// TestHelpForPathLeafWithNoArgumentShowsCRHint - This test verifies
// that a leaf command taking no arguments, no ArgHelp or ArgHelpKey
// configured, shows the "<cr>" placeholder, real Cisco and HP
// notation for "you can press enter here", rather than an empty hint
// or the "you need an argument" form the other leaf test covers.
func TestHelpForPathLeafWithNoArgumentShowsCRHint(t *testing.T) {
	tree := map[string]*Command{
		"exit": {Desc: "Exit the CLI", RunFunc: func(*AppContext, []string) error { return nil }},
	}
	got := HelpForPath(tree, []string{"exit"}, nil, DefaultListOptions())
	if got != " <cr>\n" {
		t.Errorf("HelpForPath(exit ?) = %q, want %q", got, " <cr>\n")
	}
}

// TestHelpForPathNegatedAmbiguousEmptyTokenShowsFullHelp - This test
// verifies the negated companion to
// TestHelpForPathContainerListsSubcommandsWithDescriptions: "no show
// ?" must adjust AmbigAt for the stripped leading "no" before checking
// whether the ambiguous token is empty, or it would consult the wrong
// index into tokens and fall through to the bare word-help form
// instead of the full, described listing.
func TestHelpForPathNegatedAmbiguousEmptyTokenShowsFullHelp(t *testing.T) {
	tree := helpForPathTestTree()
	got := HelpForPath(tree, []string{"no", "show", ""}, nil, DefaultListOptions())

	for _, want := range []string{"version", "running-config", "startup-config", "Show version"} {
		if !strings.Contains(got, want) {
			t.Errorf("HelpForPath(no show ?) = %q, expected it to contain %q", got, want)
		}
	}
}

// TestHelpForPathNegatedAmbiguousPartialWordListsPlainNames - This
// test verifies the negated companion to
// TestHelpForPathAmbiguousListsPlainNames: "no s?" must apply the same
// AmbigAt adjustment for a genuinely partial word, landing on the
// bare, undescribed word-help form rather than panicking or reading
// the wrong token.
func TestHelpForPathNegatedAmbiguousPartialWordListsPlainNames(t *testing.T) {
	tree := map[string]*Command{
		"show": {Desc: "Show things", Negatable: true, RunFunc: func(*AppContext, []string) error { return nil }},
		"set":  {Desc: "Set things", Negatable: true, RunFunc: func(*AppContext, []string) error { return nil }},
	}
	got := HelpForPath(tree, []string{"no", "s"}, nil, DefaultListOptions())

	if !strings.Contains(got, "show") || !strings.Contains(got, "set") {
		t.Errorf("HelpForPath(no s?) = %q, expected both \"show\" and \"set\" listed", got)
	}
	if strings.Contains(got, "Show things") {
		t.Errorf("HelpForPath(no s?) = %q, expected bare names only (word help), not descriptions", got)
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
	got := HelpForPath(tree, []string{"s"}, nil, DefaultListOptions())

	if !strings.Contains(got, "show") || !strings.Contains(got, "set") {
		t.Errorf("HelpForPath(s?) = %q, expected both \"show\" and \"set\" listed", got)
	}
	if strings.Contains(got, "Show things") {
		t.Errorf("HelpForPath(s?) = %q, expected bare names only (word help), not descriptions", got)
	}
}

// ----------------------------------------------------------------------
//
// "<cr>" alongside a runnable container's own subcommands
//
// ----------------------------------------------------------------------

// crTestTree - This function mirrors real "totp enable" versus
// "totp enable qr": "enable" is a complete, runnable command on its
// own, but also has exactly one subcommand, "qr", below it.
func crTestTree() map[string]*Command {
	return map[string]*Command{
		"totp": {
			Desc: "TOTP settings",
			Subcommands: map[string]*Command{
				"enable": {
					Desc:    "Enable TOTP",
					RunFunc: func(*AppContext, []string) error { return nil },
					Subcommands: map[string]*Command{
						"qr": {Desc: "Show a QR code", RunFunc: func(*AppContext, []string) error { return nil }},
					},
				},
				// A second sibling, itself with no children of its own,
				// keeps "totp" from having only one subcommand, so
				// "totp ?" cannot auto-descend straight through it the
				// way TestResolveNonRunnableContainerWithSoleSubcommandStillAutoDescends
				// in command_test.go covers. Without this, "totp" alone
				// would never actually be exercised by
				// TestHelpForPathContainerRunnableAsIsAppendsCR below.
				"disable": {Desc: "Disable TOTP", RunFunc: func(*AppContext, []string) error { return nil }},
			},
		},
	}
}

// TestHelpForPathAmbiguousRunnableAsIsAppendsCR - This test verifies
// that "totp enable ?", nothing typed yet after "enable", shows both
// "qr" and "<cr>" together, matching real Cisco and HP: "enable" is
// already a complete command, but "qr" is also still available below
// it. "<cr>" must come after "qr", never sorted in among the real
// names.
func TestHelpForPathAmbiguousRunnableAsIsAppendsCR(t *testing.T) {
	got := HelpForPath(crTestTree(), []string{"totp", "enable", ""}, nil, DefaultListOptions())

	wantOrder := []string{"qr", "<cr>"}
	for i, want := range wantOrder {
		if !strings.Contains(got, want) {
			t.Fatalf("HelpForPath(totp enable ?) = %q, expected it to contain %q", got, want)
		}
		if i > 0 && strings.Index(got, wantOrder[i-1]) > strings.Index(got, want) {
			t.Errorf("HelpForPath(totp enable ?) = %q, expected %q before %q", got, wantOrder[i-1], want)
		}
	}
}

// TestHelpForPathContainerRunnableAsIsAppendsCR - This test verifies
// the same "<cr>" addition on the full, described HelpText form: "totp
// ?" where "totp" itself is not runnable shows no "<cr>", but a
// container that is itself directly runnable, exercised here by
// resolving straight to "enable" and asking for its own subcommand
// listing, does.
func TestHelpForPathContainerRunnableAsIsAppendsCR(t *testing.T) {
	tree := crTestTree()

	gotTotp := HelpForPath(tree, []string{"totp", ""}, nil, DefaultListOptions())
	if strings.Contains(gotTotp, "<cr>") {
		t.Errorf("HelpForPath(totp ?) = %q, expected no <cr>: \"totp\" itself is not runnable", gotTotp)
	}

	gotEnable := HelpForPath(tree, []string{"totp", "enable", ""}, nil, DefaultListOptions())
	if !strings.Contains(gotEnable, "<cr>") {
		t.Errorf("HelpForPath(totp enable ?) = %q, expected <cr>: \"enable\" is itself runnable", gotEnable)
	}
}

// TestHelpForPathLeafWithUnsatisfiedMinArgsAndNoArgHelpReturnsEmpty -
// This test verifies the fix to a latent bug: a leaf command whose
// MinArgs is not yet satisfied, and which defines no ArgHelp or
// ArgHelpKey to hint with, must show nothing at all, not the old
// behavior of falling back to a misleading "<cr>" that implied Enter
// could be pressed when it could not.
func TestHelpForPathLeafWithUnsatisfiedMinArgsAndNoArgHelpReturnsEmpty(t *testing.T) {
	tree := map[string]*Command{
		"secret": {RunFunc: func(*AppContext, []string) error { return nil }, MinArgs: intPtr(1)},
	}
	got := HelpForPath(tree, []string{"secret"}, nil, DefaultListOptions())
	if got != "" {
		t.Errorf("HelpForPath(secret ?) = %q, want empty string, not a misleading <cr>", got)
	}
}

// TestHelpForPathLeafWithOptionalArgumentShowsHintAndCR - This test
// verifies that a leaf command which is already runnable as is,
// MinArgs unset or zero, but which also defines an ArgHelp for an
// optional argument, shows both the hint and "<cr>" together, hint
// first, matching real Cisco's own notation for "you can press enter
// here, or keep typing an optional argument".
func TestHelpForPathLeafWithOptionalArgumentShowsHintAndCR(t *testing.T) {
	tree := map[string]*Command{
		"traceroute": {
			RunFunc: func(*AppContext, []string) error { return nil },
			ArgHelp: "<host>  Optional destination to trace",
		},
	}
	got := HelpForPath(tree, []string{"traceroute"}, nil, DefaultListOptions())

	if !strings.Contains(got, "<host>") || !strings.Contains(got, "<cr>") {
		t.Errorf("HelpForPath(traceroute ?) = %q, expected both the ArgHelp hint and <cr>", got)
	}
	if strings.Index(got, "<host>") > strings.Index(got, "<cr>") {
		t.Errorf("HelpForPath(traceroute ?) = %q, expected the argument hint before <cr>", got)
	}
}

// ----------------------------------------------------------------------
//
// SortCommandNames
//
// ----------------------------------------------------------------------

// sortCommandNamesTestTree - This function gives SortCommandNames a
// small tree with two ordinary commands and two common commands,
// IsCommonCommand true, DefIndex values chosen so that alphabetical
// order and DefIndex order disagree, so a test can tell which one
// actually drove a given result.
func sortCommandNamesTestTree() map[string]*Command {
	return map[string]*Command{
		"zebra": {DefIndex: 0},
		"alpha": {DefIndex: 1},
		"help":  {DefIndex: 0, IsCommonCommand: true},
		"exit":  {DefIndex: 1, IsCommonCommand: true},
	}
}

// TestSortCommandNamesAlphabeticalMerged - This test verifies this
// project's own default: every name sorted together, purely
// alphabetically, common commands included, matching real Cisco and
// HP's own top-level listings.
func TestSortCommandNamesAlphabeticalMerged(t *testing.T) {
	tree := sortCommandNamesTestTree()
	names := []string{"zebra", "alpha", "help", "exit"}

	got := SortCommandNames(names, tree, ListOptions{Alphabetical: true, MergeCommon: true})
	want := []string{"alpha", "exit", "help", "zebra"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SortCommandNames(alphabetical, merged) = %v, want %v", got, want)
	}
}

// TestSortCommandNamesAlphabeticalAppended - This test verifies that
// turning MergeCommonCommands off keeps alphabetical order within each
// group, but moves every common command after every non-common one.
func TestSortCommandNamesAlphabeticalAppended(t *testing.T) {
	tree := sortCommandNamesTestTree()
	names := []string{"zebra", "alpha", "help", "exit"}

	got := SortCommandNames(names, tree, ListOptions{Alphabetical: true, MergeCommon: false})
	want := []string{"alpha", "zebra", "exit", "help"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SortCommandNames(alphabetical, appended) = %v, want %v", got, want)
	}
}

// TestSortCommandNamesDefinitionOrder - This test verifies that
// AlphabeticalCommandOrder false sorts by DefIndex instead, own
// commands before common commands regardless of MergeCommon, since
// there is no single true combined definition order across two
// separate tree files, see SortCommandNames's own doc comment.
func TestSortCommandNamesDefinitionOrder(t *testing.T) {
	tree := sortCommandNamesTestTree()
	names := []string{"zebra", "alpha", "help", "exit"}

	for _, mergeCommon := range []bool{true, false} {
		got := SortCommandNames(names, tree, ListOptions{Alphabetical: false, MergeCommon: mergeCommon})
		want := []string{"zebra", "alpha", "help", "exit"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("SortCommandNames(definition order, MergeCommon=%v) = %v, want %v", mergeCommon, got, want)
		}
	}
}

// TestSortCommandNamesDoesNotMutateInput - This test verifies that
// SortCommandNames returns a new slice rather than sorting names in
// place, so a caller holding on to the original, such as
// ResolveResult.Ambiguous, never sees it silently reordered.
func TestSortCommandNamesDoesNotMutateInput(t *testing.T) {
	tree := sortCommandNamesTestTree()
	names := []string{"zebra", "alpha"}
	original := append([]string(nil), names...)

	SortCommandNames(names, tree, ListOptions{Alphabetical: true, MergeCommon: true})

	if !reflect.DeepEqual(names, original) {
		t.Errorf("SortCommandNames mutated its input: got %v, want unchanged %v", names, original)
	}
}
