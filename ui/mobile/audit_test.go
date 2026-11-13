package mobile

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

// A horizontal label longer than the label column wraps inside it instead
// of painting over the field; the full text stays the accessible name.
func TestFormLabelStaysInColumn(t *testing.T) {
	for _, style := range []Style{AntMobile, Vant} {
		val := ""
		d := phone(func(c *ui.Context) {
			ui.ConfigProvider(c).Locale(ui.EnUS).Body(func() {
				Provider(c, Options{Style: style}, func() {
					Form(c).Body(func() {
						FormItem(c, "Phone number").Body(func() { Field(c, &val) })
					})
				})
			})
		}, false)
		d.EnableAccessibility()
		img := d.Settle()
		th := NewTheme(nil, style)
		g, ok := d.A11yFind(a11y.RoleGroup, "Phone number")
		if !ok {
			t.Fatalf("style %d: no group named by the full label", style)
		}
		if _, ok := d.A11yFind(a11y.RoleTextbox, "Phone number"); !ok {
			t.Fatalf("style %d: field not named by the full label", style)
		}
		lf := th.Font(th.FontBody, text.Regular)
		if g.Bounds.H < 2*lf.LineHeight {
			t.Errorf("style %d: row %.0f high, want the label on two lines", style, g.Bounds.H)
		}
		// Nothing of the label is painted in the gap between the label
		// column and the field.
		x0 := th.PadH + th.FormLabelWidth + 1
		for y := g.Bounds.Y + 2; y < g.Bounds.Y+g.Bounds.H-2; y++ {
			for x := x0; x < x0+6; x++ {
				if p := px(img, geom.Pt(x, y)); !near(p, th.Box) {
					t.Fatalf("style %d: label ink %v at (%.0f, %.0f) past the label column", style, p, x, y)
				}
			}
		}
	}
}

func TestWrapLabel(t *testing.T) {
	st := NewTheme(nil, AntMobile).Font(17, text.Regular)
	lines := wrapLabel("Phone number", st, 90, false)
	if len(lines) != 2 || lines[0] != "Phone" || lines[1] != "number" {
		t.Fatalf("got %q", lines)
	}
	long := wrapLabel("Primary contact phone number at work", st, 90, false)
	if len(long) != formLabelLines || long[0] == "" {
		t.Fatalf("got %q", long)
	}
}

// The inline Field shows the cell color in every style (no darker block
// in dark mode).
func TestFieldBackgroundMatchesCell(t *testing.T) {
	for _, style := range []Style{AntMobile, Vant, Shadcn} {
		for _, dark := range []bool{false, true} {
			base := theme.AntDesign()
			if dark {
				base = theme.AntDesign(theme.Dark())
			}
			val := ""
			d := ui.NewDriver(func(c *ui.Context) {
				Provider(c, Options{Style: style}, func() {
					Form(c).Body(func() {
						FormItem(c, "Name").Body(func() { Field(c, &val) })
					})
				})
			}, ui.DriverOptions{Width: 390, Height: 300, Theme: base})
			d.EnableAccessibility()
			img := d.Settle()
			th := NewTheme(base, style)
			f, ok := d.A11yFind(a11y.RoleTextbox, "Name")
			if !ok {
				t.Fatal("no field")
			}
			// The middle of the empty field, away from the caret.
			p := geom.Pt(f.Bounds.X+f.Bounds.W*0.75, f.Bounds.Y+f.Bounds.H/2)
			if got := px(img, p); !near(got, th.Box) {
				t.Errorf("style %d dark=%v: field %v, cell %v", style, dark, got, th.Box)
			}
		}
	}
}

