// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

// Command routercli-daemon is the persistent, sidecar daemon
// claude/DAEMON_ARCHITECTURE_DESIGN.md describes: a long running
// process, opt in through config.SystemConfig.DaemonSocketPath, that
// holds a deployment's own canonical state, ProductState, the tree
// structure's own runtime defined aliases and level passwords, the
// user database, and the role set, genuinely shared across every
// attached CLI session rather than each one holding its own separate,
// driftable copy. The routercli binary itself, package main at the
// root of this repository, keeps doing its own terminal I/O exactly as
// it always has; this binary never talks to a terminal at all, only to
// CLI clients over its own Unix domain socket, package daemon.
//
// This binary's own job is small and mostly wiring: load the same
// etc/routercli.yaml a routercli CLI process would, build an initial
// daemon.State from it the same way replaying startup-config already
// does for a standalone CLI session, open the socket, and hand
// everything else to daemon.Server, which already implements this
// project's own wire protocol, session tracking, and reboot behavior.
// SIGTERM stops this daemon; SIGHUP rereads configuration and rebroadcasts,
// both settled directly in claude/DAEMON_ARCHITECTURE_DESIGN.md's own
// "Control surface" section.
//
// The blank imports of cmd/core and cmd/session below, alongside
// cmd/product's own named import, are not about this binary running
// any of their handlers directly, it never does, only ever replaying
// startup-config through a throwaway *command.AppContext, see
// buildDaemonState. They exist so every "run:" name any tree file
// under var/tree/ names is actually registered, command.Register
// having run through each package's own init, before
// command.LoadTreeStructure and VerifyCommandLevels below ever check
// one; a real routercli CLI process needs the exact same three
// imports for the exact same reason, see main.go at the root of this
// repository.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/gotermme/routercli/auditlog"
	"github.com/gotermme/routercli/auth"
	_ "github.com/gotermme/routercli/cmd/core"
	"github.com/gotermme/routercli/cmd/product"
	_ "github.com/gotermme/routercli/cmd/session"
	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/config"
	"github.com/gotermme/routercli/daemon"

	"github.com/gologme/log"
	"github.com/pborman/getopt/v2"
)

func main() {
	defaultConfigFilename := "etc/routercli.yaml"
	sOptConfigFilename := getopt.StringLong("config", 'c', defaultConfigFilename, "The main configuration file", "string")
	getopt.HelpColumn = 35
	getopt.DisplayWidth = 120
	getopt.SetParameters("")
	getopt.Parse()

	cfg, err := config.LoadSystemConfig(*sOptConfigFilename)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load configuration:", err)
		os.Exit(1)
	}
	if cfg.DaemonSocketPath == "" {
		fmt.Fprintln(os.Stderr, "this deployment has no DaemonSocketPath configured in", *sOptConfigFilename, "- routercli-daemon has nothing to listen on")
		os.Exit(1)
	}

	logOutput := io.Writer(os.Stderr)
	if cfg.LogFile != "" {
		if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0750); err != nil {
			fmt.Fprintln(os.Stderr, "warning: failed to prepare log file directory, falling back to stderr:", err)
		} else if f, err := os.OpenFile(cfg.LogFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640); err != nil {
			fmt.Fprintln(os.Stderr, "warning: failed to open LogFile, falling back to stderr:", err)
		} else {
			logOutput = f
			defer f.Close()
		}
	}
	logger := log.New(logOutput, "routercli-daemon ", log.LstdFlags)
	switch cfg.LogLevel {
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
	if os.Getenv("ROUTERCLI_DEBUG") != "" {
		logger.EnableLevel("debug")
	}

	audit := auditlog.New(cfg.AuditLogFile, logger)
	if cfg.AuditLogEnabled {
		if err := os.MkdirAll(filepath.Dir(cfg.AuditLogFile), 0750); err != nil {
			fmt.Fprintln(os.Stderr, "failed to prepare audit log directory:", err)
			os.Exit(1)
		}
		if err := audit.Enable(); err != nil {
			fmt.Fprintln(os.Stderr, "failed to open audit log:", err)
			os.Exit(1)
		}
	}
	defer audit.Close()

	reload := func() (daemon.State, error) {
		return buildDaemonState(&cfg, logger)
	}
	initialState, err := reload()
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to build initial state:", err)
		os.Exit(1)
	}

	store := daemon.NewStore(initialState)
	defer store.Close()

	sessions := daemon.NewSessionDirectory()
	defer sessions.Close()

	keyPath := daemon.StaticKeyPath(cfg.DaemonSocketPath)
	staticPrivate, err := daemon.LoadOrCreateStaticKeyPair(keyPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to load or create this daemon's own static key pair:", err)
		os.Exit(1)
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DaemonSocketPath), 0750); err != nil {
		fmt.Fprintln(os.Stderr, "failed to prepare socket directory:", err)
		os.Exit(1)
	}
	// AllowedUIDs is seeded with only this daemon's own effective UID,
	// the minimum every deployment needs; a deployment wanting more
	// than one local account to reach this socket configures that
	// through its own operating system account and group setup around
	// socketPermissions instead, see Listen's own doc comment, rather
	// than a RouterCLI specific allow list this project does not yet
	// expose through configuration.
	checker := daemon.NewAllowedUIDs(uint32(os.Getuid()))
	listener, err := daemon.Listen(cfg.DaemonSocketPath, checker)
	if err != nil {
		fmt.Fprintln(os.Stderr, "failed to open daemon socket:", err)
		os.Exit(1)
	}

	server := daemon.NewServer(listener, staticPrivate, store, sessions, audit, reload, logger)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGHUP)
	go func() {
		for s := range sig {
			switch s {
			case syscall.SIGHUP:
				logger.Infoln("SIGHUP received, rereading configuration and rebroadcasting")
				if err := server.TriggerReboot(); err != nil {
					logger.Errorf("SIGHUP reboot failed: %v", err)
				}
			case syscall.SIGTERM:
				logger.Infoln("SIGTERM received, draining sessions and stopping")
				if err := server.Shutdown(); err != nil {
					logger.Errorf("Shutdown: %v", err)
				}
				return
			}
		}
	}()

	logger.Infoln("routercli-daemon listening on", cfg.DaemonSocketPath)
	server.Serve()
	logger.Infoln("routercli-daemon stopped")
}

