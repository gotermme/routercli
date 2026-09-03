// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package product

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gotermme/routercli/auth"
	_ "github.com/gotermme/routercli/cmd/core"
	_ "github.com/gotermme/routercli/cmd/session"
	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/i18n"
	"github.com/gotermme/routercli/tokenize"
)

// TestShowStartupConfigHandlerReturnsNoError - This test verifies
// that "show startup-config" runs without error, the same minimum
// guarantee as show.version in cmd/core.
func TestShowStartupConfigHandlerReturnsNoError(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "show.startup-config")

	if err := cmd.RunFunc(ctx, nil); err != nil {
		t.Errorf("show.startup-config handler returned unexpected error: %v", err)
	}
}

// TestShowInterfaceHandlerNoInterfacesConfigured - This test verifies
// that "show interface" with no interfaces ever touched prints the
// "nothing configured" text rather than an empty listing or an
// error.
func TestShowInterfaceHandlerNoInterfacesConfigured(t *testing.T) {
	ctx := newTestContext()
	cmd := loadTestCommand(t, "show.interface")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Errorf("show.interface handler returned unexpected error: %v", runErr)
	}
	if out == "" {
		t.Error("expected show.interface to print something when no interfaces are configured")
	}
}

// TestShowInterfaceHandlerListsConfiguredInterfacesInSortedOrder -
// This test verifies that "show interface" lists every interface that
// has been touched, each with its shutdown status and description
// when set, in sorted order regardless of Go's randomized map
// iteration, exercising sortedInterfaceNames through the handler.
func TestShowInterfaceHandlerListsConfiguredInterfacesInSortedOrder(t *testing.T) {
	ctx := newTestContext()
	// T() on a nil Translator, newTestContext's default, drops any
	// format args rather than applying them, see i18n.Translator.T's
	// own doc comment, so a real Translator with a minimal catalog is
	// needed here to actually see the interpolated description text
	// in the captured output.
	ctx.Translator = i18n.New(map[string]i18n.Catalog{
		"en": {"show.interface.description_label": "Description: %s"},
	}, "en", "en")
	state := ctx.State.(*ProductState)
	state.Interface("eth1").Shutdown = true
	state.Interface("eth0").Description = "uplink"
	cmd := loadTestCommand(t, "show.interface")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Errorf("show.interface handler returned unexpected error: %v", runErr)
	}

	eth0At := strings.Index(out, "eth0")
	eth1At := strings.Index(out, "eth1")
	if eth0At == -1 || eth1At == -1 {
		t.Fatalf("expected both eth0 and eth1 to appear in output, got %q", out)
	}
	if eth0At > eth1At {
		t.Errorf("expected eth0 to be listed before eth1 (sorted order), got %q", out)
	}
	if !strings.Contains(out, "uplink") {
		t.Errorf("expected eth0's description %q to appear, got %q", "uplink", out)
	}
}

// TestShowRunningConfigHandlerIncludesEveryConfiguredValue - This
// test verifies that "show running-config" reflects the hostname,
// description, and per-interface state that has actually been set,
// exercising printRunningConfig end to end through the registered
// handler, the same way a real "show running-config" on the device
// would.
func TestShowRunningConfigHandlerIncludesEveryConfiguredValue(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{}
	state := ctx.State.(*ProductState)
	state.Hostname = "myrouter"
	state.Description = "a lab router"
	state.Interface("eth0").Description = "uplink"
	state.Interface("eth1").Shutdown = true
	state.BannerMOTD = "welcome"
	state.BannerLogin = "please authenticate"
	cmd := loadTestCommand(t, "show.running-config")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Fatalf("show.running-config handler returned unexpected error: %v", runErr)
	}

	for _, want := range []string{"myrouter", "a lab router", "eth0", "uplink", "eth1", "shutdown", "banner motd welcome", "banner login \"please authenticate\""} {
		if !strings.Contains(out, want) {
			t.Errorf("show running-config output missing %q, got:\n%s", want, out)
		}
	}
}

// TestShowRunningConfigHandlerOmitsNeverConfiguredValues - This test
// verifies the companion case: a freshly constructed ProductState with
// nothing ever set produces output with none of hostname, description,
// or terminal geometry lines, matching how a real device only shows
// config lines for values that have actually been configured.
func TestShowRunningConfigHandlerOmitsNeverConfiguredValues(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{}
	cmd := loadTestCommand(t, "show.running-config")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Fatalf("show.running-config handler returned unexpected error: %v", runErr)
	}
	for _, unwanted := range []string{"hostname", "terminal width", "set description", "banner motd", "banner login"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("show running-config output unexpectedly contains %q for an unconfigured value, got:\n%s", unwanted, out)
		}
	}
}

