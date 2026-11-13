package mobile

import (
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// ---------------------------------------------------------------------------
// Tabs

type tabsState struct {
	head    Scroller // header scroll when titles overflow
	pane    Scroller // content swipe
	gen     uint64
	headGen uint64
	tabX    []float32 // title centres / widths relative to the header, last paint
	tabW    []float32
	headW   float32
}

// TabsW is a tab header with an underline and (optionally swipeable)
// content panes.
type TabsW struct {
	c         *ui.Context
	th        *Theme
	titles    []string
	active    *int
	swipeable bool
	changed   bool
	st        *tabsState
	outer     *ui.Box
	rtl       bool // titles and panes run right to left
}

// Tabs adds tabs titled titles; *active is the selected index. Build the
// panes with Body (or Header for a header only).
//
// In the shadcn style the header is a segmented control: a muted
// rounded-lg track with the selected title on a raised thumb.
//
// In RTL the first title is at the right, the header scrolls from its right
// end and the panes follow the titles (the next pane comes in from the
// left).
//
// ConfigProvider defaults ("mobile.Tabs"): "swipeable" (bool).
func Tabs(c *ui.Context, titles []string, active *int) *TabsW {
	w := &TabsW{c: c, th: ThemeOf(c), titles: titles, active: active, rtl: c.IsRTL()}
	w.outer = ui.Div(c)
	w.st = ui.State[tabsState](c, w.outer.ID(), nil)
	if v, ok := cfgBool(c, "Tabs", "swipeable"); ok {
		w.swipeable = v
	}
	return w
}

// Swipeable lets a horizontal swipe on the content switch tabs.
func (w *TabsW) Swipeable() *TabsW { w.swipeable = true; return w }

// Changed reports a tab switch this frame.
func (w *TabsW) Changed() bool { return w.changed }

func (w *TabsW) set(i int) {
	if i != *w.active && i >= 0 && i < len(w.titles) {
		*w.active = i
		w.changed = true
		w.c.Invalidate()
	}
}

// Header builds the header only.
func (w *TabsW) Header() *TabsW { return w.Body(nil) }

// Body builds the header and the panes; pane(i) builds pane i.
func (w *TabsW) Body(pane func(i int)) *TabsW {
	w.outer.Body(func() {
		w.header()
		if pane != nil {
			w.panes(pane)
		}
	})
	return w
}

func (w *TabsW) fonts() (text.Style, text.Style) {
	th := w.th
	if th.Style == Vant {
		return th.Font(14, text.Regular), th.Font(14, text.Medium)
	}
	if th.shad() {
		return th.Font(th.FontMain, text.Medium), th.Font(th.FontMain, text.Medium)
	}
	return th.Font(th.FontBody, text.Regular), th.Font(th.FontBody, text.Regular)
}

func (w *TabsW) header() {
	c, th, st := w.c, w.th, w.st
	n := len(w.titles)
	nf, af := w.fonts()
	pad := float32(12)
	// lead insets the segmented track of the shadcn header from the edges.
	lead := float32(0)
	if th.shad() {
		lead = th.PadH
	}
	widths := make([]float32, n)
	total := float32(0)
	for i, s := range w.titles {
		widths[i] = textW(s, af) + 2*pad
		total += widths[i]
	}
	b := ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: th.TabsHeight} }, filled(th.Box, nop))
	// The fixed height keeps the size when the accessibility children
	// below replace the measure function.
	b.Clip().Style(layout.H(th.TabsHeight))
	id := b.ID()
	r, _ := c.RectOf(id)
	view := r.W
	stretch := view > 0 && total+2*lead <= view
	if stretch {
		for i := range widths {
			widths[i] = (view - 2*lead) / float32(n)
		}
		total = view - 2*lead
	}
	total += 2 * lead
	hs := &st.head
	hs.Min, hs.Max, hs.Dim = 0, float64(max(0, total-view)), float64(view)
	g := Area(c, b, AxisHorizontal, func(Direction) bool { return !stretch })
	// The header is laid out and scrolled in logical units (hs.Pos 0 is
	// the start); sgn turns physical drags into logical ones.
	sgn := float32(1)
	if w.rtl {
		sgn = -1
	}
	// Keyboard: the header is one focus stop; Left / Right / Home / End
	// select (WAI-ARIA tabs with automatic activation; Left / Right follow
	// the visual order).
	ia := b.Focusable(input.CursorDefault)
	now := c.Now()
	if ia.Focused && n > 0 {
		to := -1
		for _, k := range keyPresses(c, id) {
			cur := *w.active
			if to >= 0 {
				cur = to
			}
			prev, next := inlineKey(w.rtl, k)
			switch {
			case prev:
				to = (cur - 1 + n) % n
			case next:
				to = (cur + 1) % n
			}
			switch k {
			case input.KeyHome:
				to = 0
			case input.KeyEnd:
				to = n - 1
			}
		}
		if to >= 0 {
			w.set(to)
			w.snapPane(now)
		}
	}
	if g.Pan.Began {
		hs.BeginDrag()
	}
	if g.Pan.Active {
		hs.DragBy(-float64(sgn * g.Pan.Delta.X))
	}
	if g.Pan.Ended {
		hs.Release(-float64(sgn*g.Pan.Velocity.X), now)
		st.headGen++
	}
	if g.Tap && r.W > 0 {
		x := spanOf(r, w.rtl).P(g.TapPos.X) - r.X + float32(hs.Pos)
		acc := lead
		for i, wd := range widths {
			if x >= acc && x < acc+wd {
				w.set(i)
				w.snapPane(now)
			}
			acc += wd
		}
	}
	// Keep the active title in view.
	if !stretch && w.changed && view > 0 {
		acc := lead
		for i := 0; i < *w.active && i < n; i++ {
			acc += widths[i]
		}
		t := float64(acc + widths[max(0, min(*w.active, n-1))]/2 - view/2)
		hs.SnapTo(max(hs.Min, min(t, hs.Max)), now)
		st.headGen++
	}
	if hs.Active() {
		if pump(c, id, st.headGen) {
			hs.Step(now)
		} else {
			hs.Settle()
		}
	}
	// Underline position (follows a content drag between tabs).
	acc := lead
	xs := make([]float32, n)
	for i := range widths {
		xs[i] = acc
		acc += widths[i]
	}
	act := max(0, min(*w.active, n-1))
	frac := float32(0)
	if pw := float32(st.pane.Dim); w.swipeable && pw > 0 && st.pane.Mode != Idle {
		frac = (float32(st.pane.Pos) - float32(act)*pw) / pw
	}
	lineCenter := func(i int) float32 { return xs[i] + widths[i]/2 }
	lineWidth := func(i int) float32 {
		if th.shad() {
			return widths[i] - 6 // the thumb, inset in the track
		}
		if th.TabsLineWidth > 0 {
			return th.TabsLineWidth
		}
		return textW(w.titles[i], af)
	}
	cx, lw := float32(0), float32(0)
	if n > 0 {
		cx, lw = lineCenter(act), lineWidth(act)
		if frac != 0 {
			j := max(0, min(n-1, act+sign(frac)))
			t := abs32(frac)
			cx = cx + (lineCenter(j)-cx)*t
			lw = lw + (lineWidth(j)-lw)*t
		}
	}
	cx = c.Anim(id, "lx", cx, th.Base.DurationMid)
	lw = c.Anim(id, "lw", lw, th.Base.DurationMid)
	off := float32(hs.Pos)
	titles, active := w.titles, *w.active
	ring := ia.FocusVisible
	if c.A11yEnabled() {
		w.tabNodes(b, xs, widths, off)
	}
	shad := th.shad()
	rtl := w.rtl
	b.Paint(func(l *paint.List, r geom.Rect) {
		l.Clip(r, geom.Radii{})
		defer l.Pop()
		// Laid out left to right; sp mirrors the parts in RTL.
		sp := spanOf(r, rtl)
		if shad && len(xs) > 0 {
			// Track (muted, rounded-lg) and thumb (background, shadow-sm).
			last := len(xs) - 1
			tr := geom.Rect{X: r.X + xs[0] - off, Y: r.Y + 6, W: xs[last] + widths[last] - xs[0], H: r.H - 12}
			tr = sp.R(tr)
			l.Fill(tr, geom.Rad(th.Radius+2), th.Fill)
			thumb := sp.R(geom.Rect{X: r.X + cx - off - lw/2, Y: tr.Y + 3, W: lw, H: tr.H - 6})
			l.Shadows(thumb, geom.Rad(th.Radius), th.Base.ShadowTertiary)
			l.Fill(thumb, geom.Rad(th.Radius), th.SegmentThumb)
		}
		for i, s := range titles {
			cell := geom.Rect{X: r.X + xs[i] - off, Y: r.Y, W: widths[i], H: r.H}
			if cell.Right() < r.X || cell.X > r.Right() {
				continue
			}
			fs, col := nf, th.TextSecondary
			if th.Style == AntMobile {
				col = th.Text
			} else if shad {
				col = th.TextWeak
			}
			if i == active {
				fs = af
				col = th.Primary
				if th.Style == Vant || shad {
					col = th.Text
				}
			}
			sp.textCenter(l, s, fs, cell, col)
		}
		if shad {
			if ring && active >= 0 && active < len(xs) {
				drawFocusRing(l, th, sp.R(geom.Rect{X: r.X + xs[active] - off, Y: r.Y + 6, W: widths[active], H: r.H - 12}))
			}
			return
		}
		lh := th.TabsLineHeight
		ly := r.Bottom() - lh
		if th.Style == Vant {
			ly = r.Bottom() - 6 - lh
		}
		sp.fill(l, geom.Rect{X: r.X + cx - off - lw/2, Y: ly, W: lw, H: lh}, geom.Rad(lh/2), th.Primary)
		if th.Style == AntMobile {
			hairline(l, r, r.X, th.Border)
		}
		if ring && active >= 0 && active < len(xs) {
			drawFocusRing(l, th, sp.R(geom.Rect{X: r.X + xs[active] - off, Y: r.Y, W: widths[active], H: r.H}))
		}
	})
}

