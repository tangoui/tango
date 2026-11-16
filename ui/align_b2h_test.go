package ui

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Batch 2, group H: Tabs, Timeline, Image, Carousel, Calendar, QRCode,
// Watermark, Masonry (docs/specs/align/2-media.md).

func b2hTabs() []TabItem {
	return []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}, {Key: "c", Label: "Gamma"}}
}

func b2hCenter(r geom.Rect) geom.Point { return geom.Pt(r.X+r.W/2, r.Y+r.H/2) }

// ---------------------------------------------------------------------------
// Tabs

// TestTabsTabPlacementStart: TabPlacement("start") and Defaults
// "tabPlacement" put a vertical bar at the start (Ant v6 tabPlacement).
func TestTabsTabPlacementStart(t *testing.T) {
	act := "a"
	var tw, tw2 *TabsW
	d := NewDriver(func(c *Context) {
		tw = Tabs(c, &act, b2hTabs()).TabPlacement("start").Show()
		ConfigProvider(c).Defaults("Tabs", Props{"tabPlacement": "end"}).Body(func() {
			tw2 = Tabs(c, &act, b2hTabs()).Show()
		})
	}, DriverOptions{Width: 600, Height: 400})
	d.EnableAccessibility()
	d.Frame()
	if tw.place != Left || tw2.place != Right {
		t.Fatalf("placement %v %v", tw.place, tw2.place)
	}
	lists := d.A11yFindAll(a11y.RoleTabList, "")
	if len(lists) != 2 || !lists[0].States.Has(a11y.Vertical) {
		t.Fatalf("tab lists %v", lists)
	}
}

// TestTabsIndicator: Indicator(size, align) places the ink bar inside the
// tab (rc-tabs useIndicator: start / center / end).
func TestTabsIndicator(t *testing.T) {
	w := &TabsW{}
	for _, tc := range []struct {
		align   string
		off, ln float32
	}{{"start", 10, 20}, {"", 50, 20}, {"center", 50, 20}, {"end", 90, 20}} {
		w.Indicator(20, tc.align)
		if off, n := w.indicator(10, 100); off != tc.off || n != tc.ln {
			t.Errorf("%q: %v %v", tc.align, off, n)
		}
	}
	w.Indicator(0, "start")
	if off, n := w.indicator(10, 100); off != 10 || n != 100 {
		t.Errorf("size 0 keeps the tab: %v %v", off, n)
	}
}

// TestTabsEmptyUnknownKey: empty items and a nil key do not panic; an
// active key not in the items falls back to the tab at the last active
// index (rc-tabs Tabs.js), without reporting Changed.
func TestTabsEmptyUnknownKey(t *testing.T) {
	items := b2hTabs()
	act := "zz"
	var tw *TabsW
	d := NewDriver(func(c *Context) {
		Tabs(c, nil, nil).Show()
		Tabs(c, nil, b2hTabs()).Show()
		e := ""
		Tabs(c, &e, []TabItem{}).Show().Panel(func() {})
		tw = Tabs(c, &act, items).Show()
	}, DriverOptions{Width: 600, Height: 400})
	d.Frame()
	if act != "a" || tw.Changed() {
		t.Fatalf("unknown key: %q changed=%v", act, tw.Changed())
	}
	if err := d.ClickLabel("Gamma"); err != nil {
		t.Fatal(err)
	}
	if act != "c" {
		t.Fatalf("click: %q", act)
	}
	items = items[:2] // the active tab went away: the nearest index wins
	d.Frame()
	if act != "b" {
		t.Fatalf("fallback after removal: %q", act)
	}
	items = append([]TabItem{}, TabItem{Key: "x", Label: "X", Disabled: true}, TabItem{Key: "y", Label: "Y"})
	act = "gone"
	d.Frame()
	if act != "y" {
		t.Fatalf("fallback skips disabled tabs: %q", act)
	}
}

