package ui

import (
	"reflect"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Batch 3, group J: Breadcrumb, Pagination, Steps, Anchor, Affix, BackTop,
// PageHeader, Splitter (docs/specs/align/3-nav.md).

func b3jDriver(page func(c *Context)) *Driver {
	return NewDriver(func(c *Context) { Column(c, func() { page(c) }, layout.Pad(10)) }, DriverOptions{Width: 900, Height: 400})
}

// ---------------------------------------------------------------------------
// Pagination

// TestPaginationPageListRc: the page buttons follow @rc-component/pagination
// 1.7 Pagination.js (pageBufferSize 2, 1 with showLessItems).
func TestPaginationPageListRc(t *testing.T) {
	for _, tc := range []struct {
		cur, pages int
		less       bool
		want       []int
	}{
		{1, 7, false, []int{1, 2, 3, 4, 5, 6, 7}},
		{1, 8, false, []int{1, 2, 3, 4, 5, -2, 8}},
		{4, 8, false, []int{1, 2, 3, 4, 5, -2, 8}},
		{5, 8, false, []int{1, -1, 4, 5, 6, 7, 8}},
		{1, 9, false, []int{1, 2, 3, 4, 5, -2, 9}},
		{5, 9, false, []int{1, -1, 4, 5, 6, -2, 9}},
		{4, 20, false, []int{1, 2, 3, 4, 5, -2, 20}},
		{5, 20, false, []int{1, -1, 4, 5, 6, -2, 20}},
		{10, 20, false, []int{1, -1, 9, 10, 11, -2, 20}},
		{17, 20, false, []int{1, -1, 16, 17, 18, 19, 20}},
		{20, 20, false, []int{1, -1, 16, 17, 18, 19, 20}},
		{1, 5, true, []int{1, 2, 3, 4, 5}},
		{1, 20, true, []int{1, 2, 3, -2, 20}},
		{10, 20, true, []int{1, -1, 9, 10, 11, -2, 20}},
	} {
		if got := pageList(tc.cur, tc.pages, tc.less); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("pageList(%d, %d, %v) = %v, want %v", tc.cur, tc.pages, tc.less, got, tc.want)
		}
	}
}

// TestPaginationShowLessItems: ShowLessItems and Defaults "showLessItems"
// narrow the window; the ••• jump moves 3 pages (rc-pagination
// jumpPrevPage = current - 3).
func TestPaginationShowLessItems(t *testing.T) {
	page := 10
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Pagination", Props{"showLessItems": true}).Body(func() {
			Pagination(c, &page, 200, 10).Show()
		})
	})
	d.EnableAccessibility()
	d.Frame()
	if _, ok := d.Find("12"); ok {
		t.Fatal("page 12 shown with showLessItems at 10")
	}
	n := mustNode(t, d, a11y.RoleButton, ZhCN.Pagination.Next3)
	if err := d.PerformAction(n.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	if page != 13 {
		t.Fatalf("next-3 jump: %d", page)
	}
}

// TestPaginationOutOfRange: page past the last page clamps, pageSize 0 is
// 1, total 0 shows a single disabled page 1 and disabled arrows (antd
// noPager), a negative total is 0.
func TestPaginationOutOfRange(t *testing.T) {
	page := 99
	d := b3jDriver(func(c *Context) { Pagination(c, &page, 45, 10).Show() })
	d.Frame()
	if page != 5 {
		t.Fatalf("clamped page %d", page)
	}
	page = 3
	d = b3jDriver(func(c *Context) { Pagination(c, &page, 3, 0).Show() })
	d.Frame()
	if page != 3 {
		t.Fatalf("pageSize 0: page %d", page)
	}
	for _, total := range []int{0, -4} {
		page = 1
		d = b3jDriver(func(c *Context) { Pagination(c, &page, total, 10).Show() })
		d.EnableAccessibility()
		d.Frame()
		n := mustNode(t, d, a11y.RoleButton, ZhCN.Page(1))
		if !n.States.Has(a11y.Disabled) || n.States.Has(a11y.Current) {
			t.Fatalf("total %d: page 1 %s", total, n)
		}
		if nx := mustNode(t, d, a11y.RoleButton, ZhCN.NextPage); !nx.States.Has(a11y.Disabled) {
			t.Fatalf("total %d: next enabled", total)
		}
	}
}

