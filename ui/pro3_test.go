package ui

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// pro3Themes runs fn under Ant Design and Element Plus.
func pro3Themes(t *testing.T, fn func(t *testing.T, th func() *theme.Theme)) {
	for _, tc := range []struct {
		name string
		th   func() *theme.Theme
	}{{"ant", func() *theme.Theme { return theme.AntDesign() }}, {"element", func() *theme.Theme { return theme.ElementPlus() }}} {
		t.Run(tc.name, func(t *testing.T) { fn(t, tc.th) })
	}
}

func pro3Driver(th func() *theme.Theme, w, h float32, page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) { Column(c, func() { page(c) }, layout.Pad(16)) }, DriverOptions{Width: w, Height: h})
	d.SetTheme(th())
	d.Settle()
	return d
}

// pro3Wait settles until cond holds (background goroutines deliver).
func pro3Wait(t *testing.T, d *Driver, cond func() bool) {
	t.Helper()
	for i := 0; i < 200; i++ {
		d.Settle()
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("timed out")
}

// pro3TextEditors returns the text editors' rects in build order (the
// innermost text-cursor hit areas).
func pro3TextEditors(d *Driver) []geom.Rect {
	var all []geom.Rect
	for _, h := range d.r.c.hits {
		if h.cursor == input.CursorText {
			all = append(all, h.rect)
		}
	}
	var out []geom.Rect
	for i, r := range all {
		inner := false
		for j, o := range all {
			if i != j && o != r && r.Contains(o.Center()) && o.W*o.H < r.W*r.H {
				inner = true
				break
			}
		}
		if !inner {
			out = append(out, r)
		}
	}
	return out
}

func pro3HasText(d *Driver, s string) bool {
	_, ok := d.FindText(s)
	return ok
}

// ---------------------------------------------------------------------------
// ProDescriptions

var pro3DescItems = []ProDescItem{
	{Key: "name", Label: "Name", Field: &ProField{Required: true}},
	{Key: "amount", Label: "Amount", ValueType: ValueMoney},
	{Key: "status", Label: "Status", ValueType: ValueStatus, ValueEnum: []ProEnum{{Value: "on", Label: "Online", Status: "success"}, {Value: "off", Label: "Offline"}}},
	{Key: "date", Label: "Date", ValueType: ValueDate, ReadOnly: true},
	{Key: "rate", Label: "Rate", ValueType: ValuePercent},
	{Key: "memo", Label: "Memo", ValueType: ValueTextArea, Span: 2},
}

func TestProDescriptionsRequestAndInlineEdit(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		release := make(chan struct{})
		var saves []string
		var failNext bool
		var p *ProDescriptionsW
		d := pro3Driver(th, 900, 600, func(c *Context) {
			p = ProDescriptions(c, pro3DescItems).Title("Detail").Column(2).Bordered().
				Request(func() (map[string]any, error) {
					<-release
					return map[string]any{"name": "abc", "amount": 12800.0, "status": "on",
						"date": time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC), "rate": 12.5}, nil
				}).
				Editable(func(key string, v any, all ProValues, done func(error)) {
					saves = append(saves, fmt.Sprintf("%s=%v", key, v))
					if failNext {
						failNext = false
						done(errors.New("save failed"))
						return
					}
					go done(nil)
				}).Show()
		})
		if !p.Loading() {
			t.Fatal("not loading while the request runs")
		}
		close(release)
		pro3Wait(t, d, func() bool { return !p.Loading() })
		for _, s := range []string{"Detail", "abc", "¥12,800.00", "Online", "2026-01-02", "12.50%"} {
			if !pro3HasText(d, s) {
				t.Fatalf("%q not shown", s)
			}
		}
		// ReadOnly items have no edit icon.
		if _, ok := d.Find(locFmt(ZhCN.ProDescriptions.EditFmt, "Date")); ok {
			t.Fatal("read-only item editable")
		}
		if err := d.ClickLabel(locFmt(ZhCN.ProDescriptions.EditFmt, "Name")); err != nil {
			t.Fatal(err)
		}
		if !p.Editing("name") {
			t.Fatal("not editing")
		}
		eds := pro3TextEditors(d)
		if len(eds) != 1 {
			t.Fatalf("editors %d", len(eds))
		}
		d.Click(eds[0].Center())
		d.Type("X")
		d.ClickLabel(ZhCN.Save)
		pro3Wait(t, d, func() bool { return !p.Editing("name") })
		if p.Values().String("name") != "abcX" || len(saves) != 1 || saves[0] != "name=abcX" {
			t.Fatalf("saved %v / %v", p.Values(), saves)
		}
		if !pro3HasText(d, "abcX") {
			t.Fatal("new value not shown")
		}
		// Required: an emptied value is refused with the locale message.
		d.ClickLabel(locFmt(ZhCN.ProDescriptions.EditFmt, "Name"))
		eds = pro3TextEditors(d)
		d.Click(eds[0].Center())
		for range 6 {
			d.Key(input.KeyBackspace, 0)
		}
		d.ClickLabel(ZhCN.Save)
		if !p.Editing("name") || len(saves) != 1 {
			t.Fatal("empty value saved")
		}
		if !pro3HasText(d, fmt.Sprintf(ZhCN.Form.RequiredInputFmt, "Name")) {
			t.Fatal("required message missing")
		}
		d.ClickLabel(ZhCN.Cancel)
		if p.Editing("name") || p.Values().String("name") != "abcX" {
			t.Fatal("cancel")
		}
		// A failing save keeps the editor open with the error.
		d.ClickLabel(locFmt(ZhCN.ProDescriptions.EditFmt, "Status"))
		failNext = true
		d.ClickLabel(ZhCN.Save)
		if !p.Editing("status") || !pro3HasText(d, "save failed") {
			t.Fatal("save error not shown")
		}
	})
}

