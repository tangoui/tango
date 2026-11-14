package pty

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The ConPTY functions are looked up at run time so that the package
// still loads on Windows versions older than 10 1809, where Start then
// fails with errConPTY.
var (
	kernel32                      = syscall.NewLazyDLL("kernel32.dll")
	procCreatePseudoConsole       = kernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole       = kernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole        = kernel32.NewProc("ClosePseudoConsole")
	procUpdateProcThreadAttribute = kernel32.NewProc("UpdateProcThreadAttribute")
)

var errConPTY = errors.New("pty: ConPTY needs Windows 10 1809 or later")

// sysPTY is a pseudo console and the two pipes connecting it to us.
//
// Read and Write hold rmu and wmu shared around ReadFile and WriteFile;
// Close takes them exclusively before it closes the handles, so a
// handle is never used after it was closed (and possibly reused).
type sysPTY struct {
	in  windows.Handle // write end of the console's input pipe
	out windows.Handle // read end of the console's output pipe

	mu  sync.Mutex     // guards hpc
	hpc windows.Handle // the pseudo console; 0 once closed

	closed   atomic.Bool
	rmu, wmu sync.RWMutex
	lastOut  atomic.Int64 // UnixNano of the last output read, 0 if none
}

// Start starts the program described by o on a new pseudo console.
func Start(o Options) (*PTY, error) {
	for _, p := range []*syscall.LazyProc{procCreatePseudoConsole, procResizePseudoConsole, procClosePseudoConsole} {
		if p.Find() != nil {
			return nil, errConPTY
		}
	}
	o, err := o.normalize()
	if err != nil {
		return nil, err
	}
	o.Path = resolvePath(o.Path, o.Dir)
	o.Env = addSystemRoot(o.Env, os.Getenv("SYSTEMROOT"))

	// The console reads input from inR and writes output to outW; it
	// keeps its own copies, so ours are closed once it exists.
	var inR, inW, outR, outW windows.Handle
	if err := windows.CreatePipe(&inR, &inW, nil, 0); err != nil {
		return nil, os.NewSyscallError("CreatePipe", err)
	}
	if err := windows.CreatePipe(&outR, &outW, nil, 0); err != nil {
		closeHandles(inR, inW)
		return nil, os.NewSyscallError("CreatePipe", err)
	}
	var hpc windows.Handle
	r, _, _ := procCreatePseudoConsole.Call(coord(o.Cols, o.Rows), uintptr(inR), uintptr(outW), 0, uintptr(unsafe.Pointer(&hpc)))
	closeHandles(inR, outW)
	if r != 0 {
		closeHandles(inW, outR)
		return nil, fmt.Errorf("pty: CreatePseudoConsole: HRESULT %#x", uint32(r))
	}
	p := &PTY{done: make(chan struct{})}
	p.sys.in, p.sys.out, p.sys.hpc = inW, outR, hpc
	pi, err := createProcess(o, hpc)
	if err != nil {
		p.sys.close()
		return nil, err
	}
	windows.CloseHandle(pi.Thread)
	p.pid = int(pi.ProcessId)
	go p.reap(pi.Process)
	return p, nil
}

// createProcess starts o's program attached to the pseudo console hpc.
func createProcess(o Options, hpc windows.Handle) (*windows.ProcessInformation, error) {
	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, os.NewSyscallError("InitializeProcThreadAttributeList", err)
	}
	defer attrs.Delete()
	// The attribute's value is the HPCON itself, not a pointer to it.
	r, _, e := procUpdateProcThreadAttribute.Call(uintptr(unsafe.Pointer(attrs.List())), 0,
		windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, uintptr(hpc), unsafe.Sizeof(hpc), 0, 0)
	if r == 0 {
		return nil, os.NewSyscallError("UpdateProcThreadAttribute", e)
	}

	app, err := windows.UTF16PtrFromString(o.Path)
	if err != nil {
		return nil, err
	}
	cmdLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(o.Args))
	if err != nil {
		return nil, err
	}
	env, err := envBlock(o.Env)
	if err != nil {
		return nil, err
	}
	var dir *uint16
	if o.Dir != "" {
		if dir, err = windows.UTF16PtrFromString(o.Dir); err != nil {
			return nil, err
		}
	}
	si := &windows.StartupInfoEx{ProcThreadAttributeList: attrs.List()}
	si.Cb = uint32(unsafe.Sizeof(*si))
	// No standard handles: the program must not inherit ours, it uses
	// the pseudo console.
	si.Flags = windows.STARTF_USESTDHANDLES
	pi := new(windows.ProcessInformation)
	err = windows.CreateProcess(app, cmdLine, nil, nil, false,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT,
		&env[0], dir, &si.StartupInfo, pi)
	if err != nil {
		return nil, &os.PathError{Op: "CreateProcess", Path: o.Path, Err: err}
	}
	return pi, nil
}

// reap waits for the program, records its exit status for Wait and
// closes the console once its output has gone quiet: a pseudo console
// outlives the programs attached to it, and Read reaches EOF only once
// it is closed.
func (p *PTY) reap(process windows.Handle) {
	var code uint32
	_, err := windows.WaitForSingleObject(process, windows.INFINITE)
	if err == nil {
		err = windows.GetExitCodeProcess(process, &code)
	}
	windows.CloseHandle(process)
	p.code = int(code)
	if err != nil {
		p.code, p.waitErr = -1, os.NewSyscallError("GetExitCodeProcess", err)
	}
	close(p.done)
	p.sys.linger(time.Now())
	p.sys.closeConsole()
}