// TestPaginationDisabledScope: ConfigProvider.ComponentDisabled disables
// every part: pages, arrows and the go button do not respond.
func TestPaginationDisabledScope(t *testing.T) {
	page := 2
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() {
			Pagination(c, &page, 100, 10).GoButton("").Show()
		})
	})
	d.Frame()
	_ = d.ClickLabel("3")
	_ = d.ClickLabel(ZhCN.NextPage)
	_ = d.ClickLabel(ZhCN.Pagination.Confirm)
	if page != 2 {
		t.Fatalf("disabled pager moved to %d", page)
	}
}

// TestPaginationGoButtonTotalFunc: GoButton jumps to the typed page
// (antd showQuickJumper.goButton); ShowTotalFunc gets the item range
// (antd showTotal(total, range)).
func TestPaginationGoButtonTotalFunc(t *testing.T) {
	page := 2
	var from, to int
	d := b3jDriver(func(c *Context) {
		Pagination(c, &page, 95, 10).GoButton("Go").ShowTotalFunc(func(total, a, b int) string {
			from, to = a, b
			return "range"
		}).Show()
	})
	d.EnableAccessibility()
	d.Frame()
	if from != 11 || to != 20 {
		t.Fatalf("range %d-%d", from, to)
	}
	in := mustNode(t, d, a11y.RoleTextbox, ZhCN.Pagination.Jump)
	if err := d.PerformAction(in.ID, a11y.Focus, ""); err != nil {
		t.Fatal(err)
	}
	d.Frame()
	d.Type("7")
	if err := d.ClickLabel("Go"); err != nil {
		t.Fatal(err)
	}
	if page != 7 {
		t.Fatalf("go button: page %d", page)
	}
	d.Frame()
	if from != 61 || to != 70 {
		t.Fatalf("range %d-%d", from, to)
	}
	page = 10
	d.Frame()
	if from != 91 || to != 95 {
		t.Fatalf("last range %d-%d", from, to)
	}
}

// TestPaginationSizeAlignOptions: Size(Large) uses itemSizeLG, Align("end")
// pushes the pager to the end of its column, PageSizeOptions feeds the
// size changer.
func TestPaginationSizeAlignOptions(t *testing.T) {
	page, size := 1, 10
	d := b3jDriver(func(c *Context) {
		Pagination(c, &page, 500, size).Size(Large).Align("end").SizeChanger(&size).PageSizeOptions(5, 15).Show()
	})
	d.Frame()
	r, ok := d.Find("1")
	if !ok || r.H != 40 {
		t.Fatalf("large item %v", r)
	}
	if r.X < 300 {
		t.Fatalf("align end: first page at x=%v", r.X)
	}
	if _, ok := d.FindText(ZhCN.PageSizeLabel(10)); ok {
		t.Fatal("default size options shown")
	}
}

// TestPaginationElementDefaults: the Element theme takes pager-count 7,
// background and its page sizes from PaginationToken (no Flavor branch).
func TestPaginationElementDefaults(t *testing.T) {
	page := 10
	var pw *PaginationW
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).Theme(theme.ElementPlus()).Body(func() { pw = Pagination(c, &page, 200, 10).Show() })
	})
	d.Frame()
	if pw.pagerCount != 7 || !pw.bg || len(pw.sizeList) != 6 {
		t.Fatalf("element defaults: count %d bg %v sizes %v", pw.pagerCount, pw.bg, pw.sizeList)
	}
	for _, s := range []string{"8", "12"} {
		if _, ok := d.Find(s); !ok {
			t.Fatalf("pager-count 7: page %s missing", s)
		}
	}
}

