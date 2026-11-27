//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/tangoui/tango/internal/imebridge/proto"
	"golang.org/x/sys/windows"
)

type tipServer struct {
	*tipSessions
	listener net.Listener
	login    uint32
	user     *windows.SID
	runMain  func(func())
	done     chan struct{}
	limit    chan struct{}
}

func newTIPServer(connection string, sessions *tipSessions, runMain func(func())) (*tipServer, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	var login uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &login); err != nil {
		return nil, err
	}
	name := fmt.Sprintf(`\\.\pipe\TangoIME-v2-%s-%d-%s`, user.User.Sid, login, connection)
	listener, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: tipPipeSecurityDescriptor(user.User.Sid.String()),
		InputBufferSize:    proto.MaxFrame, OutputBufferSize: proto.MaxFrame,
	})
	if err != nil {
		return nil, fmt.Errorf("tango/windows: open input method pipe: %w", err)
	}
	return &tipServer{tipSessions: sessions, listener: listener, login: login, user: user.User.Sid,
		runMain: runMain, done: make(chan struct{}), limit: make(chan struct{}, 256)}, nil
}

func (s *tipServer) close() {
	close(s.done)
	_ = s.listener.Close()
}

func (s *tipServer) accept() {
	for {
		conn, err := s.listener.Accept()
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

func (s *tipServer) serve(conn net.Conn) {
	defer conn.Close()
	pipe, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return
	}
	var pid, login uint32
	if windows.GetNamedPipeClientProcessId(windows.Handle(pipe.Fd()), &pid) != nil ||
		windows.ProcessIdToSessionId(pid, &login) != nil || login != s.login {
		return
	}
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return
	}
	var token windows.Token
	err = windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token)
	if err != nil {
		windows.CloseHandle(process)
		return
	}
	user, err := token.GetTokenUser()
	token.Close()
	var exe string
	if err == nil && user.User.Sid.Equals(s.user) {
		var path [1024]uint16
		length := uint32(len(path))
		if windows.QueryFullProcessImageName(process, 0, &path[0], &length) == nil {
			exe = filepath.Base(windows.UTF16ToString(path[:length]))
		}
	}
	windows.CloseHandle(process)
	if exe == "" {
		return
	}

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msg, err := proto.Read(conn)
	hello, isHello := msg.Body.(proto.Hello)
	if err != nil || !isHello || msg.Seq != 0 || hello.PID != pid {
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	closed := make(chan struct{})
	var once sync.Once
	stop := func() {
		once.Do(func() {
			close(closed)
			_ = conn.Close()
		})
	}
	out := make(chan []proto.Message, 32)
	go func() {
		defer stop()
		for {
			select {
			case <-closed:
				return
			case <-s.done:
				return
			case batch := <-out:
				_ = conn.SetWriteDeadline(time.Now().Add(125 * time.Millisecond))
				for _, msg := range batch {
					data, err := proto.Marshal(msg)
					if err != nil {
						return
					}
					n, err := conn.Write(data)
					if err != nil || n != len(data) {
						return
					}
				}
			}
		}
	}()
	defer stop()

	var c *tipClient
	// Queue cleanup on the same application thread as the connect and
	// dispatch callbacks, even if shutdown wins while connect is queued.
	defer s.runMain(func() {
		if c != nil {
			s.disconnect(c)
		}
	})
	if !s.onMain(closed, func() {
		c = s.connect(exe, func(batch []proto.Message) {
			select {
			case out <- batch:
			case <-closed:
			default:
				stop()
			}
		})
	}) {
		return
	}
	for {
		msg, err := proto.Read(conn)
		if err != nil {
			return
		}
		deadline := time.Now().Add(100 * time.Millisecond)
		if !s.onMain(closed, func() {
			// A late synchronous event has already passed through in the
			// client. Do not run its application handler on a stalled UI.
			if _, isTest := msg.Body.(proto.TestKey); isTest && !time.Now().Before(deadline) {
				return
			}
			s.dispatch(c, msg)
		}) {
			return
		}
	}
}

func (s *tipServer) onMain(closed <-chan struct{}, fn func()) bool {
	ready := make(chan struct{})
	s.runMain(func() {
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
