package ui

import (
	"math"

	"github.com/tangoui/tango/ui/a11y"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// RateW is a star rating (Ant Rate / Element el-rate).
type RateW struct {
	c          *Context
	n          *node
	val        *float64
	count      int
	half       bool
	allowClear bool
	disabled   bool
	char       string
	size       float32
	col        theme.Color
	colSet     bool
	texts      []string
	showScore  bool
	changed    bool
	hoverVal   float64
	ia         Interaction
	z          *lazy
	loc        *Locale
	th         *theme.Theme // Rate Alias applied
	tk         *theme.RateToken
}

// Rate adds a rating bound to *val (0..count, steps of 1 or 0.5).
func Rate(c *Context, val *float64) *RateW {
	w := &RateW{c: c, val: val, count: 5, allowClear: true, loc: c.Locale()}
	w.th = c.ThemeFor(theme.CompRate)
	w.tk = &w.th.Components.Rate
	w.n = c.add("rate", "")
	w.n.setRole(a11y.RoleSlider).setState(a11y.Horizontal).fill(w)
	w.n.ln.NoStretch = true
	w.n.ln.Measure = w.measure
	w.n.paint = w.paint
	w.n.ownKeys = true
	w.ia = c.interact(w.n, true, input.CursorPointer)
	w.configDefaults()
	w.z = c.lazy(w.n, w.finish)
	return w
}

// A11yLabel sets the accessible name (default: the Locale's "rating";
// inside a FormItem the item label is used).
func (w *RateW) A11yLabel(s string) *RateW { w.n.setName(s); return w }

func (w *RateW) semantics(s *a11y.Node) {
	if s.Name == "" {
		s.Name = w.loc.A11y.Rating
	}
	step := 1.0
	if w.half {
		step = 0.5
	}
	rangeSem(s, *w.val, 0, float64(w.count), step)
	s.Value = w.loc.Rating(*w.val, w.count)
	if w.dis() {
		s.States |= a11y.ReadOnly
	} else {
		s.Actions |= a11y.SetValue
	}
}

// Count sets the number of stars (default 5).
func (w *RateW) Count(n int) *RateW { w.count = max(1, n); return w }

// AllowHalf enables half-star values.
func (w *RateW) AllowHalf() *RateW { w.half = true; return w }

// AllowClear lets a click on the current value reset it to 0 (default on).
func (w *RateW) AllowClear(on bool) *RateW { w.allowClear = on; return w }

// Character replaces the star with a text character (e.g. "A", "好").
func (w *RateW) Character(s string) *RateW { w.char = s; return w }

// Size sets the star size in px (Ant 20, Element 18).
func (w *RateW) Size(px float32) *RateW { w.size = px; return w }

// Color overrides the active color.
func (w *RateW) Color(col theme.Color) *RateW { w.col, w.colSet = col, true; return w }

// Texts shows a description after the stars, one per star (Element show-text).
func (w *RateW) Texts(t ...string) *RateW { w.texts = t; return w }

// ShowScore shows the numeric value after the stars.
func (w *RateW) ShowScore() *RateW { w.showScore = true; return w }

// Disabled makes the rating read-only.
func (w *RateW) Disabled(on bool) *RateW {
	if on {
		w.disabled, w.n.focus, w.n.cursor = true, false, input.CursorDefault
	}
	return w
}

// Changed reports a new value this frame.
func (w *RateW) Changed() bool { w.z.run(); return w.changed }

// Hovered returns the value previewed under the pointer (0 = none).
func (w *RateW) Hovered() float64 { w.z.run(); return w.hoverVal }

func (w *RateW) dis() bool { return w.disabled || w.n.disabled }

func (w *RateW) starSize() float32 {
	if w.size > 0 {
		return w.size
	}
	return w.tk.StarSize
}

func (w *RateW) gap() float32 { return w.tk.StarGap }

func (w *RateW) height() float32 {
	if w.tk.Height > 0 {
		return w.tk.Height
	}
	return max(22, w.starSize()+2)
}

func (w *RateW) label(v float64) string {
	if w.showScore {
		return formatRateScore(v)
	}
	if len(w.texts) > 0 && v > 0 {
		i := int(math.Ceil(v)) - 1
		if i >= 0 && i < len(w.texts) {
			return w.texts[i]
		}
	}
	return ""
}

func formatRateScore(v float64) string {
	if v == math.Trunc(v) {
		return itoaEnt(int(v))
	}
	return itoaEnt(int(v)) + ".5"
}

func (w *RateW) labelWidth() float32 {
	mw := float32(0)
	st := w.c.style(w.th.FontSize, text.Regular)
	if w.showScore {
		mw = text.Width("0.5", st)
	}
	for _, t := range w.texts {
		mw = max(mw, text.Width(t, st))
	}
	if mw > 0 {
		mw += 8
	}
	return mw
}

func (w *RateW) measure(_, _ float32) geom.Size {
	s, g := w.starSize(), w.gap()
	return geom.Size{W: float32(w.count)*s + float32(w.count-1)*g + w.labelWidth(), H: w.height()}
}

// valueAt maps a pointer x (absolute) to a rating; stars fill from the
// inline start (the right edge in RTL).
func (w *RateW) valueAt(r geom.Rect, x float32) float64 {
	s, g := w.starSize(), w.gap()
	dx := w.c.inline(r).Off(x)
	if dx < 0 {
		return 0
	}
	i := int(dx / (s + g))
	if i >= w.count {
		return 0
	}
	frac := (dx - float32(i)*(s+g)) / s
	if frac > 1 { // in the gap: count as the full star
		frac = 1
	}
	if w.half && frac < 0.5 {
		return float64(i) + 0.5
	}
	return float64(i + 1)
}

func (w *RateW) set(v float64) {
	v = math.Max(0, math.Min(float64(w.count), v))
	if !w.half {
		v = math.Round(v)
	} else {
		v = math.Round(v*2) / 2
	}
	if v != *w.val {
		*w.val = v
		w.changed = true
		w.c.invalid = true
	}
}

func (w *RateW) finish() {
	if w.dis() {
		return
	}
	c := w.c
	if v, ok := c.takeSetNumber(w.n.id); ok {
		w.set(v)
	}
	r, ok := c.rects[w.n.id]
	if ok && w.ia.Hovered {
		w.hoverVal = w.valueAt(r, c.pointer.X)
	}
	if w.ia.Clicked && ok && c.pointer.X > 0 {
		v := w.valueAt(r, c.pointer.X)
		if v > 0 {
			if w.allowClear && v == *w.val {
				v = 0
			}
			w.set(v)
		}
	}
	step := 1.0
	if w.half {
		step = 0.5
	}
	for _, k := range c.KeysFor(w.n.id) {
		if !k.Down {
			continue
		}
		// The arrow toward the filled side's growth (the inline end)
		// increases: Left in RTL.
		prev, next := c.inlineKey(k.Key)
		switch {
		case next || k.Key == input.KeyUp:
			w.set(*w.val + step)
		case prev || k.Key == input.KeyDown:
			w.set(*w.val - step)
		}
		switch k.Key {
		case input.KeyHome:
			w.set(0)
		case input.KeyEnd:
			w.set(float64(w.count))
		}
	}
}

func (w *RateW) colors() (on, off theme.Color) {
	tk := w.tk
	on, off = tk.StarColor, tk.StarBg
	if w.dis() {
		off = tk.StarBgDisabled
	}
	if w.colSet {
		on = w.col
	}
	return on, off
}

func (w *RateW) paint(l *paint.List, r geom.Rect) {
	c, th, tk := w.c, w.th, w.tk
	s, g := w.starSize(), w.gap()
	on, off := w.colors()
	if th.Flavor == theme.StyleShadcn && w.dis() {
		l.Opacity(0.5) // disabled:opacity-50
		defer l.Pop()
	}
	v := *w.val
	hv := w.hoverVal
	if hv == 0 && !w.dis() && w.ia.Hovered {
		hv = w.valueAt(r, c.pointer.X)
	}
	if hv > 0 {
		v = hv
	}
	y := r.Y + (r.H-s)/2
	hoverIdx := -1
	if hv > 0 && tk.StarHoverScale != 1 {
		hoverIdx = int(math.Ceil(hv)) - 1
	}
	in := c.inline(r)
	for i := 0; i < w.count; i++ {
		x := in.X(float32(i)*(s+g), s)
		target := float32(1)
		if i == hoverIdx {
			target = tk.StarHoverScale
		}
		sc := c.Anim(w.n.id, "rate-s"+itoaEnt(i), target, th.DurationMid)
		sr := geom.Rect{X: x + s/2 - s*sc/2, Y: y + s/2 - s*sc/2, W: s * sc, H: s * sc}
		fill := v - float64(i)
		switch {
		case fill >= 1:
			w.drawStar(l, sr, on)
		case fill >= 0.5:
			w.drawStar(l, sr, off)
			// The half at the inline start.
			half := geom.Rect{X: sr.X - 1, Y: sr.Y - 1, W: sr.W/2 + 1, H: sr.H + 2}
			if c.IsRTL() {
				half.X = sr.X + sr.W/2
			}
			l.Clip(half, geom.Radii{})
			w.drawStar(l, sr, on)
			l.Pop()
		default:
			w.drawStar(l, sr, off)
		}
	}
	if w.ia.FocusVisible {
		fw := float32(w.count)*s + float32(w.count-1)*g
		full := in.Rect(0, y, fw, s)
		entFocusRing(c, l, full, 4)
	}
	if lb := w.label(v); lb != "" {
		off := float32(w.count)*s + float32(w.count-1)*g + 8
		col := tk.TextColor
		if w.showScore {
			col = tk.ScoreColor
			if col.A == 0 {
				col = on
			}
		}
		if w.dis() && tk.TextColorDisabled.A > 0 {
			col = tk.TextColorDisabled
		}
		entTextIn(c, l, lb, off, r, th.FontSize, text.Regular, col, 0)
	}
}

func (w *RateW) drawStar(l *paint.List, r geom.Rect, col theme.Color) {
	if w.char != "" {
		st := w.c.style(r.H, text.Regular)
		lay := text.Shape(w.char, st, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.X+(r.W-lay.Width)/2, r.Y+(r.H-lay.Height)/2), lay.Glyphs, col)
		return
	}
	if oc := w.tk.StarOutlineColor; oc.A > 0 && (col == w.tk.StarBg || col == w.tk.StarBgDisabled) && !w.colSet {
		// Outline stars for the voids (shadcn: lucide star).
		w.c.paintIcon(l, icon.Star, r.Expand(1), oc, 0)
		return
	}
	w.c.paintIcon(l, icon.StarFilled, r.Expand(1), col, 0)
}
