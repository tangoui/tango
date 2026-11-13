//go:build darwin

package gl

import "fmt"

// CGL pixel format attributes (CGLTypes.h).
const (
	kCGLPFAAllowOfflineRenderers = 96
	kCGLPFAOpenGLProfile         = 99
	kCGLOGLPVersion_3_2_Core     = 0x3200 // gives the newest core profile, 4.1
)

// headless is an offscreen GL context with its own thread: on darwin a
// CGL context of OpenGL.framework (4.1 core). It serves tests and local
// verification; applications on macOS render with ui/gpu/metal. Render
// into framebuffer objects; the context has no default framebuffer.
type headless struct {
	thread *glThread
	funcs  *glFuncs // valid on thread only

	cgl    cglFuncs
	ctx    uintptr // CGLContextObj
	closed bool
}

type cglFuncs struct {
	ChoosePixelFormat  func(attribs *int32, pix *uintptr, npix *int32) int32
	DestroyPixelFormat func(pix uintptr) int32
	CreateContext      func(pix, share uintptr, ctx *uintptr) int32
	SetCurrentContext  func(ctx uintptr) int32
	DestroyContext     func(ctx uintptr) int32
}

// newHeadlessContext creates a CGL context current on a new thread and
// loads its function table. Errors wrap ErrNoDevice.
func newHeadlessContext() (ctx *headless, err error) {
	h := &headless{thread: newGLThread()}
	h.thread.do(func() { err = h.init() })
	if err != nil {
		h.close()
		return nil, err
	}
	return h, nil
}

func (h *headless) init() error {
	lib, err := openGL()
	if err != nil {
		return err
	}
	c := &h.cgl
	if err := bindLib(lib, "OpenGL.framework", map[string]any{
		"CGLChoosePixelFormat":  &c.ChoosePixelFormat,
		"CGLDestroyPixelFormat": &c.DestroyPixelFormat,
		"CGLCreateContext":      &c.CreateContext,
		"CGLSetCurrentContext":  &c.SetCurrentContext,
		"CGLDestroyContext":     &c.DestroyContext,
	}); err != nil {
		return fmt.Errorf("%w (%w)", err, ErrNoDevice)
	}
	attribs := []int32{kCGLPFAOpenGLProfile, kCGLOGLPVersion_3_2_Core, kCGLPFAAllowOfflineRenderers, 0}
	var pix uintptr
	var npix int32
	if e := c.ChoosePixelFormat(&attribs[0], &pix, &npix); e != 0 || pix == 0 {
		return fmt.Errorf("gl: CGLChoosePixelFormat (core profile): CGL error %d: %w", e, ErrNoDevice)
	}
	e := c.CreateContext(pix, 0, &h.ctx)
	c.DestroyPixelFormat(pix)
	if e != 0 || h.ctx == 0 {
		h.ctx = 0
		return fmt.Errorf("gl: CGLCreateContext: CGL error %d: %w", e, ErrNoDevice)
	}
	if e := c.SetCurrentContext(h.ctx); e != 0 {
		return fmt.Errorf("gl: CGLSetCurrentContext: CGL error %d: %w", e, ErrNoDevice)
	}
	return h.loadFuncs()
}

// close destroys the context and stops its thread. It is idempotent.
// When the thread died (runtime.Goexit inside do) the context cannot be
// reached any more and is left to the driver.
func (h *headless) close() {
	if h.closed {
		return
	}
	h.closed = true
	if !h.thread.alive() {
		h.thread.close()
		return
	}
	h.thread.do(func() {
		if h.ctx != 0 {
			h.cgl.SetCurrentContext(0)
			h.cgl.DestroyContext(h.ctx)
			h.ctx = 0
		}
	})
	h.thread.close()
}
