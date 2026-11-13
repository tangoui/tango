package mobile

import (
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// IndexBar

type indexState struct {
	anchors []ui.ID // header node of each group, last frame
	active  int
	jump    int // pending jump target (-1 none)
}

// IndexBarW is a grouped list with an alphabet index at the end (on the
// right; on the left in RTL): tap or drag along the index to jump to a
// group. The current group's title sticks to the top.
type IndexBarW struct {
	c       *ui.Context
	th      *Theme
	indexes []string
	opts    []layout.Option
	sv      *ScrollViewW
	rtl     bool
	rail    ui.ID // the index rail
}

// IndexBar creates an index bar for groups titled indexes.
func IndexBar(c *ui.Context, indexes []string) *IndexBarW {
	return &IndexBarW{c: c, th: ThemeOf(c), indexes: indexes, rtl: c.IsRTL()}
}

// Layout applies layout options to the container (give it a height).
func (w *IndexBarW) Layout(opts ...layout.Option) *IndexBarW {
	w.opts = append(w.opts, opts...)
	return w
}

// Body builds the groups; group(i) builds the rows of group i.
func (w *IndexBarW) Body(group func(i int)) *IndexBarW {
	c, th := w.c, w.th
	n := len(w.indexes)
	vant := th.Style == Vant
	hf := th.Font(th.FontSmall, text.Regular)
	headH := float32(32)
	if vant {
		hf = th.Font(14, text.Semibold)
	} else if th.shad() {
		hf = th.Font(th.FontSmall, text.Medium)
	}
	wrap := ui.StackBox(c, nil, w.opts...)
	st := ui.State[indexState](c, wrap.ID(), func() indexState { return indexState{jump: -1} })
	if len(st.anchors) != n {
		st.anchors = make([]ui.ID, n)
	}
	rtl := w.rtl
	wrap.Body(func() {
		w.sv = ScrollView(c).Layout(layout.Grow(1))
		sv := w.sv
		// Offsets of the group headers inside the content (last frame).
		var tops []float32
		if cr, ok := c.RectOf(sv.ContentID()); ok {
			for _, id := range st.anchors {
				r, _ := c.RectOf(id)
				tops = append(tops, r.Y-cr.Y)
			}
		}
		if st.jump >= 0 && st.jump < len(tops) {
			sv.ScrollTo(float64(tops[st.jump]))
			st.jump = -1
		}
		sv.Body(func() {
			for i, s := range w.indexes {
				i, s := i, s
				ui.Key(c, s, func() {
					h := ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: headH} }, func(l *paint.List, r geom.Rect) {
						spanOf(r, rtl).textMid(l, s, hf, r.X+th.PadH, r, th.Text)
					})
					h.Bg(th.PageBg).Semantics(a11y.RoleHeading, s)
					st.anchors[i] = h.ID()
					group(i)
				})
			}
		})
		// Current group from the scroll offset.
		pos := float32(sv.ScrollOffset())
		st.active = 0
		for i, t := range tops {
			if t <= pos+0.5 {
				st.active = i
			}
		}
		w.side(st, tops)
	})
	// Sticky header of the current group.
	var tops []float32
	if cr, ok := c.RectOf(w.sv.ContentID()); ok {
		for _, id := range st.anchors {
			r, _ := c.RectOf(id)
			tops = append(tops, r.Y-cr.Y)
		}
	}
	pos := float32(w.sv.ScrollOffset())
	active := st.active
	wrap.PaintOver(func(l *paint.List, r geom.Rect) {
		if pos <= 0 || active >= len(tops) || active >= n {
			return
		}
		y := r.Y
		if active+1 < len(tops) && tops[active+1]-pos < headH {
			y = r.Y + tops[active+1] - pos - headH
		}
		// The sticky header leaves the index rail at the end uncovered.
		sp := spanOf(r, rtl)
		hr := geom.Rect{X: r.X, Y: y, W: r.W - 24, H: headH}
		l.Clip(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: headH}, geom.Radii{})
		l.Fill(sp.R(hr), geom.Radii{}, th.Box)
		col := th.Text
		if vant {
			col = th.Primary
		}
		sp.textMid(l, w.indexes[active], hf, hr.X+th.PadH, hr, col)
		hairline(l, sp.R(hr), sp.R(hr).X, th.Border)
		l.Pop()
	})
	return w
}

