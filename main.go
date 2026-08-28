// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

// Package main is a Go library designed to enable command line
// interfaces that resemble popular network equipment.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/gotermme/routercli/auditlog"
	"github.com/gotermme/routercli/auth"
	_ "github.com/gotermme/routercli/cmd/core"
	"github.com/gotermme/routercli/cmd/product"
	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/completer"
	"github.com/gotermme/routercli/config"
	"github.com/gotermme/routercli/i18n"
	"github.com/gotermme/routercli/paging"
	"github.com/gotermme/routercli/tokenize"

	"github.com/chzyer/readline"
	"github.com/gologme/log"
	"github.com/pborman/getopt/v2"
	qrcode "github.com/skip2/go-qrcode"
	"golang.org/x/term"
)

// Version and Build - These global variables hold build information.
// The Build variable is populated by the Makefile and uses the Git
// Head hash as its identifier. Both variables are used in the console
// output for --version and --help.
var (
	Version = "0.1"
	Build   string
)

func main() {
	configFileName, checkConfig := processCommandLineFlags()

	// --------------------------------------------------
	// Load System Configuration
	// --------------------------------------------------
	config, err := config.LoadSystemConfig(configFileName)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load configuration:", err)
		os.Exit(1)
	}

	// --------------------------------------------------
	// Setup Logger and Logging Levels
	// --------------------------------------------------
	logOutput := io.Writer(os.Stderr)
	if config.LogFile != "" {
		if err := mkdirForFile(config.LogFile); err != nil {
			fmt.Fprintln(os.Stderr, "warning: failed to prepare log file directory, falling back to stderr:", err)
		} else if f, err := os.OpenFile(config.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640); err != nil {
			fmt.Fprintln(os.Stderr, "warning: failed to open LogFile, falling back to stderr:", err)
		} else {
			logOutput = f
			defer f.Close()
		}
	}
	logger := log.New(logOutput, "", log.LstdFlags)
	switch config.LogLevel {
	case 1:
		logger.EnableLevel("error")
		logger.EnableLevel("info")
	case 3:
		logger.EnableLevel("error")
		logger.EnableLevel("info")
		logger.EnableLevel("warn")
	case 5:
		logger.EnableLevel("error")
		logger.EnableLevel("info")
		logger.EnableLevel("warn")
		logger.EnableLevel("debug")
	}

	// ROUTERCLI_DEBUG environment variable for one-off debug runs.
	if os.Getenv("ROUTERCLI_DEBUG") != "" {
		logger.EnableLevel("debug")
	}

	// --------------------------------------------------
	// Setup translator for help files and descriptions
	// --------------------------------------------------
	catalogs, err := i18n.LoadCatalogs(config.LanguageDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load language catalogs:", err)
		os.Exit(1)
	}
	translator := i18n.New(catalogs, config.CurrentLanguage, config.DefaultLanguage)

	// --------------------------------------------------
	// Configure System
	// --------------------------------------------------
	if config.PreventEscape {
		preventEscape()
	}

	levels, err := command.LoadTreeStructure(config.TreeStructure, config.CommonTreeFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load tree structure:", err)
		os.Exit(1)
	}
	// Every Command Level in the manifest is loaded and validated,
	// including its "run:" handler references, right here at startup.
	// A broken level_config_if.yaml fails the program immediately, not
	// the first time someone actually types "interface eth0" days from
	// now. This covers every level uniformly, root swap or nested.
	// Nothing here names "config" or "config-if" specifically. Adding
	// a level to tree_structure.yaml gets it loaded and validated the
	// same way with zero code changes, beyond the one small cmd_*.go
	// file, in cmd/core or cmd/product, every level's enter and exit
	// command always needs. See command/treestructure.go's own top of
	// file comment.

	// A command whose own feature is turned off in configuration is
	// pruned out of every level's Tree entirely, right here, before
	// anything else touches these trees, rate limiter wiring included,
	// so a disabled command never shows up in help, tab completion, or
	// VerifyCommandLevels below, rather than existing and refusing.
	// See command.PruneDisabledCommands's own doc comment and
	// var/tree/level_user.yaml, which sets requires: totp and
	// requires: password_change on its own two container commands.
	// featureFlags is the complete set of flag names any tree file in
	// this project is allowed to reference through requires:, a naming
	// convention private to this file, not something package command
	// itself defines. Adding a new gated feature later means adding
	// its own entry here, naming whichever SystemConfig boolean
	// actually controls it.
	featureFlags := map[string]bool{
		"totp":            config.EnableTOTPAuthentication,
		"password_change": config.EnableCLIAuthentication,
	}
	for _, level := range levels.Order {
		if err := command.PruneDisabledCommands(level.Tree, featureFlags, ""); err != nil {
			fmt.Fprintln(os.Stderr, "failed to prune disabled commands from the tree:", err)
			os.Exit(1)
		}
	}

	// Rate limiters are wired in here, right after loading rather than
	// during it, deliberately. LoadTreeStructure's job is building a
	// correct TreeStructure from the manifest. Nothing about policy,
	// such as how many attempts before a lockout, belongs in that
	// function, the same separation of concerns VerifyCommandLevels, a
	// few lines below, already follows. Every level gets a RateLimiter
	// unconditionally, even one with no PasswordHash set right now,
	// since "password manager", cmd/core/cmd_password_manager.go, can
	// set one at any time while the program is running, and the
	// limiter needs to already be there, ready, the moment that
	// happens. Per-command limiters are narrower, only commands that
	// already have a PasswordHash at load time get one, since, unlike
	// a Command Level's secret, nothing in this project currently sets
	// Command.PasswordHash after the tree is loaded.
	for _, level := range levels.Order {
		level.RateLimiter = auth.NewRateLimiter(config.CommandLevelMaxAttempts, config.CommandLevelAttemptWindow.AsDuration(), config.CommandLevelLockoutDuration.AsDuration())
	}
	attachPasswordRateLimiters(levels, config.CommandPasswordMaxAttempts, config.CommandPasswordAttemptWindow.AsDuration(), config.CommandPasswordLockoutDuration.AsDuration())

	// VerifyCommandLevels is a separate pass from loading, see its own
	// doc comment for why. It confirms every level's declared
	// enter_command and exit_command actually correspond to a real,
	// registered command, meaning someone really did write the
	// cmd_*.go file, in cmd/core or cmd/product, the manifest expects,
	// catching a typo or a forgotten file right here instead of the
	// first time a user types the command and gets "unknown command".
	// This runs
	// unconditionally, not just under --check-config, matching this
	// project's own convention that a broken configuration fails
	// loudly at startup.
	if problems := command.VerifyCommandLevels(levels); len(problems) > 0 {
		for _, p := range problems {
			fmt.Fprintln(os.Stderr, "%", p)
		}
		os.Exit(1)
	}
	warnPlaintextLevelSecrets(logger, levels)
	if checkConfig {
		fmt.Println("configuration OK -", len(levels.Order), "Command Levels verified")
		os.Exit(0)
	}

	audit := auditlog.New(config.AuditLogFile, logger)
	if config.AuditLogEnabled {
		if err := mkdirForFile(config.AuditLogFile); err != nil {
			fmt.Fprintln(os.Stderr, "failed to prepare audit log directory:", err)
			os.Exit(1)
		}
		if err := audit.Enable(); err != nil {
			fmt.Fprintln(os.Stderr, "failed to open audit log:", err)
			os.Exit(1)
		}
	}
	defer audit.Close()

	base := levels.Base()

	// A Session is always constructed, regardless of AuthRequired.
	// Elevation, "enable" or whatever a project's own manifest names
	// its enter commands, needs somewhere to track CommandLevel and
	// CommandLevelEnteredAt even when the separate multi-user login
	// system is not in use at all. See command.AppContext's doc
	// comment on Session for the reasoning. A session with
	// AuthRequired off simply stays with Authenticated false and
	// Username empty forever, which is the correct never logged in
	// state, not a special case handlers need to think about.
	// CommandLevel is set to the base level's name right here, since
	// auth.NewSession itself cannot do this: package auth does not
	// know what a CommandLevel is. See NewSession's doc comment.
	session := auth.NewSession()
	session.CommandLevel = base.Name

	ctx := &command.AppContext{
		State:      &product.ProductState{},
		Logger:     logger,
		Levels:     levels,
		Audit:      audit,
		Translator: translator,
		// Seeded from the base level's own Name, PromptSuffix, and
		// Tree, never a hardcoded literal, so anything comparing
		// against the current root Command Level's name always matches
		// whatever tree_structure.yaml actually names the base level.
		Position: command.NewCommandLevelStack(base.Name, base.PromptSuffix, base.Tree),
		Session:  session,
		// Built once from config, here, and reused everywhere a
		// listing of more than one command name gets ordered: this
		// AppContext.ListOptions, the completer.New call below, and
		// the non-interactive "?" fallback further down runLoop, so
		// all three always agree. See command.ListOptions's own doc
		// comment.
		ListOptions: command.ListOptions{
			Alphabetical: config.AlphabeticalCommandOrder,
			MergeCommon:  config.MergeCommonCommands,
		},
		// PageLines starts nil, unset, matching real Cisco's own
		// terminal length, session only, never seeded from a
		// configuration file. DefaultPageLines, PagingEnabled, and
		// FilterMode's own startup value all come from config here,
		// the one place that already knows them, rather than reread
		// from config a second time anywhere paging.EffectivePageLines
		// or paging.Display are actually called. filterModeFromConfig
		// converts config.FilterMatchMode's plain YAML string into
		// package paging's own typed FilterMode here, in main.go,
		// keeping package config free of any dependency on package
		// paging, the same boundary this project already keeps
		// between package config and package command or auth.
		DefaultPageLines:    config.DefaultPageLines,
		PagingEnabled:       config.PagingEnabled,
		FilterMode:          filterModeFromConfig(config.FilterMatchMode),
		MaxFilterChainDepth: config.MaxFilterChainDepth,
	}

	if config.AuthRequired {
		users, err := auth.LoadUsers(config.UsersFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "failed to load users file:", err)
			os.Exit(1)
		}
		ctx.Users = users
		ctx.UsersFile = config.UsersFile
		ctx.TOTPIssuer = config.TOTPIssuer
		ctx.TOTPMaxAttempts = config.TOTPMaxAttempts
		ctx.PasswordPolicy = auth.PasswordPolicy{
			MinLength:           config.PasswordMinLength,
			RequireUppercase:    config.PasswordRequireUppercase,
			RequireNumbers:      config.PasswordRequireNumbers,
			RequireSpecialChars: config.PasswordRequireSpecialChars,
		}
		ctx.PasswordChangeMaxAttempts = config.PasswordChangeMaxAttempts
		warnPlaintextUserSecrets(logger, users)

		// ctx.AuthProvider is only built when EnableCLIAuthentication is
		// actually on, since it is only ever consulted by
		// auth.PromptLogin's own credential check, inside
		// establishSession below, and by cmd/core/cmd_password.go's password
		// change re-authentication step. That password change command
		// itself sets requires: password_change, so it is pruned out of
		// the tree entirely whenever EnableCLIAuthentication is off,
		// see the featureFlags pruning pass above. Leaving this nil in
		// the host-only case is therefore safe: nothing reachable in
		// that configuration ever calls ctx.AuthProvider.Authenticate.
		if config.EnableCLIAuthentication {
			provider, err := auth.NewAuthProvider(config.CLIAuthProvider, users)
			if err != nil {
				fmt.Fprintln(os.Stderr, "failed to construct authentication provider:", err)
				os.Exit(1)
			}
			ctx.AuthProvider = provider
		}

		session, err := establishSession(config, users, ctx.AuthProvider, translator, audit, os.Stdin, os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, "\n%", translator.T("auth.access_denied"))
			os.Exit(1)
		}
		session.CommandLevel = base.Name
		ctx.Session = session
	}

	// The SESSION START audit entry is written unconditionally here,
	// after ctx.Session has its final value either way, regardless of
	// whether config.AuthRequired is even on, so every connection to
	// routercli leaves a record of when it began, not only ones that
	// happen to run a login prompt. ForceLog, not Log, is used
	// deliberately, the same reasoning ForceLog's own doc comment
	// gives for "audit-log disable": a session's own start must never
	// be silently missing from the trail just because something
	// between here and whatever ends the session flips audit logging
	// off. See runLoop's own SESSION END logging for the other half of
	// this pair, and Session.HostUsername's own doc comment for why
	// that, and HostConnectedAt, are folded into this one entry's own
	// command text rather than becoming a new Auditor column.
	sessionStartText := "SESSION START"
	if ctx.Session.HostUsername != "" {
		sessionStartText = fmt.Sprintf("SESSION START (host account %q connected at %s)", ctx.Session.HostUsername, ctx.Session.HostConnectedAt.Format(time.RFC3339))
	}
	audit.ForceLog(ctx.Session.Username, sessionStartText, true)

	histFile := config.HistoryFile
	if histFile == "" {
		histFile = historyFilePath()
	}
	if err := mkdirForFile(histFile); err != nil {
		fmt.Fprintln(os.Stderr, "failed to prepare history file directory:", err)
		os.Exit(1)
	}

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          buildPrompt(ctx),
		AutoComplete:    completer.NoopCompleter{},
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
		HistoryFile:     histFile,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to start readline:", err)
		os.Exit(1)
	}
	defer rl.Close()

	treeListener := completer.New(ctx.Position, rl, logger, ctx.Translator, ctx.ListOptions)
	treeListener.SetPrompt(buildPrompt(ctx))
	rl.Config.Listener = treeListener

	// Captured independently of readline so the idle timeout path can
	// restore the terminal without going through rl.Close. See
	// runLoop's doc comment on why that matters. term.GetState fails
	// harmlessly, returning a nil state, when stdin is not a real
	// terminal, such as piped input or a test harness, which is fine
	// since there is nothing to restore in that case anyway.
	termFD := int(os.Stdin.Fd())
	origTermState, _ := term.GetState(termFD)

	runLoop(rl, treeListener, ctx, runLoopOptions{
		PreventEscape:      config.PreventEscape,
		SessionIdleTimeout: config.SessionIdleTimeout.AsDuration(),
		ElevationTimeout:   config.ElevationTimeout.AsDuration(),
		TerminalFD:         termFD,
		OrigTerminalState:  origTermState,
	})
} // main()

