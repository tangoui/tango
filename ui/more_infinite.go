package ui

import (
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// InfiniteScroll (Element v-infinite-scroll / Ant List "load more")

// InfiniteScrollW is a scrolling column that asks for more content when the
// user nears its bottom.
type InfiniteScrollW struct {
	c           *Context
	sb          *ScrollBox
	st          *moreInfState
	distance    float32
	loading     bool
	done        bool
	disabled    bool
	loadingText string
	doneText    string
	need        bool
	z           *lazy
}

type moreInfState struct {
	firedH float64 // content height when NeedMore last fired (-1: re-armed)
}

// InfiniteScroll builds fn (the items) inside a vertical scroll area. Give
// it a bounded height with Height or opts (layout.H / MaxH / Grow), then
// poll NeedMore:
//
//	inf := ui.InfiniteScroll(c, func() { for _, it := range items { ... } }).
//		Height(400).Loading(loading).Done(len(items) >= total)
//	if inf.NeedMore() { loading = true; go fetch() }
func InfiniteScroll(c *Context, fn func(), opts ...layout.Option) *InfiniteScrollW {
	w := &InfiniteScrollW{c: c, distance: 0, loadingText: c.Locale().Loading + "...", doneText: c.Locale().NoMore}
	w.sb = Scroll(c, fn, opts...)
	w.st = State[moreInfState](c, hashID(w.sb.n.id, "more-inf"), func() moreInfState { return moreInfState{firedH: -1} })
	w.z = c.lazy(w.sb.n, w.finish)
	return w
}

// Height fixes the viewport height.
func (w *InfiniteScrollW) Height(px float32) *InfiniteScrollW { w.sb.n.ln.Height = px; return w }

// Layout applies layout options to the viewport.
func (w *InfiniteScrollW) Layout(opts ...layout.Option) *InfiniteScrollW {
	w.sb.n.ln.Style.Apply(opts...)
	return w
}

// Distance is how close (px) to the bottom NeedMore starts firing
// (Element infinite-scroll-distance; default 0).
func (w *InfiniteScrollW) Distance(px float32) *InfiniteScrollW { w.distance = px; return w }

// Loading shows the loading footer and suppresses NeedMore.
func (w *InfiniteScrollW) Loading(on bool) *InfiniteScrollW { w.loading = on; return w }

// Done shows the "no more" footer and stops NeedMore.
func (w *InfiniteScrollW) Done(on bool) *InfiniteScrollW { w.done = on; return w }

// Disabled stops NeedMore without showing a footer.
func (w *InfiniteScrollW) Disabled(on bool) *InfiniteScrollW { w.disabled = on; return w }

// LoadingText / DoneText replace the footer texts ("" hides the text).
func (w *InfiniteScrollW) LoadingText(s string) *InfiniteScrollW { w.loadingText = s; return w }
func (w *InfiniteScrollW) DoneText(s string) *InfiniteScrollW    { w.doneText = s; return w }

// ScrollTo sets the vertical scroll offset.
func (w *InfiniteScrollW) ScrollTo(y float64) *InfiniteScrollW {
	w.sb.s.off.Y = y
	return w
}

// Offset returns the vertical scroll offset.
func (w *InfiniteScrollW) Offset() float64 { return w.sb.s.off.Y }

// NeedMore reports, once per content height, that the viewport is within
// Distance of the bottom (or not yet filled) and more items should load.
func (w *InfiniteScrollW) NeedMore() bool { w.z.run(); return w.need }

// ID returns the scroll container's ID.
func (w *InfiniteScrollW) ID() ID { return w.sb.n.id }

func (w *InfiniteScrollW) finish() {
	c := w.c
	if w.loading || w.done {
		w.footer()
	}
	if w.loading {
		w.sb.n.setRole(a11y.RoleGroup).setState(a11y.Busy)
	}
	s := w.sb.s
	if s.view.H <= 0 {
		// Not laid out yet: ask for a frame so the check runs with sizes.
		if !w.loading && !w.done && !w.disabled && !w.sb.n.disabled {
			c.WakeAt(c.now)
		}
		return
	}
	rest := s.content.Y - float64(s.view.H) - s.off.Y
	near := rest <= float64(w.distance)+0.5
	if !near {
		w.st.firedH = -1 // re-arm after the user scrolls away
		return
	}
	if w.loading || w.done || w.disabled || w.sb.n.disabled {
		return
	}
	if w.st.firedH == s.content.Y {
		return
	}
	w.st.firedH = s.content.Y
	w.need = true
	// Re-check after the new content is laid out (fills short viewports).
	c.WakeAt(c.now)
}

func (w *InfiniteScrollW) footer() {
	c, th := w.c, w.c.Theme
	el := th.Flavor == theme.Element
	if w.loading {
		row := Row(c, func() {
			Spin(c).IndicatorSize(16).n.aHidden = true // the row is the status
			if w.loadingText != "" {
				Text(c, w.loadingText).Color(th.TextTertiary).NoWrap().n.aHidden = true
			}
		}, layout.Gap(8), layout.Center(), layout.H(48))
		name := w.loadingText
		if name == "" {
			name = c.Locale().A11y.Loading
		}
		row.n.setRole(a11y.RoleStatus).setName(name).setState(a11y.Busy)
		return
	}
	label := w.doneText
	n := c.add("more-inf-done", label)
	if label != "" {
		n.setRole(a11y.RoleText)
	}
	n.ln.Height = 48
	st := c.style(th.FontSize, text.Regular)
	n.paint = func(l *paint.List, r geom.Rect) {
		if label == "" {
			return
		}
		lay := text.Shape(label, st, c.textOpts(text.Options{NoWrap: true}))
		tx := r.X + (r.W-lay.Width)/2
		cy := r.Y + r.H/2
		l.Glyphs(geom.Pt(tx, cy-lay.Height/2), lay.Glyphs, th.TextTertiary)
		if el {
			return
		}
		// Ant List demos end with a plain divider around the text.
		gap := float32(16)
		left := tx - gap - r.X - 24
		if left > 0 {
			l.Fill(geom.Rect{X: r.X + 24, Y: cy - 0.5, W: left, H: 1}, geom.Radii{}, th.Split)
			l.Fill(geom.Rect{X: tx + lay.Width + gap, Y: cy - 0.5, W: left, H: 1}, geom.Radii{}, th.Split)
		}
	}
}
