package ui

// Spec alignment batch 1, group B (inputs and forms): behaviour added or
// corrected against antd 6.7 / Element Plus 2.14.8 / shadcn v4. Records:
// docs/specs/align/1-input.md.

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// b1bThemes runs fn under the three design systems.
func b1bThemes(t *testing.T, fn func(t *testing.T, th *theme.Theme)) {
	t.Helper()
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		t.Run(th.Flavor.String(), func(t *testing.T) { fn(t, th) })
	}
}

func b1bRect(d *Driver, id ID) geom.Rect { return d.r.c.rects[id] }

// b1bOps counts the paint ops of color col inside r (zero r: anywhere).
func b1bOps(d *Driver, col theme.Color, r geom.Rect) int {
	n := 0
	in := func(b geom.Rect) bool {
		return r.W == 0 || r.Expand(1).Contains(geom.Pt(b.X+b.W/2, b.Y+b.H/2))
	}
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Color == col && in(o.Rect) {
				n++
			}
		case paint.StrokeRect:
			if o.Color == col && in(o.Rect) {
				n++
			}
		case paint.FillPath:
			if o.Color == col && o.Path != nil && in(o.Path.Bounds()) {
				n++
			}
		case paint.Text:
			if o.Color == col && (r.W == 0 || r.Expand(1).Contains(o.Origin)) {
				n++
			}
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// InputNumber

// TestInputNumberSpinnerMode: antd 6 mode="spinner" puts − before and +
// after the value; + steps 1 → 3 and is disabled at Max.
func TestInputNumberSpinnerMode(t *testing.T) {
	b1bThemes(t, func(t *testing.T, th *theme.Theme) {
		v := 1.0
		var w *InputNumberW
		var upOff, downOff bool
		d := p2dDriver(th, func(c *Context) {
			w = InputNumber(c, &v).Mode("spinner").Min(0).Max(3).A11yLabel("n")
			upOff, downOff = w.UpDisabled(), w.DownDisabled()
		})
		if w.ctrlPos != "sides" {
			t.Fatalf("spinner mode: ctrlPos %q", w.ctrlPos)
		}
		box, up, dn := b1bRect(d, w.box.id), b1bRect(d, w.up.id), b1bRect(d, w.down.id)
		if dn.X != box.X || up.Right() != box.Right() {
			t.Fatalf("spinner layout: box %v up %v down %v", box, up, dn)
		}
		if sw := th.Components.InputNumber.SideHandleWidth; sw > 0 && up.W != sw {
			t.Fatalf("side button width %v, want token %v", up.W, sw)
		}
		d.Click(up.Center())
		d.Click(up.Center())
		if v != 3 {
			t.Fatalf("after two +: %v, want 3", v)
		}
		if !upOff || downOff {
			t.Fatalf("at Max: up disabled %v (want true), down disabled %v (want false)", upOff, downOff)
		}
		if !w.up.disabled {
			t.Fatal("+ node not disabled at Max")
		}
		d.Click(up.Center())
		if v != 3 {
			t.Fatalf("+ at Max changed the value to %v", v)
		}
		d.Click(dn.Center())
		if v != 2 {
			t.Fatalf("−: %v", v)
		}
	})
}

// TestInputNumberModeDefaults: ConfigProvider Defaults "mode" and
// "controls"; Mode("input") overrides Element's sides default.
func TestInputNumberModeDefaults(t *testing.T) {
	v := 1.0
	var a, b, e *InputNumberW
	p2dDriver(theme.ElementPlus(), func(c *Context) {
		ConfigProvider(c).Defaults("InputNumber", Props{"mode": "spinner", "controls": false}).Body(func() {
			a = InputNumber(c, &v).A11yLabel("a")
		})
		b = InputNumber(c, &v).Mode("input").A11yLabel("b")
		e = InputNumber(c, &v).A11yLabel("e")
	})
	if !a.noControls {
		t.Fatal("controls default not applied")
	}
	if b.ctrlPos != "right" || e.ctrlPos != "sides" {
		t.Fatalf("mode input %q, element default %q", b.ctrlPos, e.ctrlPos)
	}
	var s *InputNumberW
	p2dDriver(theme.AntDesign(), func(c *Context) {
		ConfigProvider(c).Defaults("InputNumber", Props{"mode": "spinner"}).Body(func() {
			s = InputNumber(c, &v).A11yLabel("s")
		})
	})
	if s.ctrlPos != "sides" {
		t.Fatalf("mode default: %q", s.ctrlPos)
	}
}

// TestInputNumberControlsHidden: Controls(false) removes the step buttons;
// the arrow keys still step (antd controls={false}); Keyboard(false) stops
// them.
func TestInputNumberControlsHidden(t *testing.T) {
	b1bThemes(t, func(t *testing.T, th *theme.Theme) {
		v, k := 1.0, 1.0
		var w, wk *InputNumberW
		d := p2dDriver(th, func(c *Context) {
			w = InputNumber(c, &v).Controls(false).A11yLabel("a")
			wk = InputNumber(c, &k).Keyboard(false).A11yLabel("b")
		})
		if r := b1bRect(d, w.handles.id); r.W != 0 {
			t.Fatalf("handles still %v wide", r.W)
		}
		if !w.UpDisabled() {
			t.Fatal("UpDisabled false without controls")
		}
		d.Click(b1bRect(d, w.ed.id).Center())
		d.Key(input.KeyUp, 0)
		if v != 2 {
			t.Fatalf("arrow up without controls: %v", v)
		}
		d.Click(b1bRect(d, wk.ed.id).Center())
		d.Key(input.KeyUp, 0)
		if k != 1 {
			t.Fatalf("Keyboard(false): arrow up changed %v", k)
		}
	})
}

// TestInputNumberReadOnlyA11y: ReadOnly blocks typing, keys and the
// (hidden) step buttons, and reports ReadOnly to assistive technology.
func TestInputNumberReadOnlyA11y(t *testing.T) {
	v := 5.0
	var w *InputNumberW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		w = InputNumber(c, &v).ReadOnly().A11yLabel("只读")
	})
	d.EnableAccessibility()
	d.Settle()
	d.Click(b1bRect(d, w.ed.id).Center())
	d.Key(input.KeyUp, 0)
	d.Type("9")
	d.Key(input.KeyEnter, 0)
	if v != 5 {
		t.Fatalf("read-only value changed to %v", v)
	}
	n, ok := d.A11yFind(a11y.RoleSpinButton, "只读")
	if !ok || n.States&a11y.ReadOnly == 0 || n.Actions&a11y.SetValue != 0 {
		t.Fatalf("spin button semantics %+v", n)
	}
}

