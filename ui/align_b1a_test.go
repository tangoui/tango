package ui

// Spec alignment batch 1, group A (basic and layout components): behaviour
// tests for the gaps closed against antd 6.7 / Element Plus 2.14.8 /
// shadcn v4 (docs/specs/align/1-basic.md).

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func center(r geom.Rect) geom.Point { return geom.Pt(r.X+r.W/2, r.Y+r.H/2) }

// ---------------------------------------------------------------------------
// Button

// TestButtonIconPlacementEnd: iconPlacement "end" puts the icon after the
// label (to its right in LTR, to its left in RTL); "start" before it.
func TestButtonIconPlacementEnd(t *testing.T) {
	for _, tc := range []struct {
		name      string
		rtl       bool
		placement string
		iconRight bool // icon x > label x
	}{
		{"ltr end", false, "end", true},
		{"ltr start", false, "start", false},
		{"rtl end", true, "end", false},
		{"rtl start", true, "start", true},
	} {
		var ix, lx float32
		var placed bool
		d := NewDriver(func(c *Context) {
			body := func() {
				b := Button(c, "Next").Icon(icon.Right).IconPlacement(tc.placement)
				if r, ok := c.RectOf(b.ID()); ok {
					ix, lx = b.place(r, 30, true)
					placed = true
				}
			}
			if tc.rtl {
				ConfigProvider(c).Direction(RTL).Body(body)
			} else {
				body()
			}
		}, DriverOptions{})
		d.Frame()
		d.Frame()
		if !placed {
			t.Fatalf("%s: button not laid out", tc.name)
		}
		if got := ix > lx; got != tc.iconRight {
			t.Errorf("%s: icon x %.1f, label x %.1f; want icon right of label = %v", tc.name, ix, lx, tc.iconRight)
		}
	}
}

// TestButtonIconPlacementDefaults: Defaults("Button", {"iconPlacement"})
// applies; an explicit call wins.
func TestButtonIconPlacementDefaults(t *testing.T) {
	var a, b *ButtonW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Button", Props{"iconPlacement": "end"}).Body(func() {
			a = Button(c, "A").Icon(icon.Right)
			b = Button(c, "B").Icon(icon.Right).IconPlacement("start")
		})
	}, DriverOptions{})
	d.Frame()
	if !a.icRight || b.icRight {
		t.Errorf("icRight a=%v b=%v, want true false", a.icRight, b.icRight)
	}
}

// TestButtonAutoInsertSpaceThemeDefault: Ant spaces a two-Chinese-character
// label by default (ConfigProvider button.autoInsertSpace = true), Element
// and shadcn do not.
func TestButtonAutoInsertSpaceThemeDefault(t *testing.T) {
	for _, tc := range []struct {
		th   *theme.Theme
		want string
	}{
		{theme.AntDesign(), "确 定"},  // i18n:ok test label
		{theme.ElementPlus(), "确定"}, // i18n:ok test label
		{theme.Shadcn(), "确定"},      // i18n:ok test label
	} {
		var got, off string
		d := NewDriver(func(c *Context) {
			got = Button(c, "确定").shownLabel()                        // i18n:ok test label
			off = Button(c, "取消").AutoInsertSpace(false).shownLabel() // i18n:ok test label
		}, DriverOptions{Theme: tc.th})
		d.Frame()
		if got != tc.want || off != "取消" { // i18n:ok test label
			t.Errorf("%v: %q / %q, want %q / unspaced", tc.th.Flavor, got, off, tc.want)
		}
	}
}

// TestButtonLoadingDelay: with a delay the button stays clickable and
// shows no spinner until the delay has passed.
func TestButtonLoadingDelay(t *testing.T) {
	clicks := 0
	var b *ButtonW
	d := NewDriver(func(c *Context) {
		b = Button(c, "Save").Loading(true).LoadingDelay(500 * time.Millisecond)
		if b.Clicked() {
			clicks++
		}
	}, DriverOptions{})
	d.Frame()
	if b.isLoading() {
		t.Fatal("loading shown before the delay")
	}
	r, _ := d.Find("Save")
	d.Click(center(r))
	if clicks != 1 {
		t.Fatalf("clicks during the delay = %d, want 1", clicks)
	}
	d.Advance(600 * time.Millisecond)
	d.Frame()
	if !b.isLoading() {
		t.Fatal("loading not shown after the delay")
	}
	d.Click(center(r))
	if clicks != 1 {
		t.Errorf("a loading button was clicked")
	}
}

