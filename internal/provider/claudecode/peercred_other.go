//go:build !linux && !darwin

package claudecode

import (
	"log/slog"
	"net"
	"sync"
)

var warnUnsupportedPlatform sync.Once

// verifyPeer always returns true on unsupported platforms. Peer credential
// verification requires OS support (SO_PEERCRED / LOCAL_PEERCRED) which is
// only available on Linux and macOS. A warning is logged once to make this
// degraded security model visible.
func verifyPeer(_ *net.UnixConn) bool {
	warnUnsupportedPlatform.Do(func() {
		slog.Warn("aitop: peer credential verification is not supported on this platform — " +
			"socket access is restricted by filesystem permissions only")
	})
	return true
}