func TestProDescriptionsRequestError(t *testing.T) {
	var p *ProDescriptionsW
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 800, 400, func(c *Context) {
		p = ProDescriptions(c, pro3DescItems).Request(func() (map[string]any, error) { return nil, errors.New("boom") }).Show()
	})
	pro3Wait(t, d, func() bool { return !p.Loading() })
	if p.Err() == nil || !pro3HasText(d, "boom") {
		t.Fatal("error not shown")
	}
}

// ---------------------------------------------------------------------------
// ProList

type pro3Row struct{ id, name, desc string }

func pro3Rows(n int) []pro3Row {
	var rs []pro3Row
	for i := range n {
		rs = append(rs, pro3Row{fmt.Sprint("k", i), fmt.Sprint("Item ", i), fmt.Sprint("Desc ", i)})
	}
	return rs
}

func pro3Metas() ProListMetas[pro3Row] {
	return ProListMetas[pro3Row]{
		Title:       func(r pro3Row) string { return r.name },
		Description: func(r pro3Row) string { return r.desc },
		SubTitle:    func(c *Context, r pro3Row) { Tag(c, "tag-"+r.id) },
		Actions:     func(c *Context, r pro3Row) { Link(c, "open-"+r.id) },
	}
}

func TestProListSelectExpandPaginate(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		rows := pro3Rows(7)
		var sel []string
		var p *ProListW[pro3Row]
		d := pro3Driver(th, 900, 900, func(c *Context) {
			p = ProList(c, rows, pro3Metas()).HeaderTitle("Projects").RowKey(func(r pro3Row) string { return r.id }).
				ToolBar(func() { Button(c, "New").Primary() }).
				Selection(&sel).Pagination(3).
				Expandable(func(c *Context, r pro3Row) { Text(c, "detail of "+r.name) }).Show()
		})
		for _, s := range []string{"Projects", "New", "Item 0", "Desc 2", "tag-k1", "open-k2"} {
			if !pro3HasText(d, s) {
				t.Fatalf("%q missing", s)
			}
		}
		if pro3HasText(d, "Item 3") {
			t.Fatal("page 2 shown on page 1")
		}
		// The checkbox sits left of the title block.
		d.Click(pro3CheckboxLeftOf(d, "Item 1").Center())
		if len(sel) != 1 || sel[0] != "k1" {
			t.Fatalf("selection %v", sel)
		}
		if !pro3HasText(d, ZhCN.Selected(1)) {
			t.Fatal("selection bar missing")
		}
		clickNth(t, d, ZhCN.Expand, 0)
		if !pro3HasText(d, "detail of Item 0") {
			t.Fatal("not expanded")
		}
		if err := d.ClickLabel(ZhCN.NextPage); err != nil {
			t.Fatal(err)
		}
		if p.Page() != 2 || !pro3HasText(d, "Item 3") || pro3HasText(d, "Item 0") {
			t.Fatalf("page %d", p.Page())
		}
		d.ClickLabel(ZhCN.ProTable.ClearSelection)
		if len(sel) != 0 {
			t.Fatal("clear selection")
		}
	})
}

