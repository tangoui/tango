package ui

import (
	"math"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// Batch 4 group A2: StatisticCard / ProStatistic / CheckCard against
// @ant-design/pro-components@3.1.14-2 (es/card/components/*), checked
// 2026-11-15. Expected values are the source values.

// a2Node returns the first node (depth first) labelled label.
func a2Node(d *Driver, label string) *node {
	var found *node
	var walk func(n *node)
	walk = func(n *node) {
		if found != nil {
			return
		}
		if n.label == label {
			found = n
			return
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(d.r.c.root)
	return found
}

// a2TextColor returns the color of the text drawn inside the leaf
// labelled label.
func a2TextColor(t *testing.T, d *Driver, label string) theme.Color {
	t.Helper()
	r := pro2MustFind(t, d, label)
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.Text); ok && r.Expand(1).Contains(o.Origin) {
			return o.Color
		}
	}
	t.Fatalf("no text op inside %q", label)
	return theme.Color{}
}

func a2Near(a, b float32) bool { return math.Abs(float64(a-b)) < 0.6 }

func a2Ant(w, h float32, page func(c *Context)) (*theme.Theme, *Driver) {
	th := theme.AntDesign()
	return th, pro2Driver(th, w, h, page)
}

// ---------------------------------------------------------------------------
// StatisticCard

// TestStatisticCardTitleColor: Pro overrides the antd statistic title
// color with colorText (pro/Statistic/style.js `.ant-statistic-title`).
func TestStatisticCardTitleColor(t *testing.T) {
	th, d := a2Ant(600, 300, func(c *Context) {
		StatisticCard(c, "Sales").Value(1, 0).Show()
	})
	if got := a2TextColor(t, d, "Sales"); got != th.Text {
		t.Fatalf("title color %v, want colorText %v", got, th.Text)
	}
}

// TestStatisticCardTrendColorsValue: trend up / down colors the whole
// value colorError / colorSuccess (pro/Statistic/style.js `&-trend-up`);
// ValueColor still wins.
func TestStatisticCardTrendColorsValue(t *testing.T) {
	custom := theme.Hex("#123456")
	th, d := a2Ant(900, 300, func(c *Context) {
		Row(c, func() {
			StatisticCard(c, "Up").ValueText("111").Trend("up").Show()
			StatisticCard(c, "Down").ValueText("222").Trend("down").Show()
			StatisticCard(c, "Own").ValueText("333").Trend("up").ValueColor(custom).Show()
		})
	})
	if got := a2TextColor(t, d, "111"); got != th.Error.Base {
		t.Fatalf("up value %v, want colorError", got)
	}
	if got := a2TextColor(t, d, "222"); got != th.Success.Base {
		t.Fatalf("down value %v, want colorSuccess", got)
	}
	if got := a2TextColor(t, d, "333"); got != custom {
		t.Fatalf("ValueColor lost: %v", got)
	}
}

// TestStatisticCardTrendIconSize: the trend triangle is 7×9 (borders
// 3.5 + 3.5 wide, 9 tall; pro/Statistic/style.js `&-trend-icon`) and sits
// before the value.
func TestStatisticCardTrendIconSize(t *testing.T) {
	_, d := a2Ant(600, 300, func(c *Context) {
		StatisticCard(c, "S").ValueText("42").Trend("up").Show()
	})
	n := a2Node(d, "up")
	if n == nil {
		t.Fatal("no trend mark")
	}
	if n.abs.W != 7 || n.abs.H != 9 {
		t.Fatalf("trend mark %vx%v, want 7x9", n.abs.W, n.abs.H)
	}
	if n.abs.X >= pro2MustFind(t, d, "42").X {
		t.Fatal("trend mark not before the value")
	}
}

// TestStatisticCardStatusTip: the status dot column is 14 wide
// (`&-wrapper &-status`), the tip icon starts 4 after the title (`&-tip`).
func TestStatisticCardStatusTip(t *testing.T) {
	_, d := a2Ant(600, 300, func(c *Context) {
		StatisticCard(c, "Title").ValueText("1").Status("success").Tooltip("help").Show()
	})
	title := pro2MustFind(t, d, "Title")
	st := a2Node(d, "pro2-stat-status")
	if st == nil || st.abs.W != 14 {
		t.Fatalf("status column %v", st)
	}
	if !a2Near(title.X-st.abs.X, 14) {
		t.Fatalf("title starts %v after the status column, want 14", title.X-st.abs.X)
	}
	tip := a2Node(d, "pro2-stat-tip")
	if tip == nil || !a2Near(tip.abs.X-title.Right(), 4) {
		t.Fatalf("tip gap: %v", tip)
	}
}

// TestStatisticCardDescInline: Desc items are inline Statistics: stacked
// with 4 between (`& + &`), fontSizeSM, title colorText, value
// colorTextSecondary, the trend mark before the value.
func TestStatisticCardDescInline(t *testing.T) {
	th, d := a2Ant(600, 400, func(c *Context) {
		StatisticCard(c, "S").ValueText("9").Desc("Week", "12%", "up").Desc("Day", "11%", "down").Show()
	})
	week, day := pro2MustFind(t, d, "Week"), pro2MustFind(t, d, "Day")
	if day.Y <= week.Y {
		t.Fatal("desc items not stacked")
	}
	if !a2Near(day.Y-week.Bottom(), 4) {
		t.Fatalf("desc gap %v, want 4", day.Y-week.Bottom())
	}
	if !a2Near(week.H, th.LineHeightFor(th.FontSizeSM)) {
		t.Fatalf("desc title height %v, want fontSizeSM line", week.H)
	}
	if got := a2TextColor(t, d, "Week"); got != th.Text {
		t.Fatalf("desc title color %v", got)
	}
	if got := a2TextColor(t, d, "12%"); got != th.TextSecondary {
		t.Fatalf("desc value color %v", got)
	}
	v := pro2MustFind(t, d, "12%")
	if !a2Near(v.X-week.Right(), 6) && v.X <= week.Right() {
		t.Fatal("desc value not after the title")
	}
	var mark *node
	var walk func(n *node)
	walk = func(n *node) {
		if n.label == "up" && n.abs.Y >= week.Y-1 && n.abs.Bottom() <= week.Bottom()+1 {
			mark = n
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(d.r.c.root)
	if mark == nil || mark.abs.X >= v.X {
		t.Fatalf("desc trend mark not before the value: %v", mark)
	}
}

// TestStatisticCardLayoutHorizontal: layout horizontal puts title and
// value on one line, space-between, at fontSize (`& &-layout-horizontal`);
// the "layout" Defaults key selects it.
func TestStatisticCardLayoutHorizontal(t *testing.T) {
	for _, viaDefaults := range []bool{false, true} {
		th, d := a2Ant(500, 300, func(c *Context) {
			build := func() {
				w := StatisticCard(c, "Left").ValueText("Right").Layout(layout.W(300))
				if !viaDefaults {
					w.StatisticLayout(StatisticHorizontal)
				}
				w.Show()
			}
			if viaDefaults {
				ConfigProvider(c).Defaults("StatisticCard", Props{"layout": "horizontal"}).Body(build)
			} else {
				build()
			}
		})
		l, r := pro2MustFind(t, d, "Left"), pro2MustFind(t, d, "Right")
		if !a2Near(l.Y, r.Y) {
			t.Fatalf("defaults=%v: not one line: %v %v", viaDefaults, l, r)
		}
		if r.X < l.Right()+100 {
			t.Fatalf("defaults=%v: value not pushed to the end: %v %v", viaDefaults, l, r)
		}
		if !a2Near(r.H, th.LineHeightFor(th.FontSize)) {
			t.Fatalf("defaults=%v: value height %v, want fontSize line", viaDefaults, r.H)
		}
	}
}

// TestStatisticCardChartPlacement: left / right charts sit 16 from the
// statistic and align to the top; a bottom chart has 8 above and below
// (pro/StatisticCard/style.js `&-chart`).
func TestStatisticCardChartPlacement(t *testing.T) {
	box := func(c *Context, label string, h float32) {
		Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 60, H: h} }, nil).Label(label).Semantics(a11y.RoleFigure, label)
	}
	_, d := a2Ant(900, 500, func(c *Context) {
		Row(c, func() {
			StatisticCard(c, "R").ValueText("Rv").ChartPlace(ChartRight).Chart(func() { box(c, "RC", 20) }).Show()
			StatisticCard(c, "L").ValueText("Lv").ChartPlace(ChartLeft).Chart(func() { box(c, "LC", 20) }).Show()
			StatisticCard(c, "B").ValueText("Bv").Chart(func() { box(c, "BC", 20) }).Footer(func() { Text(c, "Foot") }).Show()
		}, layout.AlignItems(layout.AlignStart))
	})
	stat := func(label string) geom.Rect {
		n := a2Node(d, "pro2-stat-"+label)
		if n == nil {
			t.Fatalf("no statistic %q", label)
		}
		return n.abs
	}
	rc, lc, bc := a2Node(d, "RC").abs, a2Node(d, "LC").abs, a2Node(d, "BC").abs
	if s := stat("R"); !a2Near(rc.X-s.Right(), 16) || !a2Near(rc.Y, s.Y) {
		t.Fatalf("right chart %v vs statistic %v", rc, s)
	}
	if s := stat("L"); !a2Near(s.X-lc.Right(), 16) || !a2Near(lc.Y, s.Y) {
		t.Fatalf("left chart %v vs statistic %v", lc, s)
	}
	if s := stat("B"); !a2Near(bc.Y-s.Bottom(), 8) {
		t.Fatalf("bottom chart %v gap %v, want 8", bc, bc.Y-s.Bottom())
	}
	// Footer: chart margin 8 + footer margin 8 + padding 16.
	if f := pro2MustFind(t, d, "Foot"); !a2Near(f.Y-bc.Bottom(), 8+8+16) {
		t.Fatalf("footer gap %v, want 32", f.Y-bc.Bottom())
	}
}

