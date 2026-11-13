package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// g1Dir builds fn inside a ConfigProvider with direction dir.
func g1Dir(c *Context, rtl bool, fn func()) {
	dir := LTR
	if rtl {
		dir = RTL
	}
	Column(c, func() { ConfigProvider(c).Direction(dir).Body(fn) }, layout.Pad(20))
}

// g1Icons returns the translations of the icons drawn inside r.
func g1Icons(d *Driver, r geom.Rect) []geom.Point {
	var out []geom.Point
	for _, op := range d.r.List().Ops {
		if t, ok := op.(paint.PushTransform); ok {
			if p := geom.Pt(t.M.E, t.M.F); r.Contains(p) {
				out = append(out, p)
			}
		}
	}
	return out
}

// g1Texts returns the glyph runs drawn inside r.
func g1Texts(d *Driver, r geom.Rect) []paint.Text {
	var out []paint.Text
	for _, op := range d.r.List().Ops {
		if t, ok := op.(paint.Text); ok && r.Contains(geom.Pt(t.Origin.X+0.5, t.Origin.Y)) {
			out = append(out, t)
		}
	}
	return out
}

// g1FillsIn returns the FillRect ops of color col inside r.
func g1FillsIn(d *Driver, col theme.Color, r geom.Rect) []geom.Rect {
	var out []geom.Rect
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Color == col && r.Expand(1).Contains(geom.Pt(f.Rect.X+f.Rect.W/2, f.Rect.Y+f.Rect.H/2)) {
			out = append(out, f.Rect)
		}
	}
	return out
}

// TestG1LazyKeepsScope: a widget whose children are built lazily (at the
// end of the build pass, long after its ConfigProvider closed) still
// builds them with the scope's theme and direction.
func TestG1LazyKeepsScope(t *testing.T) {
	val := "b"
	var g *RadioGroupW
	var sawDark, sawRTL bool
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ConfigProvider(c).Dark(true).Direction(RTL).Body(func() {
				g = RadioGroup(c, &val, Opts("a", "b", "c"))
				n := c.add("probe-parent", "")
				c.lazy(n, func() { sawDark, sawRTL = c.Theme.Dark, c.IsRTL() })
			})
			Text(c, "after")
		})
	}, DriverOptions{Width: 400, Height: 200})
	d.Settle()
	if !sawDark || !sawRTL {
		t.Errorf("deferred build saw dark=%v rtl=%v, want the scope's (true, true)", sawDark, sawRTL)
	}
	if len(g.n.kids) != 3 {
		t.Fatalf("radio group built %d children", len(g.n.kids))
	}
	for _, k := range g.n.kids {
		if k.env == nil || !k.env.rtl || !k.env.th.Dark || !k.ln.RTL {
			t.Fatalf("lazy child %q not in the provider's scope (env=%v)", k.label, k.env)
		}
	}
	// Mirrored: the first option sits right of the second.
	a, b := d.r.c.rects[g.n.kids[0].id], d.r.c.rects[g.n.kids[1].id]
	if a.X <= b.X {
		t.Errorf("RTL radio group not mirrored: first at %v, second at %v", a.X, b.X)
	}
	if win := d.Runner().Context().Theme; win.Dark {
		t.Error("lazy build leaked the scope theme into the window")
	}
}

// TestG1ButtonIconRTL: a leading icon sits at the inline start of the
// button content (right of the label in RTL); a trailing one at the end.
func TestG1ButtonIconRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		for _, trailing := range []bool{false, true} {
			var id ID
			d := NewDriver(func(c *Context) {
				g1Dir(c, rtl, func() {
					b := Button(c, "Search")
					if trailing {
						b.IconRight(icon.Search)
					} else {
						b.Icon(icon.Search)
					}
					id = b.ID()
				})
			}, DriverOptions{Width: 400, Height: 200})
			d.Settle()
			r := d.r.c.rects[id]
			ics, txt := g1Icons(d, r), g1Texts(d, r)
			if len(ics) != 1 || len(txt) != 1 {
				t.Fatalf("rtl=%v: %d icons, %d texts", rtl, len(ics), len(txt))
			}
			iconRight := ics[0].X > txt[0].Origin.X
			if want := rtl != trailing; iconRight != want {
				t.Errorf("rtl=%v trailing=%v: icon right of label = %v", rtl, trailing, iconRight)
			}
		}
	}
}

