// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gotermme/routercli/auth"

	"gopkg.in/yaml.v3"
)

// ----------------------------------------------------------------------
// Public Methods - Tree Structure
// ----------------------------------------------------------------------

// Base - This method returns the one level with IsBase set. Every
// Session starts here, see Session.CommandLevel in package auth, and
// it seeds the root CommandLevelStack frame in main.go. This panics if called
// before a successful LoadTreeStructure, or on a zero value
// TreeStructure, since that is a programming error, LoadTreeStructure
// guarantees exactly one base level exists on any successful return,
// not a runtime condition.
func (t *TreeStructure) Base() *CommandLevel {
	for _, e := range t.Order {
		if e.IsBase {
			return e
		}
	}
	panic("command: TreeStructure has no base level, this should have been caught by LoadTreeStructure validation")
}

// ----------------------------------------------------------------------
// Public Functions - Tree Structure
// ----------------------------------------------------------------------

// LoadTreeStructure - This function reads and validates a manifest
// like var/tree/tree_structure.yaml, resolves each Command Level's
// effective command tree, its own tree, optionally merged with its
// full parent chain per InheritParent, then merged with the common
// tree exactly once unless SkipCommonMerge, and returns the assembled
// TreeStructure. It covers every Command Level in the project the
// same way, there is no separate handling here for a privilege level
// versus a plain mode.
//
// This validates only what loading inherently requires. It does not
// check that every level's declared EnterCommand and ExitCommand is
// actually registered by some cmd_*.go file, that is
// VerifyCommandLevels's job, see its own doc comment for why that
// check lives separately. A "run:" reference inside each level's own
// tree file is validated here, transitively, through LoadTree, but
// those are ordinary command references, resolved against whatever
// cmd's init() functions already registered before main() ever runs
// this function.
//
// An empty manifest should fail loudly at startup rather than produce
// a CLI that silently has no way to reach half its commands, so this
// treats each of the following as a hard error: more than one level,
// or zero levels, setting IsBase, since a fresh session needs exactly
// one unambiguous starting point; a level whose Parent names another
// level that does not exist in this manifest; a cycle in the parent
// chain, detected by walking Parent from any level that has one and
// requiring it to terminate, reach a level with an empty Parent,
// within len(trees) steps; an unknown property name anywhere in the
// manifest, the same way config.LoadSystemConfig and auth.LoadUsers
// treat an unknown key in their own files; and a manifest containing
// more than one YAML document.
//
// commonPath is merged into every level's tree exactly once, unless
// that level sets skip_common. See MergeTrees's own doc comment for
// why a name collision between a tree and the common tree is a hard
// error rather than silently letting one side win.
func LoadTreeStructure(manifestPath, commonPath string) (*TreeStructure, error) {
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading tree structure manifest %s: %w", manifestPath, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)

	var parsed treeStructureFile
	if err := dec.Decode(&parsed); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing tree structure manifest %s: %w", manifestPath, err)
	}

	var extra treeStructureFile
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("tree structure manifest %s contains multiple YAML documents", manifestPath)
		}
		return nil, fmt.Errorf("parsing tree structure manifest %s: %w", manifestPath, err)
	}

	if len(parsed.Trees) == 0 {
		return nil, fmt.Errorf("tree structure manifest %s defines no trees at all", manifestPath)
	}

	levels := make(map[string]*CommandLevel, len(parsed.Trees))
	baseCount := 0
	for name, cl := range parsed.Trees {
		level := cl
		level.Name = name
		if level.IsBase {
			baseCount++
		}
		levels[name] = &level
	}
	if baseCount == 0 {
		return nil, fmt.Errorf("tree structure manifest %s has no base level (no tree sets is_base: true)", manifestPath)
	}
	if baseCount > 1 {
		return nil, fmt.Errorf("tree structure manifest %s has more than one base level (more than one tree sets is_base: true)", manifestPath)
	}
	for name, l := range levels {
		if l.Parent == "" {
			continue
		}
		if _, ok := levels[l.Parent]; !ok {
			return nil, fmt.Errorf("command level %q has parent %q, which is not defined in this manifest", name, l.Parent)
		}
	}

	// Cycle detection: walk each level's Parent chain up to
	// len(levels) steps. If it has not terminated by then, something
	// loops.
	for name, l := range levels {
		cur := l
		steps := 0
		for cur.Parent != "" {
			cur = levels[cur.Parent]
			steps++
			if steps > len(levels) {
				return nil, fmt.Errorf("command level %q's parent chain does not terminate, check for a cycle", name)
			}
		}
	}

	commonTree, err := LoadTree(commonPath)
	if err != nil {
		return nil, fmt.Errorf("loading common tree %s: %w", commonPath, err)
	}
	markCommonCommands(commonTree)

	// levelSwitchNames collects every EnterCommand and ExitCommand
	// declared anywhere in this manifest, "admin" and "disable" for
	// instance, this project's own shipped tree. See
	// filterLevelSwitchCommands's own doc comment for why these are
	// stripped back out again whenever InheritParent carries a tree
	// forward into a further descendant, rather than left to
	// accumulate down the whole chain the way an ordinary command
	// such as "show" correctly does.
	levelSwitchNames := make(map[string]bool, len(levels)*2)
	for _, l := range levels {
		if l.EnterCommand != "" {
			levelSwitchNames[l.EnterCommand] = true
		}
		if l.ExitCommand != "" {
			levelSwitchNames[l.ExitCommand] = true
		}
	}

	// Build the order, base first, then anything whose parent is
	// already resolved, in whatever order that ends up being, since
	// only the base level itself has no parent to wait on, along with
	// each level's raw, pre-common-merge, tree. InheritParent needs
	// the parent's already accumulated raw tree, not just the
	// parent's own TreeFile, so this has to proceed top-down rather
	// than in arbitrary map iteration order.
	rawTrees := make(map[string]map[string]*Command, len(levels))
	var order []*CommandLevel
	remaining := make(map[string]*CommandLevel, len(levels))
	for name, l := range levels {
		remaining[name] = l
	}
	for len(remaining) > 0 {
		progressed := false
		for name, l := range remaining {
			if l.Parent != "" {
				if _, ready := rawTrees[l.Parent]; !ready {
					continue // parent not resolved yet
				}
			}
			ownRaw, err := LoadTree(l.TreeFile)
			if err != nil {
				return nil, fmt.Errorf("loading Command Level %q's file %s: %w", name, l.TreeFile, err)
			}
			effectiveRaw := ownRaw
			if l.InheritParent && l.Parent != "" {
				inherited := filterLevelSwitchCommands(rawTrees[l.Parent], levelSwitchNames)
				merged, err := MergeTrees(inherited, ownRaw)
				if err != nil {
					return nil, fmt.Errorf("merging Command Level %q with parent %q: %w", name, l.Parent, err)
				}
				effectiveRaw = merged
			}
			rawTrees[name] = effectiveRaw
			finalTree := effectiveRaw
			if !l.SkipCommonMerge {
				merged, err := MergeTrees(effectiveRaw, commonTree)
				if err != nil {
					return nil, fmt.Errorf("merging Command Level %q with the common tree: %w", name, err)
				}
				finalTree = merged
			}
			l.Tree = finalTree
			order = append(order, l)
			delete(remaining, name)
			progressed = true
		}
		if !progressed {
			// The cycle detection pass above already covers this case,
			// so this should be unreachable. It is kept as a hard stop
			// rather than an infinite loop in case that pass is ever
			// weakened without this loop being revisited too.
			return nil, fmt.Errorf("tree structure manifest %s: could not resolve build order (unexpected cycle)", manifestPath)
		}
	}

	return &TreeStructure{ByName: levels, Order: order}, nil
}

