package ui

import (
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Batch 4 group C: Pro layout / list / provider alignment with Pro
// Components v3 (@ant-design/pro-components@3.1.14-2, es/layout, es/list).

func b4cDriver(th *theme.Theme, w, h float32, page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) { Column(c, func() { page(c) }, layout.Grow(1)) },
		DriverOptions{Width: w, Height: h, Theme: th})
	d.Settle()
	return d
}

func b4cThemes(t *testing.T, fn func(t *testing.T, th func() *theme.Theme)) {
	for _, tc := range []struct {
		name string
		th   func() *theme.Theme
	}{
		{"ant", func() *theme.Theme { return theme.AntDesign() }},
		{"element", func() *theme.Theme { return theme.ElementPlus() }},
		{"shadcn", func() *theme.Theme { return theme.Shadcn() }},
		{"shadcn-dark", func() *theme.Theme { return theme.Shadcn(theme.Dark()) }},
	} {
		t.Run(tc.name, func(t *testing.T) { fn(t, tc.th) })
	}
}

var b4cMenu = []MenuItem{
	{Key: "home", Label: "Home"},
	{Key: "dash", Label: "Dashboard", Children: []MenuItem{
		{Key: "analysis", Label: "Analysis"},
		{Key: "monitor", Label: "Monitor"},
	}},
	{Key: "users", Label: "Users"},
}

// ---------------------------------------------------------------------------
// ProBreadcrumb (es/layout/components/PageContainer/index.js ProBreadcrumb:
// Breadcrumb with RouteContext breadcrumb, props override).

func TestProBreadcrumbFromRouteContext(t *testing.T) {
	b4cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		sel := "monitor"
		var outside, inHeader *ProBreadcrumbW
		var rc RouteInfo
		d := b4cDriver(th(), 1200, 600, func(c *Context) {
			outside = ProBreadcrumb(c).Show()
			ProLayout(c, ProLayoutConfig{Title: "T", Layout: "side", Menu: b4cMenu, Selected: &sel, AutoBreadcrumb: true,
				HeaderContentRender: func() {
					rc = RouteContext(c)
					inHeader = ProBreadcrumb(c).Show()
				}}, func() { Text(c, "page") })
		})
		d.EnableAccessibility()
		d.Settle()
		if outside.Len() != 0 {
			t.Fatalf("outside a ProLayout ProBreadcrumb has no route items, got %d", outside.Len())
		}
		if !rc.InLayout || len(rc.BreadcrumbItems) != 2 {
			t.Fatalf("RouteContext in the header render: %+v", rc)
		}
		if inHeader.Len() != 2 {
			t.Fatalf("route breadcrumb items = %d", inHeader.Len())
		}
		navs := d.A11yFindAll(a11y.RoleNavigation, ZhCN.A11y.Breadcrumb)
		if len(navs) != 1 {
			t.Fatalf("breadcrumb landmarks = %d", len(navs))
		}
		dash, ok1 := d.A11yFind(a11y.RoleLink, "Dashboard")
		mon, ok2 := d.A11yFind(a11y.RoleText, "Monitor")
		if !ok1 || !ok2 || mon.Bounds.X <= dash.Bounds.X {
			t.Fatalf("breadcrumb levels out of order: %v %v", dash, mon)
		}
		cur := false
		for _, n := range d.A11yFindAll(a11y.RoleText, "Monitor") {
			if n.States&a11y.Current != 0 {
				cur = true
			}
		}
		for _, n := range d.A11yFindAll(a11y.RoleLink, "Monitor") {
			if n.States&a11y.Current != 0 {
				cur = true
			}
		}
		if !cur {
			t.Fatal("the last item should be the current page")
		}
	})
}

func TestProBreadcrumbItemsOverride(t *testing.T) {
	clicked := -1
	var bc *ProBreadcrumbW
	hit := false
	d := b4cDriver(theme.AntDesign(), 800, 300, func(c *Context) {
		bc = ProBreadcrumb(c).Items([]BreadcrumbItem{{Label: "Root", OnClick: func() { hit = true }}, {Label: "Leaf"}}).Separator(">").Show()
		if bc.Clicked() >= 0 {
			clicked = bc.Clicked()
		}
	})
	if bc.Len() != 2 {
		t.Fatalf("items = %d", bc.Len())
	}
	if _, ok := d.FindText(">"); !ok {
		t.Fatal("separator override ignored")
	}
	r, _ := d.FindText("Root")
	d.Click(r.Center())
	d.Settle()
	if clicked != 0 || !hit {
		t.Fatalf("clicked = %d hit = %v", clicked, hit)
	}
}

