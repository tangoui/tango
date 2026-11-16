package ui

// Spec alignment batch 2, group G (tags, badges and status): behaviour
// tests for the gaps closed against antd 6.7 / Element Plus 2.14.8 /
// shadcn v4 (docs/specs/align/2-display.md).

import (
	"math"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// b2gTag builds one tag with mod under th and returns it after a frame.
func b2gTag(th *theme.Theme, mod func(c *Context) *TagW) *TagW {
	var t *TagW
	d := NewDriver(func(c *Context) { t = mod(c) }, DriverOptions{Theme: th})
	d.Frame()
	return t
}

// ---------------------------------------------------------------------------
// Tag

// TestTagVariantDefault: antd 6 tags are "filled" (no border, defaultBg =
// colorFillTertiary over the container); Element tags keep the light
// effect (outlined).
func TestTagVariantDefault(t *testing.T) {
	ant := theme.AntDesign()
	tg := b2gTag(ant, func(c *Context) *TagW { return Tag(c, "a") })
	bg, border, _ := tg.colors()
	if border.A != 0 {
		t.Errorf("Ant default tag border %v, want none (variant filled)", border)
	}
	if want := theme.Hex("#f5f5f5"); bg != want {
		t.Errorf("Ant default tag bg %v, want %v (antd defaultBg)", bg, want)
	}
	tg = b2gTag(ant, func(c *Context) *TagW { return Tag(c, "a").Color("success") })
	if _, border, fg := tg.colors(); border.A != 0 || fg != ant.Success.Base {
		t.Errorf("Ant success filled: border %v fg %v", border, fg)
	}
	el := theme.ElementPlus()
	tg = b2gTag(el, func(c *Context) *TagW { return Tag(c, "a") })
	if _, border, _ := tg.colors(); border.A == 0 {
		t.Error("Element default tag lost its border (light effect = outlined)")
	}
}

// TestTagVariantOutlinedSolid: Variant wins over the theme; outlined keeps
// the tint and adds the border; solid fills with the base color.
func TestTagVariantOutlinedSolid(t *testing.T) {
	ant := theme.AntDesign()
	tg := b2gTag(ant, func(c *Context) *TagW { return Tag(c, "a").Color("error").Variant("outlined") })
	if bg, border, _ := tg.colors(); bg != ant.Error.Bg || border != ant.Error.Border {
		t.Errorf("outlined error: bg %v border %v", bg, border)
	}
	tg = b2gTag(ant, func(c *Context) *TagW { return Tag(c, "a").Color("error").Variant("solid") })
	if bg, border, _ := tg.colors(); bg != ant.Error.Base || border.A != 0 {
		t.Errorf("solid error: bg %v border %v", bg, border)
	}
	tg = b2gTag(ant, func(c *Context) *TagW { return Tag(c, "a").Variant("solid") })
	if bg, _, fg := tg.colors(); bg != ant.Components.Tag.SolidBg || theme.ContrastRatio(fg, bg) < 4.5 {
		t.Errorf("solid default: bg %v fg %v", bg, fg)
	}
}

// TestTagVariantDefaultsKey: Defaults("Tag", {"variant"}) applies; the
// deprecated Borderless / "bordered": false map to filled.
func TestTagVariantDefaultsKey(t *testing.T) {
	el := theme.ElementPlus()
	var a, b, cc *TagW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Tag", Props{"variant": "filled"}).Body(func() { a = Tag(c, "a") })
		b = Tag(c, "b").Borderless()
		ConfigProvider(c).Defaults("Tag", Props{"bordered": false}).Body(func() { cc = Tag(c, "c") })
	}, DriverOptions{Theme: el})
	d.Frame()
	for i, tg := range []*TagW{a, b, cc} {
		if v := tg.effVariant(); v != "filled" {
			t.Errorf("tag %d variant %q, want filled", i, v)
		}
	}
}