func TestProListGridAndLoading(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		rows := pro3Rows(4)
		var sel []string
		loading := true
		d := pro3Driver(th, 900, 700, func(c *Context) {
			ProList(c, rows, pro3Metas()).Grid(2).Selection(&sel).Loading(loading).
				RowKey(func(r pro3Row) string { return r.id }).Show()
		})
		r0, _ := d.FindText("Item 0")
		r1, _ := d.FindText("Item 1")
		r2, _ := d.FindText("Item 2")
		if !(r1.X > r0.X+100 && absf(r1.Y-r0.Y) < 1 && r2.Y > r0.Y+20) {
			t.Fatalf("grid layout %v %v %v", r0, r1, r2)
		}
		loading = false
		d.Settle()
		// Clicking the card body (its description) toggles the selection.
		r, _ := d.FindText("Desc 2")
		d.Click(r.Center())
		if len(sel) != 1 || sel[0] != "k2" {
			t.Fatalf("card selection %v", sel)
		}
		d.Click(r.Center())
		if len(sel) != 0 {
			t.Fatalf("card deselect %v", sel)
		}
	})
}

func absf(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

func TestProListEmpty(t *testing.T) {
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 600, 300, func(c *Context) {
		ProList(c, []pro3Row(nil), pro3Metas()).EmptyText("nothing here").Show()
	})
	if !pro3HasText(d, "nothing here") {
		t.Fatal("empty text")
	}
}

// ---------------------------------------------------------------------------
// LightFilter

var pro3FilterFields = []ProField{
	{Key: "status", Label: "State", ValueType: ValueSelect, ValueEnum: []ProEnum{{Value: "a", Label: "Alpha"}, {Value: "b", Label: "Beta"}}},
	{Key: "name", Label: "Keyword"},
	{Key: "vip", Label: "VIP", ValueType: ValueSwitch},
}

func TestLightFilterPickConfirmClear(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		var vals ProValues
		changes := 0
		var lf *LightFilterW
		d := pro3Driver(th, 900, 500, func(c *Context) {
			lf = LightFilter(c, pro3FilterFields).Values(&vals).Show()
			if lf.Changed() {
				changes++
			}
		})
		if err := d.ClickLabel("State"); err != nil {
			t.Fatal(err)
		}
		if lf.OpenKey() != "status" {
			t.Fatal("popup not open")
		}
		// Open the select inside the popup and pick Beta.
		eds := pro3SelectIn(d)
		d.Click(eds.Center())
		if err := d.ClickLabel("Beta"); err != nil {
			t.Fatal(err)
		}
		if lf.OpenKey() != "status" {
			t.Fatal("picking an option closed the popup")
		}
		if vals.String("status") != "" {
			t.Fatal("value applied before confirm")
		}
		d.ClickLabel(ZhCN.Confirm)
		if vals.String("status") != "b" || lf.OpenKey() != "" || changes != 1 {
			t.Fatalf("confirm: %v open=%q changes=%d", vals, lf.OpenKey(), changes)
		}
		if !pro3HasText(d, "Beta") || lf.Params()["status"] != "b" {
			t.Fatal("pill value")
		}
		// Typing a keyword then pressing outside discards the draft.
		d.ClickLabel("Keyword")
		eds2 := pro3TextEditors(d)
		d.Click(eds2[len(eds2)-1].Center())
		d.Type("foo")
		d.Click(geom.Pt(880, 480))
		if lf.OpenKey() != "" || vals.String("name") != "" {
			t.Fatalf("dismiss: %v", vals)
		}
		// The clear icon on a filled pill removes its value.
		if err := d.ClickLabel(locFmt(ZhCN.LightFilter.ClearFmt, "State")); err != nil {
			t.Fatal(err)
		}
		if _, ok := vals["status"]; ok || changes != 2 {
			t.Fatalf("clear: %v", vals)
		}
		// Clear inside the popup.
		vals["name"] = "bar"
		d.Settle()
		d.ClickLabel("Keyword")
		d.ClickLabel(ZhCN.Clear)
		if _, ok := vals["name"]; ok || lf.OpenKey() != "" {
			t.Fatalf("popup clear: %v", vals)
		}
	})
}