// TestTabsBeforeLeave: BeforeLeave can veto a change (Element
// before-leave); TabClicked reports clicks, also on the active tab.
func TestTabsBeforeLeave(t *testing.T) {
	act := "a"
	allow := false
	var asked [2]string
	var clicked string
	d := NewDriver(func(c *Context) {
		tw := Tabs(c, &act, b2hTabs()).BeforeLeave(func(n, o string) bool { asked = [2]string{n, o}; return allow }).Show()
		if k := tw.TabClicked(); k != "" {
			clicked = k
		}
	}, DriverOptions{Width: 600, Height: 300})
	d.Frame()
	_ = d.ClickLabel("Beta")
	if act != "a" || asked != [2]string{"b", "a"} || clicked != "b" {
		t.Fatalf("veto: act=%q asked=%v clicked=%q", act, asked, clicked)
	}
	allow = true
	_ = d.ClickLabel("Beta")
	if act != "b" {
		t.Fatalf("allowed: %q", act)
	}
	clicked = ""
	_ = d.ClickLabel("Beta")
	if clicked != "b" {
		t.Fatalf("click on the active tab: %q", clicked)
	}
}

// TestTabsStretchGutter: Stretch gives the tabs equal shares of the bar
// (Element stretch); TabBarGutter sets the space between tabs.
func TestTabsStretchGutter(t *testing.T) {
	act := "a"
	stretch := true
	d := NewDriver(func(c *Context) {
		w := Tabs(c, &act, b2hTabs())
		if stretch {
			w.Stretch()
		} else {
			w.TabBarGutter(10)
		}
		w.Show()
	}, DriverOptions{Width: 600, Height: 300})
	d.Settle()
	a, _ := d.Find("Alpha")
	b, _ := d.Find("Beta")
	g, _ := d.Find("Gamma")
	if a.W < 150 || abs32(a.W-b.W) > 1 || abs32(b.W-g.W) > 1 {
		t.Fatalf("stretch widths %v %v %v", a.W, b.W, g.W)
	}
	stretch = false
	d.Settle()
	a, _ = d.Find("Alpha")
	b, _ = d.Find("Beta")
	if gap := b.X - a.Right(); abs32(gap-10) > 0.5 {
		t.Fatalf("gutter %v", gap)
	}
}

// TestTabsSizes: Ant size tokens (cardHeightSM / LG, horizontalItemPadding
// SM / LG).
func TestTabsSizes(t *testing.T) {
	act := "a"
	var size ControlSize
	card := false
	d := NewDriver(func(c *Context) {
		w := Tabs(c, &act, b2hTabs()).Size(size)
		if card {
			w.Card()
		}
		w.Show()
	}, DriverOptions{Width: 600, Height: 300})
	check := func(want float32) {
		t.Helper()
		d.Settle()
		r, _ := d.Find("Alpha")
		if abs32(r.H-want) > 0.5 {
			t.Errorf("size %v card %v: height %v, want %v", size, card, r.H, want)
		}
	}
	size = Large
	check(24 + 2*16) // fontHeightLG + 2*horizontalItemPaddingLG
	size = Small
	check(22 + 2*8)
	card = true
	check(32) // cardHeightSM
	size = Large
	check(48) // cardHeightLG
}

// TestTabsDisabledScope: inside a disabled scope the tabs, the close and
// the add buttons do not respond.
func TestTabsDisabledScope(t *testing.T) {
	act := "a"
	var tw *TabsW
	added := false
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() {
			tw = Tabs(c, &act, b2hTabs()).Closable().OnAdd(func() { added = true }).Show()
		})
	}, DriverOptions{Width: 600, Height: 300})
	d.Frame()
	_ = d.ClickLabel("Beta")
	_ = d.ClickLabel(ZhCN.RemoveItem("Gamma"))
	_ = d.ClickLabel(ZhCN.Tabs.Add)
	if act != "a" || tw.Closed() != "" || tw.Added() || added || tw.TabClicked() != "" {
		t.Fatalf("disabled scope responded: act=%q closed=%q added=%v", act, tw.Closed(), added)
	}
}