// loadTestTree - This function resolves an entire tree, rather than
// loadTestCommand's single command, by writing yamlBody to a
// throwaway tree file and loading it through command.LoadTree, the
// same loader main.go uses at startup. See loadTestCommand's own doc
// comment in testhelpers_test.go for why a real file, not a directly
// constructed map, is used here too: this exercises the exact same
// "run:" resolution path a real tree file goes through.
func loadTestTree(t *testing.T, yamlBody string) map[string]*command.Command {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tree.yaml")
	if err := os.WriteFile(path, []byte(yamlBody), 0644); err != nil {
		t.Fatalf("failed to write test tree file: %v", err)
	}
	tree, err := command.LoadTree(path)
	if err != nil {
		t.Fatalf("LoadTree returned error: %v", err)
	}
	return tree
}

// replayConfigLines - This function feeds lines back through the
// exact same tokenize, resolve, validate, run sequence main.go's
// runLoop uses for anything typed at a real prompt, against whichever
// Command Level ctx.Position.Current() happens to be at the time,
// exactly what actually happens when a person pastes text into a live
// session. A line that is empty, or a Cisco style "!" comment line,
// such as runningConfigLines' own header and trailing separator, is
// skipped rather than sent through resolution, the same way a real
// terminal treats a blank or comment line as nothing to run.
func replayConfigLines(t *testing.T, ctx *command.AppContext, lines []string) {
	t.Helper()
	for _, line := range lines {
		if line == "" || strings.HasPrefix(line, "!") {
			continue
		}
		tokens, terr := tokenize.Tokenize(line)
		if terr != nil {
			t.Fatalf("failed to tokenize replayed line %q: %v", line, terr)
		}
		res := command.Resolve(ctx.Position.Current().Tree, tokens)
		if res.Command == nil || res.Command.RunFunc == nil {
			t.Fatalf("replayed line %q did not resolve to a runnable command while at Command Level %q", line, ctx.Position.Current().Name)
		}
		if !res.Negated {
			if verr := command.ValidateArgs(res.Command, res.Args); verr != nil {
				t.Fatalf("replayed line %q failed argument validation: %v", line, verr)
			}
		}
		ctx.Negated = res.Negated
		runErr := res.Command.RunFunc(ctx, res.Args)
		ctx.Negated = false
		if runErr != nil {
			t.Fatalf("replayed line %q returned an error: %v", line, runErr)
		}
	}
}

// showRoundTripLevels - This function builds a real, working three
// level Command Level Tree, exec, config, and config-if, the same
// shape var/tree/tree_structure.yaml declares, loading each level's
// own tree through command.LoadTree rather than constructing Command
// values by hand, so TestShowRunningConfigOutputReplaysBackToTheSameState
// below exercises the real, registered "configure.terminal",
// "hostname", "interface", "description.interface",
// "interface.shutdown", "terminal.length", "terminal.width", and
// "set.description" handlers, alongside "exit" and "end" from
// cmd/core/cmd_mode_control.go, not stand-ins for them.
// "configure.terminal", "terminal.length", "terminal.width", "exit",
// and "end" are registered by cmd/core, this file's own blank import
// of that package, rather than by this package itself.
func showRoundTripLevels(t *testing.T) *command.TreeStructure {
	execTree := loadTestTree(t, `commands:
  configure:
    subcommands:
      terminal:
        run: configure.terminal
  set:
    subcommands:
      description:
        minargs: 1
        maxargs: 1
        run: set.description
  alias:
    minargs: 2
    negatable: true
    run: alias
`)
	configTree := loadTestTree(t, `commands:
  hostname:
    minargs: 1
    maxargs: 1
    negatable: true
    run: hostname
  interface:
    minargs: 1
    maxargs: 1
    run: interface
  terminal:
    subcommands:
      length:
        minargs: 1
        maxargs: 1
        run: terminal.length
      width:
        minargs: 1
        maxargs: 1
        run: terminal.width
  alias:
    minargs: 2
    negatable: true
    run: alias
  exit:
    run: exit
  end:
    run: end
`)
	configIfTree := loadTestTree(t, `commands:
  description:
    minargs: 1
    maxargs: 1
    negatable: true
    run: description.interface
  shutdown:
    negatable: true
    run: interface.shutdown
  alias:
    minargs: 2
    negatable: true
    run: alias
  exit:
    run: exit
  end:
    run: end
`)

	return &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"exec":      {Name: "exec", Tree: execTree},
		"config":    {Name: "config", Parent: "exec", PromptSuffix: "(config)", EnterCommand: "configure.terminal", Tree: configTree},
		"config-if": {Name: "config-if", Parent: "config", PromptSuffix: "(config-if)", EnterCommand: "interface", Tree: configIfTree},
	}}
}