// pro3CheckboxLeftOf returns the unlabelled hit area left of the text s
// on the same row (an item's checkbox).
func pro3CheckboxLeftOf(d *Driver, s string) geom.Rect {
	r, _ := d.FindText(s)
	var best geom.Rect
	for _, h := range d.r.c.hits {
		if h.label == "" && h.rect.Right() <= r.X && h.rect.W <= 24 && h.rect.Bottom() > r.Y && h.rect.Y < r.Bottom()+30 {
			if best.Empty() || h.rect.X > best.X {
				best = h.rect
			}
		}
	}
	return best
}

// pro3SelectIn returns the rect of the Select trigger in the top layer.
func pro3SelectIn(d *Driver) geom.Rect {
	c := d.r.c
	ov := c.overlays[len(c.overlays)-1].root
	var found geom.Rect
	var walk func(n *node)
	walk = func(n *node) {
		if !found.Empty() {
			return
		}
		if n.hit && n.cursor == input.CursorPointer && n.abs.W > 100 && n.abs.H >= 24 && n.abs.H <= 40 {
			found = n.abs
			return
		}
		for _, k := range n.kids {
			walk(k)
		}
	}
	walk(ov)
	return found
}

// ---------------------------------------------------------------------------
// LoginForm

func TestLoginFormAccount(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		var got ProValues
		var lw *LoginFormW
		finished := 0
		forgot := 0
		d := pro3Driver(th, 600, 700, func(c *Context) {
			lw = LoginForm(c).Title("Tango").SubTitle("Sub title").
				OnForgot(func() { forgot++ }).
				Actions(func() { Text(c, "wechat") }).
				OnFinish(func(v ProValues, done func(error)) {
					got = v
					if v.String("password") == "bad" {
						done(errors.New("wrong password"))
						return
					}
					go done(nil)
				}).Show()
			if lw.Finished() {
				finished++
			}
		})
		for _, s := range []string{"Tango", "Sub title", ZhCN.LoginForm.AccountTab, ZhCN.LoginForm.MobileTab, ZhCN.LoginForm.Remember, ZhCN.LoginForm.Forgot, ZhCN.LoginForm.Others, "wechat"} {
			if !pro3HasText(d, s) {
				t.Fatalf("%q missing", s)
			}
		}
		d.ClickLabel(ZhCN.LoginForm.Submit)
		d.Settle()
		if got != nil || !pro3HasText(d, ZhCN.LoginForm.UsernameRequired) || !pro3HasText(d, ZhCN.LoginForm.PasswordRequired) {
			t.Fatal("empty form submitted")
		}
		eds := pro3TextEditors(d)
		if len(eds) != 2 {
			t.Fatalf("editors %d", len(eds))
		}
		d.Click(eds[0].Center())
		d.Type("admin")
		d.Click(eds[1].Center())
		d.Type("bad")
		d.ClickLabel(ZhCN.LoginForm.Submit)
		pro3Wait(t, d, func() bool { return !lw.Submitting() })
		if got.String("username") != "admin" || got.String("loginType") != LoginAccount || !got.Bool("autoLogin") {
			t.Fatalf("values %v", got)
		}
		if !pro3HasText(d, "wrong password") || finished != 0 {
			t.Fatal("error not shown")
		}
		eds = pro3TextEditors(d) // the error banner moved the inputs
		d.Click(eds[1].Center())
		d.Key(input.KeyBackspace, 0)
		d.Key(input.KeyBackspace, 0)
		d.Key(input.KeyBackspace, 0)
		d.Type("ok")
		d.Key(input.KeyEnter, 0) // Enter submits
		pro3Wait(t, d, func() bool { return finished == 1 })
		d.ClickLabel(ZhCN.LoginForm.Forgot)
		if forgot != 1 {
			t.Fatal("forgot")
		}
		// Remember me toggles autoLogin.
		d.ClickLabel(ZhCN.LoginForm.Remember)
		if lw.Values().Bool("autoLogin") {
			t.Fatal("remember toggle")
		}
	})
}