// buildDaemonState loads this deployment's own tree structure, role
// set, and, when cfg.AuthRequired is on, user database, the exact same
// four files a standalone routercli CLI process already loads at its
// own startup, then replays cfg.StartupConfigFile through
// command.LoadStartupConfig against a throwaway, disposable
// *command.AppContext built only for this one replay, to produce a
// fresh ProductState and a fresh set of runtime defined aliases and
// level passwords the same way it always has, see
// command/replay.go. This is called once at this daemon's own
// startup, and again by TriggerReboot every time a reboot actually
// runs, so a fresh daemon.State always reflects whatever is on disk
// right now, never a stale, previously loaded copy.
//
// The replay context's own DaemonClient is a throwaway
// *daemon.StandaloneClient wrapping exactly the productState, levels,
// users, and roles this function is about to return, since a replayed
// "hostname" line, for instance, already goes through
// ctx.DaemonClient.MutateProductState rather than touching ctx.State
// directly, see cmd/product/cmd_hostname.go. This disposable client is
// closed before this function returns; nothing about it survives past
// this one replay.
func buildDaemonState(cfg *config.SystemConfig, logger *log.Logger) (daemon.State, error) {
	levels, err := command.LoadTreeStructure(cfg.TreeStructure, cfg.CommonTreeFile)
	if err != nil {
		return daemon.State{}, fmt.Errorf("loading tree structure: %w", err)
	}
	for _, level := range levels.Order {
		level.RateLimiter = auth.NewRateLimiter(cfg.CommandLevelMaxAttempts, cfg.CommandLevelAttemptWindow.AsDuration(), cfg.CommandLevelLockoutDuration.AsDuration())
	}
	if problems := command.VerifyCommandLevels(levels); len(problems) > 0 {
		return daemon.State{}, fmt.Errorf("invalid tree structure: %w", errors.Join(problems...))
	}
	if problems := command.VerifyVendorDefinedSecrets(levels); len(problems) > 0 {
		return daemon.State{}, fmt.Errorf("invalid tree structure: %w", errors.Join(problems...))
	}

	roles, err := command.LoadRoles(cfg.RolesFile)
	if err != nil {
		return daemon.State{}, fmt.Errorf("loading roles: %w", err)
	}
	if problems := command.VerifyRoles(levels, roles); len(problems) > 0 {
		return daemon.State{}, fmt.Errorf("invalid roles: %w", errors.Join(problems...))
	}

	users := auth.Users{}
	if cfg.AuthRequired {
		users, err = auth.LoadUsers(cfg.UsersFile)
		if err != nil {
			return daemon.State{}, fmt.Errorf("loading users: %w", err)
		}
	}

	productState := &product.ProductState{}
	standalone := daemon.NewStandaloneClient(daemon.NewState(productState, levels, users, roles, cfg))
	defer standalone.Close()

	base := levels.Base()
	replayCtx := &command.AppContext{
		State:             productState,
		DaemonClient:      standalone,
		Logger:            logger,
		Levels:            levels,
		Roles:             roles,
		Users:             users,
		Session:           &auth.Session{CommandLevel: base.Name},
		Position:          command.NewCommandLevelStack(base.Name, base.PromptSuffix, base.Tree),
		StartupConfigFile: cfg.StartupConfigFile,
		RolesFile:         cfg.RolesFile,
	}

	if err := os.MkdirAll(filepath.Dir(cfg.StartupConfigFile), 0750); err != nil {
		return daemon.State{}, fmt.Errorf("preparing startup-config directory: %w", err)
	}
	if err := command.LoadStartupConfig(replayCtx, cfg.StartupConfigFile); err != nil {
		return daemon.State{}, fmt.Errorf("replaying startup-config: %w", err)
	}

	return daemon.NewState(productState, levels, users, roles, cfg), nil
}
