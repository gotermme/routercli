// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"github.com/gologme/log"
	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/i18n"
	"github.com/gotermme/routercli/paging"
)

// ----------------------------------------------------------------------
// Define Object Model
// ----------------------------------------------------------------------

// helpEntry - This type is one line of dynamically built help output. It
// holds a full command path, such as "show running-config", and its
// resolved description.
type helpEntry struct {
	path string
	desc string
}

// commandTreeFile - This type is the top-level shape of a tree file.
// Everything lives under a single "commands:" key. This is the only
// wrapper type this file needs. Command itself decodes directly
// through its own yaml tags, see Command's own doc comment, rather
// than through a separate mirror type.
type commandTreeFile struct {
	Commands commandMap `yaml:"commands"`
}

// CommandLevelFrame - This type represents one entered Command Level. A
// CommandLevelStack, defined below, is a stack of these. The current Command
// Level is whichever frame is on top.
//
// Name is a short identifier for this Command Level, such as "config"
// or "config-if". It is not shown to the user directly, PromptSuffix is what
// actually appears in the prompt, but it is useful for a handler that wants to
// know which Command Level it is in without matching against a formatted
// prompt string.
//
// PromptSuffix is appended to the base prompt while this frame is current, for
// example "(config)" so the prompt reads "router(config)#// ". It is empty for
// the root or exec frame, which has no suffix.
//
// Tree is the command tree reachable while this frame is current. It is built
// by merging this Command Level's own tree file with
// var/tree/level_common.yaml, see MergeTrees, so every Command Level gets
// help, exit, and end for free without redefining them.
//
// Context is an arbitrary per-frame value a Command Level's own commands can
// stash data in and read back, without CommandLevelStack or this package
// needing to know what it means. For example, entering
// "interface eth0" pushes a config interface frame with Context set to
// "eth0", and that level's own description and shutdown handlers type assert it
//
//	back to know which interface they are editing. This is the same any-typed,
//	type-assert-at-the-edge pattern AppContext.State uses, for the same reason.
//	This package stays generic, and project-specific meaning lives in
//	cmd/product.
type CommandLevelFrame struct {
	Name         string
	PromptSuffix string
	Tree         map[string]*Command
	Context      any
}

// CommandLevelStack - This type is the runtime Command Level state for one
// session, a stack of CommandLevelFrame, with the root at index 0. The root
// frame can never be popped. It represents the base Command Level, or
// whichever level a session has moved to once SetRootTree has swapped it, see
// SetRootTree's own doc comment. Leaving the root is what quits the program,
// see ErrQuit, not something CommandLevelStack itself decides.
type CommandLevelStack struct {
	frames []CommandLevelFrame
}