// establishSession - This function resolves a session's identity from
// whichever combination of config.SystemConfig.EnableHostAuthentication,
// EnableCLIAuthentication, and EnableTOTPAuthentication this
// deployment has turned on, returning the resulting *auth.Session or
// the first error encountered along the way. main calls this exactly
// once, only when config.AuthRequired is true, config.validateAuthSources
// already guarantees that AuthRequired being true means at least one
// of EnableHostAuthentication or EnableCLIAuthentication is also true,
// so this function can assume it always has at least one identity
// source to work with.
//
// When EnableHostAuthentication alone is on, the returned session
// carries the trusted operating system account on both Username and
// HostUsername, see auth.SessionFromHostIdentity, with no password
// prompt at all. If EnableTOTPAuthentication is also on, and the
// resolved account's own users.yaml entry, if any, has a TOTPSecret
// configured, a standalone TOTP challenge is required before the
// session is handed back, reusing the same auth.SecondFactorRequired
// and auth.VerifySecondFactor machinery auth.PromptLogin's own login
// flow uses, up to loginMaxAttempts tries. A resolved account with no
// matching users.yaml entry at all is treated the same way
// auth.PromptLogin treats one, see its own doc comment, a real
// identity simply carrying no second factor.
//
// When EnableCLIAuthentication is on, auth.PromptLogin drives the
// session the rest of the way, checked against provider, with
// totpEnabled passed straight through from EnableTOTPAuthentication.
// If EnableHostAuthentication also resolved a session first, the
// shared account case described on Session.HostUsername's own doc
// comment, that earlier session's HostUsername and HostConnectedAt
// are carried over onto the CLI-resolved session before it is
// returned, so the final session's own Username is whichever identity
// the CLI login actually resolved to, while still keeping a record of
// which OS account the connection itself arrived as.
//
// audit records a "LOGIN" entry for every attempt along the way,
// success or failure, the same as this project has always done for
// its CLI login path, now also covering the standalone host-plus-TOTP
// step up, which had no login attempt of its own to audit before this
// function existed.
//
// stdin and stdout are taken as explicit parameters, rather than this
// function simply reading the process-wide os.Stdin and os.Stdout
// itself, for the same reason auth.PromptLogin and its siblings
// already take an explicit io.Reader, io.Writer, and fd rather than
// assuming a global. main's own one real call site passes os.Stdin
// and os.Stdout, unchanged behavior. A test, in contrast, can hand
// this the slave end of a real pseudo terminal, github.com/creack/pty
// in this project's own test suite, so the masked password and TOTP
// code reads below have a genuine terminal device to operate against,
// the same requirement golang.org/x/term itself has, without needing
// to mutate the os.Stdin package variable for the duration of the
// test. See main_test.go's own pty helper for where this is put to
// use.
func establishSession(cfg config.SystemConfig, users auth.Users, provider auth.AuthProvider, translator *i18n.Translator, audit *auditlog.AuditLog, stdin, stdout *os.File) (*auth.Session, error) {
	var hostSession *auth.Session
	if cfg.EnableHostAuthentication {
		s, err := auth.SessionFromHostIdentity()
		if err != nil {
			return nil, err
		}
		hostSession = s
	}

	if cfg.EnableCLIAuthentication {
		// A real *auth.KeyedRateLimiter only gets constructed when
		// LoginAttemptWindow is actually configured, meaning nonzero.
		// LoadSystemConfig's own validate already guarantees that
		// LoginAttemptWindow and LoginLockoutDuration are either both
		// zero or both set, so checking just one here is sufficient. A
		// nil rate limiter tells PromptLogin to keep this project's
		// original flat maxAttempts behavior unchanged. See
		// PromptLogin's own doc comment.
		var loginRateLimiter *auth.KeyedRateLimiter
		if cfg.LoginAttemptWindow.AsDuration() > 0 {
			loginRateLimiter = auth.NewKeyedRateLimiter(cfg.LoginMaxAttempts, cfg.LoginAttemptWindow.AsDuration(), cfg.LoginLockoutDuration.AsDuration())
		}

		cliSession, err := auth.PromptLogin(stdin, stdout, int(stdin.Fd()), provider, users, cfg.EnableTOTPAuthentication, cfg.LoginMaxAttempts, loginRateLimiter, translator,
			func(username string) { audit.Log(username, "LOGIN", false) })
		if err != nil {
			return nil, err
		}
		if hostSession != nil {
			cliSession.HostUsername = hostSession.HostUsername
			cliSession.HostConnectedAt = hostSession.HostConnectedAt
		}
		audit.Log(cliSession.Username, "LOGIN", true)
		return cliSession, nil
	}

	// EnableCLIAuthentication is off, so hostSession is the whole
	// story, config.validateAuthSources already guarantees it is
	// non-nil here. The only thing left to decide is whether
	// EnableTOTPAuthentication requires a step up on top of it.
	if cfg.EnableTOTPAuthentication {
		u := users[hostSession.Username]
		if u == nil {
			// See auth.PromptLogin's own doc comment for the same
			// guard: a resolved identity with no matching users.yaml
			// entry simply has no second factor to check.
			u = &auth.User{Username: hostSession.Username}
		}
		if auth.SecondFactorRequired(u) {
			reader := bufio.NewReader(stdin)
			verified := false
			for attempt := 1; attempt <= cfg.LoginMaxAttempts; attempt++ {
				if auth.VerifySecondFactor(stdout, reader, int(stdin.Fd()), u, translator) {
					verified = true
					break
				}
				audit.Log(hostSession.Username, "LOGIN", false)
			}
			if !verified {
				return nil, auth.ErrLoginFailed
			}
		}
	}
	audit.Log(hostSession.Username, "LOGIN", true)
	return hostSession, nil
}