// TestStatisticCardFooterSpacing: footer marginBlockStart marginXS (8),
// paddingBlockStart padding (16) (pro/StatisticCard/style.js `&-footer`).
func TestStatisticCardFooterSpacing(t *testing.T) {
	_, d := a2Ant(600, 300, func(c *Context) {
		StatisticCard(c, "S").ValueText("Val").Footer(func() { Text(c, "Foot") }).Show()
	})
	s := a2Node(d, "pro2-stat-S").abs
	if f := pro2MustFind(t, d, "Foot"); !a2Near(f.Y-s.Bottom(), 24) {
		t.Fatalf("footer gap %v, want 8+16", f.Y-s.Bottom())
	}
}

// TestProStatisticInline: the standalone Pro Statistic defaults to the
// inline layout (pro/Statistic/index.js `layout = 'inline'`): title and
// value on one line 6 apart at fontSizeSM; consecutive statistics stack 4
// apart; each is a named group.
func TestProStatisticInline(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ProStatistic(c, "Users").Value(1280, 0).Trend("up").Show()
			ProStatistic(c, "Orders").ValueText("64").Show()
			ProStatistic(c, "Big").ValueText("7").StatisticLayout(StatisticVertical).Show()
		}, layout.Pad(10))
	}, DriverOptions{Width: 400, Height: 300, Accessibility: true})
	d.Settle()
	th := d.r.c.Theme
	u, v := pro2MustFind(t, d, "Users"), pro2MustFind(t, d, "1,280")
	if !a2Near(u.Y, v.Y) || v.X <= u.Right() {
		t.Fatalf("inline title/value not on one line: %v %v", u, v)
	}
	if !a2Near(u.H, th.LineHeightFor(th.FontSizeSM)) {
		t.Fatalf("inline title height %v", u.H)
	}
	if o := pro2MustFind(t, d, "Orders"); !a2Near(o.Y-u.Bottom(), 4) {
		t.Fatalf("stack gap %v, want 4", o.Y-u.Bottom())
	}
	if got := a2TextColor(t, d, "64"); got != th.TextSecondary {
		t.Fatalf("inline value color %v", got)
	}
	b, bv := pro2MustFind(t, d, "Big"), pro2MustFind(t, d, "7")
	if bv.Y <= b.Y {
		t.Fatal("vertical layout not stacked")
	}
	if _, ok := d.A11yFind(a11y.RoleGroup, "Users"); !ok {
		t.Fatal("ProStatistic not a named group")
	}
}