// TestInputNumberDisabledSpinner: a disabled or read-only field ignores
// its buttons (Review focus 2). antd leaves them out (mergedControls =
// controls && !disabled && !readOnly) and keeps the spinner value
// centred; Element keeps them, inactive.
func TestInputNumberDisabledSpinner(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus()} {
		t.Run(th.Flavor.String(), func(t *testing.T) {
			hide := th.Components.InputNumber.HideControlsInactive != 0
			for _, ro := range []bool{false, true} {
				v := 1.0
				var w *InputNumberW
				var upOff bool
				d := p2dDriver(th, func(c *Context) {
					w = InputNumber(c, &v).Mode("spinner").Width(150).A11yLabel("n")
					if ro {
						w.ReadOnly()
					} else {
						w.Disabled(true)
					}
					upOff = w.UpDisabled()
				})
				if !upOff {
					t.Fatalf("readOnly=%v: UpDisabled false", ro)
				}
				up := b1bRect(d, w.up.id)
				shown := up.W > 0 && len(w.box.kids) == 3
				if shown == hide {
					t.Fatalf("readOnly=%v: buttons shown %v, theme hides %v", ro, shown, hide)
				}
				if shown {
					if !w.up.disabled {
						t.Fatalf("readOnly=%v: shown + not disabled", ro)
					}
					d.Click(up.Center())
				} else {
					// The value stays centred in the field.
					box, ed := b1bRect(d, w.box.id), b1bRect(d, w.ed.id)
					if l, r := ed.X-box.X, box.Right()-ed.Right(); l-r > 1 || r-l > 1 {
						t.Fatalf("readOnly=%v: editor %v not centred in %v", ro, ed, box)
					}
					d.Click(geom.Pt(box.Right()-5, box.Y+box.H/2))
				}
				if v != 1 {
					t.Fatalf("readOnly=%v: value changed to %v", ro, v)
				}
			}
		})
	}
	v := 1.0
	var cp *InputNumberW
	d2 := p2dDriver(theme.ElementPlus(), func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() {
			cp = InputNumber(c, &v).Mode("spinner").A11yLabel("m")
		})
	})
	d2.Click(b1bRect(d2, cp.up.id).Center())
	if v != 1 || !cp.UpDisabled() {
		t.Fatalf("ComponentDisabled spinner: value %v", v)
	}
}

// TestInputNumberReadOnlyOutOfRange: a read-only field never rewrites its
// value: no clamping, precision or step rounding on blur or Enter
// (rc-input-number: updates only when !readOnly && !disabled).
func TestInputNumberReadOnlyOutOfRange(t *testing.T) {
	v := 99.4
	var w *InputNumberW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		w = InputNumber(c, &v).Min(0).Max(10).Precision(0).Step(2).StepStrictly().ReadOnly().A11yLabel("n")
	})
	d.Click(b1bRect(d, w.ed.id).Center())
	d.Key(input.KeyEnter, 0)
	d.Click(geom.Pt(850, 650))
	d.Click(b1bRect(d, w.ed.id).Center())
	d.Click(geom.Pt(850, 650))
	if v != 99.4 {
		t.Fatalf("read-only value rewritten to %v", v)
	}
}

