package ui

import "github.com/tangoui/tango/ui/input"

// Tab capture: by default Tab and Shift+Tab move keyboard focus and never
// reach widgets. A widget that uses Tab itself (a code editor indenting,
// a terminal) declares CaptureTab while it has focus; Tab then arrives in
// its KeysFor like any other key.
//
// Keyboard users must still be able to leave it (WCAG 2.1.2 No Keyboard
// Trap): Ctrl+Tab and Ctrl+Shift+Tab always move focus, and after Esc the
// next Tab moves focus once, unless the widget also claims Esc
// (ClaimEscape, escape_claim.go); then only Ctrl+Tab leaves it.

type tabCapture struct {
	cur, next ID   // declared during the last / this build
	escaped   bool // Esc was the last key down: the next Tab moves focus
}

// CaptureTab makes Tab and Shift+Tab reach widget id (in KeysFor) instead
// of moving focus, while id has keyboard focus. Call it during the build
// of every frame in which id wants Tab. Ctrl+Tab, or Esc then Tab, still
// moves focus out of it.
func (c *Context) CaptureTab(id ID) {
	if id != 0 && c.focus == id {
		c.tabCap.next = id
	}
}

// tabCaptured reports whether the Tab event goes to the focused widget.
func (c *Context) tabCaptured(ev input.KeyEvent) bool {
	t := &c.tabCap
	if t.cur == 0 || t.cur != c.focus || ev.Mods.Has(input.ModCtrl) {
		return false
	}
	if t.escaped && ev.Down {
		t.escaped = false
		return false // Esc, Tab: leave the widget
	}
	return !t.escaped
}

// tabEscaped records Esc while a Tab-capturing widget has focus.
func (c *Context) tabEscaped() {
	if c.tabCap.cur != 0 && c.tabCap.cur == c.focus {
		c.tabCap.escaped = true
	}
}

// PressCount returns the click count of the current (or last) primary
// press: 1 single, 2 double, 3 triple... It follows the host's count when
// the platform reports one (system double-click settings), otherwise the
// UI's own 400 ms rule. Read it while handling a press or a drag that
// started with one (word / line selection by double / triple click).
func (c *Context) PressCount() int { return max(1, c.clickCount) }
