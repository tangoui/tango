package ui

import (
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Shared helpers of the second batch of Pro components (PageContainer,
// FooterToolbar, ProCard, StatisticCard, CheckCard, ProSkeleton). Their
// texts come from Locale.PageContainer / ProCard / StatisticCard.

// pro2El reports the Element Plus flavor.
func pro2El(c *Context) bool { return c.Theme.Flavor == theme.Element }

// pro2Shad reports the shadcn flavor.
func pro2Shad(c *Context) bool { return c.Theme.Flavor == theme.StyleShadcn }

// pro2Box creates a container node of kind whose label (usually the
// visible title) takes part in the ID and lets Driver.Find locate it.
func pro2Box(c *Context, kind, label string, dir layout.Direction, opts []layout.Option) *Box {
	n := c.add(kind, label)
	n.ln.Dir = dir
	n.ln.Style.Apply(opts...)
	b := &Box{c: c, n: n}
	n.paint = b.paintBox
	return b
}

// pro2LastKid returns the most recently built child of the current parent.
func pro2LastKid(c *Context) *node {
	if c.cur == nil || len(c.cur.kids) == 0 {
		return nil
	}
	return c.cur.kids[len(c.cur.kids)-1]
}

// pro2FadeShadows scales the alpha of every shadow layer by a.
func pro2FadeShadows(src []paint.Shadow, a float32) []paint.Shadow {
	out := make([]paint.Shadow, len(src))
	for i, s := range src {
		s.Color = theme.Fade(s.Color, float64(a))
		out[i] = s
	}
	return out
}

// pro2Triangle paints a small solid triangle centred in r pointing up or
// down (statistic trend marks).
func pro2Triangle(l *paint.List, r geom.Rect, up bool, col theme.Color) {
	cx := r.X + r.W/2
	hw := r.W / 2
	p := paint.NewPath()
	if up {
		p.MoveTo(cx, r.Y).LineTo(cx+hw, r.Bottom()).LineTo(cx-hw, r.Bottom()).Close()
	} else {
		p.MoveTo(cx-hw, r.Y).LineTo(cx+hw, r.Y).LineTo(cx, r.Bottom()).Close()
	}
	l.Path(p, col)
}
