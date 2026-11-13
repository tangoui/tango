//go:build windows && (amd64 || arm64)

package windows

import (
	"sync"
	"sync/atomic"
	"syscall"
)

// Native entry points are declared up front, one sysLib per DLL and one
// sysProc per exported function, and resolved on first use. Nothing is
// loaded while the package initializes beyond what systemDirectory needs.
//
// The two types mirror what syscall.LazyDLL / LazyProc offer to the rest of
// the package (NewProc, Find, Call with the same signature and panics) and
// add an inventory: every declared function is recorded on its library,
// so a test can check every name against the Windows SDK and that it
// resolves on the running system.

// sysLib is a DLL loaded on demand. A failed load is not remembered: the
// next use tries again, as syscall.LazyDLL does.
type sysLib struct {
	path string // file name for KnownDLLs, absolute path otherwise

	mu     sync.Mutex
	loaded atomic.Pointer[syscall.DLL]
	procs  []*sysProc // inventory, in declaration order
}

// sysLibs lists every library created with newSysLib.
var sysLibs []*sysLib

func newSysLib(path string) *sysLib {
	l := &sysLib{path: path}
	sysLibs = append(sysLibs, l)
	return l
}

// load returns the loaded DLL, loading it if this is the first use.
func (l *sysLib) load() (*syscall.DLL, error) {
	if d := l.loaded.Load(); d != nil {
		return d, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if d := l.loaded.Load(); d != nil {
		return d, nil
	}
	d, err := syscall.LoadDLL(l.path)
	if err != nil {
		return nil, err
	}
	l.loaded.Store(d)
	return d, nil
}

// export declares the function name of l. The returned value can be
// stored in a package variable; it resolves nothing until used.
func (l *sysLib) export(name string) *sysProc {
	p := &sysProc{lib: l, name: name}
	l.mu.Lock()
	l.procs = append(l.procs, p)
	l.mu.Unlock()
	return p
}

// NewProc is export under the name syscall.LazyDLL uses, for the files
// that declare their own functions against the shared libraries.
func (l *sysLib) NewProc(name string) *sysProc { return l.export(name) }

// declared returns a snapshot of l's inventory.
func (l *sysLib) declared() []*sysProc {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]*sysProc(nil), l.procs...)
}

// sysProc is one exported function of a sysLib. Like the library, a failed
// lookup is retried on the next use.
type sysProc struct {
	lib  *sysLib
	name string

	mu   sync.Mutex
	addr atomic.Uintptr
}

// Find loads the library and looks the function up, reporting why it is
// unavailable (a *syscall.DLLError) or nil.
func (p *sysProc) Find() error {
	if p.addr.Load() != 0 {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.addr.Load() != 0 {
		return nil
	}
	d, err := p.lib.load()
	if err != nil {
		return err
	}
	fn, err := d.FindProc(p.name)
	if err != nil {
		return err
	}
	p.addr.Store(fn.Addr())
	return nil
}

// Available reports whether the function exists on this system; optional
// APIs (newer than the oldest supported Windows) are guarded with it.
func (p *sysProc) Available() bool { return p.Find() == nil }

// Call invokes the function with the given arguments and returns its two
// result registers and the thread's last error, wrapped as a
// syscall.Errno exactly as syscall.LazyProc.Call does. It panics when the
// function cannot be found, so optional functions must be checked with
// Available (or has) first.
//
// go:uintptrescapes makes uintptr(unsafe.Pointer(&x)) arguments written in
// the call keep x alive and in place until Call returns.
//
//go:uintptrescapes
func (p *sysProc) Call(args ...uintptr) (r1, r2 uintptr, lastErr error) {
	if err := p.Find(); err != nil {
		panic(err)
	}
	r1, r2, errno := syscall.SyscallN(p.addr.Load(), args...)
	return r1, r2, errno
}

// finder is what has needs: both sysProc and syscall.LazyProc (still used
// for DLLs declared with systemDLL) satisfy it.
type finder interface{ Find() error }

// has reports whether an optional function can be called.
func has(p finder) bool { return p.Find() == nil }