// TestStatisticCardDivider: StatisticCard.Divider is lineWidth wide in
// colorSplit with marginInline 8 / marginBlock 24; horizontal: lineWidth
// tall, marginInline 24 / marginBlock 8 (pro/Card/style.js `-divider`).
func TestStatisticCardDivider(t *testing.T) {
	th, d := a2Ant(600, 400, func(c *Context) {
		Row(c, func() {
			Text(c, "A")
			StatisticCardDivider(c, "vertical")
			Text(c, "B")
		}, layout.H(100), layout.AlignItems(layout.AlignStart))
		Column(c, func() {
			Text(c, "C")
			StatisticCardDivider(c, "horizontal")
			Text(c, "D")
		}, layout.W(200))
	})
	a, b := pro2MustFind(t, d, "A"), pro2MustFind(t, d, "B")
	if !a2Near(b.X-a.Right(), 8+1+8) {
		t.Fatalf("vertical divider spacing %v", b.X-a.Right())
	}
	cR, dR := pro2MustFind(t, d, "C"), pro2MustFind(t, d, "D")
	if !a2Near(dR.Y-cR.Bottom(), 8+1+8) {
		t.Fatalf("horizontal divider spacing %v", dR.Y-cR.Bottom())
	}
	lines := 0
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.FillRect); ok && o.Color == th.Split && (o.Rect.W == 1 || o.Rect.H == 1) {
			lines++
			if o.Rect.W == 1 && !a2Near(o.Rect.H, 100-48) {
				t.Fatalf("vertical divider height %v, want 100-2×24", o.Rect.H)
			}
			if o.Rect.H == 1 && !a2Near(o.Rect.W, 200-48) {
				t.Fatalf("horizontal divider width %v, want 200-2×24", o.Rect.W)
			}
		}
	}
	if lines != 2 {
		t.Fatalf("divider lines = %d", lines)
	}
}

// TestStatisticCardOperation: StatisticCard.Operation is fontSizeHeading4
// text, strong, colorText, bottom aligned with marginBlock 24
// (pro/Operation/style.js).
func TestStatisticCardOperation(t *testing.T) {
	th, d := a2Ant(600, 300, func(c *Context) {
		Row(c, func() {
			StatisticCardOperation(c, "=")
		}, layout.H(120))
	})
	r := pro2MustFind(t, d, "=")
	if !a2Near(r.H, th.LineHeightFor(th.Heading[3])) {
		t.Fatalf("operation height %v, want h4 line", r.H)
	}
	op := a2Node(d, "pro2-stat-op")
	if op == nil || !a2Near(op.abs.Bottom()-r.Bottom(), 0) || !a2Near(r.Bottom(), op.abs.Bottom()) {
		t.Fatalf("operation not bottom aligned: %v", op)
	}
	if got := a2TextColor(t, d, "="); got != th.Text {
		t.Fatalf("operation color %v", got)
	}
	// marginBlock 24 inside the 120 tall row plus the driver's 10 padding.
	if !a2Near(r.Bottom(), 10+120-24) {
		t.Fatalf("operation bottom %v, want %v", r.Bottom(), 10+120-24)
	}
}

// ---------------------------------------------------------------------------
// CheckCard

func a2Card(t *testing.T, d *Driver, label string) geom.Rect {
	t.Helper()
	r, ok := d.Find(label)
	if !ok {
		t.Fatalf("card %q not found", label)
	}
	return r
}