// TestShowRunningConfigOutputReplaysBackToTheSameState - This test is
// the actual round trip promise "show running-config" makes: its own
// output, copied out of one session and pasted back into a second,
// fresh one starting at exec, the same place a real login session
// lands, reproduces the exact same state. It exercises
// runningConfigLines' own level-aware wrapping directly, confirming
// the rendered text includes "configure terminal" before anything
// that only exists in config mode, "exit" between the two interface
// blocks, since cmd_interface.go refuses "interface" unless the
// session is sitting exactly in config mode already, and "end" once
// at the very end, then confirms replaying that exact text through
// the real command dispatch path, the same tokenize, resolve,
// validate, run sequence main.go's runLoop itself uses, lands on an
// ProductState equal in every field to the one that produced it.
func TestShowRunningConfigOutputReplaysBackToTheSameState(t *testing.T) {
	levels := showRoundTripLevels(t)

	ctx := newTestContext()
	ctx.Levels = levels
	ctx.Position = command.NewCommandLevelStack("exec", "", levels.ByName["exec"].Tree)
	state := ctx.State.(*ProductState)
	state.Hostname = "myrouter"
	state.Description = "a lab router"
	state.Interface("eth0").Description = "uplink"
	state.Interface("eth1").Shutdown = true
	// One alias at every level this test's own three level tree
	// actually reaches, exec, config, and config-if, exercised
	// end to end alongside hostname and interface state, confirming
	// each is written into, and read back out of, its own specific
	// Command Level, not one flat, level-agnostic block the way
	// Phase 32's own first version of this feature rendered it. See
	// cmd_alias.go's own doc comment for why a level is never typed.
	levels.ByName["exec"].Aliases = map[string][]string{"sh": {"show", "version"}}
	levels.ByName["config"].Aliases = map[string][]string{"int": {"interface"}}
	levels.ByName["config-if"].Aliases = map[string][]string{"desc": {"description"}}

	lines := runningConfigLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "configure terminal") {
		t.Fatalf("expected rendered output to include \"configure terminal\", got:\n%s", joined)
	}
	if !strings.Contains(joined, "\nexit\n") {
		t.Fatalf("expected rendered output to back out with \"exit\" between the two interface blocks, got:\n%s", joined)
	}
	if !strings.Contains(joined, "\nend\n") {
		t.Fatalf("expected rendered output to close the config block with \"end\", got:\n%s", joined)
	}
	if strings.Index(joined, "set description") > strings.Index(joined, "configure terminal") {
		t.Errorf("expected the exec level \"set description\" line before \"configure terminal\", got:\n%s", joined)
	}
	if execIdx, cfgIdx := strings.Index(joined, "alias sh show version"), strings.Index(joined, "configure terminal"); execIdx == -1 || execIdx > cfgIdx {
		t.Errorf("expected the exec level's own alias before \"configure terminal\", with no level named on the line itself, got:\n%s", joined)
	}
	if !strings.Contains(joined, "alias int interface") {
		t.Errorf("expected the config level's own alias line, got:\n%s", joined)
	}
	if !strings.Contains(joined, "interface alias-replay-placeholder") {
		t.Errorf("expected a placeholder interface entered to replay the config-if level's own alias, got:\n%s", joined)
	}
	if !strings.Contains(joined, "alias desc description") {
		t.Errorf("expected the config-if level's own alias line, got:\n%s", joined)
	}

	replayCtx := newTestContext()
	replayCtx.Levels = levels
	replayCtx.Position = command.NewCommandLevelStack("exec", "", levels.ByName["exec"].Tree)
	// rewireDaemonClient shares this exact TreeStructure with
	// ctx.DaemonClient's own Store, so cmd/core's own "alias" handler,
	// replayed below by way of this file's blank import of that
	// package, reaches the same ctx.Levels this test asserts against
	// afterward through its own ctx.DaemonClient.MutateLevels call. See
	// rewireDaemonClient's own doc comment in testhelpers_test.go.
	rewireDaemonClient(replayCtx)
	// Clear every level's own Aliases map first, so this replay proves
	// the rendered text alone reconstructs them, not that they were
	// simply left over from ctx and replayCtx sharing the same
	// *command.CommandLevel values.
	levels.ByName["exec"].Aliases = nil
	levels.ByName["config"].Aliases = nil
	levels.ByName["config-if"].Aliases = nil
	replayConfigLines(t, replayCtx, lines)
	replayState := replayCtx.State.(*ProductState)

	if replayState.Hostname != state.Hostname {
		t.Errorf("replayed Hostname = %q, want %q", replayState.Hostname, state.Hostname)
	}
	if replayState.Description != state.Description {
		t.Errorf("replayed Description = %q, want %q", replayState.Description, state.Description)
	}
	if len(replayState.Interfaces) != len(state.Interfaces) {
		t.Fatalf("replayed Interfaces has %d entries, want %d", len(replayState.Interfaces), len(state.Interfaces))
	}
	for name, iface := range state.Interfaces {
		got, ok := replayState.Interfaces[name]
		if !ok {
			t.Fatalf("replayed state is missing interface %q", name)
		}
		if got.Description != iface.Description || got.Shutdown != iface.Shutdown {
			t.Errorf("replayed interface %q = %+v, want %+v", name, got, iface)
		}
	}
	if got := levels.ByName["exec"].Aliases["sh"]; !reflect.DeepEqual(got, []string{"show", "version"}) {
		t.Errorf("replayed exec alias \"sh\" = %v, want [show version]", got)
	}
	if got := levels.ByName["config"].Aliases["int"]; !reflect.DeepEqual(got, []string{"interface"}) {
		t.Errorf("replayed config alias \"int\" = %v, want [interface]", got)
	}
	if got := levels.ByName["config-if"].Aliases["desc"]; !reflect.DeepEqual(got, []string{"description"}) {
		t.Errorf("replayed config-if alias \"desc\" = %v, want [description]", got)
	}
	// The placeholder interface used solely to replay the config-if
	// alias must never leave a real, visible interface behind, see
	// configIfAliasReplayInterfaceName's own doc comment.
	if _, exists := replayState.Interfaces[configIfAliasReplayInterfaceName]; exists {
		t.Errorf("expected the config-if alias replay placeholder to leave no real interface behind, but %q exists", configIfAliasReplayInterfaceName)
	}
	if replayCtx.Position.Current().Name != "exec" {
		t.Errorf("expected the replayed session to land back at exec (from \"end\"), got %q", replayCtx.Position.Current().Name)
	}
}

