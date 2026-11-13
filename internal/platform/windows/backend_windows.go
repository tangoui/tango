//go:build windows && (amd64 || arm64)

package windows

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/tangoui/tango/internal/platform"
)

// Backend is the Win32 + WebView2 implementation of platform.Backend.
//
// Everything runs on the thread that called Init and Run: the Win32
// message loop, every window procedure and all WebView2 callbacks. The
// only cross-thread entry points are Wake and runDeferred, which post a
// message to the hidden application window.
type Backend struct {
	h    platform.Host
	opts platform.AppOptions

	appHwnd atomic.Uintptr // hidden top-level window: Wake, deferred work, session messages
	woken   atomic.Bool    // a wmAppWake is queued; further Wakes are folded into it

	surfaces map[uintptr]*surface // by HWND
	byID     map[platform.SurfaceID]*surface
	creating *surface // inside its CreateWindowExW, before the HWND is known

	quitting   bool
	terminated bool
	active     bool // the app is in the foreground (WM_ACTIVATEAPP)

	// The WebView2 environment, shared by all surfaces, is created
	// asynchronously on first need; envWaiters run when it is done.
	env        uintptr
	envErr     error
	envStarted bool
	envWaiters []func()

	deferredMu sync.Mutex
	deferred   []func()

	defaultMenu *menuSpec // SetAppMenu(0, ...): current and future surfaces
}

var (
	_ platform.Backend   = (*Backend)(nil)
	_ platform.Clipboard = (*Backend)(nil)
	_ platform.Dialogs   = (*Backend)(nil)
	_ platform.AppMenu   = (*Backend)(nil)
)

// theBackend is the process's backend; the window procedure, a plain
// function, finds it here.
var theBackend *Backend

// New returns the Windows backend.
func New() platform.Backend {
	b := new(Backend)
	b.surfaces = make(map[uintptr]*surface)
	b.byID = make(map[platform.SurfaceID]*surface)
	return b
}

// Window classes: one for the hidden application window, one for
// surfaces. Both use wndProc, which routes by HWND.
const (
	appClass     = "TangoApp"
	surfaceClass = "TangoSurface"
)

const csHRedraw, csVRedraw = 0x0002, 0x0001 // CS_HREDRAW, CS_VREDRAW

// registerClasses registers both classes the first time it is called and
// returns the outcome of that attempt ever after.
var registerClasses = sync.OnceValue(func() error {
	proc := syscall.NewCallback(wndProc)
	arrow, _, _ := procLoadCursorW.Call(0, idcArrow)
	// Icon resource 1 of the executable, which tan build embeds; 0 (the
	// default icon) when there is none.
	appIcon, _, _ := procLoadIconW.Call(instance(), 1)
	for _, class := range [...]string{appClass, surfaceClass} {
		wc := wndClassEx{
			Size:      uint32(unsafe.Sizeof(wndClassEx{})),
			Style:     csHRedraw | csVRedraw,
			WndProc:   proc,
			Instance:  instance(),
			Icon:      appIcon,
			Cursor:    arrow,
			ClassName: u16(class),
			IconSm:    appIcon,
		}
		atom, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
		if atom == 0 {
			return fmt.Errorf("tango: RegisterClassExW(%s): %v", class, err)
		}
	}
	return nil
})

// prepareProcess sets the process-wide state the backend relies on before
// any window exists.
func prepareProcess() {
	// WebView2 and the shell's file dialogs require a single-threaded
	// apartment on the UI thread.
	procCoInitializeEx.Call(0, coinitApartmentThreaded)

	// Prefer Per-Monitor V2 (Windows 10 1703 and later), else system DPI
	// awareness. Either call is a no-op if a manifest already decided.
	switch {
	case has(procSetProcessDpiAwarenessContext):
		procSetProcessDpiAwarenessContext.Call(dpiAwarenessContextPerMonitorAwareV2)
	case has(procSetProcessDPIAware):
		procSetProcessDPIAware.Call()
	}

	activateCommonControls()
}

func (b *Backend) Init(h platform.Host, opts platform.AppOptions) error {
	if h == nil {
		return errors.New("tango: nil Host")
	}
	b.h, b.opts = h, opts
	theBackend = b
	prepareProcess()
	if err := registerClasses(); err != nil {
		return err
	}
	// The application window is never shown; it receives the backend's
	// private messages and the session and system notifications.
	hwnd, _, err := procCreateWindowExW.Call(0,
		uintptr(unsafe.Pointer(u16(appClass))), uintptr(unsafe.Pointer(u16(opts.Name))),
		wsOverlapped, 0, 0, 0, 0, 0, 0, instance(), 0)
	if hwnd == 0 {
		return fmt.Errorf("tango: cannot create the application window: %v", err)
	}
	b.appHwnd.Store(hwnd)
	b.initSystemState() // integration_windows.go
	b.initSysx()        // sysx_com_windows.go: process AppUserModelID
	// A Wake that came before the window existed could not be posted.
	if b.woken.Load() {
		postMessage(hwnd, wmAppWake, 0, 0)
	}
	return nil
}

