//go:build windows && (amd64 || arm64)

package windows

// Shared plumbing for the desktop system capabilities (sysx_*.go): COM
// objects implemented in Go with their own vtables, CoCreateInstance,
// WinRT strings and activation, and the process AppUserModelID.

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	combase = systemDLL("combase.dll")

	sxProcOleInitialize                     = ole32.NewProc("OleInitialize")
	sxProcRoInitialize                      = combase.NewProc("RoInitialize")
	sxProcRoActivateInstance                = combase.NewProc("RoActivateInstance")
	sxProcRoGetActivationFactory            = combase.NewProc("RoGetActivationFactory")
	sxProcWindowsCreateString               = combase.NewProc("WindowsCreateString")
	sxProcWindowsDeleteString               = combase.NewProc("WindowsDeleteString")
	sxProcSetCurrentProcessExplicitAppUserM = shell32.NewProc("SetCurrentProcessExplicitAppUserModelID")
	sxProcGetAsyncKeyState                  = user32.NewProc("GetAsyncKeyState")
	sxProcGetSystemMetrics                  = user32.NewProc("GetSystemMetrics")
	sxProcCloseHandle                       = kernel32.NewProc("CloseHandle")
)

var (
	sxIIDIUnknown    = guid("00000000-0000-0000-c000-000000000046")
	sxIIDIAgileObj   = guid("94ea2b94-e9cc-49e0-c0ff-ee64ca8f5b90")
	sxIIDIMarshal    = guid("00000003-0000-0000-c000-000000000046")
	sxIIDINoMarshal  = guid("ecc8691b-c1db-4dc0-855e-65f6c551af49")
	sxIIDIInspectabl = guid("af86e2e0-b12d-4c6a-9c5a-d7aa65101e90")
)

