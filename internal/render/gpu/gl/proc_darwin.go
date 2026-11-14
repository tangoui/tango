//go:build darwin

package gl

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

const openGLFramework = "/System/Library/Frameworks/OpenGL.framework/OpenGL"

// openGL returns the handle of OpenGL.framework, which exports the GL 4.1
// core entry points and CGL directly. The handle is the loader's, not GL
// state: function tables stay per context.
var openGL = sync.OnceValues(func() (uintptr, error) {
	lib, err := purego.Dlopen(openGLFramework, purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return 0, fmt.Errorf("gl: %w: %v", ErrNoDevice, err)
	}
	return lib, nil
})

// glProc returns the GL function lookup for glFuncs.load: the framework's
// symbol of that name.
func glProc() (func(name string) uintptr, error) {
	lib, err := openGL()
	if err != nil {
		return nil, err
	}
	return func(name string) uintptr {
		a, err := purego.Dlsym(lib, name)
		if err != nil {
			return 0
		}
		return a
	}, nil
}