// tabNodes exposes the header as a tab list with one (invisible) tab per
// title; the header keeps focus and points at the selected tab.
func (w *TabsW) tabNodes(b *ui.Box, xs, widths []float32, off float32) {
	c := w.c
	var sel ui.ID
	b.Semantics(a11y.RoleTabList, "").A11yState(a11y.Horizontal)
	b.Body(func() {
		for i, s := range w.titles {
			on := i == *w.active
			t := ui.Div(c, layout.Abs(xs[i]-off, 0, layout.Unset, layout.Unset), layout.Size(widths[i], w.th.TabsHeight))
			t.Semantics(a11y.RoleTab, s).A11yState(a11y.Selectable | stateIf(on, a11y.Selected)).A11y(pressSem)
			if on {
				sel = t.ID()
			}
			if act, _, ok := c.A11yAction(t.ID()); ok && act == a11y.Press {
				w.set(i)
				w.snapPane(c.Now())
			}
		}
	})
	b.A11y(func(s *a11y.Node) {
		if s.States.Has(a11y.Focused) {
			s.ActiveDescendant = a11y.NodeID(sel)
		}
	})
}

func (w *TabsW) snapPane(now time.Time) {
	st := w.st
	if w.swipeable && st.pane.Dim > 0 {
		st.pane.SnapTo(float64(*w.active)*st.pane.Dim, now)
		st.gen++
	}
}