// ---------------------------------------------------------------------------
// PageLoading (es/layout/components/PageLoading/index.js: a div with
// paddingBlockStart 100, text-align center, holding Spin size="large").

func TestPageLoadingSemantics(t *testing.T) {
	b4cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		on := true
		var top float32
		d := b4cDriver(th(), 800, 500, func(c *Context) {
			PageLoading(c).Spinning(on).Show()
		})
		d.EnableAccessibility()
		d.Settle()
		n, ok := d.A11yFind(a11y.RoleStatus, ZhCN.A11y.Loading)
		if !ok || n.States&a11y.Busy == 0 {
			t.Fatalf("PageLoading should be a busy status named %q", ZhCN.A11y.Loading)
		}
		top = n.Bounds.Y
		if top < 99 || top > 101 {
			t.Fatalf("spinner top = %v, want 100 (paddingBlockStart)", top)
		}
		if cx := n.Bounds.X + n.Bounds.W/2; cx < 399 || cx > 401 {
			t.Fatalf("spinner centre x = %v, want 400", cx)
		}
		// Large Spin indicator.
		if n.Bounds.W < 24 {
			t.Fatalf("indicator %v is not the large size", n.Bounds)
		}
		on = false
		d.Settle()
		if _, ok := d.A11yFind(a11y.RoleStatus, ZhCN.A11y.Loading); ok {
			t.Fatal("Spinning(false) still shows the status")
		}
	})
}

func TestPageLoadingDelayAndDescription(t *testing.T) {
	d := NewDriver(func(c *Context) {
		Column(c, func() { PageLoading(c).Delay(500 * time.Millisecond).Description("Loading page").Show() })
	}, DriverOptions{Width: 600, Height: 400, Theme: theme.AntDesign(), Animations: true})
	d.Settle()
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleStatus, "Loading page"); ok {
		t.Fatal("delayed PageLoading shown before its delay")
	}
	d.Advance(600 * time.Millisecond)
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleStatus, "Loading page"); !ok {
		t.Fatal("PageLoading not shown after its delay")
	}
}

func TestPageContainerLoadingUsesPageLoading(t *testing.T) {
	d := b4cDriver(theme.AntDesign(), 800, 600, func(c *Context) {
		PageContainer(c, "Page").Loading(true).Body(func() { Text(c, "body text") })
	})
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.FindText("body text"); ok {
		t.Fatal("loading container still shows its body")
	}
	n, ok := d.A11yFind(a11y.RoleStatus, ZhCN.A11y.Loading)
	if !ok {
		t.Fatal("loading status missing")
	}
	title, _ := pro2Leaf(d, "Page")
	if gap := n.Bounds.Y - title.Bottom(); gap < 100 {
		t.Fatalf("PageLoading padding: spinner %v below the title by %v (< 100)", n.Bounds, gap)
	}
}

// PageContainer renders no page header when it has nothing to show
// (memoRenderPageHeader returns null).
func TestPageContainerNoEmptyHeader(t *testing.T) {
	d := b4cDriver(theme.AntDesign(), 800, 400, func(c *Context) {
		PageContainer(c, "").Body(func() { Text(c, "only body") })
	})
	r, ok := d.FindText("only body")
	if !ok {
		t.Fatal("body missing")
	}
	// No header: the body starts after the container padding only.
	if r.Y > 40 {
		t.Fatalf("empty header still takes space: body at %v", r.Y)
	}
}

// ---------------------------------------------------------------------------
// TopNavHeader (es/layout/components/TopNavHeader: logo + title, a
// horizontal BaseMenu taking the free space, then ActionsContent with the
// avatar; -wide 1152 centred when contentWidth Fixed in the top layout).

