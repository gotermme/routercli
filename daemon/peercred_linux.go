// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

//go:build linux

package daemon

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// checkPeerCredential reads conn's own peer credential from the
// kernel, SO_PEERCRED, the Linux mechanism for a process on one end
// of a Unix domain socket to learn the real UID, GID, and PID of
// whatever process is on the other end, set by the kernel itself at
// connect time and not something the connecting process can forge by
// sending different values over the wire; nothing is read from conn's
// own application data to produce this, which is exactly why this
// check can, and must, run before a single byte of any protocol above
// it is trusted. See claude/DAEMON_ARCHITECTURE_DESIGN.md's own
// "Transport" section.
func checkPeerCredential(conn *net.UnixConn) (PeerCredential, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return PeerCredential{}, fmt.Errorf("daemon: obtaining a raw connection to read its peer credential: %w", err)
	}

	var cred *unix.Ucred
	var sockErr error
	ctrlErr := raw.Control(func(fd uintptr) {
		cred, sockErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if ctrlErr != nil {
		return PeerCredential{}, fmt.Errorf("daemon: reading SO_PEERCRED: %w", ctrlErr)
	}
	if sockErr != nil {
		return PeerCredential{}, fmt.Errorf("daemon: reading SO_PEERCRED: %w", sockErr)
	}

	return PeerCredential{UID: cred.Uid, GID: cred.Gid, PID: cred.Pid}, nil
}