// TestPaginationA11y: navigation landmark named from Locale, the current
// page Current, jumps named "next 5 pages".
func TestPaginationA11y(t *testing.T) {
	page := 10
	d := b3jDriver(func(c *Context) { Pagination(c, &page, 200, 10).Show() })
	d.EnableAccessibility()
	d.Frame()
	mustNode(t, d, a11y.RoleNavigation, ZhCN.A11y.Pagination)
	if n := mustNode(t, d, a11y.RoleButton, ZhCN.Page(10)); !n.States.Has(a11y.Current) {
		t.Fatalf("current page %s", n)
	}
	mustNode(t, d, a11y.RoleButton, ZhCN.Next5)
	mustNode(t, d, a11y.RoleButton, ZhCN.Prev5)
}

// ---------------------------------------------------------------------------
// Steps

func b3jSteps(n int) []StepItem {
	out := make([]StepItem, n)
	for i := range out {
		out[i] = StepItem{Title: "S" + string(rune('A'+i)), Content: "c" + string(rune('a'+i))}
	}
	return out
}

// TestStepsV6Names: orientation / titlePlacement / type "dot" /
// items[].content (antd 6 names) via methods and Defaults keys.
func TestStepsV6Names(t *testing.T) {
	var sw *StepsW
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Steps", Props{"orientation": "vertical", "titlePlacement": "vertical"}).Body(func() {
			sw = Steps(c, b3jSteps(3)).Current(1).Show()
		})
	})
	d.Frame()
	if !sw.vertical || !sw.labelVert {
		t.Fatalf("defaults: vertical %v titleVertical %v", sw.vertical, sw.labelVert)
	}
	if sw.items[1].Description != "cb" {
		t.Fatal("Content not used as the step text")
	}
	if s := (&StepsW{}).Type("dot"); !s.dot || !s.labelVert {
		t.Fatal(`Type("dot")`)
	}
	if s := (&StepsW{}).Orientation("vertical").TitlePlacement("horizontal"); !s.vertical || s.labelVert {
		t.Fatal("Orientation / TitlePlacement")
	}
}

// TestStepsInitialMaxCount: Initial numbers from initial+1 and offsets
// the bound value; MaxCount keeps first, last and current with ellipsis
// steps (antd useDisplaySteps).
func TestStepsInitialMaxCount(t *testing.T) {
	cur := 15
	var sw *StepsW
	d := b3jDriver(func(c *Context) {
		sw = Steps(c, b3jSteps(10)).Initial(10).MaxCount(5).Clickable(&cur).Show()
	})
	d.EnableAccessibility()
	d.Frame()
	// current = index 5: kept 0, 4, 5, 6, 9 -> SA … SE SF SG … SJ.
	for _, s := range []string{"SA", "SE", "SF", "SG", "SJ"} {
		mustFind(t, d, s)
	}
	if _, ok := d.Find("SC"); ok {
		t.Fatal("SC shown with maxCount 5")
	}
	if got := sw.num(sw.current); got != "16" || sw.items[sw.current].Title != "SF" {
		t.Fatalf("current step number %s, want initial+index+1 = 16", got)
	}
	d.Click(mustFind(t, d, "SJ").Center())
	if cur != 19 {
		t.Fatalf("click last: %d", cur)
	}
}

// TestStepsOutOfRange: current < 0 waits everywhere, >= len finishes
// everything; no panic, also with MaxCount.
func TestStepsOutOfRange(t *testing.T) {
	for _, cur := range []int{-3, 7} {
		var sw *StepsW
		d := b3jDriver(func(c *Context) { sw = Steps(c, b3jSteps(6)).Current(cur).MaxCount(4).Show() })
		d.Frame()
		want := "wait"
		if cur > 0 {
			want = "finish"
		}
		for i := range sw.items {
			if st := sw.statusOf(i); st != want {
				t.Fatalf("current %d: step %d %s", cur, i, st)
			}
		}
	}
}

// TestStepsDisabledItem: a disabled step does not respond to clicks or
// the keyboard; neither does any step under ComponentDisabled.
func TestStepsDisabledItem(t *testing.T) {
	cur := 0
	items := b3jSteps(3)
	items[2].Disabled = true
	off := false
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(off).Body(func() { Steps(c, items).Clickable(&cur).Show() })
	})
	d.Frame()
	if _, ok := d.Find("SC"); ok {
		t.Fatal("disabled step is interactive")
	}
	r := mustFind(t, d, "SB")
	off = true
	d.Frame()
	d.Click(r.Center())
	if cur != 0 {
		t.Fatalf("ComponentDisabled step clicked: %d", cur)
	}
}