// ----------------------------------------------------------------------
// Private Functions - Tree Structure
// ----------------------------------------------------------------------

// markCommonCommands - This function stamps IsCommonCommand true on
// every command in tree, recursively through Subcommands. It is
// called exactly once per program run, LoadTreeStructure's own call
// right after LoadTree(commonPath) returns and before commonTree is
// merged anywhere. That single call is enough for the whole program:
// the very same *Command values loaded there are the ones MergeTrees
// then copies its pointers from into every Command Level's own Tree,
// so marking them once here is marking them everywhere they end up.
// See Command.IsCommonCommand's own doc comment for what a caller
// does with this, ListOptions.MergeCommon's ordering.
func markCommonCommands(tree map[string]*Command) {
	for _, cmd := range tree {
		cmd.IsCommonCommand = true
		if len(cmd.Subcommands) > 0 {
			markCommonCommands(cmd.Subcommands)
		}
	}
}

// filterLevelSwitchCommands - This function returns a new map, built
// from tree, that leaves out any command whose Run names some Command
// Level's own EnterCommand or ExitCommand, checked recursively through
// Subcommands, "admin" and "configure terminal" for instance, in this
// project's own shipped tree. tree itself is never mutated, and
// neither is any Command value already inside it; a Command that
// needs a smaller Subcommands map is copied first, never edited in
// place, since the very same pointer, see MergeTrees's own doc
// comment, is very likely still reachable from whichever Command
// Level actually owns it.
//
// This exists because a level switching command only ever makes
// sense from the one exact Command Level RequireCurrentCommandLevel
// actually checks against, a session's real current position, never
// from a level reached underneath that one, config or config
// interface stacked on top of exec for instance. InheritParent's own
// job is carrying an ordinary command, "show" for instance, forward
// so it stays available at every level reached from there on,
// matching a real Cisco or HP privileged exec. A level switching
// command is structurally different: it exists to move a session
// between two exact levels, and inheriting it any further only
// leaves it sitting in a listing where typing it can never succeed,
// refused every time by RequireCurrentCommandLevel with a "you must
// be in %s mode first" a session had no way to have expected, since
// the command was offered to it in the first place. Excluding it
// here, at the one place InheritParent actually happens, means every
// simple Command Level manifest, most of them, that never sets
// InheritParent at all, or that inherits from a level with no
// EnterCommand or ExitCommand of its own, sees no behavior change
// whatsoever.
//
// This is called once per InheritParent merge, LoadTreeStructure's
// own job, always against a level's Parent's already fully resolved
// raw tree, never against a level's own TreeFile contents; a level
// keeps its own declared commands, level switching or not, exactly
// as declared, this only ever trims what a further descendant would
// otherwise also inherit.
//
// A container command, one with Subcommands but no Run of its own,
// "configure" in this project's own shipped tree for instance, is
// dropped entirely once filtering leaves it with no Subcommands and
// no Run left at all, rather than surviving as an empty, unusable
// stub a session could resolve to but never actually run anything
// through.
func filterLevelSwitchCommands(tree map[string]*Command, switchNames map[string]bool) map[string]*Command {
	if len(switchNames) == 0 {
		return tree
	}
	out := make(map[string]*Command, len(tree))
	for name, cmd := range tree {
		if cmd.Run != "" && switchNames[cmd.Run] {
			continue
		}
		effective := cmd
		if len(cmd.Subcommands) > 0 {
			filteredSub := filterLevelSwitchCommands(cmd.Subcommands, switchNames)
			if len(filteredSub) != len(cmd.Subcommands) {
				if cmd.Run == "" && len(filteredSub) == 0 {
					continue
				}
				copyCmd := *cmd
				copyCmd.Subcommands = filteredSub
				effective = &copyCmd
			}
		}
		out[name] = effective
	}
	return out
}

