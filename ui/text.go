package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"math"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// TextAlign aligns text inside its box. Start and End are logical: they
// resolve per line, so a right-to-left line (Arabic, Hebrew, or any line
// of an RTL scope) starts at the right edge.
type TextAlign uint8

const (
	// AlignStart aligns each line to its start edge: left for LTR lines,
	// right for RTL lines. It is the zero value.
	AlignStart TextAlign = iota
	// AlignCenter centers each line.
	AlignCenter
	// AlignEnd aligns each line to its end edge: right for LTR lines, left
	// for RTL lines.
	AlignEnd
	// AlignPhysicalLeft always aligns to the left edge.
	AlignPhysicalLeft
	// AlignPhysicalRight always aligns to the right edge.
	AlignPhysicalRight
)

// AlignLeft and AlignRight are transitional aliases of AlignStart and
// AlignEnd, kept while existing call sites are audited (RTL spec §3.2):
// in LTR they behave exactly as before. Use AlignStart / AlignEnd for
// "start / end" and AlignPhysicalLeft / AlignPhysicalRight where the
// physical side is required; after the audit AlignLeft / AlignRight will
// become the physical values.
const (
	AlignLeft  = AlignStart
	AlignRight = AlignEnd
)

// Label is a run of text (Typography.Text).
type Label struct {
	c        *Context
	n        *node
	s        string
	st       text.Style
	col      theme.Color
	colSet   bool
	opt      text.Options
	dirSet   bool // opt.Direction set by TextDir (else it follows the scope)
	align    TextAlign
	under    bool
	strike   bool
	code     bool
	mark     bool
	disabled bool
}

func (c *Context) style(size float32, w text.Weight) text.Style {
	return text.Style{Size: size, Weight: w, LineHeight: c.Theme.LineHeightFor(size)}
}

func newLabel(c *Context, kind, s string, size float32, w text.Weight) *Label {
	t := &Label{c: c, s: s, st: c.style(size, w)}
	t.n = c.add(kind, s)
	t.n.ln.Measure = t.measure
	t.n.paint = t.paint
	return t
}

// Text adds a single paragraph of text that wraps to its width.
func Text(c *Context, s string) *Label {
	t := newLabel(c, "text", s, c.Theme.FontSize, text.Regular)
	t.n.setRole(a11y.RoleText)
	return t
}

// Title adds a heading; level 1..5 like Typography.Title.
func Title(c *Context, s string, level int) *Label {
	level = max(1, min(level, 5))
	size := c.Theme.Heading[level-1]
	t := newLabel(c, "title", s, size, text.Weight(c.Theme.FontWeightStrong))
	t.n.setRole(a11y.RoleHeading)
	lv := level
	t.n.fill(semFunc(func(s *a11y.Node) { s.Level = lv }))
	t.st.LineHeight = headingLineHeight(size)
	t.col, t.colSet = c.Theme.TextHeading, true
	return t
}

func headingLineHeight(size float32) float32 {
	switch size {
	case 38:
		return 46
	case 30:
		return 38
	case 24:
		return 32
	case 20:
		return 28
	case 16:
		return 24
	}
	return float32(math.Round(float64(size) * 1.3))
}

func (t *Label) opts(maxW float32) text.Options {
	o := t.opt
	if !t.dirSet {
		o = t.c.textOpts(o) // interface text follows the scope direction
	}
	if !o.NoWrap || o.Ellipsis {
		o.MaxWidth = maxW
	}
	return o
}

func (t *Label) measure(maxW, maxH float32) geom.Size {
	w := maxW
	if t.opt.Ellipsis {
		w = geom.Inf // natural width; truncation happens with the final width
	}
	o := t.opts(w)
	if t.opt.NoWrap {
		o.MaxWidth = 0
	}
	sz := text.Shape(t.s, t.st, o).Size()
	if t.code {
		sz.W += 8
	}
	if sz.W > maxW && t.opt.Ellipsis {
		sz.W = maxW
	}
	return geom.Size{W: float32(math.Ceil(float64(sz.W))), H: sz.H}
}