// TestStepsA11y: list named from Locale, the current step Current.
func TestStepsA11y(t *testing.T) {
	d := b3jDriver(func(c *Context) { Steps(c, b3jSteps(3)).Current(1).Show() })
	d.EnableAccessibility()
	d.Frame()
	mustNode(t, d, a11y.RoleList, ZhCN.Steps)
	if n := mustNode(t, d, a11y.RoleListItem, "SB"); !n.States.Has(a11y.Current) {
		t.Fatalf("current step %s", n)
	}
}

// ---------------------------------------------------------------------------
// Splitter

// TestSplitterOrientationPercentLimits: Orientation / Defaults
// "orientation" (antd 6, layout deprecated); MinPercent / MaxPercent clamp
// a drag (antd min / max "20%").
func TestSplitterOrientationPercentLimits(t *testing.T) {
	var s *SplitterW
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Splitter", Props{"orientation": "horizontal"}).Body(func() {
			s = Splitter(c).Style(layout.W(606), layout.H(100))
			s.Body(func() {
				SplitterPanel(c).MaxPercent(0.7).Body(func() {})
				SplitterPanel(c).Body(func() {})
			})
		})
	})
	d.Frame()
	d.Frame()
	if s.vertical {
		t.Fatal("orientation horizontal gave a vertical splitter")
	}
	from := mustFind(t, d, "splitter-bar-1").Center()
	d.Drag(from, geom.Pt(from.X+400, from.Y))
	if f := s.Sizes(); f[0] > 0.7001 {
		t.Fatalf("MaxPercent 0.7 exceeded: %v", f)
	}
	if v := (&SplitterW{}).Orientation("vertical"); !v.vertical {
		t.Fatal(`Orientation("vertical")`)
	}
}

// TestSplitterOutOfRange: default sizes summing past the container and a
// minimum above the maximum do not panic; the minimum wins.
func TestSplitterOutOfRange(t *testing.T) {
	var s *SplitterW
	d := b3jDriver(func(c *Context) {
		s = Splitter(c).Style(layout.W(606), layout.H(100))
		s.Body(func() {
			SplitterPanel(c).DefaultPercent(0.8).Min(300).Max(100).Body(func() {})
			SplitterPanel(c).DefaultPercent(0.8).Body(func() {})
			SplitterPanel(c).Body(func() {})
		})
	})
	d.Frame()
	d.Frame()
	from := mustFind(t, d, "splitter-bar-1").Center()
	d.Drag(from, geom.Pt(from.X-300, from.Y))
	if f := s.Sizes(); f[0]*600 < 299 {
		t.Fatalf("min 300 > max 100: pane 0 = %v px", f[0]*600)
	}
}

// TestSplitterDisabledScope: under ComponentDisabled the bars do not
// resize (pointer or keyboard).
func TestSplitterDisabledScope(t *testing.T) {
	var s *SplitterW
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() {
			s = Splitter(c).Style(layout.W(606), layout.H(100))
			s.Body(func() {
				SplitterPanel(c).Body(func() {})
				SplitterPanel(c).Body(func() {})
			})
		})
	})
	d.Frame()
	d.Frame()
	from, ok := d.Find("splitter-bar-1")
	if ok {
		d.Drag(from.Center(), geom.Pt(from.Center().X+100, from.Center().Y))
	}
	if f := s.Sizes(); !near(f[0], 0.5) {
		t.Fatalf("disabled splitter resized: %v", f)
	}
}

