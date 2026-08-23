// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"reflect"
	"testing"

	"github.com/gotermme/routercli/i18n"
)

// testTree - This function gives Resolve() a small tree with a nested
// level and a hidden sibling, enough to exercise abbreviation,
// ambiguity, and hidden command matching without pulling in the full
// example tree from main.go.
func testTree() map[string]*Command {
	return map[string]*Command{
		"show": {
			Subcommands: map[string]*Command{
				"interface":      {},
				"running-config": {},
				"startup-config": {},
				"version":        {},
				"secret-debug":   {Hidden: true},
			},
		},
		"set": {
			Subcommands: map[string]*Command{
				"description": {},
			},
		},
		"help": {},
		"exit": {},
	}
}

// TestResolveExactMatch - This test verifies that a full, exactly typed command
// path resolves to itself with no leftover arguments.
func TestResolveExactMatch(t *testing.T) {
	res := Resolve(testTree(), []string{"show", "version"})
	want := []string{"show", "version"}
	if !reflect.DeepEqual(res.FullName, want) {
		t.Errorf("FullName = %v, want %v", res.FullName, want)
	}
	if len(res.Args) != 0 {
		t.Errorf("Args = %v, want empty", res.Args)
	}
}

// TestResolveUniqueAbbreviation - This test verifies that "sh run" resolves the
// same as "show running-config", each token abbreviated independently
// against its own level of the tree.
func TestResolveUniqueAbbreviation(t *testing.T) {
	res := Resolve(testTree(), []string{"sh", "run"})
	want := []string{"show", "running-config"}
	if !reflect.DeepEqual(res.FullName, want) {
		t.Errorf("FullName = %v, want %v", res.FullName, want)
	}
}

// TestResolveAmbiguous - This test verifies that a prefix matching more than one
// top-level command, here "s" against both "show" and "set", is
// reported as ambiguous with both candidates listed.
func TestResolveAmbiguous(t *testing.T) {
	res := Resolve(testTree(), []string{"s"})
	if len(res.Ambiguous) != 2 {
		t.Fatalf("expected 2 ambiguous candidates, got %v", res.Ambiguous)
	}
	want := []string{"set", "show"}
	if !reflect.DeepEqual(res.Ambiguous, want) {
		t.Errorf("Ambiguous = %v, want %v", res.Ambiguous, want)
	}
}

// TestResolveUnknownCommand - This test verifies that a token matching nothing in
// the tree falls through as an argument, with a nil Command rather than
// an error.
func TestResolveUnknownCommand(t *testing.T) {
	res := Resolve(testTree(), []string{"bogus"})
	if res.Command != nil {
		t.Errorf("expected nil Command for unknown command, got %+v", res.Command)
	}
	if !reflect.DeepEqual(res.Args, []string{"bogus"}) {
		t.Errorf("Args = %v, want [bogus] (unmatched token falls through as an argument)", res.Args)
	}
}

// TestResolveArgumentsPastLeaf - This test verifies that once a command has no
// children, everything typed after it is treated as arguments rather
// than further command matching.
func TestResolveArgumentsPastLeaf(t *testing.T) {
	res := Resolve(testTree(), []string{"show", "version", "extra", "stuff"})
	want := []string{"show", "version"}
	if !reflect.DeepEqual(res.FullName, want) {
		t.Errorf("FullName = %v, want %v", res.FullName, want)
	}
	wantArgs := []string{"extra", "stuff"}
	if !reflect.DeepEqual(res.Args, wantArgs) {
		t.Errorf("Args = %v, want %v", res.Args, wantArgs)
	}
}

// TestResolveHiddenCommandNotAbbreviatable - This test verifies that a Hidden
// command cannot be reached through an abbreviated prefix, but still
// resolves normally when typed out in full.
func TestResolveHiddenCommandNotAbbreviatable(t *testing.T) {
	res := Resolve(testTree(), []string{"show", "secret"})
	if res.Command != nil && res.Command.Hidden {
		t.Errorf("hidden command should not be reachable via abbreviation, got %+v", res.FullName)
	}
	res2 := Resolve(testTree(), []string{"show", "secret-debug"})
	want := []string{"show", "secret-debug"}
	if !reflect.DeepEqual(res2.FullName, want) {
		t.Errorf("exact-match hidden command: FullName = %v, want %v", res2.FullName, want)
	}
}