// sxCoCreate creates an in-process COM object and returns its iid
// interface (0 on failure, with the HRESULT).
func sxCoCreate(clsid, iid *GUID) (uintptr, uintptr) {
	var out uintptr
	hr, _, _ := procCoCreateInstance.Call(uintptr(unsafe.Pointer(clsid)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if failed(hr) {
		return 0, hr
	}
	return out, hr
}

// ---- COM objects implemented in Go ----

// sxObject is a COM object whose methods are Go functions. The layout
// starts with the vtable pointer, as COM requires. Objects stay reachable
// through sxObjects until their last reference is released (Go's
// collector does not move heap objects, so the address is stable).
type sxObject struct {
	vtbl  *uintptr
	refs  int32
	iids  []GUID // interfaces answered besides IUnknown
	agile bool   // answers IAgileObject: callable from any apartment
	state any
}

var (
	sxObjectsMu sync.Mutex
	sxObjects   = map[uintptr]*sxObject{}

	sxUnknownOnce                               sync.Once
	sxQueryInterfaceCB, sxAddRefCB, sxReleaseCB uintptr
)

func sxUnknownCallbacks() {
	sxUnknownOnce.Do(func() {
		sxQueryInterfaceCB = syscall.NewCallback(func(this, riid, out uintptr) uintptr {
			o := sxObjectAt(this)
			iid := *(*GUID)(native(riid))
			ok := false
			if o != nil {
				switch {
				case iid == sxIIDIUnknown:
					ok = true
				case iid == sxIIDIAgileObj:
					ok = o.agile
				case iid == sxIIDIMarshal || iid == sxIIDINoMarshal:
					ok = false
				default:
					for _, g := range o.iids {
						if g == iid {
							ok = true
							break
						}
					}
				}
			}
			if !ok {
				*(*uintptr)(native(out)) = 0
				return eNoInterface
			}
			*(*uintptr)(native(out)) = this
			sxObjectRef(this, 1)
			return sOK
		})
		sxAddRefCB = syscall.NewCallback(func(this uintptr) uintptr { return uintptr(sxObjectRef(this, 1)) })
		sxReleaseCB = syscall.NewCallback(func(this uintptr) uintptr { return uintptr(sxObjectRef(this, -1)) })
	})
}

// sxVtable builds a vtable: IUnknown followed by methods (callbacks made
// with syscall.NewCallback; create each vtable once, callbacks are a
// limited resource).
func sxVtable(methods ...uintptr) []uintptr {
	sxUnknownCallbacks()
	return append([]uintptr{sxQueryInterfaceCB, sxAddRefCB, sxReleaseCB}, methods...)
}

func sxObjectAt(this uintptr) *sxObject {
	sxObjectsMu.Lock()
	defer sxObjectsMu.Unlock()
	return sxObjects[this]
}

func sxObjectRef(this uintptr, delta int32) int32 {
	sxObjectsMu.Lock()
	defer sxObjectsMu.Unlock()
	o := sxObjects[this]
	if o == nil {
		return 0
	}
	o.refs += delta
	if o.refs <= 0 {
		delete(sxObjects, this)
		return 0
	}
	return o.refs
}

// sxNewObject returns a new object holding one reference (the caller's).
func sxNewObject(vtbl []uintptr, iids []GUID, agile bool, state any) uintptr {
	o := &sxObject{vtbl: &vtbl[0], refs: 1, iids: iids, agile: agile, state: state}
	p := uintptr(unsafe.Pointer(o))
	sxObjectsMu.Lock()
	sxObjects[p] = o
	sxObjectsMu.Unlock()
	return p
}

// sxState returns the Go state of a Go-implemented object.
func sxState[T any](this uintptr) (T, bool) {
	var zero T
	o := sxObjectAt(this)
	if o == nil {
		return zero, false
	}
	v, ok := o.state.(T)
	return v, ok
}

// ---- Agile delegates (WinRT event handlers) ----

// sxDelegate is a WinRT delegate (TypedEventHandler) whose Invoke(sender,
// args) runs fn. WinRT raises toast events on thread-pool threads, so the
// delegate is agile; fn must hop to the main thread itself.
var (
	sxDelegateOnce sync.Once
	sxDelegateVtbl []uintptr
)

type sxDelegateState struct{ fn func(sender, args uintptr) }

func sxNewDelegate(iid GUID, fn func(sender, args uintptr)) uintptr {
	sxDelegateOnce.Do(func() {
		sxDelegateVtbl = sxVtable(syscall.NewCallback(func(this, sender, args uintptr) uintptr {
			if st, ok := sxState[*sxDelegateState](this); ok && st.fn != nil {
				st.fn(sender, args)
			}
			return sOK
		}))
	})
	return sxNewObject(sxDelegateVtbl, []GUID{iid}, true, &sxDelegateState{fn})
}

// ---- WinRT ----

var (
	sxWinRTOnce sync.Once
	sxWinRTOK   bool
)

// sxWinRT reports whether WinRT activation is available (Windows 8+) and
// initializes it on this (the main, single-threaded) apartment.
func sxWinRT() bool {
	sxWinRTOnce.Do(func() {
		if !has(sxProcRoActivateInstance) || !has(sxProcWindowsCreateString) || !has(sxProcRoGetActivationFactory) {
			return
		}
		// COM is already initialized single-threaded by Init; RoInitialize
		// then returns S_FALSE (or RPC_E_CHANGED_MODE), both fine.
		sxProcRoInitialize.Call(0) // RO_INIT_SINGLETHREADED
		sxWinRTOK = true
	})
	return sxWinRTOK
}

// sxHString creates an HSTRING; free it with sxFreeHString.
func sxHString(s string) uintptr {
	u := utf16z(s)
	var h uintptr
	hr, _, _ := sxProcWindowsCreateString.Call(uintptr(unsafe.Pointer(&u[0])), uintptr(len(u)-1), uintptr(unsafe.Pointer(&h)))
	if failed(hr) {
		return 0
	}
	return h
}

func sxFreeHString(h uintptr) {
	if h != 0 {
		sxProcWindowsDeleteString.Call(h)
	}
}

// sxActivationFactory returns class's activation factory as iid.
func sxActivationFactory(class string, iid *GUID) (uintptr, uintptr) {
	h := sxHString(class)
	defer sxFreeHString(h)
	var out uintptr
	hr, _, _ := sxProcRoGetActivationFactory.Call(h, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&out)))
	if failed(hr) {
		return 0, hr
	}
	return out, hr
}

// sxActivate creates an instance of class with its default constructor
// and returns its IInspectable.
func sxActivate(class string) (uintptr, uintptr) {
	h := sxHString(class)
	defer sxFreeHString(h)
	var out uintptr
	hr, _, _ := sxProcRoActivateInstance.Call(h, uintptr(unsafe.Pointer(&out)))
	if failed(hr) {
		return 0, hr
	}
	return out, hr
}

// ---- AppUserModelID ----

// sxAppID is the process AppUserModelID set by initSysx ("" if none).
var sxAppID string

// initSysx runs at the end of Init on the main thread. It sets the
// process's explicit AppUserModelID (the app ID, else its name), which
// toasts and the Jump List are tied to, before any window exists so the
// taskbar groups every window under it. Packagers should give the Start
// menu shortcut the same System.AppUserModel.ID.
func (b *Backend) initSysx() {
	sxAppID = sxAUMID(b.opts.ID, b.opts.Name)
	if sxAppID != "" && has(sxProcSetCurrentProcessExplicitAppUserM) {
		sxProcSetCurrentProcessExplicitAppUserM.Call(uintptr(unsafe.Pointer(u16(sxAppID))))
	}
}

// sxOle initializes OLE (drag and drop) on the main thread once. COM is
// already initialized single-threaded there; OleInitialize adds the OLE
// services on top and returns S_FALSE.
var sxOleOnce sync.Once

func sxOle() {
	sxOleOnce.Do(func() { sxProcOleInitialize.Call(0) })
}
