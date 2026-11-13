package ui

import (
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Icons private to the navigation components (24×24 grid, original paths).

var (
	navIcToTop        = icon.New("navIcToTop", "M5 4H19M12 20.5V8.5M6.5 14L12 8.5L17.5 14", "")
	navIcBack         = icon.New("navIcBack", "M20 12H4.5M10.5 5.5L4 12L10.5 18.5", "").Directional()
	navIcHeadset      = icon.New("navIcHeadset", "M4.5 15V12C4.5 7.9 7.9 4.5 12 4.5C16.1 4.5 19.5 7.9 19.5 12V15M4.5 13.5H7.5V19H4.5ZM16.5 13.5H19.5V19H16.5ZM19.5 19C19.5 20.4 17.6 21 14.5 21", "")
	navIcComment      = icon.New("navIcComment", "M4 5H20V16H10.5L6 19.5V16H4Z", "")
	navIcFullExit     = icon.New("navIcFullExit", "M9 4V9H4M15 4V9H20M20 15H15V20M4 15H9V20", "")
	navIcFull         = icon.New("navIcFull", "M4 9V4H9M15 4H20V9M20 15V20H15M9 20H4V15", "")
	navIcCaretLeft    = icon.New("navIcCaretLeft", "", "M9 12L15 6.5V17.5Z").Directional()
	navIcCaretRight   = icon.New("navIcCaretRight", "", "M15 12L9 6.5V17.5Z").Directional()
	navIcCaretUp      = icon.New("navIcCaretUp", "", "M12 9L17.5 15H6.5Z")
	navIcCaretDown    = icon.New("navIcCaretDown", "", "M12 15L6.5 9H17.5Z")
	navIcQuestionMark = icon.New("navIcQuestionMark", "M8.6 9C8.6 7 10.1 5.6 12 5.6C13.9 5.6 15.4 6.9 15.4 8.7C15.4 11.2 12 11.4 12 14.2M12 18.2V18.3", "")
)

// ---------------------------------------------------------------------------
// Small helpers.

// navLabel shapes a single line of interface text (RTL base direction in
// an RTL scope).
func navLabel(c *Context, s string, size float32, w text.Weight) *text.Layout {
	return text.Shape(s, c.style(size, w), c.textOpts(text.Options{NoWrap: true}))
}

// navTextAt draws s with its line box vertically centered in [y, y+h]; x
// is the physical left edge (callers place it with c.inline).
func navTextAt(c *Context, l *paint.List, s string, size float32, w text.Weight, x, y, h float32, col theme.Color) float32 {
	lay := navLabel(c, s, size, w)
	l.Glyphs(geom.Pt(x, y+(h-lay.Height)/2), lay.Glyphs, col)
	return lay.Width
}

func navHLine(l *paint.List, x, y, w float32, col theme.Color) {
	l.Fill(geom.Rect{X: x, Y: y, W: w, H: 1}, geom.Radii{}, col)
}

func navVLine(l *paint.List, x, y, h float32, col theme.Color) {
	l.Fill(geom.Rect{X: x, Y: y, W: 1, H: h}, geom.Radii{}, col)
}

func navEl(c *Context) bool { return c.Theme.Flavor == theme.Element }

// ---------------------------------------------------------------------------
// Animated scrolling of a scroll container.

type navScrollTween struct {
	from, to float64
	start    time.Time
	dur      time.Duration
	active   bool
}

func navScrollState(c *Context, id ID) *scrollState {
	if e, ok := c.states[id]; ok {
		if s, ok := e.v.(*scrollState); ok {
			e.seen = c.pass
			return s
		}
	}
	return nil
}

// navScrollTo starts a smooth vertical scroll of container id to y.
func navScrollTo(c *Context, id ID, y float32, dur time.Duration) {
	s := navScrollState(c, id)
	if s == nil {
		return
	}
	to := max(0, min(float64(y), s.maxY()))
	tw := State[navScrollTween](c, hashID(id, "scroll-tween"), nil)
	*tw = navScrollTween{from: s.off.Y, to: to, start: c.now, dur: dur, active: true}
	navScrollStep(c, id)
}

// navScrollStep advances a running tween; call it every frame.
func navScrollStep(c *Context, id ID) {
	tw := State[navScrollTween](c, hashID(id, "scroll-tween"), nil)
	if !tw.active {
		return
	}
	s := navScrollState(c, id)
	if s == nil {
		tw.active = false
		return
	}
	t := float64(c.now.Sub(tw.start)) / float64(max(tw.dur, 1))
	if c.still() || t >= 1 { // reduced motion: jump
		s.off.Y = tw.to
		tw.active = false
		c.invalid = true
		return
	}
	s.off.Y = tw.from + (tw.to-tw.from)*EaseInOut(t)
	c.animating = true
}

// navBodyScoped runs fn with n as parent and an explicit ID scope, so the
// same content keeps its widget state when moved between parents.
func navBodyScoped(c *Context, n *node, scope ID, fn func()) {
	if fn == nil {
		return
	}
	prev, prevScope := c.cur, c.scope
	c.cur, c.scope = n, scope
	fn()
	c.cur, c.scope = prev, prevScope
}
