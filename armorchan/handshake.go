// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package armorchan

import (
	"bytes"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// ErrHandshakeFailed is returned by ClientHandshake when the peer at
// the other end of the connection could not be authenticated as the
// genuine holder of the expected static private key, either because
// it advertised a different static public key than the one the caller
// already holds, or because it failed to produce a confirmation record
// this Channel's own derived keys could decrypt. Either case means the
// same thing in practice: something other than the genuine daemon
// answered on the socket, and no Channel is returned.
var ErrHandshakeFailed = errors.New("armorchan: handshake failed, peer is not the expected daemon")

// curve is the one elliptic curve this package ever uses, Curve25519
// by way of crypto/ecdh's own X25519 implementation, chosen here once
// so ServerHandshake, ClientHandshake, and GenerateStaticKeyPair never
// each pick it separately.
func curve() ecdh.Curve {
	return ecdh.X25519()
}

// GenerateStaticKeyPair returns a fresh X25519 private key, suitable
// for a RouterCLI daemon's own persisted static identity, passed to
// ServerHandshake as its own static private key on every connection
// this daemon accepts for as long as that key stays current. Reading
// or writing this key to disk, and distributing its public half to
// connecting clients, are both concerns for whatever code wires this
// package into a real daemon, not for this package itself; see the
// package doc comment.
func GenerateStaticKeyPair() (*ecdh.PrivateKey, error) {
	key, err := curve().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("armorchan: generating static key pair: %w", err)
	}
	return key, nil
}

// protocolLabel is folded into this handshake's own transcript hash
// and into every HKDF info string this package derives a key or nonce
// base from, so a future, incompatible version of this same protocol
// can change this one constant and never risk a byte sequence from one
// version being mistaken for a valid message under the other.
const protocolLabel = "RouterCLI armorchan v1"

// derivedKeys holds every value this handshake's own HKDF step
// produces, one call to deriveKeys computing all five as an atomic
// group so ServerHandshake and ClientHandshake can never accidentally
// derive some of these from one transcript and the rest from another.
type derivedKeys struct {
	daemonToClientKey       [32]byte
	daemonToClientNonceBase [nonceLength]byte
	clientToDaemonKey       [32]byte
	clientToDaemonNonceBase [nonceLength]byte
	confirmationKey         [32]byte
}

// deriveKeys implements this package's own key schedule: an HKDF
// extract and expand, RFC 5869, over the ECDH shared secret, salted
// with a transcript hash covering both public keys this handshake
// exchanged and this package's own protocol label, expanded once per
// distinct purpose with a distinct info string, exactly the "one
// secret in, several independent, purpose bound keys out" shape TLS
// 1.3's own key schedule uses, RFC 9846 Section 7.1.
func deriveKeys(sharedSecret []byte, transcriptHash [sha256.Size]byte) (derivedKeys, error) {
	var out derivedKeys

	expand := func(info string, length int) ([]byte, error) {
		key, err := hkdf.Key(sha256.New, sharedSecret, transcriptHash[:], info, length)
		if err != nil {
			return nil, fmt.Errorf("armorchan: deriving %q: %w", info, err)
		}
		return key, nil
	}

	d2cKey, err := expand(protocolLabel+" daemon-to-client key", 32)
	if err != nil {
		return derivedKeys{}, err
	}
	copy(out.daemonToClientKey[:], d2cKey)

	d2cNonce, err := expand(protocolLabel+" daemon-to-client nonce", nonceLength)
	if err != nil {
		return derivedKeys{}, err
	}
	copy(out.daemonToClientNonceBase[:], d2cNonce)

	c2dKey, err := expand(protocolLabel+" client-to-daemon key", 32)
	if err != nil {
		return derivedKeys{}, err
	}
	copy(out.clientToDaemonKey[:], c2dKey)

	c2dNonce, err := expand(protocolLabel+" client-to-daemon nonce", nonceLength)
	if err != nil {
		return derivedKeys{}, err
	}
	copy(out.clientToDaemonNonceBase[:], c2dNonce)

	confirmKey, err := expand(protocolLabel+" server confirmation key", 32)
	if err != nil {
		return derivedKeys{}, err
	}
	copy(out.confirmationKey[:], confirmKey)

	return out, nil
}

