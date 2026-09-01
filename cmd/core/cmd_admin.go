// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package core

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"

	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"
)

// init - This function registers every command reachable from inside
// the new admin Command Level, see var/tree/level_admin.yaml and
// var/tree/tree_structure.yaml's own "admin" entry. This level
// replaces what used to be a separate level named su-config; "show
// running-config", "show startup-config", "copy running-config
// startup-config", and "erase startup-config" all still live in
// cmd/product/cmd_show.go and cmd/product/cmd_startup_config.go,
// reused here exactly as they already were, reachable now only
// because level_admin.yaml's own tree points its "run:" entries at
// those same already-registered names.
//
// "admin" and "return.admin" are the level's own entry and exit
// commands, the same enter/exit shape cmd_enable.go and the retired
// cmd_su_config.go already used, see EnterCommandLevel and
// ExitCommandLevel's own doc comments in package command. Everything
// else here is new: account create, account delete, account roles
// add and remove, erase users, restore-factory-defaults, and reload.
func init() {
	command.Register("admin", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["admin"]
		entered, err := command.EnterCommandLevel(ctx, level, ctx.Levels.ByName[level.Parent])
		if err != nil {
			return err
		}
		if !entered {
			fmt.Println(ctx.Translator.T("admin.already_here"))
			return nil
		}
		fmt.Println(ctx.Translator.T("admin.entered"))
		return nil
	})

	command.Register("return.admin", func(ctx *command.AppContext, args []string) error {
		level := ctx.Levels.ByName["admin"]
		exited, err := command.ExitCommandLevel(ctx, level, ctx.Levels.ByName[level.Parent])
		if err != nil {
			return err
		}
		if !exited {
			fmt.Println(ctx.Translator.T("return_admin.not_here"))
			return nil
		}
		fmt.Println(ctx.Translator.T("return_admin.left"))
		return nil
	})

	command.Register("account.create", runAccountCreate)
	command.Register("account.delete", runAccountDelete)
	command.Register("account.roles.add", runAccountRolesAdd)
	command.Register("account.roles.remove", runAccountRolesRemove)
	command.Register("erase.users", runEraseUsers)
	command.Register("restore-factory-defaults", runRestoreFactoryDefaults)
	command.Register("reload", runReload)
}

// ----------------------------------------------------------------------
// account create, account delete
// ----------------------------------------------------------------------

// runAccountCreate - This function is the registered "account create"
// handler. It carries the fixed command.HandlerFunc signature, so it
// simply passes the real process's stdin file descriptor and stdout
// along to runAccountCreateWithIO, the same split runPasswordChange
// in cmd_password.go already uses, and for the same reason: a test
// can hand runAccountCreateWithIO a pty's slave file directly instead
// of needing a real terminal attached to the test binary.
func runAccountCreate(ctx *command.AppContext, args []string) error {
	return runAccountCreateWithIO(ctx, int(os.Stdin.Fd()), os.Stdout, args)
}

