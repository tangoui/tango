// Package gl is the OpenGL backend of the GPU renderer: a gpu.Device that
// executes internal/render/gpu frames with OpenGL 3.3 core and a presenter that shows
// them in a GTK3 GtkGLArea.
//
// It calls OpenGL through purego without cgo. On Linux the GL entry
// points come from libepoxy (libepoxy.so.0), which dispatches to GLX or
// EGL, whichever context is current; headless devices (tests, readback)
// create an EGL surfaceless context through libEGL.so.1. On darwin a CGL
// context on OpenGL.framework (4.1 core) serves tests and local
// verification only: applications on macOS render with internal/render/gpu/metal. The
// quad shader (internal/render/gpu/gen.QuadGLSLVertex and QuadGLSLFragment) is compiled
// by the driver at run time.
//
// Every GL object and the function table belong to a context: nothing GL
// is kept in package variables, so a lost context (Android's
// EGL_CONTEXT_LOST) is replaced by creating a new context and device. GL
// calls run only on the thread the context is current on: a headless
// context owns a goroutine locked to its OS thread and runs every call
// there.
//
// The backend exists on linux and darwin. Everywhere else NewDevice fails
// with ErrUnsupported.
package gl

import "errors"

var (
	// ErrUnsupported is returned on platforms without the OpenGL backend
	// (anything but linux and darwin).
	ErrUnsupported = errors.New("gl: OpenGL is not available on this platform")
	// ErrNoDevice is wrapped by the error returned when no OpenGL 3.3 core
	// context could be created: libepoxy, libEGL or the driver is missing,
	// or the driver offers less than OpenGL 3.3. Use the CPU renderer.
	ErrNoDevice = errors.New("gl: no OpenGL 3.3 device")
	// ErrClosed is returned by calls on a device or presenter after Close.
	ErrClosed = errors.New("gl: use after Close")
)