// TestTagPresetColor: Ant preset names use the generated palette
// (lightColor 1, lightBorderColor 3, textColor 7).
func TestTagPresetColor(t *testing.T) {
	ant := theme.AntDesign()
	tg := b2gTag(ant, func(c *Context) *TagW { return Tag(c, "a").Color("blue").Variant("outlined") })
	pal := theme.AntPalette(theme.Hex("#1677ff"), false, ant.BgContainer)
	if bg, border, fg := tg.colors(); bg != pal.Bg || border != pal.Border || fg != pal.Active {
		t.Errorf("blue preset: %v %v %v, want %v %v %v", bg, border, fg, pal.Bg, pal.Border, pal.Active)
	}
}

// TestTagDisabledNoClose: a disabled closable tag does not close (method
// and ConfigProvider.ComponentDisabled), is greyed and marked disabled.
func TestTagDisabledNoClose(t *testing.T) {
	for _, viaConfig := range []bool{false, true} {
		var tg *TagW
		closed := false
		d := NewDriver(func(c *Context) {
			body := func() {
				tg = Tag(c, "close me").Closable()
				if !viaConfig {
					tg.Disabled(true)
				}
				if tg.Closed() {
					closed = true
				}
			}
			if viaConfig {
				ConfigProvider(c).ComponentDisabled(true).Body(body)
			} else {
				body()
			}
		}, DriverOptions{})
		d.Frame()
		r, ok := d.FindText("close me")
		if !ok {
			t.Fatal("tag not found")
		}
		d.Click(geom.Pt(r.Right()-6, r.Y+r.H/2))
		if closed {
			t.Errorf("viaConfig=%v: disabled tag closed", viaConfig)
		}
		var fg theme.Color
		_, _, fg = tg.colors()
		if fg != theme.AntDesign().Components.Tag.DisabledColor {
			t.Errorf("viaConfig=%v: label %v, want the disabled color", viaConfig, fg)
		}
	}
}

// TestTagCloseIcon: CloseIcon makes the tag closable with that icon.
func TestTagCloseIcon(t *testing.T) {
	closed := false
	d := NewDriver(func(c *Context) {
		if Tag(c, "x").CloseIcon(icon.CloseCircle).Closed() {
			closed = true
		}
	}, DriverOptions{})
	d.Frame()
	r, _ := d.FindText("x")
	d.Click(geom.Pt(r.Right()-6, r.Y+r.H/2))
	if !closed {
		t.Error("CloseIcon tag did not close")
	}
}

// TestTagShadcnBadgeVariants: shadcn badge.tsx ghost (transparent,
// foreground) and link (text-primary); rounded-full.
func TestTagShadcnBadgeVariants(t *testing.T) {
	sh := theme.Shadcn()
	tg := b2gTag(sh, func(c *Context) *TagW { return Tag(c, "g").Color("ghost") })
	if bg, border, fg := tg.colors(); bg.A != 0 || border.A != 0 || fg != sh.Text {
		t.Errorf("ghost: %v %v %v", bg, border, fg)
	}
	tg = b2gTag(sh, func(c *Context) *TagW { return Tag(c, "l").Color("link") })
	if _, _, fg := tg.colors(); fg != sh.Primary.Base {
		t.Errorf("link fg %v, want primary", fg)
	}
	if sh.Components.Tag.Radius < 100 {
		t.Errorf("shadcn badge radius %v, want rounded-full", sh.Components.Tag.Radius)
	}
}

// TestAlignB2GShadcnDestructiveDark: the shadcn destructive badge in dark
// mode is bg-destructive/60 over the background (badge.tsx:15, like the
// destructive button) and its white label reaches 4.5:1.
func TestAlignB2GShadcnDestructiveDark(t *testing.T) {
	dk := theme.Shadcn(theme.Dark())
	tg := b2gTag(dk, func(c *Context) *TagW { return Tag(c, "Error").Color("destructive") })
	bg, _, fg := tg.colors()
	want := theme.Over(theme.Fade(dk.Shadcn.Destructive, .6), dk.Shadcn.Background)
	if bg != want {
		t.Errorf("dark destructive bg %v, want %v", bg, want)
	}
	if r := theme.ContrastRatio(fg, bg); r < 4.5 {
		t.Errorf("dark destructive label contrast %.2f, want >= 4.5", r)
	}
	lt := theme.Shadcn()
	tg = b2gTag(lt, func(c *Context) *TagW { return Tag(c, "Error").Color("destructive") })
	if bg, _, _ := tg.colors(); bg != lt.Shadcn.Destructive {
		t.Errorf("light destructive bg %v, want bg-destructive %v", bg, lt.Shadcn.Destructive)
	}
}