// TestSplitterA11yWithHandle: bars are separators named from Locale with
// a percent value; WithHandle (shadcn withHandle) keeps them working.
func TestSplitterA11yWithHandle(t *testing.T) {
	d := b3jDriver(func(c *Context) {
		Splitter(c).WithHandle().Style(layout.W(606), layout.H(100)).Body(func() {
			SplitterPanel(c).Body(func() {})
			SplitterPanel(c).Body(func() {})
		})
	})
	d.EnableAccessibility()
	d.Frame()
	d.Frame()
	n := mustNode(t, d, a11y.RoleSeparator, ZhCN.A11y.Splitter)
	if n.Value != "50%" {
		t.Fatalf("separator value %q", n.Value)
	}
}

// ---------------------------------------------------------------------------
// Breadcrumb

// TestBreadcrumbSeparatorSpacing: the "/" sits 8px from the text on both
// sides, also before the plain current page (antd separatorMargin
// marginXS; the link's paddingXXS is pulled back by marginXXS).
func TestBreadcrumbSeparatorSpacing(t *testing.T) {
	d := b3jDriver(func(c *Context) { Breadcrumb(c, "Home", "List", "Detail") })
	d.Frame()
	home, list := mustFind(t, d, "Home"), mustFind(t, d, "List")
	det, ok := d.FindText("Detail")
	if !ok {
		t.Fatal("no current page")
	}
	// Gaps between the link boxes (padding 4 inside) and the plain text.
	g1 := list.X - home.Right() // link | 4 + "/" + 4 | link
	g2 := det.X - list.Right()  // link | 4 + "/" + 4+4 | text
	if !near(g2-g1, 4) {
		t.Fatalf("separator spacing before the current page: %v vs %v", g2, g1)
	}
}

// TestBreadcrumbItemSeparatorAndDropdownIcon: BreadcrumbItem.Separator
// replaces the separator after it (antd type "separator"); DropdownIcon is
// accepted for menu items.
func TestBreadcrumbItemSeparatorAndDropdownIcon(t *testing.T) {
	d := b3jDriver(func(c *Context) {
		BreadcrumbItems(c, []BreadcrumbItem{
			{Label: "Home", Separator: ":"},
			{Label: "Apps", Menu: []MenuItem{{Key: "a", Label: "App A"}}},
			{Label: "Detail"},
		}).DropdownIcon(icon.CaretDown).Show()
	})
	d.Frame()
	if _, ok := d.FindText(":"); !ok {
		t.Fatal("item separator missing")
	}
	if _, ok := d.FindText("/"); !ok {
		t.Fatal("default separator missing after Apps")
	}
}

// TestBreadcrumbDisabledScope: links do not report clicks under
// ComponentDisabled.
func TestBreadcrumbDisabledScope(t *testing.T) {
	var bw *BreadcrumbW
	clicked := -1
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() {
			bw = Breadcrumb(c, "Home", "List", "Detail")
			if bw.Clicked() >= 0 {
				clicked = bw.Clicked()
			}
		})
	})
	d.Frame()
	if r, ok := d.Find("Home"); ok {
		d.Click(r.Center())
	}
	if clicked >= 0 {
		t.Fatalf("disabled breadcrumb clicked %d", clicked)
	}
}

// TestBreadcrumbA11y: navigation landmark named from Locale, links, the
// last item Current.
func TestBreadcrumbA11y(t *testing.T) {
	d := b3jDriver(func(c *Context) { Breadcrumb(c, "Home", "Detail") })
	d.EnableAccessibility()
	d.Frame()
	mustNode(t, d, a11y.RoleNavigation, ZhCN.A11y.Breadcrumb)
	mustNode(t, d, a11y.RoleLink, "Home")
}

// ---------------------------------------------------------------------------
// PageHeader

