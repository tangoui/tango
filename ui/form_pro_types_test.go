package ui

import (
	"errors"
	"image"
	"image/color"
	"strings"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func proTestImage() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range img.Pix {
		img.Pix[i] = 0x80
	}
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	return img
}

// ---------------------------------------------------------------------------
// Value types (third round)

func TestProTypesText(t *testing.T) {
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 400, 200, func(c *Context) { Text(c, "x") })
	c := d.r.c
	now := c.Now()
	day := time.Date(2026, 11, 13, 9, 30, 0, 0, time.UTC)
	for _, tc := range []struct {
		vt   string
		v    any
		want string
	}{
		{ValueDateMonth, day, "2026-11"},
		{ValueDateYear, day, "2026"},
		{ValueDateQuarter, day, p2aQuarterLabel(ZhCN, day)},
		{ValueDateWeek, day, p2aFormatPeriod(ZhCN, day, PickerWeek, "2006-01-02", ZhCN.WeekStart)},
		{ValueDateTimeRange, [2]time.Time{day, day.Add(time.Hour)}, "2026-11-13 09:30:00 " + ZhCN.RangeSeparator + " 2026-11-13 10:30:00"},
		{ValueFromNow, now.Add(-30 * time.Second), "刚刚"},
		{ValueFromNow, now.Add(-5 * time.Minute), "5 分钟前"},
		{ValueFromNow, now.Add(-3 * 24 * time.Hour), "3 天前"},
		{ValueFromNow, now.Add(2 * time.Hour), "2 小时后"},
		{ValueSecond, 3605.0, "1 小时 5 秒"},
		{ValueSecond, 0.0, "0 秒"},
		{ValueSecond, 90061.0, "1 天 1 小时 1 分钟 1 秒"},
		{ValueJSONCode, `{"a":1}`, "{\n  \"a\": 1\n}"},
		{ValueJSONCode, map[string]int{"b": 2}, "{\n  \"b\": 2\n}"},
		{ValueCode, "go run .", "go run ."},
		{ValueIndex, 3, "3"},
		{ValueAvatar, "Lee", "Lee"},
		{ValueImage, []image.Image{}, "-"},
		{ValueCaptcha, "", "-"},
		{ValueFormList, []ProValues{{}, {}}, ZhCN.Items(2)},
	} {
		if got := pro3ValueText(c, tc.vt, nil, tc.v); got != tc.want {
			t.Errorf("%s %v: %q, want %q", tc.vt, tc.v, got, tc.want)
		}
	}
	if s := proFromNow(EnUS, now.Add(-2*time.Hour), now); s != "2 h ago" {
		t.Fatalf("en fromNow %q", s)
	}
	if _, ok := proJSONText("{bad"); ok {
		t.Fatal("invalid JSON accepted")
	}
	// Empty values read "-".
	for _, vt := range []string{ValueDateWeek, ValueDateTimeRange, ValueFromNow, ValueCode, ValueJSONCode, ValueImage, ValueAvatar} {
		if s := pro3ValueText(c, vt, nil, nil); s != "-" {
			t.Fatalf("%s nil: %q", vt, s)
		}
	}
}

func TestProFormReadonly(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		vals := ProValues{"name": "Alpha", "amount": 12800.0, "month": time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			"cfg": `{"x":1}`, "photo": proTestImage(), "dur": 65.0}
		readonly := true
		fields := []ProField{
			{Key: "name", Label: "Name", Required: true},
			{Key: "amount", Label: "Amount", ValueType: ValueMoney},
			{Key: "month", Label: "Month", ValueType: ValueDateMonth},
			{Key: "cfg", Label: "Config", ValueType: ValueJSONCode},
			{Key: "photo", Label: "Photo", ValueType: ValueImage},
			{Key: "dur", Label: "Duration", ValueType: ValueSecond},
			{Key: "note", Label: "Note", Readonly: true},
		}
		var pf *ProFormW
		d := pro3Driver(th, 900, 900, func(c *Context) {
			pf = ProForm(c, fields).Values(&vals).Readonly(readonly).Show()
		})
		d.EnableAccessibility()
		d.Frame()
		for _, s := range []string{"Alpha", ProFormatLocale(ZhCN, ValueMoney, nil, 12800.0), "2026-11", "1 分钟 5 秒"} {
			if !pro3HasText(d, s) {
				t.Fatalf("%q not shown read-only", s)
			}
		}
		if _, ok := d.A11yFind(a11y.RoleTextbox, ""); ok {
			t.Fatal("read-only form has a text box")
		}
		if _, ok := d.FindText(ZhCN.Form.Submit); ok {
			t.Fatal("read-only form shows the submit button")
		}
		mustNode(t, d, a11y.RoleImage, "Photo")
		if !pf.Form().Validate() {
			t.Fatal("read-only fields must not validate")
		}
		// Back to editing: the per-field Readonly stays read-only.
		readonly = false
		d.Settle()
		if _, ok := d.FindText(ZhCN.Form.Submit); !ok {
			t.Fatal("submit button missing after leaving read-only")
		}
		if n := len(d.AccessibilityTree().FindAll(a11y.RoleTextbox, "")); n == 0 {
			t.Fatal("editable form has no text boxes")
		}
		if rep := d.AuditNoContrast(); rep.Count(AuditMissingName) != 0 {
			t.Fatalf("audit: %s", rep)
		}
	})
}