// ---------------------------------------------------------------------------
// CheckableTag

// TestCheckableTagHoverFill: antd unchecked hover is colorPrimary on
// colorFillSecondary.
func TestCheckableTagHoverFill(t *testing.T) {
	var w *CheckableTagW
	on := false
	d := NewDriver(func(c *Context) { w = CheckableTag(c, "Movies", &on) }, DriverOptions{})
	d.Frame()
	r, _ := d.FindText("Movies")
	d.Move(center(r))
	bg, fg := w.colors()
	th := theme.AntDesign()
	if bg != th.FillSecondary || fg != th.Primary.Base {
		t.Errorf("hover: bg %v fg %v", bg, fg)
	}
}

// TestCheckableTagGroupSingle: single mode replaces the value and clears it
// when the checked tag is clicked again.
func TestCheckableTagGroupSingle(t *testing.T) {
	var sel []string
	changes := 0
	d := NewDriver(func(c *Context) {
		if CheckableTagGroup(c, &sel, Opts("Movies", "Books", "Music")).Changed() {
			changes++
		}
	}, DriverOptions{})
	d.Frame()
	d.ClickLabel("Movies")
	d.ClickLabel("Books")
	if len(sel) != 1 || sel[0] != "Books" {
		t.Fatalf("single: %v, want [Books]", sel)
	}
	d.ClickLabel("Books")
	if len(sel) != 0 {
		t.Errorf("unchecking: %v, want empty", sel)
	}
	if changes != 3 {
		t.Errorf("Changed %d times, want 3", changes)
	}
}

// TestCheckableTagGroupMultiple toggles each value.
func TestCheckableTagGroupMultiple(t *testing.T) {
	sel := []string{"Music"}
	d := NewDriver(func(c *Context) {
		CheckableTagGroup(c, &sel, Opts("Movies", "Books", "Music")).Multiple(true)
	}, DriverOptions{})
	d.Frame()
	d.ClickLabel("Movies")
	d.ClickLabel("Music")
	if len(sel) != 1 || sel[0] != "Movies" {
		t.Errorf("multiple: %v, want [Movies]", sel)
	}
}

// TestCheckableTagGroupDisabled: a disabled group (or option) ignores
// clicks.
func TestCheckableTagGroupDisabled(t *testing.T) {
	var sel []string
	opts := Opts("A", "B")
	opts[1].Disabled = true
	dis := true
	d := NewDriver(func(c *Context) {
		CheckableTagGroup(c, &sel, opts).Disabled(dis)
	}, DriverOptions{})
	d.Frame()
	d.ClickLabel("A")
	if len(sel) != 0 {
		t.Errorf("disabled group changed: %v", sel)
	}
	dis = false
	d.Frame()
	d.ClickLabel("B")
	if len(sel) != 0 {
		t.Errorf("disabled option changed: %v", sel)
	}
}

// ---------------------------------------------------------------------------
// Badge

// TestBadgeOutOfRangeCount: negative counts are shown (antd, Element),
// zero hides unless ShowZero, overflow folds into "N+".
func TestBadgeOutOfRangeCount(t *testing.T) {
	for _, tc := range []struct {
		n, max   int
		showZero bool
		want     string
		shown    bool
	}{
		{-5, 99, false, "-5", true},
		{0, 99, false, "", false},
		{0, 99, true, "0", true},
		{120, 99, false, "99+", true},
		{12, 10, false, "10+", true},
	} {
		var b *BadgeW
		d := NewDriver(func(c *Context) {
			b = Badge(c).Count(tc.n).Max(tc.max)
			if tc.showZero {
				b.ShowZero()
			}
		}, DriverOptions{})
		d.Frame()
		if s, ok := b.content(); s != tc.want || ok != tc.shown {
			t.Errorf("Count(%d): %q %v, want %q %v", tc.n, s, ok, tc.want, tc.shown)
		}
	}
}

