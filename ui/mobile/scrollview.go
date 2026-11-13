package mobile

import (
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
)

// ---------------------------------------------------------------------------
// Pull-to-refresh state machine

// RefreshStatus is the phase of a pull-to-refresh header.
type RefreshStatus uint8

const (
	RefreshIdle       RefreshStatus = iota
	RefreshPulling                  // dragged down, below the threshold
	RefreshCanRelease               // past the threshold: releasing refreshes
	RefreshRefreshing               // loading; the header stays open
	RefreshComplete                 // finished; shows the success text briefly
)

// RefreshMachine is the pull-to-refresh state machine (pure).
type RefreshMachine struct {
	Status    RefreshStatus
	Threshold float64       // pull distance that arms the refresh
	Delay     time.Duration // how long RefreshComplete stays
	doneAt    time.Time
}

// Pull updates the status for a drag at distance d (displayed px).
func (m *RefreshMachine) Pull(d float64) {
	switch m.Status {
	case RefreshIdle, RefreshPulling, RefreshCanRelease:
		switch {
		case d <= 0:
			m.Status = RefreshIdle
		case d >= m.Threshold:
			m.Status = RefreshCanRelease
		default:
			m.Status = RefreshPulling
		}
	}
}

// Release ends the drag; it reports whether a refresh starts.
func (m *RefreshMachine) Release() bool {
	switch m.Status {
	case RefreshCanRelease:
		m.Status = RefreshRefreshing
		return true
	case RefreshPulling:
		m.Status = RefreshIdle
	}
	return false
}

// Start enters the refreshing state programmatically.
func (m *RefreshMachine) Start() { m.Status = RefreshRefreshing }

// Finish ends a refresh at now.
func (m *RefreshMachine) Finish(now time.Time) {
	if m.Status == RefreshRefreshing {
		m.Status, m.doneAt = RefreshComplete, now
		if m.Delay <= 0 {
			m.Status = RefreshIdle
		}
	}
}

// Tick advances timed transitions; it returns the time of the next one.
func (m *RefreshMachine) Tick(now time.Time) (next time.Time) {
	if m.Status == RefreshComplete {
		end := m.doneAt.Add(m.Delay)
		if !now.Before(end) {
			m.Status = RefreshIdle
			return time.Time{}
		}
		return end
	}
	return time.Time{}
}

// Holding reports whether the header is held open.
func (m *RefreshMachine) Holding() bool {
	return m.Status == RefreshRefreshing || m.Status == RefreshComplete
}

// ---------------------------------------------------------------------------
// ScrollView

type svState struct {
	sc       Scroller
	gen      uint64
	contentH float64
	viewH    float64
	rm       RefreshMachine
	loadSeen bool
	wasHold  bool
	focus    ui.ID // keyboard focus seen last frame (scrolled into view once)
	content  ui.ID
}

// ScrollViewW is a touch scroll container: finger tracking, inertial
// fling, rubber-band edges and bounce back, with optional pull-to-refresh
// header and infinite-load footer.
type ScrollViewW struct {
	c       *ui.Context
	th      *Theme
	outer   *ui.Box
	content *ui.Box
	st      *svState

	refreshing *bool
	refreshed  bool
	pullTexts  [4]string

	loading  *bool
	finished bool
	failed   *bool
	loadMore bool
	offset   float32
	rtl      bool // the spinners lead the status texts from the right

	g Gesture
}

// ScrollView creates a vertical touch scroll container. Give it a bounded
// height (Layout(layout.Grow(1)) in a sized column, or layout.H), set
// options, then build the content with Body.
func ScrollView(c *ui.Context) *ScrollViewW {
	w := &ScrollViewW{c: c, th: ThemeOf(c), offset: 50, rtl: c.IsRTL()}
	w.outer = ui.Div(c, scrollY).Clip().A11yScroller()
	w.st = ui.State[svState](c, w.outer.ID(), nil)
	return w
}

// PullToRefresh creates a ScrollView with a pull-down refresh header.
// Dragging past the threshold and releasing sets *refreshing = true and
// Refreshed reports it; set it back to false when the data has loaded.
func PullToRefresh(c *ui.Context, refreshing *bool) *ScrollViewW {
	return ScrollView(c).Pull(refreshing)
}