func TestLoginFormMobileCountdown(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		var sent []string
		var got ProValues
		var lw *LoginFormW
		d := pro3Driver(th, 600, 700, func(c *Context) {
			lw = LoginForm(c).Title("Tango").CodeInterval(3).
				OnSendCode(func(m string, done func(error)) { sent = append(sent, m); done(nil) }).
				OnFinish(func(v ProValues, done func(error)) { got = v; done(nil) }).Show()
		})
		d.ClickLabel(ZhCN.LoginForm.MobileTab)
		if lw.LoginType() != LoginMobile {
			t.Fatal("tab")
		}
		// An invalid number does not send.
		eds := pro3TextEditors(d)
		if len(eds) != 2 {
			t.Fatalf("editors %d", len(eds))
		}
		d.Click(eds[0].Center())
		d.Type("123")
		d.ClickLabel(ZhCN.LoginForm.GetCode)
		if len(sent) != 0 || !pro3HasText(d, ZhCN.LoginForm.MobileInvalid) {
			t.Fatal("invalid number sent")
		}
		d.Click(eds[0].Center())
		for range 3 {
			d.Key(input.KeyBackspace, 0)
		}
		d.Type("13800000000")
		d.ClickLabel(ZhCN.LoginForm.GetCode)
		if len(sent) != 1 || sent[0] != "13800000000" {
			t.Fatalf("sent %v", sent)
		}
		if lw.Countdown() != 3 || !pro3HasText(d, locFmt(ZhCN.LoginForm.ResendFmt, 3)) {
			t.Fatalf("countdown %d", lw.Countdown())
		}
		d.ClickLabel(locFmt(ZhCN.LoginForm.ResendFmt, 3)) // disabled
		if len(sent) != 1 {
			t.Fatal("sent during countdown")
		}
		d.Advance(1100 * time.Millisecond)
		d.Settle()
		if !pro3HasText(d, locFmt(ZhCN.LoginForm.ResendFmt, 2)) {
			t.Fatal("countdown did not tick")
		}
		d.Advance(2 * time.Second)
		d.Settle()
		if lw.Countdown() != 0 || !pro3HasText(d, ZhCN.LoginForm.GetCode) {
			t.Fatal("countdown did not end")
		}
		d.ClickLabel(ZhCN.LoginForm.Submit)
		if got != nil || !pro3HasText(d, ZhCN.LoginForm.CaptchaRequired) {
			t.Fatal("submitted without code")
		}
		eds = pro3TextEditors(d)
		d.Click(eds[1].Center())
		d.Type("1234")
		d.ClickLabel(ZhCN.LoginForm.Submit)
		if got.String("captcha") != "1234" || got.String("mobile") != "13800000000" || got.String("loginType") != LoginMobile {
			t.Fatalf("values %v", got)
		}
	})
}

func TestLoginFormPage(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		d := pro3Driver(th, 1000, 700, func(c *Context) {
			LoginFormPage(c).Title("Tango").Side(func() { Text(c, "Welcome aboard") }).AccountOnly().Show()
		})
		side, _ := d.FindText("Welcome aboard")
		title, _ := d.FindText("Tango")
		if side.Empty() || title.Empty() || !(title.X > side.Right()) {
			t.Fatalf("page layout side=%v title=%v", side, title)
		}
		if pro3HasText(d, ZhCN.LoginForm.MobileTab) {
			t.Fatal("tabs shown in account-only mode")
		}
	})
}

// ---------------------------------------------------------------------------
// ProFormFieldSet

func TestProFormFieldSetInProForm(t *testing.T) {
	pro3Themes(t, func(t *testing.T, th func() *theme.Theme) {
		var vals ProValues
		var pf *ProFormW
		var done ProValues
		d := pro3Driver(th, 800, 500, func(c *Context) {
			fields := []ProField{
				ProFormFieldSet(c, ProFieldSet{Key: "range", Label: "Range", Separator: "~", Required: true,
					Parts: []ProField{{ValueType: ValueDigit, Placeholder: "min"}, {ValueType: ValueDigit, Placeholder: "max"}},
					Validate: func(p []any, all ProValues) string {
						a, _ := proNumber(p[0])
						b, _ := proNumber(p[1])
						if a > b {
							return "min must not exceed max"
						}
						return ""
					}}),
			}
			pf = ProForm(c, fields).Values(&vals).Horizontal().
				OnFinish(func(v ProValues, d func(error)) { done = v; d(nil) }).Show()
		})
		if !pro3HasText(d, "~") {
			t.Fatal("separator missing")
		}
		pf.Submit()
		d.Settle()
		if done != nil || !pro3HasText(d, fmt.Sprintf(ZhCN.Form.RequiredInputFmt, "Range")) {
			t.Fatal("required not enforced")
		}
		eds := pro3TextEditors(d)
		if len(eds) != 2 {
			t.Fatalf("editors %d", len(eds))
		}
		d.Click(eds[0].Center())
		d.Type("9")
		d.Click(eds[1].Center())
		d.Type("5")
		d.Click(geom.Pt(700, 450))
		pf.Submit()
		d.Settle()
		if done != nil || !pro3HasText(d, "min must not exceed max") {
			t.Fatalf("cross check: %v", vals)
		}
		d.Click(eds[1].Center())
		d.Type("0") // 50
		d.Click(geom.Pt(700, 450))
		pf.Submit()
		d.Settle()
		p := done.Parts("range")
		if len(p) != 2 || p[0] != 9.0 || p[1] != 50.0 {
			t.Fatalf("value %v", done)
		}
	})
}

