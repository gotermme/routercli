// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auth

import (
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// ----------------------------------------------------------------------
// Define Storage Format
// ----------------------------------------------------------------------

// Password hashes are stored as "$<id>$<encoded>". The crypt ID
// convention used here is 0 for plaintext and 6 for a real hash.
const (
	// cryptIDPlaintext - This constant stores the password with no
	// hashing at all. This exists only for local development and
	// testing convenience, and must never be used for a real
	// deployment. HashPassword never produces this. Only
	// VerifyPassword understands it.
	cryptIDPlaintext = "0"

	// cryptIDBcrypt - This constant states that the bcrypt algorithm
	// should be used, not something like HMAC-SHA512. HMAC-SHA512 is
	// a general-purpose hash with no work factor, so it can be cheap
	// to brute force at scale on modern hardware, which is exactly
	// wrong for password storage. bcrypt has a tunable cost and is
	// the standard choice for this kind of work. The salt is not
	// stored separately, since bcrypt's own output already embeds it.
	cryptIDBcrypt = "6"

	// bcryptCost - This constant is the bcrypt work factor, broken
	// out as a named constant so it can be adjusted over time.
	bcryptCost = bcrypt.DefaultCost
)

// ----------------------------------------------------------------------
// Public Functions - Auth
// ----------------------------------------------------------------------

// HashPassword - This function hashes a plaintext password with
// bcrypt and returns it in the "$6$<encoded>" storage format used in
// etc/users.yaml.
func HashPassword(plaintext string) (string, error) {
	encoded, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("error hashing password: %v", err)
	}
	return "$" + cryptIDBcrypt + "$" + string(encoded), nil
}

// IsPlaintextHash - This function reports whether the stored password
// is in the plaintext, "$0$...", storage format rather than a real
// hash. It returns false for anything that does not parse as a
// "$id$encoded" string.
func IsPlaintextHash(stored string) bool {
	id, _, ok := splitPasswordString(stored)
	return ok && id == cryptIDPlaintext
}

// VerifyPassword - This function checks a plaintext candidate against
// a stored "$id$encoded" hash, dispatching on id. An unrecognized id
// is treated as a verification failure rather than an error, since a
// corrupt or tampered users.yaml entry should deny access, not crash
// the process or, worse, silently let something through.
func VerifyPassword(stored, candidate string) bool {
	id, encoded, ok := splitPasswordString(stored)
	if !ok {
		return false
	}

	switch id {
	case cryptIDPlaintext:
		return encoded == candidate
	case cryptIDBcrypt:
		return bcrypt.CompareHashAndPassword([]byte(encoded), []byte(candidate)) == nil
	default:
		return false
	}
}

// ----------------------------------------------------------------------
// Private Functions - Auth
// ----------------------------------------------------------------------

// splitPasswordString - This function splits a "$id$encoded" string
// into its two parts.
func splitPasswordString(stored string) (id, encoded string, ok bool) {
	if !strings.HasPrefix(stored, "$") {
		return "", "", false
	}
	parts := strings.SplitN(stored[1:], "$", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}
