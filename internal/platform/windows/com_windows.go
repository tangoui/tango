//go:build windows && (amd64 || arm64)

package windows

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

// COM from Go without cgo. A COM interface pointer addresses an object
// whose first word points at an array of function pointers, the vtable.
// Methods are numbered by their position in that array, IUnknown's three
// first; the per-interface slot constants in the *_windows.go files follow
// the declaration order of the SDK headers. Every method receives the
// interface pointer as a hidden first argument and nearly all return an
// HRESULT.

// IUnknown slots, shared by every interface.
const (
	unkQueryInterface = iota
	unkAddRef
	unkRelease
	unkFirstOwn // first slot an interface declares itself
)

// ISequentialStream slots (objidlbase.h); IStream starts with them.
const (
	seqStreamRead = unkFirstOwn + iota
	seqStreamWrite
)

const sFalse = 1 // S_FALSE: success, but "no more" / "nothing done"

// comHeader is the start of every COM object.
type comHeader struct {
	vtbl *uintptr // first vtable entry
}

// comMethod returns the function in vtable slot of the object at obj.
func comMethod(obj uintptr, slot int) uintptr {
	h := (*comHeader)(native(obj))
	return unsafe.Slice(h.vtbl, slot+1)[slot]
}

// comCall calls vtable slot i of obj with args after the interface pointer
// and returns the method's result (an HRESULT for most methods). Like
// sysProc.Call it is go:uintptrescapes, so uintptr(unsafe.Pointer(&x))
// written in the argument list keeps x valid for the call.
//
//go:uintptrescapes
func comCall(obj uintptr, i int, args ...uintptr) uintptr {
	var frame [16]uintptr // this + up to 15 arguments, on the stack
	if len(args) >= len(frame) {
		panic("tango: comCall with too many arguments")
	}
	frame[0] = obj
	n := 1 + copy(frame[1:], args)
	result, _, _ := syscall.SyscallN(comMethod(obj, i), frame[:n]...)
	return result
}

// failed reports whether an HRESULT is an error (its sign bit is set).
func failed(hr uintptr) bool { return uint32(hr)&0x80000000 != 0 }

// hresultError describes a failed call.
func hresultError(what string, hr uintptr) error {
	return fmt.Errorf("tango: %s failed (HRESULT %#08x)", what, uint32(hr))
}

// addRef and release adjust obj's reference count; 0 is ignored.
func addRef(obj uintptr) {
	if obj != 0 {
		comCall(obj, unkAddRef)
	}
}

func release(obj uintptr) {
	if obj != 0 {
		comCall(obj, unkRelease)
	}
}

// queryInterface asks obj for interface iid and returns the new reference,
// or 0 if obj is 0 or does not implement it.
func queryInterface(obj uintptr, iid *GUID) uintptr {
	if obj == 0 {
		return 0
	}
	var itf uintptr
	if hr := comCall(obj, unkQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&itf))); failed(hr) {
		return 0
	}
	return itf
}

// comOut calls a method whose only argument is an out parameter of type T
// (a property getter such as get_IsSuccess(BOOL*)) and returns the value
// written and the HRESULT. The value is T's zero value if the method
// wrote nothing.
func comOut[T any](obj uintptr, slot int) (T, uintptr) {
	var v T
	hr := comCall(obj, slot, uintptr(unsafe.Pointer(&v)))
	return v, hr
}

// comString calls a property getter of the form get_X(LPWSTR*) and
// returns the string, freeing the callee's CoTaskMemAlloc buffer.
func comString(obj uintptr, i int) string {
	var p uintptr
	if hr := comCall(obj, i, uintptr(unsafe.Pointer(&p))); failed(hr) {
		return ""
	}
	return takeWstr(p)
}

// ---- Callback objects ----
//
// WebView2 reports events and completions to objects the caller supplies,
// each implementing one interface: IUnknown plus a single Invoke method
// with two arguments (sender/args, or result/value). Every such interface
// has the same shape, so one Go implementation serves them all.
//
// Callback objects live in a slab: fixed-size chunks of cbObject that are
// allocated as needed and never freed or moved, so an object's address
// stays valid memory for the life of the process. COM sees an object's
// vtable pointer; the chunks keep the Go side (the invoke closure)
// reachable. A slot is in use while its count is positive; at zero its
// closure is dropped and the slot goes back on the free list. Turning an
// address from COM back into an object is a range check against the
// chunks, so even a stray call with a stale pointer reads only slab
// memory. The vtable's four functions are created once, on first use, and
// shared by all objects.

type cbObject struct {
	vtbl   *[4]uintptr // QueryInterface, AddRef, Release, Invoke
	refs   int32
	invoke func(a, b uintptr)
}

const cbChunkLen = 64

type cbChunk [cbChunkLen]cbObject