// RequireCurrentCommandLevel - This function is the one place "you must
// be here to go there" is actually checked. It returns an error unless
// the session's current Command Level, whatever CommandLevelStack
// frame is on top, root or pushed, checked through Current().Name, is
// exactly parent.
// Every hand-written cmd_*.go file that enters a Command Level
// calls this. A level reached by swapping the root frame calls it
// indirectly through EnterCommandLevel, and a nested mode such as
// config or config interface calls it directly. There is exactly one
// enforcement mechanism, used the same way regardless of which kind
// of level is being entered. target is used only for the error
// message, naming both what is being entered and where the session
// needs to be first, see the commandlevel.wrong_level key in
// var/lang/en.yaml. Only parent is actually checked.
//
// This checks Current().Name rather than Session.CommandLevel on
// purpose. Session.CommandLevel only ever names a level actually
// reached through EnterCommandLevel or ExitCommandLevel, so it could
// never express "you must currently be inside config to enter config
// interface" in the first place, since config is never a
// Session.CommandLevel value at all. Current().Name works uniformly
// for both cases because EnterCommandLevel and ExitCommandLevel keep
// the root frame's Name in sync with Session.CommandLevel through
// CommandLevelStack.SetRootTree, so at the root, Current().Name and
// Session.CommandLevel always agree, and for a pushed frame,
// Current().Name is simply that frame's own Name.
func RequireCurrentCommandLevel(ctx *AppContext, target, parent string) error {
	if ctx.Position.Current().Name != parent {
		return fmt.Errorf("%s", ctx.Translator.T("commandlevel.wrong_level", target, parent))
	}
	return nil
}