// defaultHostnamePrompt - This constant is what the prompt shows
// before "hostname" has ever been set, or after "no hostname" resets
// it, matching product.defaultHostname (cmd/product/cmd_hostname.go),
// an unexported constant this file cannot reach directly. It is kept
// as a separate constant here rather than exporting that one because
// this is purely a display fallback, not the same value the framework
// treats as canonical.
const defaultHostnamePrompt = "router"

// buildPrompt - This function builds the current prompt string. For
// any privileged set of commands, it adds a "#" to the prompt.
// Privileged here means either the session is away from the base
// Command Level, !ctx.Session.AtLevel(ctx.Levels.Base().Name), or the
// Command Level stack is deeper than the root, meaning the session is
// inside a nested Command Level at all.
//
// The hostname portion reads
// ctx.State.(*product.ProductState).Hostname live, so "hostname
// myrouter" immediately rewrites the prompt on the very next redraw.
// runLoop already calls this after every dispatch specifically so a
// command like this is reflected right away, not just remembered for
// next time. It falls back to defaultHostnamePrompt when Hostname is
// empty, which is exactly the state "no hostname" leaves it in, see
// cmd/product/cmd_hostname.go, so the fallback only ever shows up
// before a hostname has been set, or after it has been explicitly
// cleared, never as a silent default that masks a real configured
// value.
func buildPrompt(ctx *command.AppContext) string {
	frame := ctx.Position.Current()
	awayFromBase := false
	if ctx.Session != nil && ctx.Levels != nil {
		awayFromBase = !ctx.Session.AtLevel(ctx.Levels.Base().Name)
	}
	marker := "> "
	if awayFromBase || ctx.Position.Depth() > 1 {
		marker = "# "
	}

	host := defaultHostnamePrompt
	if state, ok := ctx.State.(*product.ProductState); ok && state.Hostname != "" {
		host = state.Hostname
	}

	return host + frame.PromptSuffix + marker
}

