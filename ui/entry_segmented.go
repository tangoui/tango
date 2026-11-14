package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// SegmentedW is a segmented control (Ant Segmented / Element el-segmented).
type SegmentedW struct {
	_ [0]func()

	c        *Context
	th       *theme.Theme          // Segmented theme (its Alias applied)
	tk       *theme.SegmentedToken // &th.Components.Segmented
	n        *node
	val      *string
	opts     []Option
	icons    []*icon.Icon
	block    bool
	size     ControlSize
	disabled bool
	changed  bool
	ia       Interaction
	z        *lazy
	items    []*node
	itemIA   []Interaction
	vertical bool
	shape    SegmentedShape
}

// SegmentedShape is the outline of a Segmented control.
type SegmentedShape string

const (
	SegmentedShapeDefault SegmentedShape = "default" // the control's corner radius
	SegmentedShapeRound   SegmentedShape = "round"   // pill-shaped track and thumb (Ant shape="round")
)

// Segmented adds a segmented control bound to *val.
func Segmented(c *Context, val *string, opts []Option) *SegmentedW {
	w := &SegmentedW{c: c, val: val, opts: opts, size: c.defaultSize()}
	w.th = c.ThemeFor(theme.CompSegmented)
	w.tk = &w.th.Components.Segmented
	w.n = c.add("segmented", "")
	w.n.ln.Dir = layout.Row
	w.n.ln.NoStretch = true
	w.n.ownKeys = true
	w.n.paint = w.paintTrack
	w.n.over = w.paintOver
	// The track keeps focus; arrow keys move the selection (radio group
	// with the checked segment as active descendant).
	w.n.setRole(a11y.RoleRadioGroup).setState(a11y.Horizontal).fill(w)
	w.ia = c.interact(w.n, true, input.CursorDefault)
	w.configDefaults()
	w.z = c.lazy(w.n, w.build)
	return w
}

// A11yLabel sets the accessible name of the group (inside a FormItem the
// item label is used).
func (w *SegmentedW) A11yLabel(s string) *SegmentedW { w.n.setName(s); return w }

func (w *SegmentedW) semantics(s *a11y.Node) {
	if i := w.selected(); i >= 0 && i < len(w.items) {
		s.ActiveDescendant = a11y.NodeID(w.items[i].id)
	}
}

// Icons sets an icon per option (nil entries allowed).
func (w *SegmentedW) Icons(ics ...*icon.Icon) *SegmentedW { w.icons = ics; return w }

// Block stretches the control to its container, items share the width.
func (w *SegmentedW) Block() *SegmentedW {
	w.block = true
	w.n.ln.NoStretch = false
	w.n.ln.AlignSelf = layout.AlignStretch
	return w
}

// Size sets the control size.
func (w *SegmentedW) Size(s ControlSize) *SegmentedW { w.size = s; return w }

// Vertical stacks the options top to bottom (Ant orientation="vertical",
// Element direction="vertical"); they share the width of the widest one
// and the up / down arrow keys move the selection.
func (w *SegmentedW) Vertical() *SegmentedW {
	w.vertical = true
	flexbox.Apply(&w.n.ln.Style, layout.Dir(layout.Column), layout.AlignItems(layout.AlignStretch))
	w.n.aState &^= a11y.Horizontal
	w.n.setState(a11y.Vertical)
	return w
}

// Shape sets the outline: SegmentedShapeRound makes the track and the
// thumb pill-shaped.
func (w *SegmentedW) Shape(s SegmentedShape) *SegmentedW { w.shape = s; return w }

// Disabled disables every option.
func (w *SegmentedW) Disabled(on bool) *SegmentedW {
	if on {
		w.disabled, w.n.focus = true, false
	}
	return w
}

// Changed reports a new selection this frame.
func (w *SegmentedW) Changed() bool { w.z.run(); return w.changed }

func (w *SegmentedW) dis() bool { return w.disabled || w.n.disabled }

func (w *SegmentedW) itemH() float32 { return controlHeightOf(w.th, w.size) - 2*w.trackPad() }