// TestInputNumberOutOfRange: a bound value above Max shows as is (no
// panic), + is disabled and − clamps into the range (rc-input-number
// getRangeValue).
func TestInputNumberOutOfRange(t *testing.T) {
	v := 99.0
	var w *InputNumberW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		w = InputNumber(c, &v).Mode("spinner").Min(0).Max(10).A11yLabel("n")
	})
	if s := w.display(); s != "99" {
		t.Fatalf("display %q", s)
	}
	if !w.UpDisabled() || w.DownDisabled() {
		t.Fatal("limits above Max")
	}
	d.Click(b1bRect(d, w.down.id).Center())
	if v != 10 {
		t.Fatalf("− from 99 with Max 10: %v", v)
	}
}

// TestInputNumberStepStrictly: Element step-strictly rounds a typed value
// to a multiple of the step when committed.
func TestInputNumberStepStrictly(t *testing.T) {
	v := 2.0
	var w *InputNumberW
	d := p2dDriver(theme.ElementPlus(), func(c *Context) {
		w = InputNumber(c, &v).Step(2).StepStrictly().A11yLabel("n")
	})
	d.Click(b1bRect(d, w.ed.id).Center())
	d.Key(input.KeyEnd, 0)
	d.Key(input.KeyBackspace, 0)
	d.Type("5")
	d.Key(input.KeyEnter, 0)
	if v != 6 {
		t.Fatalf("step-strictly commit 5 → %v, want 6", v)
	}
	// JS Math.round: -5/2 = -2.5 rounds up to -2 → -4.
	d.Key(input.KeyEnd, 0)
	d.Key(input.KeyBackspace, 0)
	d.Type("-5")
	d.Key(input.KeyEnter, 0)
	if v != -4 {
		t.Fatalf("step-strictly commit -5 → %v, want -4", v)
	}
}

// ---------------------------------------------------------------------------
// OTP

// TestOTPSeparator: length 6 with "-" shows five separators; a function
// shows one only after the third box.
func TestOTPSeparator(t *testing.T) {
	b1bThemes(t, func(t *testing.T, th *theme.Theme) {
		a, b := "", ""
		var wa, wb *OTPW
		d := p2dDriver(th, func(c *Context) {
			wa = OTP(c, &a, 6).Separator("-")
			wb = OTP(c, &b, 6).SeparatorFunc(func(i int) string {
				if i == 2 {
					return "-"
				}
				return ""
			})
		})
		count := func(w *OTPW) int {
			n := 0
			for _, s := range w.seps {
				if b1bRect(d, s.id).W > 0 {
					n++
				}
			}
			return n
		}
		if n := count(wa); n != 5 || len(wa.seps) != 5 {
			t.Fatalf("separators shown %d of %d, want 5", n, len(wa.seps))
		}
		if n := count(wb); n != 1 {
			t.Fatalf("function separators shown %d, want 1", n)
		}
		// The separator sits between box i and box i+1.
		s0, c0, c1 := b1bRect(d, wa.seps[0].id), b1bRect(d, wa.cells[0].id), b1bRect(d, wa.cells[1].id)
		if !(s0.X >= c0.Right() && s0.Right() <= c1.X) {
			t.Fatalf("separator %v not between %v and %v", s0, c0, c1)
		}
		// An empty separator slot does not widen the gap.
		g1 := b1bRect(d, wb.cells[1].id).X - b1bRect(d, wb.cells[0].id).Right()
		g0 := b1bRect(d, wa.cells[1].id).X - b1bRect(d, wa.cells[0].id).Right() - s0.W
		if th.Flavor != theme.StyleShadcn && g1 > g0 {
			t.Fatalf("empty slot gap %v > separator gaps %v", g1, g0)
		}
		// Typing still fills the boxes in order.
		d.Click(b1bRect(d, wa.cells[0].id).Center())
		d.Type("123456")
		if a != "123456" {
			t.Fatalf("typed %q", a)
		}
	})
}

// TestOTPReadOnlyA11y: ReadOnly keeps the code and marks every box
// read-only; the boxes stay focusable.
func TestOTPReadOnlyA11y(t *testing.T) {
	v := "12"
	var w *OTPW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		w = OTP(c, &v, 4).ReadOnly().A11yLabel("code")
	})
	d.EnableAccessibility()
	d.Settle()
	d.Click(b1bRect(d, w.cells[2].id).Center())
	d.Type("9")
	d.Key(input.KeyBackspace, 0)
	if v != "12" {
		t.Fatalf("read-only code changed to %q", v)
	}
	if d.r.c.focus != w.cells[2].id {
		t.Fatal("read-only box not focusable")
	}
	n, ok := d.A11yFind(a11y.RoleTextbox, ZhCN.Digit(3, 4))
	if !ok || n.States&a11y.ReadOnly == 0 {
		t.Fatalf("box semantics %+v", n)
	}
}