func TestProFieldOfModes(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		var price any = 12.5
		mode := ProFieldRead
		var w *ProFieldW
		changed := false
		d := pro3Driver(th, 500, 200, func(c *Context) {
			w = ProFieldOf(c, ProField{Label: "Price", ValueType: ValueDigit}, &price).Mode(mode).Show()
			if w.Changed() {
				changed = true
			}
		})
		if !pro3HasText(d, "12.50") {
			t.Fatal("read mode text")
		}
		mode = ProFieldEdit
		d.Settle()
		d.EnableAccessibility()
		d.Frame()
		n := mustNode(t, d, a11y.RoleSpinButton, "Price")
		d.Click(geom.Rect{X: n.Bounds.X, Y: n.Bounds.Y, W: n.Bounds.W, H: n.Bounds.H}.Center())
		d.Key(input.KeyEnd, 0)
		for i := 0; i < 8; i++ {
			d.Key(input.KeyBackspace, 0)
		}
		d.Type("42")
		d.Key(input.KeyEnter, 0)
		d.Settle()
		if f, _ := proNumber(price); f != 42 || !changed {
			t.Fatalf("edit: %v changed=%v", price, changed)
		}
		// A read-only field ignores the edit mode.
		var tag any = "Lee"
		d2 := pro3Driver(th, 400, 200, func(c *Context) {
			ProFieldOf(c, ProField{Label: "Owner", ValueType: ValueAvatar, Readonly: true}, &tag).Mode(ProFieldEdit).Show()
		})
		d2.EnableAccessibility()
		d2.Frame()
		mustNode(t, d2, a11y.RoleImage, "Lee")
	})
}

func TestProFormNewTypesEdit(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		vals := ProValues{"cfg": "{bad"}
		sent := 0
		fields := []ProField{
			{Key: "week", Label: "Week", ValueType: ValueDateWeek},
			{Key: "span", Label: "Span", ValueType: ValueDateTimeRange},
			{Key: "cfg", Label: "Config", ValueType: ValueJSONCode},
			{Key: "code", Label: "Code", ValueType: ValueCaptcha, CaptchaSeconds: 30,
				OnCaptcha: func(v ProValues, done func(error)) { sent++; go done(nil) }},
			{Key: "photo", Label: "Photo", ValueType: ValueImage, Pick: func(done func([]UploadFile)) {
				go done([]UploadFile{{Name: "p.png", Status: "done", Thumb: proTestImage()}})
			}},
		}
		var pf *ProFormW
		d := pro3Driver(th, 900, 1000, func(c *Context) { pf = ProForm(c, fields).Values(&vals).Show() })
		d.EnableAccessibility()
		d.Frame()
		mustNode(t, d, a11y.RoleCombobox, "Week")
		// Invalid JSON fails validation with the locale message.
		if pf.Form().Validate() {
			t.Fatal("invalid JSON passed")
		}
		d.Settle()
		if pf.Form().Errors()["cfg"] != ZhCN.ProField.InvalidJSON {
			t.Fatalf("errors %v", pf.Form().Errors())
		}
		// Captcha: the button sends the code and counts down.
		if err := d.ClickLabel(ZhCN.LoginForm.GetCode); err != nil {
			t.Fatal(err)
		}
		resend := locFmt(ZhCN.LoginForm.ResendFmt, 30)
		pro3Wait(t, d, func() bool { return pro3HasText(d, resend) })
		if sent != 1 {
			t.Fatalf("OnCaptcha calls %d", sent)
		}
		d.Advance(31 * time.Second)
		d.Settle()
		if !pro3HasText(d, ZhCN.LoginForm.GetCode) {
			t.Fatal("countdown did not end")
		}
		// Image: the picture card asks the app for a picture.
		if err := d.ClickLabel(ZhCN.Upload.Trigger); err != nil {
			t.Fatal(err)
		}
		pro3Wait(t, d, func() bool { _, ok := vals["photo"].(image.Image); return ok })
	})
}