// Command - This type represents a single command in a Command Level,
// the building block every tree in this project is made of, whether
// loaded from YAML or constructed by hand.
//
// Every field except RunFunc and PasswordRateLimiter is decoded
// directly from YAML through its own tag, the same direct-tag pattern
// auth.User and config.SystemConfig already use, rather than through a
// separate mirror type. See var/tree/README.md for what each YAML
// property means.
//
// RunFunc cannot carry a yaml tag at all, since a Go function has no
// YAML representation. Run holds the raw "run:" string a YAML file
// gave for this command. LoadTree resolves it into RunFunc by looking
// that name up in the handler registry immediately after decoding,
// see resolveHandlers in loader.go. A command built directly in Go,
// rather than loaded from YAML, sets RunFunc itself and can leave Run
// empty. A container command with only Subcommands and no "run:" of
// its own leaves both empty, which is why RunFunc is nil-checked at
// dispatch time, see main.go's runLoop.
//
// PasswordRateLimiter gates repeated attempts against PasswordHash, the same
// way CommandLevel.RateLimiter gates a Command Level's own PasswordHash. See
// that field's doc comment and auth.RateLimiter's own for the general
// mechanism. This is nil until main.go wires one in, for every command with a
// non-empty PasswordHash, after loading. A nil RateLimiter behaves exactly
// like a disabled one, so this field is safe to read before that wiring
// happens.
//
// Requires names a flag that must be true for this command, and its
// Subcommands, to exist in the tree at all, checked once at startup
// by PruneDisabledCommands, see prune.go, called from main.go right
// after command.LoadTreeStructure returns. This is left empty, the
// default, for a command that is always available. This differs from
// PasswordHash and Command Level's own PasswordHash above in kind,
// not just in name: those gate whether a reachable command's own
// RunFunc is allowed to execute, while Requires gates whether the
// command is reachable, or even shown in help or tab completion, at
// all. That distinction matters for a command whose whole reason to
// exist depends on a feature being turned on, "password change" when
// config.SystemConfig.EnableCLIAuthentication is false for instance,
// where the right behavior is for the command not to exist rather
// than to exist and refuse. See var/tree/level_user.yaml for this
// project's own use of this field.
type Command struct {
	Desc         string `yaml:"desc"`
	Help         string `yaml:"help"`
	DescKey      string `yaml:"desc_key"`
	HelpKey      string `yaml:"help_key"`
	ArgHelp      string `yaml:"arghelp"`
	ArgHelpKey   string `yaml:"arghelp_key"`
	Run          string `yaml:"run"`
	Alias        string `yaml:"alias"`
	Hidden       bool   `yaml:"hidden"`
	Negatable    bool   `yaml:"negatable"`
	PasswordHash string `yaml:"password_hash"`
	MinArgs      *int   `yaml:"minargs"`
	MaxArgs      *int   `yaml:"maxargs"`
	MaxArgLength int    `yaml:"maxarglength"`
	Requires     string `yaml:"requires"`

	// Pageable, false by default, opts this command into output
	// capture, "| include", "| exclude", "| begin" filtering, and the
	// "--More--" pager, see package paging. Piping a command that is
	// not Pageable is a real error at the command line, "% ... does
	// not support output filtering or paging", not a silent no-op.
	//
	// This is deliberately opt-in rather than universal. Output
	// capture works by redirecting the real, process wide os.Stdout
	// for the duration of one RunFunc call, see
	// paging.CaptureOutput, which is only safe for a handler whose
	// entire output is produced up front, with no interactive prompt
	// of its own read from the terminal partway through. A handler
	// that does, "totp enable" or "password change" for instance,
	// MUST never set this, or its own prompt text would be silently
	// swallowed into the capture buffer instead of reaching the
	// terminal where a person needs to see it before typing a blind
	// response. Marking every report style command, "show running-config"
	// and similar, is the shipped convention this project follows,
	// matching real Cisco and HP, which paginate display commands and
	// nothing else.
	Pageable bool `yaml:"pageable"`

	Subcommands commandMap `yaml:"subcommands"`

	RunFunc             HandlerFunc       `yaml:"-"`
	PasswordRateLimiter *auth.RateLimiter `yaml:"-"`

	// DefIndex is this command's position among its own siblings, the
	// other keys of whichever "commands:" or "subcommands:" mapping it
	// was itself defined in, in the order that mapping actually reads
	// in its source YAML file. LoadTree's own commandMap.UnmarshalYAML
	// is what sets this, since Go's map[string]*Command has no
	// intrinsic order of its own to recover it from afterward. This is
	// what ListOptions.Alphabetical false, tree definition order,
	// sorts by. A Command built directly in Go rather than loaded from
	// YAML, see intPtr's own doc comment for that convention, leaves
	// this at its zero value, which is harmless: every hand-built
	// command in the same test tree ties at 0, and every real,
	// loaded tree always sets it. It is not decoded from YAML.
	DefIndex int `yaml:"-"`

	// IsCommonCommand is true for every command that came from the
	// common tree file, var/tree/level_common.yaml by default, help,
	// "?", exit, and end, merged into every Command Level's Tree by
	// MergeTrees unless SkipCommonMerge. LoadTreeStructure sets this,
	// once, right after loading the common tree and before merging it
	// anywhere, see markCommonCommands in treestructure.go. This is
	// what ListOptions.MergeCommon false, append common commands after
	// everything else, checks to decide which group a command belongs
	// to. It is not decoded from YAML, a tree file has no way to mark
	// its own commands as common, only LoadTreeStructure itself knows
	// which tree it loaded as the common one.
	IsCommonCommand bool `yaml:"-"`
}