var (
	cbVtblOnce sync.Once
	cbVtbl     [4]uintptr

	cbMu     sync.Mutex
	cbChunks []*cbChunk  // every chunk ever allocated
	cbFree   []*cbObject // unused slots, most recently freed last
)

// cbRefused lists interfaces a callback object must deny. Answering them
// with itself would let COM marshal the object to other threads or treat
// it as free-threaded, which it is not.
var cbRefused = [...]GUID{
	guid("00000003-0000-0000-C000-000000000046"), // IMarshal
	guid("94EA2B94-E9CC-49E0-C0FF-EE64CA8F5B90"), // IAgileObject
	guid("ECC8691B-C1DB-4DC0-855E-65F6C551AF49"), // INoMarshal
}

// cbLookup returns the object in use at address this, or nil. cbMu must
// be held.
func cbLookup(this uintptr) *cbObject {
	const size = unsafe.Sizeof(cbObject{})
	for _, chunk := range cbChunks {
		base := uintptr(unsafe.Pointer(&chunk[0]))
		if this < base || this >= base+cbChunkLen*size {
			continue
		}
		if (this-base)%size != 0 {
			return nil
		}
		if o := &chunk[(this-base)/size]; o.refs > 0 {
			return o
		}
		return nil
	}
	return nil
}

// cbAdjust changes the count of the object at this by delta and returns
// the new count. At zero the slot is freed; an address that is not an
// object in use reports 0.
func cbAdjust(this uintptr, delta int32) int32 {
	cbMu.Lock()
	defer cbMu.Unlock()
	o := cbLookup(this)
	if o == nil {
		return 0
	}
	o.refs += delta
	if o.refs > 0 {
		return o.refs
	}
	o.refs, o.invoke = 0, nil
	cbFree = append(cbFree, o)
	return 0
}

func cbQueryInterface(this, riid, out uintptr) uintptr {
	want := *(*GUID)(native(riid))
	result := (*uintptr)(native(out))
	for _, refused := range cbRefused {
		if want == refused {
			*result = 0
			return eNoInterface
		}
	}
	// Every other request, IUnknown and the handler interface included,
	// is answered with the object itself.
	*result = this
	cbAdjust(this, 1)
	return sOK
}

func cbAddRef(this uintptr) uintptr { return uintptr(cbAdjust(this, 1)) }

func cbRelease(this uintptr) uintptr { return uintptr(cbAdjust(this, -1)) }

func cbInvoke(this, a, b uintptr) uintptr {
	var fn func(a, b uintptr)
	cbMu.Lock()
	if o := cbLookup(this); o != nil {
		fn = o.invoke
	}
	cbMu.Unlock()
	if fn != nil {
		fn(a, b) // outside the lock: it may create or release handlers
	}
	return sOK
}

// newHandler returns a new callback object running fn on Invoke. The
// caller owns the one reference it starts with and releases it once COM
// has taken its own.
func newHandler(fn func(a, b uintptr)) uintptr {
	cbVtblOnce.Do(func() {
		cbVtbl = [4]uintptr{
			syscall.NewCallback(cbQueryInterface),
			syscall.NewCallback(cbAddRef),
			syscall.NewCallback(cbRelease),
			syscall.NewCallback(cbInvoke),
		}
	})
	cbMu.Lock()
	defer cbMu.Unlock()
	if len(cbFree) == 0 {
		chunk := new(cbChunk)
		cbChunks = append(cbChunks, chunk)
		for i := cbChunkLen - 1; i >= 0; i-- {
			cbFree = append(cbFree, &chunk[i])
		}
	}
	o := cbFree[len(cbFree)-1]
	cbFree = cbFree[:len(cbFree)-1]
	*o = cbObject{vtbl: &cbVtbl, refs: 1, invoke: fn}
	return uintptr(unsafe.Pointer(o))
}

// withHandler hands a new callback object to call (which AddRefs it if it
// keeps it), drops the creation reference afterwards and returns call's
// result.
func withHandler(fn func(a, b uintptr), call func(h uintptr) uintptr) uintptr {
	h := newHandler(fn)
	defer release(h)
	return call(h)
}

// readStream reads stream from its current position to the end.
func readStream(stream uintptr) []byte {
	var data []byte
	chunk := make([]byte, 64<<10)
	for {
		var got uint32
		hr := comCall(stream, seqStreamRead,
			uintptr(unsafe.Pointer(&chunk[0])), uintptr(len(chunk)), uintptr(unsafe.Pointer(&got)))
		data = append(data, chunk[:got]...)
		if got == 0 || hr == sFalse || failed(hr) {
			return data
		}
	}
}

// memStream copies data into a new memory-backed IStream; 0 on failure.
func memStream(data []byte) uintptr {
	var first *byte
	if len(data) != 0 {
		first = &data[0]
	}
	stream, _, _ := procSHCreateMemStream.Call(uintptr(unsafe.Pointer(first)), uintptr(len(data)))
	return stream
}