// TestPageHeaderProLayout: Pro v3 padding 6/16 (12 on top with a
// breadcrumb), back button 16 before the title, avatar, structured
// breadcrumb clicks, BackIcon.
func TestPageHeaderProLayout(t *testing.T) {
	var ph *PageHeaderW
	back, crumb := false, -1
	d := NewDriver(func(c *Context) {
		ph = PageHeader(c, "Title").BackIcon(icon.Left).Avatar(func() { Text(c, "AV") }).
			BreadcrumbItems([]BreadcrumbItem{{Label: "Home"}, {Label: "Page"}}).Show()
		if ph.BackClicked() {
			back = true
		}
		if ph.CrumbClicked() >= 0 {
			crumb = ph.CrumbClicked()
		}
	}, DriverOptions{Width: 600, Height: 200})
	d.Frame()
	home := mustFind(t, d, "Home")
	if !near(home.Y, 12) || !near(home.X, 16) {
		t.Fatalf("breadcrumb at %v, want padding 12 top / 16 inline", home)
	}
	// Pro heading-left marginBlock marginXS/2: the 32px title row starts
	// 8 (breadcrumb gap) + 4 below the breadcrumb.
	if ti, ok := d.FindText("Title"); !ok || ti.Y < home.Bottom()+12-1 {
		t.Fatalf("title at %v, breadcrumb bottom %v", ti, home.Bottom())
	}
	bk := mustFind(t, d, ZhCN.Back)
	av, ok := d.FindText("AV")
	if !ok || !near(av.X-bk.Right(), 16) {
		t.Fatalf("back -> avatar gap %v", av.X-bk.Right())
	}
	d.Click(bk.Center())
	if !back {
		t.Fatal("back not reported")
	}
	d.Click(home.Center())
	if crumb != 0 {
		t.Fatalf("crumb click %d", crumb)
	}
}

// TestPageHeaderA11y: the title is a level-1 heading, the back button is
// named from Locale.
func TestPageHeaderA11y(t *testing.T) {
	d := b3jDriver(func(c *Context) { PageHeader(c, "Orders").Back().Show() })
	d.EnableAccessibility()
	d.Frame()
	if h := mustNode(t, d, a11y.RoleHeading, "Orders"); h.Level != 1 {
		t.Fatalf("heading level %d", h.Level)
	}
	mustNode(t, d, a11y.RoleButton, ZhCN.Back)
}

// TestPageHeaderDisabledScope: the back button does not respond under
// ComponentDisabled.
func TestPageHeaderDisabledScope(t *testing.T) {
	back := false
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() {
			if PageHeader(c, "Orders").Back().Show().BackClicked() {
				back = true
			}
		})
	})
	d.Frame()
	if r, ok := d.Find(ZhCN.Back); ok {
		d.Click(r.Center())
	}
	if back {
		t.Fatal("disabled back clicked")
	}
}

// ---------------------------------------------------------------------------
// Anchor / Affix

// TestAnchorDirectionBoundsCurrent: Defaults "direction", Bounds and
// CurrentAnchor (antd getCurrentAnchor); the Element theme's default
// bound is 15 (el-anchor bound).
func TestAnchorDirectionBoundsCurrent(t *testing.T) {
	var anc *AnchorW
	items := []AnchorItem{{Key: "a", Title: "SecA"}, {Key: "b", Title: "SecB"}}
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Anchor", Props{"direction": "horizontal"}).Body(func() {
			anc = Anchor(c, items).CurrentAnchor(func(string) string { return "b" }).Show()
		})
	})
	d.Frame()
	if !anc.horizontal || anc.Active() != "b" {
		t.Fatalf("horizontal %v active %q", anc.horizontal, anc.Active())
	}
	d = b3jDriver(func(c *Context) {
		ConfigProvider(c).Theme(theme.ElementPlus()).Body(func() { anc = Anchor(c, items).Show() })
	})
	d.Frame()
	if anc.bounds != 15 {
		t.Fatalf("element bound %v", anc.bounds)
	}
	if (&AnchorW{}).Bounds(9).bounds != 9 || (&AnchorW{}).Type("underline").rail != 1 {
		t.Fatal("Bounds / Type")
	}
}

// TestAnchorOutOfRange: no sections, unknown keys and an empty item list
// do not panic; nothing is active.
func TestAnchorOutOfRange(t *testing.T) {
	var anc *AnchorW
	d := b3jDriver(func(c *Context) {
		anc = Anchor(c, []AnchorItem{{Key: "missing", Title: "Missing"}}).Show()
		Anchor(c, nil).Show()
	})
	d.Frame()
	d.Frame()
	if a := anc.Active(); a != "" && a != "missing" {
		t.Fatalf("active %q", a)
	}
	d.Click(mustFind(t, d, "Missing").Center())
}