// TestTabsA11yKeys: tab / tablist / tabpanel roles; arrows, Home and End
// select tabs (WAI-ARIA tabs, rc-tabs TabNavList onKeyDown).
func TestTabsA11yKeys(t *testing.T) {
	act := "a"
	d := NewDriver(func(c *Context) {
		Tabs(c, &act, b2hTabs()).Show().Panel(func() { Text(c, "panel "+act) })
	}, DriverOptions{Width: 600, Height: 300})
	d.EnableAccessibility()
	d.Frame()
	tab := mustNode(t, d, a11y.RoleTab, "Alpha")
	if !tab.States.Has(a11y.Selected) {
		t.Fatalf("active tab %s", tab)
	}
	mustNode(t, d, a11y.RoleTabPanel, "Alpha")
	if err := d.PerformAction(tab.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	d.Key(input.KeyEnd, 0)
	if act != "c" {
		t.Fatalf("End: %q", act)
	}
	d.Key(input.KeyHome, 0)
	if act != "a" {
		t.Fatalf("Home: %q", act)
	}
	d.Key(input.KeyRight, 0)
	if act != "b" {
		t.Fatalf("Right: %q", act)
	}
}

// TestTabsShadcnLine: the shadcn line variant draws no segmented pill and
// keeps the tabs working; the dark active trigger uses input/30 with an
// input border (tabs.tsx).
func TestTabsShadcnLine(t *testing.T) {
	act := "a"
	d := NewDriver(func(c *Context) {
		Tabs(c, &act, b2hTabs()).Variant("line").Show()
		Tabs(c, &act, b2hTabs()).Placement(Left).Variant("line").Show()
	}, DriverOptions{Width: 600, Height: 400})
	d.SetTheme(theme.Shadcn())
	d.Frame()
	_ = d.ClickLabel("Beta")
	if act != "b" {
		t.Fatalf("line variant click: %q", act)
	}
	dk := theme.Shadcn(theme.Dark()).Components.Tabs
	if dk.ItemSelectedBorder.A == 0 || dk.ItemSelectedBg == theme.Shadcn().Components.Tabs.ItemSelectedBg {
		t.Fatalf("dark active trigger %v %v", dk.ItemSelectedBg, dk.ItemSelectedBorder)
	}
}

// ---------------------------------------------------------------------------
// Timeline

func b2hTimelineX(t *testing.T, d *Driver, s string) float32 {
	t.Helper()
	r, ok := d.FindText(s)
	if !ok {
		t.Fatalf("no %q", s)
	}
	return r.X
}

// TestTimelineModeStartEnd: the default mode is "start" (content after the
// axis); Placement("end") on an item and Mode("end") put it before.
func TestTimelineModeStartEnd(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Timeline(c, []TimelineItem{{Content: "one"}, {Content: "two", Placement: "end"}}).Show()
		}, layout.W(400))
	}, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	if x1, x2 := b2hTimelineX(t, d, "one"), b2hTimelineX(t, d, "two"); x1 < 20 || x2 >= x1 {
		t.Fatalf("start / end: %v %v", x1, x2)
	}
}

// TestTimelineAlternateReverse: Element mode alternate-reverse puts the
// first item before the axis.
func TestTimelineAlternateReverse(t *testing.T) {
	mode := "alternate"
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Timeline(c, []TimelineItem{{Content: "one"}, {Content: "two"}}).Mode(mode).Show()
		}, layout.W(400))
	}, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	a1, a2 := b2hTimelineX(t, d, "one"), b2hTimelineX(t, d, "two")
	mode = "alternate-reverse"
	d.Frame()
	r1, r2 := b2hTimelineX(t, d, "one"), b2hTimelineX(t, d, "two")
	if !(a1 > 200 && a2 < 200 && r1 < 200 && r2 > 200) {
		t.Fatalf("alternate %v %v, reverse %v %v", a1, a2, r1, r2)
	}
}

// TestTimelineTitleSpan: TitleSpan(6) gives the titles a quarter of the
// width (Ant titleSpan); Title is the v6 name of Label.
func TestTimelineTitleSpan(t *testing.T) {
	span := float32(0)
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			Timeline(c, []TimelineItem{{Title: "2026-01", Content: "body"}}).TitleSpan(span).Show()
		}, layout.W(480))
	}, DriverOptions{Width: 480, Height: 300})
	d.Frame()
	mid := b2hTimelineX(t, d, "body")
	span = 6
	d.Frame()
	quarter := b2hTimelineX(t, d, "body")
	if mid < 200 || quarter > mid-80 {
		t.Fatalf("title span: centred %v, 6/24 %v", mid, quarter)
	}
}