// TestRunningConfigLinesWrapsAdminAndDiagnosticAliasesWithTheirOwnEnterAndExitWords
// - This test verifies runningConfigLines' own handling of a Command
// Level reached by swapping the whole root frame in place, see
// command.EnterCommandLevel and command.ExitCommandLevel, admin and
// diagnostic in this project's own shipped tree, rather than pushing a
// nested frame the way config does. exec's own alias needs no wrapper
// at all, this whole file already assumes a session is standing there;
// admin and diagnostic each need their own real enter and exit words,
// discovered through levelEnterWords and levelExitWords, "admin" and
// "return", or "diagnostic-mode" and "exit-diagnostic-mode", wrapped
// around their own alias lines.
func TestRunningConfigLinesWrapsAdminAndDiagnosticAliasesWithTheirOwnEnterAndExitWords(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"exec": {Name: "exec", Aliases: map[string][]string{"sh": {"show"}}, Tree: map[string]*command.Command{
			"admin":           {Run: "admin"},
			"diagnostic-mode": {Run: "diagnostic-mode"},
		}},
		"admin": {Name: "admin", Parent: "exec", EnterCommand: "admin", ExitCommand: "return.admin",
			Aliases: map[string][]string{"wr": {"copy", "running-config", "startup-config"}},
			Tree:    map[string]*command.Command{"return": {Run: "return.admin"}},
		},
		"diagnostic": {Name: "diagnostic", Parent: "exec", EnterCommand: "diagnostic-mode", ExitCommand: "exit-diagnostic-mode",
			Aliases: map[string][]string{"ping6": {"ping", "ipv6"}},
			Tree:    map[string]*command.Command{"exit-diagnostic-mode": {Run: "exit-diagnostic-mode"}},
		},
	}}
	state := ctx.State.(*ProductState)

	lines := runningConfigLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "\nalias sh show\n") {
		t.Errorf("expected exec's own alias with no enter or exit words around it, got:\n%s", joined)
	}
	if !strings.Contains(joined, "admin\nalias wr copy running-config startup-config\nreturn") {
		t.Errorf("expected admin's own alias wrapped with \"admin\" ... \"return\", got:\n%s", joined)
	}
	if !strings.Contains(joined, "diagnostic-mode\nalias ping6 ping ipv6\nexit-diagnostic-mode") {
		t.Errorf("expected diagnostic's own alias wrapped with \"diagnostic-mode\" ... \"exit-diagnostic-mode\", got:\n%s", joined)
	}
}

