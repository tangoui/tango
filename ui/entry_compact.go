package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// CompactW joins its direct child controls (Input, Select, Button,
// InputNumber, DatePicker, Addon, ...) into one group: neighbours overlap by
// one pixel and only the outer corners stay rounded (Ant Space.Compact /
// Input.Group, Element input prepend/append and button groups).
//
// The children are drawn by their own painters; Compact squares the joined
// corners by repainting the joined edge columns with a widened rectangle,
// so it works with any control without changing it.
type CompactW struct {
	c        *Context
	vertical bool
	block    bool
	opts     []layout.Option
}

// Compact starts a compact group; chain options, then Body.
//
//	ui.Compact(c).Body(func() { ui.Input(c, &v); ui.Button(c, "提交").Primary() })
func Compact(c *Context) *CompactW { return &CompactW{c: c} }

// Vertical stacks the controls (joined top/bottom edges).
func (w *CompactW) Vertical() *CompactW { w.vertical = true; return w }

// Block stretches the group to the container width.
func (w *CompactW) Block() *CompactW { w.block = true; return w }

// Layout applies layout options to the group.
func (w *CompactW) Layout(o ...layout.Option) *CompactW { w.opts = append(w.opts, o...); return w }

// Body adds the controls.
func (w *CompactW) Body(fn func()) *Box {
	c := w.c
	dir := layout.Row
	if w.vertical {
		dir = layout.Column
	}
	b := c.box("compact", dir, w.opts)
	b.n.ln.NoStretch = !w.block
	b.n.ln.Align = layout.AlignStretch
	b.Body(fn)
	n := b.n
	// Children may finish (and attach painters) in deferred passes; wrap
	// after everything built so far has run.
	if w.vertical {
		c.deferred = append(c.deferred, func() { cmpJoinV(c, n.kids) })
	} else {
		c.deferred = append(c.deferred, func() { cmpJoin(c, n) })
	}
	return b
}

// cmpR is the width of the corner column repainted on a joined side; it
// covers the largest control radius.
const cmpR = 9

// cmpExt is how far a joined side is pushed outwards when repainting the
// corner column: far enough to move icons out of the column.
const cmpExt = 80

func cmpJoin(c *Context, n *node) {
	kids := n.kids
	cnt := len(kids)
	origs := make([]PaintFunc, cnt)
	targets := make([]*node, cnt)
	for i, k := range kids {
		left, right := i > 0, i < cnt-1
		if left {
			k.ln.Margin.Left = -1
		}
		t := cmpTarget(k)
		targets[i] = t
		if t.paint == nil {
			continue
		}
		orig := t.paint
		origs[i] = orig
		t.paint = func(l *paint.List, r geom.Rect) {
			// left / right are the joined inline sides (previous / next
			// child); in RTL the previous child is on the right.
			pl, pr := left, right
			if c.IsRTL() {
				pl, pr = right, left
			}
			cmpPaint(l, r, orig, pl, pr)
		}
	}
	// Raise the joined edges of the hovered / focused child above its
	// neighbours (their borders share a pixel column).
	n.over = func(l *paint.List, _ geom.Rect) {
		for i, k := range targets {
			if origs[i] == nil || k.disabled {
				continue
			}
			if !(c.pointerIn && k.abs.Contains(c.pointer)) && !c.focusWithin(k) {
				continue
			}
			rtl := c.IsRTL()
			if i > 0 {
				cmpPaintEdge(l, k.abs, origs[i], !rtl)
			}
			if i < cnt-1 {
				cmpPaintEdge(l, k.abs, origs[i], rtl)
			}
		}
	}
}

// cmpTarget returns the painted control of a joined child, looking through
// pass-through wrappers (Dropdown, Tooltip, ...): unpadded containers with a
// single child. Controls with an inner editor (Input) are padded and stay.
func cmpTarget(k *node) *node {
	t := k
	for t.ln.Measure == nil && len(t.kids) == 1 && t.over == nil && !t.focus && t.ln.Padding == (geom.Insets{}) {
		t = t.kids[0]
	}
	return t
}

