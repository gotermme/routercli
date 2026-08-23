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
	"github.com/gotermme/routercli/cmd"
	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/completer"
	"github.com/gotermme/routercli/config"
	"github.com/gotermme/routercli/i18n"
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
	// same way with zero code changes, beyond the one small
	// cmd/cmd_*.go file every level's enter and exit command always
	// needs. See command/treestructure.go's own top of file comment.

	// Rate limiters are wired in here, right after loading rather than
	// during it, deliberately. LoadTreeStructure's job is building a
	// correct TreeStructure from the manifest. Nothing about policy,
	// such as how many attempts before a lockout, belongs in that
	// function, the same separation of concerns VerifyCommandLevels, a
	// few lines below, already follows. Every level gets a RateLimiter
	// unconditionally, even one with no PasswordHash set right now,
	// since "password manager", cmd/cmd_password_manager.go, can set
	// one at any time while the program is running, and the limiter
	// needs to already be there, ready, the moment that happens.
	// Per-command limiters are narrower, only commands that already
	// have a PasswordHash at load time get one, since, unlike a
	// Command Level's secret, nothing in this project currently sets
	// Command.PasswordHash after the tree is loaded.
	for _, level := range levels.Order {
		level.RateLimiter = auth.NewRateLimiter(config.CommandLevelMaxAttempts, config.CommandLevelAttemptWindow.AsDuration(), config.CommandLevelLockoutDuration.AsDuration())
	}
	attachPasswordRateLimiters(levels, config.CommandPasswordMaxAttempts, config.CommandPasswordAttemptWindow.AsDuration(), config.CommandPasswordLockoutDuration.AsDuration())

	// VerifyCommandLevels is a separate pass from loading, see its own
	// doc comment for why. It confirms every level's declared
	// enter_command and exit_command actually correspond to a real,
	// registered command, meaning someone really did write the
	// cmd/cmd_*.go file the manifest expects, catching a typo or a
	// forgotten file right here instead of the first time a user types
	// the command and gets "unknown command". This runs
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
		State:      &cmd.ExampleState{},
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

		// A real *auth.KeyedRateLimiter only gets constructed when
		// LoginAttemptWindow is actually configured, meaning nonzero.
		// LoadSystemConfig's own validate already guarantees that
		// LoginAttemptWindow and LoginLockoutDuration are either both
		// zero or both set, so checking just one here is sufficient. A
		// nil rate limiter tells PromptLogin to keep this project's
		// original flat maxAttempts behavior unchanged. See
		// PromptLogin's own doc comment.
		var loginRateLimiter *auth.KeyedRateLimiter
		if config.LoginAttemptWindow.AsDuration() > 0 {
			loginRateLimiter = auth.NewKeyedRateLimiter(config.LoginMaxAttempts, config.LoginAttemptWindow.AsDuration(), config.LoginLockoutDuration.AsDuration())
		}

		session, err := auth.PromptLogin(os.Stdin, os.Stdout, int(os.Stdin.Fd()), users, config.LoginMaxAttempts, loginRateLimiter, translator,
			func(username string) { audit.Log(username, "LOGIN", false) })
		if err != nil {
			fmt.Fprintln(os.Stderr, "\n%", translator.T("auth.access_denied"))
			os.Exit(1)
		}
		session.CommandLevel = base.Name
		ctx.Session = session
		audit.Log(session.Username, "LOGIN", true)
	}

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

	treeListener := completer.New(ctx.Position, rl, logger, ctx.Translator)
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

// defaultHostnamePrompt - This constant is what the prompt shows
// before "hostname" has ever been set, or after "no hostname" resets
// it, matching cmd.defaultHostname (cmd/cmd_hostname.go). It is kept
// as a separate constant here rather than importing that one because
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
// ctx.State.(*cmd.ExampleState).Hostname live, so "hostname myrouter"
// immediately rewrites the prompt on the very next redraw. runLoop
// already calls this after every dispatch specifically so a command
// like this is reflected right away, not just remembered for next
// time. It falls back to defaultHostnamePrompt when Hostname is
// empty, which is exactly the state "no hostname" leaves it in, see
// cmd/cmd_hostname.go, so the fallback only ever shows up before a
// hostname has been set, or after it has been explicitly cleared,
// never as a silent default that masks a real configured value.
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
	if state, ok := ctx.State.(*cmd.ExampleState); ok && state.Hostname != "" {
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
// the name "exit". See cmd/cmd_mode_control.go's doc comment for why
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
		// assumption ever changes.
		if len(tokens) > 0 && tokens[len(tokens)-1] == "?" {
			help := command.HelpForPath(ctx.Position.Current().Tree, tokens[:len(tokens)-1], ctx.Translator)
			if help != "" {
				fmt.Print(help)
			}
			continue
		}

		res := command.Resolve(ctx.Position.Current().Tree, tokens)
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
			// Checked first, before ValidateArgs. See this function's
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
			runErr := res.Command.RunFunc(ctx, res.Args)
			ctx.Negated = false
			isLoggableNow := ctx.Audit.WouldLog()
			if wasLoggable || isLoggableNow {
				ctx.Audit.ForceLog(username, line, runErr == nil)
			}
			if runErr == command.ErrQuit {
				return
			}
			if runErr != nil {
				fmt.Printf("%% %v\n", runErr)
			}
			rl.SetPrompt(buildPrompt(ctx))
			treeListener.SetPrompt(buildPrompt(ctx))
		}
	}
}

// runHashPasswordUtility - This function implements the
// --hashpassword flag, an administrator facing utility to generate a
// bcrypt hash for etc/users.yaml without needing to write a throwaway
// Go program to call auth.HashPassword directly. It reads the
// password once, with echo disabled, and prints the "$6$..." hash to
// stdout so it can be redirected or copied straight into the YAML
// file.
func runHashPasswordUtility() {
	fmt.Fprint(os.Stderr, "Password: ")
	pwBytes, err := term.ReadPassword(int(os.Stdin.Fd()))
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
func runTOTPSetupUtility(configPath, username string) {
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

	reader := bufio.NewReader(os.Stdin)
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
// <secret>", cmd/cmd_password_manager.go, always calls
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
		runHashPasswordUtility()
		os.Exit(0)
	}

	// If the mfa flag was given, run runTOTPSetupUtility and exit.
	if *sOptTOTPSetup != "" {
		runTOTPSetupUtility(*sOptConfigFilename, *sOptTOTPSetup)
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