// withinReauthGracePeriod - This function reports whether level's own
// LastAuthenticatedAt is recent enough, against ctx.ReauthGracePeriod,
// that EnterCommandLevel should let a session back in without
// prompting again. False whenever ReauthGracePeriod is zero, this
// project's original behavior, every entry always prompts, or when
// LastAuthenticatedAt is still its zero value, meaning no real prompt
// has ever actually succeeded for this level in this run of the
// program. This is deliberately never satisfied by anything other than
// a real, live password check succeeding inside EnterCommandLevel
// itself; nothing that only sets what PasswordHash equals is allowed
// to set LastAuthenticatedAt, see that field's own doc comment in
// model.go for why treating the two as the same thing would be a real
// pass the hash vulnerability.
func withinReauthGracePeriod(ctx *AppContext, level *CommandLevel) bool {
	if ctx.ReauthGracePeriod <= 0 {
		return false
	}
	if level.LastAuthenticatedAt.IsZero() {
		return false
	}
	return time.Since(level.LastAuthenticatedAt) < ctx.ReauthGracePeriod
}

// withinSuConfigTrust - This function reports whether some level
// marked GrantsReplayTrust was really, live authenticated recently
// enough, against ctx.SuConfigTrustWindow, that EnterCommandLevel
// should let a session into level, any level, without prompting
// again. False whenever SuConfigTrustWindow is zero, this project's
// original behavior, every entry always prompts, or when no level in
// ctx.Levels carries GrantsReplayTrust at all, or when the one, or
// more, that do have never actually been entered with a real password
// in this run of the program.
//
// This is deliberately as narrow, in what it accepts as proof, as
// withinReauthGracePeriod above: only a real, live password check
// succeeding inside EnterCommandLevel itself ever sets
// LastAuthenticatedAt on any level, GrantsReplayTrust included,
// see that field's own doc comment in model.go. What this function
// adds on top is scope, not a weaker kind of proof: one such real
// check, at a level built specifically to require one,
// var/tree/README.md's own admin for instance, is trusted widely
// enough to also waive every other level's own prompt for a bounded
// window afterward, so a whole saved configuration can be pasted back
// in and reproduce the exact same access it had before without
// hitting a prompt at every gated level along the way. See
// EnterCommandLevel's own case for this, right above, for why it
// deliberately never marks the level actually being entered as though
// its own credential had been proven too.
func withinSuConfigTrust(ctx *AppContext) bool {
	if ctx.SuConfigTrustWindow <= 0 {
		return false
	}
	if ctx.Levels == nil {
		return false
	}
	for _, l := range ctx.Levels.Order {
		if !l.GrantsReplayTrust {
			continue
		}
		if l.LastAuthenticatedAt.IsZero() {
			continue
		}
		if time.Since(l.LastAuthenticatedAt) < ctx.SuConfigTrustWindow {
			return true
		}
	}
	return false
}

