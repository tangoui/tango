package ui

// Slider (Ant Design Slider, Element Plus el-slider): single value or a
// range with two handles, marks and step dots, horizontal or vertical, a
// value tooltip while hovering / dragging, an attached InputNumber
// (Element show-input) and full keyboard support (arrows, PageUp /
// PageDown, Home / End).
//
// Structure: the "slider" node is a row (column when vertical) holding the
// track and, with WithInput, the number box. The track lays its handles
// out with flex spacers whose grow factors are the value fractions, so the
// handles land exactly on their values in the current frame. Each handle
// is a focusable node with the slider role (WAI-ARIA multi-thumb pattern);
// the track paints rail, filled part, marks and handle visuals.

import (
	"math"
	"sort"
	"strconv"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

type sliderState struct {
	drag int // handle moved by a press on the rail (-1 none)
	// grab is the value index a pressed handle currently moves (-1 none).
	// Range handles may cross: the dragged handle then takes over the
	// other value, so grab can differ from the pressed node's index.
	grab int
}

// SliderW is a slider; chain modifiers after Slider / SliderRange.
type SliderW struct {
	c        *Context
	n        *node // the slider row / column
	track    *node
	val      *float64
	hi       *float64 // range end (nil: single value)
	min, max float64
	step     float64
	disabled bool
	changed  bool
	ia       Interaction // presses on the rail
	z        *lazy
	st       *sliderState

	marks    map[float64]string
	dots     bool
	vertical bool
	height   float32
	tipFmt   func(float64) string
	tipMode  int // 0 hover / drag / focus, 1 always, -1 never
	input    bool
	noCross  bool    // range handles stop at each other instead of swapping
	push     float64 // Pushable: minimum distance kept between range handles
	aName    string
	loc      *Locale

	thumbs [2]*node
	tia    [2]Interaction

	th *theme.Theme       // ThemeFor(CompSlider)
	tk *theme.SliderToken // &th.Components.Slider
}

const (
	sliderPad    = 6  // rail inset at both ends (handle radius)
	sliderThick  = 22 // track thickness without mark labels
	sliderHandle = 20 // handle hit box
)

// Slider adds a slider bound to *val in [min,max] (step 1).
//
//	ui.Slider(c, &v, 0, 100).Marks(map[float64]string{0: "0°C", 100: "100°C"}).Tooltip(nil)
//	ui.Slider(c, &v, 0, 100).Vertical(200)
//	ui.Slider(c, &v, 0, 10).Step(0.5).WithInput()
func Slider(c *Context, val *float64, min, max float64) *SliderW {
	w := &SliderW{c: c, val: val, min: min, max: max, step: 1, loc: c.Locale()}
	w.th = c.ThemeFor(theme.CompSlider)
	w.tk = &w.th.Components.Slider
	w.n = c.add("slider", "")
	w.n.ln.Dir = layout.Row
	w.n.ln.Align = layout.AlignCenter
	w.st = State(c, w.n.id, func() sliderState { return sliderState{drag: -1, grab: -1} })
	w.ia = c.interact(w.n, false, input.CursorPointer)
	if v, ok := p2bBool(c, "Slider", "dots"); ok {
		w.dots = v
	}
	if v, ok := p2bBool(c, "Slider", "showInput"); ok {
		w.input = v
	}
	if v, ok := p2bBool(c, "Slider", "tooltip"); ok && !v {
		w.tipMode = -1
	}
	if v, ok := p2bNum(c, "Slider", "step"); ok && v > 0 {
		w.step = v
	}
	if v, ok := p2bBool(c, "Slider", "disabled"); ok && v {
		w.Disabled(true)
	}
	if v, ok := c.ConfigProp("Slider", "marks"); ok {
		if m, ok := v.(map[float64]string); ok {
			w.marks = m
		}
	}
	w.z = c.lazy(w.n, w.finish)
	return w
}

// SliderRange adds a two-handle slider bound to *lo and *hi (Ant range,
// Element range).
func SliderRange(c *Context, lo, hi *float64, min, max float64) *SliderW {
	return Slider(c, lo, min, max).Range(hi)
}

// Range turns the slider into a range: *val is the start, *hi the end.
func (w *SliderW) Range(hi *float64) *SliderW { w.hi = hi; return w }

// Step sets the value granularity (0: continuous).
func (w *SliderW) Step(s float64) *SliderW { w.step = s; return w }

// Marks labels values on the rail (a dot is drawn at each mark).
func (w *SliderW) Marks(m map[float64]string) *SliderW { w.marks = m; return w }

// Dots draws a dot at every step (Ant dots, Element show-stops).
func (w *SliderW) Dots() *SliderW { w.dots = true; return w }

// Vertical lays the slider out vertically with the given height (min at
// the bottom).
func (w *SliderW) Vertical(height float32) *SliderW {
	w.vertical, w.height = true, height
	return w
}

// Tooltip sets the tooltip formatter of the handle value (nil: the plain
// number). The tooltip shows while a handle is hovered, dragged or focused
// with the keyboard; TooltipOpen forces it.
func (w *SliderW) Tooltip(format func(float64) string) *SliderW {
	w.tipFmt = format
	if w.tipMode < 0 {
		w.tipMode = 0
	}
	return w
}

// TooltipOpen always shows (true) or never shows (false) the tooltips.
func (w *SliderW) TooltipOpen(on bool) *SliderW {
	w.tipMode = -1
	if on {
		w.tipMode = 1
	}
	return w
}

// WithInput adds InputNumbers bound to the values (Element show-input). A
// single horizontal slider has the box after the track; a range has the
// start box before the track and the end box after it. Vertically the
// boxes go below the track (a range: the end box above, the start box
// below).
func (w *SliderW) WithInput() *SliderW { w.input = true; return w }

// AllowCross sets whether a dragged range handle may pass the other one
// (they swap values and the dragged handle continues as the other end).
// Default true; false stops it at the other handle. Keyboard steps never
// cross.
func (w *SliderW) AllowCross(on bool) *SliderW { w.noCross = !on; return w }

// Pushable keeps range handles at least gap apart: moving one towards the
// other pushes it along (until it reaches the end of the slider). It turns
// crossing off. 0 disables.
func (w *SliderW) Pushable(gap float64) *SliderW { w.push = math.Max(0, gap); return w }

// Disabled turns the slider off.
func (w *SliderW) Disabled(on bool) *SliderW {
	if on {
		w.disabled, w.n.cursor = true, input.CursorNotAllowed
	}
	return w
}

// Changed reports a value change this frame.
func (w *SliderW) Changed() bool { w.z.run(); return w.changed }

// A11yLabel sets the accessible name (inside a FormItem the item label is
// used). A range names its group; the handles are "minimum" / "maximum".
func (w *SliderW) A11yLabel(s string) *SliderW { w.aName = s; return w }

// Layout applies layout options to the slider row.
func (w *SliderW) Layout(opts ...layout.Option) *SliderW { w.n.ln.Style.Apply(opts...); return w }

func (w *SliderW) off() bool { return w.disabled || w.n.disabled }

// ---------------------------------------------------------------------------
// Values

// value returns handle i's value.
func (w *SliderW) value(i int) float64 {
	if i == 1 && w.hi != nil {
		return *w.hi
	}
	return *w.val
}

// limits returns the range handle i may move in.
func (w *SliderW) limits(i int) (lo, hi float64) {
	lo, hi = w.min, w.max
	if w.hi != nil && w.push > 0 {
		// The other handle gets pushed; only the gap to the far end limits.
		if i == 0 {
			hi = math.Max(lo, hi-w.push)
		} else {
			lo = math.Min(hi, lo+w.push)
		}
		return lo, hi
	}
	if w.hi != nil {
		if i == 0 {
			hi = math.Min(hi, *w.hi)
		} else {
			lo = math.Max(lo, *w.val)
		}
	}
	return lo, hi
}

// snap rounds v to the step grid.
func (w *SliderW) snap(v float64) float64 {
	if w.step > 0 {
		v = math.Round((v-w.min)/w.step)*w.step + w.min
		// Trim float noise (0.1 steps).
		if p := sliderDecimals(w.step); p >= 0 {
			k := math.Pow(10, float64(p))
			v = math.Round(v*k) / k
		}
	}
	return v
}

// sliderDecimals returns the decimals of step (-1 when not short).
func sliderDecimals(step float64) int {
	for p := 0; p <= 8; p++ {
		k := math.Pow(10, float64(p))
		if math.Abs(step*k-math.Round(step*k)) < 1e-9 {
			return p
		}
	}
	return -1
}

// set moves handle i to v (snapped and clamped).
func (w *SliderW) set(i int, v float64) {
	lo, hi := w.limits(i)
	v = math.Max(lo, math.Min(hi, w.snap(v)))
	p := w.val
	if i == 1 && w.hi != nil {
		p = w.hi
	}
	if v != *p {
		*p = v
		w.changed = true
		w.c.invalid = true
	}
	w.pushOther(i)
}

// pushOther moves the other range handle away from handle i so that they
// stay Pushable's gap apart.
func (w *SliderW) pushOther(i int) {
	if w.hi == nil || w.push <= 0 {
		return
	}
	switch {
	case i == 0 && *w.hi-*w.val < w.push-1e-9:
		*w.hi = math.Min(w.max, *w.val+w.push)
	case i == 1 && *w.hi-*w.val < w.push-1e-9:
		*w.val = math.Max(w.min, *w.hi-w.push)
	default:
		return
	}
	w.changed = true
	w.c.invalid = true
}

// drag moves value i to v under the pointer and returns the index the
// pointer controls afterwards. Range handles cross like Ant's: dragging
// one past the other swaps the values, the dragged handle continues as
// the other end, and keyboard focus follows it.
func (w *SliderW) drag(i int, v float64) int {
	if w.hi == nil || w.noCross || w.push > 0 {
		w.set(i, v)
		return i
	}
	v = math.Max(w.min, math.Min(w.max, w.snap(v)))
	switch {
	case i == 0 && v > *w.hi:
		*w.val, *w.hi = *w.hi, v
		i = 1
	case i == 1 && v < *w.val:
		*w.hi, *w.val = *w.val, v
		i = 0
	default:
		w.set(i, v)
		return i
	}
	w.changed = true
	w.c.invalid = true
	w.c.SetFocus(w.thumbs[i].id)
	return i
}

// frac maps a value to [0,1].
func (w *SliderW) frac(v float64) float64 {
	return math.Max(0, math.Min(1, (v-w.min)/math.Max(1e-12, w.max-w.min)))
}

// fromPoint maps a pointer position over track rect r to a value. A
// horizontal slider runs from its inline start (min at the right in RTL).
func (w *SliderW) fromPoint(r geom.Rect, p geom.Point) float64 {
	var t float64
	if w.vertical {
		y0, y1 := r.Y+sliderPad, r.Bottom()-sliderPad
		t = float64((y1 - p.Y) / max(1, y1-y0))
	} else {
		px := p.X
		if w.c.IsRTL() {
			px = 2*r.X + r.W - px // mirror into LTR space
		}
		x0, x1 := r.X+sliderPad, r.Right()-sliderPad
		t = float64((px - x0) / max(1, x1-x0))
	}
	t = math.Max(0, math.Min(1, t))
	return w.min + t*(w.max-w.min)
}

func (w *SliderW) handles() int {
	if w.hi != nil {
		return 2
	}
	return 1
}

// format returns the tooltip / value text of v.
func (w *SliderW) format(v float64) string {
	if w.tipFmt != nil {
		return w.tipFmt(v)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// sortedMarks returns the mark values in ascending order.
func (w *SliderW) sortedMarks() []float64 {
	ks := make([]float64, 0, len(w.marks))
	for k := range w.marks {
		if k >= w.min && k <= w.max {
			ks = append(ks, k)
		}
	}
	sort.Float64s(ks)
	return ks
}

// ---------------------------------------------------------------------------
// Build

// markExtent is the room taken by mark labels across the rail.
func (w *SliderW) markExtent() float32 {
	if len(w.marks) == 0 {
		return 0
	}
	if !w.vertical {
		return 22
	}
	c, th := w.c, w.th
	m := float32(0)
	for _, s := range w.marks {
		m = max(m, entTextW(c, s, th.FontSize, text.Regular))
	}
	return m + 12
}

func (w *SliderW) finish() {
	c := w.c
	n := w.n
	if w.vertical {
		n.ln.Dir = layout.Column
		n.ln.Align = layout.AlignStart
	}
	n.ln.Gap = 16
	if w.th.Flavor == theme.Element {
		n.ln.Gap = 20
	}
	// Semantics: a group of one or two sliders (the handles); a range
	// group carries the A11yLabel, a single handle carries it itself.
	// The row (a rail press target) is a group; its handles are reached
	// with Tab and moved with the arrow keys.
	n.setRole(a11y.RoleGroup)
	n.aKbd = true
	if w.hi != nil && w.aName != "" {
		n.setName(w.aName)
	}
	switch {
	case !w.input:
		w.buildTrack()
	case w.hi == nil:
		w.buildTrack()
		w.numberBox(0)
	case w.vertical: // max at the top
		w.numberBox(1)
		w.buildTrack()
		w.numberBox(0)
	default:
		w.numberBox(0)
		w.buildTrack()
		w.numberBox(1)
	}
	if w.off() {
		return
	}
	// Rail presses pick the nearest handle and drag it.
	tr, ok := c.rects[w.track.id]
	if ok && w.ia.PressedNow && tr.Expand(6).Contains(c.pointer) {
		v := w.fromPoint(tr, c.pointer)
		i := 0
		if w.hi != nil && math.Abs(v-*w.hi) < math.Abs(v-*w.val) || (w.hi != nil && v > *w.hi) {
			i = 1
		}
		w.st.drag = i
		c.SetFocus(w.thumbs[i].id)
	}
	if !c.Dragging(w.n.id) && !w.ia.PressedNow || !ok {
		w.st.drag = -1
	}
	// A pressed handle grabs its value; the grab survives a crossing.
	pressed := -1
	for i := 0; i < w.handles(); i++ {
		if w.tia[i].PressedNow || c.Dragging(w.thumbs[i].id) {
			pressed = i
		}
	}
	if pressed < 0 || !ok {
		w.st.grab = -1
	} else if w.st.grab < 0 {
		w.st.grab = pressed
	}
	switch {
	case ok && w.st.grab >= 0:
		w.st.grab = w.drag(w.st.grab, w.fromPoint(tr, c.pointer))
	case ok && w.st.drag >= 0:
		w.st.drag = w.drag(w.st.drag, w.fromPoint(tr, c.pointer))
	}
	for i := 0; i < w.handles(); i++ {
		th := w.thumbs[i]
		if v, ok := c.takeSetNumber(th.id); ok {
			w.set(i, v)
		}
		w.keys(i)
	}
	w.tooltips()
}

// numberBox adds the InputNumber of value i (WithInput). Its limits are
// the handle's, so a range box cannot pass the other value; with Pushable
// the other value is pushed.
func (w *SliderW) numberBox(i int) {
	c := w.c
	p := w.val
	if i == 1 && w.hi != nil {
		p = w.hi
	}
	lo, hi := w.limits(i)
	in := InputNumber(c, p).Min(lo).Max(hi).Step(w.step).Disabled(w.off())
	switch {
	case w.th.Flavor == theme.Element && w.hi == nil && !w.vertical:
		in.Width(130)
	case w.hi != nil || w.vertical:
		in.Width(96)
	}
	if d := sliderDecimals(w.step); d > 0 {
		in.Precision(d)
	}
	switch {
	case w.hi != nil && i == 0:
		in.A11yLabel(w.loc.Slider.Min)
	case w.hi != nil:
		in.A11yLabel(w.loc.Slider.Max)
	case w.aName != "":
		in.A11yLabel(w.aName)
	}
	if in.Changed() {
		w.changed = true
		w.pushOther(i)
	}
}

// keys handles the keyboard of handle i.
func (w *SliderW) keys(i int) {
	c := w.c
	step := w.step
	if step <= 0 {
		step = (w.max - w.min) / 100
	}
	big := math.Max(step, (w.max-w.min)/10)
	if w.step > 0 {
		big = math.Max(1, math.Round(big/w.step)) * w.step
	}
	// The arrow toward the max side increases: in a horizontal RTL slider
	// that is Left. Up always increases.
	inc := input.KeyRight
	if c.IsRTL() && !w.vertical {
		inc = input.KeyLeft
	}
	for _, k := range c.KeysFor(w.thumbs[i].id) {
		if !k.Down {
			continue
		}
		v := w.value(i)
		lo, hi := w.limits(i)
		switch k.Key {
		case input.KeyLeft, input.KeyRight:
			if k.Key == inc {
				w.set(i, v+step)
			} else {
				w.set(i, v-step)
			}
		case input.KeyDown:
			w.set(i, v-step)
		case input.KeyUp:
			w.set(i, v+step)
		case input.KeyPageDown:
			w.set(i, v-big)
		case input.KeyPageUp:
			w.set(i, v+big)
		case input.KeyHome:
			w.set(i, lo)
		case input.KeyEnd:
			w.set(i, hi)
		}
	}
}

// buildTrack adds the track and its handles.
func (w *SliderW) buildTrack() {
	c := w.c
	t := c.add("slider-track", "")
	w.track = t
	ext := w.markExtent()
	t.ln.HasBasis, t.ln.Grow, t.ln.Shrink = true, 1, 1
	t.ln.Align = layout.AlignStart
	if w.vertical {
		t.ln.Dir = layout.Column
		h := w.height
		if h <= 0 {
			h = 200
		}
		t.ln.Basis, t.ln.Height = h, h
		t.ln.Width = sliderThick + ext
		t.ln.Padding = geom.Insets{Top: sliderPad, Bottom: sliderPad}
		t.ln.Grow = 0
	} else {
		t.ln.Dir = layout.Row
		t.ln.Basis = 200
		t.ln.Height = sliderThick + ext
		t.ln.Padding = geom.Insets{Left: sliderPad, Right: sliderPad}
		t.ln.MinW = 2*sliderPad + 1
	}
	t.paint = w.paint
	parent := c.cur
	c.cur = t
	// Spacers sized by the value fractions place the handles.
	fr := [2]float64{w.frac(*w.val), 0}
	if w.hi != nil {
		fr[1] = w.frac(*w.hi)
	}
	nh := w.handles()
	gaps := []float64{fr[0]}
	if nh == 2 {
		gaps = append(gaps, math.Max(0, fr[1]-fr[0]))
		gaps = append(gaps, math.Max(0, 1-fr[1]))
	} else {
		gaps = append(gaps, 1-fr[0])
	}
	if w.vertical { // top to bottom: max first
		for i, j := 0, len(gaps)-1; i < j; i, j = i+1, j-1 {
			gaps[i], gaps[j] = gaps[j], gaps[i]
		}
	}
	spacer := func(g float64) {
		s := c.add("slider-gap", "")
		s.ln.Grow = float32(g)
		s.ln.HasBasis = true
		s.ln.Shrink = 0
	}
	order := []int{0, 1}
	if w.vertical {
		order = []int{1, 0}
	}
	gi := 0
	spacer(gaps[gi])
	for _, i := range order {
		if i >= nh {
			continue
		}
		w.thumb(i)
		gi++
		spacer(gaps[gi])
	}
	c.cur = parent
}

// thumb adds the focusable handle node i.
func (w *SliderW) thumb(i int) {
	c := w.c
	h := c.add("slider-handle", "")
	w.thumbs[i] = h
	h.ln.Width, h.ln.Height = sliderHandle, sliderHandle
	half := float32(sliderHandle / 2)
	inset := float32(sliderThick-sliderHandle) / 2
	if w.vertical {
		h.ln.Margin = geom.Insets{Top: -half, Bottom: -half, Left: inset}
	} else {
		h.ln.Margin = geom.Insets{Left: -half, Right: -half, Top: inset}
	}
	cur := input.CursorPointer
	if w.off() {
		cur = input.CursorNotAllowed
		h.disabled = true
	}
	w.tia[i] = c.interact(h, !w.off(), cur)
	h.setRole(a11y.RoleSlider)
	if w.vertical {
		h.setState(a11y.Vertical)
	} else {
		h.setState(a11y.Horizontal)
	}
	switch {
	case w.hi != nil && i == 0:
		h.setName(w.loc.Slider.Min)
	case w.hi != nil:
		h.setName(w.loc.Slider.Max)
	case w.aName != "":
		h.setName(w.aName)
	}
	h.fill(sliderThumbSem{w, i})
}

type sliderThumbSem struct {
	w *SliderW
	i int
}

func (s sliderThumbSem) semantics(n *a11y.Node) {
	w := s.w
	lo, hi := w.limits(s.i)
	if w.tipFmt != nil {
		n.Value = w.tipFmt(w.value(s.i))
	}
	rangeSem(n, w.value(s.i), lo, hi, w.step)
	if !w.off() {
		n.Actions |= a11y.SetValue
	}
}

// tooltips shows the value bubble over active handles.
func (w *SliderW) tooltips() {
	c := w.c
	if w.tipMode < 0 {
		return
	}
	for i := 0; i < w.handles(); i++ {
		h := w.thumbs[i]
		show := w.tipMode > 0 || (w.tia[i].Hovered && w.st.grab < 0) || w.st.grab == i ||
			(w.st.drag == i && c.Dragging(w.n.id)) || (c.focus == h.id && c.focusVisible)
		if !show {
			continue
		}
		place := Top
		if w.vertical && w.th.Flavor == theme.Ant {
			place = InlineEnd // the label side
		}
		p2bTip(c, h.id, w.format(w.value(i)), place)
	}
}

// p2bTip shows a dark value bubble with an arrow next to anchor (slider
// handles); it ignores the pointer.
func p2bTip(c *Context, anchor ID, tip string, place Placement) {
	th := c.Theme
	ov := Overlay(c, OverlayOptions{Anchor: anchor, Place: place, Offset: 8}, func() {
		t := Text(c, tip).Color(th.TextOnSpotlight).NoWrap()
		t.n.aHidden = true
	}, layout.PadVH(6, 8))
	if th.Flavor == theme.Element {
		ov.n.ln.Padding = geom.VH(5, 11)
	}
	ov.Bg(th.BgSpotlight).Radius(th.Radius)
	if len(th.TooltipShadow) > 0 {
		ov.Shadow(th.TooltipShadow)
	}
	ov.n.setRole(a11y.RoleTooltip).setName(tip)
	ov.n.hit, ov.n.blocks = false, false
	ov.n.over = func(l *paint.List, r geom.Rect) {
		ar, ok := c.anchorRect(anchor)
		if !ok {
			return
		}
		p := paint.NewPath()
		switch {
		case r.Bottom() <= ar.Y+1: // above
			cx := ar.X + ar.W/2
			p.MoveTo(cx-6, r.Bottom()-0.5).LineTo(cx, r.Bottom()+5.5).LineTo(cx+6, r.Bottom()-0.5).Close()
		case r.Y >= ar.Bottom()-1: // below
			cx := ar.X + ar.W/2
			p.MoveTo(cx-6, r.Y+0.5).LineTo(cx, r.Y-5.5).LineTo(cx+6, r.Y+0.5).Close()
		case r.X >= ar.Right()-1: // right
			cy := ar.Y + ar.H/2
			p.MoveTo(r.X+0.5, cy-6).LineTo(r.X-5.5, cy).LineTo(r.X+0.5, cy+6).Close()
		default: // left
			cy := ar.Y + ar.H/2
			p.MoveTo(r.Right()-0.5, cy-6).LineTo(r.Right()+5.5, cy).LineTo(r.Right()-0.5, cy+6).Close()
		}
		l.Path(p, th.BgSpotlight)
	}
}

// ---------------------------------------------------------------------------
// Paint

// pos maps a value to its point on the rail center line of track r. The
// rail sits at the inline start of a vertical track (labels at the end)
// and a horizontal one runs from the inline start.
func (w *SliderW) pos(r geom.Rect, v float64) geom.Point {
	t := float32(w.frac(v))
	// Computed in LTR space, then mirrored (keeps LTR arithmetic exact).
	var p geom.Point
	if w.vertical {
		y0, y1 := r.Y+sliderPad, r.Bottom()-sliderPad
		p = geom.Pt(r.X+sliderThick/2, y1-(y1-y0)*t)
	} else {
		x0, x1 := r.X+sliderPad, r.Right()-sliderPad
		p = geom.Pt(x0+(x1-x0)*t, r.Y+sliderThick/2)
	}
	if w.c.IsRTL() {
		p.X = 2*r.X + r.W - p.X
	}
	return p
}

// inRange reports whether v lies on the filled part.
func (w *SliderW) inRange(v float64) bool {
	lo, hi := w.min, *w.val
	if w.hi != nil {
		lo, hi = *w.val, *w.hi
	}
	return v >= lo-1e-9 && v <= hi+1e-9
}

func (w *SliderW) paint(l *paint.List, r geom.Rect) {
	c, th, tk := w.c, w.th, w.tk
	el := th.Flavor == theme.Element
	shad := th.Flavor == theme.StyleShadcn
	dis := w.off()
	hov := w.ia.Hovered || c.Dragging(w.n.id)
	for i := 0; i < w.handles(); i++ {
		hov = hov || w.tia[i].Hovered || c.Dragging(w.thumbs[i].id)
	}
	railH := tk.RailSize
	railC, trackC := tk.RailBg, tk.TrackBg
	if hov {
		railC, trackC = tk.RailHoverBg, tk.TrackHoverBg
	}
	if shad && dis {
		// data-[disabled]:opacity-50 on the whole slider.
		l.Opacity(0.5)
		defer l.Pop()
	}
	if dis && !shad {
		trackC = tk.TrackBgDisabled
	}
	a, b := w.pos(r, w.min), w.pos(r, w.max)
	var rail geom.Rect
	if w.vertical {
		rail = geom.Rect{X: a.X - railH/2, Y: b.Y, W: railH, H: a.Y - b.Y}
	} else { // min may be at the right (RTL)
		rail = geom.Rect{X: min(a.X, b.X), Y: a.Y - railH/2, W: max(b.X-a.X, a.X-b.X), H: railH}
	}
	l.Fill(rail, geom.Rad(railH/2), railC)
	lo, hi := w.min, *w.val
	if w.hi != nil {
		lo, hi = *w.val, *w.hi
	}
	p0, p1 := w.pos(r, lo), w.pos(r, hi)
	if w.vertical {
		l.Fill(geom.Rect{X: rail.X, Y: p1.Y, W: railH, H: p0.Y - p1.Y}, geom.Rad(railH/2), trackC)
	} else {
		l.Fill(geom.Rect{X: min(p0.X, p1.X), Y: rail.Y, W: max(p1.X-p0.X, p0.X-p1.X), H: railH}, geom.Rad(railH/2), trackC)
	}
	// Step dots and mark dots.
	var dots []float64
	if w.dots && w.step > 0 && (w.max-w.min)/w.step <= 200 {
		for v := w.min; v <= w.max+1e-9; v += w.step {
			dots = append(dots, v)
		}
	}
	marks := w.sortedMarks()
	dots = append(dots, marks...)
	for _, v := range dots {
		p := w.pos(r, v)
		on := w.inRange(v)
		if el {
			// Element stops: white dots on the runway, none at the ends.
			if v <= w.min+1e-9 || v >= w.max-1e-9 {
				continue
			}
			l.Path(paint.NewPath().Circle(p.X, p.Y, tk.DotSize/2), th.BgContainer)
			continue
		}
		bc := tk.DotBorderColor
		if on {
			bc = trackC
		}
		l.Path(paint.NewPath().Circle(p.X, p.Y, tk.DotSize/2), bc)
		l.Path(paint.NewPath().Circle(p.X, p.Y, tk.DotSize/4), th.BgContainer)
	}
	// Mark labels.
	for _, v := range marks {
		s := w.marks[v]
		p := w.pos(r, v)
		col := th.TextTertiary
		if w.inRange(v) || el {
			col = th.TextSecondary
		}
		if dis {
			col = th.TextDisabled
		}
		tw := entTextW(c, s, th.FontSize, text.Regular)
		if w.vertical { // labels on the inline end side of the rail
			lr := c.inline(r).Rect(sliderThick+6, p.Y-11, tw+2, 22)
			entText(c, l, s, lr.X, lr, th.FontSize, text.Regular, col, 0)
		} else {
			lr := geom.Rect{X: p.X - tw/2, Y: r.Y + sliderThick, W: tw + 2, H: 22}
			entText(c, l, s, lr.X, lr, th.FontSize, text.Regular, col, 0)
		}
	}
	for i := 0; i < w.handles(); i++ {
		w.paintHandle(l, w.pos(r, w.value(i)), i, trackC)
	}
}

// paintHandle draws handle i centred at p.
func (w *SliderW) paintHandle(l *paint.List, p geom.Point, i int, trackC theme.Color) {
	c, th, tk := w.c, w.th, w.tk
	h := w.thumbs[i]
	ia := w.tia[i]
	act := (ia.Hovered && w.st.grab < 0) || w.st.grab == i || (w.st.drag == i && c.Dragging(w.n.id))
	focus := c.focus == h.id
	dis := w.off()
	switch th.Flavor {
	case theme.StyleShadcn:
		hs := tk.HandleSize
		hr := geom.Rect{X: p.X - hs/2, Y: p.Y - hs/2, W: hs, H: hs}
		if len(th.ShadowTertiary) > 0 {
			l.Shadows(hr, geom.Rad(hs/2), th.ShadowTertiary)
		}
		// Thumb: size-4 rounded-full border-primary bg-white shadow-sm;
		// hover / focus-visible: ring-4 ring-ring/50.
		if (act || (focus && c.focusVisible)) && !dis && tk.HandleActiveOutlineColor.A > 0 {
			l.Stroke(hr.Expand(4), geom.Rad(hs/2+4), 4, tk.HandleActiveOutlineColor)
		}
		l.Fill(hr, geom.Rad(hs/2), tk.HandleBg)
		l.Stroke(hr, geom.Rad(hs/2), tk.HandleLineWidth, trackC)
	case theme.Element:
		hs := tk.HandleSize
		if act || focus {
			hs = tk.HandleSizeHover
		}
		col := tk.HandleColor
		if dis {
			col = tk.HandleColorDisabled
		}
		l.Path(paint.NewPath().Circle(p.X, p.Y, hs/2), col)
		l.Path(paint.NewPath().Circle(p.X, p.Y, hs/2-tk.HandleLineWidth), tk.HandleBg)
	default:
		a := c.Anim(h.id, "h", b2f(act || focus), th.DurationMid)
		rad := tk.HandleSize/2 + (tk.HandleSizeHover-tk.HandleSize)/2*a
		ring := tk.HandleLineWidth + (tk.HandleLineWidthHover-tk.HandleLineWidth)*a
		ringC := theme.Lerp(tk.HandleColor, tk.HandleActiveColor, a)
		if dis {
			ringC = tk.HandleColorDisabled
		}
		if focus && c.focusVisible && !dis && tk.HandleActiveOutlineColor.A > 0 {
			l.Path(paint.NewPath().Circle(p.X, p.Y, rad+ring+4), tk.HandleActiveOutlineColor)
		}
		l.Path(paint.NewPath().Circle(p.X, p.Y, rad+ring), ringC)
		l.Path(paint.NewPath().Circle(p.X, p.Y, rad), tk.HandleBg)
	}
}