// runLoop - This function is the actual read, resolve, validate, and
// dispatch loop, split out from main so it can be exercised without a
// real terminal attached. A command's own PasswordHash, see
// Command.PasswordHash, is checked before ValidateArgs, so a session
// that does not know the password is refused before it ever learns
// whether its arguments would have been acceptable. This avoids
// leaking information about a password-gated command's argument
// shape to whoever just failed the password prompt.
//
// Resolution happens against ctx.Position.Current().Tree, not a fixed
// tree. This is what makes the whole mode system work, entering
// "configure terminal" pushes a new frame onto ctx.Position, and the
// very next loop iteration resolves against that frame's tree
// automatically, with no other change needed here. The prompt is
// rebuilt and pushed to readline with rl.SetPrompt() after every
// dispatch, since any command might have changed the mode or
// elevation state as a side effect. Checking whether the prompt
// actually changed first would save a few redraws, but is not worth
// the complexity given how infrequently commands actually change
// mode.
//
// Program termination is driven by command.ErrQuit, not by matching
// the name "exit". See cmd/core/cmd_mode_control.go's doc comment for why
// that matters now that "exit" behaves differently depending on mode
// depth.
//
// opts.PreventEscape controls what Ctrl-C and Ctrl-D do here, as the
// other half of preventEscape, defined below in this same file.
// preventEscape stops the OS from delivering SIGINT, SIGTSTP,
// SIGQUIT, or SIGTERM at all, but Ctrl-C and Ctrl-D inside an active
// line read are handled by chzyer/readline as ordinary keystrokes,
// ErrInterrupt or io.EOF return values, not as real signals, so
// closing that second door is this loop's job, not preventEscape's.
// When false, the default, the loop keeps ordinary CLI ergonomics.
// Ctrl-C on an empty line exits, and Ctrl-D, EOF, exits. When true,
// neither does anything observable. The only way out is the "exit"
// command itself.
//
// opts.SessionIdleTimeout, if nonzero, ends the whole session,
// matching Cisco's line exec-timeout, if no line of input arrives
// within that duration. chzyer/readline has no built-in read
// deadline, so this is implemented by running rl.Readline() in its
// own goroutine and racing it against a timer with select. If the
// timer wins, the abandoned goroutine is left blocked on the read
// forever, since there is no portable way to cancel a blocking
// terminal read out from under it. This is exactly why the timeout
// path does not simply return and let main's "defer rl.Close()" run.
// Close tries to acquire an internal lock the abandoned goroutine's
// Readline call is still holding, and deadlocks waiting for a read
// that will never return. Instead, the idle timeout path restores the
// terminal directly through opts.OrigTerminalState and
// opts.TerminalFD, captured in main before readline ever put the
// terminal in raw mode, independent of readline's own Close path, and
// calls os.Exit, which is guaranteed to terminate regardless of what
// the orphaned goroutine is doing. The cost is skipping normal
// deferred cleanup, which is acceptable here, since the audit log
// writes are unbuffered os.File.WriteString calls that already
// reached the kernel, so nothing is lost by skipping AuditLog.Close.
//
// opts.ElevationTimeout, if nonzero, is checked once at the top of
// every iteration, not through a background timer. If the session has
// been elevated for longer than the timeout, it is demoted back to
// unprivileged right there, before the newly read line is even
// resolved. Checking lazily like this, only when the user actually
// does something, is sufficient, since there is nothing useful to do
// about an idle elevated session before the user interacts again
// anyway, and it avoids a second background timer racing the idle
// timeout one.
type runLoopOptions struct {
	PreventEscape      bool
	SessionIdleTimeout time.Duration
	ElevationTimeout   time.Duration
	// TerminalFD and OrigTerminalState are used only by the idle
	// timeout path, to restore the terminal without going through
	// rl.Close(). See this function's doc comment for why that
	// distinction matters. OrigTerminalState may be nil, for example
	// when stdin is not a real terminal, as in a test harness, and the
	// restore is skipped in that case, since there is nothing
	// meaningful to restore.
	TerminalFD        int
	OrigTerminalState *term.State
}