func (w *TabsW) panes(pane func(i int)) {
	c, st := w.c, w.st
	n := len(w.titles)
	if !w.swipeable {
		ui.Key(c, *w.active, func() {
			col := ui.Column(c, func() { pane(*w.active) })
			if *w.active >= 0 && *w.active < n {
				col.Semantics(a11y.RoleTabPanel, w.titles[*w.active])
			}
		})
		return
	}
	box := ui.Div(c, scrollX).Clip()
	id := box.ID()
	pw := c.Size().W
	if r, ok := c.RectOf(id); ok && r.W > 0 {
		pw = r.W
	}
	ps := &st.pane
	ps.Min, ps.Max, ps.Dim = 0, float64(max(0, n-1))*float64(pw), float64(pw)
	ps.Snap = float64(pw)
	now := c.Now()
	if ps.Mode == Idle {
		ps.Pos = float64(*w.active) * float64(pw)
	}
	g := Area(c, box, AxisHorizontal, nil)
	sgn := float32(1)
	if w.rtl {
		sgn = -1
	}
	if g.Pan.Began {
		ps.BeginDrag()
	}
	if g.Pan.Active {
		ps.DragBy(-float64(sgn * g.Pan.Delta.X))
	}
	if g.Pan.Ended {
		// Move at most one tab per swipe.
		v := -float64(sgn * g.Pan.Velocity.X)
		cur := float64(*w.active)
		t := cur
		switch {
		case v > SwipeMinVelocity || ps.Pos > (cur+0.5)*float64(pw):
			t = cur + 1
		case v < -SwipeMinVelocity || ps.Pos < (cur-0.5)*float64(pw):
			t = cur - 1
		}
		t = max(0, min(t, float64(n-1)))
		ps.SnapTo(t*float64(pw), now)
		st.gen++
		w.set(int(t))
	}
	if ps.Active() {
		if pump(c, id, st.gen) {
			ps.Step(now)
		} else {
			ps.Settle()
		}
	}
	pos := float32(ps.Pos)
	// The row of panes scrolls by pos from its start. In RTL the layout
	// lays the panes out from the right end of the row (pane i at
	// (n-1-i)·pw), so the row moves the other way from its last pane.
	tx := -pos
	if w.rtl {
		tx = pos - float32(max(0, n-1))*pw
	}
	Within(c, id, func() {
		box.Body(func() {
			ui.Row(c, func() {
				for i := 0; i < n; i++ {
					i := i
					ui.Key(c, i, func() {
						ui.Column(c, func() {
							// Build only the panes near the viewport.
							if abs32(float32(i)*pw-pos) < 2*pw {
								pane(i)
							}
						}, layout.W(pw)).Semantics(a11y.RoleTabPanel, w.titles[i])
					})
				}
			}).Transform(geom.Translate(tx, 0))
		})
	})
}