func (t *Label) color() theme.Color {
	if t.disabled || t.n.disabled {
		return t.c.Theme.TextDisabled
	}
	if t.colSet {
		return t.col
	}
	return t.c.Theme.Text
}

func (t *Label) paint(l *paint.List, r geom.Rect) { t.paintCol(l, r, t.color()) }

// paintCol draws the label in col.
func (t *Label) paintCol(l *paint.List, r geom.Rect, col theme.Color) {
	lay := text.Shape(t.s, t.st, t.opts(r.W+0.5))
	x := r.X
	if t.code || t.mark {
		tk := &t.c.ThemeFor(theme.CompTypography).Components.Typography
		// Code and mark draw one box around the block: align it as a whole
		// (by the direction of its first line).
		rtl := len(lay.Lines) > 0 && lay.Lines[0].RTL
		x += alignLine(t.align, r.W, lay.Width, rtl)
		if t.code {
			cr := geom.Rect{X: x, Y: r.Y + 1, W: lay.Width + 8, H: lay.Height - 2}
			l.Fill(cr, geom.Rad(tk.CodeRadius), tk.CodeBg)
			if tk.CodeBorderColor.A > 0 {
				l.Stroke(cr, geom.Rad(tk.CodeRadius), 1, tk.CodeBorderColor)
			}
			x += 4
		}
		if t.mark {
			l.Fill(geom.Rect{X: x, Y: r.Y, W: lay.Width, H: lay.Height}, geom.Radii{}, tk.MarkBg)
			// The highlight may not follow the theme's lightness (Ant's gold
			// stays light in dark mode): default text takes MarkColor.
			if tk.MarkColor.A > 0 && !t.colSet && !t.disabled && !t.n.disabled {
				col = tk.MarkColor
			}
		}
		l.Glyphs(geom.Pt(x, r.Y), lay.Glyphs, col)
		t.decorate(l, lay, func(int) float32 { return x }, r.Y, col)
		return
	}
	// Each line is aligned on its own (centered / right-aligned paragraphs).
	lineX := func(i int) float32 {
		ln := &lay.Lines[i]
		return x + alignLine(t.align, r.W, ln.Width, ln.RTL)
	}
	paintAlignedGlyphs(l, lay, lineX, r.Y, col)
	t.decorate(l, lay, lineX, r.Y, col)
}

func (t *Label) decorate(l *paint.List, lay *text.Layout, lineX func(int) float32, y float32, col theme.Color) {
	for i, ln := range lay.Lines {
		x := lineX(i)
		if t.under {
			l.Fill(geom.Rect{X: x, Y: y + ln.Baseline + 2, W: ln.Width, H: 1}, geom.Radii{}, col)
		}
		if t.strike {
			l.Fill(geom.Rect{X: x, Y: y + ln.Baseline - lay.Ascent*0.32, W: ln.Width, H: 1}, geom.Radii{}, col)
		}
	}
}

// alignShift is the x offset of a left-to-right line of width lw inside a
// box of width w.
func alignShift(a TextAlign, w, lw float32) float32 { return alignLine(a, w, lw, false) }

// alignLine is the x offset of a line of width lw inside a box of width w;
// rtl is the line's direction (text.Line.RTL), which decides where Start
// and End are. RTL glyphs, like LTR ones, start at x = 0, so a start-aligned
// RTL line is shifted right by the free space.
func alignLine(a TextAlign, w, lw float32, rtl bool) float32 {
	switch a {
	case AlignCenter:
		return (w - lw) / 2
	case AlignEnd:
		if rtl {
			return 0
		}
		return w - lw
	case AlignPhysicalRight:
		return w - lw
	case AlignPhysicalLeft:
		return 0
	}
	if rtl { // AlignStart
		return w - lw
	}
	return 0
}