func (b *Backend) Run() error {
	postMessage(b.appHwnd.Load(), wmAppStarted, 0, 0)
	for b.Step(true) {
	}
	b.terminate()
	return nil
}

// terminate reports LifecycleTerminating, at most once.
func (b *Backend) terminate() {
	if b.terminated {
		return
	}
	b.terminated = true
	b.h.OnLifecycle(platform.LifecycleTerminating)
	removeAllTrays() // tray_windows.go: icons outlive the process otherwise
}

// Quit asks the loop to end with WM_QUIT. A modal loop running at that
// moment (dialog, menu) ends too and passes WM_QUIT on to the outer loop.
func (b *Backend) Quit() { procPostQuitMessage.Call(0) }

// Wake requests a Host.Drain on the UI thread. Only one wmAppWake is
// queued at a time; Wakes until its Drain begins share it.
func (b *Backend) Wake() {
	if !b.woken.CompareAndSwap(false, true) {
		return // already queued
	}
	hwnd := b.appHwnd.Load()
	if hwnd == 0 {
		return // before Init: Init posts it
	}
	if !postMessage(hwnd, wmAppWake, 0, 0) {
		b.woken.Store(false) // the queue is full; a later Wake retries
	}
}

// Step handles queued messages: with block, waits for and dispatches one;
// without, dispatches all that are queued. It returns false once WM_QUIT
// arrives (or GetMessageW fails) and keeps returning false.
func (b *Backend) Step(block bool) bool {
	if b.quitting {
		return false
	}
	var m msg
	if block {
		got, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(got) > 0 {
			b.dispatch(&m)
			return true
		}
		b.quitting = true // 0: WM_QUIT, -1: error
		return false
	}
	for {
		got, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmRemove)
		switch {
		case got == 0:
			return true // queue empty
		case m.Message == wmQuit:
			b.quitting = true
			return false
		}
		b.dispatch(&m)
	}
}

// dispatch delivers one message. Menu accelerators of the surface that
// contains the target window are tried first.
func (b *Backend) dispatch(m *msg) {
	if b.translateAccelerator(m) {
		return
	}
	procTranslateMessage.Call(uintptr(unsafe.Pointer(m)))
	procDispatchMessageW.Call(uintptr(unsafe.Pointer(m)))
}

func (b *Backend) translateAccelerator(m *msg) bool {
	if m.HWnd == 0 {
		return false
	}
	top, _, _ := procGetAncestor.Call(m.HWnd, gaRoot)
	s := b.surfaces[top]
	if s == nil || s.accel == 0 {
		return false
	}
	used, _, _ := procTranslateAcceleratorW.Call(top, s.accel, uintptr(unsafe.Pointer(m)))
	return used != 0
}

func (b *Backend) AssetOrigin() string { return assetOrigin }

// runDeferred queues fn to run on the UI thread from a later message,
// after the current call has returned. Modal dialogs start this way so
// the dialog API stays asynchronous.
func (b *Backend) runDeferred(fn func()) {
	b.deferredMu.Lock()
	b.deferred = append(b.deferred, fn)
	b.deferredMu.Unlock()
	postMessage(b.appHwnd.Load(), wmAppDeferred, 0, 0)
}

// nextDeferred takes the oldest queued function, or nil.
func (b *Backend) nextDeferred() func() {
	b.deferredMu.Lock()
	defer b.deferredMu.Unlock()
	if len(b.deferred) == 0 {
		return nil
	}
	fn := b.deferred[0]
	b.deferred = b.deferred[1:]
	return fn
}

// drainDeferred runs queued functions until none is left. One of them may
// run a modal loop that dispatches more messages, including another
// wmAppDeferred, so the queue is re-read after each.
func (b *Backend) drainDeferred() {
	for fn := b.nextDeferred(); fn != nil; fn = b.nextDeferred() {
		fn()
	}
}

// setActive reports a change of app activation once, whichever top-level
// window got the WM_ACTIVATEAPP.
func (b *Backend) setActive(on bool) {
	if on == b.active {
		return
	}
	b.active = on
	ev := platform.LifecycleBackground
	if on {
		ev = platform.LifecycleForeground
	}
	b.h.OnLifecycle(ev)
}