// ListOptions - This type controls how a listing of more than one
// command name at the same tree level is ordered, driven by
// config.SystemConfig's own AlphabeticalCommandOrder and
// MergeCommonCommands settings. See SortCommandNames, which every
// caller that prints more than one command name side by side,
// HelpText and both of package completer's own print paths, funnels
// through so a listing's order is controlled by the same two flags no
// matter which of those paths produced it.
//
// This type lives in package command, alongside Command and
// ResolveResult, rather than being read directly from
// config.SystemConfig, the same reasoning Command.ResolvedDesc already
// keeps *i18n.Translator as a plain parameter for. Package command
// must not depend on package config, so main.go builds one of these
// from the loaded SystemConfig once, at startup, and threads it
// through AppContext.ListOptions from there, see AppContext's own doc
// comment.
type ListOptions struct {
	// Alphabetical, when true, the default, sorts a listing by command
	// name. When false, a listing instead follows DefIndex, the order
	// commands are defined in their tree file, own commands before
	// common commands regardless of MergeCommon, since interleaving
	// two separate files' own definition order the way MergeCommon's
	// alphabetical form does has no sensible meaning. See
	// SortCommandNames's own doc comment for the full reasoning.
	Alphabetical bool

	// MergeCommon, when true, the default, sorts a common command,
	// help, "?", exit, end, into a listing's normal alphabetical
	// position among every other command, matching what real Cisco
	// and HP devices actually do. When false, every common command is
	// appended after every other command instead, alphabetical among
	// themselves. This only has an effect when Alphabetical is also
	// true, see Alphabetical's own doc comment for why definition
	// order always separates the two groups regardless of this
	// setting.
	MergeCommon bool
}

// DefaultListOptions - This function returns the ordering this
// project's own defaults, and real Cisco and HP behavior, both use:
// alphabetical, with common commands merged into their normal
// alphabetical position. See ListOptions's own doc comment for what
// each field means, and config.DefaultSystemConfig for where these
// same two defaults are set for a real deployment.
func DefaultListOptions() ListOptions {
	return ListOptions{Alphabetical: true, MergeCommon: true}
}

// ResolveResult - This type is what Resolve() returns, everything a
// caller needs to know about what a line of typed tokens actually
// referred to.
type ResolveResult struct {
	FullName  []string // resolved command tokens, e.g. ["show", "running-config"]. This is the real command; "no" is never part of it.
	Command   *Command // directives and handler for the final matched command, nil if none matched
	Args      []string // leftover tokens once no further command match is possible
	Ambiguous []string // candidate names, set only when a token prefix matches more than one command
	AmbigAt   int      // index into the original tokens where the ambiguity occurred
	Negated   bool     // true if the line started with "no", see Resolve()'s own doc comment

	// AmbiguousTree is the tree map Ambiguous's own candidate names
	// were drawn from, set alongside Ambiguous, nil whenever Ambiguous
	// is. A caller that wants to print those names in something other
	// than Resolve()'s own plain alphabetical order, see
	// SortCommandNames, needs this to look each one's Command back up
	// by name, which is why Resolve() hands it back rather than
	// leaving every such caller to re-walk FullName down from its own
	// copy of the root tree to reconstruct the same map.
	AmbiguousTree map[string]*Command

	// RunnableAsIs is true when Command is both set and directly
	// executable exactly as typed, pressing Enter right now would
	// actually run it, matching real Cisco and HP's own "<cr>"
	// notation. This means Command.RunFunc is not nil, and Args, with
	// one single trailing empty string stripped first, since that is
	// only ever the synthetic "nothing typed yet here" placeholder
	// completer.OnChange and HelpForPath's own callers append, never a
	// real argument, satisfies both Command.MinArgs and
	// Command.MaxArgs. See runnableAsIs, which computes this the same
	// way regardless of which of Resolve()'s several return points is
	// actually taken.
	RunnableAsIs bool
}