// TestBadgeHiddenTitle: Hidden hides the bubble (Element hidden); Title
// replaces the announced count (antd title).
func TestBadgeHiddenTitle(t *testing.T) {
	var hid *BadgeW
	d := NewDriver(func(c *Context) {
		hid = Badge(c).Count(5).Hidden(true)
		Badge(c).Count(7).Title("7 unread messages")
	}, DriverOptions{Accessibility: true})
	d.Frame()
	d.Frame()
	if _, ok := hid.content(); ok {
		t.Error("hidden badge still has content")
	}
	if _, ok := d.A11yFind(a11y.RoleText, "7 unread messages"); !ok {
		t.Error("badge title not announced")
	}
}

// ---------------------------------------------------------------------------
// Avatar

// TestAvatarSizeShape: Small / Large use the container size tokens, Shape
// sets the shape (antd size small|large, shape).
func TestAvatarSizeShape(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus()} {
		var s, l *AvatarW
		d := NewDriver(func(c *Context) {
			s = Avatar(c, "S").Small()
			l = Avatar(c, "L").Large().Shape("square")
		}, DriverOptions{Theme: th})
		d.Frame()
		tk := th.Components.Avatar
		if s.size != tk.ContainerSizeSM || l.size != tk.ContainerSizeLG || !l.square || s.square {
			t.Errorf("%s: small %v large %v square %v", th.Name, s.size, l.size, l.square)
		}
	}
}

// TestAvatarGroupMaxPopover: the "+k" avatar opens a popover listing the
// folded avatars on hover (antd max.popover).
func TestAvatarGroupMaxPopover(t *testing.T) {
	d := NewDriver(func(c *Context) {
		AvatarGroup(c).Max(2).Body(func() {
			for _, s := range []string{"A", "B", "C", "D"} {
				Avatar(c, s)
			}
		})
	}, DriverOptions{})
	d.Frame()
	if _, ok := d.FindText("C"); ok {
		t.Fatal("folded avatar visible before hover")
	}
	r, ok := d.FindText("+2")
	if !ok {
		t.Fatal("+2 avatar missing")
	}
	d.Move(center(r))
	d.Frame()
	if _, ok := d.FindText("C"); !ok {
		t.Error("popover does not list the folded avatars")
	}
}

// ---------------------------------------------------------------------------
// Statistic

// TestStatisticSeparators: groupSeparator / decimalSeparator; precision
// pads or cuts like antd's StatisticNumber (no rounding).
func TestStatisticSeparators(t *testing.T) {
	var a, b, cc, dd *StatisticW
	d := NewDriver(func(c *Context) {
		a = Statistic(c, "", 1234567.891, 2).GroupSeparator(".").DecimalSeparator(",")
		b = Statistic(c, "", 1.239, 2)
		cc = Statistic(c, "", 5, 2)
		dd = Statistic(c, "", -1234.5, -1)
	}, DriverOptions{})
	d.Frame()
	for _, tc := range []struct {
		s    *StatisticW
		want string
	}{{a, "1.234.567,89"}, {b, "1.23"}, {cc, "5.00"}, {dd, "-1,234.5"}} {
		if tc.s.value != tc.want {
			t.Errorf("value %q, want %q", tc.s.value, tc.want)
		}
	}
}

// TestStatisticOutOfRangeNaN: NaN and ±Inf render as text without panic.
func TestStatisticOutOfRangeNaN(t *testing.T) {
	var a, b *StatisticW
	d := NewDriver(func(c *Context) {
		a = Statistic(c, "x", math.NaN(), 2)
		b = Statistic(c, "y", math.Inf(-1), 0)
	}, DriverOptions{})
	d.Frame()
	if a.value != "NaN" || b.value != "-Infinity" {
		t.Errorf("NaN %q, -Inf %q", a.value, b.value)
	}
}