// TestOTPVariantUnderlined: Variant("underlined") draws only a bottom line
// under each box (antd variant, Element type).
func TestOTPVariantUnderlined(t *testing.T) {
	v := ""
	var w *OTPW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		w = OTP(c, &v, 4).Variant("underlined")
	})
	b := b1bRect(d, w.cells[0].id)
	line := false
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Rect.H == 1 && o.Rect.Y == b.Bottom()-1 && o.Rect.X == b.X {
				line = true
			}
		case paint.StrokeRect:
			if o.Rect == b {
				t.Fatal("underlined box stroked all round")
			}
		}
	}
	if !line {
		t.Fatal("no bottom line")
	}
}

// ---------------------------------------------------------------------------
// Input

// TestInputVariantUnderlined: antd 6 variant="underlined" (also through
// ConfigProvider Defaults) draws a bottom border only, no radius.
func TestInputVariantUnderlined(t *testing.T) {
	b1bThemes(t, func(t *testing.T, th *theme.Theme) {
		a, b := "x", "y"
		var wa, wb *InputW
		d := p2dDriver(th, func(c *Context) {
			wa = Input(c, &a).Variant("underlined").Width(200)
			ConfigProvider(c).Defaults("Input", Props{"variant": "underlined"}).Body(func() {
				wb = Input(c, &b).Width(200)
			})
		})
		for _, w := range []*InputW{wa, wb} {
			r := b1bRect(d, w.box.id)
			line := false
			for _, op := range d.r.List().Ops {
				switch o := op.(type) {
				case paint.FillRect:
					if o.Rect.H == 1 && o.Rect.Y == r.Bottom()-1 && o.Rect.W == r.W {
						line = true
					}
				case paint.StrokeRect:
					if o.Rect == r {
						t.Fatal("underlined input stroked all round")
					}
				}
			}
			if !line {
				t.Fatalf("no bottom line under %v", r)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// Slider

// TestSliderReverse: antd reverse puts min at the inline end; Right
// decreases the value.
func TestSliderReverse(t *testing.T) {
	v := 30.0
	var w *SliderW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		w = Slider(c, &v, 0, 100).Reverse().A11yLabel("s").Layout(layout.W(400))
	})
	tr, h := b1bRect(d, w.track.id), b1bRect(d, w.thumbs[0].id)
	if cx := h.X + h.W/2; cx < tr.X+tr.W/2 {
		t.Fatalf("reversed 30%% handle at %v, left of the centre of %v", cx, tr)
	}
	d.Click(h.Center())
	d.Key(input.KeyRight, 0)
	if v != 29 {
		t.Fatalf("Right in reverse: %v, want 29", v)
	}
	// A press near the inline start picks a value near max.
	d.Click(geom.Pt(tr.X+sliderPad+1, tr.Y+sliderThick/2))
	if v < 95 {
		t.Fatalf("press at the start of a reversed slider: %v", v)
	}
}

// TestSliderIncludedFalse: included={false} leaves the rail unfilled.
func TestSliderIncludedFalse(t *testing.T) {
	v := 50.0
	th := theme.AntDesign()
	tk := th.Components.Slider
	d := p2dDriver(th, func(c *Context) {
		Slider(c, &v, 0, 100).Included(false).A11yLabel("s")
	})
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.FillRect); ok && (o.Color == tk.TrackBg || o.Color == tk.TrackHoverBg) {
			t.Fatalf("track filled %v", o.Rect)
		}
	}
	v2 := 50.0
	d2 := p2dDriver(th, func(c *Context) { Slider(c, &v2, 0, 100).A11yLabel("s") })
	if b1bOps(d2, tk.TrackBg, geom.Rect{}) == 0 {
		t.Fatal("default slider has no track fill")
	}
}

// TestSliderChangeComplete: antd onChangeComplete fires once when the
// pointer is released after a drag, and for a keyboard step; a disabled
// slider never reports it.
func TestSliderChangeComplete(t *testing.T) {
	v := 10.0
	var w *SliderW
	changes, completes := 0, 0
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		w = Slider(c, &v, 0, 100).A11yLabel("s").Layout(layout.W(400))
		if w.Changed() {
			changes++
		}
		if w.ChangeComplete() {
			completes++
		}
	})
	h := b1bRect(d, w.thumbs[0].id)
	tr := b1bRect(d, w.track.id)
	d.Drag(h.Center(), geom.Pt(tr.X+tr.W*0.6, h.Center().Y))
	d.Settle()
	if changes == 0 || completes != 1 {
		t.Fatalf("drag: %d changes, %d completes (want ≥1, 1)", changes, completes)
	}
	d.Key(input.KeyRight, 0)
	if completes != 2 {
		t.Fatalf("keyboard step: %d completes", completes)
	}
	kv := 10.0
	var kw *SliderW
	dk := p2dDriver(theme.AntDesign(), func(c *Context) {
		kw = Slider(c, &kv, 0, 100).Keyboard(false).A11yLabel("k").Layout(layout.W(400))
	})
	dk.Click(b1bRect(dk, kw.thumbs[0].id).Center())
	before := kv
	dk.Key(input.KeyRight, 0)
	if kv != before {
		t.Fatalf("Keyboard(false): Right moved %v → %v", before, kv)
	}
	dv := 10.0
	dc := 0
	var dw *SliderW
	d2 := p2dDriver(theme.AntDesign(), func(c *Context) {
		dw = Slider(c, &dv, 0, 100).Disabled(true).A11yLabel("d").Layout(layout.W(400))
		if dw.ChangeComplete() {
			dc++
		}
	})
	dh := b1bRect(d2, dw.thumbs[0].id)
	d2.Drag(dh.Center(), dh.Center().Add(geom.Pt(100, 0)))
	d2.Settle()
	if dv != 10 || dc != 0 {
		t.Fatalf("disabled slider: value %v, completes %d", dv, dc)
	}
}

