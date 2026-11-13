package fake

// platform.CanvasGL on fake canvases, for tests of the Linux GPU glue (on
// every OS). Like CanvasMetal and CanvasHWND it is off by default
// (UseGLArea returns platform.ErrUnsupported); EnableCanvasGL turns it on
// for a backend. Attached canvases always report ErrUnsupported.

import (
	"errors"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.CanvasGL = (*Canvas)(nil)
	_ platform.CanvasGL = (*AttachedCanvas)(nil)
)

// FakeGLArea is the handle UseGLArea returns unless EnableCanvasGL names
// another one. It is not a real GtkGLArea.
const FakeGLArea uintptr = 0x474c4100

var (
	glCfgs     sync.Map // *Backend -> *metalCfg (handle 0: unsupported)
	glStates   sync.Map // *Canvas -> *metalState
	glReleases sync.Map // *Canvas -> *glRelease
)

// glRelease is a canvas's ReleaseGLArea record.
type glRelease struct {
	mu    sync.Mutex
	calls int  // ReleaseGLArea calls
	done  bool // a release undid a successful UseGLArea
}

func (c *Canvas) glRelease() *glRelease {
	r, _ := glReleases.LoadOrStore(c, &glRelease{})
	return r.(*glRelease)
}

func (b *Backend) glCfg() *metalCfg {
	c, _ := glCfgs.LoadOrStore(b, &metalCfg{})
	return c.(*metalCfg)
}

func (c *Canvas) glState() *metalState {
	m, _ := glStates.LoadOrStore(c, &metalState{})
	return m.(*metalState)
}

// EnableCanvasGL makes UseGLArea succeed on this backend's canvas
// surfaces, returning glarea (FakeGLArea when 0). Any goroutine.
func (b *Backend) EnableCanvasGL(glarea uintptr) {
	if glarea == 0 {
		glarea = FakeGLArea
	}
	m := b.glCfg()
	m.mu.Lock()
	m.layer = glarea
	m.mu.Unlock()
}

// UseGLArea implements platform.CanvasGL: main thread; the first
// successful call fixes the handle, later calls return it again.
func (c *Canvas) UseGLArea() (uintptr, error) {
	c.b.mustMain("UseGLArea")
	st := c.glState()
	st.mu.Lock()
	defer st.mu.Unlock()
	st.calls++
	if c.s.Closed() {
		return 0, errors.New("fake: UseGLArea: surface closed")
	}
	if st.layer != 0 {
		return st.layer, nil
	}
	if r := c.glRelease(); func() bool { r.mu.Lock(); defer r.mu.Unlock(); return r.done }() {
		return 0, platform.ErrUnsupported
	}
	cfg := c.b.glCfg()
	cfg.mu.Lock()
	area := cfg.layer
	cfg.mu.Unlock()
	if area == 0 {
		return 0, platform.ErrUnsupported
	}
	st.layer = area
	return area, nil
}

// UseGLArea implements platform.CanvasGL: attached canvases stay on the
// CPU path.
func (c *AttachedCanvas) UseGLArea() (uintptr, error) {
	c.b.mustMain("UseGLArea")
	st := c.glState()
	st.mu.Lock()
	st.calls++
	st.mu.Unlock()
	return 0, platform.ErrUnsupported
}

// GLAreaCalls counts UseGLArea calls (failed ones included).
func (c *Canvas) GLAreaCalls() int {
	st := c.glState()
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.calls
}

// GLArea returns the handle UseGLArea returned, or 0 while the canvas is
// on the CPU path.
func (c *Canvas) GLArea() uintptr {
	st := c.glState()
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.layer
}

// ReleaseGLArea implements platform.CanvasGL: after a successful
// UseGLArea it forgets the handle (GLArea returns 0 again) and later
// UseGLArea calls report ErrUnsupported. Main thread.
func (c *Canvas) ReleaseGLArea() {
	c.b.mustMain("ReleaseGLArea")
	r := c.glRelease()
	r.mu.Lock()
	r.calls++
	r.mu.Unlock()
	st := c.glState()
	st.mu.Lock()
	had := st.layer != 0
	st.layer = 0
	st.mu.Unlock()
	if had {
		r.mu.Lock()
		r.done = true
		r.mu.Unlock()
	}
}

// ReleaseGLArea implements platform.CanvasGL: nothing to release.
func (c *AttachedCanvas) ReleaseGLArea() { c.b.mustMain("ReleaseGLArea") }

// GLAreaReleases counts ReleaseGLArea calls, and reports whether one
// undid a successful UseGLArea.
func (c *Canvas) GLAreaReleases() (calls int, released bool) {
	r := c.glRelease()
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.done
}
