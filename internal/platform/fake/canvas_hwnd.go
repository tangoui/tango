package fake

// platform.CanvasHWND on fake canvases, for tests of the Windows GPU glue
// (on every OS). Like CanvasMetal it is off by default (UseGPUWindow
// returns platform.ErrUnsupported); EnableCanvasHWND turns it on for a
// backend. Attached canvases always report ErrUnsupported.

import (
	"errors"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.CanvasHWND = (*Canvas)(nil)
	_ platform.CanvasHWND = (*AttachedCanvas)(nil)
)

// FakeHWND is the handle UseGPUWindow returns unless EnableCanvasHWND
// names another one. It is not a real window.
const FakeHWND uintptr = 0x48574e00

var (
	hwndCfgs   sync.Map // *Backend -> *metalCfg (handle 0: unsupported)
	hwndStates sync.Map // *Canvas -> *metalState
)

func (b *Backend) hwndCfg() *metalCfg {
	c, _ := hwndCfgs.LoadOrStore(b, &metalCfg{})
	return c.(*metalCfg)
}

func (c *Canvas) hwnd() *metalState {
	m, _ := hwndStates.LoadOrStore(c, &metalState{})
	return m.(*metalState)
}

// EnableCanvasHWND makes UseGPUWindow succeed on this backend's canvas
// surfaces, returning hwnd (FakeHWND when 0). Any goroutine.
func (b *Backend) EnableCanvasHWND(hwnd uintptr) {
	if hwnd == 0 {
		hwnd = FakeHWND
	}
	m := b.hwndCfg()
	m.mu.Lock()
	m.layer = hwnd
	m.mu.Unlock()
}

// UseGPUWindow implements platform.CanvasHWND: main thread; the first
// successful call fixes the handle, later calls return it again.
func (c *Canvas) UseGPUWindow() (uintptr, error) {
	c.b.mustMain("UseGPUWindow")
	st := c.hwnd()
	st.mu.Lock()
	defer st.mu.Unlock()
	st.calls++
	if c.s.Closed() {
		return 0, errors.New("fake: UseGPUWindow: surface closed")
	}
	if st.layer != 0 {
		return st.layer, nil
	}
	cfg := c.b.hwndCfg()
	cfg.mu.Lock()
	hwnd := cfg.layer
	cfg.mu.Unlock()
	if hwnd == 0 {
		return 0, platform.ErrUnsupported
	}
	st.layer = hwnd
	return hwnd, nil
}

// UseGPUWindow implements platform.CanvasHWND: attached canvases stay on
// the CPU path.
func (c *AttachedCanvas) UseGPUWindow() (uintptr, error) {
	c.b.mustMain("UseGPUWindow")
	st := c.hwnd()
	st.mu.Lock()
	st.calls++
	st.mu.Unlock()
	return 0, platform.ErrUnsupported
}

// GPUWindowCalls counts UseGPUWindow calls (failed ones included).
func (c *Canvas) GPUWindowCalls() int {
	st := c.hwnd()
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.calls
}

// GPUWindow returns the handle UseGPUWindow returned, or 0 while the
// canvas is on the CPU path.
func (c *Canvas) GPUWindow() uintptr {
	st := c.hwnd()
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.layer
}