// InfiniteList creates a ScrollView that loads more when scrolled near the
// end: it sets *loading = true and LoadMore reports it. Set *loading back
// to false after appending rows; call Finished(true) when nothing is left.
func InfiniteList(c *ui.Context, loading *bool) *ScrollViewW {
	return ScrollView(c).Infinite(loading)
}

// Layout applies layout options to the container.
func (w *ScrollViewW) Layout(opts ...layout.Option) *ScrollViewW { w.outer.Style(opts...); return w }

// Bg sets the background color.
func (w *ScrollViewW) Bg(col paintColor) *ScrollViewW { w.outer.Bg(col); return w }

// Pull enables the pull-to-refresh header bound to *refreshing.
func (w *ScrollViewW) Pull(refreshing *bool) *ScrollViewW { w.refreshing = refreshing; return w }

// PullTexts overrides the header texts (pulling, can release, refreshing,
// complete); empty entries keep the localized defaults.
func (w *ScrollViewW) PullTexts(pulling, release, refreshing, complete string) *ScrollViewW {
	w.pullTexts = [4]string{pulling, release, refreshing, complete}
	return w
}

// Infinite enables the load-more footer bound to *loading.
func (w *ScrollViewW) Infinite(loading *bool) *ScrollViewW { w.loading = loading; return w }

// Finished marks the list complete (footer shows "no more").
func (w *ScrollViewW) Finished(on bool) *ScrollViewW { w.finished = on; return w }

// Failed binds a load error flag: the footer offers a retry tap.
func (w *ScrollViewW) Failed(failed *bool) *ScrollViewW { w.failed = failed; return w }

// Offset sets how close (px) to the end loading starts (default 50).
func (w *ScrollViewW) Offset(px float32) *ScrollViewW { w.offset = px; return w }

func (w *ScrollViewW) headHeight() float64 {
	if w.th.Style == Vant {
		return 50
	}
	return 40
}

