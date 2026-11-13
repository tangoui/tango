package fake

// platform.CanvasBackHandler on fake canvases, for tests of the Back
// forwarding (docs/specs/mobile-canvas.md §2.2). Back simulates the
// system Back action the way the Android backend handles it: while
// handling is on it reaches the canvas as KeyEscape down and up; while
// off it would leave the page, which the fake only counts. AttachedCanvas
// inherits it through its embedded Canvas.

import (
	"sync"

	"github.com/tangoui/tango/internal/platform"
	"github.com/tangoui/tango/ui/input"
)

var _ platform.CanvasBackHandler = (*Canvas)(nil)

type backState struct {
	mu     sync.Mutex
	on     bool
	calls  []bool // every SetBackHandling value, in order
	passed int    // Back presses the canvas did not take
}

var backStates sync.Map // *Canvas -> *backState

func (c *Canvas) back() *backState {
	s, _ := backStates.LoadOrStore(c, &backState{})
	return s.(*backState)
}

// SetBackHandling implements platform.CanvasBackHandler.
func (c *Canvas) SetBackHandling(on bool) {
	s := c.back()
	s.mu.Lock()
	s.on = on
	s.calls = append(s.calls, on)
	s.mu.Unlock()
}

// BackHandling reports the current handling state and every value the
// core set, in order.
func (c *Canvas) BackHandling() (on bool, calls []bool) {
	s := c.back()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.on, append([]bool(nil), s.calls...)
}

// Back simulates the system Back action. While handling is on it delivers
// KeyEscape down and up to the sink and returns true; otherwise it counts
// the press as leaving the page (BackPassed) and returns false. Any
// goroutine.
func (c *Canvas) Back() bool {
	s := c.back()
	s.mu.Lock()
	on := s.on
	if !on {
		s.passed++
	}
	s.mu.Unlock()
	if !on {
		return false
	}
	c.Input(input.KeyEvent{Key: input.KeyEscape, Code: input.KeyEscape, Down: true})
	c.Input(input.KeyEvent{Key: input.KeyEscape, Code: input.KeyEscape})
	return true
}

// BackPassed counts Back presses made while handling was off.
func (c *Canvas) BackPassed() int {
	s := c.back()
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.passed
}