func (w *IndexBarW) side(st *indexState, tops []float32) {
	c, th := w.c, w.th
	n := len(w.indexes)
	itemH := float32(16)
	if th.Style == Vant {
		itemH = 18
	}
	// The rail sits at the end edge (Abs right: the layout mirrors it to
	// the left in RTL).
	bar := ui.Div(c, layout.Abs(layout.Unset, 0, 0, 0), layout.W(24))
	w.rail = bar.ID()
	g := Area(c, bar, AxisVertical, nil)
	r, ok := c.RectOf(bar.ID())
	jump := func(i int) {
		st.jump, st.active = i, i
		c.Invalidate()
	}
	pick := func(p geom.Point) {
		if !ok || n == 0 {
			return
		}
		top := r.Y + (r.H-itemH*float32(n))/2
		i := int((p.Y - top) / itemH)
		jump(max(0, min(i, n-1)))
	}
	if g.Tap {
		pick(g.TapPos)
	}
	if g.Pan.Active {
		if p, in := c.Pointer(); in {
			pick(p)
		}
	}
	// Keyboard: the index is one focus stop, Up / Down / Home / End jump
	// between groups. Screen readers see a vertical tab list of letters.
	ia := bar.Focusable(input.CursorDefault)
	if ia.Focused && n > 0 {
		to := -1
		for _, k := range keyPresses(c, bar.ID()) {
			cur := st.active
			if to >= 0 {
				cur = to
			}
			switch k {
			case input.KeyUp:
				to = max(0, cur-1)
			case input.KeyDown:
				to = min(n-1, cur+1)
			case input.KeyHome:
				to = 0
			case input.KeyEnd:
				to = n - 1
			}
		}
		if to >= 0 {
			jump(to)
		}
	}
	if c.A11yEnabled() && n > 0 {
		bar.Semantics(a11y.RoleTabList, "").A11yState(a11y.Vertical)
		var sel ui.ID
		top := (r.H - itemH*float32(n)) / 2
		bar.Body(func() {
			for i, s := range w.indexes {
				t := ui.Div(c, layout.Abs(0, top+float32(i)*itemH, layout.Unset, layout.Unset), layout.Size(24, itemH))
				t.Semantics(a11y.RoleTab, s).A11yState(a11y.Selectable | stateIf(i == st.active, a11y.Selected)).A11y(pressSem)
				if i == st.active {
					sel = t.ID()
				}
				if act, _, ok := c.A11yAction(t.ID()); ok && act == a11y.Press {
					jump(i)
				}
			}
		})
		bar.A11y(func(s *a11y.Node) {
			if s.States.Has(a11y.Focused) {
				s.ActiveDescendant = a11y.NodeID(sel)
			}
		})
	}
	active := st.active
	ring := ia.FocusVisible
	idx := w.indexes
	bar.Paint(func(l *paint.List, r geom.Rect) {
		fs := th.Font(th.FontTiny, text.Medium)
		fs.LineHeight = itemH
		top := r.Y + (r.H-itemH*float32(len(idx)))/2
		for i, s := range idx {
			cell := geom.Rect{X: r.X, Y: top + float32(i)*itemH, W: r.W, H: itemH}
			col := th.TextSecondary
			if i == active {
				if th.Style == Vant {
					col = th.Primary
				} else {
					d := float32(14)
					l.Fill(geom.Rect{X: cell.X + (cell.W-d)/2, Y: cell.Y + (cell.H-d)/2, W: d, H: d}, geom.Rad(d/2), th.Primary)
					col = th.OnPrimary
				}
			}
			drawTextCenter(l, s, fs, cell, col)
			if ring && i == active {
				drawFocusRing(l, th, cell)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// FloatingBubble

type bubbleState struct {
	pos   geom.Point // X measured from the start edge (the right in RTL)
	init  bool
	start geom.Point
	disp  geom.Point // displayed physical position (drives the overlay shift)
}

// FloatingBubbleW is a draggable round button that floats over the page
// and sticks to the nearest side when released.
type FloatingBubbleW struct {
	c       *ui.Context
	th      *Theme
	clicked bool
	b       *ui.Box
}

// FloatingBubble adds a floating bubble showing ic. axis limits dragging
// (AxisBoth by default); magnetic snaps it to the left or right edge. It
// starts in the bottom end corner (bottom right; bottom left in RTL).
//
// ConfigProvider defaults ("mobile.FloatingBubble"): "size" (number, at
// least the 44pt touch minimum; default 48).
func FloatingBubble(c *ui.Context, ic *icon.Icon, axis Axis, magnetic bool) *FloatingBubbleW {
	th := ThemeOf(c)
	w := &FloatingBubbleW{c: c, th: th}
	if axis == AxisNone {
		axis = AxisBoth
	}
	a := anchor(c)
	st := ui.State[bubbleState](c, a.ID(), nil)
	size := float32(48)
	if v, ok := cfgNum(c, "FloatingBubble", "size"); ok && float32(v) >= th.TouchMin {
		size = float32(v)
	}
	vw, vh := c.Size().W, c.Size().H
	safe := SafeAreaInsets(c)
	margin := float32(24)
	if !st.init {
		st.init = true
		st.pos = geom.Pt(vw-size-margin, vh-size-margin-120-safe.Bottom)
		st.disp = st.pos
		if c.IsRTL() {
			st.disp.X = margin // the end corner is on the left
		}
	}
	var g Gesture
	var pr Press
	rtl := c.IsRTL()
	// phys converts a position measured from the start edge to the
	// physical left (and back: the reflection is its own inverse).
	phys := func(x float32) float32 {
		if rtl {
			return vw - size - x
		}
		return x
	}
	sgn := float32(1)
	if rtl {
		sgn = -1
	}
	ui.Overlay(c, ui.OverlayOptions{Place: ui.ScreenTop, ShiftX: st.disp.X - (vw-size)/2, ShiftY: st.disp.Y}, func() {
		detached(c, func() {
			b := ui.Div(c, layout.Size(size, size)).Radius(size / 2).Bg(th.Primary).Shadow([]paint.Shadow{{Y: 4, Blur: 12, Color: theme.RGBA(0, 0, 0, .2)}})
			g = Area(c, b, axis, nil)
			// A button for the keyboard and screen readers (name it with
			// A11yLabel); dragging stays pointer-only.
			w.b = b
			ia := b.Focusable(input.CursorPointer)
			b.Semantics(a11y.RoleButton, "")
			if ia.Clicked && ia.FocusVisible {
				pr.Tap = true
			}
			if act, _, ok := c.A11yAction(b.ID()); ok && act == a11y.Press {
				pr.Tap = true
			}
			ring := ia.FocusVisible
			pressed := c.Dragging(b.ID()) && !g.Pan.Active
			pr.Pressed = pressed
			b.Paint(func(l *paint.List, r geom.Rect) {
				if pressed {
					l.Fill(r, geom.Rad(size/2), theme.RGBA(0, 0, 0, .1))
				}
				spanOf(r, rtl).icon(l, ic, r.X+(r.W-24)/2, r.Y+(r.H-24)/2, 24, th.OnPrimary)
				if ring {
					drawFocusRing(l, th, r)
				}
			})
		})
	})
	clampX := func(x float32) float32 { return max(margin/2, min(x, vw-size-margin/2)) }
	clampY := func(y float32) float32 { return max(safe.Top+margin/2, min(y, vh-size-safe.Bottom-margin/2)) }
	if g.Pan.Began {
		st.start = st.pos
	}
	x, y := st.pos.X, st.pos.Y
	if g.Pan.Active {
		if axis&AxisHorizontal != 0 {
			x = clampX(st.start.X + sgn*g.Pan.Delta.X)
		}
		if axis&AxisVertical != 0 {
			y = clampY(st.start.Y + g.Pan.Delta.Y)
		}
		st.pos = geom.Pt(x, y)
		x = c.Anim(a.ID(), "bx", x, 0)
		y = c.Anim(a.ID(), "by", y, 0)
	} else {
		if g.Pan.Ended && magnetic {
			if st.pos.X+size/2 < vw/2 {
				st.pos.X = margin
			} else {
				st.pos.X = vw - size - margin
			}
		}
		x = c.Anim(a.ID(), "bx", st.pos.X, 300*time.Millisecond)
		y = c.Anim(a.ID(), "by", st.pos.Y, 300*time.Millisecond)
	}
	if d := geom.Pt(phys(x), y); d != st.disp {
		st.disp = d
		c.Invalidate()
	}
	w.clicked = g.Tap || pr.Tap
	return w
}

// A11yLabel sets the accessible name of the bubble (it shows an icon only).
func (w *FloatingBubbleW) A11yLabel(s string) *FloatingBubbleW {
	if w.b != nil {
		w.b.Semantics(a11y.RoleButton, s)
	}
	return w
}

// Clicked reports a tap on the bubble.
func (w *FloatingBubbleW) Clicked() bool { return w.clicked }