func TestTopNavHeaderNavigationAndUser(t *testing.T) {
	b4cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		sel := "home"
		headerClicks := 0
		d := b4cDriver(th(), 1200, 300, func(c *Context) {
			TopNavHeader(c, b4cMenu, &sel).Title("Tango Pro").UserName("Lily").
				Actions(func() { IconButton(c, nil, "Search").Size(32, 16) }).
				OnMenuHeaderClick(func() { headerClicks++ }).Show()
		})
		d.EnableAccessibility()
		d.Settle()
		if _, ok := d.A11yFind(a11y.RoleNavigation, ZhCN.A11y.Navigation); !ok {
			t.Fatal("top nav header is not a navigation landmark")
		}
		if _, ok := d.A11yFind(a11y.RoleMenuBar, ""); !ok {
			t.Fatal("horizontal menu bar missing")
		}
		title, _ := d.FindText("Tango Pro")
		home, _ := d.FindText("Home")
		user, ok := d.FindText("Lily")
		if !ok {
			t.Fatal("user area missing")
		}
		if !(title.X < home.X && home.X < user.X) {
			t.Fatalf("order logo/title < menu < user: %v %v %v", title, home, user)
		}
		if err := d.ClickLabel("Users"); err != nil {
			t.Fatal(err)
		}
		d.Settle()
		if sel != "users" {
			t.Fatalf("selected = %q", sel)
		}
		btn, ok := d.A11yFind(a11y.RoleButton, "Tango Pro")
		if !ok {
			t.Fatal("clickable menu header should be a button named by the title")
		}
		if err := d.PerformAction(btn.ID, a11y.Press, ""); err != nil {
			t.Fatal(err)
		}
		d.Settle()
		if headerClicks != 1 {
			t.Fatalf("OnMenuHeaderClick calls = %d", headerClicks)
		}
		if rep := d.AuditNoContrast(); rep.MissingInteractiveNames() > 0 {
			t.Fatalf("audit: %v", rep)
		}
	})
}

func TestTopNavHeaderNarrowFoldsMenu(t *testing.T) {
	b4cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		sel := "home"
		menu := append(append([]MenuItem(nil), b4cMenu...),
			MenuItem{Key: "reports", Label: "Reports"}, MenuItem{Key: "settings", Label: "Settings"}, MenuItem{Key: "billing", Label: "Billing"})
		d := b4cDriver(th(), 520, 300, func(c *Context) {
			TopNavHeader(c, menu, &sel).Title("Tango").UserName("Lily").Show()
		})
		d.EnableAccessibility()
		d.Settle()
		d.Settle()
		if _, ok := d.A11yFind(a11y.RoleMenuItem, ZhCN.More); !ok {
			t.Fatal("narrow header should fold the menu into the overflow item")
		}
		if _, ok := d.FindText("Billing"); ok {
			t.Fatal("last item should be folded at 520px")
		}
		title, ok1 := d.FindText("Tango")
		user, ok2 := d.FindText("Lily")
		if !ok1 || !ok2 || title.X < 0 || user.Right() > 520.5 {
			t.Fatalf("title %v / user %v not inside the window", title, user)
		}
		if rep := d.AuditNoContrast(); rep.MissingInteractiveNames() > 0 {
			t.Fatalf("audit: %v", rep)
		}
	})
}

func TestTopNavHeaderFixedWidth(t *testing.T) {
	sel := "home"
	d := b4cDriver(theme.AntDesign(), 1600, 300, func(c *Context) {
		TopNavHeader(c, b4cMenu, &sel).Title("Wide").ContentWidth("Fixed").Show()
	})
	d.Settle()
	r, _ := d.FindText("Wide")
	if side := float32(1600-TopNavHeaderWideWidth) / 2; r.X < side {
		t.Fatalf("Fixed top nav content starts at %v, want >= %v", r.X, side)
	}
}

// ProLayout's top layout builds its header with TopNavHeader.
func TestProLayoutTopUsesTopNavHeader(t *testing.T) {
	sel := "home"
	d := b4cDriver(theme.AntDesign(), 1200, 500, func(c *Context) {
		ProLayout(c, ProLayoutConfig{Title: "Shell", Layout: "top", Menu: b4cMenu, Selected: &sel, UserName: "Ann"}, func() { Text(c, "page") })
	})
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.A11yFind(a11y.RoleNavigation, ZhCN.A11y.Navigation); !ok {
		t.Fatal("top layout header is not the TopNavHeader navigation landmark")
	}
}

// ---------------------------------------------------------------------------
// GridContent (es/layout/components/GridContent: -wide maxWidth 1152 only
// when contentWidth is Fixed and the layout is top).