// TestButtonLoadingIconDisabled: the custom loading icon replaces the
// spinner; a disabled button never reports clicks.
func TestButtonLoadingIconDisabled(t *testing.T) {
	clicked := false
	var b *ButtonW
	d := NewDriver(func(c *Context) {
		b = Button(c, "Go").LoadingIcon(icon.Sync).Disabled(true)
		if b.Clicked() {
			clicked = true
		}
	}, DriverOptions{})
	d.Frame()
	r, _ := d.Find("Go")
	d.Click(center(r))
	if clicked {
		t.Error("disabled button clicked")
	}
	if b.loadIc != icon.Sync {
		t.Error("loading icon not kept")
	}
}

// ---------------------------------------------------------------------------
// Typography / Link

// TestLinkUnderlineMode: the theme default (Ant never, Element on hover,
// shadcn always) and the explicit modes.
func TestLinkUnderlineMode(t *testing.T) {
	for _, tc := range []struct {
		th          *theme.Theme
		rest, hover bool
	}{
		{theme.AntDesign(), false, false},
		{theme.ElementPlus(), false, true},
		{theme.Shadcn(), true, true},
	} {
		var lw, always, never, hov *LinkW
		d := NewDriver(func(c *Context) {
			lw = Link(c, "a")
			always = Link(c, "b").UnderlineMode("always")
			never = Link(c, "c").UnderlineMode("never")
			hov = Link(c, "d").UnderlineMode("hover")
		}, DriverOptions{Theme: tc.th})
		d.Frame()
		tk := &tc.th.Components.Typography
		if lw.underlined(tk, false) != tc.rest || lw.underlined(tk, true) != tc.hover {
			t.Errorf("%v: default underline rest/hover = %v/%v, want %v/%v", tc.th.Flavor,
				lw.underlined(tk, false), lw.underlined(tk, true), tc.rest, tc.hover)
		}
		if !always.underlined(tk, false) || never.underlined(tk, true) || hov.underlined(tk, false) || !hov.underlined(tk, true) {
			t.Errorf("%v: explicit modes wrong", tc.th.Flavor)
		}
	}
}

// TestLinkIconDisabled: an icon widens the link; a disabled link is not
// clicked and is reported disabled.
func TestLinkIconDisabled(t *testing.T) {
	clicked := false
	var plain, withIcon *LinkW
	d := NewDriver(func(c *Context) {
		Row(c, func() {
			plain = Link(c, "Docs")
			withIcon = Link(c, "Docs2").Icon(icon.Link)
			l := Link(c, "Off").Icon(icon.Link)
			l.Disabled()
			if l.Clicked() {
				clicked = true
			}
		})
	}, DriverOptions{})
	d.EnableAccessibility()
	d.Frame()
	if withIcon.measure(geom.Inf, geom.Inf).W <= plain.measure(geom.Inf, geom.Inf).W {
		t.Error("icon does not widen the link")
	}
	r, _ := d.Find("Off")
	d.Click(center(r))
	if clicked {
		t.Error("disabled link clicked")
	}
	if n, ok := d.A11yFind(a11y.RoleLink, "Off"); !ok || n.States&a11y.Disabled == 0 {
		t.Error("disabled link not reported disabled")
	}
}

// ---------------------------------------------------------------------------
// Divider

// TestDividerOrientationTitlePlacement: v6 orientation is the direction;
// the pre-v6 placement values still set the title placement; "left" and
// "right" are physical, "start" and "end" logical.
func TestDividerOrientationTitlePlacement(t *testing.T) {
	for _, tc := range []struct {
		rtl  bool
		set  func(d *DividerW)
		want string
	}{
		{false, func(d *DividerW) { d.TitlePlacement("left") }, "start"},
		{true, func(d *DividerW) { d.TitlePlacement("left") }, "end"},
		{true, func(d *DividerW) { d.TitlePlacement("start") }, "start"},
		{true, func(d *DividerW) { d.Orientation("right") }, "start"},
		{false, func(d *DividerW) { d.TitlePlacement("bogus") }, "center"},
	} {
		var got string
		NewDriver(func(c *Context) {
			body := func() {
				dv := Divider(c, "Title")
				tc.set(dv)
				got = dv.placement()
			}
			if tc.rtl {
				ConfigProvider(c).Direction(RTL).Body(body)
			} else {
				body()
			}
		}, DriverOptions{}).Frame()
		if got != tc.want {
			t.Errorf("rtl=%v: placement %q, want %q", tc.rtl, got, tc.want)
		}
	}
}

