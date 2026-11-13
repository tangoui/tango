package fake

// platform.CanvasInsets on fake canvases, for tests of the safe-area and
// keyboard-inset forwarding (docs/specs/mobile-canvas.md §3.1). Both insets
// are zero until a test sets them; a change requests a frame, as the
// mobile backends do. AttachedCanvas inherits it through its embedded
// Canvas.

import (
	"sync"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/geom"
)

var _ platform.CanvasInsets = (*Canvas)(nil)

type insetState struct {
	mu       sync.Mutex
	safe, kb geom.Insets
	polls    int
}

var insetStates sync.Map // *Canvas -> *insetState

func (c *Canvas) insets() *insetState {
	s, _ := insetStates.LoadOrStore(c, &insetState{})
	return s.(*insetState)
}

// SafeAreaInsets implements platform.CanvasInsets.
func (c *Canvas) SafeAreaInsets() geom.Insets {
	s := c.insets()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.polls++
	return s.safe
}

// KeyboardInset implements platform.CanvasInsets.
func (c *Canvas) KeyboardInset() geom.Insets {
	s := c.insets()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.polls++
	return s.kb
}

// SetSafeAreaInsets simulates the system changing the safe area (a
// rotation, a status bar appearing) and requests a frame when it changed.
// Any goroutine.
func (c *Canvas) SetSafeAreaInsets(in geom.Insets) {
	s := c.insets()
	s.mu.Lock()
	changed := s.safe != in
	s.safe = in
	s.mu.Unlock()
	if changed {
		c.Invalidate()
	}
}

// SetKeyboardInset simulates the soft keyboard showing, resizing or hiding
// and requests a frame when the inset changed. Any goroutine.
func (c *Canvas) SetKeyboardInset(in geom.Insets) {
	s := c.insets()
	s.mu.Lock()
	changed := s.kb != in
	s.kb = in
	s.mu.Unlock()
	if changed {
		c.Invalidate()
	}
}

// InsetPolls counts SafeAreaInsets and KeyboardInset calls.
func (c *Canvas) InsetPolls() int {
	s := c.insets()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.polls
}