// TestRunningConfigLinesOmitsAdminBlockWhenAdminHasNoAliases - This
// test verifies that wrappedLevelAliasLines contributes nothing at
// all, not even an empty enter, exit pair, once admin has no aliases
// of its own defined, the same "nothing configured, nothing shown"
// convention this whole file already follows everywhere else.
func TestRunningConfigLinesOmitsAdminBlockWhenAdminHasNoAliases(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"exec": {Name: "exec", Tree: map[string]*command.Command{
			"admin": {Run: "admin"},
		}},
		"admin": {Name: "admin", Parent: "exec", EnterCommand: "admin", ExitCommand: "return.admin",
			Tree: map[string]*command.Command{"return": {Run: "return.admin"}},
		},
	}}
	state := ctx.State.(*ProductState)

	lines := runningConfigLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if strings.Contains(joined, "admin") {
		t.Errorf("expected no admin block at all when admin has no aliases, got:\n%s", joined)
	}
}

// ----------------------------------------------------------------------
//
// configModeLines, password rendering
//
// ----------------------------------------------------------------------

// TestConfigModeLinesRendersOrdinaryPasswordHashInFull - This test
// verifies that a level's ordinary, user settable PasswordHash renders
// as a real "password manager hash <hash>" line, restorable by
// cmd/core/cmd_password_manager.go's own hash accepting form.
func TestConfigModeLinesRendersOrdinaryPasswordHashInFull(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{Order: []*command.CommandLevel{
		{Name: "exec", PasswordHash: "$6$$ordinaryhash"},
	}}
	state := ctx.State.(*ProductState)

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "password manager hash $6$$ordinaryhash") {
		t.Errorf("expected the ordinary hash to be reproduced in full, got:\n%s", joined)
	}
}

// TestConfigModeLinesHidesVendorDefinedPasswordHash - This test
// verifies the actual security property Task 5 exists for: a level's
// VendorDefinedPasswordHash never appears anywhere in
// configModeLines' output, in any form, replaced instead with the
// literal "<HIDDEN>" placeholder. This asserts both directions,
// the placeholder is present, and the real vendor hash string is
// absent from the joined output entirely, not just missing from the
// one line a reader might think to check.
func TestConfigModeLinesHidesVendorDefinedPasswordHash(t *testing.T) {
	const vendorHash = "$6$$super-secret-vendor-hash"
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{Order: []*command.CommandLevel{
		{Name: "diagnostic", VendorDefinedPasswordHash: vendorHash, Hidden: true},
	}}
	state := ctx.State.(*ProductState)

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "password manager hash <HIDDEN>") {
		t.Errorf("expected a \"password manager hash <HIDDEN>\" placeholder line, got:\n%s", joined)
	}
	if strings.Contains(joined, vendorHash) {
		t.Errorf("expected the real vendor defined hash to never appear in output, but it did:\n%s", joined)
	}
}

// TestConfigModeLinesVendorDefinedWinsOverOrdinaryHash - This test
// verifies that a level carrying both fields at once, itself a
// configuration command.VerifyVendorDefinedSecrets refuses to allow,
// still renders safely: the placeholder, never the ordinary hash,
// since a caller could still construct this combination directly in
// Go, bypassing YAML and the configuration checker entirely, and
// rendering must never be the path that leaks a secret a stricter
// check elsewhere assumed was unreachable.
func TestConfigModeLinesVendorDefinedWinsOverOrdinaryHash(t *testing.T) {
	const vendorHash = "$6$$vendor-wins-hash"
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{Order: []*command.CommandLevel{
		{Name: "diagnostic", PasswordHash: "$6$$ordinaryhash", VendorDefinedPasswordHash: vendorHash, Hidden: true},
	}}
	state := ctx.State.(*ProductState)

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "password manager hash <HIDDEN>") {
		t.Errorf("expected the placeholder to win when both fields are set, got:\n%s", joined)
	}
	if strings.Contains(joined, vendorHash) || strings.Contains(joined, "ordinaryhash") {
		t.Errorf("expected neither hash to appear in output when both fields are set, got:\n%s", joined)
	}
}

// TestConfigModeLinesRevealsVendorDefinedHashWhenSessionGrantsReveal -
// This test verifies the su-config side of Task 6: once the current
// session sits at a Command Level whose own RevealVendorDefinedSecrets
// is true, a level's VendorDefinedPasswordHash renders in full, the
// same as an ordinary PasswordHash always does, rather than the
// "<HIDDEN>" placeholder TestConfigModeLinesHidesVendorDefinedPasswordHash
// above confirms for every other session.
func TestConfigModeLinesRevealsVendorDefinedHashWhenSessionGrantsReveal(t *testing.T) {
	const vendorHash = "$6$$super-secret-vendor-hash"
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{
		Order: []*command.CommandLevel{
			{Name: "diagnostic", VendorDefinedPasswordHash: vendorHash, Hidden: true},
		},
		ByName: map[string]*command.CommandLevel{
			"su-config": {Name: "su-config", RevealVendorDefinedSecrets: true},
		},
	}
	ctx.Session = &auth.Session{CommandLevel: "su-config"}
	state := ctx.State.(*ProductState)

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "password manager hash "+vendorHash) {
		t.Errorf("expected the real vendor defined hash to be revealed while sitting in su-config, got:\n%s", joined)
	}
	if strings.Contains(joined, "<HIDDEN>") {
		t.Errorf("expected no \"<HIDDEN>\" placeholder while sitting in su-config, got:\n%s", joined)
	}
}