// TestCheckCardContentPadding: `-content` pads paddingSM (12) inline and
// padding (16) block for every size.
func TestCheckCardContentPadding(t *testing.T) {
	_, d := a2Ant(1200, 400, func(c *Context) {
		Row(c, func() {
			CheckCard(c, "Smol", new(bool)).Small().Show()
			CheckCard(c, "Mid", new(bool)).Show()
			CheckCard(c, "Big", new(bool)).Large().Show()
		}, layout.AlignItems(layout.AlignStart), layout.Gap(16))
	})
	for _, s := range []string{"Smol", "Mid", "Big"} {
		card, title := a2Card(t, d, s), pro2MustFind(t, d, s)
		if !a2Near(title.X-card.X, 12) {
			t.Fatalf("%s inline padding %v, want 12", s, title.X-card.X)
		}
		// Header line is lineHeight (22) tall; the title is centred in it.
		if !a2Near(card.Bottom()-title.Bottom(), 16) {
			t.Fatalf("%s block padding %v, want 16", s, card.Bottom()-title.Bottom())
		}
	}
}

// TestCheckCardTitleSize: the title is fontSize for every size, colored
// colorTextHeading (pro/CheckCard/style.js `&-title`).
func TestCheckCardTitleSize(t *testing.T) {
	th, d := a2Ant(900, 300, func(c *Context) {
		CheckCard(c, "Large", new(bool)).Large().Show()
	})
	r := pro2MustFind(t, d, "Large")
	if !a2Near(r.H, th.LineHeightFor(th.FontSize)) {
		t.Fatalf("large title height %v, want fontSize line", r.H)
	}
	if got := a2TextColor(t, d, "Large"); got != th.TextHeading {
		t.Fatalf("title color %v", got)
	}
}

// TestCheckCardRadius: borderRadius (6), not borderRadiusLG.
func TestCheckCardRadius(t *testing.T) {
	th, d := a2Ant(600, 300, func(c *Context) {
		CheckCard(c, "Round", new(bool)).Show()
	})
	card := a2Card(t, d, "Round")
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.StrokeRect); ok && o.Rect == card {
			if o.Radii != geom.Rad(th.Radius) {
				t.Fatalf("radius %v, want %v", o.Radii, th.Radius)
			}
			return
		}
	}
	t.Fatal("no card border")
}

// TestCheckCardCheckedCorner: the checked mark is the ::after triangle
// with legs 2×(borderRadius+4) = 20, 2 from the top and the end edge, in
// colorPrimary; colorTextDisabled when disabled
// (pro/CheckCard/style.js genCheckedAfterBase, `&-checked&-disabled`).
func TestCheckCardCheckedCorner(t *testing.T) {
	on, dis := true, true
	th, d := a2Ant(900, 300, func(c *Context) {
		Row(c, func() {
			CheckCard(c, "On", &on).Show()
			CheckCard(c, "OnDisabled", &dis).Disabled(true).Show()
		}, layout.AlignItems(layout.AlignStart), layout.Gap(16))
	})
	check := func(label string, col theme.Color) {
		card := a2Card(t, d, label)
		for _, op := range d.r.List().Ops {
			if o, ok := op.(paint.FillPath); ok && o.Color == col && o.Path != nil {
				b := o.Path.Bounds()
				if card.Contains(geom.Pt(b.X+b.W/2, b.Y+b.H/2)) {
					if !a2Near(b.W, 20) || !a2Near(b.H, 20) || !a2Near(b.Y-card.Y, 2) || !a2Near(card.Right()-b.Right(), 2) {
						t.Fatalf("%s corner %v in %v", label, b, card)
					}
					return
				}
			}
		}
		t.Fatalf("%s: no corner mark", label)
	}
	check("On", th.Primary.Base)
	check("OnDisabled", th.TextDisabled)
}

// TestCheckCardDisabledLook: disabled background colorBgContainerDisabled,
// border colorBorder also when checked (proCheckCardDisabled).
func TestCheckCardDisabledLook(t *testing.T) {
	on := true
	th, d := a2Ant(600, 300, func(c *Context) {
		CheckCard(c, "Dis", &on).Disabled(true).Show()
	})
	card := a2Card(t, d, "Dis")
	bg, border := false, false
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Rect == card && o.Color == th.BgContainerDisabled {
				bg = true
			}
		case paint.StrokeRect:
			if o.Rect == card && o.Color == th.Border {
				border = true
			}
		}
	}
	if !bg || !border {
		t.Fatalf("disabled bg %v border %v", bg, border)
	}
}

// TestCheckCardLoading: loading shows a 4-row skeleton padded paddingMD
// (20) and ignores clicks (`if (!cardLoading && !disabled)`).
func TestCheckCardLoading(t *testing.T) {
	on := false
	d := NewDriver(func(c *Context) {
		Column(c, func() { CheckCard(c, "Busy", &on).Loading(true).Show() }, layout.Pad(10))
	}, DriverOptions{Width: 600, Height: 400, Accessibility: true})
	d.Settle()
	card := a2Card(t, d, "Busy")
	d.Click(card.Center())
	d.Settle()
	if on {
		t.Fatal("loading card toggled")
	}
	n, ok := d.A11yFind(a11y.RoleCheckbox, "Busy")
	if !ok || n.States&a11y.Busy == 0 {
		t.Fatalf("loading card not busy: %v", n)
	}
	sk := a2Node(d, "pro2-checkcard-loading")
	if sk == nil || !a2Near(sk.abs.X-card.X, 20) || !a2Near(sk.abs.Y-card.Y, 20) {
		t.Fatalf("loading padding: %v in %v", sk, card)
	}
	if rows := a2Node(d, "pro2-checkcard-loading"); rows == nil {
		t.Fatal("no skeleton")
	}
	if got := checkCardLoadingRows; got != 4 {
		t.Fatalf("skeleton rows %d, want 4", got)
	}
}