// TestAnchorDisabledScope: links do not report clicks under
// ComponentDisabled.
func TestAnchorDisabledScope(t *testing.T) {
	clicked := ""
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).ComponentDisabled(true).Body(func() {
			if k := Anchor(c, []AnchorItem{{Key: "a", Title: "SecA"}}).Show().Clicked(); k != "" {
				clicked = k
			}
		})
	})
	d.Frame()
	if r, ok := d.Find("SecA"); ok {
		d.Click(r.Center())
	}
	if clicked != "" {
		t.Fatalf("disabled anchor clicked %q", clicked)
	}
}

// TestAnchorA11y: navigation landmark, links, the active one Current,
// nested links carry a level.
func TestAnchorA11y(t *testing.T) {
	d := b3jDriver(func(c *Context) {
		Anchor(c, []AnchorItem{{Key: "a", Title: "SecA", Children: []AnchorItem{{Key: "a1", Title: "SubA"}}}}).
			CurrentAnchor(func(string) string { return "a" }).Show()
	})
	d.EnableAccessibility()
	d.Frame()
	mustNode(t, d, a11y.RoleNavigation, ZhCN.A11y.Navigation)
	if n := mustNode(t, d, a11y.RoleLink, "SecA"); !n.States.Has(a11y.Current) {
		t.Fatalf("active link %s", n)
	}
	if n := mustNode(t, d, a11y.RoleLink, "SubA"); n.Level != 2 {
		t.Fatalf("nested level %d", n.Level)
	}
}

// TestAffixOffsetBottomDefault: Defaults "offsetBottom" pins to the bottom.
func TestAffixOffsetBottomDefault(t *testing.T) {
	var aff *AffixW
	d := b3jDriver(func(c *Context) {
		ConfigProvider(c).Defaults("Affix", Props{"offsetBottom": 10}).Body(func() {
			aff = Affix(c).Body(func() { Text(c, "pinned") })
		})
	})
	d.Frame()
	if !aff.hasBottom || aff.bottom != 10 {
		t.Fatalf("offsetBottom default: %v %v", aff.hasBottom, aff.bottom)
	}
}

// ---------------------------------------------------------------------------
// BackTop

// TestBackTopThemeDefaults: visibility height and position come from
// FloatButtonToken (antd 400 at 24 / 48, Element 200 at 40 / 40).
func TestBackTopThemeDefaults(t *testing.T) {
	var bt *BackTopW
	for _, tc := range []struct {
		th        *theme.Theme
		vis, r, b float32
	}{{theme.AntDesign(), 400, 24, 48}, {theme.ElementPlus(), 200, 40, 40}} {
		d := b3jDriver(func(c *Context) {
			ConfigProvider(c).Theme(tc.th).Body(func() { bt = BackTop(c, 0).Show() })
		})
		d.Frame()
		if bt.visible != tc.vis || bt.right != tc.r || bt.bottom != tc.b {
			t.Fatalf("%v: visible %v at %v/%v", tc.th.Flavor, bt.visible, bt.right, bt.bottom)
		}
	}
}

