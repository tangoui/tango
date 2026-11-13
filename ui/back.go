package ui

// The system Back action of phones (Android's back button and back
// gesture; docs/specs/mobile-canvas.md §2.2). Back means "close the
// top-most thing", which in Tango is what Escape already does: every
// dismissible layer (Modal, Drawer, Popover, Dropdown and context menus,
// Select lists, mobile Popup, ActionSheet, Dialog, Picker) listens for
// Escape through Context.EscapeAt / Escape / Dismissed, and Escape closes
// those layers one at a time, top first (layers.go).
//
// So the runner only has to tell the host when Back has something to
// close: after each frame it knows whether any layer above the page
// listened for Escape (Context.WantBack). A window implementing BackWindow
// is told when that changes; while it is on, the host delivers Back as an
// input.KeyEvent for input.KeyEscape (down, then up) instead of leaving
// the page, and the escape routing does the rest. Page code with its own
// notion of Back (an in-page navigation stack) calls Context.ClaimBack.

// BackWindow is implemented by windows on platforms with a system Back
// action. The runner calls SetBackHandling after a frame whenever
// Context.WantBack changes (never twice with the same value; the first
// call is SetBackHandling(true)). While on, the host delivers Back as
// KeyEscape down and up; while off, Back does what the platform does
// without the UI (go back in the page stack, leave the app). UI thread.
type BackWindow interface {
	SetBackHandling(on bool)
}

// WantBack reports whether the system Back action has something to close
// in the last frame: a dismissible overlay above the page (anything that
// listened for Escape on a layer above the page) or a ClaimBack call.
func (c *Context) WantBack() bool { return c.escTop > 0 || c.backClaimed }

// ClaimBack asks for the system Back action in this frame: while some
// frame calls it, Back arrives as an Escape key press, read with
// Context.Escape (on the page it is not consumed by overlays, which are
// above it and take Escape first). Call it every frame the page has
// something Back should undo, typically an in-page navigation stack that
// is not at its root.
func (c *Context) ClaimBack() { c.backNext = true }

// updateBack publishes WantBack to the window after a frame.
func (r *Runner) updateBack() {
	c := r.c
	c.backClaimed, c.backNext = c.backNext, false
	on := c.WantBack()
	if on == r.backOn {
		return
	}
	bw, ok := r.win.(BackWindow)
	if !ok {
		return
	}
	r.backOn = on
	bw.SetBackHandling(on)
}
