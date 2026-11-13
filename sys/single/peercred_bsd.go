//go:build (darwin && !ios) || freebsd || dragonfly || netbsd

package single

import (
	"runtime"
	"syscall"
	"unsafe"
)

// Socket level and options of the local (Unix) domain, from <sys/un.h>.
const (
	solLocal       = 0 // SOL_LOCAL
	localPeercred  = 1 // LOCAL_PEERCRED: struct xucred (macOS, FreeBSD, DragonFly)
	localPeereid   = 3 // LOCAL_PEEREID: struct unpcbid (NetBSD)
	xucredVersion0 = 0 // XUCRED_VERSION
)

// peerCredBuf is large enough for struct xucred (cr_version, cr_uid,
// cr_ngroups, cr_groups[16] and FreeBSD's trailing pid union) and for
// NetBSD's struct unpcbid. The kernel copies at most this many bytes.
type peerCredBuf struct {
	A, B, C uint32
	_       [29]uint32
}

// peerUID returns the uid of the process at the other end of a connected
// Unix socket: what getpeereid(3) reports, read directly with getsockopt(2)
// because the syscall package has no wrapper for these options.
func peerUID(fd int) (int, error) {
	var buf peerCredBuf
	opt := uintptr(localPeercred)
	if runtime.GOOS == "netbsd" {
		opt = localPeereid
	}
	n := uint32(unsafe.Sizeof(buf))
	_, _, e := syscall.Syscall6(syscall.SYS_GETSOCKOPT, uintptr(fd), solLocal, opt,
		uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&n)), 0)
	if e != 0 {
		return 0, e
	}
	if runtime.GOOS == "netbsd" {
		// struct unpcbid { pid_t unp_pid; uid_t unp_euid; gid_t unp_egid; }
		if n < 12 {
			return 0, syscall.EINVAL
		}
		return int(buf.B), nil
	}
	// struct xucred { u_int cr_version; uid_t cr_uid; ... }
	if n < 8 || buf.A != xucredVersion0 {
		return 0, syscall.EINVAL
	}
	return int(buf.B), nil
}