// TestDividerVertical: Vertical / orientation="vertical" makes an inline
// vertical separator 0.9em high (Ant) with the vertical a11y state.
func TestDividerVertical(t *testing.T) {
	var a, b *DividerW
	d := NewDriver(func(c *Context) {
		Row(c, func() {
			Text(c, "x")
			a = Divider(c).Vertical()
			b = Divider(c).Orientation("vertical")
			Text(c, "y")
		})
	}, DriverOptions{Theme: theme.AntDesign()})
	d.EnableAccessibility()
	d.Frame()
	for _, dv := range []*DividerW{a, b} {
		r := dv.Rect()
		if r.W != 1 || r.H < 12 || r.H > 13 {
			t.Errorf("vertical divider %v, want 1×12.6", r)
		}
		if dv.n.aState&a11y.Vertical == 0 || dv.n.aState&a11y.Horizontal != 0 {
			t.Error("vertical divider not marked vertical")
		}
	}
}

// TestDividerSizeVariant: Size sets the block margin (small 8, middle 16);
// Variant accepts solid / dashed / dotted only; Defaults apply.
func TestDividerSizeVariant(t *testing.T) {
	var sm, md, dflt, dotted, bad, def *DividerW
	NewDriver(func(c *Context) {
		sm = Divider(c).Size(Small)
		md = Divider(c).Size(Middle)
		dflt = Divider(c)
		dotted = Divider(c).Variant("dotted")
		bad = Divider(c).Variant("double")
		ConfigProvider(c).Defaults("Divider", Props{"variant": "dashed", "size": Small, "titlePlacement": "end"}).Body(func() {
			def = Divider(c, "T")
		})
	}, DriverOptions{Theme: theme.AntDesign()}).Frame()
	if sm.n.ln.Margin.Top != 8 || md.n.ln.Margin.Top != 16 || dflt.n.ln.Margin.Top != 24 {
		t.Errorf("margins %v %v %v, want 8 16 24", sm.n.ln.Margin.Top, md.n.ln.Margin.Top, dflt.n.ln.Margin.Top)
	}
	if dotted.variant != "dotted" || bad.variant != "solid" {
		t.Errorf("variants %q %q", dotted.variant, bad.variant)
	}
	if def.variant != "dashed" || def.n.ln.Margin.Top != 8 || def.orient != "end" {
		t.Errorf("defaults: %q %v %q", def.variant, def.n.ln.Margin.Top, def.orient)
	}
}

// TestDividerTitleFont: the titled divider is fontSizeLG (24px line) in
// Ant, 14px (22px line) in Element; Plain is the body size.
func TestDividerTitleFont(t *testing.T) {
	for _, tc := range []struct {
		th         *theme.Theme
		titled, pl float32
	}{
		{theme.AntDesign(), 24, 22},
		{theme.ElementPlus(), 22, 22},
	} {
		var a, b *DividerW
		d := NewDriver(func(c *Context) {
			a = Divider(c, "Title")
			b = Divider(c, "Title").Plain()
		}, DriverOptions{Theme: tc.th})
		d.Frame()
		if a.Rect().H != tc.titled || b.Rect().H != tc.pl {
			t.Errorf("%v: heights %v / %v, want %v / %v", tc.th.Flavor, a.Rect().H, b.Rect().H, tc.titled, tc.pl)
		}
	}
}

// ---------------------------------------------------------------------------
// Space / Flex

