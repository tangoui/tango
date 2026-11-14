package ui

import (
	"fmt"
	"math"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ProgressW is the full progress indicator: line / circle / dashboard
// types, steps, gradients, success segment, inside text, stripes and
// indeterminate motion.
type ProgressW struct {
	_ [0]func()

	c          *Context
	n          *node
	percent    float64
	success    float64
	kind       string // line | circle | dashboard
	status     string // "" | active | success | exception | warning
	steps      int
	size       float32 // circle diameter; line width when > 0
	stroke     float32
	small      bool
	showInfo   bool
	infoSet    bool // ShowInfo / HideInfo / ConfigProvider "showInfo" chose it
	inside     bool
	striped    bool
	flow       bool
	indeterm   bool
	gapDeg     float64
	gapSet     bool   // GapDegree called (a circle gets the gap too)
	gapPlace   string // "" (bottom) / top / bottom / start / end
	infoAlign  string // PercentPosition align: start / center / end (default)
	infoInner  bool   // PercentPosition type inner
	stepsGap   float32
	colFn      func(percent float64) theme.Color
	col, trail theme.Color
	grad       [2]theme.Color
	hasGrad    bool
	format     func(float64) string
	round      bool
	a11yName   string // default accessible name (Locale), captured at build
	th         *theme.Theme
	tk         *theme.ProgressToken
}

// Progress adds a progress indicator (percent 0..100): a line bar by
// default, Circle/Dashboard for rings.
func Progress(c *Context, percent float64) *ProgressW {
	p := &ProgressW{c: c, percent: clampPct(percent), kind: "line", showInfo: true, gapDeg: 75, round: true, th: c.ThemeFor(theme.CompProgress)}
	p.tk = &p.th.Components.Progress
	// ConfigProvider.Defaults("Progress", ...) keys: "size" ("small" /
	// Small), "type" ("line" | "circle" | "dashboard"), "showInfo" (bool),
	// "strokeLinecap" ("round" | "butt" | "square").
	if s, ok := dataPropSize(c, "Progress"); ok {
		p.small = s == Small
	}
	p.kind = Prop(c, "Progress", "type", p.kind)
	if v, ok := c.ConfigProp("Progress", "showInfo"); ok {
		if b, ok := v.(bool); ok {
			p.showInfo, p.infoSet = b, true
		}
	}
	p.round = Prop(c, "Progress", "strokeLinecap", "round") == "round"
	p.n = c.add("progress", "")
	p.n.ln.Measure = p.measure
	p.n.paint = p.paint
	p.a11yName = c.Locale().A11y.Progress
	p.n.setRole(a11y.RoleProgressBar).fill(p)
	return p
}

// A11yLabel sets the accessible name (default: the Locale's "progress").
func (p *ProgressW) A11yLabel(s string) *ProgressW { p.n.setName(s); return p }

func (p *ProgressW) semantics(s *a11y.Node) {
	if s.Name == "" {
		s.Name = p.a11yName
	}
	if p.indeterm {
		s.States |= a11y.Busy
		return
	}
	s.Value = p.label()
	rangeSem(s, p.percent, 0, 100, 0)
	if p.effStatus() == "active" {
		s.States |= a11y.Busy
	}
}

// clampPct clamps a percentage to 0..100; NaN counts as 0 (antd
// validProgress).
func clampPct(v float64) float64 {
	if math.IsNaN(v) {
		return 0
	}
	return math.Max(0, math.Min(100, v))
}

// Type selects "line", "circle" or "dashboard".
func (p *ProgressW) Type(t string) *ProgressW { p.kind = t; return p }

// Circle is Type("circle") with a diameter.
func (p *ProgressW) Circle(size float32) *ProgressW { p.kind, p.size = "circle", size; return p }

// Dashboard is Type("dashboard") with a diameter.
func (p *ProgressW) Dashboard(size float32) *ProgressW { p.kind, p.size = "dashboard", size; return p }

// GapDegree sets the ring opening in degrees (dashboard default 75°; a
// circle with GapDegree opens too, like antd).
func (p *ProgressW) GapDegree(d float64) *ProgressW { p.gapDeg, p.gapSet = d, true; return p }

// GapPlacement sets where the ring opening sits: "bottom" (default),
// "top", "start" or "end" (logical: mirrored in RTL). antd 6 gapPlacement
// (gapPosition is deprecated there).
func (p *ProgressW) GapPlacement(s string) *ProgressW { p.gapPlace = s; return p }

// PercentPosition places the info text of a line bar (antd 6
// percentPosition): align "start", "center" or "end" (default); typ
// "outer" (default; center puts it under the bar) or "inner" (inside the
// filled part, like Element text-inside).
func (p *ProgressW) PercentPosition(align, typ string) *ProgressW {
	p.infoAlign, p.infoInner = align, typ == "inner"
	if p.infoInner {
		p.inside = true
	}
	return p
}

// RailColor sets the color of the unfilled rail (antd railColor).
func (p *ProgressW) RailColor(col theme.Color) *ProgressW { p.trail = col; return p }

// ColorFunc picks the bar color from the percentage (Element color
// function; a color array is a ColorFunc with thresholds).
func (p *ProgressW) ColorFunc(fn func(percent float64) theme.Color) *ProgressW {
	p.colFn = fn
	return p
}

// StepsGap sets the gap between step segments in px (antd steps.gap;
// default 2).
func (p *ProgressW) StepsGap(px float32) *ProgressW { p.stepsGap = px; return p }

// Status: "active" (Ant shimmer), "success", "exception", "warning".
func (p *ProgressW) Status(s string) *ProgressW { p.status = s; return p }

// Steps splits the bar (or ring) into n segments.
func (p *ProgressW) Steps(n int) *ProgressW { p.steps = n; p.n.ln.NoStretch = true; return p }

// Width sets the line length (or circle diameter).
func (p *ProgressW) Width(px float32) *ProgressW { p.size = px; return p }

// StrokeWidth sets the bar thickness / ring width in px.
func (p *ProgressW) StrokeWidth(w float32) *ProgressW { p.stroke = w; return p }

// Small uses the compact size.
func (p *ProgressW) Small() *ProgressW { p.small = true; return p }

// HideInfo hides the percentage text / status icon.
func (p *ProgressW) HideInfo() *ProgressW { p.showInfo, p.infoSet = false, true; return p }

// ShowInfo shows the percentage text / status icon. It is the default,
// except for a shadcn line bar, which (like shadcn/ui's Progress) shows
// only the bar unless asked.
func (p *ProgressW) ShowInfo() *ProgressW { p.showInfo, p.infoSet = true, true; return p }

// lineInfo reports whether a line bar shows its text (see ShowInfo).
func (p *ProgressW) lineInfo() bool {
	if p.infoSet || p.inside {
		return p.showInfo
	}
	return p.showInfo && p.th.Flavor != theme.StyleShadcn
}

// TextInside draws the percentage inside the bar (Element text-inside).
func (p *ProgressW) TextInside() *ProgressW { p.inside = true; return p }

// Striped draws diagonal stripes; flow animates them.
func (p *ProgressW) Striped(flow bool) *ProgressW { p.striped, p.flow = true, flow; return p }

// Indeterminate shows an endlessly moving segment (Element).
func (p *ProgressW) Indeterminate() *ProgressW { p.indeterm = true; return p }

// Color overrides the bar color.
func (p *ProgressW) Color(col theme.Color) *ProgressW { p.col = col; return p }

// Gradient paints the bar from a to b.
func (p *ProgressW) Gradient(a, b theme.Color) *ProgressW {
	p.grad, p.hasGrad = [2]theme.Color{a, b}, true
	return p
}

// TrailColor sets the rail color.
//
// Deprecated: use RailColor (antd 6 renamed trailColor to railColor).
func (p *ProgressW) TrailColor(col theme.Color) *ProgressW { p.trail = col; return p }

// Success shows the first pct percent in the success color (Ant success).
func (p *ProgressW) Success(pct float64) *ProgressW { p.success = clampPct(pct); return p }

// Format customises the info text.
func (p *ProgressW) Format(f func(percent float64) string) *ProgressW { p.format = f; return p }

// Square uses butt line caps instead of round ones.
func (p *ProgressW) Square() *ProgressW { p.round = false; return p }

// Layout applies layout options.
func (p *ProgressW) Layout(o ...layout.Option) *ProgressW {
	flexbox.Apply(&p.n.ln.Style, o...)
	return p
}

func (p *ProgressW) el() bool { return p.th.Flavor == theme.Element }

func (p *ProgressW) effStatus() string {
	if p.status == "" && p.percent >= 100 {
		return "success"
	}
	return p.status
}

func (p *ProgressW) barColor() theme.Color {
	tk := p.tk
	switch p.effStatus() {
	case "success":
		return tk.SuccessColor
	case "exception", "error":
		return tk.ExceptionColor
	case "warning":
		return tk.WarningColor
	}
	if p.colFn != nil {
		return p.colFn(p.percent)
	}
	if p.col.A > 0 {
		return p.col
	}
	return tk.DefaultColor
}

func (p *ProgressW) trailColor() theme.Color {
	if p.trail.A > 0 {
		return p.trail
	}
	if p.tk.RemainingColor.A == 0 {
		return theme.Over(theme.Fade(p.barColor(), .2), p.th.BgContainer) // bg-primary/20
	}
	return p.tk.RemainingColor
}

func (p *ProgressW) label() string {
	if p.format != nil {
		return p.format(p.percent)
	}
	return fmt.Sprintf("%.0f%%", p.percent)
}

func (p *ProgressW) font() text.Style {
	th := p.th
	fs := th.FontSize
	if p.small {
		fs = th.FontSizeSM
	}
	return text.Style{Size: fs, Weight: text.Regular, LineHeight: th.LineHeightFor(fs)}
}

func (p *ProgressW) lineStroke() float32 {
	if p.stroke > 0 {
		return p.stroke
	}
	switch tk := p.tk; {
	case p.inside:
		return tk.InnerLineHeight
	case p.steps > 0:
		return tk.StepsHeight
	case p.small:
		return tk.LineHeightSM
	}
	return p.tk.LineHeight
}

func (p *ProgressW) infoWidth() float32 {
	if !p.lineInfo() || p.inside || p.indeterm || p.infoBelow() {
		return 0
	}
	return 8 + max(text.Width(p.label(), p.font()), text.Width("100%", p.font()))
}

// infoBelow reports PercentPosition("center", "outer"): the text sits
// under the bar (antd body-layout-bottom, gap marginXXS 4).
func (p *ProgressW) infoBelow() bool {
	return p.infoAlign == "center" && !p.infoInner && p.lineInfo() && !p.inside && !p.indeterm
}

func (p *ProgressW) gap() float32 {
	if p.stepsGap > 0 {
		return p.stepsGap
	}
	return p.tk.StepsGap
}

func (p *ProgressW) diameter() float32 {
	if p.size > 0 {
		return p.size
	}
	if p.small {
		return p.tk.CircleSizeSM
	}
	return p.tk.CircleSize
}

func (p *ProgressW) measure(maxW, _ float32) geom.Size {
	if p.kind != "line" {
		d := p.diameter()
		return geom.Size{W: d, H: d}
	}
	h := max(p.lineStroke(), p.font().LineHeight)
	if p.infoBelow() {
		h = p.lineStroke() + 4 + p.font().LineHeight
	}
	if p.steps > 0 {
		sw, gap := p.tk.StepsWidth, p.gap()
		if p.small {
			sw = p.tk.StepsWidthSM
		}
		if p.size > 0 {
			sw = p.size
		}
		return geom.Size{W: float32(p.steps)*(sw+gap) - gap + p.infoWidth(), H: h}
	}
	w := maxW
	if p.size > 0 {
		w = p.size + p.infoWidth()
	}
	if geom.IsInf(w) {
		w = 300
	}
	return geom.Size{W: w, H: h}
}

func (p *ProgressW) fillBar(l *paint.List, r geom.Rect, rad geom.Radii, col theme.Color) {
	if r.W <= 0 {
		return
	}
	if p.hasGrad {
		// The gradient runs from the inline start.
		p0, p1 := geom.Pt(r.X, 0), geom.Pt(r.Right(), 0)
		if p.c.IsRTL() {
			p0, p1 = p1, p0
		}
		l.LinearStops(r, rad, p0, p1, paint.Stop{Offset: 0, Color: p.grad[0]}, paint.Stop{Offset: 1, Color: p.grad[1]})
		return
	}
	l.Fill(r, rad, col)
}

func (p *ProgressW) paint(l *paint.List, r geom.Rect) {
	if p.kind == "line" {
		p.paintLine(l, r)
		return
	}
	p.paintCircle(l, r)
}

// paintInfo draws the percentage or status icon starting at x (in RTL x is
// its right edge: the info sits at the left of the bar).
func (p *ProgressW) paintInfo(l *paint.List, x float32, r geom.Rect, col theme.Color) {
	c, th := p.c, p.th
	rtl := c.IsRTL()
	st := p.effStatus()
	fs := p.font()
	if p.format == nil {
		var s string
		switch st {
		case "success":
			s = "success"
		case "exception", "error":
			s = "error"
		case "warning":
			s = "warning"
		}
		if s != "" {
			sz := fs.Size
			if rtl {
				x -= sz
			}
			ir := geom.Rect{X: x, Y: r.Y + (r.H-sz)/2, W: sz, H: sz}
			if p.el() {
				// Element: outlined circle icon in the status color.
				cx, cy := ir.X+sz/2, ir.Y+sz/2
				l.StrokePath(paint.NewPath().Circle(cx, cy, sz/2-0.7), paint.StrokeStyle{Width: 1.3}, col)
				k := sz / 16
				g := paint.NewPath()
				switch s {
				case "success":
					g.MoveTo(cx-3.6*k, cy+0.2*k).LineTo(cx-1*k, cy+2.8*k).LineTo(cx+3.8*k, cy-2.4*k)
				case "error":
					g.MoveTo(cx-2.8*k, cy-2.8*k).LineTo(cx+2.8*k, cy+2.8*k).MoveTo(cx+2.8*k, cy-2.8*k).LineTo(cx-2.8*k, cy+2.8*k)
				default:
					g.MoveTo(cx, cy-3.8*k).LineTo(cx, cy+1*k).MoveTo(cx, cy+3.6*k).LineTo(cx, cy+3.7*k)
				}
				l.StrokePath(g, paint.StrokeStyle{Width: 1.4, Cap: paint.CapRound}, col)
			} else {
				drawStatusIcon(l, s, ir, col, th.BgContainer)
			}
			return
		}
	}
	lay := text.Shape(p.label(), fs, text.Options{NoWrap: true})
	tc := p.tk.TextColor
	if rtl {
		x -= lay.Width
	}
	l.Glyphs(geom.Pt(x, r.Y+(r.H-lay.Height)/2), lay.Glyphs, tc)
}

func (p *ProgressW) phase(period time.Duration) float32 {
	c := p.c
	if c.still() {
		return 0.5
	}
	c.animating = true
	return float32(c.now.UnixMilli()%period.Milliseconds()) / float32(period.Milliseconds())
}

func (p *ProgressW) paintLine(l *paint.List, r geom.Rect) {
	tk := p.tk
	// Geometry is computed left to right and mirrored inside r in RTL, so
	// the bar fills from the inline start and the info sits at the end.
	rtl := p.c.IsRTL()
	full := r // mirror inside the whole rect even when the bar is narrowed
	mr := func(x geom.Rect) geom.Rect {
		if rtl {
			return mirrorRect(x, full)
		}
		return x
	}
	mx := func(x float32) float32 {
		if rtl {
			return 2*full.X + full.W - x
		}
		return x
	}
	h := p.lineStroke()
	col := p.barColor()
	info := p.infoWidth()
	y := r.Y + (r.H-h)/2
	below := p.infoBelow()
	if below {
		y = r.Y
	}
	// PercentPosition("start", "outer") puts the text before the bar.
	if p.infoAlign == "start" && info > 0 {
		p.paintInfo(l, mx(r.X), r, col)
		r.X += info
		r.W -= info
		info = 0
	}
	if below {
		defer func() {
			lay := text.Shape(p.label(), p.font(), text.Options{NoWrap: true})
			rr := geom.Rect{X: r.X, Y: y + h + 4, W: r.W, H: p.font().LineHeight}
			p.paintInfo(l, mx(rr.X+(rr.W-lay.Width)/2), rr, col)
		}()
	}
	rad := geom.Rad(h / 2)
	if !p.round {
		rad = geom.Radii{}
	}
	if p.steps > 0 {
		gap := p.gap()
		sw := (r.W - info + gap) / float32(p.steps)
		done := int(math.Round(p.percent / 100 * float64(p.steps)))
		for i := 0; i < p.steps; i++ {
			sr := geom.Rect{X: r.X + float32(i)*sw, Y: y, W: sw - gap, H: h}
			c := p.trailColor()
			if i < done {
				c = col
			}
			l.Fill(mr(sr), geom.Radii{}, c)
		}
		if info > 0 {
			p.paintInfo(l, mx(r.Right()-info+8), r, col)
		}
		return
	}
	bar := geom.Rect{X: r.X, Y: y, W: r.W - info, H: h}
	l.Fill(mr(bar), rad, p.trailColor())
	if p.indeterm {
		t := p.phase(3 * time.Second)
		segW := bar.W * 0.3
		x := bar.X - segW + (bar.W+segW)*t
		l.Clip(mr(bar), rad)
		p.fillBar(l, mr(geom.Rect{X: x, Y: y, W: segW, H: h}), rad, col)
		l.Pop()
		return
	}
	fw := bar.W * float32(p.percent/100)
	fill := geom.Rect{X: bar.X, Y: y, W: fw, H: h}
	p.fillBar(l, mr(fill), rad, col)
	if p.success > 0 {
		l.Fill(mr(geom.Rect{X: bar.X, Y: y, W: bar.W * float32(p.success/100), H: h}), rad, tk.SuccessColor)
	}
	if p.striped && fw > 0 {
		l.Clip(mr(fill), rad)
		off := float32(0)
		if p.flow {
			off = p.phase(time.Second) * 2 * h
		}
		w := h
		for x := bar.X - 2*h + off; x < fill.Right()+h; x += 2 * w {
			s := paint.NewPath().MoveTo(mx(x), y+h).LineTo(mx(x+w), y+h).LineTo(mx(x+w+h), y).LineTo(mx(x+h), y).Close()
			l.Path(s, theme.RGBA(255, 255, 255, .15))
		}
		l.Pop()
	}
	if p.status == "active" && fw > 0 && !p.el() {
		t := p.phase(2400 * time.Millisecond)
		hw := fw * float32(EaseOut(float64(t)))
		a := 0.5 * (1 - t)
		l.Clip(mr(fill), rad)
		l.Add(paint.LinearGradient{Rect: mr(geom.Rect{X: bar.X, Y: y, W: hw, H: h}), Radii: rad,
			P0: geom.Pt(mx(bar.X), 0), P1: geom.Pt(mx(bar.X+hw), 0),
			From: theme.Fade(theme.Hex("#ffffff"), 0), To: theme.Fade(theme.Hex("#ffffff"), float64(a))})
		l.Pop()
	}
	if p.inside && p.showInfo && fw > 0 {
		fs := text.Style{Size: tk.InnerFontSize, Weight: text.Regular, LineHeight: p.th.LineHeightFor(tk.InnerFontSize)}
		lay := text.Shape(p.label(), fs, text.Options{NoWrap: true})
		if lay.Width+10 < fw {
			// PercentPosition inner align: end (default), start or center
			// inside the filled part (antd paddingInline paddingXXS 4).
			x := fill.Right() - lay.Width - 10
			switch p.infoAlign {
			case "start":
				x = fill.X + 4
			case "center":
				x = fill.X + (fw-lay.Width)/2
			}
			lr := mr(geom.Rect{X: x, W: lay.Width})
			l.Glyphs(geom.Pt(lr.X, y+(h-lay.Height)/2), lay.Glyphs, p.th.TextOn(col, tk.InnerTextColor))
		}
	}
	if info > 0 {
		p.paintInfo(l, mx(bar.Right()+8), r, col)
	}
}

func (p *ProgressW) paintCircle(l *paint.List, r geom.Rect) {
	tk := p.tk
	d := min(r.W, r.H)
	cx, cy := r.X+r.W/2, r.Y+r.H/2
	sw := p.stroke
	if sw == 0 {
		sw = tk.CircleStrokeWidth
		if sw == 0 {
			sw = d * 6 / 100
		}
	}
	rad := d/2 - sw/2
	start, total := p.ringArc()
	capS := paint.CapRound
	if !p.round {
		capS = paint.CapButt
	}
	col := p.barColor()
	p.paintRing(l, cx, cy, rad, sw, start, total, capS, col, d)
}

// ringArc returns the start angle and sweep (radians, clockwise from the
// x axis) of a circle or dashboard ring.
func (p *ProgressW) ringArc() (start, total float64) {
	start, total = -math.Pi/2, 2*math.Pi
	if p.kind == "dashboard" || (p.gapSet && p.gapDeg > 0) {
		// The opening is centred on the gap placement (bottom by default;
		// start / end follow the direction, antd Circle.js).
		g := p.gapDeg * math.Pi / 180
		mid := math.Pi / 2
		rtl := p.c.IsRTL()
		switch p.gapPlace {
		case "top":
			mid = -math.Pi / 2
		case "start", "left":
			mid = math.Pi
			if rtl && p.gapPlace == "start" {
				mid = 0
			}
		case "end", "right":
			mid = 0
			if rtl && p.gapPlace == "end" {
				mid = math.Pi
			}
		}
		start, total = mid+g/2, 2*math.Pi-g
	}
	return
}

func (p *ProgressW) paintRing(l *paint.List, cx, cy, rad, sw float32, start, total float64, capS paint.Cap, col theme.Color, d float32) {
	tk := p.tk
	if p.steps > 0 {
		gap := 2 * math.Pi / 180 * 2
		seg := (total - gap*float64(p.steps)) / float64(p.steps)
		if p.kind == "circle" {
			seg = (total - gap*float64(p.steps)) / float64(p.steps)
		}
		done := int(math.Round(p.percent / 100 * float64(p.steps)))
		a := start + gap/2
		for i := 0; i < p.steps; i++ {
			c := p.trailColor()
			if i < done {
				c = col
			}
			l.StrokePath(paint.NewPath().Arc(cx, cy, rad, a, seg), paint.StrokeStyle{Width: sw}, c)
			a += seg + gap
		}
	} else {
		l.StrokePath(paint.NewPath().Arc(cx, cy, rad, start, total), paint.StrokeStyle{Width: sw, Cap: capS}, p.trailColor())
		if p.percent > 0 {
			arc := paint.NewPath().Arc(cx, cy, rad, start, total*p.percent/100)
			if p.hasGrad {
				// Conic gradient along the arc. Its seam is moved back by the
				// cap's angular size so a round start cap keeps the first color.
				capA := 0.0
				if capS != paint.CapButt && rad > 0 {
					capA = float64(sw/2) / float64(rad)
				}
				a0 := start - capA
				sweep := total * p.percent / 100
				l.Gradient(paint.Gradient{
					Kind:  paint.GradientConic,
					Path:  paint.StrokeToFill(arc, paint.StrokeStyle{Width: sw, Cap: capS}),
					Start: geom.Pt(cx, cy),
					End:   geom.Pt(cx+float32(math.Cos(a0)), cy+float32(math.Sin(a0))),
					Stops: []paint.Stop{
						{Offset: float32(capA / (2 * math.Pi)), Color: p.grad[0]},
						{Offset: float32((capA + sweep) / (2 * math.Pi)), Color: p.grad[1]},
					},
				})
			} else {
				l.StrokePath(arc, paint.StrokeStyle{Width: sw, Cap: capS}, col)
			}
		}
		if p.success > 0 {
			l.StrokePath(paint.NewPath().Arc(cx, cy, rad, start, total*p.success/100), paint.StrokeStyle{Width: sw, Cap: capS}, tk.SuccessColor)
		}
	}
	if !p.showInfo {
		return
	}
	fs := d*0.15 + 6
	if p.el() {
		fs = d*0.111111 + 2
	}
	st := p.effStatus()
	if p.format == nil && (st == "success" || st == "exception" || st == "error" || st == "warning") {
		is := fs * 1.25
		k := is / 24
		g := paint.NewPath()
		switch st {
		case "success":
			g.MoveTo(cx-8*k, cy+0.5*k).LineTo(cx-2.4*k, cy+6*k).LineTo(cx+8.5*k, cy-6*k)
		case "warning":
			g.MoveTo(cx, cy-8*k).LineTo(cx, cy+3*k).MoveTo(cx, cy+8*k).LineTo(cx, cy+8.2*k)
		default:
			g.MoveTo(cx-6.5*k, cy-6.5*k).LineTo(cx+6.5*k, cy+6.5*k).MoveTo(cx+6.5*k, cy-6.5*k).LineTo(cx-6.5*k, cy+6.5*k)
		}
		l.StrokePath(g, paint.StrokeStyle{Width: max(1.6, 2.2*k), Cap: paint.CapRound}, col)
		return
	}
	tc := tk.TextColor
	lay := text.Shape(p.label(), text.Style{Size: fs, LineHeight: fs * 1.2}, text.Options{NoWrap: true})
	l.Glyphs(geom.Pt(cx-lay.Width/2, cy-lay.Height/2), lay.Glyphs, tc)
}