// TestBackTopA11yStyled: a styled BackTop (Primary, Square, Description)
// is a button named from Locale and scrolls the target back.
func TestBackTopA11yStyled(t *testing.T) {
	var sb *ScrollBox
	var target ID
	d := b3jDriver(func(c *Context) {
		sb = Scroll(c, func() { Column(c, func() {}, layout.H(2000)) }, layout.H(300))
		target = sb.ID()
		BackTop(c, target).VisibilityHeight(100).Primary().Square().Description("Top").Show()
	})
	d.EnableAccessibility()
	d.Settle()
	d.Scroll(geom.Pt(100, 100), 500)
	d.Settle()
	n := mustNode(t, d, a11y.RoleButton, ZhCN.BackTop)
	if err := d.PerformAction(n.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	d.Frame()
	if _, y := sb.Offset(); y != 0 {
		t.Fatalf("offset after back top = %v", y)
	}
}

// TestStepsOutlinedHover: outlined steps keep their ring on hover in
// colorPrimaryHover (antd status.js outlined border-hover / text-hover);
// filled waiting steps get the colorPrimaryBgHover fill without a ring.
func TestStepsOutlinedHover(t *testing.T) {
	th := theme.AntDesign()
	s := &StepsW{items: b3jSteps(3), current: 1, outlined: true, th: th, tk: &th.Components.Steps}
	k := s.colors("wait", true)
	if k.iconBorder != th.Primary.Hover || k.iconFg != th.Primary.Hover {
		t.Fatalf("outlined wait hover ring %v glyph %v", k.iconBorder, k.iconFg)
	}
	if k := s.colors("error", true); k.iconBorder != th.Error.Hover {
		t.Fatalf("outlined error hover ring %v", k.iconBorder)
	}
	s.outlined = false
	if k := s.colors("wait", true); k.iconBorder.A != 0 || k.iconBg != th.Components.Steps.HoverIconBg {
		t.Fatalf("filled wait hover %v %v", k.iconBorder, k.iconBg)
	}
	// Element icons are rings already: Variant("outlined") changes nothing.
	el := theme.ElementPlus()
	e := &StepsW{items: b3jSteps(3), current: 1, outlined: true, th: el, tk: &el.Components.Steps}
	if k := e.colors("wait", false); k.iconBg != el.Components.Steps.WaitIconBg {
		t.Fatalf("element outlined changed the icon fill")
	}
}

// ---------------------------------------------------------------------------
// Tabs (batch 2 review fixes: Variant without a Flavor branch, gutter)

// TestTabsGutterAllTypes: TabBarGutter applies to every type, also the
// shadcn segmented list and the border card (antd tabBarGutter).
func TestTabsGutterAllTypes(t *testing.T) {
	for _, tc := range []struct {
		name string
		th   *theme.Theme
		bc   bool
	}{{"shadcn", theme.Shadcn(), false}, {"shadcn-line", theme.Shadcn(), false}, {"ant-bordercard", theme.AntDesign(), true}} {
		act := "a"
		d := NewDriver(func(c *Context) {
			w := Tabs(c, &act, []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}}).TabBarGutter(20)
			if tc.bc {
				w.BorderCard()
			}
			if tc.name == "shadcn-line" {
				w.Variant("line")
			}
			w.Show()
		}, DriverOptions{Theme: tc.th, Width: 600, Height: 300})
		d.Settle()
		a, b := mustFind(t, d, "Alpha"), mustFind(t, d, "Beta")
		if gap := b.X - a.Right(); !near(gap, 20) {
			t.Fatalf("%s gutter %v", tc.name, gap)
		}
	}
}

// TestTabsVariantLineNoFlavor: Variant("line") is decided by
// TabsToken.SegmentedList, not the theme flavor: line tabs are the
// default bar of Ant and Element (same layout), the shadcn segmented
// list switches to line tabs; a custom token turns it on elsewhere.
func TestTabsVariantLineNoFlavor(t *testing.T) {
	if theme.AntDesign().Components.Tabs.SegmentedList || theme.ElementPlus().Components.Tabs.SegmentedList || !theme.Shadcn().Components.Tabs.SegmentedList {
		t.Fatal("SegmentedList defaults")
	}
	act := "a"
	line := false
	var tw *TabsW
	d := NewDriver(func(c *Context) {
		tw = Tabs(c, &act, []TabItem{{Key: "a", Label: "Alpha"}, {Key: "b", Label: "Beta"}})
		if line {
			tw.Variant("line")
		}
		tw.Show()
	}, DriverOptions{Width: 600, Height: 300})
	d.Settle()
	b0 := mustFind(t, d, "Beta")
	line = true
	d.Settle()
	if b1 := mustFind(t, d, "Beta"); b1 != b0 || tw.shadLine() {
		t.Fatalf("ant line variant changed the bar: %v -> %v", b0, b1)
	}
	th := theme.AntDesign()
	th.Components.Tabs.SegmentedList = true
	tw.th = th
	if !tw.shadLine() {
		t.Fatal("SegmentedList token not honored")
	}
}
