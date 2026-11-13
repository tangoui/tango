package ui

import (
	"math"
	"time"

	"github.com/tangoui/tango/ui/a11y"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// Carousel

// CarouselW is a slideshow: scroll or fade transitions, autoplay with
// pause on hover, indicator dots/lines and arrows.
type CarouselW struct {
	c        *Context
	n        int
	slide    func(i int)
	cur      *int
	autoplay time.Duration
	fade     bool
	dots     bool
	dotPos   string // bottom | top | left | right | outside (Element)
	arrows   string // always | hover | never
	height   float32
	vertical bool
	changed  bool
	speed    time.Duration
	card     bool // Element type="card"
	noLoop   bool
	drag     bool
	swipe    float32 // fraction of the slide size that commits a swipe
	th       *theme.Theme
	tk       *theme.CarouselToken
}

type carouselState struct {
	seq     int // unbounded slide counter: index = seq mod n
	since   time.Time
	started bool
	cur     int
	focusIn bool // keyboard focus was inside the carousel last frame

	// Pointer / touch swipe (Draggable).
	dragging bool
	dragFrom geom.Point
	delta    float32 // logical travel along the slide axis
	lastD    float32
	lastT    time.Time
	vel      float32 // px per second along the axis (signed like delta)
}

// Carousel adds a slideshow of n slides; slide(i) builds slide i.
//
// ConfigProvider.Defaults("Carousel", ...) keys: "autoplay"
// (time.Duration), "dots" (bool), "arrows" (string), "dotPosition"
// (string), "type" ("card"), "effect" ("fade"), "height" (float32).
func Carousel(c *Context, n int, slide func(i int)) *CarouselW {
	w := &CarouselW{c: c, n: n, slide: slide, dots: true, dotPos: "bottom", height: 160, speed: 500 * time.Millisecond, th: c.ThemeFor(theme.CompCarousel)}
	w.tk = &w.th.Components.Carousel
	w.arrows = "never"
	if c.Theme.Flavor == theme.Element {
		w.arrows, w.height, w.speed = "hover", 300, 300*time.Millisecond
	}
	w.autoplay = Prop(c, "Carousel", "autoplay", w.autoplay)
	w.dots = Prop(c, "Carousel", "dots", w.dots)
	w.arrows = Prop(c, "Carousel", "arrows", w.arrows)
	w.dotPos = Prop(c, "Carousel", "dotPosition", w.dotPos)
	w.height = Prop(c, "Carousel", "height", w.height)
	w.fade = Prop(c, "Carousel", "effect", "") == "fade"
	w.card = Prop(c, "Carousel", "type", "") == "card"
	return w
}

// Card shows three slides at once: the current one in the middle and its
// neighbours scaled down and dimmed at the sides (Element type="card");
// clicking a side card moves to it.
func (w *CarouselW) Card() *CarouselW { w.card = true; return w }

// Current binds the active slide index.
func (w *CarouselW) Current(i *int) *CarouselW { w.cur = i; return w }

// Autoplay advances every d (0 disables).
func (w *CarouselW) Autoplay(d time.Duration) *CarouselW { w.autoplay = d; return w }

// Fade cross-fades instead of sliding.
func (w *CarouselW) Fade() *CarouselW { w.fade = true; return w }

// Vertical slides up/down (indicators on the right).
func (w *CarouselW) Vertical() *CarouselW { w.vertical, w.dotPos = true, "right"; return w }

// Dots shows/hides the indicators.
func (w *CarouselW) Dots(on bool) *CarouselW { w.dots = on; return w }

// DotPosition: bottom, top, left, right, or outside (below the frame).
func (w *CarouselW) DotPosition(p string) *CarouselW { w.dotPos = p; return w }

// Arrows: "always", "hover" or "never".
func (w *CarouselW) Arrows(mode string) *CarouselW { w.arrows = mode; return w }

// Height sets the frame height.
func (w *CarouselW) Height(px float32) *CarouselW { w.height = px; return w }

// Loop(false) stops at the ends: the first slide has no previous one and
// the last no next one (arrows and keys do nothing there, the arrows look
// disabled, autoplay stops at the last slide). Default true.
func (w *CarouselW) Loop(on bool) *CarouselW { w.noLoop = !on; return w }

// Draggable lets a pointer drag or a finger swipe move the slides: the
// slides follow the pointer, and on release a travel past the swipe
// threshold (or a quick flick) turns the page, otherwise they spring back.
// Drags that start on a control inside a slide are taken over once they
// move sideways.
func (w *CarouselW) Draggable(on bool) *CarouselW { w.drag = on; return w }

// SwipeThreshold sets the fraction of the slide size a drag must travel to
// turn the page (default 0.2).
func (w *CarouselW) SwipeThreshold(frac float32) *CarouselW { w.swipe = frac; return w }

// swipe velocity (px/s) that turns the page whatever the travel.
const carouselFlick = 400

// Changed reports a slide change in the last Show.
func (w *CarouselW) Changed() bool { return w.changed }

func (w *CarouselW) mod(i int) int { return ((i % w.n) + w.n) % w.n }

// Show builds the carousel.
func (w *CarouselW) Show() *Box {
	c := w.c
	outer := c.box("carousel-outer", layout.Column, nil)
	var box *Box
	c.body(outer.n, func() { box = c.box("carousel", layout.Stack, []layout.Option{layout.H(w.height)}) })
	box.Clip().Radius(0)
	if !w.card {
		box.Radius(w.tk.Radius) // shadcn carousel items sit in rounded cards
	}
	if w.n <= 0 {
		return outer
	}
	st := State[carouselState](c, box.n.id, nil)
	if !st.started {
		st.started, st.since = true, c.now
		if w.cur != nil {
			st.seq = w.mod(*w.cur)
		}
	}
	if w.noLoop && (st.seq < 0 || st.seq >= w.n) {
		st.seq = w.mod(st.seq) // without looping seq is the index itself
	}
	if w.cur != nil && w.mod(*w.cur) != w.mod(st.seq) {
		// App changed the index: move the shortest way.
		d := w.mod(*w.cur) - w.mod(st.seq)
		if !w.noLoop {
			if d > w.n/2 {
				d -= w.n
			} else if d < -w.n/2 {
				d += w.n
			}
		}
		st.seq += d
		st.since = c.now
	}
	loc := c.Locale()
	outer.n.setRole(a11y.RoleGroup)
	var ia Interaction
	if !w.dots && w.n > 1 {
		// No indicators to tab to: the frame itself takes focus and
		// answers to the arrow keys.
		ia = box.Focusable(input.CursorDefault)
		box.n.ownKeys = true
		box.n.setRole(a11y.RoleGroup).setName(loc.Slide(w.mod(st.seq)+1, w.n))
	} else {
		ia = box.Interact(input.CursorDefault)
	}
	hover := ia.Hovered || c.hovered(box.n.id)
	goTo := func(seq int) {
		if w.noLoop {
			seq = max(0, min(seq, w.n-1))
		}
		if seq != st.seq {
			st.seq = seq
			st.since = c.now
			w.changed = true
			c.invalid = true
		}
	}
	if ia.Focused {
		for _, k := range navKeys(c, box.n.id) {
			// Horizontal arrows are visual: in RTL Left shows the next
			// slide (slides run right to left).
			prev, next := c.inlineKey(k)
			if w.vertical {
				prev, next = k == input.KeyUp, k == input.KeyDown
			}
			switch {
			case prev:
				goTo(st.seq - 1)
			case next:
				goTo(st.seq + 1)
			}
		}
	}
	// Autoplay pauses while hovered or focused (WAI-ARIA carousel) and is
	// off with the ReduceMotion preference. NoAnimations alone keeps it:
	// slides then jump, and the advance is content, not decoration.
	atEnd := w.noLoop && st.seq >= w.n-1
	if w.autoplay > 0 && w.n > 1 && !c.reduceMotion && !atEnd {
		if hover || st.focusIn || st.dragging {
			st.since = c.now
		} else if c.now.Sub(st.since) >= w.autoplay {
			goTo(st.seq + 1)
		}
		c.WakeAt(st.since.Add(w.autoplay))
	}
	r := c.rects[box.n.id]
	size := r.W
	if w.vertical {
		size = r.H
	}
	var pos float32
	if w.drag && w.n > 1 {
		pos = w.swipeDrag(box, st, size, goTo)
	} else {
		pos = c.Anim(box.n.id, "pos", float32(st.seq), w.speed)
	}
	if size == 0 {
		c.invalid = true
	}
	box.Body(func() {
		if w.card && !w.vertical {
			w.cards(pos, size, st.seq, goTo)
		} else if w.fade {
			for i := 0; i < w.n; i++ {
				i := i
				d := float32(math.Abs(float64(pos - float32(st.seq))))
				op := float32(0)
				switch i {
				case w.mod(st.seq):
					op = 1 - min(1, d)
				case w.mod(int(math.Round(float64(pos)))):
					op = min(1, d)
				}
				if i == w.mod(st.seq) && d == 0 {
					op = 1
				}
				if op <= 0 {
					continue
				}
				Key(c, i, func() {
					sl := Column(c, func() { w.slide(i) }, layout.Abs(0, 0, 0, 0)).Opacity(op)
					sl.n.setRole(a11y.RoleGroup).setName(loc.Slide(i+1, w.n))
				})
			}
		} else {
			lo := int(math.Floor(float64(pos)))
			for k := lo; k <= lo+1; k++ {
				k := k
				if w.noLoop && (k < 0 || k >= w.n) {
					continue // past an end (the drag's rubber band)
				}
				off := (float32(k) - pos) * size
				if size == 0 && k != st.seq {
					continue
				}
				if math.Abs(float64(off)) >= float64(size) && size > 0 {
					continue
				}
				Key(c, k, func() {
					opt := layout.Abs(off, 0, -off, 0)
					if w.vertical {
						opt = layout.Abs(0, off, 0, -off)
					}
					sl := Column(c, func() { w.slide(w.mod(k)) }, opt)
					sl.n.setRole(a11y.RoleGroup).setName(loc.Slide(w.mod(k)+1, w.n))
				})
			}
		}
		cur := w.mod(st.seq)
		if w.dots && w.dotPos != "outside" {
			w.indicators(cur, goTo, st.seq)
		}
		show := w.arrows == "always" || (w.arrows == "hover" && (hover || st.focusIn))
		if show && w.n > 1 && !w.card {
			w.arrow(true, w.noLoop && st.seq <= 0, func() { goTo(st.seq - 1) })
			w.arrow(false, w.noLoop && st.seq >= w.n-1, func() { goTo(st.seq + 1) })
		}
	})
	if w.dots && w.dotPos == "outside" {
		c.body(outer.n, func() { w.indicators(w.mod(st.seq), goTo, st.seq) })
	}
	if w.cur != nil {
		*w.cur = w.mod(st.seq)
	}
	st.focusIn = c.focus != 0 && c.focusVisible && subtreeHas(outer.n, c.focus)
	return outer
}

// swipeDrag runs the pointer swipe of a Draggable carousel and returns the
// position (in slides) to show: the slides follow the pointer while it is
// down; on release the page turns past the threshold or on a flick, else
// the slides spring back.
func (w *CarouselW) swipeDrag(box *Box, st *carouselState, size float32, goTo func(int)) float32 {
	c := w.c
	id := box.n.id
	axis := PanX
	if w.vertical {
		axis = PanY
	}
	box.TakePointer(axis)
	span := size
	if w.card && !w.vertical {
		span = size / 2 // a side card sits half a frame from the centre
	}
	along := func(p geom.Point) float32 {
		if w.vertical {
			return p.Y - st.dragFrom.Y
		}
		d := p.X - st.dragFrom.X
		if c.IsRTL() {
			d = -d // slides run right to left
		}
		return d
	}
	if c.Dragging(id) {
		c.KeepPointer(id)
		if !st.dragging {
			st.dragging, st.delta, st.lastD, st.vel, st.lastT = true, 0, 0, 0, c.now
			st.dragFrom = c.pointer
			if from, ok := c.PointerTaken(id); ok {
				st.dragFrom = from
			}
		}
		d := along(c.pointer)
		if dt := c.now.Sub(st.lastT).Seconds(); dt > 0 {
			v := (d - st.lastD) / float32(dt)
			st.vel = 0.6*v + 0.4*st.vel
			st.lastD, st.lastT = d, c.now
		}
		// Rubber band past the ends without looping.
		if w.noLoop && (st.seq <= 0 && d > 0 || st.seq >= w.n-1 && d < 0) {
			d *= 0.3
		}
		st.delta = d
		if span <= 0 {
			return float32(st.seq)
		}
		return c.Anim(id, "pos", float32(st.seq)-d/span, 0)
	}
	if st.dragging {
		st.dragging = false
		thr := w.swipe
		if thr <= 0 {
			thr = 0.2
		}
		d := st.delta
		if !c.PointerCancelled(id) && span > 0 {
			// A flick needs some travel too, so a tap that jitters does
			// not turn the page.
			flick := abs32(d) >= DragSlop && (st.vel <= -carouselFlick && d < 0 || st.vel >= carouselFlick && d > 0)
			switch {
			case d <= -thr*span || flick && d < 0:
				goTo(st.seq + 1)
			case d >= thr*span || flick && d > 0:
				goTo(st.seq - 1)
			}
		}
		st.delta, st.vel = 0, 0
		c.invalid = true
	}
	return c.Anim(id, "pos", float32(st.seq), w.speed)
}

// cards builds the card-mode slides: each slide's distance from the
// (animated) position decides its place between the left edge, the centre
// and the right edge, its scale and its dimming; farther cards are built
// first so the current one is on top.
func (w *CarouselW) cards(pos, size float32, seq int, goTo func(int)) {
	c, tk := w.c, w.tk
	loc := c.Locale()
	const k = 0.83 // side card scale
	cw := size / 2
	type card struct {
		i   int
		rel float32
	}
	var cards []card
	for i := 0; i < w.n; i++ {
		rel := float32(i) - pos
		if !w.noLoop {
			rel = float32(i) - float32(math.Mod(float64(pos), float64(w.n)))
			if rel < 0 {
				rel += float32(w.n)
			}
			if rel > float32(w.n)/2 {
				rel -= float32(w.n)
			}
		}
		if math.Abs(float64(rel)) > 1.5 {
			continue
		}
		cards = append(cards, card{i, rel})
	}
	// Farthest first.
	for a := 1; a < len(cards); a++ {
		for b := a; b > 0 && math.Abs(float64(cards[b].rel)) > math.Abs(float64(cards[b-1].rel)); b-- {
			cards[b], cards[b-1] = cards[b-1], cards[b]
		}
	}
	mid := size / 2
	side := k * cw / 2 // centre of a side card whose outer edge touches the frame
	for _, cd := range cards {
		cd := cd
		r := max(-1, min(1, cd.rel))
		var cx float32
		if r < 0 {
			cx = mid + r*(mid-side)
		} else {
			cx = mid + r*(size-side-mid)
		}
		a := float32(math.Abs(float64(r)))
		scale := 1 - (1-k)*a
		op := float32(1)
		if ex := float32(math.Abs(float64(cd.rel))) - 1; ex > 0 {
			op = max(0, 1-2*ex)
		}
		left := cx - cw/2
		Key(c, cd.i, func() {
			sl := Column(c, func() { w.slide(cd.i) }, layout.Abs(left, 0, layout.Unset, 0), layout.W(cw))
			sl.Scale(scale).Clip()
			if op < 1 {
				sl.Opacity(op)
			}
			sl.n.setRole(a11y.RoleGroup).setName(loc.Slide(cd.i+1, w.n))
			ia := sl.Interact(input.CursorDefault)
			// Clicking a side card is a pointer shortcut; the keyboard
			// reaches the same slides through the indicators (or the
			// focusable frame's arrow keys without them).
			sl.n.aKbd = true
			if a > 0.01 {
				sl.n.cursor = input.CursorPointer
				if ia.Clicked {
					d := 1
					if cd.rel < 0 {
						d = -1
					}
					goTo(seq + d)
				}
			}
			mask := theme.Fade(tk.CardMaskColor, float64(0.24*a))
			sl.PaintOver(func(l *paint.List, r geom.Rect) {
				if mask.A > 0 {
					l.Fill(r, geom.Radii{}, mask)
				}
			})
		})
	}
}

// indicators adds the dot/line strip; clicking (Element: hovering also
// works with trigger=click only) selects a slide.
func (w *CarouselW) indicators(cur int, goTo func(int), seq int) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	vert := w.dotPos == "left" || w.dotPos == "right"
	dir := layout.Row
	if vert {
		dir = layout.Column
	}
	var opts []layout.Option
	switch w.dotPos {
	case "top":
		opts = []layout.Option{layout.Abs(0, 12, 0, layout.Unset), layout.JustifyContent(layout.JustifyCenter)}
	case "left":
		opts = []layout.Option{layout.Abs(12, 0, layout.Unset, 0), layout.JustifyContent(layout.JustifyCenter)}
	case "right":
		opts = []layout.Option{layout.Abs(layout.Unset, 0, 12, 0), layout.JustifyContent(layout.JustifyCenter)}
	case "outside":
		opts = []layout.Option{layout.JustifyContent(layout.JustifyCenter), layout.PadVH(12, 0)}
	default:
		opts = []layout.Option{layout.Abs(0, layout.Unset, 0, 12), layout.JustifyContent(layout.JustifyCenter)}
	}
	opts = append(opts, layout.Gap(tk.DotGap), layout.Middle())
	b := c.box("carousel-dots", dir, opts)
	orient := a11y.Horizontal
	axis := rovingHorizontal
	if vert {
		orient, axis = a11y.Vertical, rovingVertical
	}
	b.n.setRole(a11y.RoleTabList).setState(orient)
	loc := c.Locale()
	var nodes []*node
	b.Body(func() {
		for i := 0; i < w.n; i++ {
			i := i
			n := c.add("carousel-dot", "")
			active := i == cur
			dw, dh := tk.DotWidth, tk.DotHeight
			if active {
				dw = tk.DotActiveWidth
			}
			if vert {
				dw, dh = dh, dw
			}
			if el {
				// Element pads each indicator for a larger hit area.
				if vert {
					n.ln.Padding = geom.VH(4, 12)
				} else {
					n.ln.Padding = geom.VH(12, 4)
				}
			}
			ww := c.Anim(n.id, "w", dw, th.DurationSlow)
			hh := c.Anim(n.id, "h", dh, th.DurationSlow)
			n.ln.Width, n.ln.Height = ww+n.ln.Padding.Horizontal(), hh+n.ln.Padding.Vertical()
			ia := c.interact(n, true, input.CursorPointer)
			n.setRole(a11y.RoleTab).setName(loc.Slide(i+1, w.n)).setState(a11y.Selectable).stateIf(active, a11y.Selected|a11y.Current)
			nodes = append(nodes, n)
			if ia.Clicked && !active {
				d := i - cur
				goTo(seq + d)
			}
			if ia.FocusVisible {
				n.over = func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r.Inset(n.ln.Padding), geom.Radii{}) }
			}
			hov := ia.Hovered
			outside := w.dotPos == "outside"
			n.paint = func(l *paint.List, r geom.Rect) {
				in := r.Inset(n.ln.Padding)
				var col theme.Color
				switch {
				case outside && active:
					col = tk.DotOutsideActiveColor
				case outside:
					col = tk.DotOutsideColor
				case active:
					col = tk.DotActiveColor
				case hov:
					col = tk.DotHoverColor
				default:
					col = tk.DotColor
				}
				l.Fill(in, geom.Rad(min(tk.DotRadius, min(in.W, in.H)/2)), col)
			}
		}
		rovingTabStop(nodes, cur)
		ids := make([]ID, len(nodes))
		for i, n := range nodes {
			ids[i] = n.id
		}
		// Automatic activation: arrows move focus and show that slide.
		if to := rovingStep(c, ids, nil, axis); to >= 0 {
			navFocus(c, ids[to])
			if to != cur {
				goTo(seq + to - cur)
			}
		}
	})
}

