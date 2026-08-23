// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auth

// ----------------------------------------------------------------------
// Public Methods - Session
// ----------------------------------------------------------------------

// AtLevel - This method reports whether the session's current Command
// Level is exactly name. This is deliberately a comparison against an
// explicit name rather than a plain "Elevated" bool, since a tree can
// have more than one Command Level reachable from the base. Once
// there is more than one non-base level to elevate into, "is this
// session elevated" is no longer a yes-or-no question, while "is this
// session at this specific level" always has an unambiguous answer.
// buildPrompt's prompt suffix and the elevation timeout auto-revert
// in main.go's runLoop both call this against the base level's own
// Name.
func (s *Session) AtLevel(name string) bool {
	return s.CommandLevel == name
}