// transcript computes this handshake's own transcript hash, the
// client's ephemeral public key followed by the daemon's static public
// key followed by this package's own protocol label, all fed through
// SHA-256 together. Both ServerHandshake and ClientHandshake compute
// this identically, from the same two public keys in the same order,
// which is exactly what lets deriveKeys produce identical output on
// both ends without either side ever transmitting a derived key
// itself.
func transcript(clientEphemeralPublic, daemonStaticPublic *ecdh.PublicKey) [sha256.Size]byte {
	h := sha256.New()
	h.Write(clientEphemeralPublic.Bytes())
	h.Write(daemonStaticPublic.Bytes())
	h.Write([]byte(protocolLabel))
	var sum [sha256.Size]byte
	copy(sum[:], h.Sum(nil))
	return sum
}

// confirmationPlaintext is the fixed message the daemon encrypts under
// its own derived confirmation key and the client decrypts to confirm
// it is genuinely talking to the holder of the expected static private
// key. Its own content carries no meaning beyond being fixed and known
// to both sides; the confirmation key is used exactly once, for this
// one record, under a fixed all-zero nonce, which is safe only because
// that key is never reused for anything else afterward.
var confirmationPlaintext = []byte(protocolLabel + " server confirmation")

// zeroNonce is the fixed nonce used for the one, single-use
// confirmation record; see confirmationPlaintext's own doc comment for
// why reusing an all-zero nonce is safe here specifically, and nowhere
// else in this package.
var zeroNonce [nonceLength]byte

// ServerHandshake runs this package's own handshake from the daemon's
// side of conn, using daemonStaticPrivate as this daemon's own
// persisted static identity, and returns a ready to use Channel once
// the client's own first message has been read and this daemon's own
// confirmation record has been sent. ServerHandshake does not itself
// authenticate the connecting client; on the real Unix domain socket
// this package is meant to run above, that already happened before
// ServerHandshake was ever called, an SO_PEERCRED check against the
// raw connection. See the package doc comment for the full handshake.
func ServerHandshake(conn io.ReadWriter, daemonStaticPrivate *ecdh.PrivateKey) (*Channel, error) {
	clientEphemeralPublicBytes, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("armorchan: server handshake: reading client ephemeral public key: %w", err)
	}
	clientEphemeralPublic, err := curve().NewPublicKey(clientEphemeralPublicBytes)
	if err != nil {
		return nil, fmt.Errorf("armorchan: server handshake: client ephemeral public key is not a valid X25519 key: %w", err)
	}

	sharedSecret, err := daemonStaticPrivate.ECDH(clientEphemeralPublic)
	if err != nil {
		return nil, fmt.Errorf("armorchan: server handshake: computing shared secret: %w", err)
	}

	daemonStaticPublic := daemonStaticPrivate.PublicKey()
	transcriptHash := transcript(clientEphemeralPublic, daemonStaticPublic)

	keys, err := deriveKeys(sharedSecret, transcriptHash)
	if err != nil {
		return nil, fmt.Errorf("armorchan: server handshake: %w", err)
	}

	confirmAEAD, err := newAEAD(keys.confirmationKey)
	if err != nil {
		return nil, fmt.Errorf("armorchan: server handshake: %w", err)
	}
	confirmationRecord := confirmAEAD.Seal(nil, zeroNonce[:], confirmationPlaintext, transcriptHash[:])

	response := append(append([]byte{}, daemonStaticPublic.Bytes()...), confirmationRecord...)
	if err := writeFrame(conn, response); err != nil {
		return nil, fmt.Errorf("armorchan: server handshake: sending confirmation: %w", err)
	}

	sendAEAD, err := newAEAD(keys.daemonToClientKey)
	if err != nil {
		return nil, fmt.Errorf("armorchan: server handshake: %w", err)
	}
	recvAEAD, err := newAEAD(keys.clientToDaemonKey)
	if err != nil {
		return nil, fmt.Errorf("armorchan: server handshake: %w", err)
	}

	return &Channel{
		conn:     conn,
		sendAEAD: sendAEAD,
		sendBase: keys.daemonToClientNonceBase,
		recvAEAD: recvAEAD,
		recvBase: keys.clientToDaemonNonceBase,
	}, nil
}