// ---------------------------------------------------------------------------
// Empty

// TestEmptyTitle: Title adds a heading above the description (shadcn
// EmptyTitle).
func TestEmptyTitle(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Empty(c).Title("No projects").Description("Create one to start")
	}, DriverOptions{Theme: theme.Shadcn(), Accessibility: true})
	d.Frame()
	d.Frame()
	tr, ok1 := d.FindText("No projects")
	dr, ok2 := d.FindText("Create one to start")
	if !ok1 || !ok2 || tr.Y >= dr.Y {
		t.Fatalf("title %v %v, description %v %v", tr, ok1, dr, ok2)
	}
	if _, ok := d.A11yFind(a11y.RoleHeading, "No projects"); !ok {
		t.Error("empty title is not a heading")
	}
}

// ---------------------------------------------------------------------------
// Skeleton

// skelCount counts the skeleton bars in the last frame.
func skelCount(n *node) int {
	k := 0
	if n.aHidden && n.paint != nil && len(n.kids) == 0 {
		k++
	}
	for _, c := range n.kids {
		k += skelCount(c)
	}
	return k
}

// TestSkeletonDefaultRows: three paragraph rows with a title, two next to
// an avatar (antd getParagraphBasicProps); the avatar is 40px.
func TestSkeletonDefaultRows(t *testing.T) {
	for _, tc := range []struct {
		build func(c *Context)
		want  int
	}{
		{func(c *Context) { Skeleton(c).Show() }, 1 + 3},
		{func(c *Context) { Skeleton(c).Avatar(false).Show() }, 1 + 1 + 2},
		{func(c *Context) { Skeleton(c).NoTitle().Show() }, 2},
		{func(c *Context) { Skeleton(c).Avatar(false).Rows(4).Show() }, 1 + 1 + 4},
		{func(c *Context) { Skeleton(c).Count(3).Show() }, 3 * 4},
	} {
		d := NewDriver(tc.build, DriverOptions{})
		d.Frame()
		if n := skelCount(d.r.c.root); n != tc.want {
			t.Errorf("bars %d, want %d", n, tc.want)
		}
	}
	if s := theme.AntDesign().Components.Skeleton.AvatarSize; s != 40 {
		t.Errorf("avatar size %v, want 40", s)
	}
}

// ---------------------------------------------------------------------------
// Progress

// TestProgressOutOfRange: NaN, negative and >100 percentages clamp to
// 0..100 (antd validProgress).
func TestProgressOutOfRange(t *testing.T) {
	for _, tc := range []struct{ in, want float64 }{{math.NaN(), 0}, {-5, 0}, {150, 100}, {42, 42}} {
		var p *ProgressW
		d := NewDriver(func(c *Context) { p = Progress(c, tc.in).Circle(80) }, DriverOptions{})
		d.Frame()
		if p.percent != tc.want {
			t.Errorf("Progress(%v) = %v, want %v", tc.in, p.percent, tc.want)
		}
	}
}

// TestProgressGapPlacement: the ring opening is centred on the placement;
// start / end follow the direction.
func TestProgressGapPlacement(t *testing.T) {
	near := func(a, b float64) bool { return math.Abs(a-b) < 1e-9 }
	for _, tc := range []struct {
		place string
		rtl   bool
		mid   float64
	}{
		{"", false, math.Pi / 2},
		{"top", false, -math.Pi / 2},
		{"start", false, math.Pi},
		{"start", true, 0},
		{"end", false, 0},
	} {
		var start, total float64
		d := NewDriver(func(c *Context) {
			body := func() {
				p := Progress(c, 50).Circle(80).GapDegree(90).GapPlacement(tc.place)
				start, total = p.ringArc()
			}
			if tc.rtl {
				ConfigProvider(c).Direction(RTL).Body(body)
			} else {
				body()
			}
		}, DriverOptions{})
		d.Frame()
		g := math.Pi / 2
		if !near(total, 2*math.Pi-g) || !near(start, tc.mid+g/2) {
			t.Errorf("%q rtl=%v: start %.3f total %.3f, want start %.3f", tc.place, tc.rtl, start, total, tc.mid+g/2)
		}
	}
}