// TestCheckCardDefaultChecked: with a nil binding the card keeps its own
// state, starting at DefaultChecked (useControlledState(defaultChecked)).
func TestCheckCardDefaultChecked(t *testing.T) {
	var cc *CheckCardW
	d := NewDriver(func(c *Context) {
		Column(c, func() { cc = CheckCard(c, "Own", nil).DefaultChecked(true).Show() }, layout.Pad(10))
	}, DriverOptions{Width: 600, Height: 300, Accessibility: true})
	d.Settle()
	if n, ok := d.A11yFind(a11y.RoleCheckbox, "Own"); !ok || n.States&a11y.Checked == 0 || !cc.IsChecked() {
		t.Fatal("default checked not applied")
	}
	d.Click(a2Card(t, d, "Own").Center())
	d.Settle()
	if cc.IsChecked() {
		t.Fatal("uncontrolled card did not toggle")
	}
	d.Settle()
	if cc.IsChecked() {
		t.Fatal("default re-applied after the first frame")
	}
}

// TestCheckCardAvatar: a string avatar is a 48 square Avatar for every
// size, padded 8 before the detail; avatar + title without description
// centres them (`-avatar-header`).
func TestCheckCardAvatar(t *testing.T) {
	_, d := a2Ant(1200, 400, func(c *Context) {
		Row(c, func() {
			CheckCard(c, "SmallAv", new(bool)).Avatar("S").Small().Show()
			CheckCard(c, "WithDesc", new(bool)).Avatar("W").Description("text").Show()
		}, layout.AlignItems(layout.AlignStart), layout.Gap(16))
	})
	av := a2Node(d, "S")
	if av == nil || av.abs.W != 48 || av.abs.H != 48 {
		t.Fatalf("avatar %v, want 48x48", av)
	}
	title := pro2MustFind(t, d, "SmallAv")
	if !a2Near(title.X-av.abs.Right(), 8) {
		t.Fatalf("avatar gap %v", title.X-av.abs.Right())
	}
	if !a2Near(title.Y+title.H/2, av.abs.Y+av.abs.H/2) {
		t.Fatalf("avatar header not centred: %v %v", title, av.abs)
	}
	w := a2Node(d, "W")
	if wt := pro2MustFind(t, d, "WithDesc"); !a2Near(wt.Y, w.abs.Y) && wt.Y > w.abs.Y+4 {
		t.Fatalf("with description the title should align to the top: %v %v", wt, w.abs)
	}
}

// TestCheckCardSubTitle: subTitle follows the title sizeSM (12) apart
// (`-header-left gap`).
func TestCheckCardSubTitle(t *testing.T) {
	_, d := a2Ant(600, 300, func(c *Context) {
		CheckCard(c, "Main", new(bool)).SubTitle("Sub").Show()
	})
	m, s := pro2MustFind(t, d, "Main"), pro2MustFind(t, d, "Sub")
	if !a2Near(s.X-m.Right(), 12) || !a2Near(s.Y, m.Y) {
		t.Fatalf("subtitle %v after %v", s, m)
	}
}

// TestCheckCardCover: cover mode pads paddingXXS (4).
func TestCheckCardCover(t *testing.T) {
	_, d := a2Ant(600, 300, func(c *Context) {
		CheckCard(c, "Cov", new(bool)).Cover(func() {
			Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 100, H: 60} }, nil).Label("Pic").Semantics(a11y.RoleImage, "Pic")
		}).Show()
	})
	card, pic := a2Card(t, d, "Cov"), a2Node(d, "Pic")
	if pic == nil || !a2Near(pic.abs.X-card.X, 4) || !a2Near(pic.abs.Y-card.Y, 4) {
		t.Fatalf("cover padding: %v in %v", pic, card)
	}
}

// TestCheckCardContentActions: children go in `-body` (12 / 16 padding)
// below the meta; actions add the ProCard action bar.
func TestCheckCardContentActions(t *testing.T) {
	_, d := a2Ant(600, 400, func(c *Context) {
		CheckCard(c, "Host", new(bool)).Content(func() { Text(c, "Inner") }).
			Actions(func() { Text(c, "Act1") }, func() { Text(c, "Act2") }).Show()
	})
	card, title, inner := a2Card(t, d, "Host"), pro2MustFind(t, d, "Host"), pro2MustFind(t, d, "Inner")
	if !a2Near(inner.X-card.X, 12) || inner.Y <= title.Bottom() {
		t.Fatalf("content %v in %v", inner, card)
	}
	a1, a2 := pro2MustFind(t, d, "Act1"), pro2MustFind(t, d, "Act2")
	if a1.Y <= inner.Bottom() || !a2Near(a1.Y, a2.Y) || a2.X <= a1.Right() {
		t.Fatalf("actions %v %v", a1, a2)
	}
}