// paintAlignedGlyphs draws lay with every line starting at lineX(line)
// (y is the layout top). Glyphs are assigned to lines by their baseline.
func paintAlignedGlyphs(l *paint.List, lay *text.Layout, lineX func(int) float32, y float32, col theme.Color) {
	if len(lay.Lines) <= 1 {
		x := float32(0)
		if len(lay.Lines) == 1 {
			x = lineX(0)
		}
		l.Glyphs(geom.Pt(x, y), lay.Glyphs, col)
		return
	}
	gs := lay.Glyphs
	start := 0
	for i := range lay.Lines {
		bottom := lay.Lines[i].Top + lay.LineHeight
		end := start
		if i == len(lay.Lines)-1 {
			end = len(gs)
		} else {
			for end < len(gs) && gs[end].Y < bottom {
				end++
			}
		}
		if end > start {
			l.Glyphs(geom.Pt(lineX(i), y), gs[start:end], col)
		}
		start = end
	}
}

// Size sets the font size (line height follows the theme).
func (t *Label) Size(px float32) *Label {
	t.st.Size = px
	t.st.LineHeight = t.c.Theme.LineHeightFor(px)
	return t
}

// LineHeight overrides the line height in px.
func (t *Label) LineHeight(px float32) *Label { t.st.LineHeight = px; return t }

// Italic slants the text (synthetic oblique).
func (t *Label) Italic() *Label { t.st.Italic = true; return t }

// Weight sets the font weight.
func (t *Label) Weight(w text.Weight) *Label { t.st.Weight = w; return t }

// Strong uses the theme's strong weight.
func (t *Label) Strong() *Label { t.st.Weight = text.Weight(t.c.Theme.FontWeightStrong); return t }

// Color sets the text color.
func (t *Label) Color(col theme.Color) *Label { t.col, t.colSet = col, true; return t }

// Secondary uses the secondary text color.
func (t *Label) Secondary() *Label { return t.Color(t.c.Theme.TextSecondary) }

// Tertiary uses the tertiary text color (descriptions, hints).
func (t *Label) Tertiary() *Label { return t.Color(t.c.Theme.TextTertiary) }

// Type colors the text by status: "success", "warning", "danger"/"error".
func (t *Label) Type(status string) *Label {
	if status == "secondary" {
		return t.Secondary()
	}
	return t.Color(t.c.Theme.ColorOf(status).Base)
}

// Disabled greys the text out.
func (t *Label) Disabled() *Label { t.disabled = true; return t }

// Ellipsis keeps one line and truncates with "…".
func (t *Label) Ellipsis() *Label {
	t.opt.NoWrap, t.opt.Ellipsis = true, true
	t.n.ln.MinW = 0
	t.n.ln.Shrink = 1
	return t
}

// Lines limits wrapping to n lines with an ellipsis.
func (t *Label) Lines(n int) *Label { t.opt.MaxLines, t.opt.Ellipsis = n, true; return t }

// NoWrap keeps the text on one line.
func (t *Label) NoWrap() *Label { t.opt.NoWrap = true; return t }

// Align aligns the text in its box.
func (t *Label) Align(a TextAlign) *Label { t.align = a; return t }

// TextDir sets the paragraph base direction explicitly. By default label
// text follows the scope: RTL in an RTL scope, else taken from the first
// strong character (text.DirAuto).
func (t *Label) TextDir(d text.Direction) *Label { t.opt.Direction, t.dirSet = d, true; return t }

// Underline / Delete / Code / Mark decorations.
func (t *Label) Underline() *Label { t.under = true; return t }
func (t *Label) Delete() *Label    { t.strike = true; return t }
func (t *Label) Code() *Label      { t.code = true; t.st.Mono = true; return t }
func (t *Label) Mark() *Label      { t.mark = true; return t }

// Layout applies layout options to the text node.
func (t *Label) Layout(opts ...layout.Option) *Label { t.n.ln.Style.Apply(opts...); return t }

// ID returns the node ID.
func (t *Label) ID() ID { return t.n.id }

// Paragraph is Text intended for long-form content.
func Paragraph(c *Context, s string) *Label {
	t := Text(c, s)
	t.n.setRole(a11y.RoleParagraph)
	t.n.ln.Margin.Bottom = 14
	return t
}

