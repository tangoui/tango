//go:build windows && (amd64 || arm64)

package d3d11

import (
	"syscall"
	"unsafe"
)

// COM from Go without cgo. Every method receives the interface pointer as
// a hidden first argument; the slot numbers come from the vt* layouts in
// abi.go. Structures larger than 8 bytes are always passed by pointer, as
// the COM methods used here declare them, and no method used here takes a
// float by value (syscall.SyscallN only fills integer registers).

// comCall calls vtable slot of obj with args after the interface pointer
// and returns the method's result register (an HRESULT for most methods;
// only its low 32 bits are meaningful then).
//
// go:uintptrescapes keeps x alive and in place for the call when an
// argument is written uintptr(unsafe.Pointer(&x)) in the call expression.
// Pointers converted earlier, or stored inside structures passed by
// pointer, must be kept alive by the caller (runtime.KeepAlive).
//
//go:uintptrescapes
func comCall(obj uintptr, slot int, args ...uintptr) uintptr {
	var frame [16]uintptr // this + up to 15 arguments
	if len(args) >= len(frame) {
		panic("d3d11: comCall with too many arguments")
	}
	frame[0] = obj
	n := 1 + copy(frame[1:], args)
	r, _, _ := syscall.SyscallN(comMethod(obj, slot), frame[:n]...)
	return r
}

// comMethod returns the function in vtable slot of the object at obj.
func comMethod(obj uintptr, slot int) uintptr {
	if obj == 0 {
		panic("d3d11: COM call on a nil interface")
	}
	vtbl := *(**uintptr)(foreign(obj))
	return unsafe.Slice(vtbl, slot+1)[slot]
}

// foreign converts the address of memory Windows owns (a COM object, a
// blob's buffer, a mapped subresource) to an unsafe.Pointer. The memory is
// outside the Go heap, so the conversion is sound; going through a
// variable keeps go vet's unsafeptr check, meant for Go memory, quiet.
func foreign(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

// release drops one reference to obj; 0 is ignored.
func release(obj uintptr) {
	if obj != 0 {
		comCall(obj, unkRelease)
	}
}

// addRef adds a reference to obj; 0 is ignored.
func addRef(obj uintptr) {
	if obj != 0 {
		comCall(obj, unkAddRef)
	}
}

// queryInterface asks obj for interface iid and returns the new reference.
func queryInterface(obj uintptr, iid *guid, what string) (uintptr, error) {
	var itf uintptr
	hr := comCall(obj, unkQueryInterface, uintptr(unsafe.Pointer(iid)), uintptr(unsafe.Pointer(&itf)))
	if hrFailed(hr) {
		return 0, hrError("QueryInterface("+what+")", hr)
	}
	return itf, nil
}

// cstr returns s as a NUL-terminated byte string.
func cstr(s string) []byte {
	b := make([]byte, len(s)+1)
	copy(b, s)
	return b
}
