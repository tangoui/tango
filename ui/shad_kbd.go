package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// Kbd shows a keyboard key or shortcut (shadcn/ui Kbd): small muted
// rounded box in the text font.
func Kbd(c *Context, keys string) *Box {
	th := c.Theme
	st := c.style(th.FontSizeSM, text.Regular)
	st.Weight = text.Medium
	b := Custom(c, func(_, _ float32) geom.Size {
		return geom.Size{W: max(20, text.Width(keys, st)+8), H: 20}
	}, func(l *paint.List, r geom.Rect) {
		rad := geom.Rad(th.RadiusSM)
		l.Fill(r, rad, th.FillSecondary)
		if th.Flavor != theme.StyleShadcn {
			// Ant / Element <kbd>: bordered key cap.
			l.Stroke(r, rad, 1, th.Border)
		}
		lay := text.Shape(keys, st, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, th.TextTertiary)
	})
	b.n.label = keys
	b.n.setRole(a11y.RoleText) // the key names are read as text
	b.n.ln.NoStretch = true
	return b
}