// TestSpaceSeparator: three items get two separators, none before the
// first or after the last.
func TestSpaceSeparator(t *testing.T) {
	var box *Box
	NewDriver(func(c *Context) {
		box = Space(c).Separator(func() { VDivider(c) }).Body(func() {
			Text(c, "a")
			Text(c, "b")
			Text(c, "c")
		})
	}, DriverOptions{}).Frame()
	var kinds []string
	for _, k := range box.n.kids {
		if k.label == "" {
			kinds = append(kinds, "sep")
		} else {
			kinds = append(kinds, k.label)
		}
	}
	want := []string{"a", "sep", "b", "sep", "c"}
	if len(kinds) != len(want) {
		t.Fatalf("children %v, want %v", kinds, want)
	}
	for i := range want {
		if kinds[i] != want[i] {
			t.Fatalf("children %v, want %v", kinds, want)
		}
	}
}

// TestSpaceOrientation: orientation "vertical" (method and Defaults) stacks
// the items aligned to the start; Flex has the same method.
func TestSpaceOrientation(t *testing.T) {
	var a, b, f *Box
	NewDriver(func(c *Context) {
		a = Space(c).Orientation("vertical").Body(func() { Text(c, "1"); Text(c, "2") })
		ConfigProvider(c).Defaults("Space", Props{"orientation": "vertical"}).Body(func() {
			b = Space(c).Body(func() { Text(c, "3"); Text(c, "4") })
		})
		f = Flex(c).Orientation("vertical").Body(func() { Text(c, "5") })
	}, DriverOptions{}).Frame()
	for i, bx := range []*Box{a, b, f} {
		if bx.n.ln.Dir != layout.Column {
			t.Errorf("box %d not vertical", i)
		}
	}
	if a.n.ln.Align != layout.AlignStart {
		t.Error("vertical space does not align to the start")
	}
}

// ---------------------------------------------------------------------------
// Grid / Layout

// TestGridVGutterAt: the vertical gutter follows the breakpoint.
func TestGridVGutterAt(t *testing.T) {
	var box *Box
	d := NewDriver(func(c *Context) {
		box = GridRow(c).Gutter(8, 4).VGutterAt(MD, 24).Body(func() {
			Col(c, 24).Body(func() { Text(c, "a") })
			Col(c, 24).Body(func() { Text(c, "b") })
		})
	}, DriverOptions{Width: 1000})
	d.Frame()
	if box.n.ln.Gap != 24 {
		t.Errorf("wide gap %v, want 24", box.n.ln.Gap)
	}
	d.Resize(500, 400)
	d.Frame()
	if box.n.ln.Gap != 4 {
		t.Errorf("narrow gap %v, want 4", box.n.ln.Gap)
	}
}

// TestLayoutFooterStart: the footer content starts at the inline start
// (Ant and Element do not centre it).
func TestLayoutFooterStart(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		var f *Box
		d := NewDriver(func(c *Context) {
			Layout(c, func() {
				Content(c, func() {})
				f = Footer(c, func() { Text(c, "foot") })
			})
		}, DriverOptions{Theme: th, Width: 800, Height: 300})
		d.Frame()
		r, _ := d.Find("foot")
		fr := f.Rect()
		pad := th.Components.Layout.FooterPaddingInline
		if r.X-fr.X > pad+1 {
			t.Errorf("%v: footer text at %v in %v, want at the start (pad %v)", th.Flavor, r.X, fr.X, pad)
		}
	}
}

// ---------------------------------------------------------------------------
// Segmented

// TestSegmentedItemRender: custom content per option; a click on it
// selects the option; the option label names the radio.
func TestSegmentedItemRender(t *testing.T) {
	v := "a"
	var sels []bool
	d := NewDriver(func(c *Context) {
		sels = sels[:0]
		Segmented(c, &v, []Option{{Label: "Alpha", Value: "a"}, {Label: "Beta", Value: "b"}}).
			ItemRender(func(o Option, sel bool) {
				sels = append(sels, sel)
				Icon(c, icon.Star)
				Text(c, "custom-"+o.Value)
			})
	}, DriverOptions{})
	d.EnableAccessibility()
	d.Frame()
	r, ok := d.FindText("custom-b")
	if !ok {
		t.Fatal("custom content not built")
	}
	d.Click(center(r))
	if v != "b" {
		t.Fatalf("value %q after clicking the custom content, want b", v)
	}
	if len(sels) != 2 || sels[0] || !sels[1] {
		t.Errorf("selected flags %v, want [false true]", sels)
	}
	if n, ok := d.A11yFind(a11y.RoleRadio, "Beta"); !ok || n.States&a11y.Checked == 0 {
		t.Error("radio Beta not found checked")
	}
}

