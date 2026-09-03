// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package armorchan

import (
	"crypto/cipher"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"
)

// ErrChannelFaulted is returned by Send or Receive once a Channel has
// already failed once, from either direction. A Channel that has ever
// produced this error MUST NOT be used again; see the package doc
// comment for why every caller treats a faulted Channel as fatal to
// the underlying connection, closing it rather than continuing to use
// it or attempting to reset it.
var ErrChannelFaulted = errors.New("armorchan: channel already faulted, close the connection")

// ErrNonceSpaceExhausted is returned by Send if a single Channel's own
// one direction has already sent the maximum number of records this
// package allows a single set of derived keys to protect. Reaching
// this in practice would require sending on the order of 2^64 records
// over one connection, never a realistic count for a CLI session, but
// this package checks for it anyway rather than silently wrapping a
// nonce counter, which would reuse a nonce under the same key, exactly
// the failure this package's whole nonce discipline exists to prevent.
var ErrNonceSpaceExhausted = errors.New("armorchan: nonce space exhausted for this direction, reconnect required")

// Channel is a live, encrypted, authenticated, ordered channel between
// a RouterCLI daemon and one connected CLI client, established by
// ServerHandshake or ClientHandshake; see the package doc comment for
// the handshake itself and for what a Channel guarantees. A Channel
// wraps a raw connection, an io.ReadWriter, and takes over its own
// framing from that point on; nothing else should read from or write
// to that same connection once a Channel exists for it.
//
// A Channel's own two directions use independent keys and independent
// nonce counters, so one goroutine may call Send while a different
// goroutine calls Receive at the same time. Calling Send from more
// than one goroutine at once, or Receive from more than one goroutine
// at once, is not safe; each direction has exactly one caller in this
// project's own design, one goroutine reading whatever the daemon or
// the CLI sends next, at most one call to Send in flight for whatever
// this side needs to say back, so this Channel does not add its own
// internal serialization beyond what each direction's own mutex
// provides for its own nonce counter.
type Channel struct {
	conn io.ReadWriter

	sendMu     sync.Mutex
	sendAEAD   cipher.AEAD
	sendBase   [nonceLength]byte
	sendCount  uint64
	sendFailed bool

	recvMu     sync.Mutex
	recvAEAD   cipher.AEAD
	recvBase   [nonceLength]byte
	recvCount  uint64
	recvFailed bool
}

// Send encrypts plaintext under this Channel's own send direction key
// and writes it as one frame to the underlying connection. Once Send
// returns a non-nil error, this Channel's send direction has faulted;
// every later call to Send returns ErrChannelFaulted without touching
// the connection again, and the caller MUST close the connection
// rather than continuing to use this Channel.
func (c *Channel) Send(plaintext []byte) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()

	if c.sendFailed {
		return ErrChannelFaulted
	}

	if c.sendCount == math.MaxUint64 {
		c.sendFailed = true
		return ErrNonceSpaceExhausted
	}

	nonce := deriveNonce(c.sendBase, c.sendCount)
	aad := sequenceAAD(c.sendCount)
	ciphertext := c.sendAEAD.Seal(nil, nonce[:], plaintext, aad[:])

	if err := writeFrame(c.conn, ciphertext); err != nil {
		c.sendFailed = true
		return fmt.Errorf("armorchan: send: %w", err)
	}
	c.sendCount++
	return nil
}

// Receive reads one frame from the underlying connection and decrypts
// it under this Channel's own receive direction key, using the exact
// nonce and additional authenticated data this Channel's own receive
// counter currently expects next, never anything read off the wire. A
// record encrypted under a different counter value than the one this
// Channel currently expects, whether tampered, corrupted, or a
// previously seen record replayed back, fails its own authentication
// tag check here and is rejected outright; Receive never advances its
// own counter, and never accepts a record, unless decryption actually
// succeeded. Once Receive returns a non-nil error, this Channel's
// receive direction has faulted; every later call to Receive returns
// ErrChannelFaulted without touching the connection again, and the
// caller MUST close the connection rather than continuing to use this
// Channel.
func (c *Channel) Receive() ([]byte, error) {
	c.recvMu.Lock()
	defer c.recvMu.Unlock()

	if c.recvFailed {
		return nil, ErrChannelFaulted
	}

	if c.recvCount == math.MaxUint64 {
		c.recvFailed = true
		return nil, ErrNonceSpaceExhausted
	}

	ciphertext, err := readFrame(c.conn)
	if err != nil {
		c.recvFailed = true
		return nil, fmt.Errorf("armorchan: receive: %w", err)
	}

	nonce := deriveNonce(c.recvBase, c.recvCount)
	aad := sequenceAAD(c.recvCount)
	plaintext, err := c.recvAEAD.Open(nil, nonce[:], ciphertext, aad[:])
	if err != nil {
		c.recvFailed = true
		return nil, fmt.Errorf("armorchan: receive: record failed authentication, tampered, corrupted, or replayed: %w", err)
	}
	c.recvCount++
	return plaintext, nil
}

// nonceLength is the nonce size crypto/cipher's standard GCM
// construction expects, 96 bits, the same size TLS 1.3's own record
// nonce uses, RFC 9846 Section 5.3.
const nonceLength = 12

// deriveNonce reproduces TLS 1.3's own per record nonce construction,
// RFC 9846 Section 5.3: a fixed per direction base value, established
// once at handshake time and never sent on the wire again, with its
// low order 64 bits XORed against a big endian encoding of this
// record's own sequence number. Neither side ever transmits a nonce or
// a sequence number; both sides derive the exact same value only
// because both are counting records in the same direction in the same
// strict order, which is exactly the property that makes a captured
// and replayed record fail authentication against whatever the
// receiver's own counter has already moved on to.
func deriveNonce(base [nonceLength]byte, counter uint64) [nonceLength]byte {
	var nonce [nonceLength]byte
	copy(nonce[:], base[:])
	var counterBytes [8]byte
	binary.BigEndian.PutUint64(counterBytes[:], counter)
	for i := 0; i < 8; i++ {
		nonce[4+i] ^= counterBytes[i]
	}
	return nonce
}

// sequenceAAD returns this record's own sequence number, as an eight
// byte big endian value, for use as the additional authenticated data
// passed to this record's own AEAD call. Binding the same counter
// value used to derive the nonce into the record's own authenticated
// data as well is deliberate belt and suspenders: replay protection
// here does not depend on nonce derivation alone, so a future change
// to how a nonce is derived could not silently reopen a replay window
// without this second, independent check also needing to change.
func sequenceAAD(counter uint64) [8]byte {
	var aad [8]byte
	binary.BigEndian.PutUint64(aad[:], counter)
	return aad
}
