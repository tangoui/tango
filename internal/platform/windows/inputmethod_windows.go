//go:build windows && (amd64 || arm64)

package windows

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	winio "github.com/Microsoft/go-winio"
	"github.com/tangoui/tango/internal/platform"
	"golang.org/x/sys/windows"
)

type tipServer struct {
	listener net.Listener
	*tipSessions
}

var _ platform.InputMethods = (*Backend)(nil)

func (b *Backend) ServeInputMethod(connection string, sink platform.InputMethodSink) error {
	if b.inputMethod != nil {
		return errors.New("tango/windows: input method already serving")
	}
	if len(connection) == 0 || len(connection) > 120 || strings.ContainsFunc(connection, func(r rune) bool {
		return !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_')
	}) {
		return errors.New("tango/windows: invalid input method connection name")
	}
	token := windows.GetCurrentProcessToken()
	user, err := token.GetTokenUser()
	if err != nil {
		return err
	}
	var login uint32
	if err := windows.ProcessIdToSessionId(uint32(os.Getpid()), &login); err != nil {
		return err
	}
	var label [16]uintptr
	var labelSize uint32
	if err := windows.GetTokenInformation(token, windows.TokenIntegrityLevel,
		(*byte)(unsafe.Pointer(&label[0])), uint32(unsafe.Sizeof(label)), &labelSize); err != nil {
		return err
	}
	integritySID := (*windows.Tokenmandatorylabel)(unsafe.Pointer(&label[0])).Label.Sid
	integrity := integritySID.SubAuthority(uint32(integritySID.SubAuthorityCount()) - 1)
	sid := user.User.Sid.String()
	name := fmt.Sprintf(`\\.\pipe\TangoIME-%s-%d-%d-%s`, sid, login, integrity, connection)
	pipe, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;" + sid + ")",
		InputBufferSize:    1024, OutputBufferSize: 8192,
	})
	if err != nil {
		return fmt.Errorf("tango/windows: open input method pipe: %w", err)
	}
	server := &tipServer{listener: pipe, tipSessions: newTIPSessions(sink, tipProcessName)}
	b.inputMethod = server
	go server.accept(b.runDeferred)
	return nil
}

func (s *tipServer) accept(runMain func(func())) {
	clients := make(chan struct{}, 16)
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		select {
		case clients <- struct{}{}:
			go func() {
				defer func() { <-clients }()
				s.serve(conn, runMain)
			}()
		default:
			_ = conn.Close()
		}
	}
}

func (s *tipServer) serve(conn net.Conn, runMain func(func())) {
	defer conn.Close()
	deadline := time.Now().Add(75 * time.Millisecond)
	conn.SetDeadline(deadline)
	var request tipRequest
	if binary.Read(conn, binary.LittleEndian, &request) != nil || request.Version != tipVersion {
		return
	}
	// dispatch takes the session's pending text. A response the bridge
	// never reads (dispatch finished after the deadline, or the write
	// failed) goes back to the session for the next Poll.
	var mu sync.Mutex
	abandoned := false
	response := make(chan tipResponse, 1)
	runMain(func() {
		if !time.Now().Before(deadline) {
			return
		}
		result := s.dispatch(request)
		mu.Lock()
		defer mu.Unlock()
		if abandoned {
			s.requeue(request.Token, result)
			return
		}
		response <- result
	})
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case result := <-response:
		if binary.Write(conn, binary.LittleEndian, result) != nil {
			runMain(func() { s.requeue(request.Token, result) })
		}
	case <-timer.C:
		mu.Lock()
		abandoned = true
		mu.Unlock()
		select {
		case result := <-response:
			runMain(func() { s.requeue(request.Token, result) })
		default:
		}
	}
}

func tipProcessName(pid uint32) string {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(process)
	var name [1024]uint16
	length := uint32(len(name))
	if windows.QueryFullProcessImageName(process, 0, &name[0], &length) != nil {
		return ""
	}
	value := windows.UTF16ToString(name[:length])
	if i := strings.LastIndexByte(value, '\\'); i >= 0 {
		value = value[i+1:]
	}
	return value
}

func (b *Backend) IMESetComposition(id uint64, text string, cursor int) {
	if session := b.inputMethod.byID[id]; session != nil {
		session.mark(text, cursor)
	}
}

func (b *Backend) IMECommit(id uint64, text string) {
	if session := b.inputMethod.byID[id]; session != nil {
		session.commit(text)
	}
}

func (b *Backend) IMECaret(id uint64) platform.Rect {
	if session := b.inputMethod.byID[id]; session != nil {
		return session.caret
	}
	return platform.Rect{}
}

func (b *Backend) IMEClientApp(id uint64) string {
	if session := b.inputMethod.byID[id]; session != nil {
		return session.client
	}
	return ""
}
