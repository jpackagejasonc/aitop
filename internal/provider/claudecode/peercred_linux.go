//go:build linux

package claudecode

import (
	"net"
	"os"

	"golang.org/x/sys/unix"
)

// verifyPeer returns true if the connecting process is owned by the current user.
func verifyPeer(conn *net.UnixConn) bool {
	rc, err := conn.SyscallConn()
	if err != nil {
		return false
	}
	var uid uint32
	var ok bool
	_ = rc.Control(func(fd uintptr) {
		cred, err := unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
		if err == nil {
			uid = cred.Uid
			ok = true
		}
	})
	return ok && int(uid) == os.Getuid()
}