// runAccountCreateWithIO - This function drives "account create
// <username>", in one of three shapes, distinguished purely by how
// many trailing tokens follow the username, see
// var/tree/level_admin.yaml's own doc comment for why this could not
// be modeled as nested subcommands the way "totp enable" and "totp
// enable qr" are: the username itself is a free argument that must
// come before any further keyword, and this project's own command
// resolution has no way to skip past a token that failed to match a
// subcommand and resume keyword matching afterward.
//
//   - "account create <username>" alone prompts interactively for the
//     new account's first password, masked, twice, through the same
//     auth.PromptNewPassword and auth.PromptPasswordConfirmation flow
//     "password change" already uses, so a real password is never
//     typed on the command line or written to the audit log.
//   - "account create <username> generate" auto-generates a password
//     meeting ctx.PasswordPolicy, see generatePassword below, and
//     prints it once, the only time it is ever shown in plain text.
//   - "account create <username> hash <hash>" imports an
//     already-computed hash directly, never a plaintext password, the
//     same "password manager hash" precedent
//     cmd/core/cmd_password_manager.go already sets, for bulk resets
//     or preloading identical accounts across many devices.
//
// The first two shapes set MustChangePassword true on the new
// account, forcing whoever logs in with it straight into changing the
// password before anything else runs, see
// auth.User.MustChangePassword's own doc comment. The third does not,
// since an imported hash is presumed to already be the real intended
// credential, not a placeholder anyone still needs to replace.
func runAccountCreateWithIO(ctx *command.AppContext, fd int, stdout io.Writer, args []string) error {
	username := args[0]
	if ctx.Users == nil {
		return fmt.Errorf("%s", ctx.Translator.T("account.create.no_user_database"))
	}
	if _, exists := ctx.Users[username]; exists {
		return fmt.Errorf("%s", ctx.Translator.T("account.create.already_exists", username))
	}

	var hash string
	mustChange := false

	switch {
	case len(args) == 1:
		newPassword, err := auth.PromptNewPassword(stdout, fd, ctx.Translator)
		if err != nil {
			return err
		}
		confirmPassword, err := auth.PromptPasswordConfirmation(stdout, fd, ctx.Translator)
		if err != nil {
			return err
		}
		if newPassword != confirmPassword {
			return fmt.Errorf("%s", ctx.Translator.T("password.change.mismatch"))
		}
		if violations := auth.ValidatePassword(newPassword, ctx.PasswordPolicy); len(violations) > 0 {
			printPasswordViolations(ctx, violations)
			return fmt.Errorf("%s", ctx.Translator.T("account.create.policy_violation"))
		}
		h, err := auth.HashPassword(newPassword)
		if err != nil {
			return err
		}
		hash = h
		mustChange = true

	case len(args) == 2 && args[1] == "generate":
		newPassword, err := generatePassword(ctx.PasswordPolicy)
		if err != nil {
			return err
		}
		h, err := auth.HashPassword(newPassword)
		if err != nil {
			return err
		}
		hash = h
		mustChange = true
		fmt.Println(ctx.Translator.T("account.create.generated_password", newPassword))

	case len(args) == 3 && args[1] == "hash":
		if !auth.IsRecognizedHash(args[2]) {
			return fmt.Errorf("%s", ctx.Translator.T("account.create.unrecognized_hash"))
		}
		hash = args[2]
		mustChange = false

	default:
		return fmt.Errorf("%s", ctx.Translator.T("account.create.usage"))
	}

	ctx.Users[username] = &auth.User{
		Username:           username,
		PasswordHash:       hash,
		MustChangePassword: mustChange,
	}
	if err := auth.SaveUsers(ctx.UsersFile, ctx.Users); err != nil {
		delete(ctx.Users, username)
		return err
	}
	ctx.Logger.Debugln("DEBUG: account", username, "created by", ctx.Session.Username)
	fmt.Println(ctx.Translator.T("account.create.confirm", username))
	return nil
}

// runAccountDelete - This function is the registered "account delete
// <username>" handler. It refuses, rather than deletes, when username
// is the last account left holding the deployment's own bypass role,
// see isLastBypassRoleHolder below, closing off the one genuinely
// unrecoverable mistake this design could otherwise allow: a
// deployment locking itself out of its own admin level entirely, with
// no way back in short of "restore-factory-defaults", see that
// command's own doc comment for the real recovery path once that
// happens anyway, whether through this refusal being bypassed by
// hand-editing users.yaml or through any other means.
func runAccountDelete(ctx *command.AppContext, args []string) error {
	username := args[0]
	user, exists := ctx.Users[username]
	if !exists {
		return fmt.Errorf("%s", ctx.Translator.T("account.delete.not_found", username))
	}
	if isLastBypassRoleHolder(ctx, user) {
		return fmt.Errorf("%s", ctx.Translator.T("account.delete.last_bypass_role_holder", username))
	}

	delete(ctx.Users, username)
	if err := auth.SaveUsers(ctx.UsersFile, ctx.Users); err != nil {
		ctx.Users[username] = user
		return err
	}
	ctx.Logger.Debugln("DEBUG: account", username, "deleted by", ctx.Session.Username)
	fmt.Println(ctx.Translator.T("account.delete.confirm", username))
	return nil
}

// isLastBypassRoleHolder - This function reports whether user is
// currently the only account in ctx.Users holding the deployment's
// own bypass role, see command.RoleSet.BypassRole's own doc comment.
// False whenever no role in this deployment is marked bypass at all,
// or user does not hold it in the first place, there being nothing
// for either "account delete" or "account roles remove" to protect.
func isLastBypassRoleHolder(ctx *command.AppContext, user *auth.User) bool {
	if ctx.Roles == nil || ctx.Roles.BypassRole == "" {
		return false
	}
	if !hasRole(user, ctx.Roles.BypassRole) {
		return false
	}
	count := 0
	for _, u := range ctx.Users {
		if hasRole(u, ctx.Roles.BypassRole) {
			count++
		}
	}
	return count <= 1
}

