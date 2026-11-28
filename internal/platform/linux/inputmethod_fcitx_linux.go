//go:build linux && !android

package linux

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform/imesession"
	"golang.org/x/sys/unix"
)

type fcitxServer struct {
	listener *net.UnixListener
	lock     *os.File
	mux      *imesession.Mux
	done     chan struct{}
	limit    chan struct{}
	uid      uint32
}

func privateRuntimeDir(path string, uid uint32) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || info.Mode().Perm() != fcitxSocketDirMode || !ok || stat.Uid != uid {
		return fmt.Errorf("runtime directory %s must be owned by uid %d with mode 0700", path, uid)
	}
	return nil
}

func startFcitx(connection string, mux *imesession.Mux) (*fcitxServer, error) {
	runtime := os.Getenv("XDG_RUNTIME_DIR")
	path, err := fcitxSocketPath(runtime, connection)
	if err != nil {
		return nil, err
	}
	uid := uint32(os.Getuid())
	if err := privateRuntimeDir(runtime, uid); err != nil {
		return nil, err
	}
	dir := filepath.Dir(path)
	if err := os.Mkdir(dir, fcitxSocketDirMode); err != nil && !os.IsExist(err) {
		return nil, err
	}
	if err := privateRuntimeDir(dir, uid); err != nil {
		return nil, err
	}
	lockPath := path + ".lock"
	fd, err := unix.Open(lockPath, unix.O_CREAT|unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, fcitxSocketMode)
	if err != nil {
		return nil, err
	}
	lock := os.NewFile(uintptr(fd), lockPath)
	ready := false
	defer func() {
		if !ready {
			_ = lock.Close()
		}
	}()
	stat, err := lock.Stat()
	if err != nil || !stat.Mode().IsRegular() {
		return nil, fmt.Errorf("invalid Fcitx5 socket lock %s", lockPath)
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, fmt.Errorf("Fcitx5 host already starting or running: %w", err)
	}
	if info, err := os.Lstat(path); err == nil {
		stat, ok := info.Sys().(*syscall.Stat_t)
		if info.Mode()&os.ModeSocket == 0 || !ok || stat.Uid != uid {
			return nil, fmt.Errorf("refusing to remove unexpected socket entry %s", path)
		}
		peer, err := net.DialTimeout("unix", path, 100*time.Millisecond)
		if err == nil {
			_ = peer.Close()
			return nil, errors.New("Fcitx5 socket is already listening")
		}
		if !errors.Is(err, syscall.ECONNREFUSED) {
			return nil, err
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: path, Net: "unix"})
	if err != nil {
		return nil, err
	}
	listener.SetUnlinkOnClose(true)
	if err := os.Chmod(path, fcitxSocketMode); err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &fcitxServer{listener: listener, lock: lock, mux: mux, uid: uid,
		done: make(chan struct{}), limit: make(chan struct{}, 32)}
	ready = true
	go server.accept()
	return server, nil
}

func (s *fcitxServer) close() {
	close(s.done)
	_ = s.listener.Close()
	_ = s.lock.Close()
}

func (s *fcitxServer) accept() {
	for {
		conn, err := s.listener.AcceptUnix()
		if err != nil {
			return
		}
		select {
		case s.limit <- struct{}{}:
			go func() {
				defer func() { <-s.limit }()
				s.serve(conn)
			}()
		default:
			_ = conn.Close()
		}
	}
}

func fcitxPeer(conn *net.UnixConn) (*unix.Ucred, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *unix.Ucred
	var queryErr error
	if err := raw.Control(func(fd uintptr) {
		cred, queryErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	}); err != nil {
		return nil, err
	}
	return cred, queryErr
}

func (s *fcitxServer) serve(conn *net.UnixConn) {
	defer conn.Close()
	cred, err := fcitxPeer(conn)
	if err != nil || !fcitxPeerAllowed(s.uid, cred.Uid) {
		return
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	frame, err := proto.Read(conn)
	hello, ok := frame.Body.(proto.Hello)
	if err != nil || !ok || frame.Seq != 0 || hello.PID != uint32(cred.Pid) {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})

	output := make(chan []proto.Message, 32)
	closed := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			close(closed)
			_ = conn.Close()
		})
	}
	go func() {
		defer stop()
		for {
			select {
			case <-closed:
				return
			case <-s.done:
				return
			case batch := <-output:
				_ = conn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond))
				for _, item := range batch {
					data, err := proto.Marshal(item)
					if err != nil {
						// Skip unencodable frames and keep the connection open.
						continue
					}
					if n, err := conn.Write(data); err != nil || n != len(data) {
						return
					}
				}
			}
		}
	}()
	defer stop()
	var client *imesession.Client
	defer later(0, func() {
		if client != nil {
			s.mux.Disconnect(client)
		}
	})
	if !s.onMain(closed, func() {
		output <- []proto.Message{{Body: proto.Hello{PID: hello.PID}}}
		client = s.mux.Connect("fcitx5", func(batch []proto.Message) {
			select {
			case output <- batch:
			case <-closed:
			default:
				stop()
			}
		})
	}) {
		return
	}
	for {
		frame, err := proto.Read(conn)
		if err != nil {
			return
		}
		deadline := time.Now().Add(100 * time.Millisecond)
		if !s.onMain(closed, func() {
			if frame.Seq != 0 && !time.Now().Before(deadline) {
				return
			}
			s.mux.Dispatch(client, frame)
		}) {
			return
		}
	}
}

func (s *fcitxServer) onMain(closed <-chan struct{}, fn func()) bool {
	ready := make(chan struct{})
	later(0, func() {
		defer close(ready)
		select {
		case <-closed:
		case <-s.done:
		default:
			fn()
		}
	})
	select {
	case <-ready:
		return true
	case <-closed:
	case <-s.done:
	}
	return false
}