// ---------------------------------------------------------------------------
// Radio

// TestRadioGroupVerticalA11y: antd 6 vertical / orientation stacks the
// options (buttons join top to bottom) and reports a vertical radio group.
func TestRadioGroupVerticalA11y(t *testing.T) {
	b1bThemes(t, func(t *testing.T, th *theme.Theme) {
		a, b := "x", "x"
		opts := Opts("x", "yy", "zzz")
		var ga, gb *RadioGroupW
		d := p2dDriver(th, func(c *Context) {
			ga = RadioGroup(c, &a, opts).Vertical().A11yLabel("plain")
			gb = RadioGroup(c, &b, opts).Orientation("vertical").Buttons().A11yLabel("buttons")
		})
		d.EnableAccessibility()
		d.Settle()
		for _, g := range []*RadioGroupW{ga, gb} {
			k := g.n.kids
			r0, r1, r2 := b1bRect(d, k[0].id), b1bRect(d, k[1].id), b1bRect(d, k[2].id)
			if !(r1.Y > r0.Y && r2.Y > r1.Y && r0.X == r1.X && r1.X == r2.X) {
				t.Fatalf("not stacked: %v %v %v", r0, r1, r2)
			}
		}
		k := gb.n.kids
		if b1bRect(d, k[0].id).W != b1bRect(d, k[2].id).W {
			t.Fatal("vertical buttons not equally wide")
		}
		if b1bRect(d, k[1].id).Y != b1bRect(d, k[0].id).Bottom()-1 {
			t.Fatal("vertical buttons do not share their border")
		}
		n, ok := d.A11yFind(a11y.RoleRadioGroup, "plain")
		if !ok || n.States&a11y.Vertical == 0 || n.States&a11y.Horizontal != 0 {
			t.Fatalf("radio group semantics %+v", n)
		}
		// Arrow keys still move the selection.
		d.Click(b1bRect(d, ga.n.kids[0].id).Center())
		d.Key(input.KeyDown, 0)
		if a != "yy" {
			t.Fatalf("Down in a vertical group: %q", a)
		}
	})
}

// TestRadioGroupBlock: block shares the full width equally.
func TestRadioGroupBlock(t *testing.T) {
	v := "a"
	var g *RadioGroupW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		Column(c, func() {
			g = RadioGroup(c, &v, Opts("a", "bbbbbbbb", "c")).Buttons().Block()
		}, layout.W(480))
	})
	k := g.n.kids
	gr := b1bRect(d, g.n.id)
	if gr.W < 470 {
		t.Fatalf("block group %v wide", gr.W)
	}
	w0, w1 := b1bRect(d, k[0].id).W, b1bRect(d, k[1].id).W
	if w0-w1 > 1 || w1-w0 > 1 {
		t.Fatalf("unequal widths %v %v", w0, w1)
	}
}

// TestRadioDotSize: antd 6 dotSize 6 on a 16px radio; a disabled checked
// radio keeps the dot size (dotColorDisabled).
func TestRadioDotSize(t *testing.T) {
	th := theme.AntDesign()
	d := p2dDriver(th, func(c *Context) {
		Radio(c, "a", true)
		Radio(c, "b", true).Disabled(true)
	})
	found, dis := false, false
	for _, op := range d.r.List().Ops {
		o, ok := op.(paint.FillPath)
		if !ok || o.Path == nil {
			continue
		}
		b := o.Path.Bounds()
		if o.Color == th.BgContainer && b.W > 5.9 && b.W < 6.1 {
			found = true
		}
		if o.Color == th.TextDisabled && b.W > 5.9 && b.W < 6.1 {
			dis = true
		}
	}
	if !found || !dis {
		t.Fatalf("6px dot: checked %v, disabled %v", found, dis)
	}
}

// ---------------------------------------------------------------------------
// Switch

// ---------------------------------------------------------------------------
// Upload

