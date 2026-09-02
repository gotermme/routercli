// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/i18n"
)

// TestShowVersionHandlerReturnsNoError - This test verifies that
// "show version" runs without error and prints something, the
// minimum guarantee for a command with nothing else to observe.
func TestShowVersionHandlerReturnsNoError(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "show.version")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Errorf("show.version handler returned unexpected error: %v", runErr)
	}
	if out == "" {
		t.Error("expected show.version to print something")
	}
}

// ----------------------------------------------------------------------
//
// show.history / historyLines
//
// ----------------------------------------------------------------------

// TestShowHistoryHandlerMissingFilePrintsEmptyMessage - This test
// verifies that a HistoryFile that does not exist yet, nothing typed
// against it yet in any session, is treated the same as an empty
// history, not an error, mirroring "show startup-config" own missing
// file treatment.
func TestShowHistoryHandlerMissingFilePrintsEmptyMessage(t *testing.T) {
	ctx := newTestContext()
	ctx.HistoryFile = filepath.Join(t.TempDir(), "does-not-exist.log")
	ctx.DefaultHistorySize = 500
	cmd := loadTestCommand(t, "show.history")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Fatalf("show.history handler returned unexpected error: %v", runErr)
	}
	// ctx.Translator is nil here, the same minimal newTestContext every
	// other handler test in this package uses, so T() returns the
	// bracketed key itself rather than real catalog text, see
	// i18n.Translator.T's own doc comment. Checking for that bracketed
	// key is still a real, deterministic assertion that the empty
	// branch ran, not the one that prints file content.
	if !strings.Contains(out, "[[show.history.empty]]") {
		t.Errorf("output = %q, want the empty history message", out)
	}
}

// TestShowHistoryHandlerPrintsFileContent - This test verifies that
// "show history" prints back every line of a HistoryFile that fits
// within DefaultHistorySize, in the same order the file itself holds
// them, oldest first.
func TestShowHistoryHandlerPrintsFileContent(t *testing.T) {
	ctx := newTestContext()
	path := filepath.Join(t.TempDir(), "history.log")
	if err := os.WriteFile(path, []byte("show version\nshow terminal\nexit\n"), 0644); err != nil {
		t.Fatalf("failed to write test history file: %v", err)
	}
	ctx.HistoryFile = path
	ctx.DefaultHistorySize = 500
	cmd := loadTestCommand(t, "show.history")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Fatalf("show.history handler returned unexpected error: %v", runErr)
	}
	want := "show version\nshow terminal\nexit\n"
	if out != want {
		t.Errorf("output = %q, want %q", out, want)
	}
}

// TestShowHistoryHandlerTruncatesToEffectiveHistorySize - This test
// verifies that only the last EffectiveHistorySize lines of the file
// are shown when the file itself holds more than that, the most
// recent commands, not the oldest ones, matching what an operator
// actually wants from a bounded "show history".
func TestShowHistoryHandlerTruncatesToEffectiveHistorySize(t *testing.T) {
	ctx := newTestContext()
	path := filepath.Join(t.TempDir(), "history.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\nfour\nfive\n"), 0644); err != nil {
		t.Fatalf("failed to write test history file: %v", err)
	}
	ctx.HistoryFile = path
	two := 2
	ctx.HistorySize = &two
	cmd := loadTestCommand(t, "show.history")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Fatalf("show.history handler returned unexpected error: %v", runErr)
	}
	want := "four\nfive\n"
	if out != want {
		t.Errorf("output = %q, want %q (the last 2 lines only)", out, want)
	}
}

// ----------------------------------------------------------------------
//
// show.aliases / aliasesLines
//
// ----------------------------------------------------------------------