// ----------------------------------------------------------------------
//
// configModeLines, alias rendering
//
// ----------------------------------------------------------------------

// TestConfigModeLinesRendersConfigLevelAliasAsAPasteableCommand - This
// test verifies that config's own runtime defined alias, see
// command.CommandLevel.Aliases and cmd/core/cmd_alias.go, renders
// directly, as a literal "alias <name> <word...>" line with no level
// named on it, the exact command a session standing in config mode, or
// command.LoadStartupConfig replaying a saved startup-config at boot,
// would type to define it again. configModeLines only ever renders
// config's own aliases this way, flat, with no enter or exit words of
// its own, since the whole function already runs inside a "configure
// terminal" paste; see runningConfigLines for exec, admin, and
// diagnostic, each of which needs its own wrapping instead.
func TestConfigModeLinesRendersConfigLevelAliasAsAPasteableCommand(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"config": {Name: "config", Aliases: map[string][]string{
			"sh": {"show", "version"},
		}},
	}}
	state := ctx.State.(*ProductState)

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "alias sh show version") {
		t.Errorf("expected an \"alias sh show version\" line, got:\n%s", joined)
	}
}

// TestConfigModeLinesRendersConfigLevelAliasesSortedByName - This
// test verifies that more than one alias defined at the config level
// renders sorted alphabetically by alias name, the same stability
// aliasesLines in cmd/core/cmd_show.go already gives "show aliases".
func TestConfigModeLinesRendersConfigLevelAliasesSortedByName(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"config": {Name: "config", Aliases: map[string][]string{
			"wr": {"copy", "running-config", "startup-config"},
			"sh": {"show"},
		}},
	}}
	state := ctx.State.(*ProductState)

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	shIdx := strings.Index(joined, "alias sh show")
	wrIdx := strings.Index(joined, "alias wr copy running-config startup-config")
	if shIdx == -1 || wrIdx == -1 {
		t.Fatalf("expected both alias lines to be present, got:\n%s", joined)
	}
	if shIdx > wrIdx {
		t.Errorf("expected \"sh\" to render before \"wr\" (alphabetical), got:\n%s", joined)
	}
}

// TestConfigModeLinesOmitsAliasBlockWhenNoAliasIsDefined - This test
// verifies that a level with a nil or empty Aliases map, the state
// every level starts in, contributes no "alias" line at all, the same
// "nothing configured, nothing shown" convention this function
// already follows for an untouched interface.
func TestConfigModeLinesOmitsAliasBlockWhenNoAliasIsDefined(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"config": {Name: "config"},
	}}
	state := ctx.State.(*ProductState)

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if strings.Contains(joined, "alias ") {
		t.Errorf("expected no \"alias\" line when nothing is defined, got:\n%s", joined)
	}
}

// TestConfigModeLinesRendersConfigIfAliasThroughPlaceholderInterface -
// This test verifies that a config-if scoped alias, only ever
// definable while a session is actually standing inside "interface
// <name>" mode, is replayed back in by entering
// configIfAliasReplayInterfaceName, never one of the real, already
// configured interface names, since the alias itself belongs to the
// whole config-if level, not to any one interface. See
// cmd_interface.go's own doc comment for why entering and leaving
// config-if this way touches no interface state.
func TestConfigModeLinesRendersConfigIfAliasThroughPlaceholderInterface(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"config": {Name: "config", Tree: map[string]*command.Command{
			"interface": {Run: "interface"},
		}},
		"config-if": {Name: "config-if", Parent: "config", EnterCommand: "interface"},
	}}
	ctx.Levels.ByName["config-if"].Aliases = map[string][]string{"desc": {"description"}}
	state := ctx.State.(*ProductState)

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "interface "+configIfAliasReplayInterfaceName) {
		t.Errorf("expected the placeholder interface to be entered, got:\n%s", joined)
	}
	if !strings.Contains(joined, "alias desc description") {
		t.Errorf("expected the config-if level's own alias line, got:\n%s", joined)
	}
	if _, exists := state.Interfaces[configIfAliasReplayInterfaceName]; exists {
		t.Error("expected rendering alone to never create a real interface entry")
	}
}

// ----------------------------------------------------------------------
//
// configModeLines, "line" mode rendering (item 11)
//
// ----------------------------------------------------------------------