func runLoop(rl *readline.Instance, treeListener *completer.TreeListener, ctx *command.AppContext, opts runLoopOptions) {
	for {
		var line string
		var err error
		if opts.SessionIdleTimeout > 0 {
			type readResult struct {
				line string
				err  error
			}
			resultCh := make(chan readResult, 1)
			go func() {
				l, e := rl.Readline()
				resultCh <- readResult{l, e}
			}()
			select {
			case res := <-resultCh:
				line, err = res.line, res.err
			case <-time.After(opts.SessionIdleTimeout):
				fmt.Println(ctx.Translator.T("runloop.idle_timeout"))
				// os.Exit below terminates the process immediately, so
				// SESSION END is logged right here rather than after
				// this select, the same reason this branch already
				// restores the terminal directly instead of relying on
				// main's own deferred cleanup. See this function's own
				// doc comment on opts.SessionIdleTimeout for why.
				logSessionEnd(ctx)
				if opts.OrigTerminalState != nil {
					_ = term.Restore(opts.TerminalFD, opts.OrigTerminalState)
				}
				os.Exit(0)
			}
		} else {
			line, err = rl.Readline()
		}

		if err == readline.ErrInterrupt {
			if !opts.PreventEscape && len(line) == 0 {
				break
			}
			continue
		} else if err == io.EOF {
			if opts.PreventEscape {
				fmt.Println(ctx.Translator.T("runloop.eof_blocked"))
				continue
			}
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Checked here, after the blocking read returns rather than
		// before it starts, since the elapsed time that matters is how
		// long it has been since elevation as of right now, about to
		// act on this input, not as of when the wait for input began.
		// Checking beforehand would use a stale snapshot from before
		// the possibly long wait and demote one command too late.
		if opts.ElevationTimeout > 0 && ctx.Session != nil && ctx.Levels != nil && !ctx.Session.AtLevel(ctx.Levels.Base().Name) {
			if time.Since(ctx.Session.CommandLevelEnteredAt) > opts.ElevationTimeout {
				base := ctx.Levels.Base()
				ctx.Session.CommandLevel = base.Name
				ctx.Position.SetRootTree(base.Name, base.PromptSuffix, base.Tree)
				fmt.Println(ctx.Translator.T("runloop.elevation_expired"))
				rl.SetPrompt(buildPrompt(ctx))
				treeListener.SetPrompt(buildPrompt(ctx))
			}
		}

		tokens, terr := tokenize.Tokenize(line)
		if terr != nil {
			fmt.Printf("%% %v\n", terr)
			continue
		}

		// cmdTokens is what actually gets resolved against the
		// command tree. segments is every "| ..." stage that
		// followed it, still raw and unparsed at this point. A line
		// with no "|" at all leaves cmdTokens equal to tokens and
		// segments nil, see paging.SplitPipeline. stages is checked
		// against ctx.MaxFilterChainDepth right here, before
		// resolution even runs, so a line asking for too many
		// filters is refused with one clear error rather than
		// silently truncated or evaluated anyway.
		cmdTokens, segments := paging.SplitPipeline(tokens)
		stages, perr := paging.ParseStages(segments, ctx.MaxFilterChainDepth)
		if perr != nil {
			fmt.Printf("%% %v\n", perr)
			continue
		}

		// Defensive, non-interactive "?" fallback. Normally,
		// readline.Listener's key == '?' branch, completer.go's
		// handleHelp, already intercepts '?' before it ever reaches
		// here, whether stdin is a real terminal or a plain pipe,
		// since handleHelp strips the '?' before the line is ever
		// submitted. This block is retained as defense for any other
		// readline version or environment where a non-tty stdin never
		// triggers Listener callbacks at all. It costs nothing to
		// keep, and means a literal trailing "?" token still gets a
		// sensible answer instead of "% Invalid input" if that
		// assumption ever changes. Checked against cmdTokens, never
		// against a filter segment, since "?" only ever means
		// something as part of resolving a command, not as part of a
		// "| include" pattern.
		if len(cmdTokens) > 0 && cmdTokens[len(cmdTokens)-1] == "?" {
			help := command.HelpForPath(ctx.Position.Current().Tree, cmdTokens[:len(cmdTokens)-1], ctx.Translator, ctx.ListOptions)
			if help != "" {
				fmt.Print(help)
			}
			continue
		}

		res := command.Resolve(ctx.Position.Current().Tree, cmdTokens)
		username := ""
		if ctx.Session != nil {
			username = ctx.Session.Username
		}

		switch {
		case len(res.Ambiguous) > 0:
			// Lists the actual candidates, matching what a real HP
			// ProCurve or Cisco device prints for "sh r" followed by
			// Enter: bare names, space separated, with no "% ..."
			// wrapper message. This mirrors the completer's OnChange
			// Tab list output deliberately, the same situation with a
			// different key, rather than duplicating a differently
			// worded message specific to runLoop.
			for _, candidate := range res.Ambiguous {
				fmt.Println(" " + candidate)
			}
		case res.Command == nil:
			// Nothing at the top level matched at all. res.Args holds
			// the unmatched token onward, see command.Resolve. The
			// first one is the actual offending word. Anything after
			// it was never even looked at, so it is not part of the
			// error.
			fmt.Println("%", ctx.Translator.T("runloop.invalid_input", firstBadToken(res.Args)))
			ctx.Audit.Log(username, line, false)
		case res.Command.RunFunc == nil && len(res.Args) > 0:
			// A real command was found, for example "show", but what
			// followed it does not match any of its children either,
			// for example "show fan", where "fan" is not a show
			// subcommand. "incomplete" is reserved for the case right
			// below, where the user stopped short with nothing left
			// over at all, a bare "show".
			fmt.Println("%", ctx.Translator.T("runloop.invalid_input", firstBadToken(res.Args)))
			ctx.Audit.Log(username, line, false)
		case res.Command.RunFunc == nil:
			// A real container command, for example "show" on its own,
			// or "configure" without "terminal", with nothing left
			// over. The user needs to keep typing, not correct a typo.
			fmt.Println("%", ctx.Translator.T("runloop.incomplete_command", line))
		case res.Negated && !res.Command.Negatable:
			fmt.Println("%", ctx.Translator.T("runloop.not_negatable", strings.Join(res.FullName, " ")))
			ctx.Audit.Log(username, line, false)
		default:
			// Checked before anything else in this branch. A
			// filter was typed, len(stages) > 0, but this command
			// was never marked Pageable, see
			// command.Command.Pageable's own doc comment, so there
			// is nothing here to capture, filter, or page. This is a
			// real error, not a silent no-op, the same "fail loudly"
			// convention this project already applies to every other
			// malformed request.
			if len(stages) > 0 && !res.Command.Pageable {
				fmt.Println("%", ctx.Translator.T("runloop.not_pageable", strings.Join(res.FullName, " ")))
				ctx.Audit.Log(username, line, false)
				continue
			}
			// Checked next, before ValidateArgs. See this function's
			// own doc comment for why. It avoids leaking a password-
			// gated command's argument shape requirements to a
			// session that has not supplied the password yet. This
			// is independent of Command Level entirely. A command can
			// carry its own PasswordHash regardless of which level
			// the session is currently in. It is reprompted on every
			// invocation, not cached for the session the way a
			// level's own PasswordHash is, since this is deliberately
			// a one-off gate on this one action, not something a
			// session settles into. See Command.PasswordHash's doc
			// comment for how this differs from
			// CommandLevel.PasswordHash.
			if res.Command.PasswordHash != "" {
				// Checked before ever prompting, the same reasoning as
				// command.EnterCommandLevel's own rate limit check: a
				// locked out session should not be invited to try
				// again at all.
				if ok, retryAfter := res.Command.PasswordRateLimiter.Allow(); !ok {
					fmt.Println("%", ctx.Translator.T("auth.too_many_attempts", auth.RoundForDisplay(retryAfter)))
					ctx.Audit.Log(username, line, false)
					continue
				}
				password, perr := auth.PromptSecret(os.Stdout, int(os.Stdin.Fd()), ctx.Translator)
				if perr != nil || !auth.VerifyPassword(res.Command.PasswordHash, password) {
					res.Command.PasswordRateLimiter.RecordFailure()
					fmt.Println("%", ctx.Translator.T("commandlevel.access_denied"))
					ctx.Audit.Log(username, line, false)
					continue
				}
				res.Command.PasswordRateLimiter.RecordSuccess()
			}
			// ValidateArgs is skipped entirely for a negated command.
			// See command.ValidateArgs's doc comment for why "no X"
			// often has a genuinely different valid argument shape
			// than "X" itself, for example "no description" takes zero
			// args to clear a value that "description <text>" requires
			// exactly one to set. A negatable handler is expected to
			// check len(args) itself if it cares.
			if !res.Negated {
				if err := command.ValidateArgs(res.Command, res.Args); err != nil {
					fmt.Printf("%% %v\n", err)
					ctx.Audit.Log(username, line, false)
					continue
				}
			}
			// Snapshot whether this would be logged before running the
			// command, not only after. A command such as "audit-log
			// disable" or "audit-log enable" changes the audit enabled
			// state as its own side effect. Checking only the state
			// after RunFunc would mean "disable" could never log its own
			// invocation, since the state was just turned off, and
			// checking only the state before RunFunc would mean "enable"
			// could never log its own invocation, since the state had
			// not turned on yet. Logging if either side was loggable
			// makes both transitions visible in the trail, exactly the
			// pair of events an audit log exists to capture. See
			// auditlog.AuditLog's doc comments for the ForceLog
			// reasoning.
			wasLoggable := ctx.Audit.WouldLog()
			// ctx.Negated is only meaningful for the duration of this
			// one call. See AppContext.Negated's doc comment. It is
			// always reset after, so a later non-negated command in
			// the same session never accidentally inherits it.
			ctx.Negated = res.Negated
			var runErr error
			if res.Command.Pageable {
				// See dispatchPageable's own doc comment for the
				// capture, filter, and page sequence this runs
				// instead of calling RunFunc directly. A Pageable
				// command is exactly the kind this is safe for, one
				// whose entire output is produced up front with no
				// interactive prompt of its own reading from the
				// terminal partway through, see
				// Command.Pageable's own doc comment.
				runErr = dispatchPageable(ctx, res, stages, opts.TerminalFD)
			} else {
				runErr = res.Command.RunFunc(ctx, res.Args)
			}
			ctx.Negated = false
			isLoggableNow := ctx.Audit.WouldLog()
			if wasLoggable || isLoggableNow {
				ctx.Audit.ForceLog(username, line, runErr == nil)
			}
			if runErr == command.ErrQuit {
				logSessionEnd(ctx)
				return
			}
			if runErr != nil {
				fmt.Printf("%% %v\n", runErr)
			}
			rl.SetPrompt(buildPrompt(ctx))
			treeListener.SetPrompt(buildPrompt(ctx))
		}
	}
	// Reached by the two ordinary "break" exits above, Ctrl-C on an
	// empty line and Ctrl-D/EOF, neither of which goes through
	// command.ErrQuit. See this function's own doc comment on
	// opts.PreventEscape. The command.ErrQuit path and the idle
	// timeout path each log their own SESSION END before leaving this
	// function through return or os.Exit respectively, since neither
	// of those two ever reaches this line.
	logSessionEnd(ctx)
}

// logSessionEnd - This function writes the mandatory SESSION END audit
// entry, called from every real exit path out of runLoop, normal
// return, command.ErrQuit, and the idle timeout's own os.Exit branch,
// so a session that started with a SESSION START entry, see main's own
// logging right after establishSession returns, always gets a matching
// end entry too, regardless of which door it left through. ForceLog is
// used for the same reason main's own SESSION START call uses it, see
// that call site's own doc comment.
func logSessionEnd(ctx *command.AppContext) {
	username := ""
	if ctx.Session != nil {
		username = ctx.Session.Username
	}
	ctx.Audit.ForceLog(username, "SESSION END", true)
}

// runHashPasswordUtility - This function implements the
// --hashpassword flag, an administrator facing utility to generate a
// bcrypt hash for etc/users.yaml without needing to write a throwaway
// Go program to call auth.HashPassword directly. It reads the
// password once, with echo disabled, and prints the "$6$..." hash to
// stdout so it can be redirected or copied straight into the YAML
// file.
//
// stdin is taken as an explicit parameter rather than this function
// reading the process-wide os.Stdin itself, the same reasoning
// establishSession's own doc comment gives, so a test can hand this
// the slave end of a real pseudo terminal instead of a real password
// prompt requiring an actual interactive session.
func runHashPasswordUtility(stdin *os.File) {
	fmt.Fprint(os.Stderr, "Password: ")
	pwBytes, err := term.ReadPassword(int(stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error reading password:", err)
		os.Exit(1)
	}
	hash, err := auth.HashPassword(string(pwBytes))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error hashing password:", err)
		os.Exit(1)
	}
	fmt.Println(hash)
}

// runTOTPSetupUtility - This function implements the --mfa flag,
// which enrolls a user for TOTP. It generates a fresh secret and
// shows it both as a scannable QR code, an otpauth:// URI rendered
// directly in the terminal with no image file and no GUI needed, and
// as plain text for manual entry, since not every authenticator app
// workflow makes scanning convenient and some users will always
// prefer typing a secret over pointing a camera at a screen.
//
// This does not stop at just showing the secret. It then prompts for
// a live code and verifies it before printing the users.yaml line to
// add. Enrollment that looks like it worked but has a misspelled or
// misread secret is a real, common failure mode for TOTP setup in
// practice. Confirming with a real code from the app catches that
// immediately, while the terminal is still open, instead of leaving
// an administrator to discover it days later when the user actually
// tries to log in.
//
// stdin is taken as an explicit parameter, rather than this function
// reading the process-wide os.Stdin itself, so a test can supply the
// confirmation code through an ordinary io.Reader. Unlike
// establishSession and runHashPasswordUtility, this one reads its
// confirmation code with plain, echoed input, see the reader.ReadString
// call below, not through term.ReadPassword, so no real terminal
// device is actually required here, an in-memory strings.Reader is
// enough.
func runTOTPSetupUtility(configPath, username string, stdin io.Reader) {
	config, err := config.LoadSystemConfig(configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load configuration:", err)
		os.Exit(1)
	}

	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error generating TOTP secret:", err)
		os.Exit(1)
	}

	uri := auth.TOTPProvisioningURI(config.TOTPIssuer, username, secret)
	qr, err := qrcode.New(uri, qrcode.Medium)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error generating QR code:", err)
		os.Exit(1)
	}

	fmt.Printf("TOTP enrollment for %q (issuer %q)\n\n", username, config.TOTPIssuer)
	fmt.Println("Scan this QR code with your authenticator app:")
	fmt.Println()
	fmt.Println(qr.ToSmallString(false))
	fmt.Println("...or, if you cannot scan it, enter this secret manually:")
	fmt.Println()
	fmt.Println("  " + auth.FormatTOTPSecretForDisplay(secret))
	fmt.Println()
	fmt.Print("Now enter the 6-digit code your app is showing, to confirm: ")

	reader := bufio.NewReader(stdin)
	codeLine, _ := reader.ReadString('\n')
	code := strings.TrimSpace(codeLine)

	if !auth.VerifyTOTPCode(secret, code, time.Now()) {
		fmt.Fprintln(os.Stderr, "\nThat code did not verify - the secret was NOT confirmed working.")
		fmt.Fprintln(os.Stderr, "Nothing has been saved. Run --mfa again and try scanning/typing more carefully.")
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("Confirmed. Add this line to %q's entry in %s:\n\n", username, config.UsersFile)
	fmt.Printf("  totp_secret: %q\n", secret)
}