// TestUploadMaxCountOneReplaces: antd maxCount={1} replaces the current
// file with the newly picked one and keeps the trigger enabled.
func TestUploadMaxCountOneReplaces(t *testing.T) {
	dir := t.TempDir()
	mk := func(name string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a, b := mk("a.txt"), mk("b.txt")
	next := a
	files := []UploadFile{}
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		Upload(c, &files).MaxCount(1).Picker(func(string, bool, bool) ([]string, error) { return []string{next}, nil })
	})
	if err := d.ClickLabel(ZhCN.Upload.Button); err != nil {
		t.Fatal(err)
	}
	p2dWait(t, d, "first file", func() bool { return len(files) == 1 })
	next = b
	if err := d.ClickLabel(ZhCN.Upload.Button); err != nil {
		t.Fatal(err)
	}
	p2dWait(t, d, "replacement", func() bool { return len(files) == 1 && files[0].Name == "b.txt" })
	if len(files) != 1 || files[0].Path != b {
		t.Fatalf("files %+v", files)
	}
}

// ---------------------------------------------------------------------------
// Form

// TestInputPasswordVisibilityToggleA11y: antd visibilityToggle={false}
// (Element type="password" without show-password) drops the eye button.
func TestInputPasswordVisibilityToggleA11y(t *testing.T) {
	a, b := "secret", "secret"
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		Input(c, &a).Password().A11yLabel("p1")
		Input(c, &b).Password().VisibilityToggle(false).A11yLabel("p2")
	})
	d.EnableAccessibility()
	d.Settle()
	if n := len(d.A11yFindAll(a11y.RoleButton, ZhCN.A11y.ShowPassword)); n != 1 {
		t.Fatalf("%d eye buttons, want 1", n)
	}
}

// TestCompactSize: antd Space.Compact size applies to the controls that
// do not set their own.
func TestCompactSize(t *testing.T) {
	th := theme.AntDesign()
	a, b := "", ""
	var wa, wb *InputW
	d := p2dDriver(th, func(c *Context) {
		Compact(c).Size(Large).Body(func() {
			wa = Input(c, &a).Width(120)
			wb = Input(c, &b).Size(Small).Width(120)
			Button(c, "Go")
		})
	})
	if h := b1bRect(d, wa.box.id).H; h != th.ControlHeightLG {
		t.Fatalf("compact large input height %v", h)
	}
	if h := b1bRect(d, wb.box.id).H; h != th.ControlHeightSM {
		t.Fatalf("explicit small input height %v", h)
	}
}

// TestUploadShowUploadListFalse: antd showUploadList={false} hides the
// file list (text and card lists), the trigger stays.
func TestUploadShowUploadListFalse(t *testing.T) {
	files := []UploadFile{{Name: "a.pdf", Status: "done"}}
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		Upload(c, &files).ShowUploadList(false)
		ConfigProvider(c).Defaults("Upload", Props{"showUploadList": false}).Body(func() {
			Upload(c, &files).PictureCard()
		})
	})
	if _, ok := d.FindText("a.pdf"); ok {
		t.Fatal("file list shown")
	}
	if _, ok := d.FindText(ZhCN.Upload.Button); !ok {
		t.Fatal("trigger hidden")
	}
}

// TestTextAreaVariantReadOnlyA11y: antd TextArea variant (also from the
// TextArea / Input Defaults) and readOnly.
func TestTextAreaVariantReadOnlyA11y(t *testing.T) {
	a, b := "hello", "x"
	var wa, wb *TextAreaW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		wa = TextArea(c, &a).Variant("underlined").ReadOnly().A11yLabel("ta").Width(240)
		ConfigProvider(c).Defaults("TextArea", Props{"variant": "filled"}).Body(func() {
			wb = TextArea(c, &b).Width(240)
		})
	})
	d.EnableAccessibility()
	d.Settle()
	if wb.variant != "filled" {
		t.Fatalf("Defaults variant %q", wb.variant)
	}
	r := b1bRect(d, wa.box.id)
	line := false
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.FillRect); ok && o.Rect.H == 1 && o.Rect.Y == r.Bottom()-1 && o.Rect.W == r.W {
			line = true
		}
	}
	if !line {
		t.Fatal("underlined textarea: no bottom line")
	}
	d.Click(b1bRect(d, wa.ed.id).Center())
	d.Type("zz")
	if a != "hello" {
		t.Fatalf("read-only textarea changed to %q", a)
	}
	n, ok := d.A11yFind(a11y.RoleTextbox, "ta")
	if !ok || n.States&a11y.ReadOnly == 0 {
		t.Fatalf("textarea semantics %+v", n)
	}
}

// b1bIconAt reports whether a path of color col is drawn under a
// translation that lands inside r (icons are drawn at local coordinates
// inside a PushTransform).
func b1bIconAt(d *Driver, col theme.Color, r geom.Rect) bool {
	var stack []geom.Point
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.PushTransform:
			stack = append(stack, geom.Pt(o.M.E, o.M.F))
		case paint.Pop:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case paint.FillPath:
			if o.Color == col && len(stack) > 0 && o.Path != nil {
				b := o.Path.Bounds()
				at := stack[len(stack)-1]
				if r.Expand(1).Contains(geom.Pt(at.X+b.X+b.W/2, at.Y+b.Y+b.H/2)) {
					return true
				}
			}
		}
	}
	return false
}

