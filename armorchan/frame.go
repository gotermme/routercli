// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package armorchan

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxFrameSize bounds every frame this package ever reads, both the raw
// handshake messages and every encrypted record afterward. The two
// processes on either end of this channel are already restricted to
// the daemon's own account, or another explicitly allowed one, by the
// SO_PEERCRED check this package assumes already happened before a
// Channel exists at all, but that is exactly the kind of single check
// this project's own layered thinking never treats as sufficient on
// its own. Without this bound, a four byte length prefix claiming
// close to four gigabytes would make readFrame allocate that much
// memory before ever finding out the rest of the frame does not
// actually exist, a cheap way for a compromised or simply buggy peer
// to exhaust memory. One mebibyte comfortably fits the largest message
// this protocol's own catalog needs, a full show running-config style
// payload included, with room to grow.
const MaxFrameSize = 1 << 20

// ErrFrameTooLarge is returned by readFrame when a peer's own length
// prefix claims a frame larger than MaxFrameSize, before any attempt
// is made to read or allocate that much data.
var ErrFrameTooLarge = errors.New("armorchan: frame exceeds maximum allowed size")

// writeFrame writes b to w as one length prefixed frame, a four byte
// big endian length followed by b itself, in a single call to w's own
// Write, one frame, one write. Every message this package ever sends,
// the raw handshake messages and every encrypted record alike, uses
// this same framing, so readFrame is the one place a peer ever needs
// to know how to find the next message's own boundary. Writing the
// length prefix and the body together as one call, rather than two,
// is also deliberate beyond only being tidy: it means one write to the
// underlying connection always corresponds to exactly one frame,
// which every test in this package that needs to observe, capture, or
// deliberately corrupt a specific frame relies on directly.
func writeFrame(w io.Writer, b []byte) error {
	if len(b) > MaxFrameSize {
		// This is this package's own bug if it is ever reached, not a
		// hostile peer's, since every caller inside this package
		// already keeps what it sends well under MaxFrameSize; it is
		// checked anyway rather than trusted, the same "fail loudly"
		// convention this project applies everywhere else.
		return fmt.Errorf("armorchan: refusing to send a %d byte frame, larger than MaxFrameSize", len(b))
	}
	framed := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(framed[:4], uint32(len(b)))
	copy(framed[4:], b)
	if _, err := w.Write(framed); err != nil {
		return fmt.Errorf("armorchan: writing frame: %w", err)
	}
	return nil
}

// readFrame reads one length prefixed frame from r, written by
// writeFrame, and returns its body. A length prefix claiming more than
// MaxFrameSize is refused outright, with ErrFrameTooLarge, before any
// allocation sized by that untrusted value is attempted; readFrame
// never allocates more than MaxFrameSize bytes on a single call, no
// matter what a malformed or adversarial length prefix claims.
func readFrame(r io.Reader) ([]byte, error) {
	var lengthPrefix [4]byte
	if _, err := io.ReadFull(r, lengthPrefix[:]); err != nil {
		return nil, fmt.Errorf("armorchan: reading frame length: %w", err)
	}
	length := binary.BigEndian.Uint32(lengthPrefix[:])
	if length > MaxFrameSize {
		return nil, ErrFrameTooLarge
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, fmt.Errorf("armorchan: reading frame body: %w", err)
	}
	return body, nil
}
