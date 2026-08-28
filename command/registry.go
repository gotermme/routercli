// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package command

import (
	"fmt"
)

// registry - This variable holds every handler that has self-
// registered through Register(), keyed by the same string a command
// tree YAML file uses in its "run" directive. This is package-level
// rather than passed around, since an init() function, where Register
// is called from, has no other way to reach a constructor-supplied
// instance. This is the same pattern database/sql drivers and
// net/http/pprof handlers use.
var registry = map[string]HandlerFunc{}

// ----------------------------------------------------------------------
// Public Functions - Registry
// ----------------------------------------------------------------------

// Register - This function lets a command's own file in cmd/core or
// cmd/product make itself known to the command tree loader from its
// init(), without anyone having to edit this file or main.go to add a
// new command. name is the string a tree YAML file's "run" directive
// references to wire this handler into a specific place in the
// command tree.
//
// Registering the same name twice is a programming error, most likely
// two command files both claiming to be "show.version" by mistake,
// and panics immediately at program startup rather than silently
// letting the second registration win.
func Register(name string, fn HandlerFunc) {
	if _, exists := registry[name]; exists {
		panic(fmt.Sprintf("routercli: command handler %q is already registered, check for a duplicate Register() call", name))
	}
	registry[name] = fn
}

// ----------------------------------------------------------------------
// Private Functions - Registry
// ----------------------------------------------------------------------

// lookupHandler - This function looks up a registered handler by
// name. It is used by the tree loader when resolving a "run"
// directive from a tree YAML file into an actual HandlerFunc.
func lookupHandler(name string) (HandlerFunc, bool) {
	fn, ok := registry[name]
	return fn, ok
}
