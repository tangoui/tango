package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
)

// EventsFor returns, in arrival order, the keyboard input of this frame for
// widget id while it has focus: input.KeyEvent (Key folded with Key.Base,
// Code the physical key), input.TextInput and input.Composition,
// interleaved as they happened. Use it when order matters (a text editor:
// Backspace then "a" is not "a" then Backspace); KeysFor and TextFor give
// the same events split by kind.
//
// It also carries Esc (which overlays consume through Escape) and Tab when
// id declared CaptureTab. Like KeysFor it hands the events over once:
// later calls in the same frame return nil.
func (c *Context) EventsFor(id ID) []input.Event {
	if id == 0 || c.focus != id || len(c.events) == 0 {
		return nil
	}
	ev := c.events
	c.events = nil
	return ev
}

// PressMods returns the modifier keys held at the last primary press
// (Shift+click to extend a selection, Alt+click for a column cursor).
func (c *Context) PressMods() input.Modifiers { return c.pressMods }

// PointerMods returns the modifiers held at the latest pointer event (move,
// press, release, wheel) or key event, whichever came last. Unlike
// PressMods it follows modifier changes during a drag, so a drag source can
// switch between move and copy (Alt/Option) while the pointer moves or
// when only the modifier key changes.
func (c *Context) PointerMods() input.Modifiers { return c.pointerMods }

// ReleasePos returns where the primary button was released over the press
// that id held, this frame. Unlike Pointer (the last pointer event of the
// frame) it is the release event's own position, so a move that arrives in
// the same frame after the release does not shift it. ok is false when id
// was not released this frame.
func (c *Context) ReleasePos(id ID) (geom.Point, bool) {
	p, ok := c.releasePos[id]
	return p, ok
}

// WindowFocused reports whether the window has key focus (false after the
// host reported input.WindowFocus{Focused: false}). Stop caret blinking and
// other focus-only animations while it is false.
func (c *Context) WindowFocused() bool { return !c.winBlurred }