// A label above its field starts at the start edge in every style (the
// right in RTL).
func TestVerticalFormLabelAtStart(t *testing.T) {
	for _, style := range []Style{AntMobile, Vant, Shadcn} {
		for _, rtl := range []bool{false, true} {
			val := ""
			d := phone(func(c *ui.Context) {
				dir := ui.LTR
				if rtl {
					dir = ui.RTL
				}
				ui.ConfigProvider(c).Direction(dir).Locale(ui.EnUS).Body(func() {
					Provider(c, Options{Style: style}, func() {
						Form(c).Vertical().Body(func() {
							FormItem(c, "City").Body(func() { Field(c, &val) })
						})
					})
				})
			}, false)
			d.EnableAccessibility()
			img := d.Settle()
			th := NewTheme(nil, style)
			g, ok := d.A11yFind(a11y.RoleGroup, "City")
			if !ok {
				t.Fatal("no item")
			}
			// The ink of the label line.
			lf := th.Font(th.FontBody, text.Regular)
			if th.shad() {
				lf = th.Font(th.FontMain, text.Medium)
			}
			lo, hi := float32(-1), float32(-1)
			for y := g.Bounds.Y + th.CellPadV; y < g.Bounds.Y+th.CellPadV+lf.LineHeight; y++ {
				for x := float32(0); x < 390; x++ {
					if !near(px(img, geom.Pt(x, y)), th.Box) {
						if lo < 0 || x < lo {
							lo = x
						}
						hi = max(hi, x)
					}
				}
			}
			if lo < 0 {
				t.Fatalf("style %d: no label ink", style)
			}
			if !rtl && abs32(lo-th.PadH) > 3 {
				t.Errorf("style %d: label ink starts at x=%.0f, want ≈%.0f", style, lo, th.PadH)
			}
			if rtl && abs32(hi+1-(390-th.PadH)) > 3 {
				t.Errorf("style %d RTL: label ink ends at x=%.0f, want ≈%.0f", style, hi+1, 390-th.PadH)
			}
		}
	}
}

// A wheel resting on trailing disabled rows settles on the nearest
// enabled one instead of oscillating between the disabled rows.
func TestPickerSkipsTrailingDisabled(t *testing.T) {
	opts := []ui.Option{{Label: "a", Value: "a"}, {Label: "b", Value: "b"}, {Label: "c", Value: "c", Disabled: true}, {Label: "d", Value: "d", Disabled: true}}
	if k := nearestEnabled(opts, 3); k != 1 {
		t.Fatalf("nearestEnabled(3) = %d, want 1", k)
	}
	if k := nearestEnabled(opts, 2); k != 1 {
		t.Fatalf("nearestEnabled(2) = %d, want 1", k)
	}
	if k := nearestEnabled([]ui.Option{{Disabled: true}, {Disabled: true}}, 1); k != -1 {
		t.Fatalf("all disabled: %d", k)
	}
	val := []string{"d"}
	d := phone(func(c *ui.Context) {
		PickerView(c, [][]ui.Option{opts}, &val).Show()
	}, true)
	for i := 0; i < 5; i++ {
		d.Frame()
	}
	run(d, time.Second)
	if val[0] != "b" {
		t.Fatalf("settled on %v, want b", val)
	}
	// Dragging to the end comes back to b too.
	th := NewTheme(nil, AntMobile)
	ih := th.PickerItemHeight
	cy := ih * float32(th.PickerVisible) / 2
	f := newFinger(t, d)
	f.down(geom.Pt(200, cy))
	f.move(geom.Pt(200, cy-3*ih), 8, 40*time.Millisecond)
	f.hold(200 * time.Millisecond)
	f.up()
	run(d, 2*time.Second)
	if val[0] != "b" {
		t.Fatalf("after drag: %v, want b", val)
	}
}

// The date wheels' options are built once per (range, locale, columns),
// not every frame.
func TestDateColumnsCached(t *testing.T) {
	when := time.Date(2024, 2, 10, 9, 30, 0, 0, time.UTC)
	var b binding
	d := phone(func(c *ui.Context) {
		b = dateBinding(c, &when, PrecisionMinute, time.Time{}, time.Time{})
	}, false)
	d.Frame()
	first := b.cols([]int{0, 1})
	d.Frame()
	again := b.cols([]int{0, 1})
	if &first[0][0] != &again[0][0] || &first[4][0] != &again[4][0] {
		t.Fatal("options rebuilt between frames")
	}
	sel := []int{0, 1}
	if n := testing.AllocsPerRun(20, func() { b.cols(sel) }); n != 0 {
		t.Fatalf("cols allocates %.0f times", n)
	}
	// 2014 + 10 = 2024, a leap year: February has 29 days; March 31.
	if got := len(b.cols([]int{10, 1})[2]); got != 29 {
		t.Fatalf("Feb 2024: %d days", got)
	}
	if got := len(b.cols([]int{9, 1})[2]); got != 28 {
		t.Fatalf("Feb 2023: %d days", got)
	}
	if got := len(b.cols([]int{0, 2})[2]); got != 31 {
		t.Fatalf("March: %d days", got)
	}
	if got := b.cols([]int{0, 0})[4][59].Label; got != fmt.Sprintf(ui.ZhCN.Mobile.MinuteFmt, 59) {
		t.Fatalf("minute label %q", got)
	}
}