// TestSegmentedItemRenderDisabled: a disabled control ignores clicks on
// custom content.
func TestSegmentedItemRenderDisabled(t *testing.T) {
	v := "a"
	d := NewDriver(func(c *Context) {
		Segmented(c, &v, Opts("a", "b")).Disabled(true).ItemRender(func(o Option, _ bool) { Text(c, "x-"+o.Value) })
	}, DriverOptions{})
	d.Frame()
	r, _ := d.FindText("x-b")
	d.Click(center(r))
	if v != "a" {
		t.Errorf("disabled segmented changed to %q", v)
	}
}

// TestSegmentedUnknownValue: a value outside the options selects nothing
// without panicking; the arrow keys then pick an option; Orientation
// "vertical" stacks the items.
func TestSegmentedUnknownValue(t *testing.T) {
	v := "zzz"
	var w *SegmentedW
	d := NewDriver(func(c *Context) {
		w = Segmented(c, &v, Opts("a", "b")).Orientation("vertical")
	}, DriverOptions{})
	d.EnableAccessibility()
	d.Frame()
	if w.selected() != -1 {
		t.Fatal("unknown value selected an option")
	}
	if !w.vertical {
		t.Error("orientation vertical ignored")
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyDown, 0)
	if v != "a" {
		t.Errorf("value %q after Down, want a", v)
	}
}

// ---------------------------------------------------------------------------
// Toggle / ToggleGroup

// TestToggleGroupSpacing: by default the toggles touch (shadcn
// spacing=0; outlined ones share borders); Spacing(n) separates them by
// n×4px with full borders.
func TestToggleGroupSpacing(t *testing.T) {
	sel := []string{}
	var joined, spaced *ToggleGroupW
	NewDriver(func(c *Context) {
		joined = ToggleGroup(c, &sel, Opts("a", "b", "c")).Outline()
		joined.Changed()
		spaced = ToggleGroup(c, &sel, Opts("d", "e")).Outline().Spacing(2)
		spaced.Changed()
	}, DriverOptions{Theme: theme.Shadcn()}).Frame()
	if joined.n.ln.Gap != 0 || spaced.n.ln.Gap != 8 {
		t.Errorf("gaps %v / %v, want 0 / 8", joined.n.ln.Gap, spaced.n.ln.Gap)
	}
	if joined.nodes[1].ln.Margin.Left != -1 || spaced.nodes[1].ln.Margin.Left != 0 {
		t.Errorf("border sharing: joined %v spaced %v", joined.nodes[1].ln.Margin.Left, spaced.nodes[1].ln.Margin.Left)
	}
}

// TestToggleSizePadding: shadcn toggle padding px-1.5 / px-2 / px-2.5 and
// px-3 inside a group; disabled toggles do not flip.
func TestToggleSizePadding(t *testing.T) {
	var sm, md, lg *ToggleW
	off := false
	on := false
	NewDriver(func(c *Context) {
		sm = Toggle(c, "B", &on).Size(Small)
		md = Toggle(c, "B", &on)
		lg = Toggle(c, "B", &on).Size(Large)
	}, DriverOptions{Theme: theme.Shadcn()}).Frame()
	if sm.padX() != 6 || md.padX() != 8 || lg.padX() != 10 {
		t.Errorf("padding %v %v %v, want 6 8 10", sm.padX(), md.padX(), lg.padX())
	}
	// A ToggleGroup item is px-3: 8px wider than the lone px-2 toggle.
	sel := []string{}
	var lone *ToggleW
	var grp *ToggleGroupW
	NewDriver(func(c *Context) {
		lone = Toggle(c, "Bold text", &on)
		grp = ToggleGroup(c, &sel, Opts("Bold text"))
		grp.Changed()
	}, DriverOptions{Theme: theme.Shadcn()}).Frame()
	if dw := grp.nodes[0].ln.Rect.W - lone.n.ln.Rect.W; dw < 7.5 || dw > 8.5 {
		t.Errorf("group item %v vs lone %v, want +8", grp.nodes[0].ln.Rect.W, lone.n.ln.Rect.W)
	}
	d := NewDriver(func(c *Context) { Toggle(c, "Bold", &off).Disabled(true).Changed() }, DriverOptions{})
	d.Frame()
	r, _ := d.Find("Bold")
	d.Click(center(r))
	if off {
		t.Error("disabled toggle flipped")
	}
}