// TestG1InputAffixesRTL: prefix at the inline start, clear button and
// suffix at the end; an empty field puts its placeholder at the start.
func TestG1InputAffixesRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		val, empty := "hello", ""
		var w, e *InputW
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() {
				w = Input(c, &val).Prefix(icon.Search).AllowClear().Width(240)
				e = Input(c, &empty).Placeholder("Type here").Width(240)
			})
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		r := d.r.c.rects[w.box.id]
		d.Move(geom.Pt(r.X+r.W/2, r.Y+r.H/2)) // show the clear button
		d.Settle()
		mid := r.X + r.W/2
		ics := g1Icons(d, r)
		if len(ics) == 0 {
			t.Fatalf("rtl=%v: no prefix icon", rtl)
		}
		if prefixRight := ics[0].X > mid; prefixRight != rtl {
			t.Errorf("rtl=%v: prefix icon at %v (box %v)", rtl, ics[0].X, r)
		}
		cr := w.st.clearRect
		if clearRight := cr.X > mid; clearRight == rtl {
			t.Errorf("rtl=%v: clear button at %v (box %v)", rtl, cr, r)
		}
		// The clear part node (hit area) follows the painted icon.
		var part geom.Rect
		for _, k := range w.box.kids {
			if k.label == "clear" {
				part = d.r.c.rects[k.id]
			}
		}
		if !part.Contains(geom.Pt(cr.X+cr.W/2, cr.Y+cr.H/2)) {
			t.Errorf("rtl=%v: clear hit area %v does not cover the icon %v", rtl, part, cr)
		}
		// The editor area sits between the affixes (mirrored padding).
		er := d.r.c.rects[w.ed.id]
		if edRight := er.X+er.W/2 > mid; edRight == rtl && er.W < r.W-60 {
			t.Errorf("rtl=%v: editor %v not shifted away from the prefix", rtl, er)
		}
		// Placeholder: start-aligned.
		er = d.r.c.rects[e.ed.id]
		ph := g1Texts(d, er.Expand(2))
		if len(ph) != 1 {
			t.Fatalf("rtl=%v: %d placeholder runs", rtl, len(ph))
		}
		left := ph[0].Origin.X
		if rtl && left < er.X+er.W/2 {
			t.Errorf("RTL placeholder starts at %v, want right-aligned in %v", left, er)
		}
		if !rtl && left != er.X {
			t.Errorf("LTR placeholder at %v, want %v", left, er.X)
		}
	}
}

// TestG1InputNumberRTL: the step handles sit at the inline end (left in
// RTL) while digits stay left to right.
func TestG1InputNumberRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		v := 42.0
		var w *InputNumberW
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() { w = InputNumber(c, &v).Width(120) })
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		box, h := d.r.c.rects[w.box.id], d.r.c.rects[w.handles.id]
		if handlesLeft := h.X < box.X+box.W/2; handlesLeft != rtl {
			t.Errorf("rtl=%v: handles at %v in %v", rtl, h, box)
		}
		// Up still increments (the keys are vertical).
		d.Click(geom.Pt(box.X+box.W/2, box.Y+box.H/2))
		d.Key(input.KeyUp, 0)
		d.Settle()
		if v != 43 {
			t.Errorf("rtl=%v: Up gave %v", rtl, v)
		}
	}
}

// TestG1SelectArrowRTL: the dropdown arrow sits at the inline end.
func TestG1SelectArrowRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		v := "Apple"
		var w *SelectW
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() { w = Select(c, &v, Opts("Apple", "Banana")).Width(200) })
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		r := d.r.c.rects[w.n.id]
		ics := g1Icons(d, r)
		if len(ics) != 1 {
			t.Fatalf("rtl=%v: %d icons", rtl, len(ics))
		}
		if arrowLeft := ics[0].X < r.X+r.W/2; arrowLeft != rtl {
			t.Errorf("rtl=%v: arrow at %v in %v", rtl, ics[0], r)
		}
		txt := g1Texts(d, r)
		if len(txt) != 1 {
			t.Fatalf("rtl=%v: %d value texts", rtl, len(txt))
		}
		if valueRight := txt[0].Origin.X > r.X+r.W/2; valueRight != rtl {
			t.Errorf("rtl=%v: value at %v in %v", rtl, txt[0].Origin, r)
		}
	}
}