// EnterCommandLevel - This function does the generic, mechanical work
// of moving a session into a root swap Command Level. That work is
// RequireCurrentCommandLevel, the password check, or honoring a recent
// enough LastAuthenticatedAt in its place, see withinReauthGracePeriod
// above, updating Session.CommandLevel and Session.CommandLevelEnteredAt,
// and swapping the root CommandLevelStack frame through SetRootTree. It
// deliberately does not print, log, or audit anything. Whether and
// how to tell the user that the session moved, whether to write a log
// line or an audit entry, or to say nothing at all, is entirely the
// calling cmd_*.go file's decision, not this framework's.
// Different projects, or even different levels in the same project,
// may want different feedback here. See cmd/core/cmd_enable.go for the
// calling convention.
//
// The return values distinguish four outcomes the caller needs to
// tell apart:
//   - entered true, err nil: the session moved into level. The caller
//     decides what, if anything, to report.
//   - entered false, err nil: the session was already at level. This
//     is a no-op, not a failure. The caller decides what, if
//     anything, to report, typically something like "Already in
//     %s.", distinct from the entered true message.
//   - entered false, err set, rate limited: level.RateLimiter has
//     recorded enough recent failures to trigger a lockout, see
//     auth.RateLimiter's own doc comment. This is checked and
//     returned before ever prompting for a password, so a locked out
//     session is not even invited to try again. The error is already
//     a translated message naming how much longer the lockout has to
//     run.
//   - entered false, err set, refused: either the wrong current
//     Command Level (see RequireCurrentCommandLevel) or a wrong or
//     missing password. The error is already a translated message
//     that the caller can return as is, or replace with its own.
//
// This is only meant for levels reached by swapping the root frame,
// such as an exec or diagnostic level, tracked by Session.CommandLevel.
// A nested, stacking mode such as config or config interface is
// structurally different, since more than one of those can be active
// at once, which SetRootTree cannot express. Those call
// RequireCurrentCommandLevel directly and push a CommandLevelStack
// frame themselves instead, see cmd_configure.go and cmd_interface.go.
func EnterCommandLevel(ctx *AppContext, level, parent *CommandLevel) (entered bool, err error) {
	if ctx.Session.CommandLevel == level.Name {
		return false, nil
	}
	if err := RequireCurrentCommandLevel(ctx, level.Name, level.Parent); err != nil {
		return false, err
	}
	// The role gate is checked before the password gate below, and
	// before ever prompting, the same "do not invite a session to try
	// at all" reasoning the rate limit check inside the default case
	// below already follows. ReplayingStartupConfig waives this the
	// same way it waives the password gate right below it: nobody has
	// typed anything at all yet during boot time replay, no Session
	// even exists, so there is no logged in user's own roles to check
	// in the first place. See AppContext.ReplayingStartupConfig's own
	// doc comment. AllowedRoles and PasswordHash are independent
	// gates, see Command.AllowedRoles's own doc comment; both are
	// enforced when both are set on the same level.
	if !ctx.ReplayingStartupConfig && len(level.AllowedRoles) > 0 && !Authorized(ctx, level.AllowedRoles) {
		return false, fmt.Errorf("%s", ctx.Translator.T("commandlevel.access_denied"))
	}
	switch effectiveHash := level.EffectivePasswordHash(); {
	case ctx.ReplayingStartupConfig:
		// Trusted boot time replay, see AppContext.ReplayingStartupConfig's
		// own doc comment. Checked first, before effectiveHash is even
		// looked at, since this waves a session through regardless of
		// whether level actually has a password configured at all.
		// Deliberately does NOT set level.LastAuthenticatedAt: nobody
		// typed a credential here, real or otherwise, the trust comes
		// from this code running as this operating system process at
		// all, a different kind of proof than anything
		// withinReauthGracePeriod or withinSuConfigTrust ever accept,
		// and letting it masquerade as one of those would let a
		// session that later reenters this same level, for real, skip
		// a prompt it never actually earned.
	case effectiveHash == "":
		// No password at all, nothing to check.
	case withinReauthGracePeriod(ctx, level):
		// Within the grace period from a real, earlier prompt this
		// same run of the program already answered; see
		// withinReauthGracePeriod below. Sliding the window forward
		// again here, the same way sudo's own cached authentication
		// does, means a session that keeps stepping back into this
		// level regularly never has to re-answer as long as it never
		// actually goes stale, rather than the window only ever
		// counting down from the first prompt.
		level.LastAuthenticatedAt = time.Now()
	case withinSuConfigTrust(ctx):
		// Granted through admin's own recent, real, live credential
		// check, see withinSuConfigTrust below, not through anything
		// at this level itself. Deliberately does NOT set
		// level.LastAuthenticatedAt: nobody actually proved this
		// level's own credential, only admin's, and letting
		// this branch mark this level as if it had been would falsely
		// let a later, ordinary reentry skip the prompt through
		// withinReauthGracePeriod once SuConfigTrustWindow itself has
		// long since expired.
	default:
		// Checked before ever prompting. See this function's own doc
		// comment on the four return value cases for why a locked out
		// session is not invited to try again.
		if ok, retryAfter := level.RateLimiter.Allow(); !ok {
			return false, fmt.Errorf("%s", ctx.Translator.T("auth.too_many_attempts", auth.RoundForDisplay(retryAfter)))
		}
		password, perr := auth.PromptSecret(os.Stdout, int(os.Stdin.Fd()), ctx.Translator)
		if perr != nil || !auth.VerifyPassword(effectiveHash, password) {
			level.RateLimiter.RecordFailure()
			return false, fmt.Errorf("%s", ctx.Translator.T("commandlevel.access_denied"))
		}
		level.RateLimiter.RecordSuccess()
		level.LastAuthenticatedAt = time.Now()
	}
	ctx.Session.CommandLevel = level.Name
	ctx.Session.CommandLevelEnteredAt = time.Now()
	ctx.Position.SetRootTree(level.Name, level.PromptSuffix, level.Tree)
	ctx.Logger.Debugln("DEBUG: session entered Command Level", level.Name, "for user", ctx.Session.Username)
	return true, nil
}

// ExitCommandLevel - This function is the mirror of EnterCommandLevel.
// It moves a session back from level to parent. It follows the same
// no printing, no logging beyond Debugln policy, and returns the same
// shape, exited instead of entered, for the same reason, see
// EnterCommandLevel's own doc comment. exited is false with a nil
// error when the session was not actually at level, a no-op rather
// than a failure, for example "disable" run twice in a row, or from
// somewhere it does not apply.
func ExitCommandLevel(ctx *AppContext, level, parent *CommandLevel) (exited bool, err error) {
	if ctx.Session.CommandLevel != level.Name {
		return false, nil
	}
	ctx.Session.CommandLevel = parent.Name
	ctx.Position.SetRootTree(parent.Name, parent.PromptSuffix, parent.Tree)
	ctx.Logger.Debugln("DEBUG: session left Command Level", level.Name, "back to", parent.Name, "for user", ctx.Session.Username)
	return true, nil
}
