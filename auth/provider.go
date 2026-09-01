// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auth

import (
	"fmt"
)

// ----------------------------------------------------------------------
// Public Methods - LocalAuthProvider
// ----------------------------------------------------------------------

// Authenticate - This method implements AuthProvider for
// LocalAuthProvider. A nonexistent username still runs a real
// comparison, through whichever PasswordHasher is currently the
// default, see PasswordHasher.Dummy in auth.go, before returning, the
// same timing side channel defense VerifyLogin has always performed,
// now living here since this is the one place that actually knows
// whether a username exists in this backend's own Users map. Going
// through the default hasher rather than calling bcrypt directly is
// what keeps this defense correct even after a project calls
// SetDefaultPasswordHasher to move away from bcrypt entirely; a fixed
// bcrypt comparison here would burn the wrong amount of CPU time once
// real logins are being checked against a different algorithm, quietly
// reopening the exact timing side channel this exists to close.
func (p *LocalAuthProvider) Authenticate(username, password string) (bool, error) {
	u, ok := p.Users[username]
	if !ok {
		defaultPasswordHasher.Verify(defaultPasswordHasher.Dummy(), password)
		return false, nil
	}
	return VerifyPassword(u.PasswordHash, password), nil
}

// ----------------------------------------------------------------------
// Public Functions - Provider
// ----------------------------------------------------------------------

// NewAuthProvider - This function builds the AuthProvider a
// config.SystemConfig.AuthProviders entry's Type names, "local" being
// the only recognized value today. An unrecognized Type is an error
// rather than something silently ignored, the same fail loudly
// convention every other malformed setting in this project follows,
// since a typo'd Type would otherwise mean a deployment believes it
// is checking passwords against a backend that does not actually
// exist. users is only meaningful for the "local" Type; a future
// Type, an LDAP or a RADIUS backend for instance, would take whatever
// connection details its own config.AuthProviderConfig fields
// eventually carry instead.
func NewAuthProvider(providerType string, users Users) (AuthProvider, error) {
	switch providerType {
	case "local":
		return NewLocalAuthProvider(users), nil
	default:
		return nil, fmt.Errorf("unrecognized authentication provider type %q", providerType)
	}
}