func TestGridContentWideOnlyInTopLayout(t *testing.T) {
	for _, lay := range []string{"top", "side", "mix"} {
		sel := "home"
		var inner *Box
		b4cDriver(theme.AntDesign(), 1600, 600, func(c *Context) {
			ProLayout(c, ProLayoutConfig{Title: "T", Layout: lay, ContentWidth: "Fixed", Selected: &sel, Menu: b4cMenu, NoPadding: true}, func() {
				inner = GridContent(c).Body(func() { Text(c, "x") })
			})
		})
		w := inner.n.abs.W
		if lay == "top" && (w > 1152.5 || w < 1100) {
			t.Fatalf("%s: Fixed width %v, want 1152", lay, w)
		}
		if lay != "top" && w <= 1152.5 {
			t.Fatalf("%s: Fixed should not narrow the content outside the top layout (width %v)", lay, w)
		}
	}
}

// ---------------------------------------------------------------------------
// SettingDrawer (es/layout/components/SettingDrawer/index.js changeSetting,
// colorList, Drawer size 300; LayoutChange.js disabled rules).

func TestSettingDrawerV3Rules(t *testing.T) {
	if n := len(ProSettingsPresetColors); n != 9 {
		t.Fatalf("preset colors = %d, want 9 (techBlue … purple)", n)
	}
	if ProSettingsPresetColors[1] != theme.Hex("#1890ff") {
		t.Fatalf("daybreak = %v", ProSettingsPresetColors[1])
	}
	s := DefaultProSettings()
	s.NavTheme = "dark"
	open := true
	d := b4cDriver(theme.AntDesign(), 1200, 800, func(c *Context) {
		ConfigProvider(c).LocaleCode("en-US").Body(func() { SettingDrawer(c, &s).Open(&open).Show() })
	})
	d.EnableAccessibility()
	d.Settle()
	tx := EnUS.SettingDrawer
	press := func(role a11y.Role, name string) {
		t.Helper()
		n, ok := d.A11yFind(role, name)
		if !ok {
			t.Fatalf("%q missing", name)
		}
		if err := d.PerformAction(n.ID, a11y.Press, ""); err != nil {
			t.Fatal(err)
		}
		d.Settle()
	}
	press(a11y.RoleRadio, tx.Top)
	if s.Layout != "top" || s.ContentWidth != "Fixed" {
		t.Fatalf("top layout should switch to Fixed width: %+v", s)
	}
	if n, ok := d.A11yFind(a11y.RoleSwitch, tx.FixSiderbar); !ok || n.States&a11y.Disabled == 0 {
		t.Fatal("fixed sider switch should be disabled in the top layout")
	}
	press(a11y.RoleRadio, tx.Mix)
	if s.Layout != "mix" || s.ContentWidth != "Fluid" || s.NavTheme != "light" {
		t.Fatalf("mix layout: %+v", s)
	}
	dr, ok := d.A11yFind(a11y.RoleDialog, tx.Title)
	if !ok {
		t.Fatal("drawer dialog missing")
	}
	if w := dr.Bounds.W; w < 299 || w > 301 {
		t.Fatalf("drawer width %v, want 300", w)
	}
}

// ---------------------------------------------------------------------------
// ProList (es/list/ListView.js, Item.js, style/index.js)

func b4cListMetas() ProListMetas[pro3Row] {
	return ProListMetas[pro3Row]{
		Title:       func(r pro3Row) string { return r.name },
		Description: func(r pro3Row) string { return r.desc },
		Content:     func(c *Context, r pro3Row) { Text(c, "content-"+r.id) },
		Actions:     func(c *Context, r pro3Row) { Link(c, "open-"+r.id) },
	}
}