// TestResolveEmptyTokenListsChildren - This test verifies that an empty trailing
// token, what the completer feeds in for a trailing space such as
// "show <TAB>", surfaces every non-hidden child as an ambiguous match
// rather than matching nothing.
func TestResolveEmptyTokenListsChildren(t *testing.T) {
	res := Resolve(testTree(), []string{"show", ""})
	want := []string{"interface", "running-config", "startup-config", "version"}
	if !reflect.DeepEqual(res.Ambiguous, want) {
		t.Errorf("Ambiguous = %v, want %v (secret-debug must stay excluded)", res.Ambiguous, want)
	}
}

// TestResolveMultipleSubtreeOptionsNeverAutoPicksOne - This test is
// the direct regression test for a core design principle. With
// several sibling options under one command, here "show interface",
// "show running-config", "show startup-config", and "show version",
// pressing Tab on "show " must never silently auto complete to one of
// them. There is no correct way to guess which one the user meant, so
// the only correct behavior is to present the full list and require
// the user to keep typing. This mirrors Cisco IOS and HP ProCurve,
// where auto completing an ambiguous multi-way choice would silently
// run the wrong command if the user just hit Enter right after Tab,
// expecting confirmation rather than a guess.
//
// This is deliberately distinct from the single remaining child case,
// see TestResolveUniqueAbbreviation, where exactly one candidate
// exists, so completing it is not a guess, it is the only possible
// answer.
func TestResolveMultipleSubtreeOptionsNeverAutoPicksOne(t *testing.T) {
	tree := testTree()
	res := Resolve(tree, []string{"show", ""})

	// res.Command correctly stops at the last unambiguous match, "show"
	// itself. It must not be any of the 4 sibling candidates, since
	// resolving into one of them would be exactly the silent guess this
	// test exists to rule out.
	if res.Command != tree["show"] {
		t.Errorf("res.Command should be the \"show\" command itself (last unambiguous match), got %+v", res.Command)
	}
	if len(res.FullName) != 1 || res.FullName[0] != "show" {
		t.Errorf("FullName should stop at the last unambiguous token (\"show\"), got %v", res.FullName)
	}
	if len(res.Ambiguous) != 4 {
		t.Fatalf("expected all 4 sibling options as candidates, got %v", res.Ambiguous)
	}
	want := map[string]bool{"interface": true, "running-config": true, "startup-config": true, "version": true}
	for _, c := range res.Ambiguous {
		if !want[c] {
			t.Errorf("unexpected candidate %q in Ambiguous", c)
		}
	}
}

// TestResolveUniquePrefixesAmongManySiblings - This test is the flip
// side of the test above, using the exact same four sibling commands.
// A prefix that narrows to exactly one of them must still resolve
// immediately, with no reason to make the user type the whole thing,
// even though several other siblings exist in the tree. Ambiguity is
// about how many candidates match what has actually been typed so
// far, not about how many siblings exist in total.
func TestResolveUniquePrefixesAmongManySiblings(t *testing.T) {
	cases := []struct {
		typed string
		want  string
	}{
		{"int", "interface"},        // only sibling starting with "int"
		{"run", "running-config"},   // only sibling starting with "run"
		{"start", "startup-config"}, // only sibling starting with "start"
		{"v", "version"},            // only sibling starting with "v"
	}
	for _, c := range cases {
		res := Resolve(testTree(), []string{"show", c.typed})
		want := []string{"show", c.want}
		if !reflect.DeepEqual(res.FullName, want) {
			t.Errorf("Resolve(show, %q).FullName = %v, want %v", c.typed, res.FullName, want)
		}
		if len(res.Ambiguous) != 0 {
			t.Errorf("Resolve(show, %q) should not be ambiguous, got candidates %v", c.typed, res.Ambiguous)
		}
	}
}

