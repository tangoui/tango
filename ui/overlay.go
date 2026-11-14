package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
)

// Placement positions an overlay relative to an anchor or the window.
//
// BottomStart/BottomEnd/TopStart/TopEnd and the Inline* and Screen{Start,
// End,TopStart,TopEnd} values are logical: Start is the inline start edge
// (left in LTR, right in RTL). Left/Right/RightStart/LeftStart/RightEnd/
// LeftEnd and ScreenLeft/ScreenRight/ScreenTopRight/ScreenTopLeft are
// physical and never mirror. The overlay's direction is that of the scope
// that built it; see Resolve.
//
// Append-only: never reorder or insert; the zero value stays BottomStart.
type Placement uint8

const (
	BottomStart Placement = iota // below anchor, aligned to its inline start edge
	BottomEnd                    // below anchor, aligned to its inline end edge
	Bottom                       // below, centered
	TopStart                     // above anchor, aligned to its inline start edge
	TopEnd                       // above anchor, aligned to its inline end edge
	Top
	RightStart      // right of anchor, top edges aligned
	LeftStart       // left of anchor, top edges aligned
	ScreenCenter    // centered in the window
	ScreenTop       // horizontally centered, near the top (messages)
	ScreenTopRight  // notifications
	ScreenRight     // drawers: full height on the right
	ScreenLeft      // full height on the left
	ScreenFill      // covers the window (masks)
	ScreenTopCenter // modal: centered horizontally, 100px from top
	Right           // right of anchor, vertically centered
	Left            // left of anchor, vertically centered
	RightEnd        // right of anchor, bottom edges aligned
	LeftEnd         // left of anchor, bottom edges aligned

	InlineEnd         // after the anchor on the inline axis, vertically centered
	InlineEndTop      // after the anchor, top edges aligned (submenus)
	InlineEndBottom   // after the anchor, bottom edges aligned
	InlineStart       // before the anchor on the inline axis, vertically centered
	InlineStartTop    // before the anchor, top edges aligned
	InlineStartBottom // before the anchor, bottom edges aligned
	ScreenEnd         // full height on the inline end side (drawers)
	ScreenStart       // full height on the inline start side
	ScreenTopEnd      // top corner on the inline end side (notifications)
	ScreenTopStart    // top corner on the inline start side
	ScreenTopLeft     // physical top-left corner
)

// Resolve maps a logical placement to the physical one used for the
// given direction. Physical placements are returned unchanged, so in LTR
// every placement resolves to its LTR meaning.
func (p Placement) Resolve(rtl bool) Placement {
	switch p {
	case InlineEnd:
		p = Right
	case InlineEndTop:
		p = RightStart
	case InlineEndBottom:
		p = RightEnd
	case InlineStart:
		p = Left
	case InlineStartTop:
		p = LeftStart
	case InlineStartBottom:
		p = LeftEnd
	case ScreenEnd:
		p = ScreenRight
	case ScreenStart:
		p = ScreenLeft
	case ScreenTopEnd:
		p = ScreenTopRight
	case ScreenTopStart:
		p = ScreenTopLeft
	default:
		if !rtl {
			return p
		}
		switch p {
		case BottomStart:
			return BottomEnd
		case BottomEnd:
			return BottomStart
		case TopStart:
			return TopEnd
		case TopEnd:
			return TopStart
		}
		return p
	}
	if rtl {
		return mirrorPlace[p]
	}
	return p
}

// mirrorPlace swaps the physical side of the placements Inline*/Screen*
// resolve to.
var mirrorPlace = map[Placement]Placement{
	Right: Left, RightStart: LeftStart, RightEnd: LeftEnd,
	Left: Right, LeftStart: RightStart, LeftEnd: RightEnd,
	ScreenRight: ScreenLeft, ScreenLeft: ScreenRight,
	ScreenTopRight: ScreenTopLeft, ScreenTopLeft: ScreenTopRight,
}

// OverlayOptions configure an overlay layer.
type OverlayOptions struct {
	_ [0]func()

	Anchor     ID // widget to attach to (anchored placements)
	Place      Placement
	Offset     float32 // gap between anchor and overlay
	MatchWidth bool    // at least as wide as the anchor
	Margin     float32 // distance from window edges for Screen* placements
	Flip       bool    // flip to the opposite side when it does not fit (currently always on for anchored placements)
	ShiftX     float32 // translation applied after placement (enter/leave motion)
	ShiftY     float32
}

type overlay struct {
	root *node
	opts OverlayOptions
	rtl  bool // direction of the scope that built the overlay
}