// TestG1SwitchThumbRTL: an on switch has its thumb at the inline end.
func TestG1SwitchThumbRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		on := true
		var w *SwitchW
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() { w = Switch(c, &on) })
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		r := d.r.c.rects[w.n.id]
		th := d.r.c.Theme.Components.Switch
		thumbs := g1FillsIn(d, th.HandleBg, r)
		if len(thumbs) != 1 {
			t.Fatalf("rtl=%v: %d thumbs", rtl, len(thumbs))
		}
		if left := thumbs[0].X+thumbs[0].W/2 < r.X+r.W/2; left != rtl {
			t.Errorf("rtl=%v: on thumb at %v in %v", rtl, thumbs[0], r)
		}
	}
}

// TestG1CheckboxRadioRTL: the box / circle sits at the inline start, the
// label after it.
func TestG1CheckboxRadioRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		on := false
		var cb *CheckboxW
		var rd *RadioW
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() {
				cb = Checkbox(c, "Remember", &on)
				rd = Radio(c, "Choice", false)
			})
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		th := d.r.c.Theme
		for _, n := range []*node{cb.n, rd.n} {
			r := d.r.c.rects[n.id]
			txt := g1Texts(d, r)
			if len(txt) != 1 {
				t.Fatalf("rtl=%v %s: %d labels", rtl, n.label, len(txt))
			}
			boxRight := r.Right() - th.CheckboxSize
			if rtl && txt[0].Origin.X >= boxRight {
				t.Errorf("RTL %s: label at %v overlaps the box at %v", n.label, txt[0].Origin.X, boxRight)
			}
			if !rtl && txt[0].Origin.X < r.X+th.CheckboxSize {
				t.Errorf("LTR %s: label at %v", n.label, txt[0].Origin.X)
			}
		}
		// Clicking the box side toggles.
		r := d.r.c.rects[cb.n.id]
		x := r.X + 4
		if rtl {
			x = r.Right() - 4
		}
		d.Click(geom.Pt(x, r.Y+r.H/2))
		d.Settle()
		if !on {
			t.Errorf("rtl=%v: click on the box did not toggle", rtl)
		}
	}
}

// TestG1RateRTL: stars fill from the inline start; the arrow toward the
// end (Left in RTL) increases.
func TestG1RateRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		v := 0.0
		var w *RateW
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() { w = Rate(c, &v) })
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		r := d.r.c.rects[w.n.id]
		// The first star is at the start edge.
		x := r.X + 4
		if rtl {
			x = r.Right() - 4
		}
		d.Click(geom.Pt(x, r.Y+r.H/2))
		d.Settle()
		if v != 1 {
			t.Fatalf("rtl=%v: click on the start star gave %v", rtl, v)
		}
		inc, dec := input.KeyRight, input.KeyLeft
		if rtl {
			inc, dec = dec, inc
		}
		d.Key(inc, 0)
		d.Settle()
		if v != 2 {
			t.Errorf("rtl=%v: increase key gave %v", rtl, v)
		}
		d.Key(dec, 0)
		d.Key(dec, 0)
		d.Settle()
		if v != 0 {
			t.Errorf("rtl=%v: decrease key gave %v", rtl, v)
		}
	}
}

// TestG1RateHalfRTL: a half star shows its start half filled.
func TestG1RateHalfRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		v := 0.5
		var w *RateW
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() { w = Rate(c, &v).AllowHalf().Count(1) })
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		r := d.r.c.rects[w.n.id]
		var clip geom.Rect
		for _, op := range d.r.List().Ops {
			if c, ok := op.(paint.PushClip); ok && r.Expand(2).Contains(geom.Pt(c.Rect.X+c.Rect.W/2, c.Rect.Y+c.Rect.H/2)) {
				clip = c.Rect
			}
		}
		if clip.Empty() {
			t.Fatalf("rtl=%v: no half-star clip", rtl)
		}
		if right := clip.X+clip.W/2 > r.X+r.H/2; right != rtl {
			t.Errorf("rtl=%v: half clip %v in %v", rtl, clip, r)
		}
	}
}

