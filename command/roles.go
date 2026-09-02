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
	"sort"

	"gopkg.in/yaml.v3"
)

// ----------------------------------------------------------------------
// Define Object Model
// ----------------------------------------------------------------------

// Role - This type is one entry in a deployment's own var/tree/roles.yaml,
// naming one role a user may hold and be granted through a Command
// or CommandLevel's own AllowedRoles list. Roles in this project are
// deliberately flat and unordered, see RoleSet's own doc comment, not
// a numbered hierarchy the way Cisco's own privilege levels are.
type Role struct {
	// Name identifies this role. It is what a Command or CommandLevel's
	// own AllowedRoles list references, and what auth.User.Roles
	// stores. This is not decoded from YAML; LoadRoles sets it from
	// this role's own key under the manifest's "roles:" map, the same
	// way CommandLevel.Name is set from tree_structure.yaml's own map
	// key.
	Name string `yaml:"-"`

	// Desc is a short, human readable description of what this role
	// is for, purely documentation, read by nothing in package command
	// itself today.
	Desc string `yaml:"desc"`

	// Bypass, false by default, marks this role as the deployment's
	// one reserved escape hatch. A user holding it automatically
	// passes every AllowedRoles check, on any Command or CommandLevel,
	// regardless of what that list actually contains, see Authorized
	// below. At most one role across the whole manifest may set this;
	// LoadRoles rejects a manifest that sets it on more than one. This
	// is what lets a deployment's very first administrator account log
	// in and start assigning ordinary roles to everyone else, see
	// var/tree/README.md's own roles section for the full bootstrap
	// reasoning.
	Bypass bool `yaml:"bypass"`
}

// RoleSet - This type is the whole loaded, validated var/tree/roles.yaml
// manifest, every role a deployment has declared, indexed by name for
// fast lookup by Authorized and by whichever admin command validates
// a role name a session typed, see cmd/core/cmd_admin.go's "account
// roles add" and "account roles remove".
//
// Roles are flat and unordered by design: a user may hold more than
// one, and a Command or CommandLevel's own AllowedRoles check is
// satisfied by any overlap at all between the two, never by rank or
// hierarchy. See Authorized's own doc comment for exactly how a check
// is decided.
type RoleSet struct {
	ByName map[string]*Role
	Order  []*Role // alphabetical by name, for anything that wants a stable listing

	// BypassRole is the Name of whichever role, if any, set bypass:
	// true in the manifest. Empty when no role in this deployment is
	// marked bypass. See Role.Bypass's own doc comment.
	BypassRole string
}

// rolesFile - This type is the top-level shape of the roles manifest.
// Everything lives under a single "roles:" key, the same top-level
// key convention every other YAML file in this project already
// follows. Role itself decodes directly through its own yaml tags
// rather than through a separate mirror type.
type rolesFile struct {
	Roles map[string]Role `yaml:"roles"`
}

// ----------------------------------------------------------------------
// Public Functions - Roles
// ----------------------------------------------------------------------

// LoadRoles - This function reads a deployment's own role declaration
// file, working default path var/tree/roles.yaml, see
// config.SystemConfig.RolesFile. A missing file is not an error; it
// returns an empty, valid RoleSet with no BypassRole, the correct
// state for a deployment that never uses AllowedRoles anywhere in its
// tree at all, see RoleSet's own doc comment. A file that exists but
// fails to parse, sets more than one role bypass: true, or contains
// more than one YAML document, is a hard error, the same fail loudly
// convention config.LoadSystemConfig and auth.LoadUsers already
// follow for their own files. Unknown YAML keys are also a hard
// error, for the same reason those two files already treat them that
// way, a misspelled property should never be silently dropped.
func LoadRoles(path string) (*RoleSet, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &RoleSet{ByName: map[string]*Role{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading roles manifest %s: %w", path, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)

	var parsed rolesFile
	if err := dec.Decode(&parsed); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("parsing roles manifest %s: %w", path, err)
	}

	var extra rolesFile
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("roles manifest %s contains multiple YAML documents", path)
		}
		return nil, fmt.Errorf("parsing roles manifest %s: %w", path, err)
	}

	byName := make(map[string]*Role, len(parsed.Roles))
	bypassRole := ""
	for name, r := range parsed.Roles {
		role := r
		role.Name = name
		if role.Bypass {
			if bypassRole != "" {
				return nil, fmt.Errorf("roles manifest %s marks more than one role bypass: true (%q and %q) - at most one role across the whole manifest may be the bypass role", path, bypassRole, name)
			}
			bypassRole = name
		}
		byName[name] = &role
	}

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	order := make([]*Role, 0, len(names))
	for _, name := range names {
		order = append(order, byName[name])
	}

	return &RoleSet{ByName: byName, Order: order, BypassRole: bypassRole}, nil
}

// CurrentUserRoles - This function returns the roles the currently
// logged in session's own account holds, from ctx.Users, or nil when
// there is no logged in session, no user database at all, meaning
// AuthRequired is off, or no matching entry for the session's own
// username. A nil or empty result is not itself an error; whether
// that leaves a role gated command or level reachable at all is
// Authorized's own decision, see its doc comment, not this
// function's.
func CurrentUserRoles(ctx *AppContext) []string {
	if ctx.Session == nil || !ctx.Session.Authenticated {
		return nil
	}
	if ctx.Users == nil {
		return nil
	}
	u := ctx.Users[ctx.Session.Username]
	if u == nil {
		return nil
	}
	return u.Roles
}

// Authorized - This function is the one place a Command or
// CommandLevel's own AllowedRoles list is actually checked, called by
// main.go's runLoop for Command.AllowedRoles and by EnterCommandLevel
// for CommandLevel.AllowedRoles, both at the same point
// EffectivePasswordHash is already checked. An empty allowedRoles
// means this particular command or level carries no role gate at
// all, and this always returns true, which is what keeps every
// existing tree file, none of which sets AllowedRoles, working
// completely unchanged.
//
// AllowedRoles is also never enforced while ctx.AuthRequired is
// false, regardless of what allowedRoles actually contains, and this
// returns true immediately in that case too. This project exists as
// a library first, meant to be picked up with nothing configured yet
// and produce a genuinely working, wide open command line, not one
// that quietly locks a project builder out of a level they just
// declared in their own tree file. AuthRequired off means no session
// anywhere has a real identity at all, see CurrentUserRoles, so there
// is no meaningful "wrong role" to refuse in the first place, only
// "nobody has ever logged in here yet," the correct state for
// everything to stay reachable. A role gate only starts to mean
// anything the moment a deployment, its own vendor, or its very
// first administrator, turns AuthRequired on, exactly the point real
// identity starts to exist for any session to hold a role against.
//
// Otherwise, a session is authorized when the currently logged in
// user, see CurrentUserRoles, holds ctx.Roles's own bypass role, if
// one is declared, or holds any role at all that also appears in
// allowedRoles. A user with no roles, or none that overlap, is
// refused, deny by default, the same fail-closed convention
// PasswordHash already follows for a wrong or missing credential.
func Authorized(ctx *AppContext, allowedRoles []string) bool {
	if len(allowedRoles) == 0 {
		return true
	}
	if !ctx.AuthRequired {
		return true
	}
	bypass := ""
	if ctx.Roles != nil {
		bypass = ctx.Roles.BypassRole
	}
	for _, r := range CurrentUserRoles(ctx) {
		if bypass != "" && r == bypass {
			return true
		}
		for _, a := range allowedRoles {
			if r == a {
				return true
			}
		}
	}
	return false
}