// historyFilePath - This function returns a per-user history file
// path rather than a hardcoded one. It is used as the fallback when
// the configuration file does not set HistoryFile explicitly. It is
// deliberately not under logs/ by default, since a personal, shell
// style history file more naturally belongs in the user's home
// directory than in a project-relative logs folder that might not
// even be writable by them.
func historyFilePath() string {
	if u, err := user.Current(); err == nil && u.HomeDir != "" {
		return filepath.Join(u.HomeDir, ".routercli_history")
	}
	return ".routercli_history"
}

// mkdirForFile - This function ensures the directory containing path
// exists, so that a fresh checkout with an empty, or entirely
// missing, logs/ directory does not fail the first time something
// tries to write history or audit output there.
func mkdirForFile(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0750)
}

// attachPasswordRateLimiters - This function walks every loaded
// Command Level's tree and attaches a fresh auth.RateLimiter to each
// command that has a PasswordHash set. See
// Command.PasswordRateLimiter's own doc comment for what it is for.
//
// visited tracks *command.Command pointers, not names, since the same
// command can appear in more than one level's merged tree at once. A
// command inherited through InheritParent, see
// command.LoadTreeStructure, is the identical pointer in every level
// that inherits it, not a copy. A walk that did not track this would
// redo the work of attaching a limiter, and walking that command's own
// subcommands, every time it re-encountered that shared command
// through a different level's tree, and would recurse forever on a
// hand-built, self-referencing tree. visited ensures exactly one
// limiter per unique command, regardless of how many levels can
// currently reach it.
func attachPasswordRateLimiters(levels *command.TreeStructure, maxAttempts int, window, lockout time.Duration) {
	visited := make(map[*command.Command]bool)
	var walk func(tree map[string]*command.Command)
	walk = func(tree map[string]*command.Command) {
		for _, cmd := range tree {
			if visited[cmd] {
				continue
			}
			visited[cmd] = true
			if cmd.PasswordHash != "" {
				cmd.PasswordRateLimiter = auth.NewRateLimiter(maxAttempts, window, lockout)
			}
			walk(cmd.Subcommands)
		}
	}
	for _, level := range levels.Order {
		walk(level.Tree)
	}
}