// AppContext - This type carries the shared dependencies every command
// handler needs. Handlers register themselves from init(), see
// Register below, which runs before main() has constructed the real
// State, Logger, Session, or Tree, so there is nothing yet to close
// over at that point. AppContext is passed explicitly into each
// handler at call time instead, which also means a handler's own
// signature shows exactly what it depends on.
//
// State is example or project-specific session data, such as the
// demo's Description value. It is deliberately typed as any rather
// than a concrete struct, since this package is the generic, reusable
// framework and must not know about project-specific state. A command
// in cmd/product type asserts this to its own concrete type, see
// cmd/product/state.go for the pattern. cmd/core, by design, never
// touches State at all, see that package's own doc comment.
//
// Logger is the shared gologme/log logger used for debug-level
// tracing.
//
// Session is the current login and Command Level state, see package
// auth. It is always non-nil, so CommandLevel and
// CommandLevelEnteredAt always have somewhere to live regardless of
// whether AuthRequired is even in use. Check Session.Authenticated,
// not Session == nil, to know whether a real login happened. Username
// is empty and Authenticated is false for a session that was never
// logged in, which is the correct default when AuthRequired is off.
//
// Users is the loaded user database, so a handler can look up the
// current user's own record, such as their TOTPSecret, without
// main.go needing to pass that one value through some other channel.
// This is nil if AuthRequired is off.
//
// UsersFile is the path Users was loaded from, so a handler that
// changes a User's own record, such as the totp enable and totp
// disable commands in package core (cmd/core), can call auth.SaveUsers
// against the same file rather than needing that path threaded through
// some other channel. This is empty if AuthRequired is off, the same
// as Users itself.
//
// TOTPIssuer is the name shown in a user's authenticator app next to
// their account name, passed straight through to
// auth.TOTPProvisioningURI. This mirrors config.SystemConfig's own
// TOTPIssuer setting, so a handler enrolling a user in TOTP mid-
// session presents the same issuer name main.go's --mfa flag already
// does.
//
// TOTPMaxAttempts is how many times in a row a handler such as totp
// enable or totp disable lets a session retype a rejected TOTP code
// before giving up, mirroring config.SystemConfig's own
// TOTPMaxAttempts setting, the same retry ceiling
// auth.PromptLogin already enforces for a login attempt through its
// own maxAttempts parameter.
//
// PasswordPolicy is the set of rules a new password must satisfy,
// passed straight through to auth.ValidatePassword by the password
// change handler in cmd/core/cmd_password.go. It mirrors
// config.SystemConfig's own Password* settings, built once from that
// SystemConfig by main.go, the same wiring pattern TOTPIssuer already
// follows.
//
// PasswordChangeMaxAttempts is how many times in a row the password
// change handler lets a session retry, both re-authenticating with
// its current password and a second factor code if one is required,
// and re-typing a new password that failed to match its own
// confirmation or PasswordPolicy above, before giving up on that
// attempt, mirroring config.SystemConfig's own
// PasswordChangeMaxAttempts setting, the same retry ceiling shape
// TOTPMaxAttempts above already uses.
//
// AuthProvider is the backend cmd/core/cmd_password.go's own
// re-authentication step, checking the current password before a
// change is allowed, checks a typed password against, see
// auth.AuthProvider. This is the same value main.go built once,
// through auth.NewAuthProvider, from config.SystemConfig's
// AuthProviders and CLIAuthProvider settings, and already used for
// the session's own original login, so a password change is checked
// against whatever backend actually owns this account rather than
// always assuming the local users.yaml database. This is nil if
// EnableCLIAuthentication is off, since there is then no CLI login of
// its own for any backend to have checked in the first place.
//
// Audit records what commands ran. It is nil-safe, see the Auditor
// interface below, so this package does not need to import package
// auditlog directly.
//
// Translator resolves user-facing message keys to the active
// language's text, see package i18n. A handler calls
// ctx.Translator.T("key") instead of hardcoding English strings, so a
// new language is a new var/lang/<code>.yaml file, not a source
// change.
//
// Position is the session's current stack of entered Command Levels,
// such as exec, config, or config interface, see
// commandlevelstack.go. This is what actually determines which
// commands are reachable at any given moment. runLoop and the
// completer both resolve against Position.Current().Tree, not
// anything on Levels below. A command that enters a nested Command
// Level, such as "configure terminal", does so by calling
// ctx.Position.Push(...), see cmd/core/cmd_configure.go.
//
// Levels is every Command Level the project defines, loaded from one
// manifest, var/tree/tree_structure.yaml, see TreeStructure and
// CommandLevel. This covers both levels reached by swapping the root
// frame, where Session.CommandLevel names which one a session is
// currently in, and nested, stacking modes such as config and config
// interface, which a hand-written cmd_*.go file pulls by name
// through ctx.Levels.ByName. Every level's enter and exit command is a
// hand-written cmd_*.go file. There is no dynamic registration
// anywhere in package command. Every level here was already loaded,
// merged with its parent chain if applicable, and merged with the
// common tree at startup, so a broken tree file fails the program
// immediately instead of only being discovered the first time someone
// types the command that needed it. This is also how "show
// running-config" lists every level with a secret configured, and how
// cmd_password_manager.go sets the current level's own secret through
// ctx.Levels.ByName[ctx.Session.CommandLevel].
//
// Negated is true for exactly the duration of one RunFunc() call, when
// that call was reached through a leading "no", see
// ResolveResult.Negated and Resolve()'s own doc comment. runLoop sets
// this immediately before calling RunFunc() and clears it immediately
// after, so a handler should read it at the start of its own function
// body rather than stash it for later, since it is not meaningful
// once RunFunc() returns. A Negatable handler typically branches on this
// once, near the top of its function.
//
// ListOptions controls how HelpText and HelpForPath order a listing
// of more than one command name, see that type's own doc comment. It
// is built once, at startup, from config.SystemConfig's own
// AlphabeticalCommandOrder and MergeCommonCommands settings, main.go's
// job, and read from here by both the "help" command's own handler
// and runLoop's non-interactive "?" fallback, and passed by main.go
// straight through to completer.New for the interactive "?" and Tab
// paths, see that function's own doc comment. Its zero value,
// Alphabetical false and MergeCommon false, is not this project's
// default, DefaultListOptions is, so a caller that constructs an
// AppContext by hand, in a test for instance, needs to set this
// explicitly rather than relying on the zero value doing the right
// thing.
type AppContext struct {
	State           any
	Logger          *log.Logger
	Session         *auth.Session
	Users           auth.Users
	UsersFile       string
	TOTPIssuer      string
	TOTPMaxAttempts int

	PasswordPolicy            auth.PasswordPolicy
	PasswordChangeMaxAttempts int
	AuthProvider              auth.AuthProvider

	Levels      *TreeStructure
	Audit       Auditor
	Translator  *i18n.Translator
	Position    *CommandLevelStack
	Negated     bool
	ListOptions ListOptions

	// PageLines is the live, per session override behind
	// paging.EffectivePageLines, nil until "terminal length <n>" is
	// typed, matching real Cisco's own session scoped, never
	// persisted terminal length. Once set, including to zero,
	// "terminal length 0", the real convention for "never pause,"
	// this session's own pager honors it exactly, in place of
	// auto-detecting the real terminal's own height. See
	// cmd/core/cmd_terminal.go's "terminal length" handler, the only
	// place this is ever written.
	PageLines *int

	// DefaultPageLines is config.SystemConfig.DefaultPageLines,
	// copied here once at startup, the fallback page size
	// paging.EffectivePageLines falls back to only when PageLines is
	// unset and the real terminal's own height cannot be read, piped
	// or redirected stdin for instance.
	DefaultPageLines int

	// PagingEnabled is config.SystemConfig.PagingEnabled, copied here
	// once at startup. This is the deployment wide switch for the
	// interactive "--More--" pause itself, independent of PageLines;
	// a deployment can keep "| include" and the rest of output
	// filtering available while never blocking a session on a
	// keypress. See paging.Display.
	PagingEnabled bool

	// FilterMode is the live, mutable match mode "| include",
	// "| exclude", and "| begin" patterns are checked with,
	// substring by default, matching config.SystemConfig.FilterMatchMode's
	// own startup value, changed at runtime through "terminal
	// filter-mode <substring|regex>". See paging.FilterMode.
	FilterMode paging.FilterMode

	// MaxFilterChainDepth is config.SystemConfig.MaxFilterChainDepth,
	// copied here once at startup, the most "| ..." stages one typed
	// line may chain together, checked by paging.ParseStages. A value
	// of zero disables output filtering entirely for this deployment.
	MaxFilterChainDepth int

	// TerminalWidth is the live, per session override for how wide a
	// line this session's own terminal is, nil until "terminal width
	// <n>" is typed, matching real Cisco's own session scoped, never
	// persisted terminal width. Nothing in this package reads
	// TerminalWidth today; it exists so a handler that formats output
	// to a fixed width, wrapping a long line or laying out columns for
	// instance, has one place to find the session's own override
	// instead of every implementation inventing its own field for the
	// same idea. See cmd/core/cmd_terminal.go's "terminal width"
	// handler, the only place this is ever written.
	TerminalWidth *int
}