// TestToggleGroupA11yDisabled: the group is one toolbar; a disabled option
// does not toggle.
func TestToggleGroupA11yDisabled(t *testing.T) {
	sel := []string{}
	d := NewDriver(func(c *Context) {
		ToggleGroup(c, &sel, []Option{{Label: "Bold", Value: "b"}, {Label: "Italic", Value: "i", Disabled: true}}).A11yLabel("Format").Changed()
	}, DriverOptions{})
	d.EnableAccessibility()
	d.Frame()
	if _, ok := d.A11yFind(a11y.RoleToolbar, "Format"); !ok {
		t.Error("toolbar not found")
	}
	r, _ := d.Find("Italic")
	d.Click(center(r))
	if len(sel) != 0 {
		t.Errorf("disabled option toggled: %v", sel)
	}
	r, _ = d.Find("Bold")
	d.Click(center(r))
	if len(sel) != 1 || sel[0] != "b" {
		t.Errorf("selection %v, want [b]", sel)
	}
}

// ---------------------------------------------------------------------------
// Rate

// TestRateSizes: Small / Large follow the theme (Ant 15 / 20 / 25, Element
// 14 / 18 / 18 in 24 / 32 / 40 rows); Size in px wins; Defaults "size".
func TestRateSizes(t *testing.T) {
	for _, tc := range []struct {
		th              *theme.Theme
		sm, md, lg, hLG float32
	}{
		{theme.AntDesign(), 15, 20, 25, 27},
		{theme.ElementPlus(), 14, 18, 18, 40},
	} {
		var sm, md, lg, px, def *RateW
		v := 3.0
		NewDriver(func(c *Context) {
			sm = Rate(c, &v).Small()
			md = Rate(c, &v)
			lg = Rate(c, &v).Large()
			px = Rate(c, &v).Large().Size(30)
			ConfigProvider(c).Defaults("Rate", Props{"size": Small}).Body(func() { def = Rate(c, &v) })
		}, DriverOptions{Theme: tc.th}).Frame()
		if sm.starSize() != tc.sm || md.starSize() != tc.md || lg.starSize() != tc.lg || px.starSize() != 30 || def.starSize() != tc.sm {
			t.Errorf("%v: sizes %v %v %v %v %v", tc.th.Flavor, sm.starSize(), md.starSize(), lg.starSize(), px.starSize(), def.starSize())
		}
		if lg.height() != tc.hLG {
			t.Errorf("%v: large height %v, want %v", tc.th.Flavor, lg.height(), tc.hLG)
		}
	}
}

// TestRateKeyboardOff: Keyboard(false) ignores the arrow keys; disabled
// rates ignore them too.
func TestRateKeyboardOff(t *testing.T) {
	a, b := 2.0, 2.0
	d := NewDriver(func(c *Context) {
		Rate(c, &a).Keyboard(false).A11yLabel("A")
		Rate(c, &b).Disabled(true).A11yLabel("B")
	}, DriverOptions{})
	d.Frame()
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyRight, 0)
	if a != 2 || b != 2 {
		t.Errorf("values %v %v, want unchanged", a, b)
	}
}

// TestRateColorsThresholds: Element colors switch at the thresholds; one
// color applies to every value; Color wins; VoidColor paints the voids.
func TestRateColorsThresholds(t *testing.T) {
	lo, mid, hi := theme.Hex("#99a9bf"), theme.Hex("#f7ba2a"), theme.Hex("#ff9900")
	v := 0.0
	var w, one, fixed *RateW
	NewDriver(func(c *Context) {
		w = Rate(c, &v).Colors(lo, mid, hi).VoidColor(lo)
		one = Rate(c, &v).Colors(mid)
		fixed = Rate(c, &v).Colors(lo, mid, hi).Color(hi)
	}, DriverOptions{}).Frame()
	for _, tc := range []struct {
		v    float64
		want theme.Color
	}{{1, lo}, {2, lo}, {3, mid}, {3.5, mid}, {4, hi}, {5, hi}} {
		if on, _ := w.colors(tc.v); on != tc.want {
			t.Errorf("value %v: color %v, want %v", tc.v, on, tc.want)
		}
	}
	if _, off := w.colors(1); off != lo {
		t.Error("void color ignored")
	}
	if on, _ := one.colors(5); on != mid {
		t.Error("single color ignored")
	}
	if on, _ := fixed.colors(1); on != hi {
		t.Error("Color does not win over Colors")
	}
}

