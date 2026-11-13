//go:build (darwin || freebsd || linux || netbsd || openbsd || dragonfly) && !ios && !android

package single

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func init() {
	removeFiles = func(name string) {
		sock, lock, err := paths(name)
		if err != nil {
			return
		}
		os.Remove(sock)
		os.Remove(lock)
	}
}

// A socket file left by a crashed primary does not block the next one.
func TestStaleSocket(t *testing.T) {
	n := name(uniqueKey(t))
	sock, _, err := paths(n)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sock, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	srv, err := listen(n, func(message) {})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	if err := send(n, []byte(`{"args":[]}`), timeout); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(sock); err != nil || fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("socket %v %v", fi.Mode(), err)
	}
}

// Only a directory owned by us, with no access for others and not a
// symlink, holds the socket.
func TestCheckPrivate(t *testing.T) {
	uid := os.Getuid()
	base := t.TempDir()
	good := filepath.Join(base, "good")
	if err := privateDir(good, uid); err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Lstat(good); err != nil || fi.Mode().Perm() != 0o700 {
		t.Fatalf("created %v %v", fi.Mode(), err)
	}
	if err := privateDir(good, uid); err != nil {
		t.Fatalf("existing private dir refused: %v", err)
	}
	if err := checkPrivate(good, uid+1); err == nil || !strings.Contains(err.Error(), "owned by") {
		t.Fatalf("dir of another owner accepted: %v", err)
	}
	for _, mode := range []os.FileMode{0o750, 0o705, 0o770, 0o777, 0o777 | os.ModeSticky} {
		d := filepath.Join(base, "mode"+strconv.FormatUint(uint64(mode), 8))
		if err := os.Mkdir(d, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(d, mode); err != nil {
			t.Fatal(err)
		}
		if err := privateDir(d, uid); err == nil {
			t.Errorf("mode %v accepted", mode)
		}
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(good, link); err != nil {
		t.Fatal(err)
	}
	if err := privateDir(link, uid); err == nil {
		t.Error("symlink to a private dir accepted")
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := privateDir(file, uid); err == nil {
		t.Error("regular file accepted")
	}
}

// A tango-<uid> directory someone else prepared (here: world-writable) in
// $TMPDIR is not used; a usable $XDG_RUNTIME_DIR is, an open one is not.
func TestDirRefusesSharedDirs(t *testing.T) {
	n := name(uniqueKey(t))
	base := t.TempDir()
	t.Setenv("TMPDIR", base)
	t.Setenv("XDG_RUNTIME_DIR", "")
	planted := filepath.Join(base, "tango-"+strconv.Itoa(os.Getuid()))
	if err := os.Mkdir(planted, 0o700); err != nil {
		t.Fatal(err)
	}
	os.Chmod(planted, 0o777)
	d, err := dir(n)
	if err == nil && d == planted {
		t.Fatalf("used planted %s", d)
	}

	if runtime.GOOS != "linux" {
		return
	}
	xdg := filepath.Join(t.TempDir(), "run")
	if err := os.Mkdir(xdg, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_RUNTIME_DIR", xdg)
	if d, err := dir(n); err != nil || d != xdg {
		t.Fatalf("private XDG_RUNTIME_DIR: %q %v", d, err)
	}
	os.Chmod(xdg, 0o755)
	if d, _ := dir(n); d == xdg {
		t.Fatal("XDG_RUNTIME_DIR with mode 0755 used")
	}
}

func socketpair(t *testing.T) (net.Conn, net.Conn) {
	t.Helper()
	fds, err := syscall.Socketpair(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err != nil {
		t.Fatal(err)
	}
	var cs [2]net.Conn
	for i, fd := range fds {
		f := os.NewFile(uintptr(fd), "socketpair")
		c, err := net.FileConn(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { c.Close() })
		cs[i] = c
	}
	return cs[0], cs[1]
}

// Peer credentials name our own uid; any other expected uid is refused.
func TestPeerUID(t *testing.T) {
	if runtime.GOOS == "openbsd" {
		t.Skip("no peer credentials without x/sys")
	}
	a, b := socketpair(t)
	for _, c := range []net.Conn{a, b} {
		raw, err := c.(*net.UnixConn).SyscallConn()
		if err != nil {
			t.Fatal(err)
		}
		var uid int
		var perr error
		raw.Control(func(fd uintptr) { uid, perr = peerUID(int(fd)) })
		if perr != nil || uid != os.Getuid() {
			t.Fatalf("peerUID = %d, %v; want %d", uid, perr, os.Getuid())
		}
		if err := checkPeerUID(c, os.Getuid()); err != nil {
			t.Fatal(err)
		}
		if err := checkPeerUID(c, os.Getuid()+1); !errors.Is(err, errUntrusted) {
			t.Fatalf("foreign uid: %v, want errUntrusted", err)
		}
	}
}

// When the primary's socket belongs to another uid, a later instance sends
// nothing and claim reports an error without forwarding (the app then runs
// on its own) instead of waiting for a primary that is not ours.
func TestClaimRefusesForeignPrimary(t *testing.T) {
	if runtime.GOOS == "openbsd" {
		t.Skip("no peer credentials without x/sys")
	}
	n := name(uniqueKey(t))
	got := make(chan message, 1)
	srv, err := listen(n, func(m message) { got <- m })
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	me := os.Getuid()
	peerWant = func() int { return me + 1 }
	defer func() { peerWant = os.Getuid }()
	if err := send(n, []byte(`{"args":["secret"]}`), timeout); !errors.Is(err, errUntrusted) {
		t.Fatalf("send: %v, want errUntrusted", err)
	}
	s2, fwd, err := claim(n, []string{"secret"}, func(message) {}, timeout)
	if s2 != nil || fwd || !errors.Is(err, errUntrusted) || errors.Is(err, errReach) {
		t.Fatalf("claim: srv=%v fwd=%v err=%v", s2, fwd, err)
	}
	select {
	case m := <-got:
		t.Fatalf("delivered %+v", m)
	default:
	}
}
