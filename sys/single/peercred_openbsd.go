//go:build openbsd

package single

// peerUID: OpenBSD has SO_PEERCRED, but the syscall package wraps neither
// it nor raw system calls there (they return ENOSYS). The private 0700
// directory alone keeps other users from the socket.
func peerUID(int) (int, error) { return 0, errNoPeerCred }