func TestFieldSetInForm(t *testing.T) {
	vals := ProValues{"phone": []any{"86", nil}}
	var f *FormW
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 700, 300, func(c *Context) {
		f = Form(c)
		f.Body(func() {
			FieldSet(c, f, ProFieldSet{Key: "phone", Label: "Phone", Required: true,
				Parts: []ProField{{ValueType: ValueSelect, Width: 90, ValueEnum: []ProEnum{{Value: "86", Label: "+86"}, {Value: "1", Label: "+1"}}}, {}}}, vals)
		})
	})
	if f.Validate() {
		t.Fatal("half-filled set passed")
	}
	eds := pro3TextEditors(d)
	d.Click(eds[len(eds)-1].Center())
	d.Type("555")
	d.Settle()
	if !f.Validate() {
		t.Fatalf("filled set failed: %v", f.Errors())
	}
	if p := vals.Parts("phone"); fmt.Sprint(p) != "[86 555]" {
		t.Fatalf("parts %v", p)
	}
}

// pro3 labels must stay out of hard-coded CJK (they come from
// Locale.LoginForm, so a customized locale changes them).
func TestPro3LocaleKeys(t *testing.T) {
	l := *EnUS
	l.LoginForm.Submit = "Sign in"
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Locale(&l).Body(func() { LoginForm(c).Show() })
	}, DriverOptions{Width: 500, Height: 600})
	d.Settle()
	if !pro3HasText(d, "Sign in") {
		t.Fatal("locale override not used")
	}
}

// ---------------------------------------------------------------------------
// Accessibility

// TestPro3A11yLightFilterKeyboard: pills are Tab stops named "label: value";
// Enter opens the popup with focus inside, Esc closes it and focus returns
// to the pill.
func TestPro3A11yLightFilterKeyboard(t *testing.T) {
	vals := ProValues{"status": "b"}
	var lf *LightFilterW
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 900, 500, func(c *Context) {
		lf = LightFilter(c, pro3FilterFields).Values(&vals).Show()
	})
	d.EnableAccessibility()
	if _, ok := d.A11yFind(a11y.RoleButton, locFmt(ZhCN.LightFilter.PillFmt, "State", "Beta")); !ok {
		t.Fatal("pill name lacks the value")
	}
	if n, ok := d.A11yFind(a11y.RoleButton, "Keyword"); !ok || !n.States.Has(a11y.Focusable) || !n.States.Has(a11y.HasPopup) {
		t.Fatalf("empty pill: %+v", n)
	}
	if rep := d.AuditNoContrast(); rep.Count(AuditNotKeyboard) != 0 || rep.Count(AuditMissingName) != 0 {
		t.Fatalf("audit: %s", rep)
	}
	d.Key(input.KeyTab, 0)
	if f := d.A11yFocused(); f == nil || f.Name != locFmt(ZhCN.LightFilter.PillFmt, "State", "Beta") {
		t.Fatalf("Tab focus: %+v", f)
	}
	d.Key(input.KeyEnter, 0)
	if lf.OpenKey() != "status" {
		t.Fatal("Enter did not open the popup")
	}
	if n, _ := d.A11yFind(a11y.RoleButton, locFmt(ZhCN.LightFilter.PillFmt, "State", "Beta")); n == nil || !n.States.Has(a11y.Expanded) || n.Controls == 0 {
		t.Fatalf("pill not expanded: %+v", n)
	}
	if f := d.A11yFocused(); f == nil || f.Name == locFmt(ZhCN.LightFilter.PillFmt, "State", "Beta") {
		t.Fatalf("focus did not move into the popup: %+v", f)
	}
	d.Key(input.KeyEscape, 0)
	d.Settle()
	if lf.OpenKey() != "" {
		t.Fatal("Esc did not close the popup")
	}
	if f := d.A11yFocused(); f == nil || f.Name != locFmt(ZhCN.LightFilter.PillFmt, "State", "Beta") {
		t.Fatalf("focus not returned to the pill: %+v", f)
	}
	// Space opens it as well.
	d.Key(input.KeySpace, 0)
	if lf.OpenKey() != "status" {
		t.Fatal("Space did not open the popup")
	}
}