// ClientHandshake runs this package's own handshake from the CLI
// client's side of conn, generating a fresh ephemeral key pair for
// this one connection, and returns a ready to use Channel once the
// daemon's own confirmation record has been received and verified
// against expectedDaemonStaticPublic. ClientHandshake returns
// ErrHandshakeFailed, rather than any Channel, if the peer on the
// other end of conn ever fails to prove it holds the private key
// matching expectedDaemonStaticPublic; see the package doc comment for
// the full handshake and what this proof actually establishes.
func ClientHandshake(conn io.ReadWriter, expectedDaemonStaticPublic *ecdh.PublicKey) (*Channel, error) {
	clientEphemeralPrivate, err := curve().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("armorchan: client handshake: generating ephemeral key pair: %w", err)
	}
	clientEphemeralPublic := clientEphemeralPrivate.PublicKey()

	if err := writeFrame(conn, clientEphemeralPublic.Bytes()); err != nil {
		return nil, fmt.Errorf("armorchan: client handshake: sending ephemeral public key: %w", err)
	}

	response, err := readFrame(conn)
	if err != nil {
		return nil, fmt.Errorf("armorchan: client handshake: reading server response: %w", err)
	}
	staticKeyLength := len(expectedDaemonStaticPublic.Bytes())
	if len(response) < staticKeyLength {
		return nil, fmt.Errorf("armorchan: client handshake: server response shorter than one static public key")
	}
	advertisedStaticPublicBytes := response[:staticKeyLength]
	confirmationRecord := response[staticKeyLength:]

	if !bytes.Equal(advertisedStaticPublicBytes, expectedDaemonStaticPublic.Bytes()) {
		return nil, fmt.Errorf("%w: server advertised a different static public key than expected", ErrHandshakeFailed)
	}

	sharedSecret, err := clientEphemeralPrivate.ECDH(expectedDaemonStaticPublic)
	if err != nil {
		return nil, fmt.Errorf("armorchan: client handshake: computing shared secret: %w", err)
	}

	transcriptHash := transcript(clientEphemeralPublic, expectedDaemonStaticPublic)

	keys, err := deriveKeys(sharedSecret, transcriptHash)
	if err != nil {
		return nil, fmt.Errorf("armorchan: client handshake: %w", err)
	}

	confirmAEAD, err := newAEAD(keys.confirmationKey)
	if err != nil {
		return nil, fmt.Errorf("armorchan: client handshake: %w", err)
	}
	if _, err := confirmAEAD.Open(nil, zeroNonce[:], confirmationRecord, transcriptHash[:]); err != nil {
		return nil, fmt.Errorf("%w: could not decrypt server confirmation record: %v", ErrHandshakeFailed, err)
	}

	sendAEAD, err := newAEAD(keys.clientToDaemonKey)
	if err != nil {
		return nil, fmt.Errorf("armorchan: client handshake: %w", err)
	}
	recvAEAD, err := newAEAD(keys.daemonToClientKey)
	if err != nil {
		return nil, fmt.Errorf("armorchan: client handshake: %w", err)
	}

	return &Channel{
		conn:     conn,
		sendAEAD: sendAEAD,
		sendBase: keys.clientToDaemonNonceBase,
		recvAEAD: recvAEAD,
		recvBase: keys.daemonToClientNonceBase,
	}, nil
}
