package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Under shadcn, widgets of one family look alike: the ones below used to
// keep Ant's look where a sibling already had the shadcn one.

// TestShadcnPickerFieldsMatchDatePicker: TimePicker and DateTimePicker
// fields put a 16px icon px-3 from the end, as DatePicker's shadcn field
// does (they kept Ant's 14px icon 11px in).
func TestShadcnPickerFieldsMatchDatePicker(t *testing.T) {
	th := theme.Shadcn()
	var dp *DatePickerW
	var tp *TimePickerW
	var dtp *DateTimePickerW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			var a, b, e time.Time
			dp = DatePicker(c, &a).Width(240).A11yLabel("d")
			tp = TimePicker(c, &b).Width(240).A11yLabel("t")
			dtp = DateTimePicker(c, &e).Width(240)
		}, layout.Pad(20), layout.Gap(12))
	}, DriverOptions{Width: 400, Height: 300, Theme: th})
	d.Settle()
	for _, f := range []struct {
		name  string
		field geom.Rect
		icon  geom.Rect
		pad   float32
	}{
		{"DatePicker", dp.n.abs, dp.st.clearR, 0},
		{"TimePicker", tp.f.n.abs, tp.f.st.clearR, tp.f.padding().Left},
		{"DateTimePicker", dtp.f.n.abs, dtp.f.st.clearR, dtp.f.padding().Left},
	} {
		if f.icon.W != 16 || f.icon.H != 16 {
			t.Errorf("%s: icon %vx%v, want 16x16", f.name, f.icon.W, f.icon.H)
		}
		if got, want := f.field.Right()-f.icon.Right(), th.ControlPaddingH; got != want {
			t.Errorf("%s: icon %v from the end, want %v", f.name, got, want)
		}
		if f.pad != 0 && f.pad != th.ControlPaddingH {
			t.Errorf("%s: start padding %v, want %v", f.name, f.pad, th.ControlPaddingH)
		}
	}
}

// fillsOf returns the FillRect ops painted in col.
func fillsOf(d *Driver, col theme.Color) []paint.FillRect {
	var out []paint.FillRect
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Color == col {
			out = append(out, f)
		}
	}
	return out
}

// TestShadcnVirtualTreeSelectionLikeTree: VirtualTree's selected title is
// highlighted with the Tree token's radius (shadcn rounded-md), as Tree's
// is (it used the small radius in every design system).
func TestShadcnVirtualTreeSelectionLikeTree(t *testing.T) {
	for _, th := range []*theme.Theme{theme.Shadcn(), theme.AntDesign()} {
		tk := th.Components.Tree
		nodes := []TreeNode{{Key: "a", Title: "Alpha"}, {Key: "b", Title: "Beta"}}
		d := NewDriver(func(c *Context) {
			Column(c, func() { cur := "a"; VirtualTree(c, nodes).Height(200).Current(&cur).Show() }, layout.Pad(10))
		}, DriverOptions{Width: 300, Height: 260, Theme: th})
		d.Settle()
		fills := fillsOf(d, tk.NodeSelectedBg)
		if len(fills) == 0 {
			t.Fatalf("%v: no selected fill", th.Flavor)
		}
		if got := fills[0].Radii.TL; got != tk.NodeBorderRadius {
			t.Errorf("%v: selected radius %v, want the Tree token's %v", th.Flavor, got, tk.NodeBorderRadius)
		}
	}
}

// TestShadcnSkeletonPulses: shadcn's Skeleton pulses its opacity
// (animate-pulse) instead of Ant's sweeping gradient band.
func TestShadcnSkeletonPulses(t *testing.T) {
	gradients := func(d *Driver) int {
		n := 0
		for _, op := range d.r.List().Ops {
			if _, ok := op.(paint.Gradient); ok {
				n++
			}
		}
		return n
	}
	page := func(c *Context) { Column(c, func() { Skeleton(c).Active().Show() }, layout.Pad(10)) }
	ant := NewDriver(page, DriverOptions{Width: 300, Height: 200, Theme: theme.AntDesign(), Animations: true})
	ant.Frame()
	ant.Advance(700 * time.Millisecond)
	ant.Frame()
	if gradients(ant) == 0 {
		t.Fatal("Ant skeleton: no shimmer band")
	}
	th := theme.Shadcn()
	d := NewDriver(page, DriverOptions{Width: 300, Height: 200, Theme: th, Animations: true})
	base := th.Components.Skeleton.GradientFromColor
	alpha := func() uint8 {
		var a uint8
		for _, op := range d.r.List().Ops {
			if f, ok := op.(paint.FillRect); ok && f.Color.R == base.R && f.Color.G == base.G && f.Color.B == base.B {
				a = f.Color.A
			}
		}
		return a
	}
	d.Frame()
	a0 := alpha()
	if gradients(d) != 0 {
		t.Error("shadcn skeleton: painted a shimmer gradient")
	}
	d.Advance(1000 * time.Millisecond) // the middle of the 2s pulse: half opacity
	d.Frame()
	a1 := alpha()
	if a0 == 0 || a1 == 0 || float32(a1) > float32(a0)*0.6 {
		t.Errorf("shadcn skeleton alpha %d then %d, want a pulse down to about half", a0, a1)
	}
}