// Body builds the scrolling content.
func (w *ScrollViewW) Body(fn func()) *ScrollViewW {
	c, st := w.c, w.st
	id := w.outer.ID()
	now := c.Now()
	sc := &st.sc
	head := w.headHeight()
	st.rm.Threshold = head + 20
	if w.th.Style == Vant {
		st.rm.Threshold = head
	}
	st.rm.Delay = 500 * time.Millisecond

	accept := func(Direction) bool { return true }
	w.g = Area(c, w.outer, AxisVertical, accept)
	g := w.g

	// Range from the previous frame's geometry. While the soft keyboard
	// covers the bottom of the view the content may scroll that much
	// further, so its end can be lifted above the keyboard.
	kbPad := float64(0)
	if r, ok := c.RectOf(id); ok {
		st.viewH = float64(r.H)
		if v := c.KeyboardCover(r); v > 0 {
			kbPad = float64(v) + 8 // and the gap kept above the keyboard
		}
	}
	sc.Dim = st.viewH
	sc.Max = max(0, st.contentH-st.viewH+kbPad)
	sc.Min = 0

	// Pull-to-refresh: programmatic start / finish.
	if w.refreshing != nil {
		if *w.refreshing && st.rm.Status != RefreshRefreshing && sc.Mode != Dragging {
			st.rm.Start()
			if sc.Pos > -head {
				sc.SnapTo(-head, now)
				st.gen++
			}
		}
		if !*w.refreshing && st.rm.Status == RefreshRefreshing {
			st.rm.Finish(now)
		}
		if next := st.rm.Tick(now); !next.IsZero() {
			c.WakeAt(next)
		}
		if st.rm.Holding() {
			sc.Min = -head
		}
	}

	// Finger.
	if g.TouchStart && sc.Active() {
		sc.Stop()
	}
	if g.Pan.Began {
		sc.BeginDrag()
	}
	if g.Pan.Active {
		sc.DragBy(-float64(g.Pan.Delta.Y))
		if w.refreshing != nil && !st.rm.Holding() {
			st.rm.Pull(-sc.Pos)
		}
	}
	if g.Pan.Ended {
		if w.refreshing != nil && st.rm.Release() {
			*w.refreshing = true
			w.refreshed = true
			sc.Min = -head
		}
		sc.Release(-float64(g.Pan.Velocity.Y), now)
		st.gen++
	}
	// Wheel / trackpad (desktop testing).
	if _, dy, ok := c.WheelFor(id); ok && sc.Mode != Dragging {
		sc.Stop()
		sc.Pos = max(sc.Min, min(sc.Pos+float64(dy), sc.Max))
	}
	// Keyboard focus moved to a widget inside the content (a field, a
	// switch): scroll it into view.
	if f := c.Focused(); f != st.focus {
		st.focus = f
		fr, ok1 := c.RectOf(f)
		cr, ok2 := c.RectOf(st.content)
		vr, ok3 := c.RectOf(id)
		if f != 0 && ok1 && ok2 && ok3 && fr.X >= cr.X-0.5 && fr.Right() <= cr.Right()+0.5 && fr.Y >= cr.Y-0.5 && fr.Bottom() <= cr.Bottom()+0.5 {
			// Keep a margin of about one row past the focused widget: content
			// outside the viewport is not painted, so the next widget only
			// becomes a Tab stop once it peeks in.
			m := min(max(2*fr.H, 48), vr.H/3)
			d := 0.0
			if fr.Bottom()+m > vr.Bottom() {
				d = float64(fr.Bottom() + m - vr.Bottom())
			}
			if fr.Y-m-float32(d) < vr.Y {
				d = float64(fr.Y - m - vr.Y)
			}
			if d != 0 && sc.Mode != Dragging {
				sc.Stop()
				sc.Pos = max(sc.Min, min(sc.Pos+d, sc.Max))
				c.Invalidate()
			}
		}
	}
	// The soft keyboard came up over the focused field (or focus moved
	// under it): lift the field above the keyboard.
	if k, ok := c.PendingKeyboardReveal(); ok && sc.Mode != Dragging {
		fr, ok1 := c.RectOf(k.Field)
		cr, ok2 := c.RectOf(st.content)
		vr, ok3 := c.RectOf(id)
		if ok1 && ok2 && ok3 && fr.Y >= cr.Y-0.5 && fr.Bottom() <= cr.Bottom()+0.5 {
			top := max(vr.Y, SafeAreaInsets(c).Top)
			bottom := min(vr.Bottom(), k.Limit)
			d := revealDelta(k.Target(top), top, bottom)
			sc.Stop()
			sc.Pos = max(sc.Min, min(sc.Pos+d, sc.Max))
			c.ClaimKeyboardReveal()
			c.Invalidate()
		}
	}
	// A screen reader asked to show a widget of the content (moving its
	// cursor to a row scrolled out of view): bring it fully into the part
	// of the view not covered by system bars or the soft keyboard.
	if t, ok := c.A11yScrollRequest(id); ok && sc.Mode != Dragging {
		tr, ok1 := c.RectOf(t)
		vr, ok2 := c.RectOf(id)
		in := SafeAreaInsets(c)
		top := max(vr.Y, in.Top)
		bottom := min(vr.Bottom(), c.Size().H-max(in.Bottom, c.KeyboardInset().Bottom))
		if ok1 && ok2 && bottom-top >= 1 {
			if d := revealDelta(tr, top, bottom); d != 0 {
				sc.Stop()
				sc.Pos = max(sc.Min, min(sc.Pos+d, sc.Max))
				c.Invalidate()
			}
		}
	}
	// Out of range after a content or header change: bounce back.
	if sc.Mode == Idle && (sc.Pos < sc.Min-0.5 || sc.Pos > sc.Max+0.5) {
		sc.Release(0, now)
		st.gen++
	}
	if sc.Active() {
		if pump(c, id, st.gen) {
			sc.Step(now)
		} else {
			sc.Settle()
		}
	}
	if w.refreshing != nil && !st.rm.Holding() && sc.Mode != Dragging && sc.Pos >= 0 {
		st.rm.Pull(0)
	}

	// Infinite loading.
	if w.loading != nil && !*w.loading && !w.finished && (w.failed == nil || !*w.failed) && st.contentH > 0 {
		if sc.Pos+st.viewH >= st.contentH-float64(w.offset) {
			*w.loading = true
			w.loadMore = true
		}
	}

	// Paint the refresh header in the space revealed above the content.
	if w.refreshing != nil {
		texts := w.texts()
		ang := 0.0
		if st.rm.Status == RefreshRefreshing && sc.Pos < 0 {
			ang = spinAngle(c, id)
		}
		w.outer.Paint(func(l *paint.List, r geom.Rect) {
			w.paintHeader(l, r, texts, ang)
		})
	}

	pos := float32(sc.Pos)
	Within(c, id, func() {
		w.outer.Body(func() {
			w.content = ui.Div(c).Transform(geom.Translate(0, -pos))
			st.content = w.content.ID()
			w.content.Body(func() {
				fn()
				if w.loading != nil {
					w.footer()
				}
			})
			if w.refreshing != nil && c.A11yEnabled() {
				// The pull-to-refresh header text as a polite live region.
				s := ""
				if st.rm.Status != RefreshIdle {
					s = w.texts()[min(int(st.rm.Status)-1, 3)]
				}
				ui.Div(c, layout.Abs(0, 0, 0, layout.Unset), layout.H(float32(head))).
					Semantics(a11y.RoleStatus, s).A11yState(stateIf(st.rm.Status == RefreshRefreshing, a11y.Busy))
			}
		})
	})
	if r, ok := c.RectOf(w.content.ID()); ok {
		st.contentH = float64(r.H)
	}
	return w
}