// TestTimelineLoadingOrientation: a Loading item is busy (spinner dot);
// Orientation("horizontal") lays the list out horizontally; empty items
// do not panic.
func TestTimelineLoadingOrientation(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Timeline(c, nil).Show()
		Timeline(c, []TimelineItem{{Content: "done"}, {Content: "running", Loading: true, Center: true}}).Variant("filled").Show()
		Timeline(c, []TimelineItem{{Content: "h1"}, {Content: "h2"}}).Orientation("horizontal").Show()
	}, DriverOptions{Width: 500, Height: 500})
	d.EnableAccessibility()
	d.Frame()
	busy := 0
	for _, n := range d.A11yFindAll(a11y.RoleListItem, "") {
		if n.States.Has(a11y.Busy) {
			busy++
		}
	}
	if busy != 1 {
		t.Fatalf("busy items %d", busy)
	}
	horiz := 0
	for _, n := range d.A11yFindAll(a11y.RoleList, "") {
		if n.States.Has(a11y.Horizontal) {
			horiz++
		}
	}
	if horiz != 1 {
		t.Fatalf("horizontal lists %d", horiz)
	}
}

// ---------------------------------------------------------------------------
// Carousel

func b2hSlide(c *Context) func(int) {
	return func(i int) { Text(c, "slide") }
}

// TestCarouselOutOfRange: Current outside [0, n) is clamped; n = 0 does
// not panic.
func TestCarouselOutOfRange(t *testing.T) {
	cur := 99
	neg := -3
	none := 5
	d := NewDriver(func(c *Context) {
		Carousel(c, 3, b2hSlide(c)).Current(&cur).Show()
		Carousel(c, 3, b2hSlide(c)).Current(&neg).Show()
		Carousel(c, 0, b2hSlide(c)).Current(&none).Arrows("always").Show()
	}, DriverOptions{Width: 400, Height: 600})
	d.Frame()
	if cur != 2 || neg != 0 {
		t.Fatalf("clamp: %d %d", cur, neg)
	}
}