// Auditor - This type is the minimal interface AppContext needs from
// an audit log, so this package can avoid importing package auditlog
// directly. Any type with these methods satisfies it.
//
// WouldLog and ForceLog exist so main.go's runLoop can snapshot
// whether a command should be logged before running it, and still log
// it even if the command's own side effect was to turn logging off,
// such as "audit-log disable". See auditlog.AuditLog's own doc
// comments for the full reasoning.
type Auditor interface {
	Log(username, command string, success bool)
	WouldLog() bool
	ForceLog(username, command string, success bool)
}

// HandlerFunc - This type is the signature every command handler must
// implement, regardless of which file it lives in. args are the
// leftover tokens after the command itself resolved, already
// validated against MinArgs, MaxArgs, and MaxArgLength by the time
// this is called.
type HandlerFunc func(ctx *AppContext, args []string) error

// CommandLevel and TreeStructure, below, describe the whole
// collection of commands a CLI environment offers, organized into
// Command Levels. A Command Level is every command reachable at one
// position in that structure. A project using this framework
// describes its entire set of Command Levels in one manifest,
// var/tree/tree_structure.yaml, rather than fixing them in Go code. A
// project may want more than one privilege step, for example operator
// then technician then factory debug, each gated by its own secret,
// as well as ordinary nested modes such as configure terminal or
// interface that carry no password requirement at all. Both kinds are
// CommandLevel values, described the same way in the manifest.
//
// Every Command Level's enter and exit command is registered by a
// hand-written cmd_*.go file, see cmd/core/cmd_enable.go,
// cmd/product/cmd_diagnostic_mode.go, cmd/core/cmd_configure.go, and
// cmd/product/cmd_interface.go for examples. Nothing in this package
// registers a command dynamically from the manifest. Naming a level
// "enable" in tree_structure.yaml does not create an "enable" command
// by itself, the same way naming a command "show" in a tree file does
// not create a "show" command by itself. EnterCommand and ExitCommand
// on CommandLevel are declared, verifiable metadata describing what a
// hand-written file is expected to have registered, checked against
// the actual registry by VerifyCommandLevels below. What a Command
// Level's enter or exit command actually prints, logs, or audits on
// success, on a no-op, or on refusal is entirely up to that hand-
// written file, not an opinion this framework holds for every
// project. See EnterCommandLevel and ExitCommandLevel below for the
// mechanics.