// TestCheckCardKeyboardA11y: focusable, Enter toggles; disabled is
// exposed and not reachable.
func TestCheckCardKeyboardA11y(t *testing.T) {
	on, off := false, false
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			CheckCard(c, "Kb", &on).Show()
			CheckCard(c, "Off", &off).Disabled(true).Show()
		}, layout.Pad(10))
	}, DriverOptions{Width: 600, Height: 400, Accessibility: true})
	d.Settle()
	d.Key(input.KeyTab, 0)
	d.Settle()
	d.Key(input.KeyEnter, 0)
	d.Settle()
	if !on {
		t.Fatal("Enter did not toggle")
	}
	d.Key(input.KeyTab, 0)
	d.Settle()
	d.Key(input.KeySpace, 0)
	d.Settle()
	if off {
		t.Fatal("disabled card reached by keyboard")
	}
	if n, ok := d.A11yFind(a11y.RoleCheckbox, "Off"); !ok || n.States&a11y.Disabled == 0 {
		t.Fatalf("disabled not exposed: %v", n)
	}
}

// ---------------------------------------------------------------------------
// CheckCardGroup

var a2Opts = []CheckCardOption{{Value: "a", Title: "Alpha"}, {Value: "b", Title: "Beta"}, {Value: "c"}}

// TestCheckCardGroupUncontrolled: useControlledState(defaultValue, value)
// with multiple (pro/CheckCard/Group.js).
func TestCheckCardGroupUncontrolled(t *testing.T) {
	var single, multi *CheckCardGroupW
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			single = CheckCardGroup(c, a2Opts).DefaultValue("b").Show()
			Key(c, "m", func() {
				multi = CheckCardGroup(c, []CheckCardOption{{Value: "x", Title: "Xray"}, {Value: "y", Title: "Yank"}, {Value: "z", Title: "Zulu"}}).
					Multiple().DefaultValues("z").Show()
			})
		}, layout.Pad(10))
	}, DriverOptions{Width: 1100, Height: 600, Accessibility: true})
	d.Settle()
	if single.Selected() != "b" {
		t.Fatalf("default value %q", single.Selected())
	}
	if n, ok := d.A11yFind(a11y.RoleRadio, "Beta"); !ok || n.States&a11y.Checked == 0 {
		t.Fatal("default value not checked")
	}
	d.ClickLabel("Alpha")
	d.Settle()
	if single.Selected() != "a" {
		t.Fatalf("after click %q", single.Selected())
	}
	d.ClickLabel("Alpha")
	d.Settle()
	if single.Selected() != "" {
		t.Fatalf("re-click should clear, got %q", single.Selected())
	}
	d.ClickLabel("Xray")
	d.Settle()
	got := multi.SelectedValues()
	if len(got) != 2 || got[0] != "x" || got[1] != "z" {
		t.Fatalf("multiple values %v, want [x z] in option order", got)
	}
	if _, ok := d.A11yFind(a11y.RoleCheckbox, "Yank"); !ok {
		t.Fatal("multiple group cards are not checkboxes")
	}
}

// TestCheckCardGroupTitleFallback: a string option is {title: s, value: s}.
func TestCheckCardGroupTitleFallback(t *testing.T) {
	_, d := a2Ant(1100, 400, func(c *Context) {
		CheckCardGroup(c, a2Opts).Show()
	})
	pro2MustFind(t, d, "c")
}

// TestCheckCardGroupLoading: a loading group shows every card loading,
// unchecked and not clickable.
func TestCheckCardGroupLoading(t *testing.T) {
	v := "a"
	d := NewDriver(func(c *Context) {
		Column(c, func() { CheckCardGroup(c, a2Opts).Value(&v).Loading(true).Show() }, layout.Pad(10))
	}, DriverOptions{Width: 1100, Height: 400, Accessibility: true})
	d.Settle()
	if n, ok := d.A11yFind(a11y.RoleRadio, "Alpha"); !ok || n.States&a11y.Checked != 0 {
		t.Fatalf("loading card shown checked: %v", n)
	}
	d.Click(a2Card(t, d, "Beta").Center())
	d.Settle()
	if v != "a" {
		t.Fatalf("loading group changed to %q", v)
	}
}

// TestCheckCardGroupBorderless: group bordered=false passes to the cards.
func TestCheckCardGroupBorderless(t *testing.T) {
	th, d := a2Ant(1100, 400, func(c *Context) {
		CheckCardGroup(c, a2Opts).Borderless().Show()
	})
	card := a2Card(t, d, "Alpha")
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.StrokeRect); ok && o.Rect == card && o.Color == th.Border {
			t.Fatal("borderless group card still bordered")
		}
	}
}

