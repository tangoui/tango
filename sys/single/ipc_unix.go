//go:build (darwin || freebsd || linux || netbsd || openbsd || dragonfly) && !ios && !android

package single

import (
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

const mobile = false

func userTag() string { return strconv.Itoa(os.Getuid()) }

// sunPathMax keeps socket paths below sun_path: 104 bytes on macOS and the
// BSDs, 108 on Linux (unix(7), unix(4)).
const sunPathMax = 100

// dir returns a directory only this user can write to, for the socket and
// the lock file. Names in it are guessable, so a shared directory such as
// /tmp would let another user take them first and receive our arguments.
//
// It is $XDG_RUNTIME_DIR on Linux when that is usable (the XDG Base
// Directory spec makes it owned by the user with mode 0700; we check), else
// a "tango-<uid>" directory we create with mode 0700 in $TMPDIR (per user on
// macOS) or /tmp. A directory that is not ours alone is refused: the caller
// then runs without single-instance mode instead of trusting it.
func dir(name string) (string, error) {
	uid := os.Getuid()
	fits := func(d string) bool { return len(filepath.Join(d, name+".sock")) < sunPathMax }
	var errs []error
	if runtime.GOOS == "linux" {
		if d := os.Getenv("XDG_RUNTIME_DIR"); d != "" && filepath.IsAbs(d) && fits(d) {
			err := checkPrivate(d, uid)
			if err == nil {
				return d, nil
			}
			errs = append(errs, err)
		}
	}
	sub := "tango-" + strconv.Itoa(uid)
	tried := map[string]bool{}
	for _, base := range []string{os.TempDir(), "/tmp"} {
		d := filepath.Join(base, sub)
		if tried[d] || !fits(d) {
			continue
		}
		tried[d] = true
		err := privateDir(d, uid)
		if err == nil {
			return d, nil
		}
		errs = append(errs, err)
	}
	if len(errs) == 0 {
		errs = append(errs, errors.New("single: no directory short enough for a socket path"))
	}
	return "", errors.Join(errs...)
}

// privateDir creates d with mode 0700 unless it exists, then checks it.
func privateDir(d string, uid int) error {
	if err := os.Mkdir(d, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	return checkPrivate(d, uid)
}

// checkPrivate reports whether d (not following a final symlink) is a
// directory owned by uid that no one else may access. Its parent is a
// sticky directory like /tmp or one of ours, so once checked it cannot be
// swapped by another user.
func checkPrivate(d string, uid int) error {
	fi, err := os.Lstat(d)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("single: %s is not a directory (%v)", d, fi.Mode().Type())
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("single: cannot read the owner of %s", d)
	}
	if int(st.Uid) != uid {
		return fmt.Errorf("single: %s is owned by uid %d, not %d", d, st.Uid, uid)
	}
	if p := fi.Mode().Perm(); p&0o077 != 0 {
		return fmt.Errorf("single: %s has mode %#o, want no access for group and others", d, p)
	}
	return nil
}

func paths(name string) (sock, lock string, err error) {
	d, err := dir(name)
	if err != nil {
		return "", "", err
	}
	return filepath.Join(d, name+".sock"), filepath.Join(d, name+".lock"), nil
}

// errNoPeerCred: the system has no peer credential query we can make.
var errNoPeerCred = errors.New("single: peer credentials unavailable")

// checkPeer verifies that the other end of c runs as this user. Where the
// system offers no way to ask (errNoPeerCred), the private directory is the
// only guard.
// checkPeer is the later instance's check of the primary.
func checkPeer(c net.Conn) error { return checkPeerUID(c, peerWant()) }

// peerWant is the uid a peer must have; tests replace it.
var peerWant = os.Getuid

func checkPeerUID(c net.Conn, want int) error {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return fmt.Errorf("%w: not a unix socket", errUntrusted)
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return fmt.Errorf("%w: %v", errUntrusted, err)
	}
	var uid int
	var perr error
	if err := raw.Control(func(fd uintptr) { uid, perr = peerUID(int(fd)) }); err != nil {
		return fmt.Errorf("%w: %v", errUntrusted, err)
	}
	if errors.Is(perr, errNoPeerCred) {
		return nil
	}
	if perr != nil {
		return fmt.Errorf("%w: peer credentials: %v", errUntrusted, perr)
	}
	if uid != want {
		return fmt.Errorf("%w: peer runs as uid %d", errUntrusted, uid)
	}
	return nil
}

type unixServer struct {
	ln   net.Listener
	lock *os.File
	c    closeOnce
}

func (s *unixServer) Close() error {
	return s.c.do(func() error {
		err := s.ln.Close() // also unlinks the socket
		s.lock.Close()      // releases the flock
		return err
	})
}

func listen(name string, deliver func(message)) (server, error) {
	sock, lockPath, err := paths(name)
	if err != nil {
		return nil, err
	}
	lock, err := os.OpenFile(lockPath, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, errBusy
		}
		return nil, err
	}
	// We hold the lock: any socket file is a leftover of a dead primary.
	os.Remove(sock)
	old := syscall.Umask(0o077)
	ln, err := net.Listen("unix", sock)
	syscall.Umask(old)
	if err != nil {
		lock.Close()
		return nil, err
	}
	s := &unixServer{ln: ln, lock: lock}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				if checkPeerUID(c, os.Getuid()) != nil {
					return // another user's process: drop it unread
				}
				c.SetDeadline(time.Now().Add(5 * time.Second))
				serve(c, deliver)
			}()
		}
	}()
	return s, nil
}

func send(name string, msg []byte, timeout time.Duration) error {
	sock, _, err := paths(name)
	if err != nil {
		return err
	}
	if timeout < 100*time.Millisecond {
		timeout = 100 * time.Millisecond
	}
	c, err := net.DialTimeout("unix", sock, timeout)
	if err != nil {
		return err
	}
	defer c.Close()
	// Check who listens before sending anything: the arguments may hold
	// deep links with tokens, file paths and so on.
	if err := checkPeer(c); err != nil {
		return err
	}
	c.SetDeadline(time.Now().Add(timeout))
	return exchange(c, msg)
}