// TestRateScoreFormatOutOfRange: ScoreFormat formats the score; a value
// above the count draws without panicking and the keys clamp it.
func TestRateScoreFormatOutOfRange(t *testing.T) {
	v := 9.0
	var w *RateW
	d := NewDriver(func(c *Context) {
		w = Rate(c, &v).ShowScore().ScoreFormat(func(x float64) string { return formatRateScore(x) + " pts" })
	}, DriverOptions{})
	d.EnableAccessibility()
	d.Frame()
	if got := w.label(3); got != "3 pts" {
		t.Errorf("score %q, want 3 pts", got)
	}
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyRight, 0)
	if v != 5 {
		t.Errorf("value %v after Right from 9, want clamped 5", v)
	}
	if n, ok := d.A11yFind(a11y.RoleSlider, ""); !ok || n.ValueMax != 5 {
		t.Errorf("a11y slider found=%v", ok)
	}
}

// TestLinkTypeDefault: Element type="default" is the regular text color
// that turns link-colored on hover (same as Secondary).
func TestLinkTypeDefault(t *testing.T) {
	th := theme.ElementPlus()
	var lw *LinkW
	NewDriver(func(c *Context) { lw = Link(c, "a").Type("default") }, DriverOptions{Theme: th}).Frame()
	tk := &th.Components.Typography
	base, hover, _ := lw.colors(th, tk)
	if base != th.TextSecondary || hover != tk.LinkHoverColor {
		t.Errorf("default link colors %v / %v", base, hover)
	}
}

// TestVDividerShadcnFullHeight: shadcn's vertical separator is h-full (the
// row's height); Ant's is 0.9em.
func TestVDividerShadcnFullHeight(t *testing.T) {
	for _, tc := range []struct {
		th   *theme.Theme
		want float32
	}{{theme.Shadcn(), 36}, {theme.AntDesign(), 12.6}} {
		var rect geom.Rect
		var row *Box
		d := NewDriver(func(c *Context) {
			row = Row(c, func() {
				Button(c, "A")
				VDivider(c)
				dv := Divider(c).Vertical()
				Button(c, "B")
				rect = dv.Rect()
			})
		}, DriverOptions{Theme: tc.th})
		d.Frame()
		d.Frame()
		if rect.H < tc.want-0.5 || rect.H > tc.want+0.5 {
			t.Errorf("%v: vertical divider height %v, want %v", tc.th.Flavor, rect.H, tc.want)
		}
		if h := row.n.kids[1].ln.Rect.H; h < tc.want-0.5 || h > tc.want+0.5 {
			t.Errorf("%v: VDivider height %v, want %v", tc.th.Flavor, h, tc.want)
		}
	}
}

// TestSegmentedTooltips: hovering an option shows its tooltip; clicking
// still selects it.
func TestSegmentedTooltips(t *testing.T) {
	v := "a"
	d := NewDriver(func(c *Context) {
		Segmented(c, &v, Opts("a", "b")).Tooltips("", "tip-b")
	}, DriverOptions{})
	d.Frame()
	r, _ := d.Find("b")
	d.Move(center(r))
	d.Frame()
	if _, ok := d.FindText("tip-b"); !ok {
		t.Error("tooltip not shown")
	}
	ra, _ := d.Find("a")
	d.Move(center(ra))
	d.Frame()
	if _, ok := d.FindText("tip-b"); ok {
		t.Error("tooltip stays on another option")
	}
	d.Click(center(r))
	if v != "b" {
		t.Errorf("value %q, want b", v)
	}
}