// TestCheckCardGroupChildren: an option with children renders a
// collapsible sub group (SubCheckCardGroup): title row, then the cards;
// clicking the title folds them.
func TestCheckCardGroupChildren(t *testing.T) {
	v := ""
	opts := []CheckCardOption{{Value: "g", Title: "Group", Children: []CheckCardOption{{Value: "k1", Title: "Kid1"}, {Value: "k2", Title: "Kid2"}}}}
	d := NewDriver(func(c *Context) {
		Column(c, func() { CheckCardGroup(c, opts).Value(&v).Show() }, layout.Pad(10))
	}, DriverOptions{Width: 1100, Height: 500, Accessibility: true})
	d.Settle()
	head := pro2MustFind(t, d, "Group")
	kid := a2Card(t, d, "Kid1")
	if kid.Y <= head.Bottom() {
		t.Fatal("children not below the sub group title")
	}
	if n, ok := d.A11yFind(a11y.RoleButton, "Group"); !ok || n.States&a11y.Expanded == 0 {
		t.Fatalf("sub group title: %v", n)
	}
	d.ClickLabel("Kid2")
	d.Settle()
	if v != "k2" {
		t.Fatalf("child selection %q", v)
	}
	d.Click(head.Center())
	d.Settle()
	if r, ok := d.Find("Kid1"); ok && !r.Empty() {
		t.Fatal("collapsed sub group still shows its cards")
	}
}

// TestCheckCardGroupBody: CheckCards built inside Body follow the group
// by Value (CheckCardGroupContext).
func TestCheckCardGroupBody(t *testing.T) {
	v := "p"
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			CheckCardGroup(c, nil).Value(&v).Body(func() {
				CheckCard(c, "Pear", nil).Value("p").Description("fruit").Show()
				CheckCard(c, "Quince", nil).Value("q").Extra(func() { Text(c, "x") }).Show()
			})
		}, layout.Pad(10))
	}, DriverOptions{Width: 1100, Height: 400, Accessibility: true})
	d.Settle()
	if n, ok := d.A11yFind(a11y.RoleRadio, "Pear"); !ok || n.States&a11y.Checked == 0 {
		t.Fatalf("body card not checked from the group: %v", n)
	}
	d.ClickLabel("Quince")
	d.Settle()
	if v != "q" {
		t.Fatalf("group value %q", v)
	}
	if _, ok := d.A11yFind(a11y.RoleRadioGroup, ""); !ok {
		t.Fatal("no radio group")
	}
}

// ---------------------------------------------------------------------------
// Themes

// TestAlignB4A2Themes builds every new piece in the six built-in themes.
func TestAlignB4A2Themes(t *testing.T) {
	for _, th := range theme.BuiltinThemes() {
		v := "a"
		d := NewDriver(func(c *Context) {
			Column(c, func() {
				StatisticCardGroup(c).Body(func() {
					StatisticCard(c, "One").ValueText("1").Trend("up").Desc("W", "1%", "down").Show()
					StatisticCard(c, "Two").ValueText("2").StatisticLayout(StatisticHorizontal).Show()
				})
				ProStatistic(c, "Three").ValueText("3").Show()
				StatisticCardOperation(c, "+")
				StatisticCardDivider(c, "horizontal")
				CheckCardGroup(c, a2Opts).Value(&v).Show()
				CheckCard(c, "Solo", nil).SubTitle("s").DefaultChecked(true).Show()
			}, layout.Pad(10), layout.Gap(8))
		}, DriverOptions{Width: 1100, Height: 900, Theme: th, Accessibility: true})
		d.Settle()
		for _, s := range []string{"One", "Two", "Three", "+", "Alpha", "Solo"} {
			pro2MustFind(t, d, s)
		}
		if _, ok := d.A11yFind(a11y.RoleCheckbox, "Solo"); !ok {
			t.Fatalf("%v dark=%v: no checkbox", th.Flavor, th.Dark)
		}
	}
}

// Group is StatisticCard with body padding 0, not an implicit split
// (pro/StatisticCard/index.js `Group`).
func TestStatisticCardGroupDefaults(t *testing.T) {
	var group *ProCardW
	_, d := a2Ant(700, 300, func(c *Context) {
		group = StatisticCardGroup(c)
		group.Body(func() { StatisticCard(c, "Child").ValueText("1").Show() })
	})
	if group.split != "" || group.bodyPad == nil || *group.bodyPad != (geom.Insets{}) {
		t.Fatalf("Group must only default to zero body padding, split=%q padding=%v", group.split, group.bodyPad)
	}
	pro2MustFind(t, d, "Child")
}

func TestCheckCardGroupDefaults(t *testing.T) {
	var g *CheckCardGroupW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("CheckCardGroup", Props{"multiple": true, "defaultValue": []string{"b"}, "bordered": false, "size": "small"}).Body(func() { g = CheckCardGroup(c, a2Opts).Show() })
	}, DriverOptions{Width: 900, Height: 300, Accessibility: true})
	d.Settle()
	if vs := g.SelectedValues(); len(vs) != 1 || vs[0] != "b" {
		t.Fatalf("defaults values %v", vs)
	}
	if n, ok := d.A11yFind(a11y.RoleCheckbox, "Beta"); !ok || n.States&a11y.Checked == 0 {
		t.Fatal("multiple/defaultValue defaults not applied")
	}
	if r := a2Card(t, d, "Alpha"); r.W != 212 {
		t.Fatalf("size default width %v", r.W)
	}
}

