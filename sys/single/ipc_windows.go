//go:build windows

package single

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const mobile = false

var (
	kernel32                        = syscall.NewLazyDLL("kernel32.dll")
	procCreateNamedPipeW            = kernel32.NewProc("CreateNamedPipeW")
	procConnectNamedPipe            = kernel32.NewProc("ConnectNamedPipe")
	procDisconnectNamedPipe         = kernel32.NewProc("DisconnectNamedPipe")
	procWaitNamedPipeW              = kernel32.NewProc("WaitNamedPipeW")
	procFlushFileBuffers            = kernel32.NewProc("FlushFileBuffers")
	procGetNamedPipeServerProcessId = kernel32.NewProc("GetNamedPipeServerProcessId")
	advapi32                        = syscall.NewLazyDLL("advapi32.dll")
	procConvertStringSDToSD         = advapi32.NewProc("ConvertStringSecurityDescriptorToSecurityDescriptorW")
	user32                          = syscall.NewLazyDLL("user32.dll")
	procAllowSetForegroundWn        = user32.NewProc("AllowSetForegroundWindow")
)

const (
	pipeAccessDuplex          = 0x3
	fileFlagFirstPipeInstance = 0x80000
	pipeRejectRemoteClients   = 0x8 // PIPE_TYPE_BYTE|PIPE_READMODE_BYTE|PIPE_WAIT are 0
	pipeUnlimitedInstances    = 255

	// CreateFile dwFlagsAndAttributes: the server may identify the client
	// but not impersonate it.
	securitySqosPresent    = 0x00100000 // SECURITY_SQOS_PRESENT
	securityIdentification = 0x00010000 // SECURITY_IDENTIFICATION

	processQueryLimitedInformation = 0x1000 // PROCESS_QUERY_LIMITED_INFORMATION
	sddlRevision1                  = 1      // SDDL_REVISION_1

	errorAccessDenied  syscall.Errno = 5
	errorPipeBusy      syscall.Errno = 231
	errorPipeConnected syscall.Errno = 535

	asfwAny = ^uintptr(0) // ASFW_ANY: (DWORD)-1
)

// currentSID is the string SID of the user this process runs as, from the
// process token (TokenUser; ConvertSidToStringSidW).
var currentSID = sync.OnceValues(func() (string, error) {
	t, err := syscall.OpenCurrentProcessToken()
	if err != nil {
		return "", err
	}
	defer t.Close()
	return tokenSID(t)
})

func tokenSID(t syscall.Token) (string, error) {
	u, err := t.GetTokenUser()
	if err != nil {
		return "", err
	}
	return u.User.Sid.String()
}

// userTag separates users; pipe names are machine-wide.
func userTag() string {
	sid, _ := currentSID()
	return sid
}

