package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/text"
)

// Inline-direction helpers for component paint and input code
// (docs/specs/rtl-and-component-tokens.md §3.4, §3.6). Layout mirrors node
// rectangles by itself; these cover what a component draws or hit-tests
// inside its own rectangle, where "start" is the left edge in LTR and the
// right edge in RTL.

// inlineSpan maps logical inline offsets (measured from the start edge)
// inside r to physical x coordinates.
type inlineSpan struct {
	r   geom.Rect
	rtl bool
}

// inline returns the span of r in the current scope's direction (during
// paint: the direction of the node being painted).
func (c *Context) inline(r geom.Rect) inlineSpan { return inlineSpan{r: r, rtl: c.dir.rtl} }

// X is the physical left x of an element of width w placed off from the
// start edge.
func (s inlineSpan) X(off, w float32) float32 {
	if s.rtl {
		return s.r.X + s.r.W - off - w
	}
	return s.r.X + off
}

// Rect is the physical rectangle of an element of size w×h placed off from
// the start edge; y is absolute (not mirrored).
func (s inlineSpan) Rect(off, y, w, h float32) geom.Rect {
	return geom.Rect{X: s.X(off, w), Y: y, W: w, H: h}
}

// Start is the x of the start edge.
func (s inlineSpan) Start() float32 {
	if s.rtl {
		return s.r.X + s.r.W
	}
	return s.r.X
}

// End is the x of the end edge.
func (s inlineSpan) End() float32 {
	if s.rtl {
		return s.r.X
	}
	return s.r.X + s.r.W
}

// Off is the logical offset of the physical x from the start edge.
func (s inlineSpan) Off(x float32) float32 {
	if s.rtl {
		return s.r.X + s.r.W - x
	}
	return x - s.r.X
}

// Sign is +1 when moving toward the end edge increases x, else -1; use it
// to turn physical pointer deltas into logical ones.
func (s inlineSpan) Sign() float32 {
	if s.rtl {
		return -1
	}
	return 1
}

// Frac is the logical position of p within the span, clamped to [0, 1]
// (0 at the start edge): slider values, progress, ratings.
func (s inlineSpan) Frac(p geom.Point) float32 {
	if s.r.W <= 0 {
		return 0
	}
	f := min(max((p.X-s.r.X)/s.r.W, 0), 1)
	if s.rtl {
		return 1 - f
	}
	return f
}

// mirrorRect reflects r horizontally inside box.
func mirrorRect(r, box geom.Rect) geom.Rect {
	r.X = 2*box.X + box.W - r.X - r.W
	return r
}

// inlineKey reports whether k means "previous" or "next" along the inline
// axis. Arrow keys are visual: in RTL Left moves to the next item and
// Right to the previous one. Other keys report neither.
func (c *Context) inlineKey(k input.Key) (prev, next bool) {
	switch k {
	case input.KeyLeft:
		return !c.dir.rtl, c.dir.rtl
	case input.KeyRight:
		return c.dir.rtl, !c.dir.rtl
	}
	return false, false
}

// textOpts sets the paragraph base direction of interface text (labels,
// buttons, menus, headers): RTL in an RTL scope; in LTR scopes o is
// returned unchanged (DirAuto), so LTR output is exactly as before. An
// explicit o.Direction is kept. User content (inputs, table cells) keeps
// DirAuto instead.
func (c *Context) textOpts(o text.Options) text.Options {
	if c.dir.rtl && o.Direction == text.DirAuto {
		o.Direction = text.DirRTL
	}
	return o
}