// revealDelta returns how far to scroll (positive: content moves up) so
// that window rect t lies between window ys top and bottom: the least
// amount, or aligned to top when t is taller than that span.
func revealDelta(t geom.Rect, top, bottom float32) float64 {
	switch {
	case t.H > bottom-top || t.Y < top:
		return float64(t.Y - top)
	case t.Bottom() > bottom:
		return float64(t.Bottom() - bottom)
	}
	return 0
}

func (w *ScrollViewW) texts() [4]string {
	c := w.c
	d := [4]string{
		c.Locale().Mobile.PullToRefresh,
		c.Locale().Mobile.ReleaseToRefresh,
		c.Locale().Loading,
		c.Locale().Mobile.Refreshed,
	}
	for i, s := range w.pullTexts {
		if s != "" {
			d[i] = s
		}
	}
	return d
}

func (w *ScrollViewW) paintHeader(l *paint.List, r geom.Rect, texts [4]string, ang float64) {
	th, st := w.th, w.st
	shown := float32(-st.sc.Pos)
	if shown <= 0.5 {
		return
	}
	head := float32(w.headHeight())
	hr := geom.Rect{X: r.X, Y: r.Y + shown - head, W: r.W, H: head}
	if shown > head {
		hr.Y = r.Y + (shown-head)/2
	}
	var s string
	switch st.rm.Status {
	case RefreshPulling, RefreshIdle:
		s = texts[0]
	case RefreshCanRelease:
		s = texts[1]
	case RefreshRefreshing:
		s = texts[2]
	case RefreshComplete:
		s = texts[3]
	}
	fs := th.Font(th.FontMain, text.Regular)
	if th.Style == Vant {
		fs = th.Font(14, text.Regular)
	}
	sp := spanOf(hr, w.rtl)
	tw := textW(s, fs)
	x := hr.X + (hr.W-tw)/2
	if st.rm.Status == RefreshRefreshing {
		sz := float32(16)
		x += (sz + 6) / 2
		drawSpinner(l, sp.P(x-6-sz/2), hr.Y+hr.H/2, sz/2, ang, th.TextWeak)
	}
	sp.textMid(l, s, fs, x, hr, th.TextWeak)
}

// footer builds the infinite-load footer.
func (w *ScrollViewW) footer() {
	c, th := w.c, w.th
	loc := c.Locale()
	var s string
	spin := false
	switch {
	case w.failed != nil && *w.failed:
		s = loc.LoadFailed + loc.Mobile.TapToRetry
	case w.finished:
		s = loc.NoMore
	default:
		s = loc.Loading
		spin = *w.loading
	}
	fs := th.Font(th.FontMain, text.Regular)
	if th.Style == Vant {
		fs = th.Font(14, text.Regular)
	}
	ang := 0.0
	b := ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: 50} }, nop)
	if spin {
		ang = spinAngle(c, b.ID())
	}
	// A retry button after a failure, else a polite status line.
	if w.failed != nil && *w.failed {
		b.Semantics(a11y.RoleButton, s)
	} else {
		b.Semantics(a11y.RoleStatus, s).A11yState(stateIf(spin, a11y.Busy))
	}
	pr := Touchable(c, b)
	if pr.Tap && w.failed != nil && *w.failed {
		*w.failed = false
		*w.loading = true
		w.loadMore = true
	}
	rtl := w.rtl
	b.Paint(func(l *paint.List, r geom.Rect) {
		sp := spanOf(r, rtl)
		tw := textW(s, fs)
		x := r.X + (r.W-tw)/2
		if spin {
			x += 11
			drawSpinner(l, sp.P(x-14), r.Y+r.H/2, 8, ang, th.TextWeak)
		}
		sp.textMid(l, s, fs, x, r, th.TextWeak)
	})
}