// Overlay builds fn in a new layer above everything built so far. Layers
// swallow pointer input over their area (and over the whole window for
// ScreenFill). The overlay is placed during layout of the current frame,
// using its measured size, so it never needs a frame to settle.
//
// Dismissible overlays close through Dismissed (or EscapeAt and
// PressedOutside) with the returned box's ID: Escape and outside presses
// then reach only the top-most open layer.
func Overlay(c *Context, o OverlayOptions, fn func(), opts ...layout.Option) *Box {
	prev, prevScope, prevOv := c.cur, c.scope, c.curOverlay
	root := &node{id: c.newID("overlay", ""), disabled: false}
	root.ln.Ref = root
	flexbox.Apply(&root.ln.Style, opts...)
	root.hit = true
	root.blocks = true
	c.stampEnv(root) // overlay roots skip c.add: record the scope's theme, locale and direction
	ov := &overlay{root: root, opts: o, rtl: c.IsRTL()}
	root.ln.RTL = ov.rtl
	c.overlays = append(c.overlays, ov)
	c.cur, c.scope, c.curOverlay = root, root.id, root.id
	fn()
	c.cur, c.scope, c.curOverlay = prev, prevScope, prevOv
	b := &Box{c: c, n: root}
	root.paint = b.paintBox
	return b
}

func (c *Context) anchorRect(id ID) (geom.Rect, bool) {
	if r, ok := c.nextRects[id]; ok {
		return r, true
	}
	r, ok := c.rects[id]
	return r, ok
}

func (c *Context) layoutOverlay(o *overlay) {
	c.placeOverlay(o)
	o.root.ln.Rect.X += o.opts.ShiftX
	o.root.ln.Rect.Y += o.opts.ShiftY
}

func (c *Context) placeOverlay(o *overlay) {
	link(o.root)
	vw, vh := c.size.W, c.size.H
	ln := &o.root.ln
	m := o.opts.Margin
	phys := o.opts.Place.Resolve(o.rtl)
	switch phys {
	case ScreenFill:
		flexbox.Solve(ln, vw, vh)
		ln.Rect.X, ln.Rect.Y = 0, 0
		return
	case ScreenRight, ScreenLeft:
		sz := flexbox.SolveAt(ln, vw, vh, false, true)
		ln.Rect.Y = 0
		if phys == ScreenRight {
			ln.Rect.X = vw - sz.W
		} else {
			ln.Rect.X = 0
		}
		return
	case ScreenCenter, ScreenTop, ScreenTopRight, ScreenTopLeft, ScreenTopCenter:
		sz := flexbox.SolveAt(ln, vw-2*m, vh-2*m, false, false)
		switch phys {
		case ScreenCenter:
			ln.Rect.X, ln.Rect.Y = (vw-sz.W)/2, (vh-sz.H)/2
		case ScreenTop:
			ln.Rect.X, ln.Rect.Y = (vw-sz.W)/2, m
		case ScreenTopCenter:
			ln.Rect.X, ln.Rect.Y = (vw-sz.W)/2, min(m, max(0, (vh-sz.H)/2))
		case ScreenTopRight:
			ln.Rect.X, ln.Rect.Y = vw-sz.W-m, m
		case ScreenTopLeft:
			ln.Rect.X, ln.Rect.Y = m, m
		}
		return
	}
	a, ok := c.anchorRect(o.opts.Anchor)
	if !ok {
		a = geom.Rect{X: vw / 2, Y: vh / 2}
	}
	if o.opts.MatchWidth && ln.Width == 0 {
		ln.MinW = a.W
	}
	sz := flexbox.SolveAt(ln, vw, vh, false, false)
	off := o.opts.Offset
	var x, y float32
	place := phys
	below := vh - a.Bottom() - off
	above := a.Y - off
	right := vw - a.Right() - off
	left := a.X - off
	switch place {
	case BottomStart, BottomEnd, Bottom:
		if sz.H > below && above > below {
			place += TopStart - BottomStart
		}
	case TopStart, TopEnd, Top:
		if sz.H > above && below > above {
			place -= TopStart - BottomStart
		}
	case Right, RightStart, RightEnd:
		if sz.W > right && left > right {
			place = sideFlip[place]
		}
	case Left, LeftStart, LeftEnd:
		if sz.W > left && right > left {
			place = sideFlip[place]
		}
	}
	switch place {
	case BottomStart:
		x, y = a.X, a.Bottom()+off
	case BottomEnd:
		x, y = a.Right()-sz.W, a.Bottom()+off
	case Bottom:
		x, y = a.X+(a.W-sz.W)/2, a.Bottom()+off
	case TopStart:
		x, y = a.X, a.Y-off-sz.H
	case TopEnd:
		x, y = a.Right()-sz.W, a.Y-off-sz.H
	case Top:
		x, y = a.X+(a.W-sz.W)/2, a.Y-off-sz.H
	case RightStart:
		x, y = a.Right()+off, a.Y
	case LeftStart:
		x, y = a.X-off-sz.W, a.Y
	case Right:
		x, y = a.Right()+off, a.Y+(a.H-sz.H)/2
	case Left:
		x, y = a.X-off-sz.W, a.Y+(a.H-sz.H)/2
	case RightEnd:
		x, y = a.Right()+off, a.Bottom()-sz.H
	case LeftEnd:
		x, y = a.X-off-sz.W, a.Bottom()-sz.H
	}
	x = max(4, min(x, vw-sz.W-4))
	y = max(4, min(y, vh-sz.H-4))
	ln.Rect.X, ln.Rect.Y = x, y
}

var sideFlip = map[Placement]Placement{
	Right: Left, RightStart: LeftStart, RightEnd: LeftEnd,
	Left: Right, LeftStart: RightStart, LeftEnd: RightEnd,
}