// TestConfigModeLinesOmitsLineBlockWhenNothingIsSet - This test
// verifies that a ProductState.Line with every field left nil, the
// state every deployment starts in, contributes no "line" block at
// all, the same "nothing configured, nothing shown" convention this
// function already follows for hostname, banners, and interfaces.
func TestConfigModeLinesOmitsLineBlockWhenNothingIsSet(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{}
	state := ctx.State.(*ProductState)

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if strings.Contains(joined, "line") {
		t.Errorf("expected no \"line\" block when nothing is set, got:\n%s", joined)
	}
}

// TestConfigModeLinesRendersLineBlockWithEverySetField - This test
// verifies that "line length", "line width", and "line paging" each
// render as their own indented line inside a "line" block, in the
// same shape a session, or a saved startup-config replayed at boot,
// would type them.
func TestConfigModeLinesRendersLineBlockWithEverySetField(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{}
	state := ctx.State.(*ProductState)
	length, width, paging := 30, 100, false
	state.Line = LineDefaults{Length: &length, Width: &width, Paging: &paging}

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	for _, want := range []string{"line", " length 30", " width 100", " no paging"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected %q in output, got:\n%s", want, joined)
		}
	}
}

// TestConfigModeLinesRendersLineBlockWithOnlyOneFieldSet - This test
// verifies that a ProductState.Line with only one field ever set,
// Length here, renders only that one line inside the "line" block,
// with no "width" or "paging" line invented for the two fields still
// left nil.
func TestConfigModeLinesRendersLineBlockWithOnlyOneFieldSet(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{}
	state := ctx.State.(*ProductState)
	length := 40
	state.Line = LineDefaults{Length: &length}

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, " length 40") {
		t.Errorf("expected \" length 40\" in output, got:\n%s", joined)
	}
	if strings.Contains(joined, "width") || strings.Contains(joined, "paging") {
		t.Errorf("expected no \"width\" or \"paging\" line, got:\n%s", joined)
	}
}

// TestConfigModeLinesRendersConfigLineAliasInsideTheSameBlock - This
// test verifies that a config-line scoped alias renders inside the
// same enter, exit block as length, width, and paging, rather than a
// separate block of its own, since config-line, unlike config-if, is
// only ever one single, deployment wide instance; there is no reason
// to enter and leave it twice. This also confirms the "line" block
// itself is rendered even when every one of Length, Width, and Paging
// is left nil, so long as config-line has at least one alias defined.
func TestConfigModeLinesRendersConfigLineAliasInsideTheSameBlock(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"config-line": {Name: "config-line", Aliases: map[string][]string{
			"pg": {"paging"},
		}},
	}}
	state := ctx.State.(*ProductState)

	lines := configModeLines(ctx, state)
	joined := strings.Join(lines, "\n")

	if !strings.Contains(joined, "line\n alias pg paging") {
		t.Errorf("expected the config-line alias inside the same block as \"line\", got:\n%s", joined)
	}
}

// TestConfigModeLinesExitsTheLastInterfaceBeforeLineBlock - This test
// verifies the correctness fix this function's own doc comment on
// hasLineBlock describes: when both an interface block and a "line"
// block are present, the last interface block gets its own trailing
// "exit" too, even though it would otherwise be the very last block
// and skip it, since cmd/product/cmd_line.go's own "line" command
// only accepts running from exactly config mode, not still nested
// inside a previous interface's own config-if mode.
func TestConfigModeLinesExitsTheLastInterfaceBeforeLineBlock(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{}
	state := ctx.State.(*ProductState)
	state.Interface("eth0").Description = "uplink"
	length := 30
	state.Line = LineDefaults{Length: &length}

	lines := configModeLines(ctx, state)

	interfaceIdx := -1
	lineIdx := -1
	exitBetween := false
	for i, line := range lines {
		if strings.HasPrefix(line, "interface ") {
			interfaceIdx = i
		}
		if line == "line" {
			lineIdx = i
		}
	}
	if interfaceIdx == -1 || lineIdx == -1 {
		t.Fatalf("expected both an \"interface\" line and a \"line\" line, got: %v", lines)
	}
	for i := interfaceIdx + 1; i < lineIdx; i++ {
		if lines[i] == "exit" {
			exitBetween = true
		}
	}
	if !exitBetween {
		t.Errorf("expected an \"exit\" line between the interface block and the line block, got: %v", lines)
	}
}

// ----------------------------------------------------------------------
//
// currentLevelRevealsVendorDefinedSecrets
//
// ----------------------------------------------------------------------