// TestRateTooltipsCharacterFunc: the tooltip of the star under the
// pointer shows; CharacterFunc and Icons are kept per star.
func TestRateTooltipsCharacterFunc(t *testing.T) {
	v := 0.0
	var w *RateW
	d := NewDriver(func(c *Context) {
		w = Rate(c, &v).Tooltips("terrible", "bad", "normal", "good", "wonderful").
			CharacterFunc(func(i int) string { return string(rune('1' + i)) }).Icons(icon.Heart, nil)
	}, DriverOptions{})
	d.Frame()
	rr, _ := d.r.c.RectOf(w.n.id)
	s, g := w.starSize(), w.gap()
	d.Move(geom.Pt(rr.X+2*(s+g)+s/2, rr.Y+rr.H/2))
	d.Frame()
	if _, ok := d.FindText("normal"); !ok {
		t.Error("tooltip of the third star not shown")
	}
	if w.charFn(2) != "3" || w.icOn != icon.Heart {
		t.Error("character func / icons not kept")
	}
}

// TestButtonAutoInsertSpacePrecedence: explicit call > Defaults > theme
// token, in both directions.
func TestButtonAutoInsertSpacePrecedence(t *testing.T) {
	for _, tc := range []struct {
		th           *theme.Theme
		def          bool
		want, wantEx string
	}{
		{theme.AntDesign(), false, "确定", "确 定"},  // i18n:ok test label
		{theme.ElementPlus(), true, "确 定", "确定"}, // i18n:ok test label
	} {
		var got, ex string
		NewDriver(func(c *Context) {
			ConfigProvider(c).Defaults("Button", Props{"autoInsertSpace": tc.def}).Body(func() {
				got = Button(c, "确定").shownLabel()                         // i18n:ok test label
				ex = Button(c, "确定").AutoInsertSpace(!tc.def).shownLabel() // i18n:ok test label
			})
		}, DriverOptions{Theme: tc.th}).Frame()
		if got != tc.want || ex != tc.wantEx {
			t.Errorf("%v: Defaults %q, explicit %q; want %q, %q", tc.th.Flavor, got, ex, tc.want, tc.wantEx)
		}
	}
}

// TestLinkDisabledNoHoverUnderline: a disabled Element link shows no hover
// underline (el-link is-hover-underline only when not disabled).
func TestLinkDisabledNoHoverUnderline(t *testing.T) {
	th := theme.ElementPlus()
	var lw *LinkW
	d := NewDriver(func(c *Context) {
		lw = Link(c, "Off")
		lw.Disabled()
	}, DriverOptions{Theme: th})
	d.Frame()
	r, _ := d.Find("Off")
	d.Move(center(r))
	d.Frame()
	if !lw.ia.Hovered && !lw.c.rects[lw.n.id].Contains(center(r)) {
		t.Fatal("pointer not over the link")
	}
	if lw.shownUnderline() {
		t.Error("disabled link underlined on hover")
	}
}

// TestDividerDefaultsVerticalSizeString: Defaults "vertical" and a string
// "size"; Element ignores size (el-divider has none).
func TestDividerDefaultsVerticalSizeString(t *testing.T) {
	var v, s, el *DividerW
	NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Divider", Props{"vertical": true}).Body(func() { v = Divider(c) })
		ConfigProvider(c).Defaults("Divider", Props{"size": "small"}).Body(func() { s = Divider(c) })
	}, DriverOptions{Theme: theme.AntDesign()}).Frame()
	NewDriver(func(c *Context) { el = Divider(c).Size(Small) }, DriverOptions{Theme: theme.ElementPlus()}).Frame()
	if !v.vertical || s.n.ln.Margin.Top != 8 || el.n.ln.Margin.Top != 24 {
		t.Errorf("vertical %v, small margin %v, element small margin %v", v.vertical, s.n.ln.Margin.Top, el.n.ln.Margin.Top)
	}
	if v.n.aState&a11y.Horizontal != 0 || v.n.aState&a11y.Vertical == 0 {
		t.Errorf("Defaults vertical: a11y state %v, want only Vertical", v.n.aState)
	}
}

// TestSegmentedOrientationHorizontalResets: a later "horizontal" undoes
// "vertical" (Defaults or chain).
func TestSegmentedOrientationHorizontalResets(t *testing.T) {
	v := "a"
	var w *SegmentedW
	d := NewDriver(func(c *Context) {
		w = Segmented(c, &v, Opts("a", "b")).Orientation("vertical").Orientation("horizontal")
	}, DriverOptions{})
	d.Frame()
	if w.vertical || w.n.ln.Dir != layout.Row || w.n.aState&a11y.Vertical != 0 {
		t.Error("horizontal did not reset vertical")
	}
}
