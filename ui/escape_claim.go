package ui

import "github.com/tangoui/tango/ui/input"

// Esc claim: by default Esc is an overlay key. It closes the top-most
// dismissible layer (Context.Escape, layers.go), is left out of KeysFor
// and only appears in EventsFor. A widget that needs Esc for itself (a
// terminal running vim) declares ClaimEscape while it has focus. A keyboard
// Esc then reaches its KeysFor and EventsFor like any other key, and
// overlays around it stay open.
//
// The system Back of phones also arrives as Esc, marked with Code
// input.KeyBack (back.go). A claim never takes it: Back still closes the
// top-most overlay or reaches ClaimBack, and WantBack is unchanged. Neither
// its press nor its release reaches the claimer (not even in EventsFor).
//
// Keyboard users leave such a widget with Ctrl+Tab (tab_capture.go). Esc
// then Tab does not move focus out of it, because the widget took the Esc.

type escapeClaim struct {
	cur, next ID // declared during the last / this build
}

// ClaimEscape makes keyboard Esc reach widget id (in KeysFor and EventsFor)
// instead of closing overlays, while id has keyboard focus. Call it during
// the build of every frame in which id wants Esc. Android's system Back
// (Esc with Code input.KeyBack) is not claimed and still closes overlays.
func (c *Context) ClaimEscape(id ID) {
	if id != 0 && c.focus == id {
		c.escClaim.next = id
	}
}

// escapeClaimed reports whether the Esc event goes to the focused widget.
func (c *Context) escapeClaimed(ev input.KeyEvent) bool {
	return ev.Code != input.KeyBack && c.escClaim.cur != 0 && c.escClaim.cur == c.focus
}

// backClaimedAway reports whether ev is Back (press or release) while a
// widget holds the Esc claim. Back bypasses the claim and goes to the
// overlays (c.escape), so the claimer gets neither half of it: not the
// press in EventsFor, not an unpaired release in KeysFor / EventsFor.
// Without a claim Back flows as before.
func (c *Context) backClaimedAway(ev input.KeyEvent) bool {
	return ev.Code == input.KeyBack && c.escClaim.cur != 0 && c.escClaim.cur == c.focus
}