// TestResolveSharedPrefixAmongSiblingsIsAmbiguous - This test checks
// the genuinely shared leading prefix case. "running-config" and
// "startup-config" both contain "config", but that is not the
// relevant prefix, since resolution matches from the start of the
// token, not a substring anywhere inside it. Nothing in the fixture
// tree collides on a real leading substring today, so this documents
// that "s" alone resolves uniquely to "startup-config" right now. It
// is a placeholder for the day a second real "s" prefixed sibling
// gets added, for example "show summary", so this test starts failing
// the moment that happens and something has to consciously decide the
// new behavior, rather than the ambiguity rule silently degrading.
func TestResolveSharedPrefixAmongSiblingsIsAmbiguous(t *testing.T) {
	res := Resolve(testTree(), []string{"show", "s"})
	want := []string{"show", "startup-config"}
	if !reflect.DeepEqual(res.FullName, want) {
		t.Errorf("Resolve(show, \"s\").FullName = %v, want %v (only one sibling starts with \"s\" today)", res.FullName, want)
	}
}

// TestValidateArgsMinMax - This test verifies that ValidateArgs rejects both too
// few and too many arguments against MinArgs and MaxArgs, and accepts
// a count right in between.
func TestValidateArgsMinMax(t *testing.T) {
	cmd := &Command{MinArgs: intPtr(1), MaxArgs: intPtr(1)}

	if err := ValidateArgs(cmd, nil); err == nil {
		t.Error("expected error for too few arguments, got nil")
	}
	if err := ValidateArgs(cmd, []string{"a", "b"}); err == nil {
		t.Error("expected error for too many arguments, got nil")
	}
	if err := ValidateArgs(cmd, []string{"a"}); err != nil {
		t.Errorf("expected exactly 1 argument to be accepted, got error: %v", err)
	}
}

// TestValidateArgsUnsetMeansUnconstrained - This test verifies that the zero
// value Command, which is what every leaf command has unless it opts in,
// does not silently reject arguments. A plain int with a 0 default
// for MinArgs and MaxArgs would otherwise have caused exactly that.
func TestValidateArgsUnsetMeansUnconstrained(t *testing.T) {
	cmd := &Command{}
	if err := ValidateArgs(cmd, []string{"a", "b", "c"}); err != nil {
		t.Errorf("unset MinArgs/MaxArgs should allow any argument count, got error: %v", err)
	}
}

// TestValidateArgsMaxLength - This test verifies that ValidateArgs accepts an
// argument right at MaxArgLength and rejects one past it.
func TestValidateArgsMaxLength(t *testing.T) {
	cmd := &Command{MaxArgLength: 5}
	if err := ValidateArgs(cmd, []string{"short"}); err != nil {
		t.Errorf("5-char argument should fit a 5-char limit, got error: %v", err)
	}
	if err := ValidateArgs(cmd, []string{"toolong"}); err == nil {
		t.Error("expected error for an argument past MaxArgLength, got nil")
	}
}

// TestResolveNegatedCommand - This test locks down the core "no"
// mechanism. "no shutdown" must resolve to the same Command as
// "shutdown" alone, with Negated set to true and "no" itself never
// appearing anywhere in FullName. This is what lets a single handler
// serve both directions, see registry.go's AppContext.Negated doc
// comment.
func TestResolveNegatedCommand(t *testing.T) {
	tree := map[string]*Command{
		"shutdown": {Negatable: true},
	}
	res := Resolve(tree, []string{"no", "shutdown"})

	if !res.Negated {
		t.Error("Negated should be true for a \"no\" prefixed command")
	}
	want := []string{"shutdown"}
	if !reflect.DeepEqual(res.FullName, want) {
		t.Errorf("FullName = %v, want %v (must not include \"no\")", res.FullName, want)
	}
	if res.Command != tree["shutdown"] {
		t.Errorf("Command = %+v, want the shutdown command itself", res.Command)
	}
}

// TestResolveNegatedCommandWithAbbreviation - This test verifies that the real
// command name after "no" still goes through the normal abbreviation
// matching logic, so "no shut" resolves the same as "no shutdown"
// when "shutdown" is the only match.
func TestResolveNegatedCommandWithAbbreviation(t *testing.T) {
	tree := map[string]*Command{
		"shutdown": {Negatable: true},
	}
	res := Resolve(tree, []string{"no", "shut"})
	want := []string{"shutdown"}
	if !reflect.DeepEqual(res.FullName, want) {
		t.Errorf("FullName = %v, want %v", res.FullName, want)
	}
	if !res.Negated {
		t.Error("Negated should be true")
	}
}