// TestSwitchActionIcons: Element active-action-icon / inactive-action-icon
// draw the icon on the thumb in the track color of the state.
func TestSwitchActionIcons(t *testing.T) {
	th := theme.ElementPlus()
	tk := th.Components.Switch
	on := true
	var w *SwitchW
	d := p2dDriver(th, func(c *Context) {
		w = Switch(c, &on).ActionIcons(icon.Check, icon.Close).A11yLabel("s")
	})
	r := b1bRect(d, w.n.id)
	h := tk.TrackHeight
	right := geom.Rect{X: r.Right() - h, Y: r.Y, W: h, H: r.H}
	if !b1bIconAt(d, tk.OnBg, right) {
		t.Fatalf("no on-colored icon on the thumb at the end of %v", r)
	}
	d.Click(r.Center())
	if on {
		t.Fatal("click did not toggle")
	}
	d.Settle()
	left := geom.Rect{X: r.X, Y: r.Y, W: h, H: r.H}
	if !b1bIconAt(d, tk.OffBg, left) {
		t.Fatal("no off-colored icon on the thumb at the start")
	}
	if b1bIconAt(d, tk.OnBg, right) {
		t.Fatal("on icon still drawn when off")
	}
}

// b1bTexts returns the text ops of color col.
func b1bTexts(d *Driver, col theme.Color) []paint.Text {
	var out []paint.Text
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.Text); ok && o.Color == col {
			out = append(out, o)
		}
	}
	return out
}

// TestFormRequiredMarkOptional: antd requiredMark="optional" appends the
// locale's "(optional)" to optional labels, after the tooltip and before
// the colon, and drops the asterisk; the tooltip icon sits between the
// text and the mark also when the label width follows the content.
func TestFormRequiredMarkOptional(t *testing.T) {
	for _, viaDefaults := range []bool{false, true} {
		for _, lw := range []float32{200, 0} {
			th := theme.AntDesign()
			tk := th.Components.Form
			var s1, s2 string
			var f *FormW
			var nick *FormItemW
			d := p2dDriver(th, func(c *Context) {
				ConfigProvider(c).Locale(EnUS).Body(func() {
					build := func() {
						f = Form(c).LabelWidth(lw)
						if !viaDefaults {
							f.RequiredMarkOptional()
						}
						f.Body(func() {
							f.Item("Name").Required().Help("help").Body(func() { Input(c, &s1) })
							nick = f.Item("Nick").Tooltip("tip")
							nick.Body(func() { Input(c, &s2) })
						})
					}
					if viaDefaults {
						ConfigProvider(c).Defaults("Form", Props{"requiredMark": "optional"}).Body(build)
					} else {
						build()
					}
				})
			})
			name := func() string {
				if viaDefaults {
					return "defaults"
				}
				return "method"
			}()
			if !f.optional || !f.noStar {
				t.Fatalf("%s lw=%v: optional %v noStar %v", name, lw, f.optional, f.noStar)
			}
			if n := len(b1bTexts(d, tk.LabelRequiredMarkColor)); n != 0 {
				t.Fatalf("%s lw=%v: asterisk still drawn", name, lw)
			}
			// The label row of Nick: [Nick][tip][(optional)][:].
			lr := b1bRect(d, nick.labelID)
			var label, opt, colon float32 = -1, -1, -1
			n := 0
			for _, o := range b1bTexts(d, tk.LabelColor) {
				if o.Origin.Y < lr.Y-2 || o.Origin.Y > lr.Bottom() {
					continue
				}
				if n == 0 {
					label = o.Origin.X
				} else {
					colon = o.Origin.X
				}
				n++
			}
			for _, o := range b1bTexts(d, tk.LabelOptionalColor) {
				if o.Origin.Y >= lr.Y-2 && o.Origin.Y <= lr.Bottom() {
					opt = o.Origin.X
				}
			}
			tip := b1bRect(d, nick.tipID)
			if !(label >= 0 && label < tip.X && tip.Right() <= opt && opt < colon) {
				t.Fatalf("%s lw=%v: order text %v tip %v optional %v colon %v", name, lw, label, tip, opt, colon)
			}
		}
	}
}

