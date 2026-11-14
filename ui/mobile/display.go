package mobile

import (
	"math"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Button

// Size is a mobile control size.
type Size uint8

const (
	Middle Size = iota
	Mini
	Small
	Large
)

func (s Size) index() int {
	switch s {
	case Mini:
		return 0
	case Small:
		return 1
	case Large:
		return 3
	}
	return 2
}

// ButtonW is a touch button sized for fingers.
type ButtonW struct {
	c        *ui.Context
	th       *Theme
	b        *ui.Box
	label    string
	color    string
	fill     string
	size     Size
	block    bool
	round    bool
	loading  bool
	disabled bool
	ic       *icon.Icon
	pr       Press
	ang      float64
	rtl      bool
}

// Button adds a button labelled label. In the shadcn style the default
// color is the outline variant, "primary" the near-black default variant
// and "danger" the destructive one, with medium-weight labels and h-11
// (Middle) touch heights.
//
// ConfigProvider defaults ("mobile.Button"): "size" ("mini", "small",
// "middle", "large", a mobile.Size or a ui.ControlSize; otherwise the
// ConfigProvider size), "shape" ("round" or "default"), "fill" ("solid",
// "outline", "none"), "color" (string), "block" (bool).
func Button(c *ui.Context, label string) *ButtonW {
	th := ThemeOf(c)
	w := &ButtonW{c: c, th: th, label: label, fill: "solid", rtl: c.IsRTL()}
	if s, ok := cfgSize(c, "Button"); ok {
		w.size = s
	} else if s, ok := c.ComponentSize(); ok {
		w.size = sizeOfControl(s)
	}
	if v, ok := cfgRound(c, "Button"); ok {
		w.round = v
	}
	if v, ok := cfgStr(c, "Button", "fill"); ok {
		w.fill = v
	}
	if v, ok := cfgStr(c, "Button", "color"); ok {
		w.color = v
	}
	w.b = ui.Custom(c, w.measure, w.paint)
	w.b.Style(layout.NoStretch())
	w.b.Semantics(a11y.RoleButton, label)
	w.pr = Touchable(c, w.b)
	if v, ok := cfgBool(c, "Button", "block"); ok && v {
		w.Block()
	}
	return w
}

// A11yLabel sets the accessible name (icon-only buttons).
func (w *ButtonW) A11yLabel(s string) *ButtonW { w.b.Semantics(a11y.RoleButton, s); return w }

// inactiveSem removes the activation of a disabled or loading control
// that is still a focus stop.
func inactiveSem(s *a11y.Node) { s.Actions &^= a11y.Press }

// Color sets the color: "default", "primary", "success", "warning", "danger".
func (w *ButtonW) Color(s string) *ButtonW { w.color = s; return w }

// Primary is Color("primary").
func (w *ButtonW) Primary() *ButtonW { return w.Color("primary") }

// Danger is Color("danger").
func (w *ButtonW) Danger() *ButtonW { return w.Color("danger") }

// Fill sets "solid" (default), "outline" or "none".
func (w *ButtonW) Fill(s string) *ButtonW { w.fill = s; return w }

// Size sets the size.
func (w *ButtonW) Size(s Size) *ButtonW { w.size = s; return w }

// Block stretches the button to the full width.
func (w *ButtonW) Block() *ButtonW {
	w.block = true
	w.b.Style(layout.Stretch())
	return w
}

// Round makes the ends fully rounded.
func (w *ButtonW) Round() *ButtonW { w.round = true; return w }

// Icon adds a leading icon.
func (w *ButtonW) Icon(ic *icon.Icon) *ButtonW { w.ic = ic; return w }

// Loading shows a spinner and ignores taps.
func (w *ButtonW) Loading(on bool) *ButtonW {
	w.loading = on
	if on {
		w.ang = spinAngle(w.c, w.b.ID())
		w.b.A11yState(a11y.Busy)
		if w.c.A11yEnabled() {
			w.b.A11y(inactiveSem)
		}
	}
	return w
}

// Disabled greys the button out.
func (w *ButtonW) Disabled(on bool) *ButtonW {
	w.disabled = on
	if on {
		w.b.A11yState(a11y.Disabled)
		if w.c.A11yEnabled() {
			w.b.A11y(inactiveSem)
		}
	}
	return w
}

// Layout applies layout options.
func (w *ButtonW) Layout(opts ...layout.Option) *ButtonW { w.b.Style(opts...); return w }

// Clicked reports a tap this frame.
func (w *ButtonW) Clicked() bool { return w.pr.Tap && !w.disabled && !w.loading }

// ID returns the button's ID.
func (w *ButtonW) ID() ui.ID { return w.b.ID() }

func (w *ButtonW) font() text.Style {
	th := w.th
	return th.Font(th.ButtonFont[w.size.index()], th.ButtonWeight)
}

func (w *ButtonW) measure(maxW, _ float32) geom.Size {
	th := w.th
	h := th.ButtonHeight[w.size.index()]
	tw := textW(w.label, w.font())
	if w.ic != nil || w.loading {
		tw += w.font().Size + 4
	}
	pad := []float32{6, 10, 15, 20}[w.size.index()]
	if th.Style == Vant {
		pad = []float32{4, 8, 15, 15}[w.size.index()]
	} else if th.shad() {
		pad = []float32{8, 12, 16, 24}[w.size.index()] // px-2 / px-3 / px-4 / px-6
	}
	if w.block {
		return geom.Size{W: maxW, H: h}
	}
	return geom.Size{W: tw + 2*pad, H: h}
}

func (w *ButtonW) paint(l *paint.List, r geom.Rect) {
	th := w.th
	rad := th.ButtonRadius
	if w.round {
		rad = r.H / 2
	}
	rr := geom.Rad(rad)
	base := th.Primary
	solid := false
	switch w.color {
	case "", "default":
		base = th.Text
	default:
		base = th.ColorOf(w.color)
		solid = w.fill == "solid"
	}
	fg := base
	shad := th.shad()
	switch {
	case solid:
		if shad {
			l.Shadows(r, rr, th.Base.ButtonShadow)
		}
		l.Fill(r, rr, base)
		fg = th.onColor(w.color)
	case w.fill == "none":
		if shad && w.pr.Pressed && !w.disabled && !w.loading {
			l.Fill(r, rr, th.Fill) // ghost: accent background when pressed
		}
	case w.color == "" || w.color == "default":
		if shad {
			l.Shadows(r, rr, th.Base.ButtonShadow)
		}
		l.Fill(r, rr, th.Box)
		bc := th.Border
		if th.Style == AntMobile && !th.Dark {
			bc = theme.Hex("#eeeeee")
		} else if shad {
			bc = th.CardBorder
		}
		l.Stroke(r, rr, 1, bc)
	default:
		l.Fill(r, rr, th.Box)
		l.Stroke(r, rr, 1, base)
	}
	if w.disabled || w.loading {
		a := float32(0.4)
		if shad {
			a = 0.5 // disabled:opacity-50
		}
		l.Opacity(a)
	} else if w.pr.Pressed && !(shad && w.fill == "none") {
		l.Fill(r, rr, theme.RGBA(0, 0, 0, .08))
	}
	fs := w.font()
	tw := textW(w.label, fs)
	is := fs.Size
	if w.ic != nil || w.loading {
		tw += is + 4
	}
	// The icon (or spinner) leads the label: on its right in RTL.
	sp := spanOf(r, w.rtl)
	x := r.X + (r.W-tw)/2
	if w.loading {
		drawSpinner(l, sp.P(x+is/2), r.Y+r.H/2, is/2-1, w.ang, fg)
		x += is + 4
	} else if w.ic != nil {
		sp.icon(l, w.ic, x, r.Y+(r.H-is)/2, is, fg)
		x += is + 4
	}
	sp.textMid(l, w.label, fs, x, r, fg)
	if w.disabled || w.loading {
		l.Pop()
	}
}

// ---------------------------------------------------------------------------
// Badge

// Badge wraps content with a badge at its top-end corner (top right; top
// left in RTL): content is the text ("99+"), "dot" for a dot, or "" for
// none.
func Badge(c *ui.Context, content string, fn func()) *ui.Box {
	th := ThemeOf(c)
	b := ui.StackBox(c, fn)
	b.Style(layout.NoStretch())
	if content == "" {
		return b
	}
	if content != "dot" && c.A11yEnabled() {
		// The count is painted: read it as the description of the content.
		b.Semantics(a11y.RoleGroup, "").A11y(func(s *a11y.Node) { s.Description = content })
	}
	rtl := c.IsRTL()
	return b.PaintOver(func(l *paint.List, r geom.Rect) {
		x := spanOf(r, rtl).P(r.Right())
		if content == "dot" {
			drawBadge(l, th, "", x, r.Y)
			return
		}
		drawBadge(l, th, content, x, r.Y)
	})
}

// ---------------------------------------------------------------------------
// Tag

// TagW is a small label.
type TagW struct {
	c     *ui.Context
	th    *Theme
	label string
	color string
	fill  string
	round bool
}

// Tag adds a tag (a shadcn Badge in the shadcn style: rounded-md,
// text-xs medium; "default" is the secondary variant).
//
// ConfigProvider defaults ("mobile.Tag"): "color" (string), "fill"
// ("solid", "outline", "light"), "round" (bool).
func Tag(c *ui.Context, label string) *TagW {
	th := ThemeOf(c)
	w := &TagW{c: c, th: th, label: label, color: "default", fill: "solid"}
	if v, ok := cfgStr(c, "Tag", "color"); ok {
		w.color = v
	}
	if v, ok := cfgStr(c, "Tag", "fill"); ok {
		w.fill = v
	}
	if v, ok := cfgBool(c, "Tag", "round"); ok {
		w.round = v
	}
	ui.Custom(c, w.measure, w.paint).Style(layout.NoStretch(), layout.Self(layout.AlignCenter)).
		Semantics(a11y.RoleText, label)
	return w
}

// Color sets "default", "primary", "success", "warning", "danger".
func (w *TagW) Color(s string) *TagW { w.color = s; return w }

// Outline draws a border instead of a solid fill.
func (w *TagW) Outline() *TagW { w.fill = "outline"; return w }

// Light uses a tinted background with colored text.
func (w *TagW) Light() *TagW { w.fill = "light"; return w }

// Round makes the tag pill-shaped.
func (w *TagW) Round() *TagW { w.round = true; return w }

func (w *TagW) font() text.Style {
	st := w.th.Font(12, text.Regular)
	st.LineHeight = 16
	if w.th.shad() {
		st.Weight = text.Medium
	}
	return st
}

func (w *TagW) measure(_, _ float32) geom.Size {
	if w.th.shad() {
		return geom.Size{W: textW(w.label, w.font()) + 16, H: 22} // px-2 py-0.5
	}
	return geom.Size{W: textW(w.label, w.font()) + 10, H: 18}
}

func (w *TagW) paint(l *paint.List, r geom.Rect) {
	th := w.th
	if th.shad() {
		w.paintShad(l, r)
		return
	}
	rad := geom.Rad(2)
	if w.round {
		rad = geom.Rad(r.H / 2)
	}
	base := th.ColorOf(w.color)
	if w.color == "default" {
		base = th.TextSecondary
		if th.Style == Vant {
			base = th.TextWeak
		}
	}
	fg := th.OnPrimary
	switch w.fill {
	case "outline":
		l.Stroke(r, rad, 1, base)
		fg = base
	case "light":
		l.Fill(r, rad, fadeCol(base, 0.12))
		fg = base
	default:
		if w.color == "default" && th.Style == AntMobile {
			l.Fill(r, rad, th.Fill)
			fg = th.TextSecondary
			l.Stroke(r, rad, 1, th.Border)
		} else {
			l.Fill(r, rad, base)
			fg = th.onColor(w.color)
		}
	}
	drawTextCenter(l, w.label, w.font(), r, fg)
}

// paintShad draws the shadcn Badge variants: solid (default / secondary
// / destructive …), outline (border with foreground text) and light
// (tinted).
func (w *TagW) paintShad(l *paint.List, r geom.Rect) {
	th := w.th
	rad := geom.Rad(th.Radius)
	if w.round {
		rad = geom.Rad(r.H / 2)
	}
	def := w.color == "default" || w.color == ""
	base := th.ColorOf(w.color)
	var fg paintColor
	switch {
	case w.fill == "outline":
		l.Stroke(r, rad, 1, th.CardBorder)
		fg = th.Text
		if !def {
			fg = base
		}
	case w.fill == "light":
		if def {
			l.Fill(r, rad, th.Fill)
			fg = th.TextSecondary
		} else {
			l.Fill(r, rad, fadeCol(base, 0.12))
			fg = base
		}
	case def:
		l.Fill(r, rad, th.Fill) // secondary
		fg = th.Text
	default:
		l.Fill(r, rad, base)
		fg = th.onColor(w.color)
	}
	drawTextCenter(l, w.label, w.font(), r, fg)
}

// ---------------------------------------------------------------------------
// Grid

// GridItem is one cell of a Grid.
type GridItem struct {
	Text  string
	Icon  *icon.Icon
	Badge string // "", "dot" or count text
}

// GridW is a grid of icon + text entries.
type GridW struct {
	c       *ui.Context
	th      *Theme
	cols    int
	items   []GridItem
	border  bool
	pr      Press
	clicked int
	rtl     bool // cells run right to left
}

// Grid adds a grid of items in cols columns. In the shadcn style the
// icons sit on muted rounded-xl tiles and cells have no separators.
//
// ConfigProvider defaults ("mobile.Grid"): "bordered" (bool).
func Grid(c *ui.Context, cols int, items []GridItem) *GridW {
	th := ThemeOf(c)
	w := &GridW{c: c, th: th, cols: max(1, cols), items: items, border: !th.shad(), clicked: -1, rtl: c.IsRTL()}
	if v, ok := cfgBool(c, "Grid", "bordered"); ok {
		w.border = v
	}
	m := newMirror(c, true)
	b := ui.Custom(c, w.measure, filled(th.Box, w.paint))
	w.pr = touchAs(c, b, touchPointer)
	// Screen readers and the keyboard: one button per cell over the
	// painted grid.
	if m.over(b, a11y.RoleGroup, "", 0) != nil {
		r, _ := c.RectOf(b.ID())
		cw, ch := r.W/float32(w.cols), w.cellH()
		sp := spanOf(geom.Rect{W: r.W}, w.rtl)
		for i, it := range items {
			p := m.part(sp.R(geom.Rect{X: float32(i%w.cols) * cw, Y: float32(i/w.cols) * ch, W: cw, H: ch}), a11y.RoleButton, it.Text, 0, true)
			if it.Badge != "" && it.Badge != "dot" && c.A11yEnabled() {
				badge := it.Badge
				p.box.A11y(func(s *a11y.Node) { s.Description = badge })
			}
			if p.tap {
				w.clicked = i
			}
		}
		m.done()
	}
	if w.pr.Tap {
		if r, ok := c.RectOf(b.ID()); ok {
			cw, ch := r.W/float32(w.cols), w.cellH()
			i := int((w.pr.TapPos.Y-r.Y)/ch)*w.cols + int((spanOf(r, w.rtl).P(w.pr.TapPos.X)-r.X)/cw)
			if i >= 0 && i < len(items) {
				w.clicked = i
			}
		}
	}
	return w
}

// Border toggles cell separators.
func (w *GridW) Border(on bool) *GridW { w.border = on; return w }

// Clicked returns the index tapped this frame.
func (w *GridW) Clicked() (int, bool) { return w.clicked, w.clicked >= 0 }

func (w *GridW) cellH() float32 {
	if w.th.Style == Vant {
		return 84
	}
	if w.th.shad() {
		return 92
	}
	return 80
}

func (w *GridW) measure(maxW, _ float32) geom.Size {
	rows := (len(w.items) + w.cols - 1) / w.cols
	return geom.Size{W: maxW, H: float32(rows) * w.cellH()}
}

func (w *GridW) paint(l *paint.List, r geom.Rect) {
	th := w.th
	cw, ch := r.W/float32(w.cols), w.cellH()
	is := float32(28)
	fs := th.Font(12, text.Regular)
	if th.Style != Vant {
		fs = th.Font(th.FontSmall, text.Regular)
	}
	tile := float32(0)
	if th.shad() {
		is, tile = 22, 48
	}
	// Laid out left to right; sp mirrors the parts in RTL.
	sp := spanOf(r, w.rtl)
	for i, it := range w.items {
		cell := geom.Rect{X: r.X + float32(i%w.cols)*cw, Y: r.Y + float32(i/w.cols)*ch, W: cw, H: ch}
		if w.pr.Pressed && cell.Contains(sp.Pt(w.pr.PressPos)) {
			l.Fill(sp.R(cell), geom.Radii{}, th.Press)
		}
		top := cell.Y + (ch-is-8-fs.LineHeight)/2
		ix := cell.X + (cw-is)/2
		if tile > 0 {
			// shadcn: the icon on a muted tile; the badge hugs the tile.
			top = cell.Y + (ch-tile-6-fs.LineHeight)/2
			tr := geom.Rect{X: cell.X + (cw-tile)/2, Y: top, W: tile, H: tile}
			l.Fill(sp.R(tr), geom.Rad(th.RadiusLarge), th.Fill)
			if it.Icon != nil {
				sp.icon(l, it.Icon, tr.X+(tile-is)/2, tr.Y+(tile-is)/2, is, th.Text)
			}
			switch it.Badge {
			case "":
			case "dot":
				drawBadge(l, th, "", sp.P(tr.Right()-2), tr.Y+2)
			default:
				drawBadge(l, th, it.Badge, sp.P(tr.Right()-2), tr.Y+2)
			}
			sp.textCenter(l, it.Text, fs, geom.Rect{X: cell.X, Y: top + tile + 6, W: cw, H: fs.LineHeight}, th.Text)
			if w.border {
				sp.fill(l, geom.Rect{X: cell.Right() - 0.5, Y: cell.Y, W: 0.5, H: ch}, geom.Radii{}, th.Border)
				sp.fill(l, geom.Rect{X: cell.X, Y: cell.Bottom() - 0.5, W: cw, H: 0.5}, geom.Radii{}, th.Border)
			}
			continue
		}
		if it.Icon != nil {
			sp.icon(l, it.Icon, ix, top, is, th.Text)
		}
		switch it.Badge {
		case "":
		case "dot":
			drawBadge(l, th, "", sp.P(ix+is), top+2)
		default:
			drawBadge(l, th, it.Badge, sp.P(ix+is+4), top+3)
		}
		col := th.Text
		if th.Style == Vant {
			col = th.TextSecondary
		}
		sp.textCenter(l, it.Text, fs, geom.Rect{X: cell.X, Y: top + is + 8, W: cw, H: fs.LineHeight}, col)
		if w.border {
			sp.fill(l, geom.Rect{X: cell.Right() - 0.5, Y: cell.Y, W: 0.5, H: ch}, geom.Radii{}, th.Border)
			sp.fill(l, geom.Rect{X: cell.X, Y: cell.Bottom() - 0.5, W: cw, H: 0.5}, geom.Radii{}, th.Border)
		}
	}
}

// ---------------------------------------------------------------------------
// NoticeBar

type noticeState struct {
	closed bool
	start  time.Time
}

// NoticeBarW is a one-line announcement bar, optionally scrolling.
type NoticeBarW struct {
	c         *ui.Context
	th        *Theme
	msg       string
	color     string
	closeable bool
	scroll    bool
	closed    bool
	st        *noticeState
	pr        Press
	off       float32
	b         *ui.Box
	m         *mirror
	rtl       bool
}

// NoticeBar adds an announcement bar showing msg (a neutral shadcn Alert
// strip in the shadcn style).
//
// ConfigProvider defaults ("mobile.NoticeBar"): "color" (string),
// "closeable", "scroll" (bool).
func NoticeBar(c *ui.Context, msg string) *NoticeBarW {
	th := ThemeOf(c)
	w := &NoticeBarW{c: c, th: th, msg: msg, color: "default", rtl: c.IsRTL()}
	w.m = newMirror(c, true)
	b := ui.Custom(c, func(maxW, _ float32) geom.Size {
		if w.st.closed {
			return geom.Size{}
		}
		return geom.Size{W: maxW, H: 40}
	}, nop)
	w.b = b
	w.st = ui.State[noticeState](c, b.ID(), nil)
	w.pr = touchAs(c, b, touchPointer)
	if w.st.start.IsZero() {
		w.st.start = c.Now()
	}
	b.Paint(w.paint)
	if !w.st.closed {
		w.m.over(b, a11y.RoleStatus, msg, 0)
	} else {
		b.A11yHidden()
	}
	if v, ok := cfgStr(c, "NoticeBar", "color"); ok {
		w.color = v
	}
	if v, ok := cfgBool(c, "NoticeBar", "closeable"); ok && v {
		w.Closeable()
	}
	if v, ok := cfgBool(c, "NoticeBar", "scroll"); ok && v {
		w.Scroll()
	}
	return w
}

// Color sets "default" (orange), "alert", "error" or "info".
func (w *NoticeBarW) Color(s string) *NoticeBarW { w.color = s; return w }

// Closeable shows a close button.
func (w *NoticeBarW) Closeable() *NoticeBarW {
	if w.closeable {
		return w
	}
	w.closeable = true
	r, ok := w.c.RectOf(w.b.ID())
	var p partRes
	if !w.st.closed {
		z := geom.Rect{X: r.W - w.th.PadH - 28, W: w.th.PadH + 28, H: r.H}
		p = w.m.part(spanOf(geom.Rect{W: r.W}, w.rtl).R(z), a11y.RoleButton, w.c.Locale().Close, 0, true)
		w.m.done()
	}
	if (w.pr.Tap || p.tap) && !w.st.closed {
		if p.tap || ok && spanOf(r, w.rtl).P(w.pr.TapPos.X) > r.Right()-w.th.PadH-28 {
			w.st.closed, w.closed = true, true
			w.c.Invalidate()
		}
	}
	return w
}

// Scroll makes long text scroll (marquee; towards the start, so to the
// right in RTL).
func (w *NoticeBarW) Scroll() *NoticeBarW {
	w.scroll = true
	if !w.c.ReduceMotion() && pump(w.c, w.b.ID(), 0) {
		w.off = float32(w.c.Now().Sub(w.st.start).Seconds() * 50)
	}
	return w
}

// Closed reports a tap on the close button this frame.
func (w *NoticeBarW) Closed() bool { return w.closed }

func (w *NoticeBarW) colors() (bg, fg paintColor) {
	th := w.th
	switch w.color {
	case "error":
		return th.Danger, th.OnSolid
	case "info":
		return th.PrimaryLight, th.Primary
	case "alert":
		return th.NoticeBg, th.NoticeText
	}
	return th.NoticeBg, th.NoticeText
}

func (w *NoticeBarW) paint(l *paint.List, r geom.Rect) {
	if w.st.closed || r.H <= 0 {
		return
	}
	th := w.th
	bg, fg := w.colors()
	l.Fill(r, geom.Radii{}, bg)
	if th.NoticeBorder.A > 0 && bg == th.NoticeBg {
		hairlineTop(l, r, r.X, th.NoticeBorder)
		hairline(l, r, r.X, th.NoticeBorder)
	}
	fs := th.Font(th.FontMain, text.Regular)
	if th.Style == Vant {
		fs = th.Font(14, text.Regular)
	}
	// Laid out left to right; sp mirrors the parts in RTL.
	sp := spanOf(r, w.rtl)
	x := r.X + th.PadH
	sp.icon(l, icon.Sound, x, r.Y+(r.H-18)/2, 18, fg)
	x += 18 + 8
	right := r.Right() - th.PadH
	if w.closeable {
		sp.icon(l, icon.Close, right-16, r.Y+(r.H-16)/2, 16, fg)
		right -= 16 + 8
	}
	clip := geom.Rect{X: x, Y: r.Y, W: max(0, right-x), H: r.H}
	l.Clip(sp.R(clip), geom.Radii{})
	tw := textW(w.msg, fs)
	if w.scroll && tw > clip.W {
		span := tw + clip.W
		off := float32(math.Mod(float64(w.off), float64(span)))
		sp.textMid(l, w.msg, fs, clip.X-off+min(off, 0), clip, fg)
		if clip.X-off+tw < clip.Right() {
			sp.textMid(l, w.msg, fs, clip.X-off+span, clip, fg)
		}
	} else {
		lay := text.Shape(w.msg, fs, sp.opts(text.Options{MaxWidth: clip.W, MaxLines: 1, Ellipsis: true}))
		l.Glyphs(geom.Pt(sp.X(clip.X, lay.Width), r.Y+(r.H-lay.Height)/2), lay.Glyphs, fg)
	}
	l.Pop()
}

// ---------------------------------------------------------------------------
// Skeleton

// SkeletonW is a loading placeholder of grey bars.
type SkeletonW struct {
	th       *Theme
	title    bool
	rows     int
	avatar   bool
	animated bool
	phase    float32
	rtl      bool
}

// Skeleton adds a placeholder; choose parts with Title, Paragraph, Avatar.
// The shadcn style pulses rounded-md muted bars instead of a shimmer.
//
// ConfigProvider defaults ("mobile.Skeleton"): "title", "avatar",
// "animated" (bool), "rows" (int).
func Skeleton(c *ui.Context) *SkeletonW {
	th := ThemeOf(c)
	w := &SkeletonW{th: th, rows: 3, rtl: c.IsRTL()}
	b := ui.Custom(c, w.measure, w.paint)
	b.Semantics(a11y.RoleStatus, c.Locale().A11y.Loading).A11yState(a11y.Busy)
	if !c.ReduceMotion() && pump(c, b.ID(), 0) {
		period := int64(1400)
		if th.shad() {
			period = 2000 // animate-pulse
		}
		w.phase = float32(c.Now().UnixMilli()%period) / float32(period)
	}
	if v, ok := cfgBool(c, "Skeleton", "title"); ok {
		w.title = v
	}
	if v, ok := cfgBool(c, "Skeleton", "avatar"); ok {
		w.avatar = v
	}
	if v, ok := cfgNum(c, "Skeleton", "rows"); ok {
		w.rows = int(v)
	}
	if v, ok := cfgBool(c, "Skeleton", "animated"); ok && !v {
		w.phase = -1
	}
	return w
}

// Title shows a title bar.
func (w *SkeletonW) Title() *SkeletonW { w.title = true; return w }

// Paragraph sets the number of text rows.
func (w *SkeletonW) Paragraph(n int) *SkeletonW { w.rows = n; return w }

// Avatar shows a round avatar placeholder.
func (w *SkeletonW) Avatar() *SkeletonW { w.avatar = true; return w }

// Animated toggles the shimmer (on by default when animations run).
func (w *SkeletonW) Animated(on bool) *SkeletonW {
	if !on {
		w.phase = -1
	}
	return w
}

func (w *SkeletonW) measure(maxW, _ float32) geom.Size {
	h := float32(w.rows)*16 + float32(max(0, w.rows-1))*12
	if w.title {
		h += 21 + 16
	}
	if w.avatar {
		h = max(h, 40)
	}
	return geom.Size{W: maxW, H: h + 2*w.th.PadH}
}

func (w *SkeletonW) paint(l *paint.List, r geom.Rect) {
	th := w.th
	fill := th.Fill
	r = r.Inset(geom.All(th.PadH))
	// Laid out left to right (avatar, then bars from the start); sp
	// mirrors them, and the shimmer's sweep, in RTL.
	sp := spanOf(r, w.rtl)
	x := r.X
	if w.avatar {
		l.Fill(sp.R(geom.Rect{X: x, Y: r.Y, W: 40, H: 40}), geom.Rad(20), fill)
		x += 40 + 16
	}
	y := r.Y
	wd := r.Right() - x
	shad := th.shad()
	rad := geom.Rad(2)
	if shad {
		rad = geom.Rad(th.Radius)
		if w.phase >= 0 {
			// Opacity 1 → 0.5 → 1 (Tailwind animate-pulse).
			l.Opacity(float32(0.75 + 0.25*math.Cos(2*math.Pi*float64(w.phase))))
			defer l.Pop()
		}
	}
	bar := func(bw, bh float32) {
		br := geom.Rect{X: x, Y: y, W: bw, H: bh}
		l.Fill(sp.R(br), rad, fill)
		if w.phase >= 0 && !shad {
			hl := fadeCol(th.Box, 0.5)
			sx := br.X - br.W + w.phase*br.W*3
			l.LinearStops(sp.R(br), geom.Rad(2), geom.Pt(sp.P(sx), 0), geom.Pt(sp.P(sx+br.W), 0),
				paint.Stop{Offset: 0, Color: fadeCol(hl, 0)}, paint.Stop{Offset: 0.5, Color: hl}, paint.Stop{Offset: 1, Color: fadeCol(hl, 0)})
		}
	}
	if w.title {
		bar(wd*0.4, 21)
		y += 21 + 16
	}
	for i := 0; i < w.rows; i++ {
		bw := wd
		if i == w.rows-1 && w.rows > 1 {
			bw = wd * 0.6
		}
		bar(bw, 16)
		y += 16 + 12
	}
}

// ---------------------------------------------------------------------------
// Empty & Result

// EmptyW is an empty-state placeholder.
type EmptyW struct {
	th   *Theme
	desc string
	b    *ui.Box
}

// Empty adds an empty state with the localized "暂无数据" (shadcn: an
// icon on a muted tile above the muted caption).
//
// ConfigProvider defaults ("mobile.Empty"): "description" (string).
func Empty(c *ui.Context) *EmptyW {
	th := ThemeOf(c)
	w := &EmptyW{th: th, desc: c.Locale().NoData}
	if v, ok := cfgStr(c, "Empty", "description"); ok {
		w.desc = v
	}
	w.b = ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: 180} }, w.paint)
	w.b.Semantics(a11y.RoleText, w.desc)
	return w
}