// LinkW is a clickable text link.
type LinkW struct {
	*Label
	ia     Interaction
	status string      // Type: "" = link colors
	ccol   theme.Color // Color: one color for every state
	ccolOn bool
}

// Link adds a hyperlink-styled text button.
func Link(c *Context, s string) *LinkW {
	t := newLabel(c, "link", s, c.Theme.FontSize, text.Regular)
	t.n.setRole(a11y.RoleLink)
	lw := &LinkW{Label: t}
	t.n.fill(lw)
	lw.ia = c.interact(t.n, true, input.CursorPointer)
	th := c.Theme
	tk := &c.ThemeFor(theme.CompTypography).Components.Typography
	if th.Flavor == theme.StyleShadcn {
		// shadcn typography: font-medium underline underline-offset-4.
		t.under = true
		t.st.Weight = text.Medium
	}
	id := t.n.id
	t.n.paint = func(l *paint.List, r geom.Rect) {
		var col theme.Color
		switch {
		case t.disabled || t.n.disabled:
			col = th.TextDisabled
		case lw.ccolOn:
			col = lw.ccol
		case t.colSet: // Label-level color (Tertiary ...)
			col = t.col
		default:
			base, hover, active := lw.colors(th, tk)
			if lw.ia.Pressed {
				col = active
			} else {
				h := c.Anim(id, "hover", b2f(lw.ia.Hovered), th.DurationMid)
				col = theme.Lerp(base, hover, h)
			}
		}
		t.paintCol(l, r, col)
		if lw.ia.FocusVisible {
			if th.Flavor == theme.StyleShadcn {
				drawFocusRing(l, th, r, geom.Rad(th.RadiusSM))
			} else {
				l.Stroke(r.Expand(2), geom.Rad(4), 2, th.FocusRing)
			}
		}
	}
	return lw
}

// colors returns the idle, hover and pressed colors of the link's Type.
func (lw *LinkW) colors(th *theme.Theme, tk *theme.TypographyToken) (base, hover, active theme.Color) {
	switch lw.status {
	case "":
		return tk.LinkColor, tk.LinkHoverColor, tk.LinkActiveColor
	case "secondary":
		return th.TextSecondary, tk.LinkHoverColor, tk.LinkActiveColor
	}
	p := th.ColorOf(lw.status)
	return p.Base, p.Hover, p.Active
}

// Type colors the link by status: "success", "warning", "danger" /
// "error", "info" use that palette (hover and pressed shades follow);
// "secondary" starts in the secondary text color and turns link-colored
// on hover. "" restores the link colors.
func (lw *LinkW) Type(status string) *LinkW { lw.status = status; return lw }

// Color draws the link in col in every state (disabled still greys out).
func (lw *LinkW) Color(col theme.Color) *LinkW { lw.ccol, lw.ccolOn = col, true; return lw }

// Secondary is Type("secondary").
func (lw *LinkW) Secondary() *LinkW { return lw.Type("secondary") }

// Clicked reports activation.
func (lw *LinkW) Clicked() bool { return lw.ia.Clicked && !lw.disabled }

func (lw *LinkW) semantics(s *a11y.Node) {
	if lw.disabled {
		s.States |= a11y.Disabled
		s.States &^= a11y.Focusable | a11y.Focused
		s.Actions &^= a11y.Press
	}
}

// Hovered reports hover.
func (lw *LinkW) Hovered() bool { return lw.ia.Hovered }

// ---------------------------------------------------------------------------

// IconW is an icon leaf.
type IconW struct {
	c      *Context
	n      *node
	ic     *icon.Icon
	size   float32
	col    theme.Color
	colSet bool
	spin   bool
	rot    float64
}

// Icon adds a vector icon (default 14px, current text color).
func Icon(c *Context, ic *icon.Icon) *IconW {
	w := &IconW{c: c, ic: ic, size: c.Theme.FontSize}
	w.n = c.add("icon", ic.Name)
	w.n.ln.Measure = func(_, _ float32) geom.Size { return geom.Size{W: w.size, H: w.size} }
	w.n.ln.AlignSelf = layout.AlignCenter
	w.n.paint = func(l *paint.List, r geom.Rect) {
		col := c.Theme.Text
		if w.colSet {
			col = w.col
		}
		ang := w.rot
		if w.spin {
			ang += spinAngle(c)
		}
		c.paintIcon(l, w.ic, r, col, ang) // mirrors directional icons in RTL
	}
	return w
}