// hasRole - This function reports whether user's own Roles list
// contains name.
func hasRole(user *auth.User, name string) bool {
	for _, r := range user.Roles {
		if r == name {
			return true
		}
	}
	return false
}

// ----------------------------------------------------------------------
// account roles add, account roles remove
// ----------------------------------------------------------------------

// runAccountRolesAdd - This function is the registered "account roles
// add <username> <role>" handler. role must already be declared in
// var/tree/roles.yaml; an unrecognized role name is refused outright
// rather than silently accepted and left to never match any
// AllowedRoles check, the same fail loudly convention this project
// applies to every other malformed request.
func runAccountRolesAdd(ctx *command.AppContext, args []string) error {
	username, roleName := args[0], args[1]
	user, exists := ctx.Users[username]
	if !exists {
		return fmt.Errorf("%s", ctx.Translator.T("account.roles.no_such_account", username))
	}
	if !roleKnown(ctx.Roles, roleName) {
		return fmt.Errorf("%s", ctx.Translator.T("account.roles.unknown_role", roleName))
	}
	if hasRole(user, roleName) {
		fmt.Println(ctx.Translator.T("account.roles.add.already_has", username, roleName))
		return nil
	}

	previous := append([]string(nil), user.Roles...)
	user.Roles = append(user.Roles, roleName)
	if err := auth.SaveUsers(ctx.UsersFile, ctx.Users); err != nil {
		user.Roles = previous
		return err
	}
	ctx.Logger.Debugln("DEBUG: role", roleName, "added to", username, "by", ctx.Session.Username)
	fmt.Println(ctx.Translator.T("account.roles.add.confirm", roleName, username))
	return nil
}

// runAccountRolesRemove - This function is the registered "account
// roles remove <username> <role>" handler. It carries the same last
// bypass role holder protection as "account delete", see
// isLastBypassRoleHolder's own doc comment: removing a lone remaining
// bypass role from its own holder is exactly as unrecoverable as
// deleting that account outright would be.
func runAccountRolesRemove(ctx *command.AppContext, args []string) error {
	username, roleName := args[0], args[1]
	user, exists := ctx.Users[username]
	if !exists {
		return fmt.Errorf("%s", ctx.Translator.T("account.roles.no_such_account", username))
	}
	if !hasRole(user, roleName) {
		fmt.Println(ctx.Translator.T("account.roles.remove.does_not_have", username, roleName))
		return nil
	}
	if ctx.Roles != nil && roleName == ctx.Roles.BypassRole && isLastBypassRoleHolder(ctx, user) {
		return fmt.Errorf("%s", ctx.Translator.T("account.delete.last_bypass_role_holder", username))
	}

	previous := append([]string(nil), user.Roles...)
	kept := previous[:0:0]
	for _, r := range user.Roles {
		if r != roleName {
			kept = append(kept, r)
		}
	}
	user.Roles = kept
	if err := auth.SaveUsers(ctx.UsersFile, ctx.Users); err != nil {
		user.Roles = previous
		return err
	}
	ctx.Logger.Debugln("DEBUG: role", roleName, "removed from", username, "by", ctx.Session.Username)
	fmt.Println(ctx.Translator.T("account.roles.remove.confirm", roleName, username))
	return nil
}

// roleKnown - This function reports whether name is a role this
// deployment has actually declared in var/tree/roles.yaml. False for
// a nil roles, meaning no roles.yaml was ever loaded at all.
func roleKnown(roles *command.RoleSet, name string) bool {
	if roles == nil {
		return false
	}
	_, ok := roles.ByName[name]
	return ok
}

// ----------------------------------------------------------------------
// Password generation
// ----------------------------------------------------------------------

const (
	passwordGenMinLength = 16
	passwordGenLower     = "abcdefghijklmnopqrstuvwxyz"
	passwordGenUpper     = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	passwordGenDigits    = "0123456789"
	passwordGenSpecial   = "!@#$%^&*()-_=+"
)

