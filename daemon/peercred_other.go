// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

//go:build !linux

package daemon

import (
	"fmt"
	"net"
	"runtime"
)

// checkPeerCredential has no implementation on this platform yet.
// claude/DAEMON_ARCHITECTURE_DESIGN.md's own "Transport" section names
// LOCAL_PEERCRED as the BSD family equivalent of Linux's SO_PEERCRED,
// see peercred_linux.go, but that mechanism has not been built and
// verified here; this package would rather refuse outright, on every
// platform other than Linux, than silently accept a connection this
// package cannot actually check the peer credential of, the same fail
// loudly convention this project applies everywhere else. Listen
// itself still succeeds on any platform Go supports a Unix domain
// socket on; only Accept, by way of this function, refuses every
// connection until a real implementation lands here.
func checkPeerCredential(conn *net.UnixConn) (PeerCredential, error) {
	return PeerCredential{}, fmt.Errorf("daemon: peer credential checking is not yet implemented on %s, see peercred_linux.go for the only platform currently supported", runtime.GOOS)
}
