// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auth

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ----------------------------------------------------------------------
// Public Methods - LocalAuthProvider
// ----------------------------------------------------------------------

// Authenticate - This method implements AuthProvider for
// LocalAuthProvider. A nonexistent username still runs a real bcrypt
// comparison against dummyBcryptHash before returning, the same
// timing side channel defense VerifyLogin has always performed, now
// living here since this is the one place that actually knows whether
// a username exists in this backend's own Users map. See
// dummyBcryptHash's own doc comment in login.go.
func (p *LocalAuthProvider) Authenticate(username, password string) (bool, error) {
	u, ok := p.Users[username]
	if !ok {
		bcrypt.CompareHashAndPassword([]byte(dummyBcryptHash), []byte(password))
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
