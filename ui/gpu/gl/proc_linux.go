//go:build linux

package gl

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// epoxy returns the handle of libepoxy, which resolves GL entry points for
// whichever context is current, GLX or EGL. The handle is the loader's,
// not GL state: function tables stay per context.
var epoxy = sync.OnceValues(func() (uintptr, error) {
	lib, err := purego.Dlopen("libepoxy.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("gl: %w: libepoxy.so.0: %v", ErrNoDevice, err)
	}
	return lib, nil
})

// glProc returns the GL function lookup for glFuncs.load. libepoxy
// exports each GL function glFoo as a variable epoxy_glFoo holding a
// function pointer; the lookup returns its value. Until the first call
// that value is libepoxy's stub, which resolves the real function for the
// current context, stores it in the variable and calls it; afterwards the
// stub sees the variable rewritten and only forwards. Calling the value
// read here is therefore right either way, costing the stub's one
// indirection.
func glProc() (func(name string) uintptr, error) {
	lib, err := epoxy()
	if err != nil {
		return nil, err
	}
	return func(name string) uintptr {
		v, err := purego.Dlsym(lib, "epoxy_"+name)
		if err != nil || v == 0 {
			return 0
		}
		return **(**uintptr)(unsafe.Pointer(&v)) // *(*uintptr)(v): the variable's value
	}, nil
}