// Description replaces the caption.
func (w *EmptyW) Description(s string) *EmptyW {
	w.desc = s
	w.b.Semantics(a11y.RoleText, s)
	return w
}

func (w *EmptyW) paint(l *paint.List, r geom.Rect) {
	th := w.th
	cx := r.X + r.W/2
	top := r.Y + 24
	if th.shad() {
		tile := geom.Rect{X: cx - 24, Y: top + 28, W: 48, H: 48}
		l.Fill(tile, geom.Rad(th.RadiusLarge-4), th.Fill)
		drawIcon(l, icon.Inbox, tile.X+12, tile.Y+12, 24, th.Text)
		fs := th.Font(th.FontMain, text.Regular)
		drawTextCenter(l, w.desc, fs, geom.Rect{X: r.X, Y: tile.Bottom() + 16, W: r.W, H: fs.LineHeight}, th.TextWeak)
		return
	}
	// A simple open box illustration drawn with neutral fills.
	light := th.Fill
	mid := theme.Mix(th.Fill, th.TextLight, 0.45)
	l.Fill(geom.Rect{X: cx - 52, Y: top + 82, W: 104, H: 8}, geom.Rad(4), light)
	body := paint.NewPath().MoveTo(cx-40, top+40).LineTo(cx+40, top+40).LineTo(cx+40, top+84).LineTo(cx-40, top+84).Close()
	l.Path(body, mid)
	lid := paint.NewPath().MoveTo(cx-40, top+40).LineTo(cx-28, top+20).LineTo(cx+28, top+20).LineTo(cx+40, top+40).Close()
	l.Path(lid, light)
	slot := paint.NewPath().MoveTo(cx-40, top+40).LineTo(cx-14, top+40).LineTo(cx-10, top+50).LineTo(cx+10, top+50).LineTo(cx+14, top+40).LineTo(cx+40, top+40)
	l.StrokePath(slot, paint.StrokeStyle{Width: 1.5}, th.Box)
	fs := th.Font(th.FontMain, text.Regular)
	if th.Style == Vant {
		fs = th.Font(14, text.Regular)
	}
	drawTextCenter(l, w.desc, fs, geom.Rect{X: r.X, Y: top + 104, W: r.W, H: fs.LineHeight}, th.TextWeak)
}