func TestCheckCardGroupBodyMultiple(t *testing.T) {
	var g *CheckCardGroupW
	_, d := a2Ant(1100, 300, func(c *Context) {
		g = CheckCardGroup(c, nil).Multiple().DefaultValues("z").Body(func() {
			CheckCard(c, "BodyA", nil).Value("a").Show()
			CheckCard(c, "BodyZ", nil).Value("z").Show()
		})
	})
	d.ClickLabel("BodyA")
	d.Settle()
	if vs := g.SelectedValues(); len(vs) != 2 || vs[0] != "a" || vs[1] != "z" {
		t.Fatalf("body multiple selection %v", vs)
	}
	d.ClickLabel("BodyZ")
	d.Settle()
	if vs := g.SelectedValues(); len(vs) != 1 || vs[0] != "a" {
		t.Fatalf("body multiple deselection %v", vs)
	}
}

func TestStatisticCardStatusDot(t *testing.T) {
	th, d := a2Ant(500, 300, func(c *Context) { StatisticCard(c, "S").Status("success").ValueText("1").Show() })
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.FillRect); ok && o.Color == th.Success.Base {
			if o.Rect.W != 6 || o.Rect.H != 6 {
				t.Fatalf("status dot %v, want 6x6", o.Rect)
			}
			return
		}
	}
	t.Fatal("no status dot")
}

func TestCheckCardLoadingBackground(t *testing.T) {
	th, d := a2Ant(500, 300, func(c *Context) { CheckCard(c, "Busy", nil).Loading(true).Show() })
	card := a2Card(t, d, "Busy")
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.FillRect); ok && o.Rect == card {
			if o.Color != th.BgContainer {
				t.Fatalf("loading background %v, want container (not disabled)", o.Color)
			}
			return
		}
	}
	t.Fatal("no card background")
}

func TestStatisticCardDefaultValue(t *testing.T) {
	_, d := a2Ant(500, 300, func(c *Context) { StatisticCard(c, "Default").Show() })
	pro2MustFind(t, d, "0")
}

// Review: Middle is an explicit size, not absence; group size wins over
// the global ControlSize when the card did not explicitly choose one.
func TestCheckCardGroupSizePresence(t *testing.T) {
	_, d := a2Ant(1200, 400, func(c *Context) {
		ConfigProvider(c).Size(Small).Body(func() {
			CheckCardGroup(c, nil).Size(Large).Body(func() {
				CheckCard(c, "Inherit", nil).Value("i").Show()
				CheckCard(c, "Explicit", nil).Value("e").Size(Middle).Show()
			})
		})
	})
	if r := a2Card(t, d, "Inherit"); r.W != 440 {
		t.Fatalf("group should override global size, got %v", r.W)
	}
	if r := a2Card(t, d, "Explicit"); r.W != 320 {
		t.Fatalf("explicit Middle should win, got %v", r.W)
	}
}

func TestCheckCardSubgroupFocusAndExpand(t *testing.T) {
	th := theme.AntDesign()
	opts := []CheckCardOption{{Value: "g", Title: "Fold", Children: []CheckCardOption{{Value: "a", Title: "Child"}}}}
	d := NewDriver(func(c *Context) { CheckCardGroup(c, opts).Show() }, DriverOptions{Width: 900, Height: 400, Theme: th, Accessibility: true})
	d.Settle()
	d.Key(input.KeyTab, 0)
	d.Settle()
	n, ok := d.A11yFind(a11y.RoleButton, "Fold")
	if !ok || n.States&a11y.Focused == 0 {
		t.Fatalf("subgroup title not focused: %v", n)
	}
	focusRing := false
	for _, op := range d.r.List().Ops {
		if o, ok := op.(paint.StrokeRect); ok && o.Color == th.FocusOutline && o.Width == th.FocusRingWidth {
			focusRing = true
		}
	}
	if !focusRing {
		t.Fatal("no visible focus ring on subgroup title")
	}
	d.Key(input.KeyEnter, 0)
	d.Settle()
	n, ok = d.A11yFind(a11y.RoleButton, "Fold")
	if !ok || n.States&a11y.Expandable == 0 || n.States&a11y.Expanded != 0 {
		t.Fatalf("collapsed title not expandable: %v", n)
	}
	if err := d.A11yPerform(a11y.RoleButton, "Fold", a11y.Expand, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	pro2MustFind(t, d, "Child")
}

func TestCheckCardOptionMiddleSize(t *testing.T) {
	_, d := a2Ant(1100, 300, func(c *Context) {
		CheckCardGroup(c, []CheckCardOption{{Value: "i", Title: "Inherited"}, {Value: "e", Title: "ExplicitOpt", Size: Middle, SizeSet: true}}).Size(Large).Show()
	})
	if r := a2Card(t, d, "Inherited"); r.W != 440 {
		t.Fatalf("inherit option width %v", r.W)
	}
	if r := a2Card(t, d, "ExplicitOpt"); r.W != 320 {
		t.Fatalf("explicit Middle option width %v", r.W)
	}
}

// Cover only replaces meta, not React children/actions, which are siblings
// of metaDom in pro/CheckCard/index.js.
func TestCheckCardCoverKeepsBodyActions(t *testing.T) {
	_, d := a2Ant(600, 400, func(c *Context) {
		CheckCard(c, "Cover", nil).Cover(func() { Text(c, "Picture") }).Content(func() { Text(c, "BodyAfterCover") }).Actions(func() { Text(c, "CoverAction") }).Show()
	})
	pro2MustFind(t, d, "BodyAfterCover")
	pro2MustFind(t, d, "CoverAction")
}