func TestProFormListField(t *testing.T) {
	vals := ProValues{}
	fields := []ProField{{Key: "members", Label: "Members", ValueType: ValueFormList, Required: true,
		List: &ProFieldList{Max: 2, Fields: []ProField{
			{Key: "name", Label: "Name", Required: true},
			{Key: "role", Label: "Role", ValueType: ValueSelect, ValueEnum: []ProEnum{{Value: "dev", Label: "Dev"}}, Initial: "dev"},
		}}}}
	var pf *ProFormW
	readonly := false
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 900, 600, func(c *Context) {
		pf = ProForm(c, fields).Values(&vals).Readonly(readonly).Show()
	})
	if pf.Form().Validate() {
		t.Fatal("empty required list passed")
	}
	if err := d.ClickLabel(ZhCN.Form.AddRow); err != nil {
		t.Fatal(err)
	}
	d.Settle()
	rows, _ := vals["members"].([]ProValues)
	if len(rows) != 1 || rows[0].String("role") != "dev" {
		t.Fatalf("add row: %v", vals["members"])
	}
	if pf.Form().Validate() {
		t.Fatal("empty row name passed")
	}
	rows[0]["name"] = "Ann"
	d.Settle()
	if !pf.Form().Validate() {
		t.Fatalf("valid list failed: %v", pf.Form().Errors())
	}
	c := vals.Clone()
	c["members"].([]ProValues)[0]["name"] = "Bob"
	if rows[0].String("name") != "Ann" {
		t.Fatal("Clone shares list rows")
	}
	readonly = true
	d.Settle()
	if !pro3HasText(d, "Ann") || !pro3HasText(d, "Dev") {
		t.Fatal("read-only list rows")
	}
}

func TestProTableNewValueTypes(t *testing.T) {
	type row struct {
		name string
		secs float64
		pic  image.Image
		code string
	}
	rows := []row{{"a", 61, proTestImage(), "x := 1"}, {"b", 7200, nil, ""}}
	cols := []ProColumn[row]{
		{Key: "idx", Title: "#", ValueType: ValueIndex, Width: 60},
		{Key: "name", Title: "Name", Value: func(r row) any { return r.name }},
		{Key: "secs", Title: "Time", ValueType: ValueSecond, Value: func(r row) any { return r.secs }},
		{Key: "pic", Title: "Pic", ValueType: ValueImage, Value: func(r row) any {
			if r.pic == nil {
				return nil
			}
			return r.pic
		}},
		{Key: "code", Title: "Code", ValueType: ValueCode, Value: func(r row) any { return r.code }},
	}
	d := NewDriver(func(c *Context) {
		ProTable(c, cols).RowKey(func(r row) string { return r.name }).
			Request(func(req ProRequest, done func(ProResult[row])) { done(ProResult[row]{Rows: rows, Total: 2}) }).Show()
	}, DriverOptions{Width: 1000, Height: 600, Accessibility: true})
	d.Settle()
	for _, s := range []string{"1", "2", "1 分钟 1 秒", "2 小时", "x := 1"} {
		if !pro3HasText(d, s) {
			t.Fatalf("%q not in the table", s)
		}
	}
	mustNode(t, d, a11y.RoleImage, "Pic")
	// Index / image / code columns are not search fields.
	if _, ok := d.FindText("#"); !ok {
		t.Fatal("index header")
	}
	if n := len(d.AccessibilityTree().FindAll(a11y.RoleTextbox, "")); n > 2 {
		t.Fatalf("unexpected search boxes: %d", n)
	}
}

// ---------------------------------------------------------------------------
// SchemaForm

