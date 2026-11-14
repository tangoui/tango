package ui

import (
	"time"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
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