// TestPro3A11yProList: selection checkboxes are named by the item title,
// clickable rows and selectable cards are Tab stops driven by Enter / Space.
func TestPro3A11yProList(t *testing.T) {
	type row struct{ k, name string }
	rows := []row{{"a", "Alpha"}, {"b", "Beta"}}
	metas := ProListMetas[row]{Title: func(r row) string { return r.name }}
	var sel, sel2 []string
	var clicked []string
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 900, 700, func(c *Context) {
		ProList(c, rows, metas).RowKey(func(r row) string { return r.k }).Selection(&sel).
			OnItemClick(func(r row) { clicked = append(clicked, r.k) }).Show()
		ProList(c, rows, metas).RowKey(func(r row) string { return r.k }).Selection(&sel2).Grid(2).Show()
	})
	d.EnableAccessibility()
	if got := len(d.A11yFindAll(a11y.RoleCheckbox, "Alpha")); got != 2 {
		t.Fatalf("checkboxes named Alpha: %d", got)
	}
	if rep := d.AuditNoContrast(); rep.Count(AuditNotKeyboard) != 0 || rep.Count(AuditMissingName) != 0 {
		t.Fatalf("audit: %s", rep)
	}
	items := d.A11yFindAll(a11y.RoleListItem, "Beta")
	if len(items) != 2 {
		t.Fatalf("items %d", len(items))
	}
	for _, it := range items {
		if !it.States.Has(a11y.Focusable) {
			t.Fatalf("item not focusable: %+v", it)
		}
	}
	// Tab order: row Alpha, its checkbox, row Beta, its checkbox, card Alpha...
	for i := 0; i < 3; i++ {
		d.Key(input.KeyTab, 0)
	}
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleListItem || f.Name != "Beta" {
		t.Fatalf("focus: %+v", f)
	}
	d.Key(input.KeyEnter, 0)
	if len(clicked) != 1 || clicked[0] != "b" {
		t.Fatalf("Enter on row: %v", clicked)
	}
	for i := 0; i < 2; i++ {
		d.Key(input.KeyTab, 0)
	}
	if f := d.A11yFocused(); f == nil || f.Role != a11y.RoleListItem || f.Name != "Alpha" {
		t.Fatalf("card focus: %+v", f)
	}
	d.Key(input.KeySpace, 0)
	if len(sel2) != 1 || sel2[0] != "a" {
		t.Fatalf("Space on card: %v", sel2)
	}
	if n := d.A11yFindAll(a11y.RoleListItem, "Alpha")[1]; !n.States.Has(a11y.Selected) {
		t.Fatalf("card not selected: %+v", n)
	}
}

// TestPro3A11yLoginForm: the code button is not a live region (the
// countdown is announced only when it starts and ends, see
// TestP2ELoginCountdownAnnounce) and the other-login slot is a named group.
func TestPro3A11yLoginForm(t *testing.T) {
	d := pro3Driver(func() *theme.Theme { return theme.AntDesign() }, 600, 700, func(c *Context) {
		LoginForm(c).Title("Tango").MobileOnly().Actions(func() {
			Button(c, "").Icon(icon.Mail).Circle().A11yLabel("Email")
		}).Show()
	})
	d.EnableAccessibility()
	b, ok := d.A11yFind(a11y.RoleButton, ZhCN.LoginForm.GetCode)
	if !ok || b.Live != a11y.LiveOff {
		t.Fatalf("code button: %+v", b)
	}
	if _, ok := d.A11yFind(a11y.RoleGroup, ZhCN.LoginForm.Others); !ok {
		t.Fatal("other login group")
	}
	if rep := d.AuditNoContrast(); rep.Count(AuditMissingName) != 0 {
		t.Fatalf("audit: %s", rep)
	}
}