// ResultW is a full-width outcome page block.
type ResultW struct {
	th     *Theme
	status string
	title  string
	desc   string
}

// Result adds an outcome block: status "success", "error", "info",
// "waiting" or "warning".
func Result(c *ui.Context, status, title string) *ResultW {
	th := ThemeOf(c)
	w := &ResultW{th: th, status: status, title: title}
	b := ui.Custom(c, w.measure, filled(th.Box, w.paint))
	// An outcome message: a polite live region named by the title, the
	// explanation as its description.
	b.Semantics(a11y.RoleStatus, title)
	if c.A11yEnabled() {
		b.A11y(func(s *a11y.Node) { s.Description = w.desc })
	}
	return w
}

// Description adds an explanation under the title.
func (w *ResultW) Description(s string) *ResultW { w.desc = s; return w }

func (w *ResultW) measure(maxW, _ float32) geom.Size {
	h := float32(32 + 64 + 16 + 30 + 32)
	if w.desc != "" {
		lay := text.Shape(w.desc, w.th.Font(w.th.FontSmall+1, text.Regular), text.Options{MaxWidth: maxW - 48})
		h += lay.Height + 8
	}
	return geom.Size{W: maxW, H: h}
}

func (w *ResultW) paint(l *paint.List, r geom.Rect) {
	th := w.th
	col := th.Primary
	ic := icon.Info
	switch w.status {
	case "success":
		col, ic = th.Success, icon.Check
	case "error":
		col, ic = th.Danger, icon.Close
	case "warning":
		col, ic = th.Warning, icon.Exclamation
	case "waiting":
		col, ic = th.Primary, icon.Clock
	}
	cx := r.X + r.W/2
	top := r.Y + 32
	ir := geom.Rect{X: cx - 32, Y: top, W: 64, H: 64}
	if ic == icon.Check || ic == icon.Close {
		l.Fill(ir, geom.Rad(32), col)
		icon.Draw(l, ic, ir.Inset(geom.All(16)), th.OnSolid, 0)
	} else {
		icon.Draw(l, ic, ir, col, 0)
	}
	tf := th.Font(th.FontLarge, text.Regular)
	if th.shad() {
		tf.Weight = text.Semibold
	}
	drawTextCenter(l, w.title, tf, geom.Rect{X: r.X, Y: top + 64 + 16, W: r.W, H: tf.LineHeight}, th.Text)
	if w.desc != "" {
		df := th.Font(th.FontSmall+1, text.Regular)
		lay := text.Shape(w.desc, df, text.Options{MaxWidth: r.W - 48})
		y := top + 64 + 16 + tf.LineHeight + 8
		l.Glyphs(geom.Pt(cx-lay.Width/2, y), lay.Glyphs, th.TextWeak)
	}
}
