// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package daemon

import (
	"github.com/gotermme/routercli/auth"
	"github.com/gotermme/routercli/command"
	"github.com/gotermme/routercli/config"
)

// State is the RouterCLI daemon's own canonical, shared state, the
// concrete type a real deployment's own *Store[State] holds. Every
// field here is something claude/DAEMON_ARCHITECTURE_DESIGN.md names
// directly as genuinely shared, moving out of
// command.AppContext.State and command.AppContext.Levels and into
// this one place instead; see that document's own "What is genuinely
// shared, and what stays per session" section. Nothing genuinely per
// session, a Session, a Position, a Negated flag, belongs here at
// all; those stay on each CLI session's own AppContext, entirely
// outside this package, exactly as that same section describes.
//
// A zero State is not generally useful on its own; construct one with
// NewState. State itself is an ordinary Go struct with no locking of
// its own, by design: every field here is only ever safe to read or
// write from inside a function passed to a Store's own Do call, never
// directly, the same way this whole package's own single writer
// design intends. Nothing in this package enforces that from the
// type system alone, the same way sync.Mutex does not stop a caller
// from touching what it protects without holding it first; this is a
// documented convention this package's own tests hold to, not a
// compiler guarantee.
type State struct {
	// ProductState holds whatever a deployment's own product package
	// considers its running configuration, hostname, interfaces,
	// description, banners, line defaults, and so on for this
	// project's own cmd/product, kept fully opaque here, an any,
	// exactly matching command.AppContext.State's own existing
	// genericity. This package never reads or writes through it
	// itself; wiring a concrete product state type up to real
	// mutations is later phase work, not this package's own concern.
	ProductState any

	// Levels holds every Command Level's own definition, including
	// each level's own runtime defined Aliases map and its own
	// PasswordHash/VendorDefinedPasswordHash, moved here from
	// command.AppContext.Levels. A nil Levels is not valid state for
	// a real deployment, the same way a nil *command.TreeStructure is
	// never valid on AppContext today, but NewState does not itself
	// refuse a nil value, leaving that validation to whatever loads a
	// real deployment's own tree structure before constructing a
	// Store around this State.
	Levels *command.TreeStructure

	// Users holds every account this deployment knows about, moved
	// here from what each CLI process today loads independently from
	// UsersFile at its own startup.
	Users auth.Users

	// Roles holds this deployment's own role declarations, moved here
	// from what each CLI process today loads independently from
	// RolesFile at its own startup.
	Roles *command.RoleSet

	// Config holds every other deployment wide setting this daemon
	// loaded once, at its own startup, from etc/routercli.yaml,
	// AuthRequired among them, handed to every attaching session from
	// here rather than each CLI process reading routercli.yaml for
	// itself; see claude/DAEMON_ARCHITECTURE_DESIGN.md's own "What is
	// genuinely shared" section for the reasoning. A daemon deployment
	// is expected to treat this as read only after startup, replacing
	// it wholesale on reboot rather than mutating individual fields in
	// place, the same "one function producing one canonical text"
	// discipline this project already applies to running-config
	// itself.
	Config *config.SystemConfig
}

// NewState returns a State built from the four pieces of already
// loaded state a real daemon startup, or a test, assembles separately:
// whatever product state a deployment's own product package produced,
// an already loaded tree structure, an already loaded set of users,
// and an already loaded role set, together with the already loaded
// system configuration. NewState does no loading of its own, and does
// no validation beyond what its own callers already performed loading
// each piece; it exists only so constructing a State reads as one
// clear step, at one call site, rather than four separate field
// assignments repeated at every place a State gets built.
func NewState(productState any, levels *command.TreeStructure, users auth.Users, roles *command.RoleSet, cfg *config.SystemConfig) State {
	return State{
		ProductState: productState,
		Levels:       levels,
		Users:        users,
		Roles:        roles,
		Config:       cfg,
	}
}