func TestSchemaFormLayouts(t *testing.T) {
	schema := []ProField{
		{Key: "name", Label: "Name", Group: "Basic", Required: true},
		{Key: "status", Label: "Status", Group: "Basic", ValueType: ValueSelect, ValueEnum: []ProEnum{{Value: "on", Label: "On"}}},
		{Key: "amount", Label: "Amount", Group: "Money", ValueType: ValueDigit},
	}
	lt := SchemaLayoutForm
	open := false
	vals := ProValues{}
	var sf *SchemaFormW
	finished := 0
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 1000, 700, func(c *Context) {
		sf = SchemaForm(c, schema).LayoutType(lt).Values(&vals).Open(&open).Title("Edit").
			OnFinish(func(v ProValues, done func(error)) { finished++; done(nil) }).Show()
	})
	// Form: groups become ProFormGroup titles.
	if !pro3HasText(d, "Basic") || !pro3HasText(d, ZhCN.Form.Submit) {
		t.Fatal("form shell")
	}
	// ModalForm: nothing until opened, then a dialog.
	lt = SchemaLayoutModalForm
	d.Settle()
	if pro3HasText(d, "Edit") {
		t.Fatal("closed modal shown")
	}
	open = true
	d.Settle()
	d.EnableAccessibility()
	d.Frame()
	mustNode(t, d, a11y.RoleDialog, "Edit")
	open = false
	// StepsForm: one step per group.
	lt = SchemaLayoutStepsForm
	d.Settle()
	if !pro3HasText(d, "Basic") || !pro3HasText(d, "Money") || !pro3HasText(d, ZhCN.Form.NextStep) {
		t.Fatal("steps from groups")
	}
	// QueryFilter: search reports the params.
	lt = SchemaLayoutQueryFilter
	vals = ProValues{"name": "x"}
	d.Settle()
	if err := d.ClickLabel(ZhCN.Form.Query); err != nil {
		t.Fatal(err)
	}
	if sf.Params()["name"] != "x" {
		t.Fatalf("query: %v", sf.Params())
	}
	// LightFilter: a pill per field.
	lt = SchemaLayoutLightFilter
	d.Settle()
	mustNode(t, d, a11y.RoleToolbar, ZhCN.LightFilter.Name)
	if steps := schemaSteps(schema, 2); len(steps) != 2 || steps[0].Title != "Basic" || steps[0].Fields[0].Group != "" || steps[1].Columns != 2 {
		t.Fatalf("schemaSteps %+v", steps)
	}
	_ = finished
}

// ---------------------------------------------------------------------------
// ProLayout

func TestProLayoutAutoBreadcrumbAndHooks(t *testing.T) {
	for _, th := range []func() *theme.Theme{func() *theme.Theme { return theme.AntDesign() }, func() *theme.Theme { return theme.Shadcn() }} {
		menu := []MenuItem{{Key: "home", Label: "Home"}, {Key: "sys", Label: "System", Children: []MenuItem{
			{Key: "users", Label: "Users"}, {Key: "roles", Label: "Roles"}}}}
		sel := "users"
		hdrCalls, menuHdr := 0, 0
		clicked := ""
		d := NewDriver(func(c *Context) {
			ProLayout(c, ProLayoutConfig{Title: "Admin", Menu: menu, Selected: &sel, AutoBreadcrumb: true,
				MenuItemRender: func(it MenuItem) MenuItem {
					if it.Key == "roles" {
						it.Label = "Roles*"
					}
					return it
				},
				BreadcrumbRender: func(items []BreadcrumbItem) []BreadcrumbItem {
					return append([]BreadcrumbItem{{Label: "Root"}}, items...)
				},
				HeaderRender:     func(def func()) { hdrCalls++; def() },
				MenuHeaderRender: func(def func(), collapsed bool) { menuHdr++; def(); Text(c, "v1.0") },
				MenuFooterRender: func(bool) { Text(c, "menu-foot") },
				FooterLinks:      []FooterLink{{Key: "gh", Title: "GitHub", OnClick: func() { clicked = "gh" }}},
				Footer:           "© 2026 Tango",
			}, func() {
				Text(c, "crumbs:"+strings.Join(RouteContext(c).Breadcrumb, "/"))
			})
		}, DriverOptions{Width: 1200, Height: 700, Theme: th(), Accessibility: true})
		d.Settle()
		for _, s := range []string{"crumbs:Root/System/Users", "Roles*", "v1.0", "menu-foot", "© 2026 Tango", "GitHub"} {
			if !pro3HasText(d, s) {
				t.Fatalf("%s: %q missing", th().Flavor, s)
			}
		}
		if hdrCalls == 0 || menuHdr == 0 {
			t.Fatalf("hooks not called: header %d menu header %d", hdrCalls, menuHdr)
		}
		if err := d.ClickLabel("GitHub"); err != nil || clicked != "gh" {
			t.Fatalf("footer link: %v %q", err, clicked)
		}
		if p := MenuPath(menu, "roles"); len(p) != 2 || p[0].Key != "sys" {
			t.Fatalf("MenuPath %v", p)
		}
		if MenuPath(menu, "nope") != nil {
			t.Fatal("MenuPath of a missing key")
		}
	}
	// Clicking an ancestor that is a menu item selects it.
	menu := []MenuItem{{Key: "dash", Label: "Dash", Children: []MenuItem{{Key: "a", Label: "A"}}}, {Key: "b", Label: "B"}}
	sel := "a"
	d := NewDriver(func(c *Context) {
		ProLayout(c, ProLayoutConfig{Title: "Admin", Menu: menu, Selected: &sel, AutoBreadcrumb: true, Layout: "top",
			BreadcrumbRender: func(items []BreadcrumbItem) []BreadcrumbItem {
				// "B" as a clickable ancestor of the page.
				return append([]BreadcrumbItem{{Label: "Back to B", OnClick: func() { sel = "b" }}}, items...)
			}}, func() { Text(c, "page") })
	}, DriverOptions{Width: 1200, Height: 600})
	d.Settle()
	if err := d.ClickLabel("Back to B"); err != nil || sel != "b" {
		t.Fatalf("crumb click: %v %q", err, sel)
	}
}