// TestShadcnSearchButtonOutline: Search's icon button under shadcn hovers
// like an outline button (border kept, accent background) instead of
// turning its border primary (near-black in shadcn).
func TestShadcnSearchButtonOutline(t *testing.T) {
	th := theme.Shadcn()
	btk := th.Components.Button
	var r geom.Rect
	d := NewDriver(func(c *Context) {
		Column(c, func() { s := ""; Search(c, &s) }, layout.Pad(20), layout.W(300))
	}, DriverOptions{Width: 400, Height: 120, Theme: th})
	d.Settle()
	r, ok := d.Find(d.r.c.Locale().Search)
	if !ok {
		t.Fatal("no search button")
	}
	d.Move(r.Center())
	d.Settle()
	var border, hoverBg bool
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.StrokeRect:
			if o.Rect == r {
				border = o.Color == btk.DefaultBorderColor
				if o.Color == th.Primary.Hover && th.Primary.Hover != btk.DefaultBorderColor {
					t.Error("hovered search button border turned primary")
				}
			}
		case paint.FillRect:
			if o.Rect == r && o.Color == btk.DefaultHoverBg {
				hoverBg = true
			}
		}
	}
	if !border || !hoverBg {
		t.Errorf("hovered search button: border kept %v, accent background %v", border, hoverBg)
	}
}

// TestShadcnTransferRowsInset: one-way and custom-rendered Transfer rows
// start at the Transfer token's inset (shadcn px-2), as the default rows
// do; they had Ant's 12px.
func TestShadcnTransferRowsInset(t *testing.T) {
	th := theme.Shadcn()
	items := []TransferItem{{Key: "a", Title: "Apple"}, {Key: "b", Title: "Banana"}}
	target := []string{"b"}
	d := NewDriver(func(c *Context) {
		Column(c, func() { Transfer(c, items, &target).OneWay() }, layout.Pad(10))
	}, DriverOptions{Width: 700, Height: 400, Theme: th})
	d.Settle()
	var pads []float32
	var walk func(n *node)
	walk = func(n *node) {
		if n.role == a11y.RoleOption && n.ln.Padding.Right == 8 && n.ln.Padding.Left > 0 {
			pads = append(pads, n.ln.Padding.Left)
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(d.r.c.root)
	if len(pads) == 0 {
		t.Fatal("no one-way rows")
	}
	for _, p := range pads {
		if p != th.Components.Transfer.ItemPaddingInline {
			t.Errorf("one-way row inset %v, want %v", p, th.Components.Transfer.ItemPaddingInline)
		}
	}
}

// TestShadcnCountdownLikeStatistic: Countdown's number uses Statistic's
// token (shadcn font-bold, Element 20px), so a countdown next to a
// statistic matches it; it was regular weight in every design system.
func TestShadcnCountdownLikeStatistic(t *testing.T) {
	for _, th := range []*theme.Theme{theme.Shadcn(), theme.AntDesign(), theme.ElementPlus()} {
		size := th.Components.Statistic.ContentFontSize
		d := NewDriver(func(c *Context) {
			Row(c, func() {
				Statistic(c, "Users", 1234, 0)
				Countdown(c, "Left", c.Now().Add(90*time.Second))
			}, layout.Pad(10), layout.Gap(40))
		}, DriverOptions{Width: 500, Height: 160, Theme: th})
		d.Settle()
		srcs := map[paint.GlyphSource]bool{}
		runs := 0
		for _, op := range d.r.List().Ops {
			if tx, ok := op.(paint.Text); ok && len(tx.Glyphs) > 0 && tx.Glyphs[0].Size == size {
				srcs[tx.Glyphs[0].Src] = true
				runs++
			}
		}
		if runs < 2 {
			t.Fatalf("%v: %d text runs at the %vpx value size, want the statistic's and the countdown's", th.Flavor, runs, size)
		}
		if len(srcs) != 1 {
			t.Errorf("%v: statistic and countdown values use %d different faces", th.Flavor, len(srcs))
		}
	}
}

// TestShadcnTypographyTipLikeTooltip: the tooltip of Typography's copy
// (and edit) button is styled by the Tooltip token like Tooltip (shadcn:
// text-xs, px-3 py-1.5); it hard-coded Ant's look.
func TestShadcnTypographyTipLikeTooltip(t *testing.T) {
	th := theme.Shadcn()
	tk := th.Components.Tooltip
	d := NewDriver(func(c *Context) {
		Column(c, func() { Text(c, "copy me").Copyable() }, layout.Pad(60))
	}, DriverOptions{Width: 400, Height: 200, Theme: th})
	rec, done := d.recordIcons()
	d.Settle()
	done()
	var r geom.Rect
	for _, n := range rec.notes {
		if n.name == "copy" {
			r = n.r
		}
	}
	if r.Empty() {
		t.Fatal("no copy icon")
	}
	d.Move(r.Center())
	d.Settle()
	var bg *paint.FillRect
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Color == tk.Bg && f.Rect.Bottom() <= r.Y {
			bg = &f
		}
	}
	if bg == nil {
		t.Fatal("no tooltip above the copy button")
	}
	var size float32
	for _, op := range d.r.List().Ops {
		if tx, ok := op.(paint.Text); ok && tx.Color == tk.Color && len(tx.Glyphs) > 0 && bg.Rect.Contains(tx.Origin) {
			size = tx.Glyphs[0].Size
		}
	}
	if size != tk.ContentFontSize {
		t.Errorf("tooltip text %vpx, want the Tooltip token's %vpx", size, tk.ContentFontSize)
	}
}