// arrow adds the previous (left: true) or next button; Abs offsets are
// logical, so in RTL the previous button sits at the right.
func (w *CarouselW) arrow(left, disabled bool, fn func()) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	n := c.add("carousel-arrow", map[bool]string{true: c.Locale().Carousel.Prev, false: c.Locale().Carousel.Next}[left])
	size := tk.ArrowSize
	n.ln.Width, n.ln.Height = size, size
	if w.vertical {
		if left {
			n.ln.Style.Apply(layout.Abs(layout.Unset, 16, layout.Unset, layout.Unset))
		} else {
			n.ln.Style.Apply(layout.Abs(layout.Unset, layout.Unset, layout.Unset, 16))
		}
		if r, ok := c.rects[c.cur.id]; ok {
			n.ln.Pos.Left = (r.W - size) / 2
		}
		n.ln.AlignSelf = layout.AlignCenter
	} else if left {
		n.ln.Style.Apply(layout.Abs(16, (w.height-size)/2, layout.Unset, layout.Unset))
	} else {
		n.ln.Style.Apply(layout.Abs(layout.Unset, (w.height-size)/2, 16, layout.Unset))
	}
	n.ln.AlignSelf = layout.AlignCenter
	if disabled {
		n.disabled = true
	}
	ia := c.interact(n, true, input.CursorPointer)
	n.setRole(a11y.RoleButton)
	if disabled {
		n.setState(a11y.Disabled)
	}
	if ia.Clicked && !disabled {
		fn()
	}
	if ia.FocusVisible {
		n.over = func(l *paint.List, r geom.Rect) { drawFocusRing(l, th, r, geom.Rad(r.W/2)) }
	}
	op := c.AnimFrom(n.id, "in", 0, 1, th.DurationMid)
	if disabled {
		op *= tk.ArrowDisabledOpacity
		ia.Hovered = false
	}
	n.paint = func(l *paint.List, r geom.Rect) {
		ic := icon.Left
		if !left {
			ic = icon.Right
		}
		if w.vertical {
			ic = icon.Up
			if !left {
				ic = icon.Down
			}
		}
		col, bg := tk.ArrowColor, tk.ArrowBg
		if ia.Hovered {
			col, bg = tk.ArrowHoverColor, tk.ArrowHoverBg
		}
		if el {
			l.Path(paint.NewPath().Circle(r.X+r.W/2, r.Y+r.H/2, r.W/2), theme.Fade(bg, float64(op)))
			c.paintIcon(l, ic, r.Inset(geom.All(10)), theme.Fade(col, float64(op)), 0)
			return
		}
		if th.Flavor == theme.StyleShadcn {
			if c.IsRTL() && !w.vertical {
				// shad2bCarouselArrow does not mirror: hand it the
				// opposite arrow so prev points right in RTL.
				ic = map[bool]*icon.Icon{true: icon.Right, false: icon.Left}[left]
			}
			shad2bCarouselArrow(l, th, r, ic, ia.Hovered, op)
			return
		}
		if bg.A > 0 {
			l.Path(paint.NewPath().Circle(r.X+r.W/2, r.Y+r.H/2, r.W/2), theme.Fade(bg, float64(op)))
		}
		c.paintIcon(l, ic, r.Inset(geom.All(4)), theme.Fade(col, float64(op)), 0)
	}
}