// CommandLevel - This type describes one named Command Level in the
// project's Tree Structure. Every field except Name, Tree, and
// RateLimiter is decoded directly from YAML through its own tag, the
// same direct-tag pattern auth.User and config.SystemConfig already
// use, rather than through a separate mirror type.
type CommandLevel struct {
	// Name identifies this level. It is what Session.CommandLevel is
	// set to while a session is inside it, for a level reached by
	// swapping the root frame through EnterCommandLevel and
	// ExitCommandLevel, and it is what other levels reference through
	// Parent. This is not decoded from YAML at all. LoadTreeStructure
	// sets it to a level's own key under the manifest's "trees:" map
	// after decoding, the same way auth.LoadUsers sets User.Username
	// from that file's own map key.
	Name string `yaml:"-"`

	// TreeFile is this level's own command tree, see LoadTree, what it
	// contributes on top of, or instead of, its parent's tree,
	// depending on InheritParent.
	TreeFile string `yaml:"tree_file"`

	// IsBase marks the one level every session starts in, see
	// TreeStructure.Base, the root CommandLevelStack frame. Exactly one level
	// across the whole manifest must set this. It is unrelated to
	// Parent: a nested level such as config also has a Parent set, its
	// own, for example "exec", but is not the base. The base level
	// itself is the only one with an empty Parent.
	IsBase bool `yaml:"is_base"`

	// Parent is the Name of the level a session's current mode must be
	// in to reach this one, checked through RequireCurrentCommandLevel by
	// whichever hand-written cmd_*.go file registers this level's
	// enter command. It is empty only for the base level itself, which
	// by definition has nowhere before it.
	Parent string `yaml:"parent"`

	// InheritParent, when true, means this level's effective command
	// tree is its own TreeFile's commands merged on top of everything
	// reachable in Parent's tree, recursively up the chain, matching a
	// real Cisco or HP privileged exec, which keeps every user exec
	// command available once elevated, on top of what elevation itself
	// unlocks. If false, this level's tree is only its own TreeFile's
	// commands, and the parent's commands are not carried forward at
	// all. This is ignored without a Parent.
	InheritParent bool `yaml:"inherit_parent"`

	// EnterCommand is the name of the command that moves a session
	// from Parent into this level, declared metadata a hand-written
	// cmd_*.go file is expected to have registered under this
	// exact name, see VerifyCommandLevels, which checks that this is
	// actually true. It is empty only for the base level, which
	// nothing enters. Every other level must set this, so it can be
	// verified.
	EnterCommand string `yaml:"enter_command"`

	// ExitCommand is the name of the command that moves a session back
	// from this level to Parent without ending the session, the
	// generalized form of "disable". This may be empty for a level
	// with no dedicated exit command of its own. It can still be left
	// by the generic, always available "exit" or "end", it just has no
	// specially named command for stepping back one level while
	// staying connected. This is only meaningful alongside
	// EnterCommand.
	ExitCommand string `yaml:"exit_command"`

	// PasswordHash is the secret required to run EnterCommand
	// successfully: a bcrypt hash, or an empty string when no password
	// is needed. It can be changed at runtime, see
	// cmd/core/cmd_password_manager.go, which sets this field on whichever
	// CommandLevel the current session is in. It is seeded from the
	// manifest's own password_hash entry at load time, which is
	// optional. This is distinct from Command.PasswordHash in
	// command.go, which gates one specific command rather than entry
	// into a whole level. A project can use either, both, or neither.
	PasswordHash string `yaml:"password_hash"`

	// PromptSuffix is appended to the prompt while this level, or a
	// mode pushed from it such as config interface on top of config,
	// is current, for example "(config)". This is purely manifest
	// data, read generically by whatever hand-written cmd_*.go
	// file enters this level, see EnterCommandLevel,
	// cmd_configure.go, and cmd_interface.go.
	PromptSuffix string `yaml:"prompt_suffix"`

	// SkipCommonMerge, when true, means this level's Tree does not get
	// help, exit, and end merged in from the common tree. The default,
	// false, merges common in, since every level in this project wants
	// that. This is an opt out rather than something every level has
	// to ask for.
	SkipCommonMerge bool `yaml:"skip_common"`

	// Tree is this level's fully resolved command tree: its own
	// commands, merged with the parent chain if InheritParent, merged
	// with the common tree exactly once unless SkipCommonMerge. It is
	// populated by LoadTreeStructure, and nil until then. It is not
	// decoded from YAML.
	Tree map[string]*Command `yaml:"-"`

	// RateLimiter gates repeated EnterCommand attempts against
	// PasswordHash. See auth.RateLimiter's own doc comment for the
	// general mechanism, and EnterCommandLevel below for how it is
	// actually checked. This is nil until main.go wires one in after
	// LoadTreeStructure returns. A nil RateLimiter behaves exactly like
	// a disabled one, so this field is safe to read before that wiring
	// happens, it just means rate limiting is not active yet. It is
	// not decoded from YAML.
	RateLimiter *auth.RateLimiter `yaml:"-"`
}

