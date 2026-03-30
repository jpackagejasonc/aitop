//go:build darwin

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
		// SOL_LOCAL = 0 on macOS; LOCAL_PEERCRED = 0x001
		xucred, err := unix.GetsockoptXucred(int(fd), 0, unix.LOCAL_PEERCRED)
		if err == nil {
			uid = xucred.Uid
			ok = true
		}
	})
	return ok && int(uid) == os.Getuid()
}