func abs32(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// ---------------------------------------------------------------------------
// Swiper

type swiperState struct {
	sc     Scroller
	gen    uint64
	nextAt time.Time
	moveTo int // pending direction of a glide: -1, 0, 1
}

// SwiperW is a swipeable carousel of slides with indicator dots.
type SwiperW struct {
	c         *ui.Context
	th        *Theme
	cur       *int
	own       int
	autoplay  time.Duration
	loop      bool
	height    float32
	indicator bool
	vertical  bool
	changed   bool
	radius    float32
	rtl       bool // the next slide comes in from the left
}

// Swiper creates a carousel; *cur is the current slide (nil keeps it
// internally). Set options, then build slides with Slides. In RTL the
// slides run right to left: the next slide comes in from the left, a swipe
// to the right shows it and the indicator starts at the right.
//
// ConfigProvider defaults ("mobile.Swiper"): "autoplay" (time.Duration
// or ms), "loop", "indicator" (bool), "height", "radius" (numbers).
func Swiper(c *ui.Context, cur *int) *SwiperW {
	w := &SwiperW{c: c, th: ThemeOf(c), cur: cur, height: 150, indicator: true, rtl: c.IsRTL()}
	if w.cur == nil {
		w.cur = &w.own
	}
	if v, ok := cfgDur(c, "Swiper", "autoplay"); ok {
		w.autoplay = v
	}
	if v, ok := cfgBool(c, "Swiper", "loop"); ok {
		w.loop = v
	}
	if v, ok := cfgBool(c, "Swiper", "indicator"); ok {
		w.indicator = v
	}
	if v, ok := cfgNum(c, "Swiper", "height"); ok {
		w.height = float32(v)
	}
	if v, ok := cfgNum(c, "Swiper", "radius"); ok {
		w.radius = float32(v)
	}
	return w
}

// Autoplay advances every d.
func (w *SwiperW) Autoplay(d time.Duration) *SwiperW { w.autoplay = d; return w }

// Loop wraps from the last slide to the first.
func (w *SwiperW) Loop() *SwiperW { w.loop = true; return w }

// Height sets the slide height (default 150).
func (w *SwiperW) Height(px float32) *SwiperW { w.height = px; return w }

// Radius rounds the corners.
func (w *SwiperW) Radius(px float32) *SwiperW { w.radius = px; return w }

// Indicator toggles the dots.
func (w *SwiperW) Indicator(on bool) *SwiperW { w.indicator = on; return w }

// Changed reports a slide change this frame.
func (w *SwiperW) Changed() bool { return w.changed }

// Slides builds n slides; slide(i) builds slide i.
func (w *SwiperW) Slides(n int, slide func(i int)) *SwiperW {
	c, th := w.c, w.th
	box := ui.Div(c, layout.H(w.height), scrollX).Clip()
	if w.radius > 0 {
		box.Radius(w.radius)
	}
	id := box.ID()
	st := ui.State[swiperState](c, id, nil)
	if n <= 0 {
		return w
	}
	*w.cur = max(0, min(*w.cur, n-1))
	cur := *w.cur
	pw := c.Size().W
	if r, ok := c.RectOf(id); ok && r.W > 0 {
		pw = r.W
	}
	hasPrev, hasNext := w.loop || cur > 0, w.loop || cur < n-1
	if n == 1 {
		hasPrev, hasNext = false, false
	}
	sc := &st.sc
	sc.Dim, sc.Min, sc.Max = float64(pw), 0, 0
	if hasPrev {
		sc.Min = -float64(pw)
	}
	if hasNext {
		sc.Max = float64(pw)
	}
	now := c.Now()
	g := Area(c, box, AxisHorizontal, nil)
	// Keyboard and screen readers: the carousel is a focus stop; Left /
	// Right (in the visual direction; Increment / Decrement) move one
	// slide.
	ia := box.Focusable(input.CursorDefault)
	// The scroller works in logical units (Pos > 0 moves towards the next
	// slide); sgn turns physical drags into logical ones.
	sgn := float32(1)
	if w.rtl {
		sgn = -1
	}
	box.Semantics(a11y.RoleGroup, "")
	if c.A11yEnabled() {
		box.A11y(stepSem)
	}
	if g.Pan.Began {
		sc.BeginDrag()
	}
	if g.Pan.Active {
		sc.DragBy(-float64(sgn * g.Pan.Delta.X))
	}
	glide := func(dir int) {
		st.moveTo = dir
		sc.SnapTo(float64(dir)*float64(pw), now)
		st.gen++
	}
	if g.Pan.Ended {
		v := -float64(sgn * g.Pan.Velocity.X)
		dir := 0
		switch {
		case (v > SwipeMinVelocity || sc.Pos > float64(pw)/2) && hasNext && sc.Pos > 0:
			dir = 1
		case (v < -SwipeMinVelocity || sc.Pos < -float64(pw)/2) && hasPrev && sc.Pos < 0:
			dir = -1
		}
		glide(dir)
		st.nextAt = time.Time{}
	}
	if sc.Mode == Idle && st.moveTo == 0 {
		dir := 0
		if act, _, ok := c.A11yAction(id); ok {
			switch act {
			case a11y.Increment:
				dir = 1
			case a11y.Decrement:
				dir = -1
			}
			keyPresses(c, id) // the action's synthesized keys
		} else if ia.Focused {
			for _, k := range keyPresses(c, id) {
				switch prev, next := inlineKey(w.rtl, k); {
				case next:
					dir = 1
				case prev:
					dir = -1
				}
			}
		}
		if (dir > 0 && hasNext) || (dir < 0 && hasPrev) {
			glide(dir)
			st.nextAt = time.Time{}
		}
	}
	// Autoplay stands still under reduced motion and while the carousel
	// has keyboard focus.
	if w.autoplay > 0 && sc.Mode == Idle && n > 1 && !c.ReduceMotion() && !ia.Focused {
		if st.nextAt.IsZero() {
			st.nextAt = now.Add(w.autoplay)
		}
		if !now.Before(st.nextAt) && hasNext {
			glide(1)
			st.nextAt = time.Time{}
		} else {
			c.WakeAt(st.nextAt)
		}
	}
	if sc.Active() {
		if pump(c, id, st.gen) {
			sc.Step(now)
		} else {
			sc.Settle()
		}
	}
	if sc.Mode == Idle && st.moveTo != 0 {
		cur = (cur + st.moveTo + n) % n
		*w.cur = cur
		st.moveTo = 0
		sc.Pos = 0
		w.changed = true
		c.Invalidate()
	}
	pos := float32(sc.Pos)
	Within(c, id, func() {
		box.Body(func() {
			for _, k := range []int{-1, 0, 1} {
				i := cur + k
				if w.loop {
					i = (i + n) % n
				}
				if i < 0 || i >= n || (k != 0 && n == 1) || (k == -1 && !hasPrev) || (k == 1 && !hasNext) {
					continue
				}
				x := float32(k)*pw - pos
				if abs32(x) >= pw {
					continue
				}
				x *= sgn // RTL: the next slide is on the left
				k := k
				ui.Key(c, k*1000003+i, func() {
					sl := ui.Div(c, layout.Abs(0, 0, layout.Unset, 0), layout.W(pw)).Transform(geom.Translate(x, 0)).Body(func() { slide(i) })
					if c.A11yEnabled() {
						sl.Semantics(a11y.RoleGroup, c.Locale().Slide(i+1, n))
					}
				})
			}
		})
	})
	ring := ia.FocusVisible
	if (w.indicator && n > 1) || ring {
		dots := w.indicator && n > 1
		box.PaintOver(func(l *paint.List, r geom.Rect) {
			if dots {
				w.dots(l, r, n, cur, pos/pw, spanOf(r, w.rtl))
			}
			if ring {
				drawFocusRing(l, th, r)
			}
		})
	}
	_ = th
	return w
}

// dots draws the indicator, its first dot at the start (sp mirrors it).
func (w *SwiperW) dots(l *paint.List, r geom.Rect, n, cur int, frac float32, sp span) {
	th := w.th
	vant := th.Style == Vant
	if vant {
		d, gap := float32(6), float32(6)
		total := float32(n)*d + float32(n-1)*gap
		x := r.X + (r.W-total)/2
		y := r.Bottom() - 12 - d/2
		for i := 0; i < n; i++ {
			col := fadeCol(th.Border, 0.6)
			if i == cur {
				col = th.Primary
			}
			sp.fill(l, geom.Rect{X: x, Y: y, W: d, H: d}, geom.Rad(d/2), col)
			x += d + gap
		}
		return
	}
	dw, aw, dh, gap := float32(3), float32(13), float32(3), float32(3)
	total := float32(n-1)*dw + aw + float32(n-1)*gap
	x := r.X + (r.W-total)/2
	y := r.Bottom() - 6 - dh
	for i := 0; i < n; i++ {
		wd, col := dw, fadeCol(th.Base.Text, 0.2)
		if i == cur {
			wd, col = aw, th.Primary
		}
		sp.fill(l, geom.Rect{X: x, Y: y, W: wd, H: dh}, geom.Rad(1), col)
		x += wd + gap
	}
}