// TestProgressPercentPosition: center/outer puts the text under the bar
// (taller); start/outer keeps the height; ColorFunc and RailColor apply.
func TestProgressPercentPosition(t *testing.T) {
	var plain, below *ProgressW
	d := NewDriver(func(c *Context) {
		plain = Progress(c, 30).Layout(layout.W(300))
		below = Progress(c, 30).PercentPosition("center", "outer").Layout(layout.W(300))
	}, DriverOptions{})
	d.Frame()
	ph := plain.measure(300, 0).H
	bh := below.measure(300, 0).H
	if bh <= ph {
		t.Errorf("center/outer height %v, want more than %v", bh, ph)
	}
	if below.infoWidth() != 0 {
		t.Error("center/outer text still takes width beside the bar")
	}
	red := theme.Hex("#ff0000")
	var p *ProgressW
	d = NewDriver(func(c *Context) {
		p = Progress(c, 80).ColorFunc(func(pct float64) theme.Color {
			if pct > 50 {
				return red
			}
			return theme.Hex("#00ff00")
		}).RailColor(red)
	}, DriverOptions{})
	d.Frame()
	if p.barColor() != red || p.trailColor() != red {
		t.Errorf("ColorFunc %v, RailColor %v", p.barColor(), p.trailColor())
	}
}

// ---------------------------------------------------------------------------
// Tooltip and Popover

// TestTooltipDelayFromTheme: with animations on, an Ant tooltip waits the
// antd mouseEnterDelay (0.1 s) before showing; Element shows at once.
func TestTooltipDelayFromTheme(t *testing.T) {
	for _, tc := range []struct {
		th        *theme.Theme
		openFirst bool
	}{{theme.AntDesign(), false}, {theme.ElementPlus(), true}} {
		var tip *TooltipW
		d := NewDriver(func(c *Context) {
			tip = Tooltip(c, "hint", func() { Button(c, "Hover me") })
		}, DriverOptions{Theme: tc.th, Animations: true})
		d.Frame()
		r, _ := d.FindText("Hover me")
		d.Move(center(r))
		d.Frame()
		if tip.IsOpen() != tc.openFirst {
			t.Errorf("%s: open right after hover = %v, want %v", tc.th.Name, tip.IsOpen(), tc.openFirst)
		}
		d.Advance(150 * time.Millisecond)
		d.Frame()
		if !tip.IsOpen() {
			t.Errorf("%s: not open after the delay", tc.th.Name)
		}
	}
}

// TestTooltipOpenControlled: Open(&v) shows the tooltip without hover and
// hides it on v=false; Disabled never shows it.
func TestTooltipOpenControlled(t *testing.T) {
	open, dis := true, false
	var tip *TooltipW
	d := NewDriver(func(c *Context) {
		tip = Tooltip(c, "hint", func() { Text(c, "anchor") }).Open(&open).Disabled(dis)
	}, DriverOptions{})
	d.Frame()
	if !tip.IsOpen() {
		t.Fatal("Open(true) did not show the tooltip")
	}
	open = false
	d.Frame()
	if tip.IsOpen() {
		t.Error("Open(false) did not hide the tooltip")
	}
	open, dis = true, true
	d.Frame()
	if tip.IsOpen() {
		t.Error("disabled tooltip shown")
	}
}

