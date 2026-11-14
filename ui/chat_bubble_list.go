package ui

import (
	"math"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// BubbleList (Ant Design X Bubble.List)

// blEstimate is the height assumed for a message never laid out.
const blEstimate = 80

type blBuilt struct {
	key string
	id  ID
}

type blState struct {
	h         map[string]float32 // measured heights (gap included) by key
	sum       float64            // of all measured heights
	init      bool
	follow    bool // stick to the bottom as content grows
	lastOff   float64
	anchorKey string  // first visible message while not following
	anchorOff float64 // viewport top minus that message's top
	built     []blBuilt
	wasTop    bool
}

// BubbleListW is a scrolling message stream that only builds the messages
// near the viewport (heights are measured and cached by key), keeps the
// bottom in view while new content streams in, and stops following when
// the user scrolls up, showing a "back to bottom" button instead.
type BubbleListW struct {
	_ [0]func()

	c          *Context
	outer      *Box
	sb         *ScrollBox
	st         *blState
	z          *lazy
	loc        *Locale
	n          int
	key        func(i int) string
	item       func(i int)
	gap, pad   float32
	overscan   float32
	noFollow   bool
	toBottom   bool
	topReached bool
	first, end int
}

// BubbleList adds a message list of n items; key(i) must be a stable,
// unique key of message i (heights and widget state are kept by key) and
// item(i) builds it (usually a Bubble). Give the list a bounded height
// (Grow inside a sized parent, or layout.H).
//
//	ui.BubbleList(c, len(msgs), func(i int) string { return msgs[i].ID }, func(i int) {
//		ui.Bubble(c, msgs[i].Role, msgs[i].Text)
//	}, layout.Grow(1))
func BubbleList(c *Context, n int, key func(i int) string, item func(i int), opts ...layout.Option) *BubbleListW {
	w := &BubbleListW{c: c, n: max(0, n), key: key, item: item, gap: 8, pad: 16, overscan: 600, loc: c.Locale()}
	w.outer = c.box("bubble-list", layout.Column, opts)
	w.outer.n.ln.MinH = 0
	w.st = State[blState](c, w.outer.n.id, nil)
	if !w.st.init {
		w.st.init, w.st.follow = true, true
		w.st.h = map[string]float32{}
	}
	c.body(w.outer.n, func() {
		b := c.box("bubble-scroll", layout.Column, []layout.Option{layout.Grow(1), layout.Basis(0), layout.MinH(0)})
		b.n.ln.ScrollY = true
		b.n.setRole(a11y.RoleList).setName(w.loc.Chat.Messages).fill(w)
		w.sb = c.finishScroll(b, nil)
	})
	w.z = c.lazy(w.sb.n, w.build)
	return w
}

func (w *BubbleListW) semantics(s *a11y.Node) { s.RowCount = w.n }

// A11yLabel sets the accessible name (default Locale.Chat.Messages).
func (w *BubbleListW) A11yLabel(s string) *BubbleListW { w.sb.n.setName(s); return w }

// Gap sets the space between messages (default 8; the action row under
// a message reserves its own space).
func (w *BubbleListW) Gap(px float32) *BubbleListW { w.gap = max(0, px); return w }

// Padding sets the space around the messages (default 16).
func (w *BubbleListW) Padding(px float32) *BubbleListW { w.pad = max(0, px); return w }

// Overscan sets how far beyond the viewport messages are built, in px
// (default 600).
func (w *BubbleListW) Overscan(px float32) *BubbleListW { w.overscan = max(0, px); return w }

// NoAutoScroll stops following new content at the bottom.
func (w *BubbleListW) NoAutoScroll() *BubbleListW { w.noFollow = true; return w }

// ScrollToBottom jumps to the end and resumes following new content.
func (w *BubbleListW) ScrollToBottom() *BubbleListW { w.toBottom = true; return w }

// Following reports whether the list sticks to the bottom.
func (w *BubbleListW) Following() bool { w.z.run(); return w.st.follow && !w.noFollow }

// TopReached reports the frame the user scrolled to the very top (load
// earlier messages then; the view keeps its place when they are
// prepended).
func (w *BubbleListW) TopReached() bool { w.z.run(); return w.topReached }

// Range returns the half-open range of messages built this frame.
func (w *BubbleListW) Range() (first, end int) { w.z.run(); return w.first, w.end }

// ID returns the scroll container's ID.
func (w *BubbleListW) ID() ID { return w.sb.n.id }

func (w *BubbleListW) viewH() float64 {
	if h := w.sb.s.view.H; h > 0 {
		return float64(h)
	}
	if r, ok := w.c.rects[w.outer.n.id]; ok && r.H > 0 {
		return float64(r.H)
	}
	return float64(w.c.size.H)
}

func (w *BubbleListW) estimate() float32 {
	if k := len(w.st.h); k > 0 {
		return float32(w.st.sum / float64(k))
	}
	return blEstimate
}

func (w *BubbleListW) build() {
	c, st, s := w.c, w.st, w.sb.s
	// Heights of the messages laid out last frame.
	changed := false
	for _, b := range st.built {
		r, ok := c.rects[b.id]
		if !ok {
			continue
		}
		if old, had := st.h[b.key]; !had || math.Abs(float64(old-r.H)) > 0.5 {
			st.sum += float64(r.H - old)
			st.h[b.key] = r.H
			changed = true
		}
	}
	if len(st.h) > 4*max(w.n, 64) { // drop keys of deleted messages
		live := make(map[string]float32, w.n)
		st.sum = 0
		for i := range w.n {
			if h, ok := st.h[w.key(i)]; ok {
				live[w.key(i)] = h
				st.sum += float64(h)
			}
		}
		st.h = live
	}
	est := w.estimate()
	keys := make([]string, w.n)
	tops := make([]float64, w.n+1)
	tops[0] = float64(w.pad) // top padding of the list
	anchor := -1
	for i := range w.n {
		k := w.key(i)
		keys[i] = k
		h, ok := st.h[k]
		if !ok {
			h = est
		}
		tops[i+1] = tops[i] + float64(h)
		if k == st.anchorKey {
			anchor = i
		}
	}
	total := tops[w.n] + float64(w.pad)
	view := w.viewH()
	maxOff := max(0, total-view)

	off := s.off.Y
	moved := math.Abs(off-st.lastOff) > 0.5 // wheel, scrollbar, keyboard, touch
	switch {
	case w.toBottom:
		st.follow = true
	case moved && off < st.lastOff:
		st.follow = false
	case moved && off >= maxOff-2:
		st.follow = true
	}
	if w.noFollow {
		st.follow = false
	}
	switch {
	case st.follow:
		off = maxOff
	case !moved && anchor >= 0:
		off = tops[anchor] + st.anchorOff // keep the view on its message
	}
	off = max(0, min(off, maxOff))
	if off != s.off.Y {
		s.off.Y = off
	}
	st.lastOff = off
	w.topReached = off <= 1 && !st.wasTop && total > view
	st.wasTop = off <= 1

	// The first message starting at or below the viewport top anchors the
	// view: its top is exact even when the messages above it are only
	// estimated (prepended history), so re-measuring them cannot move it.
	if a := upperIndex(tops[:w.n], off-0.5); a < w.n {
		st.anchorKey, st.anchorOff = keys[a], off-tops[a]
	} else if w.n > 0 {
		st.anchorKey, st.anchorOff = keys[w.n-1], off-tops[w.n-1]
	}
	lo, hi := off-float64(w.overscan), off+view+float64(w.overscan)
	w.first = max(0, upperIndex(tops, lo)-1)
	w.end = min(w.n, upperIndex(tops, hi))
	s.base.Y = tops[w.first]
	if w.first == 0 {
		s.base.Y = 0
		Gap(c, w.pad)
	}
	s.extent.Y = total
	st.built = st.built[:0]
	for i := w.first; i < w.end; i++ {
		Key(c, keys[i], func() {
			// Every message carries the gap below it, so its measured
			// height does not change when messages are added around it.
			in := geom.Insets{Left: w.pad, Right: w.pad, Bottom: w.gap}
			row := Column(c, func() { w.item(i) }, layout.Padding(in))
			row.n.setRole(a11y.RoleListItem)
			if c.a11yOn {
				pos, size := i+1, w.n
				row.n.fill(semFunc(func(s *a11y.Node) { s.PosInSet, s.SetSize = pos, size }))
			}
			st.built = append(st.built, blBuilt{keys[i], row.n.id})
		})
	}
	if changed {
		c.animating = true // settle the new heights next frame
	}
	if !st.follow && total-view-off > 1 {
		c.body(w.outer.n, w.bottomButton)
	}
}

// upperIndex returns the first i with tops[i] > y (len(tops) if none).
func upperIndex(tops []float64, y float64) int {
	lo, hi := 0, len(tops)
	for lo < hi {
		m := (lo + hi) / 2
		if tops[m] > y {
			hi = m
		} else {
			lo = m + 1
		}
	}
	return lo
}

// bottomButton is the floating "back to bottom" button over the list end.
func (w *BubbleListW) bottomButton() {
	c := w.c
	th := c.ThemeFor(theme.CompChat)
	tk := &th.Components.Chat
	Row(c, func() {
		Tooltip(c, w.loc.Chat.ScrollToBottom, func() {
			const size = 32
			b := Row(c, nil, layout.Size(size, size))
			b.Bg(tk.ScrollButtonBg).Radius(size / 2).Shadow(tk.ScrollButtonShadow)
			if tk.ScrollButtonBorder.A > 0 {
				b.Border(1, tk.ScrollButtonBorder)
			}
			b.n.setRole(a11y.RoleButton).setName(w.loc.Chat.ScrollToBottom)
			ia := c.interact(b.n, true, input.CursorPointer)
			b.Opacity(c.AnimFrom(b.n.id, "in", 0, 1, th.DurationMid))
			b.n.over = func(l *paint.List, r geom.Rect) {
				col := th.TextSecondary
				if ia.Hovered {
					col = th.Text
				}
				c.paintIcon(l, icon.ArrowDown, geom.Rect{X: r.X + 8, Y: r.Y + 8, W: 16, H: 16}, col, 0)
				if ia.FocusVisible {
					drawFocusRing(l, th, r, geom.Rad(size/2))
				}
			}
			if ia.Clicked {
				w.st.follow = true
				c.invalid = true
			}
		})
	}, layout.Abs(0, layout.Unset, 0, 12), layout.Center())
}