func TestProLayoutNoFixSiderbar(t *testing.T) {
	for _, th := range []func() *theme.Theme{func() *theme.Theme { return theme.AntDesign() }, func() *theme.Theme { return theme.Shadcn() }} {
		for _, mode := range []string{"side", "mix"} {
			nofix := false
			var items []MenuItem
			for i := 0; i < 3; i++ {
				items = append(items, MenuItem{Key: string(rune('a' + i)), Label: "Item " + string(rune('A'+i))})
			}
			sel := "a"
			d := NewDriver(func(c *Context) {
				ProLayout(c, ProLayoutConfig{Title: "Admin", Menu: items, Selected: &sel, Layout: mode, NoFixSiderbar: nofix}, func() {
					Column(c, func() { Text(c, "tall page") }, layout.H(2000))
				})
			}, DriverOptions{Width: 1000, Height: 500, Theme: th()})
			d.Settle()
			scroll := func() float32 {
				before := p2cText(t, d, "Item A").Y
				d.Scroll(geom.Pt(700, 300), 40)
				d.Settle()
				after := p2cText(t, d, "Item A").Y
				d.Scroll(geom.Pt(700, 300), -40)
				d.Settle()
				return before - after
			}
			if moved := scroll(); moved > 0.5 {
				t.Fatalf("%s/%s fixed sider moved %v", th().Flavor, mode, moved)
			}
			nofix = true
			d.Settle()
			d.Settle()
			if moved := scroll(); moved < 20 {
				t.Fatalf("%s/%s sider did not scroll with the page (%v)", th().Flavor, mode, moved)
			}
		}
	}
	var cfg ProLayoutConfig
	s := DefaultProSettings()
	s.FixSiderbar = false
	s.Apply(&cfg)
	if !cfg.NoFixSiderbar {
		t.Fatal("Apply FixSiderbar")
	}
}

// ---------------------------------------------------------------------------
// ProCard actions

func TestProCardActions(t *testing.T) {
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus(), theme.Shadcn()} {
		hits := 0
		folded := false
		d := pro2Driver(th, 600, 400, func(c *Context) {
			ProCard(c, "Card").Collapsed(&folded).Actions(
				func() {
					if CardAction(c, icon.Edit, "Edit").Clicked() {
						hits++
					}
				},
				func() { Text(c, "More") },
			).Body(func() { Text(c, "body text") })
		})
		body := p2cText(t, d, "body text")
		more := p2cText(t, d, "More")
		if more.Y <= body.Bottom() {
			t.Fatalf("actions not under the body: %v %v", body, more)
		}
		if err := d.ClickLabel("Edit"); err != nil || hits != 1 {
			t.Fatalf("action click %v %d", err, hits)
		}
		d.EnableAccessibility()
		d.Frame()
		mustNode(t, d, a11y.RoleToolbar, ZhCN.A11y.Toolbar)
		folded = true
		d.Settle()
		if pro3HasText(d, "More") {
			t.Fatal("actions shown while folded")
		}
	}
}

var _ = errors.New
