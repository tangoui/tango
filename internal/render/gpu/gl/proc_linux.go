//go:build linux && !android

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
	// libepoxy lazily opens a desktop GL dispatch library on its first GL
	// call and aborts the process if neither exists. EGL/Mesa alone can
	// create a context, so check before calling any of epoxy's stubs. Keep
	// the handle open for the lifetime of the process, like libepoxy itself.
	var desktop uintptr
	var lastErr error
	for _, name := range []string{"libOpenGL.so.0", "libGL.so.1"} {
		desktop, lastErr = purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if lastErr == nil {
			break
		}
	}
	if desktop == 0 {
		return 0, fmt.Errorf("gl: %w: no desktop GL dispatch library (libOpenGL.so.0 or libGL.so.1): %v", ErrNoDevice, lastErr)
	}
	lib, err := purego.Dlopen("libepoxy.so.0", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		purego.Dlclose(desktop)
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
