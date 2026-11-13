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

// ---------------------------------------------------------------------------
// CheckableTag (Ant Tag.CheckableTag / Element el-check-tag)

// CheckableTagW is a tag that toggles between checked and unchecked.
type CheckableTagW struct {
	c        *Context
	n        *node
	label    string
	val      *bool
	ic       *icon.Icon
	typ      string
	disabled bool
	changed  bool
	ia       Interaction
	z        *lazy
}

// CheckableTag adds a tag bound to *checked; clicking (or Enter/Space when
// focused) toggles it.
func CheckableTag(c *Context, label string, checked *bool) *CheckableTagW {
	w := &CheckableTagW{c: c, label: label, val: checked}
	w.n = c.add("checktag", label)
	w.n.ln.NoStretch = true
	w.n.ln.AlignSelf = layout.AlignAuto
	w.n.ln.Measure = w.measure
	w.n.paint = w.paint
	w.n.setRole(a11y.RoleButton).setState(a11y.Toggleable).fill(w)
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.z = c.lazy(w.n, w.finish)
	return w
}

// A11yLabel sets the accessible name (needed for an icon-only tag).
func (w *CheckableTagW) A11yLabel(s string) *CheckableTagW { w.n.setName(s); return w }

func (w *CheckableTagW) semantics(s *a11y.Node) {
	if *w.val {
		s.States |= a11y.Pressed
	}
}

// Icon puts an icon before the label.
func (w *CheckableTagW) Icon(ic *icon.Icon) *CheckableTagW { w.ic = ic; return w }

// Type tints the checked state with a status palette ("primary" default,
// "success", "warning", "error", "info"). Element Plus also tints the
// unchecked state.
func (w *CheckableTagW) Type(status string) *CheckableTagW { w.typ = status; return w }

// Disabled blocks toggling when on.
func (w *CheckableTagW) Disabled(on bool) *CheckableTagW {
	if on {
		w.disabled, w.n.focus, w.n.cursor = true, false, input.CursorNotAllowed
	}
	return w
}

// Changed reports a toggle this frame.
func (w *CheckableTagW) Changed() bool { w.z.run(); return w.changed }

// Checked reports the current value.
func (w *CheckableTagW) Checked() bool { w.z.run(); return *w.val }

func (w *CheckableTagW) isDisabled() bool { return w.disabled || w.n.disabled }

func (w *CheckableTagW) finish() {
	if w.ia.Clicked && !w.isDisabled() {
		*w.val = !*w.val
		w.changed = true
		w.c.invalid = true
	}
}

func (w *CheckableTagW) metrics() (font text.Style, padX, h, is float32) {
	th := w.c.Theme
	if th.Flavor == theme.Element {
		return w.c.style(th.FontSize, text.Bold), 15, 28, 14
	}
	if th.Flavor == theme.StyleShadcn {
		// Badge: px-2 py-0.5 text-xs font-medium, size-3 icons.
		return w.c.style(th.FontSizeSM, text.Medium), 8, 22, 12
	}
	return w.c.style(th.FontSizeSM, text.Regular), 8, 22, 12
}

func (w *CheckableTagW) measure(_, _ float32) geom.Size {
	font, padX, h, is := w.metrics()
	tw := text.Width(w.label, font)
	if w.ic != nil {
		tw += is
		if w.label != "" {
			tw += 4
		}
	}
	return geom.Size{W: tw + 2*padX, H: h}
}

func (w *CheckableTagW) colors() (bg, fg theme.Color) {
	th := w.c.Theme
	on := *w.val
	dis := w.isDisabled()
	pressed := w.ia.Pressed
	hov := w.ia.Hovered
	if th.Flavor == theme.Element {
		pal := th.Primary
		if w.typ != "" {
			pal = th.ColorOf(w.typ)
		}
		off := th.Info
		if w.typ != "" {
			off = pal
		}
		switch {
		case dis && on:
			return pal.Shades[1], th.TextDisabled
		case dis:
			return off.Bg, th.TextDisabled
		case on && (hov || pressed):
			return pal.Shades[2], pal.Base
		case on:
			return pal.Shades[1], pal.Base
		case hov || pressed:
			return off.Shades[2], off.Base
		}
		return off.Bg, off.Base
	}
	pal := th.Primary
	if w.typ != "" && w.typ != "primary" {
		pal = th.ColorOf(w.typ)
	}
	switch {
	case dis && on:
		return th.BgContainerDisabled, th.TextDisabled
	case dis:
		return theme.Color{}, th.TextDisabled
	case pressed:
		return pal.Active, th.TextOnPrimary
	case on && hov:
		return pal.Hover, th.TextOnPrimary
	case on:
		return pal.Base, th.TextOnPrimary
	case hov:
		return theme.Color{}, pal.Base
	}
	return theme.Color{}, th.Text
}

func (w *CheckableTagW) paint(l *paint.List, r geom.Rect) {
	th := w.c.Theme
	el := th.Flavor == theme.Element
	font, padX, _, is := w.metrics()
	bg, fg := w.colors()
	if th.Flavor == theme.StyleShadcn {
		w.paintShadcn(l, r, font, padX, is)
		return
	}
	rad := th.RadiusSM
	if el {
		rad = th.Radius
	}
	if bg.A > 0 {
		l.Fill(r, geom.Rad(rad), bg)
	}
	s := w.c.inline(r) // icon then label from the inline start
	off := padX
	if w.ic != nil {
		w.c.drawIconAt(l, w.ic, s.X(off, is), r.Y+(r.H-is)/2, is, fg)
		off += is + 4
	}
	if w.label != "" {
		lay := text.Shape(w.label, font, w.c.textOpts(text.Options{NoWrap: true}))
		l.Glyphs(geom.Pt(s.X(off, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
	}
	if w.ia.FocusVisible {
		if el {
			l.Stroke(r.Expand(2), geom.Rad(rad+2), 2, th.Primary.Shades[3])
		} else {
			l.Stroke(r.Expand(3), geom.Rad(rad+3), 2, th.FocusRing)
		}
	}
}