// generatePassword - This function returns a random password
// satisfying policy, for "account create <username> generate". The
// generated length is the larger of policy.MinLength and
// passwordGenMinLength, sixteen characters, deliberately longer than
// whatever bare minimum a deployment configured, capped at
// auth.MaxPasswordLength so ValidatePassword's own TooLong check can
// never reject a password this function just generated. Every
// character class policy actually requires is guaranteed to appear at
// least once, then the rest of the length is filled from the full
// allowed pool and the whole result is shuffled with crypto/rand, so
// a required character never sits predictably at a fixed position.
func generatePassword(policy auth.PasswordPolicy) (string, error) {
	length := policy.MinLength
	if length < passwordGenMinLength {
		length = passwordGenMinLength
	}
	if length > auth.MaxPasswordLength {
		length = auth.MaxPasswordLength
	}

	pool := passwordGenLower + passwordGenDigits
	var required []byte

	if policy.RequireUppercase {
		pool += passwordGenUpper
		c, err := randomChar(passwordGenUpper)
		if err != nil {
			return "", err
		}
		required = append(required, c)
	}
	if policy.RequireNumbers {
		c, err := randomChar(passwordGenDigits)
		if err != nil {
			return "", err
		}
		required = append(required, c)
	}
	if policy.RequireSpecialChars {
		pool += passwordGenSpecial
		c, err := randomChar(passwordGenSpecial)
		if err != nil {
			return "", err
		}
		required = append(required, c)
	}

	remaining := length - len(required)
	if remaining < 0 {
		remaining = 0
	}
	buf := make([]byte, 0, length)
	buf = append(buf, required...)
	for i := 0; i < remaining; i++ {
		c, err := randomChar(pool)
		if err != nil {
			return "", err
		}
		buf = append(buf, c)
	}

	shuffled, err := shuffleBytes(buf)
	if err != nil {
		return "", err
	}
	return string(shuffled), nil
}

// randomChar - This function returns one byte chosen uniformly at
// random from charset, using crypto/rand rather than math/rand, the
// same reasoning every other secret this project generates,
// auth.HashPassword's own salt among them, already follows: a
// generated password is a real credential, not test data.
func randomChar(charset string) (byte, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
	if err != nil {
		return 0, err
	}
	return charset[n.Int64()], nil
}

// shuffleBytes - This function returns a copy of b in a random order,
// a standard Fisher-Yates shuffle driven by crypto/rand, so
// generatePassword's own required characters, appended in a fixed
// order above, do not predictably end up at the front of every
// generated password.
func shuffleBytes(b []byte) ([]byte, error) {
	out := make([]byte, len(b))
	copy(out, b)
	for i := len(out) - 1; i > 0; i-- {
		j, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			return nil, err
		}
		out[i], out[j.Int64()] = out[j.Int64()], out[i]
	}
	return out, nil
}

// ----------------------------------------------------------------------
// erase users, restore-factory-defaults, reload
// ----------------------------------------------------------------------

// restoreFromDefaults - This function overwrites live with a fresh
// copy of ctx.DefaultsDir's own skeleton file, matched to live's own
// base name, for example etc/defaults/users.yaml restoring
// etc/users.yaml. restored is false, with a nil error, when no
// matching default file exists at all, a deployment that never set
// one up for this particular file, distinct from a real error reading
// or writing one that does exist.
func restoreFromDefaults(ctx *command.AppContext, live string, perm os.FileMode) (restored bool, err error) {
	def := filepath.Join(ctx.DefaultsDir, filepath.Base(live))
	data, err := os.ReadFile(def)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading default file %q: %w", def, err)
	}
	if dir := filepath.Dir(live); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return false, fmt.Errorf("preparing directory for %q: %w", live, err)
		}
	}
	if err := os.WriteFile(live, data, perm); err != nil {
		return false, fmt.Errorf("writing %q: %w", live, err)
	}
	return true, nil
}

