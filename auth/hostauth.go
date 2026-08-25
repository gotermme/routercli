// Copyright 2026 Bret Jordan, All rights reserved.
//
// Use of this source code is governed by an Apache 2.0 license
// that can be found in the LICENSE file in the root of the source tree.

package auth

import (
	"fmt"
	"time"

	osuser "os/user"
)

// ----------------------------------------------------------------------
// Public Functions - Host Authentication
// ----------------------------------------------------------------------

// SessionFromHostIdentity - This function builds an already
// authenticated Session directly from the operating system account
// routercli itself is running as, read through os/user.Current, with
// no password prompted for or checked at all. This exists for
// config.SystemConfig.EnableHostAuthentication, meant for a
// deployment reached over SSH where sshd already authenticated the
// underlying Unix account, whether routercli is installed as that
// account's own login shell or reached through a ForceCommand, before
// routercli ever started. Trusting that identity here is sound
// specifically because it is not client controlled: the operating
// system itself decided which account this process is running as,
// before this function, or any other part of routercli, ever ran.
//
// The returned Session carries the same account name on both Username
// and HostUsername, and the current instant on HostConnectedAt. A
// caller that also runs its own CLI login on top of this, see
// main.go's establishSession for when EnableCLIAuthentication is also
// on, is expected to overwrite Username with whatever that login
// resolves to while keeping HostUsername and HostConnectedAt as they
// are here, since those two describe how the connection arrived, not
// who is now identified as using it.
func SessionFromHostIdentity() (*Session, error) {
	u, err := osuser.Current()
	if err != nil {
		return nil, fmt.Errorf("error reading host account identity: %v", err)
	}

	now := time.Now()
	return &Session{
		Username:        u.Username,
		Authenticated:   true,
		HostUsername:    u.Username,
		HostConnectedAt: now,
	}, nil
}