// TestCarouselDotPlacement: DotPlacement("start") stacks the indicators
// vertically and slides vertically (Ant v6: start / end imply vertical).
func TestCarouselDotPlacement(t *testing.T) {
	var w *CarouselW
	d := NewDriver(func(c *Context) {
		w = Carousel(c, 3, b2hSlide(c)).DotPlacement("start")
		w.Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.EnableAccessibility()
	d.Frame()
	if !w.vertical || w.dotPos != "left" {
		t.Fatalf("vertical %v pos %q", w.vertical, w.dotPos)
	}
	l := d.A11yFindAll(a11y.RoleTabList, "")
	if len(l) != 1 || !l[0].States.Has(a11y.Vertical) {
		t.Fatalf("indicators %v", l)
	}
}

// TestCarouselDisabledArrows: arrows and indicators do not respond in a
// disabled scope.
func TestCarouselDisabledArrows(t *testing.T) {
	cur := 0
	d := NewDriver(func(c *Context) {
		Disabled(c, true, func() {
			Carousel(c, 3, b2hSlide(c)).Current(&cur).Arrows("always").Show()
		})
	}, DriverOptions{Width: 400, Height: 300})
	d.Frame()
	_ = d.ClickLabel(ZhCN.Carousel.Next)
	d.Frame()
	if cur != 0 {
		t.Fatalf("disabled arrow moved to %d", cur)
	}
}

// TestCarouselHoverTriggerPause: Trigger("hover") selects a slide when its
// dot is hovered (Element trigger); PauseOnHover(false) keeps autoplay
// running under the pointer.
func TestCarouselHoverTriggerPause(t *testing.T) {
	cur := 0
	pause := true
	d := NewDriver(func(c *Context) {
		w := Carousel(c, 3, b2hSlide(c)).Current(&cur).Trigger("hover")
		if !pause {
			w.Autoplay(time.Second).PauseOnHover(false)
		}
		w.Show()
	}, DriverOptions{Width: 400, Height: 300})
	d.EnableAccessibility()
	d.Frame()
	dot := mustNode(t, d, a11y.RoleTab, ZhCN.Slide(3, 3))
	r := dot.Bounds
	d.Move(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
	d.Frame()
	if cur != 2 {
		t.Fatalf("hover trigger: %d", cur)
	}
	pause = false
	d.Move(geom.Pt(200, 60)) // over the frame
	d.Frame()
	d.Advance(1100 * time.Millisecond)
	d.Frame()
	d.Frame()
	if cur != 0 {
		t.Fatalf("autoplay paused under the pointer: %d", cur)
	}
}

// ---------------------------------------------------------------------------
// Image

func b2hImg() image.Image {
	m := image.NewRGBA(image.Rect(0, 0, 60, 40))
	for i := range m.Pix {
		m.Pix[i] = 0x80
	}
	m.Set(0, 0, color.RGBA{255, 0, 0, 255})
	return m
}

// TestImagePreviewScaleBounds: zoom steps by 1+ScaleStep between MinScale
// and MaxScale (rc-image scaleStep / minScale / maxScale; theme defaults
// Ant 0.5 / 1 / 50, Element 0.2 / 0.2 / 7).
func TestImagePreviewScaleBounds(t *testing.T) {
	src := b2hImg()
	var w *ImageW
	d := NewDriver(func(c *Context) {
		w = Image(c, src).Width(60).Alt("pic").Preview(true).MaxScale(2)
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	_ = d.ClickLabel("pic")
	lo, hi, step := w.scales()
	if lo != 1 || hi != 2 || step != .5 {
		t.Fatalf("ant scales %v %v %v", lo, hi, step)
	}
	_ = d.ClickLabel(ZhCN.Image.ZoomIn)
	if w.PreviewScale() != 1.5 {
		t.Fatalf("zoom in %v", w.PreviewScale())
	}
	_ = d.ClickLabel(ZhCN.Image.ZoomIn)
	_ = d.ClickLabel(ZhCN.Image.ZoomIn)
	if w.PreviewScale() != 2 {
		t.Fatalf("max %v", w.PreviewScale())
	}
	d.SetTheme(theme.ElementPlus())
	d.Frame()
	if lo, hi, step := w.scales(); lo != .2 || hi != 2 || step != .2 {
		t.Fatalf("element scales %v %v %v", lo, hi, step)
	}
}

// TestImagePreviewOutOfRangeInfinite: a group index out of range is
// clamped; Ant stops at the last image (buttons and arrow keys), Element
// wraps (infinite); MaskClosable(false) and CloseOnEscape(false) keep the
// preview open.
func TestImagePreviewOutOfRangeInfinite(t *testing.T) {
	src := b2hImg()
	imgs := []image.Image{src, src, src}
	var w *ImageW
	d := NewDriver(func(c *Context) {
		w = Image(c, src).Width(60).Alt("pic").PreviewGroup(imgs, 9).MaskClosable(false).CloseOnEscape(false)
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	if w.groupIdx != 2 {
		t.Fatalf("index clamp %d", w.groupIdx)
	}
	_ = d.ClickLabel("pic")
	d.Key(input.KeyRight, 0)
	if w.st.idx != 2 {
		t.Fatalf("ant arrow past the end: %d", w.st.idx)
	}
	d.Click(geom.Pt(400, 120)) // backdrop
	d.Key(input.KeyEscape, 0)
	if !w.PreviewOpen() {
		t.Fatal("closed despite MaskClosable(false) / CloseOnEscape(false)")
	}
	w.Infinite(true)
	d.SetTheme(theme.ElementPlus())
	d.Frame()
	d.Key(input.KeyRight, 0)
	if w.st.idx != 0 {
		t.Fatalf("element wraps: %d", w.st.idx)
	}
}

// TestImagePreviewDisabled: a disabled scope does not open the preview;
// OpenPreview opens it from code and PreviewSrc replaces the image.
func TestImagePreviewDisabled(t *testing.T) {
	src, big := b2hImg(), image.NewRGBA(image.Rect(0, 0, 300, 200))
	var w, w2 *ImageW
	open := false
	d := NewDriver(func(c *Context) {
		Disabled(c, true, func() { w = Image(c, src).Width(60).Alt("off").Preview(true) })
		w2 = Image(c, src).Width(60).Alt("on").PreviewSrc(big)
		if open {
			w2.OpenPreview()
			open = false
		}
	}, DriverOptions{Width: 800, Height: 600})
	d.Frame()
	_ = d.ClickLabel("off")
	if w.PreviewOpen() {
		t.Fatal("disabled image opened its preview")
	}
	open = true
	d.Frame()
	d.Frame()
	if !w2.PreviewOpen() || w2.previewImages()[0] != image.Image(big) {
		t.Fatal("OpenPreview / PreviewSrc")
	}
}

// ---------------------------------------------------------------------------
// Calendar

// TestCalendarValidRange: days outside ValidRange cannot be picked and the
// year select lists only the range's years (Ant validRange).
func TestCalendarValidRange(t *testing.T) {
	val := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	lo, hi := time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC), time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC)
	var w *CalendarW
	d := NewDriver(func(c *Context) {
		w = Calendar(c, &val).Card().ValidRange(lo, hi)
		w.Changed()
	}, DriverOptions{Width: 500, Height: 600})
	d.Frame()
	_ = d.ClickLabel("2026-03-05")
	if val.Day() != 15 {
		t.Fatalf("picked outside the range: %v", val)
	}
	_ = d.ClickLabel("2026-03-12")
	if val.Day() != 12 {
		t.Fatalf("inside the range: %v", val)
	}
	if !w.validMonth(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)) || w.validMonth(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("validMonth")
	}
}

// TestCalendarShowWeekSelect: ShowWeek adds ISO week row headers;
// Element ControllerType("select") shows year / month selects; a nil
// value does not panic.
func TestCalendarShowWeekSelect(t *testing.T) {
	val := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	d := NewDriver(func(c *Context) {
		Calendar(c, &val).Card().ShowWeek(true)
		Calendar(c, nil).ControllerType("select").Formatter(func(v int, kind string) string { return "" })
	}, DriverOptions{Width: 900, Height: 1400})
	d.EnableAccessibility()
	d.SetTheme(theme.ElementPlus())
	d.Frame()
	if n := len(d.A11yFindAll(a11y.RoleRowHeader, "")); n != 6 {
		t.Fatalf("week headers %d", n)
	}
	if n := len(d.A11yFindAll(a11y.RoleCombobox, ZhCN.Calendar.Month)); n != 1 {
		t.Fatalf("month selects %d", n)
	}
}

// ---------------------------------------------------------------------------
// QRCode, Watermark, Masonry

// TestQRCodeBoostEmpty: the level is boosted while the version stays
// (Ant boostLevel, default true); an empty value encodes nothing.
func TestQRCodeBoostEmpty(t *testing.T) {
	var q1, q2, q3 *QRCodeW
	d := NewDriver(func(c *Context) {
		q1 = QRCode(c, "hi").Level("L")
		q2 = QRCode(c, "hi").Level("L").BoostLevel(false).MarginSize(2)
		q3 = QRCode(c, "")
	}, DriverOptions{Width: 600, Height: 600})
	d.Frame()
	m1, m2 := q1.Matrix(), q2.Matrix()
	if m1.Level != QRLevelH || m2.Level != QRLevelL || m1.Version != m2.Version {
		t.Fatalf("boost: %v v%d, plain %v v%d", m1.Level, m1.Version, m2.Level, m2.Version)
	}
	if q3.Matrix() != nil {
		t.Fatal("empty value encoded")
	}
}

// TestWatermarkMarkSize: the mark is the widest line by the line heights
// plus 3px between lines (antd getMarkSize); image marks default to
// 120×64; MarkSize overrides.
func TestWatermarkMarkSize(t *testing.T) {
	var w1, w2, w3 *WatermarkW
	d := NewDriver(func(c *Context) {
		w1 = Watermark(c, "Tango", "UI").Rotate(0)
		w1.Body(func() { Div(c) })
		w2 = Watermark(c).Image(b2hImg(), 0, 0).Offset(130, 0)
		w2.Body(func() { Div(c) })
		w3 = Watermark(c, "x").MarkSize(200, 50).FontStyle("italic")
		w3.Body(func() { Div(c) })
	}, DriverOptions{Width: 400, Height: 400})
	d.Frame()
	lays, mw, mh := w1.markSize()
	if len(lays) != 2 || mh != float32(int(lays[0].Height+lays[1].Height+3+0.999)) || mw < lays[0].Width {
		t.Fatalf("text mark %v×%v", mw, mh)
	}
	if _, mw, mh := w2.markSize(); mw != 120 || mh != 64 {
		t.Fatalf("image mark %v×%v", mw, mh)
	}
	if _, mw, mh := w3.markSize(); mw != 200 || mh != 50 {
		t.Fatalf("MarkSize %v×%v", mw, mh)
	}
}

// TestMasonryLayoutChangedEmpty: LayoutChanged reports a new column
// assignment (Ant onLayoutChange); zero items do not panic.
func TestMasonryLayoutChangedEmpty(t *testing.T) {
	n, cols := 6, 3
	var w *MasonryW
	changes := 0
	d := NewDriver(func(c *Context) {
		Masonry(c, 0, func(int) {}).Show()
		w = Masonry(c, n, func(i int) { Div(c).Style(layout.H(float32(20 + 10*i))) }).Columns(cols).ItemHeight(func(i int) float32 { return float32(20 + 10*i) }).Show()
		if w.LayoutChanged() {
			changes++
		}
	}, DriverOptions{Width: 600, Height: 600})
	d.Frame()
	d.Frame()
	first := changes
	cols = 2
	d.Frame()
	if first != 1 || changes != 2 || len(w.Assignment()) != n {
		t.Fatalf("changes %d then %d", first, changes)
	}
}

// TestTabsDockLegacy: Dock's strip ignores Defaults("Tabs", ...) and keeps
// the card height and padding it had before batch 2 at Size(Small).
func TestTabsDockLegacy(t *testing.T) {
	act := "zz"
	var dt *TabsW
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Tabs", Props{"stretch": true, "variant": "line", "tabBarGutter": 40, "tabPlacement": "start", "centered": true}).Body(func() {
			dt = dockTabs(c, &act, b2hTabs()).Closable().Size(Small).Show()
		})
	}, DriverOptions{Width: 600, Height: 300})
	d.Frame()
	if dt.stretch || dt.variant != "" || dt.gutterSet || dt.place != Top || dt.centered {
		t.Fatalf("defaults leaked into Dock: %+v", dt)
	}
	tk := &dt.th.Components.Tabs
	if dt.cardH() != tk.CardHeight || dt.cardPadH() != 16 {
		t.Fatalf("dock card %v / %v", dt.cardH(), dt.cardPadH())
	}
	if act != "zz" {
		t.Fatalf("dock active key rewritten: %q", act)
	}
}

