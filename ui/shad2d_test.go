package ui

import (
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Tests of the shadcn look of the Pro / layout components, their
// ConfigProvider defaults and the fourth Pro batch (ProTable params,
// polling, columns state, alert render; SettingDrawer, ProFormGroup,
// RouteContext, GridContent, App).

func shad2dDriver(th *theme.Theme, w, h float32, page func(c *Context)) *Driver {
	return pro2Driver(th, w, h, page)
}

func TestShad2dProCardShadcnLook(t *testing.T) {
	th := theme.Shadcn()
	var card *ProCardW
	d := shad2dDriver(th, 600, 300, func(c *Context) {
		card = ProCard(c, "Revenue").SubTitle("This month").Extra(func() { Button(c, "Act").Small() }).
			Layout(layout.W(400)).Body(func() { Text(c, "body") })
	})
	if got, want := card.box.n.radii, shad2dCardRadius(th); got.TL != want {
		t.Fatalf("shadcn ProCard radius %v, want rounded-xl %v", got.TL, want)
	}
	if !card.bordered && card.borderSet {
		t.Fatal("shadcn ProCard should be outlined by default")
	}
	cr := card.box.n.abs
	act := pro2MustFind(t, d, "Act")
	if right := cr.Right() - 24; act.Right() < right-1 || act.Right() > right+1 {
		t.Fatalf("action not at the right padding edge: action right %v, card content right %v", act.Right(), right)
	}
	title := pro2MustFind(t, d, "Revenue")
	sub := pro2MustFind(t, d, "This month")
	if sub.Y <= title.Y || sub.X != title.X {
		t.Fatalf("CardDescription should sit under the title: title %v sub %v", title, sub)
	}
	if act.Y > title.Y+2 {
		t.Fatalf("action should be top aligned with a description: action %v title %v", act, title)
	}
}

func TestShad2dProCardAntUnchanged(t *testing.T) {
	th := theme.AntDesign()
	var card *ProCardW
	shad2dDriver(th, 600, 300, func(c *Context) {
		card = ProCard(c, "Revenue").SubTitle("sub").Body(func() { Text(c, "body") })
	})
	if card.box.n.radii.TL != th.RadiusLG {
		t.Fatalf("Ant ProCard radius changed: %v", card.box.n.radii.TL)
	}
}

func TestShad2dProCardDefaults(t *testing.T) {
	var card *ProCardW
	shad2dDriver(theme.AntDesign(), 600, 300, func(c *Context) {
		ConfigProvider(c).Defaults("ProCard", Props{"size": "small", "bordered": true, "headerBordered": true}).Body(func() {
			card = ProCard(c, "T").Body(func() {})
		})
	})
	if !card.small || !card.bordered || !card.headerBordered {
		t.Fatalf("defaults not applied: small=%v bordered=%v headerBordered=%v", card.small, card.bordered, card.headerBordered)
	}
	shad2dDriver(theme.AntDesign(), 600, 300, func(c *Context) {
		ConfigProvider(c).Defaults("ProCard", Props{"bordered": true}).Body(func() {
			card = ProCard(c, "T").Borderless().Body(func() {})
		})
	})
	if card.bordered {
		t.Fatal("explicit Borderless must win over the default")
	}
}

type shad2dRow struct {
	key  string
	name string
}

func shad2dRows(n int) []shad2dRow {
	out := make([]shad2dRow, n)
	for i := range out {
		out[i] = shad2dRow{key: fmt.Sprint("k", i), name: fmt.Sprint("Row ", i)}
	}
	return out
}

func shad2dServer(all []shad2dRow, req ProRequest) ProResult[shad2dRow] {
	size := req.PageSize
	if size <= 0 {
		size = len(all)
	}
	a := min(len(all), (req.Page-1)*size)
	b := min(len(all), a+size)
	return ProResult[shad2dRow]{Rows: all[a:b], Total: len(all)}
}

func shad2dCols() []ProColumn[shad2dRow] {
	return []ProColumn[shad2dRow]{{Key: "name", Title: "Name", Value: func(r shad2dRow) any { return r.name }, HideInSearch: true}}
}

func TestShad2dProTableShadcnPager(t *testing.T) {
	all := shad2dRows(25)
	var reqs []ProRequest
	d := shad2dDriver(theme.Shadcn(), 900, 700, func(c *Context) {
		ConfigProvider(c).LocaleCode("en-US").Body(func() {
			ProTable(c, shad2dCols()).NoSearch().Request(func(req ProRequest, done func(ProResult[shad2dRow])) {
				reqs = append(reqs, req)
				done(shad2dServer(all, req))
			}).Show()
		})
	})
	if _, ok := d.FindText("Row 0"); !ok {
		t.Fatal("first page not shown")
	}
	if err := d.ClickLabel(EnUS.NextPage); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if last := reqs[len(reqs)-1]; last.Page != 2 {
		t.Fatalf("Next should request page 2, got %d", last.Page)
	}
	if _, ok := d.FindText("Row 10"); !ok {
		t.Fatal("second page not shown")
	}
}

func TestShad2dProTableParamsAndPolling(t *testing.T) {
	all := shad2dRows(30)
	status := "a"
	n := 0
	var last ProRequest
	var pt *ProTableW[shad2dRow]
	d := shad2dDriver(theme.AntDesign(), 900, 700, func(c *Context) {
		pt = ProTable(c, shad2dCols()).NoSearch().Params(map[string]string{"status": status}).Polling(2 * time.Second).
			Request(func(req ProRequest, done func(ProResult[shad2dRow])) {
				n++
				last = req
				done(shad2dServer(all, req))
			})
		pt.Show()
	})
	if n != 1 || last.Params["status"] != "a" {
		t.Fatalf("first request: n=%d params=%v", n, last.Params)
	}
	status = "b"
	d.Settle()
	if n != 2 || last.Params["status"] != "b" || last.Page != 1 {
		t.Fatalf("a Params change should request again: n=%d req=%+v", n, last)
	}
	if m, ok := last.ExtParams.(map[string]string); !ok || m["status"] != "b" {
		t.Fatalf("ExtParams = %#v", last.ExtParams)
	}
	d.Advance(2100 * time.Millisecond)
	d.Settle()
	if n != 3 {
		t.Fatalf("polling should request after the interval, n=%d", n)
	}
	pt.PausePolling()
	d.Advance(5 * time.Second)
	d.Settle()
	if n != 3 {
		t.Fatalf("paused polling requested again, n=%d", n)
	}
	pt.ResumePolling()
	d.Settle()
	if n != 4 {
		t.Fatalf("resumed polling (interval long past) should request, n=%d", n)
	}
}

type shad2dMapStore map[string]TableColumnState

func (m shad2dMapStore) LoadColumnsState(k string) (TableColumnState, bool) {
	s, ok := m[k]
	return s, ok
}
func (m shad2dMapStore) SaveColumnsState(k string, s TableColumnState) { m[k] = s }

func TestShad2dProTableColumnsState(t *testing.T) {
	all := shad2dRows(5)
	cols := []ProColumn[shad2dRow]{
		{Key: "name", Title: "Name", Value: func(r shad2dRow) any { return r.name }, HideInSearch: true},
		{Key: "key", Title: "KeyCol", Value: func(r shad2dRow) any { return r.key }, HideInSearch: true},
	}
	store := shad2dMapStore{"t": {Hidden: map[string]bool{"key": true}}}
	var bound TableColumnState
	changes := 0
	var pt *ProTableW[shad2dRow]
	d := shad2dDriver(theme.AntDesign(), 900, 500, func(c *Context) {
		pt = ProTable(c, cols).NoSearch().ColumnsState(&bound).PersistColumnsState("t", store).
			OnColumnsStateChange(func(TableColumnState) { changes++ }).
			Request(func(req ProRequest, done func(ProResult[shad2dRow])) { done(shad2dServer(all, req)) })
		pt.Show()
	})
	if !bound.Hidden["key"] {
		t.Fatal("persisted state not loaded into the bound state")
	}
	if _, ok := d.FindText("KeyCol"); ok {
		t.Fatal("hidden column still shown")
	}
	if changes != 0 {
		t.Fatalf("loading is not a change, got %d", changes)
	}
	bound.Hidden["key"] = false
	d.Settle()
	if changes != 1 || store["t"].Hidden["key"] {
		t.Fatalf("change not reported / saved: changes=%d store=%v", changes, store["t"])
	}
	if _, ok := d.FindText("KeyCol"); !ok {
		t.Fatal("column not shown after the bound state changed")
	}
}

func TestShad2dProTableAlertRender(t *testing.T) {
	for name, th := range map[string]*theme.Theme{"ant": theme.AntDesign(), "shadcn": theme.Shadcn()} {
		t.Run(name, func(t *testing.T) {
			all := shad2dRows(5)
			sel := []string{"k1", "k2"}
			var got []string
			d := shad2dDriver(th, 900, 500, func(c *Context) {
				ProTable(c, shad2dCols()).NoSearch().RowKey(func(r shad2dRow) string { return r.key }).Selection(&sel).
					TableAlertRender(func(keys []string, clear func()) { got = keys; Text(c, "custom-alert") }).
					TableAlertOptionRender(func(keys []string, clear func()) {
						if Button(c, "wipe").Clicked() {
							clear()
						}
					}).
					Request(func(req ProRequest, done func(ProResult[shad2dRow])) { done(shad2dServer(all, req)) }).Show()
			})
			if len(got) != 2 {
				t.Fatalf("alert render keys %v", got)
			}
			if _, ok := d.FindText("custom-alert"); !ok {
				t.Fatal("custom alert not shown")
			}
			if err := d.ClickLabel("wipe"); err != nil {
				t.Fatal(err)
			}
			d.Settle()
			if len(sel) != 0 {
				t.Fatalf("clear() should empty the selection, got %v", sel)
			}
		})
	}
}

func TestShad2dProTableDefaults(t *testing.T) {
	var pt *ProTableW[shad2dRow]
	var req ProRequest
	shad2dDriver(theme.AntDesign(), 900, 500, func(c *Context) {
		ConfigProvider(c).Defaults("ProTable", Props{"search": false, "pagination": false, "options": false, "headerTitle": "Rules"}).Body(func() {
			pt = ProTable(c, shad2dCols()).Request(func(r ProRequest, done func(ProResult[shad2dRow])) {
				req = r
				done(shad2dServer(shad2dRows(3), r))
			})
			pt.Show()
		})
	})
	if pt.search || !pt.noPager || !pt.noOptions || pt.title != "Rules" {
		t.Fatalf("defaults not applied: search=%v noPager=%v noOptions=%v title=%q", pt.search, pt.noPager, pt.noOptions, pt.title)
	}
	if req.PageSize != 0 {
		t.Fatalf("pagination=false should request every row, PageSize=%d", req.PageSize)
	}
}

func TestShad2dRouteContextGridContent(t *testing.T) {
	var in, out RouteInfo
	var inner *Box
	sel := "a"
	shad2dDriver(theme.AntDesign(), 1600, 600, func(c *Context) {
		out = RouteContext(c)
		ProLayout(c, ProLayoutConfig{Title: "T", Layout: "top", ContentWidth: "Fixed", Selected: &sel,
			Menu: []MenuItem{{Key: "a", Label: "A"}}}, func() {
			in = RouteContext(c)
			inner = GridContent(c).Body(func() { Text(c, "x") })
		})
	})
	if out.InLayout {
		t.Fatal("RouteContext outside a ProLayout should report InLayout=false")
	}
	if !in.InLayout || in.Layout != "top" || in.ContentWidth != "Fixed" || in.Selected != "a" {
		t.Fatalf("RouteContext inside: %+v", in)
	}
	if w := inner.n.abs.W; w > GridContentFixedWidth+0.5 || w <= 0 {
		t.Fatalf("Fixed GridContent width %v", w)
	}
}

func TestShad2dProLayoutShadcnSidebar(t *testing.T) {
	collapsed := false
	sel := "a"
	d := shad2dDriver(theme.Shadcn(), 1200, 600, func(c *Context) {
		ProLayout(c, ProLayoutConfig{Title: "Acme", UserName: "Ann", Collapsed: &collapsed, Selected: &sel,
			Menu: []MenuItem{{Key: "a", Label: "Alpha"}}}, func() { Text(c, "page") })
	})
	d.EnableAccessibility()
	d.Settle()
	if _, ok := d.FindText("Ann"); !ok {
		t.Fatal("NavUser row missing from the sidebar footer")
	}
	n, ok := d.A11yFind(a11y.RoleButton, ZhCN.CollapseMenu)
	if !ok {
		t.Fatal("sidebar trigger has no accessible button")
	}
	if err := d.PerformAction(n.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if !collapsed {
		t.Fatal("trigger should collapse the sidebar")
	}
	if _, ok := d.FindText("Acme"); ok {
		t.Fatal("collapsed icon rail should hide the title")
	}
}

func TestShad2dSettingDrawer(t *testing.T) {
	s := DefaultProSettings()
	open := true
	d := shad2dDriver(theme.AntDesign(), 1200, 800, func(c *Context) {
		ConfigProvider(c).LocaleCode("en-US").Body(func() {
			SettingDrawer(c, &s).Open(&open).Show()
		})
	})
	d.EnableAccessibility()
	d.Settle()
	tx := EnUS.SettingDrawer
	n, ok := d.A11yFind(a11y.RoleRadio, tx.Top)
	if !ok {
		t.Fatal("layout radio missing")
	}
	if err := d.PerformAction(n.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if s.Layout != "top" {
		t.Fatalf("layout = %q", s.Layout)
	}
	var cfg ProLayoutConfig
	s.FixedHeader = false
	s.Apply(&cfg)
	if cfg.Layout != "top" || !cfg.NoFixedHeader {
		t.Fatalf("Apply: %+v", cfg)
	}
	if rep := d.AuditNoContrast(); rep.MissingInteractiveNames() > 0 {
		t.Fatalf("a11y audit: %v", rep)
	}
}

func TestShad2dSettingTextsComplete(t *testing.T) {
	for _, l := range []*Locale{ZhCN, EnUS, ZhTW, Ja} {
		tx := l.SettingDrawer
		for _, s := range []string{tx.Open, tx.Title, tx.NavTheme, tx.Light, tx.Dark, tx.RealDark, tx.PrimaryColor, tx.Layout,
			tx.Side, tx.Top, tx.Mix, tx.ContentWidth, tx.Fluid, tx.Fixed, tx.FixedHeader, tx.FixSiderbar, tx.ColorWeak, tx.Other, tx.Compact, tx.ColorFmt} {
			if s == "" {
				t.Fatalf("%s: empty SettingDrawer text", l.Code)
			}
		}
	}
	if l, _ := LocaleByCode("zh-HK"); l.SettingDrawer.Title != ZhTW.SettingDrawer.Title {
		t.Fatal("zh-HK should use the traditional pack")
	}
	// A partial translation falls back to English per field.
	l := &Locale{Code: "fr-FR", SettingDrawer: SettingDrawerLocale{Title: "Réglages"}}
	FillLocale(l, EnUS)
	if l.SettingDrawer.Title != "Réglages" || l.SettingDrawer.Open != EnUS.SettingDrawer.Open {
		t.Fatalf("FillLocale: %+v", l.SettingDrawer)
	}
}

func TestShad2dProFormGroup(t *testing.T) {
	vals := ProValues{}
	folded := false
	fields := []ProField{{Key: "a", Label: "FieldA", Group: "G1"}, {Key: "b", Label: "FieldB", Group: "G1"}, {Key: "c", Label: "FieldC"}}
	d := shad2dDriver(theme.AntDesign(), 900, 600, func(c *Context) {
		ProForm(c, fields).Values(&vals).Show()
		ProForm(c, nil).NoSubmitter().Children(func(f *FormW) {
			ProFormGroup(c, "Folding").Collapsed(&folded).Columns(2).Body(func() {
				ProFormItems(c, f, []ProField{{Key: "x", Label: "FieldX"}, {Key: "y", Label: "FieldY"}}, vals)
			})
		}).Show()
	})
	if _, ok := d.FindText("G1"); !ok {
		t.Fatal("group title from ProField.Group missing")
	}
	x, _ := d.FindText("FieldX")
	y, _ := d.FindText("FieldY")
	if x.Empty() || y.Empty() || y.Y > x.Y+1 || y.X <= x.X {
		t.Fatalf("Columns(2) should lay items side by side: %v %v", x, y)
	}
	fr, ok := d.FindText("Folding")
	if !ok {
		t.Fatal("group title missing")
	}
	d.Click(fr.Center())
	d.Settle()
	if !folded {
		t.Fatal("collapsible group did not fold")
	}
	if _, ok := d.FindText("FieldX"); ok {
		t.Fatal("folded group still shows its items")
	}
}

func TestShad2dApp(t *testing.T) {
	var api, outside AppAPI
	d := shad2dDriver(theme.AntDesign(), 600, 400, func(c *Context) {
		outside = UseApp(c)
		App(c).Message(AppMessageConfig{MaxCount: 2}).Body(func() {
			api = UseApp(c)
			if Button(c, "msg").Clicked() {
				api.Message("info", "hello")
			}
		})
	})
	if outside.InApp() || !api.InApp() {
		t.Fatalf("InApp outside=%v inside=%v", outside.InApp(), api.InApp())
	}
	for i := 0; i < 4; i++ {
		if err := d.ClickLabel("msg"); err != nil {
			t.Fatal(err)
		}
		d.Settle()
	}
	if n := len(d.r.c.toasts().msgs); n != 2 {
		t.Fatalf("MaxCount 2: %d messages on screen", n)
	}
}

func TestShad2dLoginFormShadcnLabels(t *testing.T) {
	d := shad2dDriver(theme.Shadcn(), 900, 700, func(c *Context) {
		ConfigProvider(c).LocaleCode("en-US").Body(func() {
			LoginForm(c).Title("Acme").SubTitle("Sign in").AccountOnly().Actions(func() { Button(c, "G") }).Show()
		})
	})
	if _, ok := d.FindText(EnUS.LoginForm.Username); !ok {
		t.Fatal("shadcn login fields should be labelled")
	}
	if _, ok := d.FindText(EnUS.LoginForm.Others); !ok {
		t.Fatal("'or continue with' separator missing")
	}
}

func TestShad2dLightFilterShadcn(t *testing.T) {
	vals := ProValues{"s": "a"}
	d := shad2dDriver(theme.Shadcn(), 900, 400, func(c *Context) {
		LightFilter(c, []ProField{{Key: "s", Label: "Status", ValueType: ValueSelect, ValueEnum: []ProEnum{{Value: "a", Label: "Alpha"}}}}).Values(&vals).Show()
	})
	if _, ok := d.FindText("Alpha"); !ok {
		t.Fatal("value badge missing")
	}
	d.EnableAccessibility()
	d.Settle()
	n, ok := d.A11yFind(a11y.RoleButton, locFmt(ZhCN.LightFilter.ClearFmt, "Status"))
	if !ok {
		t.Fatal("clear button missing")
	}
	if err := d.PerformAction(n.ID, a11y.Press, ""); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	if _, has := vals["s"]; has {
		t.Fatal("clear did not remove the value")
	}
}
