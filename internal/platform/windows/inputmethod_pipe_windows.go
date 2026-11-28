//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	winio "github.com/Microsoft/go-winio"
	"github.com/tangoui/tango/internal/imebridge/proto"
	"github.com/tangoui/tango/internal/platform/imesession"
	"golang.org/x/sys/windows"
)

type tipServer struct {
	*imesession.Mux
	listener net.Listener
	login    uint32
	user     *windows.SID
	runMain  func(func())
	done     chan struct{}
	limit    chan struct{}
}

func newTIPServer(connection string, sessions *imesession.Mux, runMain func(func())) (*tipServer, error) {
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
	return &tipServer{Mux: sessions, listener: listener, login: login, user: user.User.Sid,
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
	handle := windows.Handle(pipe.Fd())
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	msg, err := proto.Read(conn)
	hello, isHello := msg.Body.(proto.Hello)
	if err != nil || !isHello || msg.Seq != 0 || hello.PID != pid {
		return
	}
	exe, user := tipClientProcess(pid)
	if user == nil {
		// A protected or packaged client may be inaccessible through
		// OpenProcessToken. Identify it through the pipe token instead.
		user = tipPipeClientUser(handle)
	}
	if user == nil || !user.Equals(s.user) {
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
						// An unencodable message, such as an oversized
						// menu, is dropped without closing the connection.
						continue
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

	var c *imesession.Client
	// Queue cleanup on the same application thread as the connect and
	// dispatch callbacks, even if shutdown wins while connect is queued.
	defer s.runMain(func() {
		if c != nil {
			s.Disconnect(c)
		}
	})
	if !s.onMain(closed, func() {
		c = s.Connect(exe, func(batch []proto.Message) {
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
			s.Dispatch(c, msg)
		}) {
			return
		}
	}
}

var tipImpersonateNamedPipeClient = windows.NewLazySystemDLL("advapi32.dll").NewProc("ImpersonateNamedPipeClient")

func tipPipeClientUser(pipe windows.Handle) (sid *windows.SID) {
	runtime.LockOSThread()
	if ok, _, _ := tipImpersonateNamedPipeClient.Call(uintptr(pipe)); ok == 0 {
		runtime.UnlockOSThread()
		return nil
	}
	defer func() {
		if windows.RevertToSelf() == nil {
			runtime.UnlockOSThread()
		} else {
			// The caller returns on nil, discarding this OS thread.
			sid = nil
		}
	}()
	var token windows.Token
	if windows.OpenThreadToken(windows.CurrentThread(), windows.TOKEN_QUERY, true, &token) != nil {
		return nil
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil
	}
	return user.User.Sid
}

func tipClientProcess(pid uint32) (exe string, user *windows.SID) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", nil
	}
	defer windows.CloseHandle(process)
	var path [1024]uint16
	length := uint32(len(path))
	if windows.QueryFullProcessImageName(process, 0, &path[0], &length) == nil {
		exe = filepath.Base(windows.UTF16ToString(path[:length]))
	}
	var token windows.Token
	if windows.OpenProcessToken(process, windows.TOKEN_QUERY, &token) != nil {
		return exe, nil
	}
	defer token.Close()
	account, err := token.GetTokenUser()
	if err != nil {
		return exe, nil
	}
	return exe, account.User.Sid
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