// TreeStructure - This type is the whole loaded, validated manifest,
// every Command Level in the project, indexed by name for fast
// lookup, for entering or exiting a level, a hand-written
// cmd_*.go file pulling a nested mode's tree, or "password
// manager" finding the current level's own record, alongside the
// ordered slice a caller might want, for example to print every
// level's status.
type TreeStructure struct {
	ByName map[string]*CommandLevel
	Order  []*CommandLevel // load order from the manifest, base level first
}

// treeStructureFile - This type is the top-level shape of the tree
// structure manifest. Everything lives under a single "trees:" key.
// This is the only wrapper type this file needs. CommandLevel itself
// decodes directly through its own yaml tags, see its own doc
// comment, rather than through a separate mirror type.
type treeStructureFile struct {
	Trees map[string]CommandLevel `yaml:"trees"`
}

// ----------------------------------------------------------------------
// Initialization Functions
// ----------------------------------------------------------------------

// NewCommandLevelStack - This function constructs a CommandLevelStack
// with a single root frame. rootName and rootSuffix are usually the
// manifest's base Command Level's own Name and an empty string, but are
// not hardcoded here, since a different project might call its base
// level something else, or give it a PromptSuffix of its own.
func NewCommandLevelStack(rootName, rootSuffix string, rootTree map[string]*Command) *CommandLevelStack {
	return &CommandLevelStack{
		frames: []CommandLevelFrame{{Name: rootName, PromptSuffix: rootSuffix, Tree: rootTree}},
	}
}