func cmpPaint(l *paint.List, r geom.Rect, orig PaintFunc, left, right bool) {
	pad := float32(6) // keep focus glow / shadows above and below
	// Push centred and edge content (labels, icons) of the widened paint
	// well past the corner column.
	ext := 2*r.W + cmpExt
	// 1. Corner columns of joined sides, painted square.
	if left {
		l.Clip(geom.Rect{X: r.X, Y: r.Y - pad, W: cmpR, H: r.H + 2*pad}, geom.Radii{})
		orig(l, geom.Rect{X: r.X - ext, Y: r.Y, W: r.W + ext, H: r.H})
		l.Pop()
		cmpPaintEdge(l, r, orig, true)
	}
	if right {
		l.Clip(geom.Rect{X: r.Right() - cmpR, Y: r.Y - pad, W: cmpR, H: r.H + 2*pad}, geom.Radii{})
		orig(l, geom.Rect{X: r.X, Y: r.Y, W: r.W + ext, H: r.H})
		l.Pop()
		cmpPaintEdge(l, r, orig, false)
	}
	// 2. The body, excluding the joined columns, painted last with the real
	// rect so painters caching geometry keep the right values.
	body := geom.Rect{X: r.X - pad, Y: r.Y - pad, W: r.W + 2*pad, H: r.H + 2*pad}
	if left {
		body.W -= pad + cmpR
		body.X = r.X + cmpR
	}
	if right {
		body.W = r.Right() - cmpR - body.X
	}
	l.Clip(body, geom.Radii{})
	orig(l, r)
	l.Pop()
}

// cmpPaintEdge paints the vertical border of a joined side: the control is
// painted with a rect stretched vertically so the border runs straight over
// the full height, clipped to the 1px border column.
func cmpPaintEdge(l *paint.List, r geom.Rect, orig PaintFunc, leftSide bool) {
	x := r.X
	if !leftSide {
		x = r.Right() - 1
	}
	l.Clip(geom.Rect{X: x, Y: r.Y, W: 1, H: r.H}, geom.Radii{})
	orig(l, geom.Rect{X: r.X, Y: r.Y - cmpR, W: r.W, H: r.H + 2*cmpR})
	l.Pop()
}

// ---------------------------------------------------------------------------
// Addon

// AddonW is a static (or clickable) label attached before/after an input
// inside Compact (Ant addonBefore/addonAfter, Element prepend/append).
type AddonW struct {
	c        *Context
	n        *node
	label    string
	ic       *icon.Icon
	size     ControlSize
	disabled bool
	ia       Interaction
	clicky   bool
}

// Addon adds an addon box with a label.
func Addon(c *Context, label string) *AddonW {
	a := &AddonW{c: c, label: label}
	a.n = c.add("addon", label)
	a.n.ln.NoStretch = true
	a.n.ln.Measure = a.measure
	a.n.paint = a.paint
	a.n.setRole(a11y.RoleText) // static text ("https://", ".com"); a button once Clicked is called
	return a
}

// A11yLabel sets the accessible name (needed for an icon-only addon
// button).
func (a *AddonW) A11yLabel(s string) *AddonW { a.n.setName(s); return a }

// Icon shows an icon before (or instead of) the label.
func (a *AddonW) Icon(ic *icon.Icon) *AddonW        { a.ic = ic; return a }
func (a *AddonW) Size(s ControlSize) *AddonW        { a.size = s; return a }
func (a *AddonW) Layout(o ...layout.Option) *AddonW { a.n.ln.Style.Apply(o...); return a }
func (a *AddonW) Disabled(on bool) *AddonW {
	if on {
		a.disabled, a.n.disabled = true, true
		a.n.cursor = input.CursorNotAllowed
	}
	return a
}

// Clicked makes the addon a button (Element append button) and reports a click.
func (a *AddonW) Clicked() bool {
	if !a.clicky {
		a.clicky = true
		a.n.setRole(a11y.RoleButton)
		a.ia = a.c.interact(a.n, true, input.CursorPointer)
	}
	return a.ia.Clicked && !a.disabled && !a.n.disabled
}