// pipePath names the pipe after the key and the user's SID. Anyone can
// compute it, so the name only keeps users apart; the DACL and the server
// owner check in send keep them safe from each other.
func pipePath(name string) (string, error) {
	sid, err := currentSID()
	if err != nil {
		return "", fmt.Errorf("single: current user SID: %w", err)
	}
	return `\\.\pipe\` + name + "-" + sid, nil
}

// pipeSDDL grants generic all to the user and no one else; "P" keeps
// inheritable entries out. LocalSystem and administrators get no entry.
func pipeSDDL(sid string) string { return "D:P(A;;GA;;;" + sid + ")" }

// securityAttributes builds SECURITY_ATTRIBUTES from SDDL; free releases
// the descriptor (LocalFree).
func securityAttributes(sddl string) (sa *syscall.SecurityAttributes, free func(), err error) {
	s, err := syscall.UTF16PtrFromString(sddl)
	if err != nil {
		return nil, nil, err
	}
	var sd uintptr
	r, _, e := procConvertStringSDToSD.Call(uintptr(unsafe.Pointer(s)), sddlRevision1, uintptr(unsafe.Pointer(&sd)), 0)
	if r == 0 {
		return nil, nil, fmt.Errorf("single: security descriptor: %w", e)
	}
	sa = &syscall.SecurityAttributes{SecurityDescriptor: sd}
	sa.Length = uint32(unsafe.Sizeof(*sa))
	return sa, func() { syscall.LocalFree(syscall.Handle(sd)) }, nil
}

// pipeHandle is a pipe end usable as io.ReadWriter.
type pipeHandle syscall.Handle

func (h pipeHandle) Read(p []byte) (int, error) {
	var n uint32
	err := syscall.ReadFile(syscall.Handle(h), p, &n, nil)
	if n == 0 && err == nil {
		return 0, errors.New("single: pipe closed")
	}
	return int(n), err
}

func (h pipeHandle) Write(p []byte) (int, error) {
	var n uint32
	err := syscall.WriteFile(syscall.Handle(h), p, &n, nil)
	return int(n), err
}

// createPipe creates an instance of the pipe at path whose DACL admits only
// the current user. With first, FILE_FLAG_FIRST_PIPE_INSTANCE makes it fail
// with ERROR_ACCESS_DENIED if any instance of the name exists, whoever made
// it: that is the atomic "am I first" test, so another user who created the
// name earlier cannot make us a second instance of their pipe.
func createPipe(path string, first bool) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	sid, err := currentSID()
	if err != nil {
		return syscall.InvalidHandle, err
	}
	sa, free, err := securityAttributes(pipeSDDL(sid))
	if err != nil {
		return syscall.InvalidHandle, err
	}
	defer free()
	mode := uintptr(pipeAccessDuplex)
	if first {
		mode |= fileFlagFirstPipeInstance
	}
	r, _, e := procCreateNamedPipeW.Call(uintptr(unsafe.Pointer(p)), mode, pipeRejectRemoteClients,
		pipeUnlimitedInstances, 4096, 4096, 0, uintptr(unsafe.Pointer(sa)))
	if syscall.Handle(r) == syscall.InvalidHandle {
		if first && e == errorAccessDenied {
			// The name is taken, maybe not by us: errBusy only sends us
			// to send, which checks the server's owner before trusting it.
			return syscall.InvalidHandle, errBusy
		}
		return syscall.InvalidHandle, e
	}
	return syscall.Handle(r), nil
}

type pipeServer struct {
	path   string
	closed atomic.Bool
	done   chan struct{} // closed when the accept loop has released its instance
	c      closeOnce
}

// Close stops the accept loop: ConnectNamedPipe blocks synchronously, so
// connect once ourselves to wake it. It returns once the loop has closed
// its listening instance, so the key is free for a new primary (bounded,
// in case the wake-up connection could not be made).
func (s *pipeServer) Close() error {
	return s.c.do(func() error {
		s.closed.Store(true)
		if h, err := openPipe(s.path); err == nil {
			syscall.CloseHandle(h)
		}
		select {
		case <-s.done:
		case <-time.After(2 * time.Second):
		}
		return nil
	})
}

func listen(name string, deliver func(message)) (server, error) {
	path, err := pipePath(name)
	if err != nil {
		return nil, err
	}
	h, err := createPipe(path, true)
	if err != nil {
		return nil, err
	}
	s := &pipeServer{path: path, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		cur := h
		for {
			r, _, e := procConnectNamedPipe.Call(uintptr(cur), 0)
			if s.closed.Load() {
				syscall.CloseHandle(cur)
				return
			}
			if r == 0 && e != errorPipeConnected {
				syscall.CloseHandle(cur)
				return
			}
			// Create the next instance before releasing this one, so the
			// name never disappears and a new process can never pass the
			// first-instance test while we run.
			next, err := createPipe(path, false)
			go func(c syscall.Handle) {
				defer syscall.CloseHandle(c)
				serve(pipeHandle(c), deliver)
				procFlushFileBuffers.Call(uintptr(c))
				procDisconnectNamedPipe.Call(uintptr(c))
			}(cur)
			if err != nil {
				return
			}
			cur = next
		}
	}()
	return s, nil
}

// openPipe connects to the pipe at path. SECURITY_SQOS_PRESENT with
// SECURITY_IDENTIFICATION lets the server learn who we are but not act as
// us (impersonation levels, CreateFileW).
func openPipe(path string) (syscall.Handle, error) {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return syscall.InvalidHandle, err
	}
	return syscall.CreateFile(p, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING,
		securitySqosPresent|securityIdentification, 0)
}

// checkServer verifies that the process serving the connected pipe h runs
// as the user with SID want: GetNamedPipeServerProcessId, then that
// process's token user (OpenProcessToken, TokenUser).
func checkServer(h syscall.Handle, want string) error {
	var pid uint32
	if r, _, e := procGetNamedPipeServerProcessId.Call(uintptr(h), uintptr(unsafe.Pointer(&pid))); r == 0 {
		return fmt.Errorf("%w: server process: %v", errUntrusted, e)
	}
	ph, err := syscall.OpenProcess(processQueryLimitedInformation, false, pid)
	if err != nil {
		return fmt.Errorf("%w: open server process %d: %v", errUntrusted, pid, err)
	}
	defer syscall.CloseHandle(ph)
	var t syscall.Token
	if err := syscall.OpenProcessToken(ph, syscall.TOKEN_QUERY, &t); err != nil {
		return fmt.Errorf("%w: server process %d token: %v", errUntrusted, pid, err)
	}
	defer t.Close()
	sid, err := tokenSID(t)
	if err != nil {
		return fmt.Errorf("%w: server process %d user: %v", errUntrusted, pid, err)
	}
	if sid != want {
		return fmt.Errorf("%w: server process %d runs as %s", errUntrusted, pid, sid)
	}
	return nil
}

func send(name string, msg []byte, timeout time.Duration) error {
	path, err := pipePath(name)
	if err != nil {
		return err
	}
	me, err := currentSID()
	if err != nil {
		return err
	}
	if timeout < 100*time.Millisecond {
		timeout = 100 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	var h syscall.Handle
	for {
		var err error
		h, err = openPipe(path)
		if err == nil {
			break
		}
		if err == errorAccessDenied {
			// Our own pipe always admits us: this one is someone else's.
			return fmt.Errorf("%w: %s: %v", errUntrusted, path, err)
		}
		if err != errorPipeBusy || time.Now().After(deadline) {
			return err
		}
		p, _ := syscall.UTF16PtrFromString(path)
		procWaitNamedPipeW.Call(uintptr(unsafe.Pointer(p)), uintptr(time.Until(deadline)/time.Millisecond))
	}
	// Nothing is written before the server is known to be ours.
	if err := checkServer(h, me); err != nil {
		syscall.CloseHandle(h)
		return err
	}
	// Let the primary bring its window to the front: Windows only lets the
	// foreground process (us, just launched) hand that right on.
	procAllowSetForegroundWn.Call(asfwAny)
	done := make(chan error, 1)
	go func() { done <- exchange(pipeHandle(h), msg) }()
	select {
	case err := <-done:
		syscall.CloseHandle(h)
		return err
	case <-time.After(time.Until(deadline)):
		syscall.CloseHandle(h) // unblocks the pending ReadFile
		return errors.New("single: primary did not answer")
	}
}