// trackPad is the inset of the items inside the track.
func (w *SegmentedW) trackPad() float32 { return w.tk.TrackPadding }

// weight is the item label weight.
func (w *SegmentedW) weight() text.Weight { return themeWeight(w.tk.ItemFontWeight) }

func (w *SegmentedW) padX() float32 {
	if w.size == Small {
		return w.tk.ItemPaddingInlineSM
	}
	return w.tk.ItemPaddingInline
}

func (w *SegmentedW) font() float32 { return controlFontOf(w.th, w.size) }

func (w *SegmentedW) radii() (outer, inner float32) {
	th := w.th
	if w.shape == SegmentedShapeRound {
		ih := w.itemH()
		return ih/2 + w.trackPad(), ih / 2
	}
	if th.Flavor == theme.Element {
		return th.Radius, th.Radius
	}
	if th.Flavor == theme.StyleShadcn {
		return th.RadiusLG, th.Radius
	}
	switch w.size {
	case Small:
		return th.RadiusSM, th.RadiusXS
	case Large:
		return th.RadiusLG, th.Radius
	}
	return th.Radius, th.RadiusSM
}

func (w *SegmentedW) iconAt(i int) *icon.Icon {
	if i < len(w.icons) {
		return w.icons[i]
	}
	return nil
}

func (w *SegmentedW) selected() int {
	for i, o := range w.opts {
		if o.Value == *w.val {
			return i
		}
	}
	return -1
}

func (w *SegmentedW) pick(i int) {
	if i < 0 || i >= len(w.opts) || w.opts[i].Disabled {
		return
	}
	if *w.val != w.opts[i].Value {
		*w.val = w.opts[i].Value
		w.changed = true
		w.c.invalid = true
	}
}

func (w *SegmentedW) build() {
	c := w.c
	w.n.ln.Padding = geom.All(w.trackPad())
	w.n.ln.Gap = 0
	if w.dis() {
		w.n.cursor = input.CursorNotAllowed
	}
	for i, o := range w.opts {
		i, o := i, o
		it := c.add("segmented-item", o.Label)
		it.setRole(a11y.RoleRadio).setState(a11y.Checkable).stateIf(o.Value == *w.val, a11y.Checked)
		if o.Label == "" {
			it.setName(o.Value) // icon-only segment
		}
		it.aKbd = true // arrow keys of the focused track
		it.ln.Height = w.itemH()
		ic := w.iconAt(i)
		it.ln.Measure = func(_, _ float32) geom.Size {
			tw := entTextW(c, o.Label, w.font(), w.weight())
			if ic != nil {
				tw += w.font() + 6
				if o.Label == "" {
					tw -= 6
				}
			}
			return geom.Size{W: tw + 2*w.padX(), H: w.itemH()}
		}
		if w.block {
			it.ln.Grow = 1
			it.ln.HasBasis = true
		}
		if o.Disabled || w.dis() {
			it.disabled = true
		}
		cur := input.CursorPointer
		if it.disabled {
			cur = input.CursorNotAllowed
		}
		ia := c.interact(it, false, cur)
		if ia.Clicked && !it.disabled {
			w.pick(i)
			c.SetFocus(w.n.id)
		}
		w.items = append(w.items, it)
		w.itemIA = append(w.itemIA, ia)
		it.paint = func(l *paint.List, r geom.Rect) { w.paintItem(l, r, i, ia) }
	}
	if w.dis() {
		return
	}
	for _, k := range c.KeysFor(w.n.id) {
		if !k.Down {
			continue
		}
		d := 0
		switch prev, next := c.inlineKey(k.Key); {
		case prev || k.Key == input.KeyUp:
			d = -1
		case next || k.Key == input.KeyDown:
			d = 1
		}
		if d == 0 || len(w.opts) == 0 {
			continue
		}
		i := w.selected()
		for range w.opts {
			i = (i + d + len(w.opts)) % len(w.opts)
			if !w.opts[i].Disabled {
				w.pick(i)
				break
			}
		}
	}
}

func (w *SegmentedW) colors() (track, thumb, fg, fgSel, hoverBg theme.Color) {
	tk := w.tk
	return tk.TrackBg, tk.ItemSelectedBg, tk.ItemColor, tk.ItemSelectedColor, tk.ItemHoverBg
}