func TestProListV3ExpandDefaults(t *testing.T) {
	b4cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		rows := pro3Rows(3)
		var sel []string
		d := b4cDriver(th(), 900, 700, func(c *Context) {
			ProList(c, rows, b4cListMetas()).RowKey(func(r pro3Row) string { return r.id }).Selection(&sel).
				Expandable(func(c *Context, r pro3Row) { Text(c, "detail-"+r.id) }).Show()
		})
		// defaultExpandAllRows = true.
		for _, s := range []string{"detail-k0", "detail-k2", "Desc 1", "content-k1"} {
			if !pro3HasText(d, s) {
				t.Fatalf("%q should be visible (rows expanded by default)", s)
			}
		}
		// Checkbox, then the expand icon, then the meta (Item.js header-option).
		cb := pro3CheckboxLeftOf(d, "Item 0")
		ex, ok := d.Find(ZhCN.Collapse)
		if !ok || cb.Empty() || !(cb.X < ex.X) {
			t.Fatalf("checkbox %v should precede the expand icon %v", cb, ex)
		}
		// Content sits under the title (-row-content marginBlockStart).
		ti, _ := d.FindText("Item 0")
		ct, _ := d.FindText("content-k0")
		if !(ct.Y > ti.Bottom()) {
			t.Fatalf("content %v should be below the title %v", ct, ti)
		}
		clickNth(t, d, ZhCN.Collapse, 0)
		d.Settle()
		// Collapsed: description, content and the expanded row are hidden.
		for _, s := range []string{"detail-k0", "Desc 0", "content-k0"} {
			if pro3HasText(d, s) {
				t.Fatalf("%q visible on a collapsed row", s)
			}
		}
		if !pro3HasText(d, "detail-k1") {
			t.Fatal("other rows should stay expanded")
		}
	})
}

func TestProListV3ExpandOptions(t *testing.T) {
	rows := pro3Rows(3)
	keys := []string{"k1"}
	d := b4cDriver(theme.AntDesign(), 900, 700, func(c *Context) {
		ProList(c, rows, b4cListMetas()).RowKey(func(r pro3Row) string { return r.id }).
			Expandable(func(c *Context, r pro3Row) { Text(c, "detail-"+r.id) }).
			ExpandedRowKeys(&keys).ExpandRowByClick().
			RowExpandable(func(r pro3Row) bool { return r.id != "k2" }).Show()
	})
	if pro3HasText(d, "detail-k0") || !pro3HasText(d, "detail-k1") {
		t.Fatal("ExpandedRowKeys should control the expanded rows")
	}
	// k2 cannot expand: no expand icon, its content always shows.
	if n := b4cHits(d, ZhCN.Expand) + b4cHits(d, ZhCN.Collapse); n != 2 {
		t.Fatalf("expand icons = %d, want 2", n)
	}
	if !pro3HasText(d, "content-k2") {
		t.Fatal("a row that cannot expand shows its content")
	}
	r, _ := d.FindText("Item 0")
	d.Click(r.Center())
	d.Settle()
	if len(keys) != 2 || !pro3HasText(d, "detail-k0") {
		t.Fatalf("ExpandRowByClick: keys %v", keys)
	}
}

func TestProListV3LayoutStates(t *testing.T) {
	b4cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		rows := pro3Rows(5)
		var sel []string
		loading := true
		page := 0
		var p *ProListW[pro3Row]
		d := b4cDriver(th(), 900, 900, func(c *Context) {
			p = ProList(c, rows, b4cListMetas()).RowKey(func(r pro3Row) string { return r.id }).
				ItemLayout("vertical").Selection(&sel).Pagination(2).Loading(loading).Show()
			page = p.Page()
		})
		d.EnableAccessibility()
		d.Settle()
		if n, ok := d.A11yFind(a11y.RoleStatus, ZhCN.A11y.Loading); !ok || n.States&a11y.Busy == 0 {
			t.Fatal("loading list should expose a busy status")
		}
		loading = false
		d.Settle()
		// Vertical item layout: actions under the content.
		ct, _ := d.FindText("content-k0")
		ac, _ := d.FindText("open-k0")
		if !(ac.Y > ct.Bottom()-1) {
			t.Fatalf("vertical layout: actions %v should be under the content %v", ac, ct)
		}
		if err := d.ClickLabel(ZhCN.NextPage); err != nil {
			t.Fatal(err)
		}
		d.Settle()
		if page != 2 || !pro3HasText(d, "Item 2") || pro3HasText(d, "Item 0") {
			t.Fatalf("page %d", page)
		}
		d.Click(pro3CheckboxLeftOf(d, "Item 3").Center())
		d.Settle()
		if len(sel) != 1 || sel[0] != "k3" {
			t.Fatalf("selection %v", sel)
		}
		items := d.A11yFindAll(a11y.RoleListItem, "Item 3")
		if len(items) == 0 || items[0].States&a11y.Selected == 0 {
			t.Fatal("selected item should carry the Selected state")
		}
	})
}

