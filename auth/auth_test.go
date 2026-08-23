// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auth

import "testing"

// TestHashAndVerifyRoundTrip - This test verifies that a password hashed with
// HashPassword is accepted by VerifyPassword when correct and
// rejected when wrong.
func TestHashAndVerifyRoundTrip(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if !VerifyPassword(hash, "correct horse battery staple") {
		t.Error("VerifyPassword should accept the correct password")
	}
	if VerifyPassword(hash, "wrong password") {
		t.Error("VerifyPassword should reject an incorrect password")
	}
}

// TestHashPasswordNeverProducesPlaintextID - This test verifies that HashPassword
// always produces a bcrypt ("$6$") hash, never the plaintext ("$0$")
// form VerifyPassword also understands.
func TestHashPasswordNeverProducesPlaintextID(t *testing.T) {
	hash, err := HashPassword("test")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if len(hash) < 3 || hash[:3] != "$"+cryptIDBcrypt+"$" {
		t.Errorf("HashPassword output %q does not start with the bcrypt ID prefix", hash)
	}
}

// TestVerifyPasswordPlaintext - This test verifies that VerifyPassword still
// recognizes a "$0$" plaintext entry, a development and testing
// convenience, even though HashPassword itself never produces one.
func TestVerifyPasswordPlaintext(t *testing.T) {
	if !VerifyPassword("$0$hunter2", "hunter2") {
		t.Error("plaintext ($0$) verification should accept a matching password")
	}
	if VerifyPassword("$0$hunter2", "wrong") {
		t.Error("plaintext ($0$) verification should reject a non-matching password")
	}
}

// TestVerifyPasswordMalformedOrUnknownID - This test verifies that a stored value
// that is not a well-formed "$id$encoded" string, or that names an
// unrecognized id, fails verification instead of erroring or matching
// by accident.
func TestVerifyPasswordMalformedOrUnknownID(t *testing.T) {
	cases := []string{
		"not-even-dollar-prefixed",
		"$",
		"$6",
		"$99$something",
	}
	for _, stored := range cases {
		if VerifyPassword(stored, "anything") {
			t.Errorf("VerifyPassword(%q, ...) should fail closed, returned true", stored)
		}
	}
}

// TestIsPlaintextHash - This test verifies that IsPlaintextHash recognizes a
// "$0$" stored value as plaintext, reports false for a real bcrypt
// hash, and fails closed, reporting false rather than erroring, for a
// value that does not parse as a "$id$encoded" string at all.
func TestIsPlaintextHash(t *testing.T) {
	if !IsPlaintextHash("$0$hunter2") {
		t.Error("expected a $0$ stored value to be reported as plaintext")
	}
	hash, err := HashPassword("hunter2")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if IsPlaintextHash(hash) {
		t.Error("expected a real bcrypt hash to not be reported as plaintext")
	}
	if IsPlaintextHash("not-even-dollar-prefixed") {
		t.Error("expected a malformed stored value to not be reported as plaintext")
	}
}