// TestCarouselDotDurationIdle: the DotDuration fill asks for frames only
// while autoplay counts: not when hover-paused, not when stopped on the
// last slide of Loop(false).
func TestCarouselDotDurationIdle(t *testing.T) {
	cur := 0
	loop := true
	d := NewDriver(func(c *Context) {
		Carousel(c, 3, b2hSlide(c)).Current(&cur).Autoplay(time.Second).DotDuration(true).Loop(loop).Height(160).Show()
	}, DriverOptions{Width: 400, Height: 300, Animations: true})
	d.Move(geom.Pt(390, 290)) // outside
	d.Frame()
	if !d.NeedsFrame() {
		t.Fatal("counting autoplay should animate the active dot")
	}
	d.Move(geom.Pt(200, 80)) // over the frame: paused
	d.Settle()
	if d.NeedsFrame() {
		t.Fatal("hover-paused autoplay keeps animating")
	}
	d.Move(geom.Pt(390, 290))
	loop, cur = false, 2 // stopped on the last slide
	d.Settle()
	if d.NeedsFrame() {
		t.Fatal("autoplay stopped at the end keeps animating")
	}
}

// TestWatermarkNegativeGap: a gap that swallows the mark does not hang.
func TestWatermarkNegativeGap(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Watermark(c, "x").Gap(-1000, -1000).Body(func() { Div(c, layout.W(200), layout.H(200)) })
	}, DriverOptions{Width: 300, Height: 300})
	d.Frame()
}

// TestImagePreviewElementDblClickMode: Element's viewer has no
// double-click zoom; its reset button toggles fitted / original size.
func TestImagePreviewElementDblClickMode(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2000, 1500))
	var w *ImageW
	d := NewDriver(func(c *Context) {
		w = Image(c, src).Width(60).Alt("pic").Preview(true)
	}, DriverOptions{Width: 800, Height: 600, Theme: theme.ElementPlus()})
	d.Frame()
	_ = d.ClickLabel("pic")
	d.Frame()
	p := b2hCenter(w.st.imgRect)
	fitted := w.st.imgRect.W
	d.Click(p)
	d.Click(p)
	if w.PreviewScale() != 1 {
		t.Fatalf("element double click zoomed: %v", w.PreviewScale())
	}
	_ = d.ClickLabel(ZhCN.Reset)
	d.Frame()
	if !w.st.original || w.st.imgRect.W <= fitted {
		t.Fatalf("original mode: %v %v -> %v", w.st.original, fitted, w.st.imgRect.W)
	}
}