// TestPopoverDelayAndWidth: Ant hover popovers wait 0.1 s; Element
// popovers are 150px wide (el-popover width).
func TestPopoverDelayAndWidth(t *testing.T) {
	var p *PopoverW
	d := NewDriver(func(c *Context) {
		p = Popover(c).Text("content")
		p.Body(func() { Button(c, "Trigger") })
	}, DriverOptions{Animations: true})
	d.Frame()
	r, _ := d.FindText("Trigger")
	d.Move(center(r))
	d.Frame()
	if p.IsOpen() {
		t.Error("Ant popover opened before mouseEnterDelay")
	}
	d.Advance(150 * time.Millisecond)
	d.Frame()
	d.Frame()
	if !p.IsOpen() {
		t.Error("Ant popover not open after the delay")
	}

	open := true
	d = NewDriver(func(c *Context) {
		Popover(c).Trigger("click").Open(&open).Text("x").Body(func() { Button(c, "T") })
	}, DriverOptions{Theme: theme.ElementPlus()})
	d.Frame()
	d.Frame()
	r, ok := d.FindText("x")
	if !ok {
		t.Fatal("popover text missing")
	}
	var panel *node
	for _, ov := range d.r.c.overlays {
		if n := b2gFindRole(ov.root, a11y.RoleDialog); n != nil {
			panel = n
		}
	}
	if panel == nil || panel.abs.W != 150 {
		t.Errorf("Element popover panel %v, want 150 wide", panel)
	}
	if w := theme.ElementPlus().Components.Popover.Width; w != 150 {
		t.Errorf("Element popover width token %v, want 150", w)
	}
}

// b2gFindRole returns the first node of n's subtree with role r.
func b2gFindRole(n *node, r a11y.Role) *node {
	if n == nil {
		return nil
	}
	if n.role == r {
		return n
	}
	for _, k := range n.kids {
		if f := b2gFindRole(k, r); f != nil {
			return f
		}
	}
	return nil
}

// TestTagRound: Round draws a pill (Element round).
func TestTagRound(t *testing.T) {
	th := theme.AntDesign()
	d := NewDriver(func(c *Context) { Tag(c, "pill").Round() }, DriverOptions{Theme: th})
	d.Frame()
	found := false
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Color == th.Components.Tag.DefaultBg {
			found = true
			if f.Radii.TL != f.Rect.H/2 {
				t.Errorf("radius %v, want %v", f.Radii.TL, f.Rect.H/2)
			}
		}
	}
	if !found {
		t.Error("tag fill not found")
	}
}

// ---------------------------------------------------------------------------
// Review fixes

// TestSkeletonSquareAvatar: Avatar(true) draws a square avatar with the
// block radius (it used to be round).
func TestSkeletonSquareAvatar(t *testing.T) {
	th := theme.AntDesign()
	tk := th.Components.Skeleton
	d := NewDriver(func(c *Context) { Skeleton(c).Avatar(true).Show() }, DriverOptions{Theme: th})
	d.Frame()
	found := false
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.FillRect); ok && f.Rect.W == tk.AvatarSize && f.Rect.H == tk.AvatarSize {
			found = true
			if f.Radii.TL != tk.BlockRadius {
				t.Errorf("square avatar radius %v, want %v", f.Radii.TL, tk.BlockRadius)
			}
		}
	}
	if !found {
		t.Error("avatar block not painted")
	}
}

// TestTooltipDisabledNoWrite: a disabled controlled tooltip neither shows
// nor writes the Open value on hover.
func TestTooltipDisabledNoWrite(t *testing.T) {
	open := false
	changed := false
	d := NewDriver(func(c *Context) {
		tip := Tooltip(c, "hint", func() { Button(c, "Hover") }).Open(&open).Disabled(true)
		if tip.OpenChanged() {
			changed = true
		}
	}, DriverOptions{})
	d.Frame()
	r, _ := d.FindText("Hover")
	d.Move(center(r))
	d.Frame()
	if open || changed {
		t.Errorf("disabled tooltip wrote open=%v changed=%v", open, changed)
	}
}

// TestTooltipControlledClickEsc: a controlled click tooltip follows the
// application's value (one click reopens after the app closed it), and
// Escape closes an application-opened hover tooltip.
func TestTooltipControlledClickEsc(t *testing.T) {
	open := false
	d := NewDriver(func(c *Context) {
		Tooltip(c, "hint", func() { Button(c, "Click") }).Trigger("click").Open(&open)
	}, DriverOptions{})
	d.Frame()
	d.ClickLabel("Click")
	if !open {
		t.Fatal("click did not open")
	}
	open = false // the application closes it
	d.Frame()
	d.ClickLabel("Click")
	if !open {
		t.Error("one click after the app closed it did not reopen")
	}

	hopen := true
	d = NewDriver(func(c *Context) {
		Tooltip(c, "hint", func() { Button(c, "Hover") }).Open(&hopen)
	}, DriverOptions{})
	d.Frame()
	d.Frame()
	d.Key(input.KeyEscape, 0)
	d.Frame()
	if hopen {
		t.Error("Escape did not close the application-opened tooltip")
	}
}

