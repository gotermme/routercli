// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

/*
Package armorchan implements the encrypted, authenticated channel the
RouterCLI daemon and its CLI clients speak over a local Unix domain
socket, described in full in claude/DAEMON_ARCHITECTURE_DESIGN.md.
Nothing in this package is a Cisco or HP concept; it is a small, general
purpose transport, reusable anywhere two local processes need a
server-authenticated, encrypted, ordered channel above a raw
connection.

This package depends on nothing beyond the Go standard library.
Key exchange uses crypto/ecdh, Curve25519, X25519. Key derivation uses
crypto/hkdf, standard library since Go 1.24, RFC 5869. Encryption uses
crypto/aes and crypto/cipher, AES-256-GCM. The handshake choreography
that ties those primitives together is this package's own code, built
deliberately rather than adopted from a third party protocol framework,
modeled structurally on TLS 1.3's own key schedule, RFC 9846, which
obsoletes the original TLS 1.3 specification, RFC 8446, with a minor,
backward compatible update that keeps the same key schedule shape this
package leans on.

A hand rolled protocol, even one built from sound, standard library
primitives and a standards derived shape, carries more residual risk
than a widely used, independently audited library, simply because fewer
eyes have looked at this specific arrangement of those primitives
before it ships. This package answers that with weight of testing: see
armorchan_test.go, channel_test.go, and kat_test.go for the known
answer, tamper, replay, and nonce reuse tests this carries as a direct
result. claude/DAEMON_ARCHITECTURE_DESIGN.md recommends an outside
review of this package specifically, by someone other than whoever
wrote it, before RouterCLI relies on it in a real deployment.

# The handshake

The daemon holds one persisted static X25519 key pair. Its public half
is distributed to CLI clients out of band, most simply a world readable
file the daemon writes at its own startup; this package accepts that
public key as a parameter rather than reading any file itself, keeping
key distribution a concern for the code that wires this package into a
real socket. Each connecting CLI client generates a fresh ephemeral
X25519 key pair for that one connection alone. The client's own
identity does not need proving at this layer; on the real Unix domain
socket this package is meant to run above, SO_PEERCRED already
established which local user is connecting before a single byte of
this handshake is exchanged, so authenticating the daemon to the client
is the one property this handshake actually needs to provide.

The four messages are: the client sends its ephemeral public key; the
daemon derives a shared secret from its own static private key and
that ephemeral public key, folds it through HKDF bound to a transcript
hash covering both public keys and a fixed protocol label, and sends
back its own static public key alongside a confirmation record
encrypted under a key derived from that same secret; the client checks
the advertised static public key against the one it already holds and
attempts to decrypt the confirmation record with its own independently
derived key. Successful decryption is the proof the client needed:
nothing else on the local machine could have produced a record
decryptable under a key derived from the private key matching the
daemon's own known static public key. See ServerHandshake and
ClientHandshake.

# The channel

A successful handshake produces a *Channel, holding two independent
AES-256-GCM ciphers, one per direction, and one independent nonce
counter per direction. Every nonce is derived from a per-direction
base value XORed with a monotonically increasing counter, exactly the
construction TLS 1.3's own record protection uses, RFC 9846 Section
5.3, never reused, never randomly generated. The same counter value is
also bound into each record's own additional authenticated data, so a
captured record replayed later in the same connection is rejected by
its own authentication tag failing against the receiver's own,
already-advanced counter, rather than silently accepted a second time.
Any authentication failure, a tampered record, a replayed record, or
anything else that fails to decrypt, permanently faults the Channel;
every RouterCLI daemon and client using this package MUST treat a
faulted Channel as fatal to that connection and close it, matching how
this project already treats any other unrecoverable session ending
event. See Channel, Channel.Send, and Channel.Receive.
*/
package armorchan
