// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package armorchan

import (
	"crypto/aes"
	"crypto/cipher"
	"fmt"
)

// newAEAD builds the one authenticated cipher this package ever uses,
// AES-256-GCM by way of crypto/aes and crypto/cipher's own standard
// GCM construction, from a 32 byte key. Every derived key this package
// produces is exactly 32 bytes, see deriveKeys, so this is the only
// place a key size is assumed rather than checked, and even here
// cipher.NewGCM would itself refuse a key of the wrong size through
// aes.NewCipher failing first.
func newAEAD(key [32]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("armorchan: constructing AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("armorchan: constructing GCM: %w", err)
	}
	return aead, nil
}