// runEraseUsers - This function is the registered "erase users"
// handler. Unlike "erase startup-config", which simply deletes its
// own file, see cmd/product/cmd_startup_config.go's own doc comment
// for why that is safe there, deleting users.yaml to nothing would
// permanently lock every account, the bypass role included, out of
// this level and out of the whole deployment's own identity database.
// This instead replaces it with ctx.DefaultsDir's own skeleton copy,
// see restoreFromDefaults, then reloads it into ctx.Users so the
// current session immediately sees the restored account set.
func runEraseUsers(ctx *command.AppContext, args []string) error {
	restored, err := restoreFromDefaults(ctx, ctx.UsersFile, 0600)
	if err != nil {
		return fmt.Errorf("%s", ctx.Translator.T("erase_users.failed", err))
	}
	if !restored {
		fmt.Println(ctx.Translator.T("erase_users.no_defaults"))
		return nil
	}
	users, err := auth.LoadUsers(ctx.UsersFile)
	if err != nil {
		return fmt.Errorf("%s", ctx.Translator.T("erase_users.failed", err))
	}
	ctx.Users = users
	ctx.Logger.Debugln("DEBUG: users.yaml restored to factory defaults by", ctx.Session.Username)
	fmt.Println(ctx.Translator.T("erase_users.confirm"))
	return nil
}

// runRestoreFactoryDefaults - This function is the registered
// "restore-factory-defaults" handler, the real recovery path once
// "account delete" or "account roles remove" has refused as far as it
// is willing to, or once a deployment simply wants a genuine factory
// reset. It erases startup-config the same way "erase startup-config"
// already does, restores users.yaml and roles.yaml from
// ctx.DefaultsDir, see restoreFromDefaults, and then runs the exact
// same logic "reload" does, ending this connection so the next one
// starts completely fresh. A missing default for roles.yaml is not
// itself an error, the same "nothing to restore" tolerance
// restoreFromDefaults already gives every file it is asked to
// restore; a deployment that never used AllowedRoles at all may
// reasonably have no etc/defaults/roles.yaml of its own.
func runRestoreFactoryDefaults(ctx *command.AppContext, args []string) error {
	if err := os.Remove(ctx.StartupConfigFile); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%s", ctx.Translator.T("restore_factory_defaults.failed", err))
	}
	if _, err := restoreFromDefaults(ctx, ctx.UsersFile, 0600); err != nil {
		return fmt.Errorf("%s", ctx.Translator.T("restore_factory_defaults.failed", err))
	}
	if _, err := restoreFromDefaults(ctx, ctx.RolesFile, 0640); err != nil {
		return fmt.Errorf("%s", ctx.Translator.T("restore_factory_defaults.failed", err))
	}
	ctx.Logger.Debugln("DEBUG: factory defaults restored by", ctx.Session.Username)
	fmt.Println(ctx.Translator.T("restore_factory_defaults.confirm"))
	return runReload(ctx, nil)
}

// runReload - This function is the registered "reload" handler, also
// called directly by runRestoreFactoryDefaults above once it has
// finished restoring files on disk. RouterCLI is one OS process per
// connection, with no persistent daemon behind it today, so there is
// no single device process for this to reboot the way a real Cisco
// "reload" reboots one. What this does instead: re-read users.yaml,
// roles.yaml, and startup-config fresh from disk, through
// auth.LoadUsers, command.LoadRoles, and command.LoadStartupConfig,
// rebuilding this session's own in memory state from them, then end
// this connection through command.ErrQuit, the same sentinel "exit"
// at the base level already returns, forcing whoever ran this to
// reconnect. Any other already connected session, once such a thing
// exists, keeps running on its own stale in memory state until it
// independently reloads or reconnects too; making that better is real
// future work, a persistent daemon architecture, not something this
// command can promise on its own today.
func runReload(ctx *command.AppContext, args []string) error {
	if ctx.UsersFile != "" {
		users, err := auth.LoadUsers(ctx.UsersFile)
		if err != nil {
			return fmt.Errorf("%s", ctx.Translator.T("reload.failed", err))
		}
		ctx.Users = users
	}
	roles, err := command.LoadRoles(ctx.RolesFile)
	if err != nil {
		return fmt.Errorf("%s", ctx.Translator.T("reload.failed", err))
	}
	ctx.Roles = roles
	if err := command.LoadStartupConfig(ctx, ctx.StartupConfigFile); err != nil {
		return fmt.Errorf("%s", ctx.Translator.T("reload.failed", err))
	}

	ctx.Logger.Debugln("DEBUG: reload run by", ctx.Session.Username)
	fmt.Println(ctx.Translator.T("reload.confirm"))
	return command.ErrQuit
}