// TestPopoverHoverGrace: without animations the hover popover still keeps
// a 100ms leave grace so the pointer can reach the panel.
func TestPopoverHoverGrace(t *testing.T) {
	var p *PopoverW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			p = Popover(c).Placement("bottom").Text("panel text")
			p.Body(func() { Button(c, "Trigger") })
		}, layout.Pad(40))
	}, DriverOptions{})
	d.Frame()
	r, _ := d.FindText("Trigger")
	d.Move(center(r))
	d.Frame()
	if !p.IsOpen() {
		t.Fatal("hover did not open")
	}
	d.Move(geom.Pt(r.X+r.W/2, r.Bottom()+5)) // the gap between trigger and panel
	d.Advance(50 * time.Millisecond)
	d.Frame()
	pr, ok := d.FindText("panel text")
	if !ok || !p.IsOpen() {
		t.Fatal("popover closed inside the grace period")
	}
	d.Move(center(pr))
	d.Advance(200 * time.Millisecond)
	d.Frame()
	if !p.IsOpen() {
		t.Error("popover closed with the pointer on the panel")
	}
}

// TestAvatarGroupPopoverAutoWidth: the "+N" popover fits the folded
// avatars instead of Element's default 150px popover width.
func TestAvatarGroupPopoverAutoWidth(t *testing.T) {
	th := theme.ElementPlus()
	d := NewDriver(func(c *Context) {
		AvatarGroup(c).Max(1).Body(func() {
			for _, s := range []string{"A", "B", "C", "D", "E", "F"} {
				Avatar(c, s)
			}
		})
	}, DriverOptions{Theme: th})
	d.Frame()
	r, _ := d.FindText("+5")
	d.Move(center(r))
	d.Frame()
	fr, ok1 := d.FindText("B")
	lr, ok2 := d.FindText("F")
	if !ok1 || !ok2 {
		t.Fatal("folded avatars not listed")
	}
	if lr.Right()-fr.X < 5*th.Components.Avatar.ContainerSize {
		t.Errorf("folded avatars span %v, want at least %v", lr.Right()-fr.X, 5*th.Components.Avatar.ContainerSize)
	}
	var panel *node
	for _, ov := range d.r.c.overlays {
		if n := b2gFindRole(ov.root, a11y.RoleTooltip); n != nil {
			panel = n
		}
	}
	if panel == nil {
		t.Fatal("popover panel not found")
	}
	if panel.abs.Right() < lr.Right() || panel.abs.W == th.Components.Popover.Width {
		t.Errorf("popover %v does not fit the avatars ending at %v", panel.abs, lr.Right())
	}
}

// TestCheckableTagGroupNilSelection: a nil selection renders unchecked
// tags and ignores clicks without panicking.
func TestCheckableTagGroupNilSelection(t *testing.T) {
	d := NewDriver(func(c *Context) { CheckableTagGroup(c, nil, Opts("A", "B")) }, DriverOptions{})
	d.Frame()
	d.ClickLabel("A")
}

// TestTagDisabledA11yConfig: under ConfigProvider.ComponentDisabled a
// plain (not closable) tag is reported disabled.
func TestTagDisabledA11yConfig(t *testing.T) {
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() { Tag(c, "plain") })
	}, DriverOptions{Accessibility: true})
	d.Frame()
	d.Frame()
	n, ok := d.A11yFind(a11y.RoleText, "plain")
	if !ok || n.States&a11y.Disabled == 0 {
		t.Errorf("tag a11y %v, want Disabled", n)
	}
}