// rowsOverscan is how many rows Rows builds beyond each edge of the view,
// so a fling's first frames and keyboard focus moves find a row built.
const rowsOverscan = 4

// Rows builds a long list of n rows of a fixed height rowH inside Body,
// creating only the rows in or near the visible part of the view: the
// rest is a blank of the right height, so the scroll range, the fling and
// the infinite-load trigger behave as if every row were there. row(i)
// builds row i (keyed by i, so its state follows the index); keep the
// row's content within rowH. Other content may come before or after the
// rows in the same Body.
//
//	sv := mobile.ScrollView(c).Layout(layout.Grow(1))
//	sv.Body(func() {
//		mobile.List(c, "").Body(func() {
//			sv.Rows(len(items), 50, func(i int) { mobile.Cell(c, items[i]) })
//		})
//	})
//
// Rows that are not built are not in the accessibility tree; a screen
// reader reaches them by scrolling. For variable row heights or a desktop
// scroll bar use ui.VirtualList inside the page instead.
func (w *ScrollViewW) Rows(n int, rowH float32, row func(i int)) {
	c, st := w.c, w.st
	if n <= 0 || rowH <= 0 {
		return
	}
	blk := ui.Div(c, layout.H(float32(n)*rowH), layout.Shrink(0))
	// Where the block starts in the content, from the last frame (both
	// boxes move together, so the scroll translation cancels out).
	top := float32(0)
	if br, ok := c.RectOf(blk.ID()); ok {
		if cr, ok := c.RectOf(st.content); ok {
			top = br.Y - cr.Y
		}
	}
	view := float32(st.viewH)
	if view <= 0 {
		view = c.Size().H
	}
	pos := float32(st.sc.Pos)
	first := max(0, int((pos-top)/rowH)-rowsOverscan)
	last := min(n, int((pos+view-top)/rowH)+1+rowsOverscan)
	if first >= last { // scrolled well past the block (or before it)
		first, last = 0, 0
	}
	blk.Body(func() {
		if first > 0 {
			ui.Div(c, layout.H(float32(first)*rowH), layout.Shrink(0))
		}
		for i := first; i < last; i++ {
			ui.Key(c, i, func() {
				ui.Div(c, layout.H(rowH), layout.Shrink(0)).Body(func() { row(i) })
			})
		}
	})
}

// Refreshed reports that the user pulled to refresh this frame.
func (w *ScrollViewW) Refreshed() bool { return w.refreshed }

// LoadMore reports that the list asked for more rows this frame.
func (w *ScrollViewW) LoadMore() bool { return w.loadMore }

// Status returns the pull-to-refresh status.
func (w *ScrollViewW) Status() RefreshStatus { return w.st.rm.Status }

// Offset returns the scroll offset (negative while pulled down).
func (w *ScrollViewW) ScrollOffset() float64 { return w.st.sc.Pos }

// ScrollTo jumps to offset y (clamped).
func (w *ScrollViewW) ScrollTo(y float64) *ScrollViewW {
	sc := &w.st.sc
	sc.Mode = Idle
	sc.Pos = max(0, min(y, max(0, w.st.contentH-w.st.viewH)))
	w.c.Invalidate()
	return w
}

// Scrolling reports whether the view is being dragged or still moving.
func (w *ScrollViewW) Scrolling() bool { return w.st.sc.Mode != Idle }

// ID returns the container's ID.
func (w *ScrollViewW) ID() ui.ID { return w.outer.ID() }

// ContentID returns the ID of the moving content box.
func (w *ScrollViewW) ContentID() ui.ID {
	if w.content == nil {
		return w.st.content // before Body: the box of the last frame
	}
	return w.content.ID()
}