// TestCurrentLevelRevealsVendorDefinedSecretsTrueAtRevealingLevel -
// This test verifies the ordinary case: the current session's own
// Command Level carries RevealVendorDefinedSecrets true.
func TestCurrentLevelRevealsVendorDefinedSecretsTrueAtRevealingLevel(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"su-config": {Name: "su-config", RevealVendorDefinedSecrets: true},
	}}
	ctx.Session = &auth.Session{CommandLevel: "su-config"}

	if !currentLevelRevealsVendorDefinedSecrets(ctx) {
		t.Error("expected true while sitting at a level with RevealVendorDefinedSecrets set")
	}
}

// TestCurrentLevelRevealsVendorDefinedSecretsFalseAtOtherLevel - This
// test verifies that an ordinary level, exec here, with
// RevealVendorDefinedSecrets left at its zero value, never reveals
// anything, even though some other loaded level, su-config here, does.
func TestCurrentLevelRevealsVendorDefinedSecretsFalseAtOtherLevel(t *testing.T) {
	ctx := newTestContext()
	ctx.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"exec":      {Name: "exec"},
		"su-config": {Name: "su-config", RevealVendorDefinedSecrets: true},
	}}
	ctx.Session = &auth.Session{CommandLevel: "exec"}

	if currentLevelRevealsVendorDefinedSecrets(ctx) {
		t.Error("expected false while sitting at exec, which does not set RevealVendorDefinedSecrets")
	}
}

// TestCurrentLevelRevealsVendorDefinedSecretsFalseWithoutContext -
// This test verifies the safe, false default whenever ctx.Levels or
// ctx.Session is nil, or ctx.Session.CommandLevel does not resolve to
// a real, loaded level, the same unconfigured-context safety
// cmd_password_manager.go's own current level lookup follows.
func TestCurrentLevelRevealsVendorDefinedSecretsFalseWithoutContext(t *testing.T) {
	nilLevels := newTestContext()
	nilLevels.Session = &auth.Session{CommandLevel: "su-config"}
	if currentLevelRevealsVendorDefinedSecrets(nilLevels) {
		t.Error("expected false when ctx.Levels is nil")
	}

	nilSession := newTestContext()
	nilSession.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{
		"su-config": {Name: "su-config", RevealVendorDefinedSecrets: true},
	}}
	if currentLevelRevealsVendorDefinedSecrets(nilSession) {
		t.Error("expected false when ctx.Session is nil")
	}

	unresolvedLevel := newTestContext()
	unresolvedLevel.Levels = &command.TreeStructure{ByName: map[string]*command.CommandLevel{}}
	unresolvedLevel.Session = &auth.Session{CommandLevel: "su-config"}
	if currentLevelRevealsVendorDefinedSecrets(unresolvedLevel) {
		t.Error("expected false when ctx.Session.CommandLevel does not resolve to a loaded level")
	}
}

// ----------------------------------------------------------------------
//
// show startup-config, real file backed
//
// ----------------------------------------------------------------------

// TestShowStartupConfigHandlerPrintsFileContentsWhenPresent - This
// test verifies that "show startup-config" prints ctx.StartupConfigFile's
// own real content once something has actually been saved there, in
// place of TestShowStartupConfigHandlerReturnsNoError's own "nothing
// saved yet" case.
func TestShowStartupConfigHandlerPrintsFileContentsWhenPresent(t *testing.T) {
	ctx := newTestContext()
	ctx.StartupConfigFile = filepath.Join(t.TempDir(), "startup-config")
	const saved = "! (example running-config)\nhostname myrouter\n!\n"
	if err := os.WriteFile(ctx.StartupConfigFile, []byte(saved), 0640); err != nil {
		t.Fatalf("failed to seed startup-config file: %v", err)
	}
	cmd := loadTestCommand(t, "show.startup-config")

	var runErr error
	out := captureStdout(t, func() { runErr = cmd.RunFunc(ctx, nil) })
	if runErr != nil {
		t.Fatalf("show.startup-config handler returned unexpected error: %v", runErr)
	}
	if out != saved {
		t.Errorf("show startup-config output = %q, want %q", out, saved)
	}
}

// TestShowStartupConfigHandlerReturnsErrorOnReadFailure - This test
// verifies that a read failure other than the file simply not existing
// yet, ctx.StartupConfigFile naming a directory here, is reported back
// as a real error rather than silently treated the same as "nothing
// saved yet".
func TestShowStartupConfigHandlerReturnsErrorOnReadFailure(t *testing.T) {
	ctx := newTestContext()
	ctx.StartupConfigFile = t.TempDir()
	cmd := loadTestCommand(t, "show.startup-config")

	if err := cmd.RunFunc(ctx, nil); err == nil {
		t.Fatal("expected an error when ctx.StartupConfigFile names a directory, got nil")
	}
}