// Under a high-contrast ui theme the mobile neutrals meet WCAG AA: text
// 4.5:1 and arrows / borders 3:1 on the surfaces they are drawn on.
func TestMobileThemeHighContrast(t *testing.T) {
	bases := []*theme.Theme{theme.AntDesign(), theme.AntDesign(theme.Dark()), theme.Shadcn(), theme.Shadcn(theme.Dark())}
	for _, base := range bases {
		for _, style := range []Style{AntMobile, Vant, Shadcn} {
			th := NewTheme(theme.HighContrast(base), style)
			name := fmt.Sprintf("style %d dark=%v", style, base.Dark)
			check := func(what string, fg theme.Color, min float64, bgs ...theme.Color) {
				for _, bg := range bgs {
					if r := theme.ContrastRatio(fg, bg); r+1e-9 < min {
						t.Errorf("%s: %s %s on %s = %.2f:1, want %.1f", name, what, theme.HexString(fg), theme.HexString(bg), r, min)
					}
				}
			}
			surf := []theme.Color{th.Box, th.PageBg, th.Fill}
			check("Text", th.Text, 4.5, surf...)
			check("TextSecondary", th.TextSecondary, 4.5, surf...)
			check("TextWeak", th.TextWeak, 4.5, surf...)
			check("TextLight (arrow)", th.TextLight, 3, th.Box, th.PageBg)
			check("Border", th.Border, 3, th.Box, th.PageBg)
			check("SwitchOff", th.SwitchOff, 3, th.Box)
			check("NoticeText", th.NoticeText, 4.5, th.NoticeBg)
			// The style is kept: the default theme is untouched.
			if def := NewTheme(base, style); def.Text == th.Text && def.TextWeak == th.TextWeak && def.Border == th.Border && theme.ContrastRatio(def.Border, def.Box) < 3 {
				t.Errorf("%s: high contrast changed nothing", name)
			}
		}
	}
}

// IndicatorWhite paints the current dot white (visible on a colored
// slide); the default keeps the style's primary dot.
func TestSwiperIndicatorColor(t *testing.T) {
	blue := theme.Hex("#1677ff")
	for _, white := range []bool{false, true} {
		cur := 0
		d := phone(func(c *ui.Context) {
			sw := Swiper(c, &cur)
			if white {
				sw.IndicatorWhite()
			}
			sw.Slides(3, func(i int) { ui.Div(c, layout.Grow(1)).Bg(blue) })
		}, false)
		img := d.Settle()
		// Ant Mobile dots: 3px, the current one 13px wide, 6px above the
		// bottom of the 150px swiper, centred.
		total := float32(2*3 + 13 + 2*3)
		p := geom.Pt((390-total)/2+6, 150-7.5)
		got := px(img, p)
		if white && !near(got, theme.Hex("#ffffff")) {
			t.Errorf("white indicator: %v", got)
		}
		if !white && near(got, theme.Hex("#ffffff")) {
			t.Errorf("default indicator turned white")
		}
	}
}

// ScrollViewW.Rows builds only the rows near the view, keeps the full
// scroll range and builds the right rows after scrolling.
func TestScrollViewRows(t *testing.T) {
	const n, rowH = 10000, 50
	built := 0
	seen := map[int]bool{}
	has := func(i int) bool { return seen[i] }
	var sv *ScrollViewW
	d := phone(func(c *ui.Context) {
		built = 0
		clear(seen)
		sv = ScrollView(c).Layout(layout.H(600))
		sv.Body(func() {
			ui.Text(c, "header")
			List(c, "").Body(func() {
				sv.Rows(n, rowH, func(i int) {
					built++
					seen[i] = true
					Cell(c, fmt.Sprint("row ", i))
				})
			})
		})
	}, false)
	d.Settle()
	if built == 0 || built > 600/rowH+2*rowsOverscan+2 {
		t.Fatalf("built %d rows", built)
	}
	if !has(0) {
		t.Fatal("first row missing")
	}
	d.Frame() // the content height is known from the second frame on
	sv.ScrollTo(5000 * rowH)
	d.Settle()
	d.Settle()
	if !has(5000) {
		t.Fatalf("row 5000 not built after scrolling (offset %v)", sv.ScrollOffset())
	}
	if has(0) {
		t.Fatal("row 0 still built")
	}
	if built > 600/rowH+2*rowsOverscan+2 {
		t.Fatalf("built %d rows after scrolling", built)
	}
	// The end of the list is reachable: the content keeps its full height.
	sv.ScrollTo(1e9)
	d.Settle()
	d.Settle()
	if !has(n - 1) {
		t.Fatal("last row not reachable")
	}
}