// TestShowAliasesHandlerNoAliasesPrintsEmptyMessage - This test
// verifies that a deployment with no runtime defined alias anywhere,
// the state every fresh CommandLevel starts in, prints the "no
// aliases" message rather than an empty, silent listing.
func TestShowAliasesHandlerNoAliasesPrintsEmptyMessage(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{
		Order: []*command.CommandLevel{
			{Name: "exec"},
		},
	}
	cmd := loadTestCommand(t, "show.aliases")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Fatalf("show.aliases handler returned unexpected error: %v", runErr)
	}
	if !strings.Contains(out, "[[show.aliases.empty]]") {
		t.Errorf("output = %q, want the empty aliases message", out)
	}
}

// TestAliasesLinesListsEveryLevelSortedByAliasName - This test
// verifies aliasesLines directly, rather than through "show aliases"
// itself, since ctx.Translator is nil in every handler test in this
// package, see newTestContext's own doc comment, which would leave
// i18n.Translator.T returning only a bracketed key with no argument
// substitution, unable to prove ordering or content either way. It
// verifies that every level in ctx.Levels.Order carrying at least one
// alias contributes its own header, in Order's own load order, that
// alias names within one level are sorted alphabetically, not in
// whatever order Go's own unordered map iteration happens to produce,
// and that a level with no aliases defined contributes nothing at
// all, not even its own header.
func TestAliasesLinesListsEveryLevelSortedByAliasName(t *testing.T) {
	ctx := newTestContext()
	// T() on a nil Translator, newTestContext's default, drops any
	// format args rather than applying them, see i18n.Translator.T's
	// own doc comment, so a real Translator with a minimal catalog is
	// needed here to actually see the alias name and expansion text
	// interpolated into the captured output.
	ctx.Translator = i18n.New(map[string]i18n.Catalog{
		"en": {
			"show.aliases.level_header": "%s mode:",
			"show.aliases.line":         "  %s -> %s",
		},
	}, "en", "en")
	ctx.Levels = &command.TreeStructure{
		Order: []*command.CommandLevel{
			{Name: "exec", Aliases: map[string][]string{
				"wr": {"copy", "running-config", "startup-config"},
				"sh": {"show"},
			}},
			{Name: "config", Aliases: nil},
		},
	}

	lines := aliasesLines(ctx)
	joined := strings.Join(lines, "\n")

	shIdx := strings.Index(joined, "sh")
	wrIdx := strings.Index(joined, "wr")
	if shIdx == -1 || wrIdx == -1 || shIdx > wrIdx {
		t.Errorf("aliasesLines = %q, want \"sh\" listed before \"wr\" (alphabetical)", joined)
	}
	if strings.Contains(joined, "config mode:") {
		t.Errorf("aliasesLines = %q, want no header for \"config\", which has no aliases defined", joined)
	}
	if !strings.Contains(joined, "show") || !strings.Contains(joined, "copy running-config startup-config") {
		t.Errorf("aliasesLines = %q, want each alias's own expansion in full", joined)
	}
}

// TestShowHistoryHandlerZeroSizePrintsEmptyMessage - This test
// verifies that "terminal history size 0" makes "show history" report
// nothing, the same "zero means none" convention "terminal length 0"
// already carries for paging, even though the file itself still has
// real content on disk, untouched.
func TestShowHistoryHandlerZeroSizePrintsEmptyMessage(t *testing.T) {
	ctx := newTestContext()
	path := filepath.Join(t.TempDir(), "history.log")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0644); err != nil {
		t.Fatalf("failed to write test history file: %v", err)
	}
	ctx.HistoryFile = path
	zero := 0
	ctx.HistorySize = &zero
	cmd := loadTestCommand(t, "show.history")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Fatalf("show.history handler returned unexpected error: %v", runErr)
	}
	// ctx.Translator is nil here, the same minimal newTestContext every
	// other handler test in this package uses, so T() returns the
	// bracketed key itself rather than real catalog text, see
	// i18n.Translator.T's own doc comment. Checking for that bracketed
	// key is still a real, deterministic assertion that the empty
	// branch ran, not the one that prints file content.
	if !strings.Contains(out, "[[show.history.empty]]") {
		t.Errorf("output = %q, want the empty history message", out)
	}
}
