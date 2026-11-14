package ui

import (
	"strconv"
	"time"

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

// StatusBarW is the strip at the bottom of a workbench window (VS Code /
// JetBrains status bar): items at its start (Left) and end (Right).
type StatusBarW struct {
	_ [0]func()

	c           *Context
	th          *theme.Theme
	tk          *theme.StatusBarToken
	box         *Box
	left, right func()
	status      string
	z           *lazy
}

// StatusBar adds a status bar; fill its two sides with StatusBarItem:
//
//	ui.StatusBar(c).
//		Left(func() {
//			if ui.StatusBarItem(c, "main").Icon(icon.Branch).Tooltip("切换分支").Clicked() { ... }
//			ui.StatusBarItem(c, "").Icon(icon.Sync).Loading(true)
//		}).
//		Right(func() {
//			ui.StatusBarItem(c, "Ln 12, Col 5")
//			ui.StatusBarItem(c, "UTF-8").Static()
//		})
//
// Left and right are logical: in RTL the sides swap. The bar stretches
// across its parent column; the left side clips when the items do not fit.
func StatusBar(c *Context) *StatusBarW {
	s := &StatusBarW{c: c}
	s.th = c.ThemeFor(theme.CompStatusBar)
	s.tk = &s.th.Components.StatusBar
	s.box = c.box("statusbar", layout.Row, []layout.Option{layout.Middle()})
	s.box.n.setRole(a11y.RoleToolbar).setName(c.Locale().StatusBar.Label)
	s.box.n.paint = s.paint
	s.z = c.lazy(s.box.n, s.build)
	return s
}

// Left builds the items at the bar's start.
func (s *StatusBarW) Left(fn func()) *StatusBarW { s.left = fn; return s }

// Right builds the items at the bar's end.
func (s *StatusBarW) Right(fn func()) *StatusBarW { s.right = fn; return s }

// Status fills the whole bar with a status color ("primary", "success",
// "warning", "error", "info"; "" the default neutral bar), as VS Code
// colors it while debugging or without a folder. Item text switches to the
// matching on-solid color.
func (s *StatusBarW) Status(st string) *StatusBarW { s.status = st; return s }

// A11yLabel replaces the accessible name (Locale.StatusBar.Label).
func (s *StatusBarW) A11yLabel(name string) *StatusBarW { s.box.n.setName(name); return s }

// Layout adds layout options to the bar (margins, width).
func (s *StatusBarW) Layout(o ...layout.Option) *StatusBarW {
	flexbox.Apply(&s.box.n.ln.Style, o...)
	return s
}

// ID returns the bar's ID.
func (s *StatusBarW) ID() ID { return s.box.n.id }

// statusBarEnv is what a bar tells the items built inside it.
type statusBarEnv struct {
	fg    theme.Color // item text on the bar
	solid bool        // the bar has a status fill
}

// statusBarOf returns the environment of the status bar being built around
// the current node, or nil outside a bar.
func (c *Context) statusBarOf() *statusBarEnv {
	for p := c.cur; p != nil; p = p.parent {
		if e, ok := c.states[hashID(p.id, "statusbar-env")]; ok {
			e.seen = c.pass
			if v, ok := e.v.(*statusBarEnv); ok {
				return v
			}
		}
	}
	return nil
}

// colors returns the bar fill and the item text color on it.
func (s *StatusBarW) colors() (bg, fg theme.Color, solid bool) {
	if s.status == "" {
		return s.tk.Bg, s.tk.ItemColor, false
	}
	bg = s.th.ColorOf(s.status).Base
	return bg, s.th.TextOn(bg, s.th.TextOnSolid), true
}

func (s *StatusBarW) build() {
	c, tk := s.c, s.tk
	s.box.Style(layout.H(tk.Height), layout.PadVH(0, tk.PaddingInline), layout.Gap(tk.Gap))
	_, fg, solid := s.colors()
	// Items find the bar through this slot (statusBarOf).
	*State[statusBarEnv](c, hashID(s.box.n.id, "statusbar-env"), nil) = statusBarEnv{fg: fg, solid: solid}
	side := func(fn func(), grow bool) {
		b := Row(c, func() {
			if fn != nil {
				fn()
			}
		}, layout.Middle(), layout.Gap(tk.Gap), layout.Self(layout.AlignStretch))
		if grow {
			b.Style(layout.Grow(1), layout.Shrink(1), layout.MinW(0))
			b.Clip()
		} else {
			b.Style(layout.Shrink(0))
		}
	}
	side(s.left, true)
	side(s.right, false)
}

func (s *StatusBarW) paint(l *paint.List, r geom.Rect) {
	bg, _, _ := s.colors()
	l.Fill(r, geom.Radii{}, bg)
	if s.tk.BorderColor.A > 0 && s.status == "" {
		l.Fill(geom.Rect{X: r.X, Y: r.Y, W: r.W, H: 1}, geom.Radii{}, s.tk.BorderColor)
	}
}

// StatusBarItemW is one entry of a StatusBar: an icon, text, a count badge
// and a progress bar, in that order, any of them optional.
type StatusBarItemW struct {
	_ [0]func()

	c        *Context
	th       *theme.Theme
	tk       *theme.StatusBarToken
	env      *statusBarEnv
	n        *node
	label    string
	ic       *icon.Icon
	loading  bool
	badge    string
	progress float64 // 0..1; negative: none
	status   string
	tip      string
	static   bool
	disabled bool
	ia       Interaction
	z        *lazy
}

// StatusBarItem adds an entry to the StatusBar being built (outside a bar
// it draws the same, at the bar's height). label is the visible text and
// the accessible name; an icon-only item ("" label) is named by its
// Tooltip or A11yLabel.
//
// Items are buttons by default: hover highlight, Tab stop, Enter / Space
// or a click report Clicked. Static makes a plain read-out.
func StatusBarItem(c *Context, label string) *StatusBarItemW {
	w := &StatusBarItemW{c: c, label: label, progress: -1}
	w.th = c.ThemeFor(theme.CompStatusBar)
	w.tk = &w.th.Components.StatusBar
	w.env = c.statusBarOf()
	w.n = c.add("statusbar-item", label)
	w.n.ln.NoStretch = true
	w.n.ln.Measure = w.measure
	w.n.paint = w.paint
	w.z = c.lazy(w.n, w.build)
	return w
}

// Icon shows ic before the text.
func (w *StatusBarItemW) Icon(ic *icon.Icon) *StatusBarItemW { w.ic = ic; return w }

// Loading spins the icon (icon.Loading when the item has none): a task is
// running.
func (w *StatusBarItemW) Loading(on bool) *StatusBarItemW { w.loading = on; return w }

// Badge shows a count after the text (0 hides it; above 99 shows "99+").
func (w *StatusBarItemW) Badge(n int) *StatusBarItemW {
	switch {
	case n <= 0:
		w.badge = ""
	case n > 99:
		w.badge = "99+"
	default:
		w.badge = strconv.Itoa(n)
	}
	return w
}

// BadgeText shows s in the badge ("" hides it).
func (w *StatusBarItemW) BadgeText(s string) *StatusBarItemW { w.badge = s; return w }

// Progress shows a small determinate progress bar after the text; p runs
// from 0 to 1 (a negative p hides it). Use Loading for work of unknown
// length.
func (w *StatusBarItemW) Progress(p float64) *StatusBarItemW {
	w.progress = min(p, 1)
	return w
}

// Status fills the item with a status color ("warning", "error",
// "success", "info", "primary"), as VS Code marks problems or a remote
// connection.
func (w *StatusBarItemW) Status(s string) *StatusBarItemW { w.status = s; return w }

// Tooltip shows tip above the item while the pointer rests on it or while
// it has keyboard focus; screen readers get it as the description (as the
// name of an icon-only item).
func (w *StatusBarItemW) Tooltip(tip string) *StatusBarItemW { w.tip = tip; return w }

// Static makes the item a plain read-out: no hover highlight, not a Tab
// stop, never Clicked (a Tooltip still shows on hover).
func (w *StatusBarItemW) Static() *StatusBarItemW { w.static = true; return w }

// Disabled dims the item and stops it from reacting.
func (w *StatusBarItemW) Disabled(on bool) *StatusBarItemW { w.disabled = on; return w }

// A11yLabel replaces the accessible name.
func (w *StatusBarItemW) A11yLabel(s string) *StatusBarItemW { w.n.setName(s); return w }

// Clicked reports a click (or Enter / Space) on the item.
func (w *StatusBarItemW) Clicked() bool {
	w.z.run()
	return !w.static && !w.isDisabled() && w.ia.Clicked
}

// Hovered reports whether the pointer is over the item.
func (w *StatusBarItemW) Hovered() bool { w.z.run(); return w.ia.Hovered }

// ID returns the item's ID.
func (w *StatusBarItemW) ID() ID { return w.n.id }

func (w *StatusBarItemW) isDisabled() bool { return w.disabled || w.n.disabled }

func (w *StatusBarItemW) build() {
	c, n := w.c, w.n
	switch {
	case !w.static:
		w.ia = c.interact(n, true, input.CursorPointer)
		n.setRole(a11y.RoleButton)
	case w.tip != "":
		w.ia = c.interact(n, false, input.CursorDefault)
		n.setRole(a11y.RoleText)
	default:
		n.setRole(a11y.RoleText)
	}
	if w.isDisabled() {
		n.focus, n.cursor = false, input.CursorNotAllowed
		n.setState(a11y.Disabled)
		w.ia = Interaction{ID: n.id}
	}
	if !n.aNameSet && w.label == "" && w.tip != "" {
		n.setName(w.tip)
	}
	n.fill(w)
	if w.tip == "" {
		return
	}
	enter := Prop(c, "Tooltip", "mouseEnterDelay", time.Duration(0))
	if HoverDelay(c, n.id, w.ia.Hovered || w.ia.FocusVisible, enter, 0) {
		if r, ok := c.rects[n.id]; ok {
			typoTip(c, n.id, r, w.tip)
		}
	}
}

// semantics describes the item with its tooltip, badge and progress.
func (w *StatusBarItemW) semantics(s *a11y.Node) {
	if w.tip != "" && w.tip != s.Name {
		addDescription(s, w.tip)
	}
	if w.badge != "" {
		addDescription(s, w.badge)
	}
	if w.progress >= 0 {
		s.Value = strconv.Itoa(int(w.progress*100+0.5)) + "%"
	}
	if w.loading {
		s.States |= a11y.Busy
	}
}

func (w *StatusBarItemW) textStyle() text.Style {
	return w.c.style(w.tk.ItemFontSize, text.Regular)
}

func (w *StatusBarItemW) badgeStyle() text.Style {
	st := w.c.style(w.tk.BadgeFontSize, text.Regular)
	st.Weight = text.Medium
	return st
}

func (w *StatusBarItemW) icon() *icon.Icon {
	if w.ic == nil && w.loading {
		return icon.Loading
	}
	return w.ic
}

// parts returns the inline widths of the item's parts (0 when absent).
func (w *StatusBarItemW) parts() (ic, txt, prog, badge float32) {
	tk := w.tk
	if w.icon() != nil {
		ic = tk.IconSize
	}
	if w.label != "" {
		txt = text.Width(w.label, w.textStyle())
	}
	if w.progress >= 0 {
		prog = tk.ProgressWidth
	}
	if w.badge != "" {
		badge = max(tk.BadgeHeight, text.Width(w.badge, w.badgeStyle())+8)
	}
	return
}

func (w *StatusBarItemW) measure(_, _ float32) geom.Size {
	tk := w.tk
	ws := [4]float32{}
	ws[0], ws[1], ws[2], ws[3] = w.parts()
	total, k := 2*tk.ItemPaddingInline, 0
	for _, v := range ws {
		if v > 0 {
			total += v
			k++
		}
	}
	if k > 1 {
		total += float32(k-1) * tk.ItemGap
	}
	return geom.Size{W: total, H: tk.Height}
}

func (w *StatusBarItemW) paint(l *paint.List, r geom.Rect) {
	c, th, tk := w.c, w.th, w.tk
	fg := tk.ItemColor
	solidBar := false
	if w.env != nil {
		fg, solidBar = w.env.fg, w.env.solid
	}
	rad := geom.Rad(tk.ItemRadius)
	if w.isDisabled() {
		l.Opacity(0.5)
		defer l.Pop()
	}
	interactive := !w.static && !w.isDisabled()
	switch {
	case w.status != "":
		bg := th.ColorOf(w.status).Base
		if interactive && w.ia.Hovered {
			bg = th.ColorOf(w.status).Hover
		}
		l.Fill(r, rad, bg)
		fg = th.TextOn(bg, th.TextOnSolid)
	case interactive && solidBar:
		if w.ia.Pressed {
			l.Fill(r, rad, theme.Fade(fg, .2))
		} else if w.ia.Hovered {
			l.Fill(r, rad, theme.Fade(fg, .12))
		}
	case interactive:
		if w.ia.Pressed && tk.ItemActiveBg.A > 0 {
			l.Fill(r, rad, tk.ItemActiveBg)
		} else if w.ia.Hovered && tk.ItemHoverBg.A > 0 {
			l.Fill(r, rad, tk.ItemHoverBg)
		}
		if w.ia.Hovered {
			fg = tk.ItemHoverColor
		}
	}
	icw, txw, pw, bw := w.parts()
	sp := c.inline(r)
	off := tk.ItemPaddingInline
	cy := r.Y + r.H/2
	advance := func(v float32) {
		if v > 0 {
			off += v + tk.ItemGap
		}
	}
	if ic := w.icon(); ic != nil {
		ang := 0.0
		if w.loading {
			ang = spinAngle(c)
		}
		c.paintIcon(l, ic, sp.Rect(off, cy-icw/2, icw, icw), fg, ang)
		advance(icw)
	}
	if w.label != "" {
		lay := text.Shape(w.label, w.textStyle(), c.textOpts(text.Options{NoWrap: true}))
		l.Glyphs(geom.Pt(sp.X(off, txw), cy-lay.Height/2), lay.Glyphs, fg)
		advance(txw)
	}
	if pw > 0 {
		h := tk.ProgressHeight
		trackC, barC := tk.ProgressTrack, tk.ProgressColor
		if solidBar || w.status != "" {
			trackC, barC = theme.Fade(fg, .3), fg
		}
		l.Fill(sp.Rect(off, cy-h/2, pw, h), geom.Rad(h/2), trackC)
		if v := float32(max(w.progress, 0)) * pw; v > 0 {
			l.Fill(sp.Rect(off, cy-h/2, v, h), geom.Rad(h/2), barC)
		}
		advance(pw)
	}
	if bw > 0 {
		h := tk.BadgeHeight
		br := sp.Rect(off, cy-h/2, bw, h)
		bg, col := tk.BadgeBg, tk.BadgeColor
		if solidBar || w.status != "" {
			bg, col = theme.Fade(fg, .25), fg
		}
		l.Fill(br, geom.Rad(h/2), bg)
		lay := text.Shape(w.badge, w.badgeStyle(), text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(br.X+(br.W-lay.Width)/2, br.Y+(br.H-lay.Height)/2), lay.Glyphs, col)
	}
	if w.ia.FocusVisible {
		l.Stroke(geom.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}, rad, 2, th.FocusRing)
	}
}