func (w *SegmentedW) paintTrack(l *paint.List, r geom.Rect) {
	c, th := w.c, w.th
	track, thumb, _, _, _ := w.colors()
	outer, inner := w.radii()
	l.Fill(r, geom.Rad(outer), track)
	sel := w.selected()
	if sel < 0 || sel >= len(w.items) {
		return
	}
	// The thumb slides along logical offsets (from the inline start), so
	// it moves the same way as the selection in both directions.
	kr := w.items[sel].ln.Rect
	var tr geom.Rect
	if w.vertical {
		y := c.Anim(w.n.id, "seg-y", kr.Y, th.DurationSlow)
		ht := c.Anim(w.n.id, "seg-ht", kr.H, th.DurationSlow)
		tr = geom.Rect{X: r.X + kr.X, Y: r.Y + y, W: kr.W, H: ht}
	} else {
		s := c.inline(r)
		lo := kr.X
		if c.IsRTL() {
			lo = r.W - kr.X - kr.W
		}
		x := c.Anim(w.n.id, "seg-x", lo, th.DurationSlow)
		wd := c.Anim(w.n.id, "seg-w", kr.W, th.DurationSlow)
		tr = s.Rect(x, r.Y+kr.Y, wd, kr.H)
	}
	if w.dis() {
		if th.Flavor == theme.Element {
			thumb = th.Primary.Border
		} else if th.Flavor == theme.StyleShadcn {
			l.Opacity(0.5)
			defer l.Pop()
		} else {
			thumb = th.Fill
			thumb.A /= 2
		}
	}
	if len(w.tk.ItemSelectedShadow) > 0 && !w.dis() {
		l.Shadows(tr, geom.Rad(inner), w.tk.ItemSelectedShadow)
	}
	l.Fill(tr, geom.Rad(inner), thumb)
}

func (w *SegmentedW) paintItem(l *paint.List, r geom.Rect, i int, ia Interaction) {
	c, th := w.c, w.th
	_, _, fg, fgSel, hoverBg := w.colors()
	_, inner := w.radii()
	o := w.opts[i]
	sel := o.Value == *w.val
	col := fg
	switch {
	case w.items[i].disabled && th.Flavor == theme.StyleShadcn:
		col = theme.Fade(fg, .5)
		if sel {
			col = theme.Fade(fgSel, .5)
		}
	case w.items[i].disabled:
		col = th.TextDisabled
		if sel && th.Flavor == theme.Element && w.dis() {
			col = th.TextOnPrimary
		}
	case sel:
		col = fgSel
	case ia.Hovered:
		col = w.tk.ItemHoverColor
		h := c.Anim(w.items[i].id, "seg-h", 1, th.DurationMid)
		l.Fill(r, geom.Rad(inner), theme.Fade(hoverBg, float64(h)))
	}
	fs := w.font()
	tw := entTextW(c, o.Label, fs, w.weight())
	ic := w.iconAt(i)
	total := tw
	if ic != nil {
		total += fs
		if o.Label != "" {
			total += 6
		}
	}
	// Icon then label from the inline start, centred as a group.
	off := (r.W - total) / 2
	if ic != nil {
		c.drawIconAt(l, ic, c.inline(r).X(off, fs), r.Y+(r.H-fs)/2, fs, col)
		off += fs + 6
	}
	maxW := r.W - 2*w.padX()
	if ic != nil {
		maxW -= fs + 6
	}
	entTextIn(c, l, o.Label, off, r, fs, w.weight(), col, max(maxW, tw))
}

func (w *SegmentedW) paintOver(l *paint.List, r geom.Rect) {
	if !w.ia.FocusVisible {
		return
	}
	sel := w.selected()
	if sel < 0 || sel >= len(w.items) {
		return
	}
	_, inner := w.radii()
	kr := w.items[sel].ln.Rect
	entFocusRing(w.c, l, geom.Rect{X: r.X + kr.X, Y: r.Y + kr.Y, W: kr.W, H: kr.H}, inner)
}