func (a *AddonW) padX() float32 {
	if a.c.Theme.Flavor == theme.Element {
		return 20
	}
	if a.size == Small {
		return 7
	}
	return a.c.Theme.ControlPaddingH
}

func (a *AddonW) measure(_, _ float32) geom.Size {
	c := a.c
	fs := c.controlFont(a.size)
	w := entTextW(c, a.label, fs, text.Regular)
	if a.ic != nil {
		if w > 0 {
			w += 8
		}
		w += fs
	}
	return geom.Size{W: w + 2*a.padX(), H: c.controlHeight(a.size)}
}

func (a *AddonW) paint(l *paint.List, r geom.Rect) {
	c, th := a.c, a.c.ThemeFor(theme.CompInput)
	rad := geom.Rad(controlRadiusOf(th, a.size))
	bg, fg := th.Components.Input.AddonBg, th.Components.Input.AddonColor
	dis := a.disabled || a.n.disabled
	if dis {
		fg = th.TextDisabled
	}
	if a.clicky && a.ia.Hovered && !dis {
		fg = th.Primary.Base
	}
	l.Fill(r, rad, th.BgContainer)
	l.Fill(r, rad, bg)
	l.Stroke(r, rad, 1, th.Border)
	fs := controlFontOf(th, a.size)
	w := entTextW(c, a.label, fs, text.Regular)
	if a.ic != nil {
		if w > 0 {
			w += 8
		}
		w += fs
	}
	off := (r.W - w) / 2 // icon then label from the inline start
	if a.ic != nil {
		c.drawIconAt(l, a.ic, c.inline(r).X(off, fs), r.Y+(r.H-fs)/2, fs, fg)
		off += fs + 8
	}
	entTextIn(c, l, a.label, off, r, fs, text.Regular, fg, 0)
	if a.clicky && a.ia.FocusVisible {
		entFocusRing(c, l, r, rad.TL)
	}
}

// cmpJoinV squares the inner corners of vertically joined children and
// overlaps their borders by one pixel: each child is repainted with its
// rounded ends pushed outside a clip.
func cmpJoinV(c *Context, kids []*node) {
	if len(kids) < 2 {
		return
	}
	th := c.Theme
	for i, k := range kids {
		first, last := i == 0, i == len(kids)-1
		if !first {
			k.ln.Margin.Top -= 1
		}
		t := cmpTarget(k)
		if t.paint == nil {
			continue
		}
		orig := t.paint
		sqTop, sqBottom := !first, !last
		t.paint = func(l *paint.List, r geom.Rect) {
			// The strip only has to cover the corner radius; keep it narrow
			// so centered content of small controls stays untouched.
			R := max(4, min(th.RadiusLG+2, r.W/4))
			cmpPaintV(l, r, orig, sqTop, sqBottom, R)
		}
	}
}

func cmpPaintV(l *paint.List, r geom.Rect, orig PaintFunc, sqTop, sqBottom bool, R float32) {
	if !sqTop && !sqBottom {
		orig(l, r)
		return
	}
	const far = 2000 // pushes rounded ends and centered content out of the strip
	clipped := func(clip, at geom.Rect) {
		l.Clip(clip, geom.Radii{})
		orig(l, at)
		l.Pop()
	}
	mid := r
	if sqTop {
		mid.Y += R
		mid.H -= R
	}
	if sqBottom {
		mid.H -= R
	}
	clipped(mid, r)
	if sqTop {
		clipped(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: R}, geom.Rect{X: r.X, Y: r.Y - far, W: r.W, H: r.H + far})
		clipped(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Rect{X: r.X - far, Y: r.Y, W: r.W + 2*far, H: r.H})
	}
	if sqBottom {
		clipped(geom.Rect{X: r.X, Y: r.Bottom() - R, W: r.W, H: R}, geom.Rect{X: r.X, Y: r.Y, W: r.W, H: r.H + far})
		clipped(geom.Rect{X: r.X, Y: r.Bottom() - 1, W: r.W, H: 1}, geom.Rect{X: r.X - far, Y: r.Y, W: r.W + 2*far, H: r.H})
	}
}
