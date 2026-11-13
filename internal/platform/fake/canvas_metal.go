package fake

// platform.CanvasMetal on fake canvases, for tests of the GPU glue. It is
// off by default (UseMetalLayer returns platform.ErrUnsupported, so
// existing tests keep the CPU path); EnableCanvasMetal turns it on for a
// backend. A switched canvas accepts Present(nil) as "the GPU presented".
// Attached canvases always report ErrUnsupported, as on macOS.

import (
	"errors"
	"sync"

	"github.com/tangoui/tango/internal/platform"
)

var (
	_ platform.CanvasMetal = (*Canvas)(nil)
	_ platform.CanvasMetal = (*AttachedCanvas)(nil)
)

// FakeMetalLayer is the layer handle UseMetalLayer returns unless
// EnableCanvasMetal names another one. It is not a real CAMetalLayer.
const FakeMetalLayer uintptr = 0x4d544c00

type metalCfg struct {
	mu    sync.Mutex
	layer uintptr // 0: CanvasMetal unsupported
}

type metalState struct {
	mu    sync.Mutex
	calls int
	layer uintptr // after a successful call
}

var (
	metalCfgs   sync.Map // *Backend -> *metalCfg
	metalStates sync.Map // *Canvas -> *metalState
)

func (b *Backend) metalCfg() *metalCfg {
	c, _ := metalCfgs.LoadOrStore(b, &metalCfg{})
	return c.(*metalCfg)
}

func (c *Canvas) metal() *metalState {
	m, _ := metalStates.LoadOrStore(c, &metalState{})
	return m.(*metalState)
}

// EnableCanvasMetal makes UseMetalLayer succeed on this backend's canvas
// surfaces, returning layer (FakeMetalLayer when 0). Any goroutine.
func (b *Backend) EnableCanvasMetal(layer uintptr) {
	if layer == 0 {
		layer = FakeMetalLayer
	}
	m := b.metalCfg()
	m.mu.Lock()
	m.layer = layer
	m.mu.Unlock()
}

// UseMetalLayer implements platform.CanvasMetal: main thread; the first
// successful call fixes the handle, later calls return it again.
func (c *Canvas) UseMetalLayer() (uintptr, error) {
	c.b.mustMain("UseMetalLayer")
	st := c.metal()
	st.mu.Lock()
	defer st.mu.Unlock()
	st.calls++
	if c.s.Closed() {
		return 0, errors.New("fake: UseMetalLayer: surface closed")
	}
	if st.layer != 0 {
		return st.layer, nil
	}
	cfg := c.b.metalCfg()
	cfg.mu.Lock()
	layer := cfg.layer
	cfg.mu.Unlock()
	if layer == 0 {
		return 0, platform.ErrUnsupported
	}
	st.layer = layer
	return layer, nil
}

// UseMetalLayer implements platform.CanvasMetal: attached canvases stay on
// the CPU path.
func (c *AttachedCanvas) UseMetalLayer() (uintptr, error) {
	c.b.mustMain("UseMetalLayer")
	st := c.metal()
	st.mu.Lock()
	st.calls++
	st.mu.Unlock()
	return 0, platform.ErrUnsupported
}

// MetalCalls counts UseMetalLayer calls (failed ones included).
func (c *Canvas) MetalCalls() int {
	st := c.metal()
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.calls
}

// MetalLayer returns the handle UseMetalLayer returned, or 0 while the
// canvas is on the CPU path.
func (c *Canvas) MetalLayer() uintptr {
	st := c.metal()
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.layer
}