// TestG1SegmentedRTL: the items mirror, the thumb follows the selection
// and Left moves to the next item in RTL.
func TestG1SegmentedRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		v := "A"
		var w *SegmentedW
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() { w = Segmented(c, &v, Opts("A", "B", "C")) })
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		a := d.r.c.rects[w.items[0].id]
		th := d.r.c.Theme.Components.Segmented
		thumbs := g1FillsIn(d, th.ItemSelectedBg, d.r.c.rects[w.n.id])
		if len(thumbs) != 1 || !near(thumbs[0].X, a.X) {
			t.Fatalf("rtl=%v: thumb %v, selected item %v", rtl, thumbs, a)
		}
		d.r.c.SetFocus(w.n.id)
		next := input.KeyRight
		if rtl {
			next = input.KeyLeft
		}
		d.Key(next, 0)
		d.Settle()
		if v != "B" {
			t.Errorf("rtl=%v: next key selected %q", rtl, v)
		}
	}
}

// TestG1SelectorTagCloseRTL: a multiple select's chip puts its close icon
// at the inline end and the hit test follows.
func TestG1SelectorTagCloseRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		vals := []string{"Apple", "Cherry"}
		var w *MultiSelectW
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() { w = MultiSelect(c, &vals, Opts("Apple", "Banana", "Cherry")).Width(260) })
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		tag, ok := d.Find("Apple")
		if !ok {
			t.Fatal("no chip")
		}
		x := tag.Right() - 10
		if rtl {
			x = tag.X + 10
		}
		d.Click(geom.Pt(x, tag.Y+tag.H/2))
		d.Settle()
		if len(vals) != 1 || vals[0] != "Cherry" {
			t.Errorf("rtl=%v: click on the close side left %v", rtl, vals)
		}
		_ = w
	}
}

// TestG1RadioButtonsRTL: the outer corners of a button radio group follow
// the inline direction (the first option is rounded on its right in RTL).
func TestG1RadioButtonsRTL(t *testing.T) {
	for _, rtl := range []bool{false, true} {
		v := "a"
		var g *RadioGroupW
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() { g = RadioGroup(c, &v, Opts("a", "b")).Buttons() })
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		first := d.r.c.rects[g.n.kids[0].id]
		var rad geom.Radii
		for _, op := range d.r.List().Ops {
			if f, ok := op.(paint.FillRect); ok && f.Rect == first {
				rad = f.Radii
			}
		}
		if rtl && (rad.TR == 0 || rad.TL != 0) || !rtl && (rad.TL == 0 || rad.TR != 0) {
			t.Errorf("rtl=%v: first button radii %+v", rtl, rad)
		}
	}
}

// TestG1TypographySuffixRTL: Copyable's icon follows the text at its
// inline end (left of it in RTL) and its focusable button follows.
func TestG1TypographySuffixRTL(t *testing.T) {
	for _, tc := range []struct{ rtl, wrap bool }{{false, false}, {true, false}, {false, true}, {true, true}} {
		rtl := tc.rtl
		var lb *Label
		d := NewDriver(func(c *Context) {
			g1Dir(c, rtl, func() {
				if tc.wrap { // the label's parent in the RTL scope too
					Column(c, func() { lb = Text(c, "Copy me").Copyable() }, layout.Pad(5))
				} else {
					lb = Text(c, "Copy me").Copyable()
				}
			})
		}, DriverOptions{Width: 400, Height: 200})
		d.Settle()
		d.Settle()
		st := lb.typo()
		r := d.r.c.rects[lb.n.id]
		txt := g1Texts(d, r)
		if len(st.rects) != 1 || len(txt) != 1 {
			t.Fatalf("rtl=%v: %d icons, %d texts", rtl, len(st.rects), len(txt))
		}
		ir := st.rects[0]
		if iconLeft := ir.X < txt[0].Origin.X; iconLeft != rtl {
			t.Errorf("rtl=%v: copy icon %v, text at %v", rtl, ir, txt[0].Origin)
		}
		covered := false
		for _, k := range lb.n.parent.kids {
			if k != lb.n && d.r.c.rects[k.id].Contains(geom.Pt(ir.X+ir.W/2, ir.Y+ir.H/2)) {
				covered = true
			}
		}
		if !covered {
			t.Errorf("rtl=%v: no copy button over the icon %v", rtl, ir)
		}
	}
}