// linger waits, after the program exited at exit, for the console's
// output to go quiet (see lingerDelay), or until Close.
func (s *sysPTY) linger(exit time.Time) {
	for !s.closed.Load() {
		var last time.Time
		if ns := s.lastOut.Load(); ns != 0 {
			last = time.Unix(0, ns)
		}
		d := lingerDelay(exit, last, time.Now())
		if d <= 0 {
			return
		}
		time.Sleep(min(d, 20*time.Millisecond))
	}
}

func (s *sysPTY) read(b []byte) (int, error) {
	s.rmu.RLock()
	defer s.rmu.RUnlock()
	if len(b) == 0 {
		return 0, nil
	}
	for {
		if s.closed.Load() {
			return 0, &os.PathError{Op: "read", Path: "conpty", Err: os.ErrClosed}
		}
		var n uint32
		err := windows.ReadFile(s.out, b, &n, nil)
		switch {
		case err == windows.ERROR_BROKEN_PIPE:
			return int(n), io.EOF // the console is gone
		case err != nil:
			if s.closed.Load() {
				err = os.ErrClosed
			}
			return int(n), &os.PathError{Op: "read", Path: "conpty", Err: err}
		case n > 0:
			s.lastOut.Store(time.Now().UnixNano())
			return int(n), nil
		}
		// A zero-byte read is a zero-byte write on the other end.
	}
}

func (s *sysPTY) write(b []byte) (int, error) {
	s.wmu.RLock()
	defer s.wmu.RUnlock()
	total := 0
	for len(b) > 0 {
		if s.closed.Load() {
			return total, &os.PathError{Op: "write", Path: "conpty", Err: os.ErrClosed}
		}
		var n uint32
		err := windows.WriteFile(s.in, b, &n, nil)
		total += int(n)
		b = b[n:]
		if err != nil {
			if s.closed.Load() {
				err = os.ErrClosed
			}
			return total, &os.PathError{Op: "write", Path: "conpty", Err: err}
		}
	}
	return total, nil
}

func (s *sysPTY) resize(cols, rows, _, _ int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hpc == 0 {
		return os.ErrClosed
	}
	if r, _, _ := procResizePseudoConsole.Call(uintptr(s.hpc), coord(cols, rows)); r != 0 {
		return fmt.Errorf("pty: ResizePseudoConsole: HRESULT %#x", uint32(r))
	}
	return nil
}

// closeConsole closes the pseudo console, once. The console then ends
// its programs, writes what output is left and closes its pipe ends.
// On Windows versions before 11 24H2 ClosePseudoConsole waits until
// that output has been read.
func (s *sysPTY) closeConsole() {
	s.mu.Lock()
	h := s.hpc
	s.hpc = 0
	s.mu.Unlock()
	if h != 0 {
		procClosePseudoConsole.Call(uintptr(h))
	}
}

func (s *sysPTY) close() error {
	s.closed.Store(true)
	// Cancel a pending ReadFile and WriteFile; one that has not started
	// yet sees closed, or fails once the console is gone.
	windows.CancelIoEx(s.out, nil)
	windows.CancelIoEx(s.in, nil)
	// Read the console's last output ourselves, so that closing it
	// cannot block on a full pipe, until it closes its end.
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		buf := make([]byte, 4096)
		for {
			var n uint32
			if err := windows.ReadFile(s.out, buf, &n, nil); err != nil {
				return
			}
		}
	}()
	s.closeConsole()
	<-drained

	s.wmu.Lock()
	err := windows.CloseHandle(s.in)
	s.wmu.Unlock()
	s.rmu.Lock()
	if err2 := windows.CloseHandle(s.out); err == nil {
		err = err2
	}
	s.rmu.Unlock()
	if err != nil {
		return os.NewSyscallError("CloseHandle", err)
	}
	return nil
}

// coord packs a COORD{X: cols, Y: rows}, passed by value, into a
// register.
func coord(cols, rows int) uintptr {
	return uintptr(uint16(cols)) | uintptr(uint16(rows))<<16
}

// envBlock returns env as a Unicode environment block: NUL-terminated
// "key=value" strings followed by an extra NUL.
func envBlock(env []string) ([]uint16, error) {
	var b []uint16
	for _, kv := range env {
		for i := 0; i < len(kv); i++ {
			if kv[i] == 0 {
				return nil, fmt.Errorf("pty: environment variable contains NUL: %q", kv)
			}
		}
		b = append(b, utf16.Encode([]rune(kv))...)
		b = append(b, 0)
	}
	if len(b) == 0 {
		b = append(b, 0) // an empty block still has two NULs
	}
	return append(b, 0), nil
}

func closeHandles(hs ...windows.Handle) {
	for _, h := range hs {
		windows.CloseHandle(h)
	}
}

// DefaultShell returns the first of pwsh, powershell and cmd found in
// PATH, else %ComSpec%, else cmd.exe in the system directory, with the
// arguments to pass as Options.Args.
func DefaultShell() (path string, args []string) {
	for _, name := range []string{"pwsh", "powershell", "cmd"} {
		if p, err := exec.LookPath(name + ".exe"); err == nil {
			return p, []string{name}
		}
	}
	if p := os.Getenv("ComSpec"); p != "" {
		return p, []string{"cmd"}
	}
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		dir = `C:\Windows\System32`
	}
	return filepath.Join(dir, "cmd.exe"), []string{"cmd"}
}