func TestProListV3EmptyAndGrid(t *testing.T) {
	b4cThemes(t, func(t *testing.T, th func() *theme.Theme) {
		d := b4cDriver(th(), 600, 300, func(c *Context) {
			ProList(c, []pro3Row(nil), b4cListMetas()).Show()
		})
		if !pro3HasText(d, ZhCN.NoData) {
			t.Fatal("empty ProList should show the empty text")
		}
		rows := pro3Rows(4)
		d = b4cDriver(th(), 900, 700, func(c *Context) {
			ProList(c, rows, b4cListMetas()).Grid(2).RowKey(func(r pro3Row) string { return r.id }).Show()
		})
		t0, _ := d.FindText("Item 0")
		t1, _ := d.FindText("Item 1")
		a0, _ := d.FindText("open-k0")
		if !(t1.X > t0.X+100) {
			t.Fatalf("grid columns %v %v", t0, t1)
		}
		// Card mode: actions are the card's header extra (CheckCard extra).
		if !(a0.Y < t0.Bottom()+4 && a0.X > t0.X) {
			t.Fatalf("card actions %v should sit in the header row of %v", a0, t0)
		}
	})
}

// b4cHits counts the hit regions labelled label in the last frame.
func b4cHits(d *Driver, label string) int {
	n := 0
	for _, h := range d.r.c.hits {
		if h.label == label {
			n++
		}
	}
	return n
}

func TestProLayoutV3Defaults(t *testing.T) {
	s := DefaultProSettings()
	if s.NavTheme != "light" || s.FixedHeader {
		t.Fatalf("v3 defaults: %+v", s)
	}
	var ri RouteInfo
	b4cDriver(theme.AntDesign(), 1200, 600, func(c *Context) {
		ProLayout(c, ProLayoutConfig{Title: "Defaults", Menu: b4cMenu}, func() { ri = RouteContext(c) })
	})
	if ri.FixedHeader {
		t.Fatal("fixedHeader defaults to false in Pro v3")
	}
}

func TestProLayoutNewEntriesDefaultsIsolation(t *testing.T) {
	var inside, outside *PageLoadingW
	var bc *ProBreadcrumbW
	var top *TopNavHeaderW
	b4cDriver(theme.AntDesign(), 800, 500, func(c *Context) {
		ConfigProvider(c).Defaults("PageLoading", Props{"spinning": false, "size": Small, "description": "Default loading"}).
			Defaults("ProBreadcrumb", Props{"separator": ">"}).Defaults("TopNavHeader", Props{"title": "Default title", "userName": "Default user", "contentWidth": "Fixed"}).Body(func() {
			inside = PageLoading(c)
			inside.Show()
			bc = ProBreadcrumb(c).Items([]BreadcrumbItem{{Label: "A"}, {Label: "B"}}).Show()
			top = TopNavHeader(c, nil, nil).Show()
		})
		outside = PageLoading(c)
	})
	if inside.spinning || inside.size != Small || inside.description != "Default loading" {
		t.Fatalf("PageLoading defaults: %+v", inside)
	}
	if !outside.spinning || outside.size != Large || outside.description != "" {
		t.Fatal("PageLoading defaults leaked out of scope")
	}
	if bc.sep != ">" || top.cfg.Title != "Default title" || top.cfg.UserName != "Default user" || top.cfg.ContentWidth != "Fixed" {
		t.Fatal("new entries ignored defaults")
	}
}

