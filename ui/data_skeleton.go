package ui

import (
	"math"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/internal/flexbox"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Skeleton

// SkeletonW is a loading placeholder: optional avatar, a title bar and
// paragraph lines, with an optional shimmer ("active").
type SkeletonW struct {
	_ [0]func()

	c       *Context
	active  bool
	avatar  bool
	square  bool
	title   bool
	rows    int
	round   bool
	loading bool
}

// Skeleton starts a skeleton with a title and three paragraph rows.
//
// ConfigProvider.Defaults("Skeleton", ...) keys: "active", "round" (bool).
func Skeleton(c *Context) *SkeletonW {
	return &SkeletonW{c: c, title: true, rows: 3, loading: true,
		active: Prop(c, "Skeleton", "active", false), round: Prop(c, "Skeleton", "round", false)}
}

// Active turns on the shimmer animation.
func (s *SkeletonW) Active() *SkeletonW { s.active = true; return s }

// Avatar adds a round avatar placeholder (square=true for square).
func (s *SkeletonW) Avatar(square bool) *SkeletonW { s.avatar, s.square = true, square; return s }

// Rows sets the paragraph rows (0 hides the paragraph).
func (s *SkeletonW) Rows(n int) *SkeletonW { s.rows = n; return s }

// NoTitle hides the title bar.
func (s *SkeletonW) NoTitle() *SkeletonW { s.title = false; return s }

// Round uses pill-shaped bars.
func (s *SkeletonW) Round() *SkeletonW { s.round = true; return s }

// Loading switches between the skeleton (true) and the real content.
func (s *SkeletonW) Loading(on bool) *SkeletonW { s.loading = on; return s }

// Show builds the skeleton.
func (s *SkeletonW) Show() *Box { return s.Body(nil) }

// Body builds the skeleton while loading, else fn.
func (s *SkeletonW) Body(fn func()) *Box {
	c := s.c
	if !s.loading {
		return Column(c, fn)
	}
	tk := &c.ThemeFor(theme.CompSkeleton).Components.Skeleton
	row := Row(c, func() {
		if s.avatar {
			// Margin.Right is the inline end: the avatar leads in RTL too.
			skelBlock(c, s.active, 32, 32, s.square, true, layout.Margin(geom.Insets{Right: 16}))
		}
		Column(c, func() {
			rad := false
			if s.title {
				w := skelBlock(c, s.active, 0, tk.TitleHeight, s.round, false, layout.Margin(geom.Insets{Top: 4}))
				w.n.ln.Width = 0
				w.widthPct = 0.38
				if s.avatar {
					w.widthPct = 0.5
				}
			}
			for i := 0; i < s.rows; i++ {
				mt := tk.ParagraphGap
				if i == 0 && s.title {
					mt = tk.ParagraphMarginTop
					if s.avatar {
						mt = tk.ParagraphMarginTopAvatar
					}
				}
				b := skelBlock(c, s.active, 0, tk.ParagraphLiHeight, s.round || rad, false, layout.Margin(geom.Insets{Top: mt}))
				if i == s.rows-1 && s.rows > 1 {
					b.widthPct = 0.61
				}
			}
		}, layout.Grow(1))
	}, layout.AlignItems(layout.AlignStart))
	// The bars are decoration: expose one busy "loading" group.
	row.n.setRole(a11y.RoleGroup).setName(c.Locale().Loading).setState(a11y.Busy)
	return row
}

type skelW struct {
	c        *Context
	n        *node
	widthPct float32
}

// skelBlock adds one grey bar/circle. w == 0 stretches.
func skelBlock(c *Context, active bool, w, h float32, round, circle bool, opts ...layout.Option) *skelW {
	b := &skelW{c: c}
	b.n = c.add("skeleton", "")
	flexbox.Apply(&b.n.ln.Style, opts...)
	b.n.ln.Width, b.n.ln.Height = w, h
	b.n.ln.Measure = func(maxW, _ float32) geom.Size {
		if w > 0 {
			return geom.Size{W: w, H: h}
		}
		return geom.Size{W: 0, H: h}
	}
	b.n.paint = func(l *paint.List, r geom.Rect) {
		if b.widthPct > 0 {
			// Partial-width bars start at the inline start.
			w := r.W * b.widthPct
			r = c.inline(r).Rect(0, r.Y, w, r.H)
		}
		rad := geom.Rad(skelToken(c).BlockRadius)
		if round {
			rad = geom.Rad(r.H / 2)
		}
		if circle {
			rad = geom.Rad(min(r.W, r.H) / 2)
		}
		skelPaint(c, l, r, rad, active)
	}
	return b
}

// skelToken returns the Skeleton token of the current scope (at paint: of
// the node being painted).
func skelToken(c *Context) *theme.SkeletonToken {
	return &c.ThemeFor(theme.CompSkeleton).Components.Skeleton
}

func skelPaint(c *Context, l *paint.List, r geom.Rect, rad geom.Radii, active bool) {
	tk := skelToken(c)
	base, hi := tk.GradientFromColor, tk.GradientToColor
	if active && c.Theme.Flavor == theme.StyleShadcn {
		// shadcn Skeleton is bg-muted animate-pulse: the block fades to
		// half opacity and back, no sweeping band.
		l.Fill(r, rad, theme.Fade(base, float64(shadPulse(c))))
		return
	}
	l.Fill(r, rad, base)
	if !active {
		return
	}
	// Shimmer: a soft highlight band sweeping left to right every 1.4s
	// (right to left in RTL).
	// The band is positioned in window space so neighbouring blocks move
	// together, as with Ant's background-position animation.
	period := int64(1400)
	t := float32(0.45)
	if !c.still() {
		t = float32(c.now.UnixMilli()%period) / float32(period)
		c.animating = true
	}
	vw := max(c.size.W, 1)
	bw := vw * 0.4
	x := -bw + (vw+bw)*t
	if c.IsRTL() {
		x = vw - (vw+bw)*t
	}
	band := geom.Rect{X: x, Y: r.Y, W: bw, H: r.H}
	clip := band.Intersect(r)
	if clip.Empty() {
		return
	}
	trans := hi
	trans.A = 0
	l.Clip(r, rad)
	l.LinearStops(band, geom.Radii{}, geom.Pt(x, 0), geom.Pt(x+bw, 0),
		paint.Stop{Offset: 0, Color: trans}, paint.Stop{Offset: 0.5, Color: hi}, paint.Stop{Offset: 1, Color: trans})
	l.Pop()
}

// SkeletonButton adds a button-shaped placeholder (shape: "default", "round", "circle").
func SkeletonButton(c *Context, size ControlSize, shape string, active bool) {
	h := c.controlHeight(size)
	w := h * 2
	switch shape {
	case "circle":
		skelBlock(c, active, h, h, false, true)
	case "round":
		skelBlock(c, active, w, h, true, false)
	default:
		b := skelBlock(c, active, w, h, false, false)
		_ = b
	}
}

// SkeletonInput adds an input-shaped placeholder.
func SkeletonInput(c *Context, size ControlSize, active bool) {
	h := c.controlHeight(size)
	skelBlock(c, active, h*5, h, false, false)
}

// SkeletonAvatar adds an avatar placeholder.
func SkeletonAvatar(c *Context, size float32, square, active bool) {
	skelBlock(c, active, size, size, false, !square)
}

// SkeletonImage adds a picture placeholder with a mountain glyph.
func SkeletonImage(c *Context, w, h float32, active bool) {
	b := skelBlock(c, active, w, h, false, false)
	prev := b.n.paint
	b.n.paint = func(l *paint.List, r geom.Rect) {
		prev(l, r)
		s := min(r.W, r.H) * 0.4
		col := skelToken(c).IconColor
		icon.Draw(l, icon.Picture, geom.Rect{X: r.X + (r.W-s)/2, Y: r.Y + (r.H-s)/2, W: s, H: s}, col, 0)
	}
}

// ---------------------------------------------------------------------------
// Countdown

// CountdownW counts down to a deadline (Statistic.Countdown / el-countdown).
type CountdownW struct {
	_ [0]func()

	c        *Context
	n        *node
	title    string
	deadline time.Time
	format   string
	prefix   string
	suffix   string
	finished bool
	color    theme.Color
}

// Countdown adds a countdown to deadline. Format tokens: D, HH, mm, ss, SSS
// (default "HH:mm:ss"); text in [brackets] is literal.
func Countdown(c *Context, title string, deadline time.Time) *CountdownW {
	w := &CountdownW{c: c, title: title, deadline: deadline, format: "HH:mm:ss"}
	th := c.Theme
	w.color = th.TextHeading
	w.n = c.add("countdown", title)
	w.n.setRole(a11y.RoleTimer)
	st := State[struct{ done bool }](c, w.n.id, nil)
	left := deadline.Sub(c.now)
	if left <= 0 {
		left = 0
		if !st.done {
			st.done = true
			w.finished = true
		}
	} else {
		st.done = false
	}
	// Title and value as Statistic's (its token: Element 12 / 20 px,
	// shadcn font-medium label and font-bold number).
	stk := &c.ThemeFor(theme.CompStatistic).Components.Statistic
	tw, vw := themeWeight(stk.TitleFontWeight), themeWeight(stk.ContentFontWeight)
	tst := c.style(stk.TitleFontSize, tw)
	vst := c.style(stk.ContentFontSize, vw)
	vst.LineHeight = stk.ContentLineHeight
	value := func() string { return FormatDuration(left, w.format) }
	if c.a11yOn {
		w.n.fill(semFunc(func(s *a11y.Node) {
			s.Value = w.prefix + value() + w.suffix
			if s.Name == "" {
				s.Name = s.Value
			}
		}))
	}
	w.n.ln.Measure = func(_, _ float32) geom.Size {
		tW := c.dataShape(w.title, tst.Size, tw).Width
		v := c.dataShape(w.prefix+value()+w.suffix, vst.Size, vw).Width
		h := vst.LineHeight
		if w.title != "" {
			h += tst.LineHeight + 4
		}
		return geom.Size{W: max(tW, v+2), H: h}
	}
	w.n.paint = func(l *paint.List, r geom.Rect) {
		y, sp := r.Y, c.inline(r) // title and value at the inline start
		if w.title != "" {
			tl := c.dataShape(w.title, tst.Size, tw)
			dataText(l, tl, sp.X(0, tl.Width), y, stk.TitleColor)
			y += tst.LineHeight + 4
		}
		lay := c.dataShape(w.prefix+value()+w.suffix, vst.Size, vw)
		dataText(l, lay, sp.X(0, lay.Width), y+(vst.LineHeight-lay.Height)/2, w.color)
	}
	if left > 0 {
		// Wake at the next visible change of the display.
		step := time.Second
		if containsToken(w.format, "SSS") {
			step = 33 * time.Millisecond
		}
		rem := left % step
		if rem == 0 {
			rem = step
		}
		c.WakeAt(c.now.Add(rem))
	}
	return w
}

// Format sets the display format (see Countdown).
func (w *CountdownW) Format(f string) *CountdownW { w.format = f; return w }

// Prefix / Suffix add text around the value.
func (w *CountdownW) Prefix(s string) *CountdownW { w.prefix = s; return w }
func (w *CountdownW) Suffix(s string) *CountdownW { w.suffix = s; return w }

// Color sets the value color.
func (w *CountdownW) Color(col theme.Color) *CountdownW { w.color = col; return w }

// Finished reports (once) that the deadline passed this frame.
func (w *CountdownW) Finished() bool { return w.finished }

func containsToken(f, tok string) bool {
	for i := 0; i+len(tok) <= len(f); i++ {
		if f[i:i+len(tok)] == tok {
			return true
		}
	}
	return false
}

// FormatDuration renders d with tokens D (days), HH, mm, ss, SSS. Larger
// units absent from the format carry into the largest present one.
func FormatDuration(d time.Duration, format string) string {
	if d < 0 {
		d = 0
	}
	ms := d.Milliseconds()
	hasD, hasH, hasM := containsToken(format, "D"), containsToken(format, "H"), containsToken(format, "m")
	var days, hours, mins, secs, millis int64
	millis = ms % 1000
	total := ms / 1000
	if hasD {
		days = total / 86400
		total %= 86400
	}
	if hasH {
		hours = total / 3600
		total %= 3600
	}
	if hasM {
		mins = total / 60
		total %= 60
	}
	secs = total
	pad := func(v int64, n int) string {
		s := itoa64(v)
		for len(s) < n {
			s = "0" + s
		}
		return s
	}
	var out []byte
	for i := 0; i < len(format); {
		switch {
		case format[i] == '[':
			j := i + 1
			for j < len(format) && format[j] != ']' {
				j++
			}
			out = append(out, format[i+1:min(j, len(format))]...)
			i = j + 1
		case hasPrefix(format[i:], "SSS"):
			out = append(out, pad(millis, 3)...)
			i += 3
		case hasPrefix(format[i:], "HH"):
			out = append(out, pad(hours, 2)...)
			i += 2
		case hasPrefix(format[i:], "mm"):
			out = append(out, pad(mins, 2)...)
			i += 2
		case hasPrefix(format[i:], "ss"):
			out = append(out, pad(secs, 2)...)
			i += 2
		case hasPrefix(format[i:], "DD"):
			out = append(out, pad(days, 2)...)
			i += 2
		case format[i] == 'D':
			out = append(out, itoa64(days)...)
			i++
		case format[i] == 'H':
			out = append(out, itoa64(hours)...)
			i++
		default:
			out = append(out, format[i])
			i++
		}
	}
	return string(out)
}

func hasPrefix(s, p string) bool { return len(s) >= len(p) && s[:len(p)] == p }

func itoa64(v int64) string {
	if v == 0 {
		return "0"
	}
	var b [20]byte
	n := len(b)
	for v > 0 {
		n--
		b[n] = byte('0' + v%10)
		v /= 10
	}
	return string(b[n:])
}

// ---------------------------------------------------------------------------
// Statistic additions

// Loading replaces the value with a skeleton bar.
func (s *StatisticW) Loading(on bool) *StatisticW {
	if !on {
		return s
	}
	s.n.setState(a11y.Busy)
	c := s.c
	prevMeasure := s.n.ln.Measure
	s.n.ln.Measure = func(w, h float32) geom.Size {
		sz := prevMeasure(w, h)
		sz.W = max(sz.W, 120)
		return sz
	}
	s.n.paint = func(l *paint.List, r geom.Rect) {
		// The title as Statistic paints it; the bar as a Skeleton block.
		tw := themeWeight(s.tk.TitleFontWeight)
		tst := c.style(s.tk.TitleFontSize, tw)
		y, sp := r.Y, c.inline(r)
		if s.title != "" {
			tl := c.dataShape(s.title, tst.Size, tw)
			dataText(l, tl, sp.X(0, tl.Width), y, s.tk.TitleColor)
			y += tst.LineHeight + 4
		}
		bh := float32(16)
		skelPaint(c, l, sp.Rect(0, y+(s.vst.LineHeight-bh)/2, min(r.W, 120), bh), geom.Rad(skelToken(c).BlockRadius), true)
	}
	return s
}

// CountUp animates the displayed value from 0 to v (precision digits) over
// the slow duration ×4 the first time the statistic appears.
func (s *StatisticW) CountUp(v float64, precision int) *StatisticW {
	a := s.c.AnimFrom(s.n.id, "countup", 0, 1, 4*s.c.Theme.DurationSlow)
	s.value = FormatNumber(v*float64(a), precision)
	if a >= 1 {
		s.value = FormatNumber(v, precision)
	}
	_ = math.Abs
	return s
}

// shadPulse is Tailwind's animate-pulse opacity (keyframes 50% {opacity:
// .5}, 2s cubic-bezier(0.4, 0, 0.6, 1) infinite) at the current frame; 1
// when animations are off.
func shadPulse(c *Context) float32 {
	if c.still() {
		return 1
	}
	c.animating = true
	t := float64(c.now.UnixMilli()%2000) / 2000
	p := 2 * t // 0 → 1 over the first second, back over the second
	if t > .5 {
		p = 2 * (1 - t)
	}
	e := (1 - math.Cos(math.Pi*p)) / 2 // symmetric ease-in-out
	return float32(1 - .5*e)
}