// warnPlaintextUserSecrets - This function logs a warning, at the
// "warn" level, see LogLevel in config/config.go, for every user in
// users whose PasswordHash is stored in the plaintext, "$0$...", form
// rather than a real bcrypt hash. See auth.IsPlaintextHash and
// cryptIDPlaintext's own doc comment in auth/auth.go for why that
// form exists at all, local development and testing convenience, and
// why it must never reach a real deployment. This is purely
// informational. It never refuses to start or blocks login, since
// plaintext storage is deliberately still a supported, working mode
// for exactly the testing use case it exists for. It just makes sure
// nobody accidentally ships that mode without noticing, by surfacing
// it somewhere they will actually see it, the configured log, rather
// than something they would have to know to go check auth/auth.go's
// own source comment to learn about.
func warnPlaintextUserSecrets(logger *log.Logger, users auth.Users) {
	for _, name := range sortedUserNames(users) {
		u := users[name]
		if auth.IsPlaintextHash(u.PasswordHash) {
			logger.Warnln("WARN: user", name, "has a plaintext (non-hashed) password set - this must never be used in a real deployment")
		}
	}
}

// sortedUserNames - This function returns every username in users,
// sorted, so warnPlaintextUserSecrets's output is stable between
// runs, since Go map iteration order is randomized, rather than
// shuffling which warning prints first every time the same broken
// users.yaml is loaded.
func sortedUserNames(users auth.Users) []string {
	names := make([]string, 0, len(users))
	for name := range users {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// warnPlaintextLevelSecrets - This function is the Tree Structure
// equivalent of warnPlaintextUserSecrets above. It warns, at "warn"
// level, about any Command Level whose PasswordHash is stored in
// plaintext form. This can only happen if
// var/tree/tree_structure.yaml itself was hand-edited with a raw
// "$0$..." value in password_hash:, since "password manager
// <secret>", cmd/core/cmd_password_manager.go, always calls
// auth.HashPassword, which never produces the plaintext form. So this
// path only catches a manifest that was authored that way directly,
// the same testing convenience reasoning as the login side.
func warnPlaintextLevelSecrets(logger *log.Logger, levels *command.TreeStructure) {
	for _, level := range levels.Order {
		if auth.IsPlaintextHash(level.PasswordHash) {
			logger.Warnln("WARN: Command Level", level.Name, "has a plaintext (non-hashed) password_hash set - this must never be used in a real deployment")
		}
	}
}

// dispatchPageable - This function runs a Pageable command's own
// RunFunc through paging.CaptureOutput, then paging.ApplyFilters, then
// paging.Display, in that order, in place of calling RunFunc directly
// the way runLoop's default branch does for every other command. See
// Command.Pageable's own doc comment for which commands this is safe
// for at all.
//
// A failure inside paging.CaptureOutput itself, essentially
// impossible in practice, is returned directly, since nothing was
// captured at all and there is nothing left to filter or page. A
// failure inside paging.ApplyFilters, an invalid regular expression
// typed as a filter pattern for instance, only possible when
// ctx.FilterMode is FilterModeRegex, is printed directly here rather
// than returned, since it describes the filter itself, not the
// command that produced the output being filtered; RunFunc's own
// runErr is still returned afterward either way, so a command that
// both printed something and returned an error still reports that
// error through this function's caller exactly as it would have
// running unpaged.
func dispatchPageable(ctx *command.AppContext, res command.ResolveResult, stages []paging.FilterStage, fd int) error {
	var runErr error
	lines, cerr := paging.CaptureOutput(func() { runErr = res.Command.RunFunc(ctx, res.Args) })
	if cerr != nil {
		return cerr
	}

	filtered, ferr := paging.ApplyFilters(lines, stages, ctx.FilterMode)
	if ferr != nil {
		fmt.Printf("%% %v\n", ferr)
		return runErr
	}

	pageLines := paging.EffectivePageLines(fd, ctx.PageLines, ctx.DefaultPageLines)
	if derr := paging.Display(os.Stdout, fd, ctx.Translator, filtered, pageLines, ctx.PagingEnabled); derr != nil {
		fmt.Printf("%% %v\n", derr)
	}
	return runErr
}

// filterModeFromConfig - This function converts
// config.SystemConfig.FilterMatchMode's plain YAML string into
// package paging's own typed FilterMode, the one place this project
// crosses that boundary, so package config never needs to import
// package paging just to describe its own default. config.LoadSystemConfig's
// own validate already rejects any value other than "substring" or
// "regex" before this ever runs, so the switch below has no error
// path of its own; an unrecognized value cannot reach here from a
// real, loaded configuration.
func filterModeFromConfig(mode string) paging.FilterMode {
	if mode == "regex" {
		return paging.FilterModeRegex
	}
	return paging.FilterModeSubstring
}

// firstBadToken - This function returns the first entry of a
// ResolveResult's Args slice, or an empty string if it is empty. It
// is extracted so the runLoop switch above does not repeat the same
// guard twice. Both res.Command == nil and res.Command.RunFunc == nil with
// leftover Args need the one bad word, not the whole leftover tail.
// "show fan extra junk" should report "Invalid input: fan", not
// "Invalid input: fan extra junk", matching how a real device points
// at the first word it could not place and stops there without
// trying to interpret what follows it.
func firstBadToken(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// --------------------------------------------------
//
// Private functions
//
// --------------------------------------------------

// preventEscape - This function catches various signals to prevent a
// user from escaping the CLI into a standard command shell through
// things like Ctrl-C, Ctrl-Z suspend, Ctrl-\, or a plain kill on the
// process. The only sanctioned way out then becomes the "exit"
// command itself. This is an optional configuration setting, and
// should only be used in production systems.
//
// Deliberately not covered here are SIGABRT, SIGSEGV, and SIGILL,
// since these mean something to the Go runtime itself, which uses
// SIGSEGV and SIGILL for parts of its own crash handling, such as
// translating certain faults into a recoverable panic on some
// platforms. Ignoring these would not actually provide any real
// escape prevention, since they are synchronous processor traps
// triggered by a bug, not a user-initiated escape action, so there is
// nothing for a hostile user to exploit here the way there is with
// SIGINT, SIGTSTP, SIGQUIT, or SIGTERM. Overriding their disposition
// in a Go program is explicitly discouraged by the runtime
// documentation, and could make a genuine crash harder to diagnose
// instead of preventing anything. Blocking these anyway would need a
// signal.Notify handler and a recover strategy, not a blanket Ignore,
// which is a different and riskier feature.
//
// No process on Unix, in any language, can block SIGKILL or SIGSTOP.
// Those two are enforced unconditionally by the kernel, so
// "kill -9 <pid>" or "kill -STOP <pid>" always works regardless of
// this function.
func preventEscape() {
	signal.Ignore(
		syscall.SIGINT,  // Ctrl-C
		syscall.SIGTSTP, // Ctrl-Z (suspend to background)
		syscall.SIGQUIT, // Ctrl-\
		syscall.SIGTERM, // plain `kill <pid>`
	)
}

// processCommandLineFlags - This function processes the command line
// flags and prints version or help information as needed.
func processCommandLineFlags() (configFile string, checkConfig bool) {
	defaultConfigFilename := "etc/routercli.yaml"
	sOptConfigFilename := getopt.StringLong("config", 'c', defaultConfigFilename, "The main configuration file", "string")
	bOptHashPassword := getopt.BoolLong("hashpassword", 0, "Create a bcrypt hash suitable for etc/users.yaml")
	sOptTOTPSetup := getopt.StringLong("mfa", 'm', "", "Generate a TOTP config for <username>")
	bOptCheckConfig := getopt.BoolLong("check-config", 0, "Load and verify the tree structure and Command Levels, then exit")
	bOptHelp := getopt.BoolLong("help", 0, "Help")
	bOptVer := getopt.BoolLong("version", 0, "Version")

	getopt.HelpColumn = 35
	getopt.DisplayWidth = 120
	getopt.SetParameters("")
	getopt.Parse()

	// If the version flag was given, print the version information
	// and exit.
	if *bOptVer {
		printOutputHeader()
		os.Exit(0)
	}

	// If the help flag was given, print the help information and
	// exit.
	if *bOptHelp {
		printOutputHeader()
		getopt.Usage()
		os.Exit(0)
	}

	// If the hashpassword flag was given, run
	// runHashPasswordUtility and exit.
	if *bOptHashPassword {
		runHashPasswordUtility(os.Stdin)
		os.Exit(0)
	}

	// If the mfa flag was given, run runTOTPSetupUtility and exit.
	if *sOptTOTPSetup != "" {
		runTOTPSetupUtility(*sOptConfigFilename, *sOptTOTPSetup, os.Stdin)
		os.Exit(0)
	}

	return *sOptConfigFilename, *bOptCheckConfig
}

// printOutputHeader - This function prints a header for console
// output.
func printOutputHeader() {
	fmt.Println("")
	fmt.Println("Router CLI")
	fmt.Println("Copyright: Bret Jordan")
	fmt.Println("Version:", Version)
	if Build != "" {
		fmt.Println("Build:", Build)
	}
	fmt.Println("")
}
