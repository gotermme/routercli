# RouterCLI

A Go framework for building network equipment-style command line interfaces,
inspired by Cisco IOS and HP ProCurve. This currently supports nested Command
Levels, changing prompts, abbreviation-aware tab completion (`sh run` → 
`show running-config`), `no <command>` negation, hidden and password-gated 
commands, multi-user authentication, an audit log, multi-factor authentication 
(TOTP), and multi-language support.


[![Go Reference](https://pkg.go.dev/badge/github.com/gotermme/routercli.svg)](https://pkg.go.dev/github.com/gotermme/routercli) [![CI](https://github.com/gotermme/routercli/actions/workflows/ci.yml/badge.svg)](https://github.com/gotermme/routercli/actions/workflows/ci.yml) [![codecov](https://codecov.io/gh/gotermme/routercli/graph/badge.svg)](https://codecov.io/gh/gotermme/routercli) [![Release](https://img.shields.io/github/v/release/gotermme/routercli)](https://github.com/gotermme/routercli/releases) [![Go Version](https://img.shields.io/github/go-mod/go-version/gotermme/routercli)](https://github.com/gotermme/routercli) [![License](https://img.shields.io/github/license/gotermme/routercli)](https://github.com/gotermme/routercli/blob/main/LICENSE)


## Quick start

```sh
go build .
./routercli
```

That runs the bundled example CLI with no setup required:

```
router> show version
RouterCLI 0.1
router> enable
Entering exec mode.
router# configure terminal
router(config)# hostname mytest
Hostname set to mytest.
mytest(config)# interface eth0
mytest(config-if)# description "uplink to core"
Interface description set.
mytest(config-if)# end
mytest# show running-config
! (example running-config)
hostname mytest
interface eth0
 description "uplink to core"
!
mytest# disable
Leaving exec mode, back to base.
mytest> exit
```

Notice the prompt picks up the configured hostname immediately, and that
`configure terminal` requires `enable` first. `config` is a Command Level
whose `parent` is `exec`, so it is unreachable from the base level (see
`var/tree/README.md`).

`go run main.go` also works. `main.go` is deliberately the only source file in
`package main`; everything else lives in its own package. The one exception is
`main_test.go`, which is also in `package main` since a Go test file has to
share the package of the code it tests.

## What is actually part of the framework versus the example

This repository contains both the RouterCLI framework and a working example CLI.
It is important to understand the distinction when building your own CLI:

 - **The framework**: 
   - `auditlog/`, `auth/`, 
   - `command/`, `config/`, `completer/`
   - `tokenize/`, `i18n/`. 
   - These packages know nothing about routers, interfaces, or hostnames. They
     are the reusable machinery that enables things like: Auditing,
     Authentication, Command resolution, Configuration file parsing, Tab
     completion, Interactive `?` help, Command Levels and Tree Structure
     processing, and internationalization (i18n)
 - **The CLI Environment (currently populated with an example)**: 
   - `cmd/` (the command handlers, such as `show`, `hostname`, etc),
   - `var/tree/*.yaml` (the Tree Structure, Command Level definitions)
   - `var/lang/*.yaml` (the language catalog)
   - These are demonstration content, meant to be read, copied, and replaced.

## Layout

```
RouterCLI
│
├── Framework
│   ├── command
│   ├── completer
│   ├── auth
│   ├── auditlog
│   ├── config
│   ├── tokenize
│   └── i18n
│
└── Example CLI
    ├── cmd/
    ├── var/tree/
    ├── var/lang/
    └── etc/
```

Building your own CLI means keeping the framework packages as they are,
replacing the example command handlers and tree files with your own, and
writing the small entry and exit files your own Tree Structure needs.

## Directory layout

```
Core System

main.go        The entry point: startup, config loading, and the read loop.

auth/          Password hashing (bcrypt), multi-user store, login, 
               TOTP second-factor enrollment and verification, and
               rate limiting (login attempts and, via package
               command, Command Level and per-command passwords)

auditlog/      Timestamped, runtime-toggleable audit trail

command/       The framework core: Command, Resolve(), ValidateArgs(),
               HelpText(), the plugin registry, CommandLevelStack, the
               YAML tree loader, and the Tree Structure / Command Level
               system (CommandLevel, LoadTreeStructure,
               EnterCommandLevel/ExitCommandLevel, VerifyCommandLevels)

completer/     Tab completion and the interactive "?" contextual help
               (readline Listener, not readline's own AutoComplete, which
               is too limited for abbreviation-aware, tree-driven completion)

config/        A YAML configuration file parser

tokenize/      Quote-aware line tokenizer and its inverse, used for the
               show-running-config round-trip guarantee

i18n/          Support for internationalization via a language catalog
               (flat YAML per language, with a real fallback chain)


End Application

cmd/           One file per command, each self-registering through init().

var/tree/      The Tree Structure manifest (tree_structure.yaml) and
               each Command Level's own command tree (level_*.yaml).

var/lang/      The language catalog, one YAML file per language

var/log/       System, history, and audit logs

etc/           The main configuration file (routercli.yaml) and the
               multi-user database (users.yaml)
```

## Building your own CLI on top of this

### 1. Add a command

Every command is one file in `cmd/` that registers a handler from
`init()`. Copy the shape of an existing one: `cmd/cmd_show.go` for a
simple read-only command, `cmd/cmd_set.go` for one that takes an
argument and mutates state, or `cmd/cmd_interface.go` if you are building
something that enters a new mode.

```go
// cmd/cmd_ping.go
package cmd

import (
   "fmt"

   "github.com/gotermme/routercli/command"
)

func init() {
   command.Register("ping", func(ctx *command.AppContext, args []string) error {
      fmt.Println(ctx.Translator.T("ping.text"))
      return nil
   })
}
```

Then add it to whichever `var/tree/level_*.yaml` file should offer it:

```yaml
ping:
  desc_key: ping.desc
  run: ping
```

`run:` must match the string passed to `command.Register()` exactly, or the
program refuses to start. A bad handler reference is a hard startup error on
purpose, since a command that silently does nothing because of a typo is a
worse failure mode than refusing to launch. That is the whole mechanism: no
import statements to add anywhere else, and no switch statement to extend.
`main.go` never needs to change.

### 2. Command Level options

Please see `var/tree/README.md` for more information on the Tree Structure,
Command Levels, and the commands within them.

### 3. Supporting `no <command>`

Set `negatable: true` on the command, then check `ctx.Negated` inside the
handler. This is exactly how the bundled example's own `interface shutdown`
works:

```go
command.Register("interface.shutdown", func(ctx *command.AppContext, args []string) error {
   iface := ...
   if ctx.Negated {
      iface.Shutdown = false
      return nil
   }
   iface.Shutdown = true
   return nil
})
```

`ctx.Negated` is true for exactly the duration of that one call, so read it
once near the top of the handler. `no <command>` runs the same handler
as `<command>` alone. It is not a second registration. Argument
validation (`minargs` and `maxargs`) is skipped entirely when negated, since
"no X" usually has a different valid shape than "X" itself. For example, `no
description` takes zero arguments to clear a value that `description <text>`
set. The handler decides what is acceptable for its own negated case.

A command must opt in with `negatable: true`. Nothing is negatable by default.
In particular, do not mark a mode-pushing command such as `interface` negatable
without a handler that actually knows what `ctx.Negated` should mean for it.
`no interface eth0` naturally reads as "delete this interface", not "enter its
config mode negated", and nothing does that deletion semantic for you
automatically.

### 4. Adding a new Command Level

A Command Level is an entry in `var/tree/tree_structure.yaml`, its own
`level_<name>.yaml`, and a hand-written `cmd/cmd_*.go` file that actually moves
a session into it. See `var/tree/README.md` for the full manifest schema. There
is no separate registration step in `main.go`. Add the entry to the manifest
and write the one small file, and it is available.

**A nested, stacking mode** (config-if layered on top of config, for
example) calls `command.RequireCurrentCommandLevel` directly and pushes
its own `CommandLevelStack` frame. This is what `cmd_configure.go` and
`cmd_interface.go` already do:

```go
// entering the new mode
level := ctx.Levels.ByName["config-router"]
if err := command.RequireCurrentCommandLevel(ctx, "config-router", level.Parent); err != nil {
   return err
}
ctx.Position.Push(command.CommandLevelFrame{
   Name:         "config-router",
   PromptSuffix: level.PromptSuffix,
   Tree:         level.Tree,
   Context:      protocolName, // whatever the mode's commands need back
})
```

**A root swap Command Level** (a new Command Level alongside `exec` or
`diagnostic`, where, unlike the nested mode above, only one can ever be
active at a time) calls `command.EnterCommandLevel` and
`command.ExitCommandLevel` instead. See `cmd_enable.go` and
`cmd_diagnostic_mode.go` for the exact pattern to copy.

Either way, `help`, `exit`, and `end` come from `var/tree/level_common.yaml`
automatically. Do not redefine them in a Command-Level-specific file; doing so
is a hard startup error (see `command.MergeTrees`). `exit` goes up exactly one
level, and only quits the whole program when already at the root. `end` jumps
straight back to the root from anywhere. Both come for free once your level's
tree is loaded through the manifest. You do not implement them per level.

Run `./routercli --check-config` after adding a new level. It loads and verifies
the whole Tree Structure, including confirming that your new `enter_command`
and `exit_command` really were registered, and exits without starting the
interactive loop, so you can catch a typo or a forgotten file immediately.

### 5. Adding a language

Copy `var/lang/en.yaml`, translate the values, and save it as
`var/lang/<code>.yaml`. That is it, with no code change needed. Missing
keys in a partial translation fall back to `DefaultLanguage`, and then to
a visible `[[bracketed.key]]` placeholder if truly missing everywhere,
rather than failing or showing blank text.

Switch language at runtime with `language set <code>`, or set
`CurrentLanguage` and `DefaultLanguage` in `etc/routercli.yaml` for the
startup defaults.

### 6. Turning on authentication

```yaml
# etc/routercli.yaml
AuthRequired: true
UsersFile: etc/users.yaml
```

Generate a password hash for `etc/users.yaml`:

```sh
./routercli --hashpassword
```

A user needs `password` set to log in, but this only matters when
`AuthRequired` is `true`. When it is `false`, login is skipped entirely and
`etc/users.yaml` is never even read.

Setting a password gate to enter a Command Level is also possible. The example
uses the concept of elevating with `enable`, but this would be called whatever
your own manifest calls the command to do this kind of Command Level swap. It
is a separate, decoupled concern from login. It checks the Command Level's own
shared secret, `password_hash` in `var/tree/tree_structure.yaml`, settable at
runtime through `password manager` from config mode. That command prompts for
the new secret, masked, rather than taking it as a same-line argument,
specifically so it never ends up in the audit log or readline history in
plaintext.

**Rate limiting** guards login, elevating into a Command Level, and an
individual command's own `password_hash`, each independently
configurable:

```yaml
# etc/routercli.yaml
LoginAttemptWindow: 2m
LoginLockoutDuration: 5m
CommandLevelMaxAttempts: 3
CommandLevelAttemptWindow: 2m
CommandLevelLockoutDuration: 5m
```

The windowed lockout behavior shown above is disabled by default for all three,
matching a real device's own `enable` having no attempt limit either. Login
itself still keeps this project's original flat cap of `LoginMaxAttempts`
(three by default) even with the window off, since that part was never
optional. See `etc/README.md` for the full field list
(`Login*`, `CommandLevel*`, and `CommandPassword*`, nine fields in total) and
exactly how the window and lockout pair validation works.

**Two-factor login (TOTP / Google Authenticator)** is opt-in per user.
Enroll someone:

```sh
./routercli --mfa alice
```

This shows a QR code and the plain text secret, for apps or situations where
scanning is not convenient. Either works with Google Authenticator, Authy,
1Password, or any standard TOTP app. It then asks you to type the code your app
is showing right now, to confirm the secret actually works, before printing the
`totp_secret:` line to add to that user's entry in `etc/users.yaml`. A user
with no `totp_secret` set is never prompted for a code at all, so this does not
change the login flow for anyone who has not been enrolled. As of right now this
is all manually enabled.

Authentication in this project lives entirely in `auth/`, with password hashing,
the TOTP implementation, and the login flow all in one place, specifically so a
future second-factor method, most likely a hardware security key, has one clear
seam to plug into: `auth.SecondFactorRequired` and `auth.VerifySecondFactor`
are the only two functions that need to know which methods exist.

### 7. Audit logging

```yaml
# etc/routercli.yaml
AuditLogFile: var/log/audit.log
AuditLogEnabled: false   # starting state only
```

The audit log can also be enabled at runtime, if you so desire. The shipping
example does this with `audit-log enable`, `audit-log disable`, and `audit-log
status`. This is useful for turning it on mid-session without a restart. In the
example these are only reachable once a session has moved past the base Command
Level and live in `var/tree/level_exec.yaml` not the base level's own tree.
This means an unelevated session cannot reach them. Every dispatched command is
recorded with a timestamp, the username, or `-` if unauthenticated, and whether
it succeeded.

This is a different thing from the general system log(`LogLevel` and `LogFile`
in `etc/routercli.yaml`). The audit log is a fixed-format compliance record of
commands run, while the system log is debug-level tracing plus warnings for
diagnosing RouterCLI itself. `LogFile` defaults to stderr. Set it to send that
output to a real file instead, independent of `AuditLogFile`.

### 8. Locking down the shell

```yaml
# etc/routercli.yaml
PreventEscape: true
```

When this is enabled RouterCLI attempts to prevent a shell escape by ignoring
things like Ctrl-C, Ctrl-Z, Ctrl-\\, and `kill <pid>` at the OS level. Ctrl-D
no longer ends the session either; it instead prints a reminder to use `exit`
and keeps reading. The only way out becomes the `exit` command. NOTE: This
cannot block `SIGKILL` or `SIGSTOP`. Nothing can, on any OS, in any language.
That is a kernel guarantee, not a gap in this implementation.

## 9. Testing

```sh
test -z "$(gofmt -l .)"
go vet ./...
go build ./...
go test ./...
go test -race ./...
```

Every framework package has real unit test coverage. `command/` in
particular is worth reading if you are extending the resolution or
negation logic, since its tests double as executable documentation of
exactly what is guaranteed, for example
`TestResolveMultipleSubtreeOptionsNeverAutoPicksOne` and
`TestResolveNegatedCommand`.

To check a Tree Structure, either your own or the bundled example's after
editing it, run `./routercli --check-config`. It loads and verifies the Tree
Structure without starting the interactive loop, and is worth running as part
of the same routine.

## 10. Code execution logic

1. `main.go` runs.
2. Every `init()` in `cmd/` runs. This happens automatically, before
   `main()` even begins, purely because `main.go` imports package `cmd`.
   Every command, including each Command Level's own enter and exit
   command, is registered here.
3. `main.go` reads `etc/routercli.yaml` to find the Tree Structure
   manifest's path, along with the common tree's path.
4. `main.go` calls `command.LoadTreeStructure`, which parses
   `tree_structure.yaml`, resolves each Command Level's parent chain and
   inheritance, loads every `level_<name>.yaml` file, and merges in the
   common tree, validating every `run:` reference against the registry
   as it goes.
5. `main.go` calls `command.VerifyCommandLevels`, confirming every
   declared `enter_command` and `exit_command` actually corresponds to a
   registered command. This is the same check whether running normally or
   through `--check-config`. A broken manifest fails loudly here, either way.
6. If `--check-config` was passed, `main.go` prints the result and exits
   without starting the interactive loop. Otherwise, it builds the session
   and enters the read, resolve, validate, and dispatch loop.