// wndProc is the window procedure of both classes. Messages for the app
// window and for known surfaces go to their handlers; a surface being
// created gets its HWND bound on its first message, which arrives inside
// CreateWindowExW. Whatever is not handled goes to DefWindowProcW.
func wndProc(hwnd, m, wp, lp uintptr) uintptr {
	if r, done := route(hwnd, uint32(m), wp, lp); done {
		return r
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

func route(hwnd uintptr, m uint32, wp, lp uintptr) (uintptr, bool) {
	b := theBackend
	switch {
	case b == nil:
		return 0, false
	case hwnd == b.appHwnd.Load():
		return b.appMessage(m, wp, lp)
	}
	if s := b.surfaces[hwnd]; s != nil {
		return s.message(m, wp, lp)
	}
	if s := b.creating; s != nil && s.hwnd == 0 {
		s.hwnd = hwnd
		b.surfaces[hwnd] = s
		return s.message(m, wp, lp)
	}
	return 0, false
}

// appMessage handles the application window's messages.
func (b *Backend) appMessage(m uint32, wp, lp uintptr) (uintptr, bool) {
	switch m {
	case wmAppWake:
		b.woken.Store(false) // Wakes from here on queue a new message
		b.h.Drain()
	case wmAppStarted:
		b.h.OnLifecycle(platform.LifecycleStarted)
	case wmAppDeferred:
		b.drainDeferred()
	case wmActivateApp:
		b.setActive(wp != 0)
	case wmQueryEndSession:
		return 1, true // agree to end the session
	case wmEndSession:
		if wp != 0 {
			b.terminate() // the process is ended once this returns
		}
	case wmSettingChange, wmDisplayChange, wmDwmColorizationColorChanged:
		b.systemMessage(m, wp, lp) // integration_windows.go
		return 0, false            // DefWindowProc sees them as well
	default:
		return 0, false
	}
	return 0, true
}

// startEnvironment starts creating the shared WebView2 environment unless
// that already began. Its profile lives in userDataDir, keyed by app ID or
// name (the executable's file name if the app has none).
func (b *Backend) startEnvironment() error {
	if b.envStarted {
		return nil
	}
	var exeDir, exeName string
	if exe, err := os.Executable(); err == nil {
		exeDir, exeName = filepath.Dir(exe), filepath.Base(exe)
	}
	name := b.opts.Name
	if name == "" {
		name = exeName
	}
	dir := userDataDir(os.Getenv, exeDir, b.opts.ID, name)
	_ = os.MkdirAll(dir, 0o755) // WebView2 reports a usable error if this failed
	err := createEnvironment(dir, b.environmentReady)
	// After a failure nothing is remembered, so a runtime installed in the
	// meantime is found by the next surface.
	b.envStarted = err == nil
	return err
}

// environmentReady records the outcome of environment creation and runs
// whatever was waiting for it.
func (b *Backend) environmentReady(env uintptr, err error) {
	b.env, b.envErr = env, err
	if err != nil {
		log.Print(err)
	}
	waiting := b.envWaiters
	b.envWaiters = nil
	for _, fn := range waiting {
		fn()
	}
}

// whenEnvironment runs fn as soon as the environment is created or has
// failed: now if that is already known.
func (b *Backend) whenEnvironment(fn func()) {
	if b.env == 0 && b.envErr == nil {
		b.envWaiters = append(b.envWaiters, fn)
		return
	}
	fn()
}

// actCtxW is ACTCTXW (winbase.h).
type actCtxW struct {
	cbSize                 uint32
	dwFlags                uint32
	lpSource               *uint16
	wProcessorArchitecture uint16
	wLangID                uint16
	lpAssemblyDirectory    *uint16
	lpResourceName         uintptr // MAKEINTRESOURCE value
	lpApplicationName      *uint16
	hModule                uintptr
}

// activateCommonControls makes comctl32 version 6, and with it task
// dialogs, available to executables whose manifest does not request it.
// It creates and activates an activation context from a manifest that
// shell32.dll embeds for exactly this purpose (resource ID 124, which
// names the v6 common controls assembly). Without CreateActCtxW, or if
// the context cannot be created, nothing changes.
func activateCommonControls() {
	if !has(procCreateActCtxW) {
		return
	}
	const (
		actctxFlagAssemblyDirectoryValid = 0x004 // ACTCTX_FLAG_ASSEMBLY_DIRECTORY_VALID
		actctxFlagResourceNameValid      = 0x008 // ACTCTX_FLAG_RESOURCE_NAME_VALID
		shell32CommonControlsManifest    = 124
		invalidHandleValue               = ^uintptr(0)
	)
	ctx := actCtxW{
		cbSize:              uint32(unsafe.Sizeof(actCtxW{})),
		dwFlags:             actctxFlagAssemblyDirectoryValid | actctxFlagResourceNameValid,
		lpSource:            u16(inSystemDir("shell32.dll")),
		lpAssemblyDirectory: u16(sysDir),
		lpResourceName:      shell32CommonControlsManifest,
	}
	handle, _, _ := procCreateActCtxW.Call(uintptr(unsafe.Pointer(&ctx)))
	if handle == invalidHandleValue {
		return
	}
	// The context stays active for the life of the thread; the cookie
	// would only be needed to deactivate it.
	var cookie uintptr
	procActivateActCtx.Call(handle, uintptr(unsafe.Pointer(&cookie)))
}
