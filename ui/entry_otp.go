package ui

import (
	"strings"
	"time"

	"github.com/tangoui/tango/ui/a11y"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// OTPW is a one-time-password input: one box per character.
type OTPW struct {
	c         *Context
	n         *node
	cells     []*node
	ias       []Interaction
	val       *string
	st        *otpState
	mask      string
	numeric   bool
	disabled  bool
	status    string
	size      ControlSize
	changed   bool
	completed bool
	z         *lazy
}

type otpState struct {
	cells    []string
	activity time.Time
}

// OTP adds an n-character code input bound to *val.
func OTP(c *Context, val *string, n int) *OTPW {
	n = max(1, n)
	w := &OTPW{c: c, val: val, status: c.itemStatus, size: c.defaultSize()}
	w.configDefaults()
	w.n = c.add("otp", "")
	w.n.ln.Dir = layout.Row
	w.n.ln.Gap = 8
	if c.Theme.Flavor == theme.StyleShadcn {
		w.n.ln.Gap = 0 // shadcn InputOTP: one group of joined slots
	}
	w.n.ln.NoStretch = true
	w.n.setRole(a11y.RoleGroup)
	w.st = State[otpState](c, w.n.id, nil)
	if len(w.st.cells) != n {
		w.st.cells = make([]string, n)
		w.st.cells[0] = "\x00" // force a sync below
	}
	w.sync()
	w.ias = make([]Interaction, n)
	loc := c.Locale()
	c.body(w.n, func() {
		for i := 0; i < n; i++ {
			i := i
			cell := c.add("otp-cell", "otp-"+itoaEnt(i))
			cell.ownKeys = true
			cell.setRole(a11y.RoleTextbox).setName(loc.Digit(i+1, n))
			if c.a11yOn {
				cell.fill(semFunc(func(s *a11y.Node) {
					if i < len(w.st.cells) {
						s.Value = w.st.cells[i]
					}
					if w.mask != "" {
						s.States |= a11y.Protected
					}
					if w.status == "error" {
						s.States |= a11y.Invalid
					}
				}))
			}
			cell.ln.Measure = func(_, _ float32) geom.Size {
				h := c.controlHeight(w.size)
				return geom.Size{W: h, H: h}
			}
			cell.paint = func(l *paint.List, r geom.Rect) { w.paintCell(l, r, i) }
			w.cells = append(w.cells, cell)
			w.ias[i] = c.interact(cell, true, input.CursorText)
		}
	})
	w.z = c.lazy(w.n, w.finish)
	return w
}

// sync resets the boxes when the bound value changed elsewhere.
func (w *OTPW) sync() {
	cells := w.st.cells
	if strings.Join(cells, "") == *w.val {
		return
	}
	rs := []rune(*w.val)
	for i := range cells {
		cells[i] = ""
		if i < len(rs) {
			cells[i] = string(rs[i])
		}
	}
}

// A11yLabel names the group of boxes (inside a FormItem use the item
// label); each box is announced as "character n of total".
func (w *OTPW) A11yLabel(s string) *OTPW { w.n.setName(s); return w }

// Mask shows s instead of each character (e.g. "•").
func (w *OTPW) Mask(s string) *OTPW { w.mask = s; return w }

// Numeric accepts digits only.
func (w *OTPW) Numeric() *OTPW           { w.numeric = true; return w }
func (w *OTPW) Status(s string) *OTPW    { w.status = s; return w }
func (w *OTPW) Size(s ControlSize) *OTPW { w.size = s; return w }
func (w *OTPW) Gap(px float32) *OTPW     { w.n.ln.Gap = px; return w }
func (w *OTPW) Disabled(on bool) *OTPW {
	if on {
		w.disabled = true
		for _, k := range w.cells {
			k.disabled, k.focus, k.cursor = true, false, input.CursorNotAllowed
		}
	}
	return w
}

// Changed reports an edit this frame.
func (w *OTPW) Changed() bool { w.z.run(); return w.changed }

// Completed reports an edit that filled every box.
func (w *OTPW) Completed() bool { w.z.run(); return w.completed }

// Focus focuses the first empty box.
func (w *OTPW) Focus() *OTPW {
	for i, s := range w.st.cells {
		if s == "" {
			w.c.SetFocus(w.cells[i].id)
			return w
		}
	}
	w.c.SetFocus(w.cells[len(w.cells)-1].id)
	return w
}

// CellID returns the ID of box i.
func (w *OTPW) CellID(i int) ID { return w.cells[i].id }

func (w *OTPW) accept(r rune) bool {
	if r < ' ' {
		return false
	}
	if w.numeric {
		return r >= '0' && r <= '9'
	}
	return r != ' '
}

// joined reports whether the slots form one shadcn group (no gap).
func (w *OTPW) joined() bool {
	return w.c.Theme.Flavor == theme.StyleShadcn && w.n.ln.Gap == 0
}

func (w *OTPW) finish() {
	c := w.c
	if w.joined() {
		for i, cell := range w.cells {
			if i > 0 {
				cell.ln.Margin.Left = -1 // share the border with the left slot
			}
		}
		w.n.over = w.paintGroupFocus
	}
	w.sync()
	cells := w.st.cells
	n := len(cells)
	if w.disabled || w.n.disabled {
		return
	}
	edited := false
	focusTo := -1
	put := func(from int, s string) int {
		p := from
		for _, r := range s {
			if p >= n {
				break
			}
			if !w.accept(r) {
				continue
			}
			cells[p] = string(r)
			p++
			edited = true
		}
		return p
	}
	for i, cell := range w.cells {
		if v, ok := c.takeSetValue(cell.id); ok {
			// Assistive technology replaces the code from this box on.
			for j := i; j < n; j++ {
				cells[j] = ""
			}
			edited = true
			if p := put(i, v); p > i {
				focusTo = min(p, n-1)
			}
		}
		if c.focus != cell.id {
			continue
		}
		for _, t := range c.TextFor(cell.id) {
			p := put(i, t)
			if p > i {
				focusTo = min(p, n-1)
			}
		}
		for _, k := range c.KeysFor(cell.id) {
			if !k.Down {
				continue
			}
			switch k.Key {
			case input.KeyBackspace:
				if cells[i] != "" {
					cells[i] = ""
					edited = true
				} else if i > 0 {
					cells[i-1] = ""
					edited = true
					focusTo = i - 1
				}
			case input.KeyDelete:
				if cells[i] != "" {
					cells[i] = ""
					edited = true
				}
			case input.KeyLeft, input.KeyRight:
				// Visual arrows: the cells follow the inline direction.
				if prev, _ := c.inlineKey(k.Key); prev {
					focusTo = max(0, i-1)
				} else {
					focusTo = min(n-1, i+1)
				}
			case input.KeyHome:
				focusTo = 0
			case input.KeyEnd:
				focusTo = n - 1
			case input.KeyV:
				if k.Mods.Shortcut() && c.win != nil {
					p := put(i, c.win.ReadClipboard())
					if p > i {
						focusTo = min(p, n-1)
					}
				}
			}
		}
		w.st.activity = c.now
	}
	if focusTo >= 0 {
		c.SetFocus(w.cells[focusTo].id)
		w.st.activity = c.now
	}
	if edited {
		v := strings.Join(cells, "")
		if v != *w.val {
			*w.val = v
			w.changed = true
			full := true
			for _, s := range cells {
				full = full && s != ""
			}
			w.completed = full
		}
		c.invalid = true
	}
}

// slotRadii rounds only the outer corners of the first and last slot.
func (w *OTPW) slotRadii(i int) geom.Radii {
	r := w.c.controlRadius(w.size)
	var out geom.Radii
	if i == 0 {
		out.TL, out.BL = r, r
	}
	if i == len(w.cells)-1 {
		out.TR, out.BR = r, r
	}
	return out
}

// paintGroupFocus draws the focused slot's ring border and halo above its
// neighbours (shadcn: border-ring + ring-[3px] ring-ring/50).
func (w *OTPW) paintGroupFocus(l *paint.List, _ geom.Rect) {
	c, th := w.c, w.c.Theme
	for i, cell := range w.cells {
		if c.focus != cell.id || w.disabled || cell.disabled {
			continue
		}
		r := cell.abs
		rad := w.slotRadii(i)
		bc, halo := th.ControlFocusBorder, th.ControlOutline
		if w.status == "error" {
			bc, halo = th.Error.Base, th.ControlOutlineError
		}
		if halo.A > 0 && th.ControlOutlineWidth > 0 {
			g := th.ControlOutlineWidth
			l.Stroke(r.Expand(g), rad.Grow(g), g, halo)
		}
		l.Stroke(r, rad, 1, bc)
	}
}

func (w *OTPW) paintCell(l *paint.List, r geom.Rect, i int) {
	c, th := w.c, w.c.Theme
	cell := w.cells[i]
	dis := w.disabled || cell.disabled
	focused := c.focus == cell.id
	rad := geom.Rad(c.controlRadius(w.size))
	if w.joined() {
		rad = w.slotRadii(i)
		// The focus halo is drawn over the whole group (paintGroupFocus) so
		// the next slot does not cover it.
		c.paintFrame(l, r, rad, cell.id, frameState{disabled: dis, status: w.status})
	} else {
		c.paintFrame(l, r, rad, cell.id, frameState{hover: w.ias[i].Hovered, focus: focused, disabled: dis, status: w.status})
	}
	fs := c.controlFont(w.size)
	st := c.style(fs, text.Regular)
	col := th.Text
	if dis {
		col = th.TextDisabled
	}
	s := w.st.cells[i]
	if s != "" && w.mask != "" {
		s = w.mask
	}
	comp := ""
	if focused {
		comp, _ = c.CompositionFor(cell.id)
	}
	if comp != "" {
		s = comp
	}
	lay := text.Shape(s, st, text.Options{NoWrap: true})
	x := r.X + (r.W-lay.Width)/2
	y := r.Y + (r.H-lay.Height)/2
	switch {
	case s == "•" && comp == "":
		// The font's bullet is tiny at control size; draw a proper dot.
		l.Path(paint.NewPath().Circle(r.X+r.W/2, r.Y+r.H/2, fs/5), col)
	case s != "":
		l.Glyphs(geom.Pt(x, y), lay.Glyphs, col)
	}
	if comp != "" {
		l.Fill(geom.Rect{X: x, Y: y + lay.Height - 3, W: lay.Width, H: 1}, geom.Radii{}, col)
	}
	if !focused {
		return
	}
	lh := th.LineHeightFor(fs)
	caret := geom.Rect{X: x + lay.Width, Y: r.Y + (r.H-lh)/2 + 2, W: 1, H: lh - 4}
	if s == "" {
		caret.X = r.X + r.W/2
	}
	c.WantTextInput(caret)
	if s != "" && comp == "" {
		return // a filled, focused box shows its character without a caret
	}
	phase := c.now.Sub(w.st.activity) % (1060 * time.Millisecond)
	if phase < 530*time.Millisecond {
		l.Fill(caret, geom.Radii{}, th.Text)
		c.WakeAt(c.now.Add(530*time.Millisecond - phase))
	} else {
		c.WakeAt(c.now.Add(1060*time.Millisecond - phase))
	}
}