func spinAngle(c *Context) float64 {
	if c.still() {
		return 0 // deterministic frame for screenshots/tests (or reduced motion); no frame requests
	}
	c.animating = true
	ms := c.now.UnixMilli() % 1000
	return float64(ms) / 1000 * 2 * math.Pi
}

// Size sets the icon size in px.
func (w *IconW) Size(px float32) *IconW { w.size = px; return w }

// A11yLabel exposes a meaningful icon (a status, not decoration) as an
// image with this accessible name. Icons are decorative by default.
func (w *IconW) A11yLabel(s string) *IconW {
	w.n.setRole(a11y.RoleImage)
	w.n.setName(s)
	return w
}

// Color sets the icon color.
func (w *IconW) Color(col theme.Color) *IconW { w.col, w.colSet = col, true; return w }

// Spin rotates the icon continuously (loading).
func (w *IconW) Spin() *IconW { w.spin = true; return w }

// Rotate rotates the icon by degrees.
func (w *IconW) Rotate(deg float64) *IconW { w.rot = deg * math.Pi / 180; return w }

// drawIcon paints an icon centered in a square of size s at (x,y). It
// never mirrors; paint code migrating to RTL calls c.drawIconAt instead.
func drawIcon(l *paint.List, ic *icon.Icon, x, y, s float32, col theme.Color) {
	icon.Draw(l, ic, geom.Rect{X: x, Y: y, W: s, H: s}, col, 0)
}

// drawIconAt is drawIcon that mirrors directional icons when the node being
// painted lays out right to left (identical to drawIcon in LTR).
func (c *Context) drawIconAt(l *paint.List, ic *icon.Icon, x, y, s float32, col theme.Color) {
	c.paintIcon(l, ic, geom.Rect{X: x, Y: y, W: s, H: s}, col, 0)
}

// drawStatusIcon draws a filled status circle with a white glyph.
func drawStatusIcon(l *paint.List, status string, r geom.Rect, col, fg theme.Color) {
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	rad := r.W / 2
	l.Path(paint.NewPath().Circle(cx, cy, rad), col)
	s := r.W / 24
	w := paint.StrokeStyle{Width: 2 * s, Cap: paint.CapRound}
	p := paint.NewPath()
	switch status {
	case "success":
		p.MoveTo(cx-4.6*s, cy+0.2*s).LineTo(cx-1.2*s, cy+3.6*s).LineTo(cx+5*s, cy-3.2*s)
		l.StrokePath(p, w, fg)
	case "error":
		p.MoveTo(cx-3.8*s, cy-3.8*s).LineTo(cx+3.8*s, cy+3.8*s).MoveTo(cx+3.8*s, cy-3.8*s).LineTo(cx-3.8*s, cy+3.8*s)
		l.StrokePath(p, w, fg)
	case "warning":
		p.MoveTo(cx, cy-5.5*s).LineTo(cx, cy+1.2*s)
		l.StrokePath(p, w, fg)
		l.Path(paint.NewPath().Circle(cx, cy+5*s, 1.3*s), fg)
	default: // info
		p.MoveTo(cx, cy-1.2*s).LineTo(cx, cy+5.5*s)
		l.StrokePath(p, w, fg)
		l.Path(paint.NewPath().Circle(cx, cy-5*s, 1.3*s), fg)
	}
}

// statusIcon is a leaf drawing drawStatusIcon.
func statusIcon(c *Context, status string, size float32) *node {
	n := c.add("status-icon", status)
	n.ln.Width, n.ln.Height = size, size
	th := c.Theme
	n.paint = func(l *paint.List, r geom.Rect) {
		drawStatusIcon(l, status, r, th.ColorOf(status).Base, th.BgContainer)
	}
	return n
}
