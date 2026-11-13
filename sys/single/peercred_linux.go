//go:build linux && !android

package single

import "syscall"

// peerUID returns the uid of the process at the other end of a connected
// Unix socket: SO_PEERCRED holds the credentials the peer had when it
// called connect(2) or listen(2) (unix(7)).
func peerUID(fd int) (int, error) {
	u, err := syscall.GetsockoptUcred(fd, syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	if err != nil {
		return 0, err
	}
	return int(u.Uid), nil
}