func TestTopNavHeaderOverflowNavigation(t *testing.T) {
	sel := "home"
	d := b4cDriver(theme.AntDesign(), 480, 400, func(c *Context) {
		TopNavHeader(c, []MenuItem{{Key: "home", Label: "Home"}, {Key: "projects", Label: "Projects"}, {Key: "reports", Label: "Reports"}, {Key: "billing", Label: "Billing"}}, &sel).Title("Team").UserName("Lily").Show()
	})
	d.EnableAccessibility()
	d.Settle()
	more, ok := d.A11yFind(a11y.RoleMenuItem, ZhCN.More)
	if !ok {
		t.Fatal("overflow control missing")
	}
	if err := d.PerformAction(more.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	billing, ok := d.A11yFind(a11y.RoleMenuItem, "Billing")
	if !ok {
		t.Fatal("folded navigation item unreachable")
	}
	if err := d.PerformAction(billing.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if sel != "billing" {
		t.Fatalf("overflow navigation selected %q", sel)
	}
}

func TestProLayoutExplicitFixedHeader(t *testing.T) {
	s := DefaultProSettings()
	s.FixedHeader = true
	var got RouteInfo
	b4cDriver(theme.AntDesign(), 800, 400, func(c *Context) {
		cfg := ProLayoutConfig{Title: "Fixed"}
		s.Apply(&cfg)
		ProLayout(c, cfg, func() { got = RouteContext(c) })
	})
	if !got.FixedHeader {
		t.Fatal("ProSettings.FixedHeader(true) lost to the default false")
	}
}

func TestProLayoutPageContainerOwnsHeader(t *testing.T) {
	for _, mode := range []string{"top", "mix"} {
		t.Run(mode, func(t *testing.T) {
			sel := "monitor"
			container := true
			d := b4cDriver(theme.AntDesign(), 1200, 700, func(c *Context) {
				ProLayout(c, ProLayoutConfig{Title: "Shell", Layout: mode, Menu: b4cMenu, Selected: &sel, AutoBreadcrumb: true, PageTitle: "Page title"}, func() {
					if container {
						PageContainer(c, "").Body(func() { Text(c, "body") })
					} else {
						Text(c, "body")
					}
				})
			})
			d.EnableAccessibility()
			d.Settle()
			if n := len(d.A11yFindAll(a11y.RoleHeading, "Page title")); n != 1 {
				t.Fatalf("page headings %d, want 1", n)
			}
			if n := len(d.A11yFindAll(a11y.RoleNavigation, ZhCN.A11y.Breadcrumb)); n != 1 {
				t.Fatalf("breadcrumb landmarks %d, want 1", n)
			}
			container = false
			d.Settle()
			if n := len(d.A11yFindAll(a11y.RoleHeading, "Page title")); n != 1 {
				t.Fatalf("bare content page headings %d", n)
			}
			if n := len(d.A11yFindAll(a11y.RoleNavigation, ZhCN.A11y.Breadcrumb)); n != 1 {
				t.Fatalf("bare content breadcrumbs %d", n)
			}
		})
	}
}

func TestProListV3ExpansionInitializesOnce(t *testing.T) {
	rows := pro3Rows(1)
	def := true
	d := b4cDriver(theme.AntDesign(), 900, 700, func(c *Context) {
		ProList(c, rows, b4cListMetas()).RowKey(func(r pro3Row) string { return r.id }).DefaultExpandAllRows(def).
			Expandable(func(c *Context, r pro3Row) { Text(c, "detail-"+r.id) }).Show()
	})
	if !pro3HasText(d, "detail-k0") {
		t.Fatal("initial row not expanded")
	}
	rows = pro3Rows(2)
	d.Settle()
	if pro3HasText(d, "detail-k1") {
		t.Fatal("appended row inherited the default expansion after mount")
	}
	def = false
	d.Settle()
	if !pro3HasText(d, "detail-k0") {
		t.Fatal("changing the default reset a mounted row")
	}
	rows = []pro3Row{{id: "replacement", name: "Replacement"}}
	def = true
	d.Settle()
	if pro3HasText(d, "detail-replacement") {
		t.Fatal("replaced data reapplied the expansion default")
	}
}

func TestProLayoutSharedTokenConsumption(t *testing.T) {
	th := theme.AntDesign()
	th.Components.Layout.ProSiderWidth = 311
	th.Components.Layout.PagePaddingInline = 47
	th.Components.Layout.PagePaddingBlock = 37
	th.Components.Layout.PageHeaderPaddingTop = 9
	th.Components.Layout.PageHeaderPaddingBottom = 19
	th.Components.FooterToolbar.MinHeight = 71
	th.Components.FooterToolbar.PaddingInline = 29
	var page *PageContainerW
	var bar *FooterToolbarW
	d := b4cDriver(th, 1200, 700, func(c *Context) {
		ProLayout(c, ProLayoutConfig{Title: "Tokens"}, func() {
			page = PageContainer(c, "Token page").Body(func() { Text(c, "Token body") })
			bar = FooterToolbar(c).Body(func() { Button(c, "Save") })
		})
	})
	d.EnableAccessibility()
	d.Settle()
	sider, ok := d.A11yFind(a11y.RoleNavigation, ZhCN.A11y.Navigation)
	if !ok || absf(sider.Bounds.W-311) > .5 {
		t.Fatalf("Pro sider width %v", sider)
	}
	if page.header.pad == nil || page.header.pad.Left != 47 || page.header.pad.Top != 9 || page.header.pad.Bottom != 19 {
		t.Fatalf("page header padding %v", page.header.pad)
	}
	if absf(bar.box.n.abs.H-71) > .5 || bar.box.n.ln.Padding.Left != 29 {
		t.Fatalf("footer toolbar rect %v / pad %v", bar.box.n.abs, bar.box.n.ln.Padding)
	}
}

func TestFooterToolbarDefaultFixed(t *testing.T) {
	var fixed, flow *FooterToolbarW
	b4cDriver(theme.AntDesign(), 800, 400, func(c *Context) {
		fixed = FooterToolbar(c).Body(func() { Button(c, "Fixed") })
		flow = FooterToolbar(c).Fixed(false).Body(func() { Button(c, "Flow") })
	})
	if !fixed.fixed || fixed.affix != nil {
		t.Fatal("FooterToolbar should be fixed by default")
	}
	if flow.fixed || flow.sticky || flow.affix != nil {
		t.Fatal("Fixed(false) should opt into normal flow")
	}
}

func TestAppMaxCountUsesSharedCloseLifecycle(t *testing.T) {
	for _, plain := range []bool{false, true} {
		name := "api"
		if plain {
			name = "plain"
		}
		t.Run(name, func(t *testing.T) {
			closedMsg, closedNote := 0, 0
			d := b4cDriver(theme.AntDesign(), 800, 500, func(c *Context) {
				App(c).Message(AppMessageConfig{MaxCount: 1}).Notification(AppNotificationConfig{MaxCount: 1}).Body(func() {
					if Button(c, "Open feedback").Clicked() {
						if plain {
							Message(c, "info", "Message").Duration(0).OnClose(func() { closedMsg++ })
							Notification(c, "info", "Notification").Duration(0).OnClose(func() { closedNote++ })
						} else {
							app := UseApp(c)
							app.Message("info", "Message").Duration(0).OnClose(func() { closedMsg++ })
							app.Notification("info", "Notification").Duration(0).OnClose(func() { closedNote++ })
						}
					}
				})
			})
			for range 2 {
				if err := d.ClickLabel("Open feedback"); err != nil {
					t.Fatal(err)
				}
				d.Settle()
			}
			if closedMsg != 1 || closedNote != 1 {
				t.Fatalf("evicted feedback OnClose = %d / %d, want 1 / 1", closedMsg, closedNote)
			}
			if len(d.r.c.toasts().msgs) != 1 || len(d.r.c.navNotes().list) != 1 {
				t.Fatal("maxCount should retain exactly one feedback of each kind")
			}
		})
	}
}

func TestFooterToolbarFixedBounds(t *testing.T) {
	for _, long := range []bool{false, true} {
		var ft *FooterToolbarW
		d := b4cDriver(theme.AntDesign(), 800, 500, func(c *Context) {
			Scroll(c, func() {
				Text(c, "before")
				if long {
					Gap(c, 1200)
				}
				ft = FooterToolbar(c).Body(func() { Button(c, "Fixed action") })
				Gap(c, 800)
			}, layout.Grow(1))
		})
		check := func() {
			t.Helper()
			if b := ft.box.n.abs.Bottom(); absf(b-500) > .5 {
				t.Fatalf("long=%v: fixed bottom %v, want500", long, b)
			}
		}
		check()
		d.Scroll(geom.Pt(300, 200), 600)
		d.Settle()
		check()
	}
}

func TestFooterToolbarFixedTargetBounds(t *testing.T) {
	var target ID
	var ft *FooterToolbarW
	d := b4cDriver(theme.AntDesign(), 800, 600, func(c *Context) {
		Gap(c, 70)
		sb := Scroll(c, func() {
			Text(c, "target content")
			ft = FooterToolbar(c).Target(target).Body(func() { Button(c, "Target action") })
			Gap(c, 1200)
		}, layout.H(300))
		target = sb.ID()
	})
	d.Settle()
	check := func() {
		t.Helper()
		view, ok := d.r.c.RectOf(target)
		if !ok {
			t.Fatal("target missing")
		}
		if b := ft.box.n.abs.Bottom(); absf(b-view.Bottom()) > .5 {
			t.Fatalf("target bottom %v, bar bottom %v", view.Bottom(), b)
		}
	}
	check()
	d.Scroll(geom.Pt(300, 200), 600)
	d.Settle()
	check()
}
