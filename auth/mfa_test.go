// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auth

import (
	"testing"
	"time"
)

// TestSecondFactorRequiredTrueWithTOTPSecret - This test verifies that a user with
// TOTPSecret set is reported as needing a second factor at login.
func TestSecondFactorRequiredTrueWithTOTPSecret(t *testing.T) {
	u := &User{Username: "alice", PasswordHash: "$0$x", TOTPSecret: "JBSWY3DPEHPK3PXP"}
	if !SecondFactorRequired(u) {
		t.Error("expected SecondFactorRequired to be true when TOTPSecret is set")
	}
}

// TestSecondFactorRequiredFalseWithoutTOTPSecret - This test verifies that a user
// with no TOTPSecret is reported as not needing a second factor.
func TestSecondFactorRequiredFalseWithoutTOTPSecret(t *testing.T) {
	u := &User{Username: "bob", PasswordHash: "$0$x"}
	if SecondFactorRequired(u) {
		t.Error("expected SecondFactorRequired to be false when no second factor is configured")
	}
}

// TestVerifySecondFactorCodeAcceptsValidTOTPCode - This test verifies
// that VerifySecondFactorCode returns true for a code freshly
// generated against a user's own TOTPSecret at the same instant it is
// checked against.
func TestVerifySecondFactorCodeAcceptsValidTOTPCode(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret returned error: %v", err)
	}
	u := &User{Username: "alice", PasswordHash: "$0$x", TOTPSecret: secret}
	now := time.Now()
	code, err := GenerateTOTPCode(secret, now)
	if err != nil {
		t.Fatalf("GenerateTOTPCode returned error: %v", err)
	}

	if !VerifySecondFactorCode(u, code, now) {
		t.Error("expected VerifySecondFactorCode to accept a freshly generated, valid TOTP code")
	}
}

// TestVerifySecondFactorCodeRejectsWrongTOTPCode - This test verifies
// that VerifySecondFactorCode returns false for a code generated well
// outside the verification skew window, rather than a fixed wrong
// code that could, astronomically unlikely but not impossible,
// actually be correct, the same technique cmd_totp_test.go's own TOTP
// tests use.
func TestVerifySecondFactorCodeRejectsWrongTOTPCode(t *testing.T) {
	secret, _ := GenerateTOTPSecret()
	u := &User{Username: "alice", PasswordHash: "$0$x", TOTPSecret: secret}
	now := time.Now()
	wrongCode, _ := GenerateTOTPCode(secret, now.Add(-10*time.Minute))

	if VerifySecondFactorCode(u, wrongCode, now) {
		t.Error("expected VerifySecondFactorCode to reject a code generated well outside the skew window")
	}
}

// TestVerifySecondFactorCodeFalseWithoutTOTPSecret - This test
// verifies VerifySecondFactorCode's own documented contract: it
// returns false, never true, for a user with no second factor
// configured at all, regardless of what code is supplied, mirroring
// VerifySecondFactor's contract for the same case.
func TestVerifySecondFactorCodeFalseWithoutTOTPSecret(t *testing.T) {
	u := &User{Username: "bob", PasswordHash: "$0$x"}
	if VerifySecondFactorCode(u, "123456", time.Now()) {
		t.Error("expected VerifySecondFactorCode to return false for a user with no second factor configured")
	}
}

// TestVerifySecondFactorCodeFalseForEmptyCode - This test verifies
// that an empty code, the state a caller such as
// cmd_password.go's runPasswordChange is in when a second factor is
// required but nothing was actually read, is rejected rather than
// somehow matching.
func TestVerifySecondFactorCodeFalseForEmptyCode(t *testing.T) {
	secret, _ := GenerateTOTPSecret()
	u := &User{Username: "alice", PasswordHash: "$0$x", TOTPSecret: secret}
	if VerifySecondFactorCode(u, "", time.Now()) {
		t.Error("expected VerifySecondFactorCode to reject an empty code")
	}
}