// TestTextAreaDefaultRows: antd (a native textarea) and Element default
// to 2 rows; shadcn's frame is at least min-h-16, also with AutoSize.
func TestTextAreaDefaultRows(t *testing.T) {
	for _, c := range []struct {
		th   *theme.Theme
		want float32
	}{
		{theme.AntDesign(), 2*22 + 2*4}, // two 22px lines, paddingBlock 4
		{theme.ElementPlus(), 2*22 + 2*4},
		{theme.Shadcn(), 64}, // min-h-16
	} {
		v := ""
		var w, wa *TextAreaW
		d := p2dDriver(c.th, func(cx *Context) {
			w = TextArea(cx, &v).Width(240)
			wa = TextArea(cx, &v).AutoSize(1, 4).Width(240)
		})
		if w.rows != 2 {
			t.Fatalf("default rows %d", w.rows)
		}
		if h := b1bRect(d, w.box.id).H; h != c.want {
			t.Fatalf("%v: textarea height %v, want %v", c.th.Flavor, h, c.want)
		}
		if h := b1bRect(d, wa.box.id).H; c.th.Flavor == theme.StyleShadcn && h != 64 {
			t.Fatalf("shadcn AutoSize height %v, want 64", h)
		}
	}
}

// TestVariantDefaultsFallback: antd useVariant fallbacks. OTP and Search
// fall back to the "Input" key ('otp' / 'inputSearch' → 'input');
// TextArea reads only its own key.
func TestVariantDefaultsFallback(t *testing.T) {
	a, o, i1, i2 := "", "", "", ""
	var ta *TextAreaW
	var otp *OTPW
	var s1, s2 *SearchW
	p2dDriver(theme.AntDesign(), func(c *Context) {
		ConfigProvider(c).Defaults("Input", Props{"variant": "filled"}).Body(func() {
			ta = TextArea(c, &a)
			otp = OTP(c, &o, 4)
			s1 = Search(c, &i1)
		})
		ConfigProvider(c).Defaults("Search", Props{"variant": "underlined"}).Body(func() {
			s2 = Search(c, &i2)
		})
	})
	if ta.variant != "" {
		t.Fatalf("TextArea took the Input variant %q", ta.variant)
	}
	if otp.variant != "filled" {
		t.Fatalf("OTP variant %q, want the Input fallback", otp.variant)
	}
	if s1.variant != "" || s2.variant != "underlined" {
		t.Fatalf("Search variants %q / %q", s1.variant, s2.variant)
	}
}

// TestSliderVerticalReverseKeys: rc-slider "ttb" (vertical reverse):
// Up and Right decrease, Down and Left increase.
func TestSliderVerticalReverseKeys(t *testing.T) {
	v := 50.0
	var w *SliderW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		w = Slider(c, &v, 0, 100).Vertical(200).Reverse().A11yLabel("s")
	})
	d.Click(b1bRect(d, w.thumbs[0].id).Center())
	v = 50
	d.Frame()
	for _, k := range []struct {
		key  input.Key
		want float64
	}{{input.KeyUp, 49}, {input.KeyDown, 50}, {input.KeyRight, 49}, {input.KeyLeft, 50}} {
		d.Key(k.key, 0)
		if v != k.want {
			t.Fatalf("%v → %v, want %v", k.key, v, k.want)
		}
	}
}

// TestUploadReplaceKeepsOldSlice: MaxCount(1) replaces into a new array,
// so a slice the app kept still holds the old file.
func TestUploadReplaceKeepsOldSlice(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "new.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := make([]UploadFile, 1, 4)
	files[0] = UploadFile{Name: "old.txt", Status: "done"}
	kept := files
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		Upload(c, &files).MaxCount(1).Picker(func(string, bool, bool) ([]string, error) { return []string{p}, nil })
	})
	if err := d.ClickLabel(ZhCN.Upload.Button); err != nil {
		t.Fatal(err)
	}
	p2dWait(t, d, "replacement", func() bool { return len(files) == 1 && files[0].Name == "new.txt" })
	if kept[0].Name != "old.txt" {
		t.Fatalf("old slice overwritten: %+v", kept[0])
	}
}

// TestRadioDarkDotAndBlockCentre: antd radioColor colorWhite (a white dot
// in dark mode too); a block group centres plain radios in their share.
func TestRadioDarkDotAndBlockCentre(t *testing.T) {
	th := theme.AntDesign(theme.Dark())
	white := theme.Hex("#ffffff")
	v := "a"
	var g *RadioGroupW
	d := p2dDriver(th, func(c *Context) {
		Column(c, func() { g = RadioGroup(c, &v, Opts("a", "b")).Block() }, layout.W(400))
	})
	dot := false
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.FillPath); ok && o.Color == white && o.Path != nil && math.Abs(float64(o.Path.Bounds().W-6)) < 0.2 {
			dot = true
		}
	}
	if !dot {
		t.Fatal("dark checked radio: no white 6px dot")
	}
	k := b1bRect(d, g.n.kids[0].id)
	if k.W < 150 {
		t.Fatalf("block radio %v not stretched", k)
	}
	// The circle is centred: its 6px dot sits near the middle, not at the start.
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.FillPath); ok && o.Color == white && o.Path != nil {
			if cx := o.Path.Bounds().X; cx < k.X+k.W/4 {
				t.Fatalf("block radio dot at %v, start of %v", cx, k)
			}
		}
	}
}