// TestResolveNegatedAmbiguous - This test verifies that Negated is still reported
// true even when the command name after "no" is itself ambiguous.
func TestResolveNegatedAmbiguous(t *testing.T) {
	tree := map[string]*Command{
		"shutdown":    {Negatable: true},
		"show-secret": {},
	}
	res := Resolve(tree, []string{"no", "sh"})
	if !res.Negated {
		t.Error("Negated should be true even when the rest is ambiguous")
	}
	if len(res.Ambiguous) != 2 {
		t.Errorf("expected 2 ambiguous candidates after \"no\", got %v", res.Ambiguous)
	}
}

// TestResolveNegatedUnknownCommand - This test verifies that Negated is still
// reported true even when the command name after "no" does not match
// anything at all.
func TestResolveNegatedUnknownCommand(t *testing.T) {
	tree := map[string]*Command{"shutdown": {Negatable: true}}
	res := Resolve(tree, []string{"no", "bogus"})
	if !res.Negated {
		t.Error("Negated should still be true even for an unknown command")
	}
	if res.Command != nil {
		t.Errorf("expected no Command for an unknown negated command, got %+v", res.Command)
	}
}

// TestResolveBareNoListsChildren - This test verifies that "no <TAB>", an empty
// token after "no", lists the same candidates as a bare Tab at this
// level would. The completer relies on this to offer completions
// after "no ".
func TestResolveBareNoListsChildren(t *testing.T) {
	tree := map[string]*Command{
		"shutdown":    {Negatable: true},
		"description": {Negatable: true},
	}
	res := Resolve(tree, []string{"no", ""})
	if !res.Negated {
		t.Error("Negated should be true")
	}
	if len(res.Ambiguous) != 2 {
		t.Errorf("expected both children listed after bare \"no \", got %v", res.Ambiguous)
	}
}

// TestResolveNonNegatedUnaffected - This test verifies that a plain command with
// no "no" prefix has Negated false, guarding against a regression
// where Negated accidentally defaults to something other than false.
func TestResolveNonNegatedUnaffected(t *testing.T) {
	tree := map[string]*Command{"shutdown": {Negatable: true}}
	res := Resolve(tree, []string{"shutdown"})
	if res.Negated {
		t.Error("Negated should be false when the line did not start with \"no\"")
	}
}

// TestCommandResolvedDescHelpArgHelp - This test verifies that
// ResolvedDesc, ResolvedHelp, and ResolvedArgHelp each prefer their
// translated form when a *Translator and the matching *Key field are
// both set, and fall back to the literal field, Desc, Help, or
// ArgHelp, when the translator is nil, matching how a tree with i18n
// never wired in at all is meant to work.
func TestCommandResolvedDescHelpArgHelp(t *testing.T) {
	catalog := i18n.Catalog{
		"show.desc":    "translated desc",
		"show.help":    "translated help",
		"show.arghelp": "translated arghelp",
	}
	tr := i18n.New(map[string]i18n.Catalog{"en": catalog}, "en", "en")

	n := &Command{
		Desc: "literal desc", DescKey: "show.desc",
		Help: "literal help", HelpKey: "show.help",
		ArgHelp: "literal arghelp", ArgHelpKey: "show.arghelp",
	}

	if got := n.ResolvedDesc(tr); got != "translated desc" {
		t.Errorf("ResolvedDesc with a translator = %q, want %q", got, "translated desc")
	}
	if got := n.ResolvedHelp(tr); got != "translated help" {
		t.Errorf("ResolvedHelp with a translator = %q, want %q", got, "translated help")
	}
	if got := n.ResolvedArgHelp(tr); got != "translated arghelp" {
		t.Errorf("ResolvedArgHelp with a translator = %q, want %q", got, "translated arghelp")
	}

	if got := n.ResolvedDesc(nil); got != "literal desc" {
		t.Errorf("ResolvedDesc with a nil translator = %q, want the literal Desc %q", got, "literal desc")
	}
	if got := n.ResolvedHelp(nil); got != "literal help" {
		t.Errorf("ResolvedHelp with a nil translator = %q, want the literal Help %q", got, "literal help")
	}
	if got := n.ResolvedArgHelp(nil); got != "literal arghelp" {
		t.Errorf("ResolvedArgHelp with a nil translator = %q, want the literal ArgHelp %q", got, "literal arghelp")
	}
}
