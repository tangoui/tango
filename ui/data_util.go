package ui

// Shared helpers of the data-display components (data_*.go). Kept small and
// private; candidates for promotion into the base layer are listed in the
// package report.

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// dataShape shapes a single line of text with the theme line height.
func (c *Context) dataShape(s string, size float32, w text.Weight) *text.Layout {
	return text.Shape(s, c.style(size, w), text.Options{NoWrap: true})
}

// dataText draws a single line of text with its top-left at (x, y).
func dataText(l *paint.List, lay *text.Layout, x, y float32, col theme.Color) {
	l.Glyphs(geom.Pt(x, y), lay.Glyphs, col)
}

// dataHLine / dataVLine draw 1px hairlines.
func dataHLine(l *paint.List, x, y, w float32, col theme.Color) {
	l.Fill(geom.Rect{X: x, Y: y, W: w, H: 1}, geom.Radii{}, col)
}

func dataVLine(l *paint.List, x, y, h float32, col theme.Color) {
	l.Fill(geom.Rect{X: x, Y: y, W: 1, H: h}, geom.Radii{}, col)
}

// dataStartX is the x of a 1px vertical divider just inside the inline
// start of r (its left edge; its right edge in RTL).
func dataStartX(c *Context, r geom.Rect) float32 {
	if c.IsRTL() {
		return r.Right() - 1
	}
	return r.X
}

// dataEndX is the x of a 1px vertical divider just inside the inline end
// of r (its right edge; its left edge in RTL).
func dataEndX(c *Context, r geom.Rect) float32 {
	if c.IsRTL() {
		return r.X
	}
	return r.Right() - 1
}

// dataCenterText adds wrapped text whose every line is centered (Label's
// AlignCenter centers the block, not each line).
func dataCenterText(c *Context, s string, size float32, col theme.Color, opts ...layout.Option) *node {
	st := c.style(size, text.Regular)
	n := c.add("text", s)
	n.ln.Style.Apply(opts...)
	n.ln.Measure = func(maxW, _ float32) geom.Size {
		return text.Shape(s, st, c.textOpts(text.Options{MaxWidth: maxW})).Size()
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		lay := text.Shape(s, st, c.textOpts(text.Options{MaxWidth: r.W + 0.5}))
		for _, ln := range lay.Lines {
			var gs []paint.Glyph
			for _, g := range lay.Glyphs {
				if g.Y > ln.Top && g.Y <= ln.Top+lay.LineHeight {
					gs = append(gs, g)
				}
			}
			if len(gs) == 0 {
				continue
			}
			x0 := gs[0].X
			for _, g := range gs {
				x0 = min(x0, g.X) // the leftmost glyph (RTL lines run backwards)
			}
			for i := range gs {
				gs[i].X -= x0
			}
			l.Glyphs(geom.Pt(r.X+(r.W-ln.Width)/2, r.Y), gs, col)
		}
	}
	return n
}

// dataLazy builds n's content with fn once all chained modifiers have run
// (at the first query or before the build ends). The Disabled depth of the
// call site is kept, so content built later is disabled like its siblings.
func dataLazy(c *Context, n *node, fn func()) *lazy {
	dd := c.disabledDepth
	return c.lazy(n, func() {
		prev := c.disabledDepth
		c.disabledDepth = dd
		fn()
		c.disabledDepth = prev
	})
}

// dataFirstSem returns the first node of n's subtree (n excluded, preorder)
// that the semantics tree exposes; with inter only interactive ones.
func dataFirstSem(n *node, inter bool) *node {
	for _, k := range n.kids {
		if k.aHidden {
			continue
		}
		r := k.role
		if r == a11y.RoleNone && interactiveUnannotated(k) {
			r = a11y.RoleButton
		}
		if r != a11y.RoleNone && (!inter || r.Interactive() || r == a11y.RoleCombobox) {
			return k
		}
		if f := dataFirstSem(k, inter); f != nil {
			return f
		}
	}
	return nil
}

// dataDescribe appends desc() to the accessible description of the first
// semantic node inside n, preferring an interactive one (the count of a
// Badge on the button or avatar it wraps, a Tooltip's text on its
// trigger). It reports false when n holds no semantic node. Nothing is
// done when accessibility is off.
func dataDescribe(c *Context, n *node, desc func() string) bool {
	if !c.a11yOn {
		return true
	}
	t := dataFirstSem(n, true)
	if t == nil {
		t = dataFirstSem(n, false)
	}
	if t == nil {
		return false
	}
	t.fill(semFunc(func(s *a11y.Node) { addDescription(s, desc()) }))
	return true
}

// addDescription appends d to the node's description.
func addDescription(s *a11y.Node, d string) {
	switch {
	case d == "":
	case s.Description == "":
		s.Description = d
	default:
		s.Description += "; " + d
	}
}

// dataRespCols holds per-breakpoint column counts (0: unset), like Ant's
// column={{ xs: 1, sm: 2, md: 3 }}.
type dataRespCols [6]int

func (r *dataRespCols) set(bp Breakpoint, n int) {
	if int(bp) < len(r) {
		r[bp] = max(1, n)
	}
}

// setMap copies a map of breakpoint counts.
func (r *dataRespCols) setMap(m map[Breakpoint]int) {
	for bp, n := range m {
		r.set(bp, n)
	}
}

// resolve returns the count of the largest set breakpoint not above the
// window width, else fallback.
func (r *dataRespCols) resolve(c *Context, fallback int) int {
	for b := int(ScreenBreakpoint(c)); b >= 0; b-- {
		if r[b] > 0 {
			return r[b]
		}
	}
	return fallback
}

// dataPropCols reads an int or map[Breakpoint]int "column" style default.
func dataPropCols(c *Context, comp, key string, n *int, resp *dataRespCols) {
	v, ok := c.ConfigProp(comp, key)
	if !ok {
		return
	}
	switch x := v.(type) {
	case map[Breakpoint]int:
		resp.setMap(x)
	default:
		if f, ok := p2bNum(c, comp, key); ok && f >= 1 {
			*n = int(f)
		}
	}
}

// dataPropSize reads a "size" default: a ControlSize or "small" /
// "middle" / "default" / "large".
func dataPropSize(c *Context, comp string) (ControlSize, bool) {
	v, ok := c.ConfigProp(comp, "size")
	if !ok {
		return Middle, false
	}
	switch x := v.(type) {
	case ControlSize:
		return x, true
	case string:
		switch x {
		case "small":
			return Small, true
		case "middle", "default":
			return Middle, true
		case "large":
			return Large, true
		}
	}
	return Middle, false
}
