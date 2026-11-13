package ui

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// typeAfterLabel clicks the control right of a horizontal form label and
// types s.
func typeAfterLabel(t *testing.T, d *Driver, label, s string) {
	t.Helper()
	r := painted(d, label)
	if r.Empty() {
		t.Fatalf("label %q not found", label)
	}
	d.Click(geom.Pt(r.Right()+30, r.Y+r.H/2))
	d.Type(s)
}

func formDriver(page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) { Column(c, func() { page(c) }, layout.Pad(10)) }, DriverOptions{Width: 900, Height: 700})
	d.Settle()
	return d
}

// ---------------------------------------------------------------------------
// Form.List

type flRow struct{ name string }

func TestFormListAddRemoveMoveValidate(t *testing.T) {
	rows := []flRow{{"a"}, {""}}
	var f *FormW
	var fl *FormListW[flRow]
	d := formDriver(func(c *Context) {
		f = Form(c)
		f.Body(func() {
			fl = FormList(f, "成员", &rows).Min(1).Max(3).Creator(func() flRow { return flRow{"new"} }).
				Body(func(row *FormListRow[flRow]) {
					f.Item("姓名").Name(row.Name("name")).Rules(&row.Value.name, Required("必填")).Body(func() {
						Input(c, &row.Value.name).Width(120)
					})
				})
		})
	})
	// Validation runs per row: only the empty row fails.
	if f.Validate() {
		t.Fatal("empty row passed")
	}
	d.Settle()
	errs := 0
	for k, v := range f.Errors() {
		if v != "" {
			errs++
			if !strings.HasPrefix(k, "成员.") || !strings.HasSuffix(k, ".name") {
				t.Fatalf("error key %q", k)
			}
		}
	}
	if errs != 1 {
		t.Fatalf("errors %v", f.Errors())
	}
	loc := ZhCN.Form
	if err := d.ClickLabel(loc.AddRow); err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 || rows[2].name != "new" {
		t.Fatalf("add: %v", rows)
	}
	d.ClickLabel(loc.AddRow) // at Max: disabled
	if len(rows) != 3 {
		t.Fatalf("add beyond max: %v", rows)
	}
	// Move the new row up (the last "上移" button belongs to it).
	clickNth(t, d, loc.MoveUp, 2)
	if fmt.Sprint(rows) != "[{a} {new} {}]" {
		t.Fatalf("move up: %v", rows)
	}
	clickNth(t, d, loc.MoveDown, 0)
	if fmt.Sprint(rows) != "[{new} {a} {}]" {
		t.Fatalf("move down: %v", rows)
	}
	// Errors follow their row through moves: the empty row still fails.
	f2 := f
	if f2.Validate() {
		t.Fatal("still invalid")
	}
	clickNth(t, d, ZhCN.Delete, 2)
	clickNth(t, d, ZhCN.Delete, 1)
	if fmt.Sprint(rows) != "[{new}]" {
		t.Fatalf("delete: %v", rows)
	}
	clickNth(t, d, ZhCN.Delete, 0) // at Min
	if len(rows) != 1 {
		t.Fatalf("delete below min: %v", rows)
	}
	d.Settle()
	if !f.Validate() {
		t.Fatalf("valid list failed: %v", f.Errors())
	}
	_ = fl
}

// clickNth clicks the i-th widget labelled label in build order.
func clickNth(t *testing.T, d *Driver, label string, i int) {
	t.Helper()
	var rs []geom.Rect
	for _, h := range d.r.c.hits {
		if h.label == label {
			rs = append(rs, h.rect)
		}
	}
	if i >= len(rs) {
		t.Fatalf("%q #%d not found (%d)", label, i, len(rs))
	}
	d.Click(rs[i].Center())
}

func TestFormListRule(t *testing.T) {
	var rows []flRow
	var f *FormW
	d := formDriver(func(c *Context) {
		f = Form(c)
		f.Body(func() {
			FormList(f, "成员", &rows).Rule(func(n int) string {
				if n == 0 {
					return "至少一位成员"
				}
				return ""
			}).Body(func(row *FormListRow[flRow]) {})
		})
	})
	if f.Validate() || f.Errors()["成员.__list"] != "至少一位成员" {
		t.Fatalf("list rule: %v", f.Errors())
	}
	d.ClickLabel(ZhCN.Form.AddRow)
	if !f.Validate() {
		t.Fatalf("list rule after add: %v", f.Errors())
	}
}

// ---------------------------------------------------------------------------
// Linkage and validation.

func TestFormDependenciesHiddenDisabled(t *testing.T) {
	kind, company, province, city, pwd, confirm := "person", "", "zj", "hz", "abc", "abc"
	resets := 0
	var f *FormW
	var cityDisabled bool
	d := formDriver(func(c *Context) {
		f = Form(c)
		f.Body(func() {
			f.Item("类型").Name("kind").RulesFunc(func() string { return kind }).Body(func() { Text(c, kind) })
			f.Item("企业").Name("company").Hidden(f.Value("kind") != "company").Rules(&company, Required("请输入企业")).
				Body(func() { Input(c, &company) })
			f.Item("省").Name("province").RulesFunc(func() string { return province }).Body(func() { Text(c, province) })
			f.Item("市").Name("city").Dependencies("province").OnDepsChange(func() { city = ""; resets++ }).
				Disabled(province == "").Rules(&city, Required("请选择城市")).Body(func() {
				in := Input(c, &city)
				cityDisabled = in.box.disabled
			})
			f.Item("密码").Name("pwd").Rules(&pwd).Body(func() { Input(c, &pwd) })
			f.Item("确认").Name("confirm").Dependencies("pwd").Rules(&confirm, Match(&pwd, "不一致")).Body(func() { Input(c, &confirm) })
		})
	})
	if !f.Validate() {
		t.Fatalf("initial: %v", f.Errors())
	}
	if _, ok := tblNode(d, "企业"); ok {
		t.Fatal("hidden item built")
	}
	kind = "company"
	d.Settle()
	d.Settle()
	if _, ok := tblNode(d, "企业"); !ok {
		t.Fatal("item not shown after its dependency changed")
	}
	if f.Validate() || f.Errors()["company"] == "" {
		t.Fatalf("shown item not validated: %v", f.Errors())
	}
	// Changing the province resets the city (option refresh) and the
	// re-validation shows the city error.
	province = "js"
	d.Settle()
	if city != "" || resets != 1 {
		t.Fatalf("city not reset: %q (%d)", city, resets)
	}
	if f.Errors()["city"] == "" {
		t.Fatalf("dependent field not re-validated: %v", f.Errors())
	}
	province = ""
	d.Settle()
	if !cityDisabled {
		t.Fatal("city not disabled")
	}
	// Changing the password re-checks the confirmation (cross-field).
	pwd = "abcd"
	d.Settle()
	if f.Errors()["confirm"] != "不一致" {
		t.Fatalf("confirm not re-validated: %v", f.Errors())
	}
}

func TestFormAsyncValidationAndSubmit(t *testing.T) {
	user := "admin"
	var pending []func(string)
	var f *FormW
	finished := 0
	d := formDriver(func(c *Context) {
		f = Form(c)
		f.Body(func() {
			f.Item("用户名").Name("user").Rules(&user, Required("必填"), AsyncCheck(func(v string, done func(string)) {
				pending = append(pending, func(msg string) { done(msg) })
			})).Body(func() { Input(c, &user) })
			if Button(c, "提交").Clicked() {
				f.Submit()
			}
		})
		if f.Finished() {
			finished++
		}
	})
	d.ClickLabel("提交")
	if len(pending) != 1 || !f.Validating() || !f.Submitting() {
		t.Fatalf("async rule not started: %d %v %v", len(pending), f.Validating(), f.Submitting())
	}
	if _, ok := tblNode(d, ZhCN.Form.Validating); !ok {
		t.Fatal("validating hint missing")
	}
	pending[0]("已被占用")
	d.Settle()
	if f.Validating() || f.Errors()["user"] != "已被占用" || finished != 0 || f.Submitting() {
		t.Fatalf("async error: %v finished %d", f.Errors(), finished)
	}
	// Same value: no new request. New value: a new one; a pass finishes.
	d.ClickLabel("提交")
	if len(pending) != 1 {
		t.Fatalf("re-validated the same value: %d", len(pending))
	}
	user = "tango"
	d.Settle()
	d.ClickLabel("提交")
	if len(pending) != 2 {
		t.Fatalf("new value not validated: %d", len(pending))
	}
	pending[1]("")
	d.Settle()
	if finished != 1 || f.Errors()["user"] != "" {
		t.Fatalf("submit after async pass: finished %d errs %v", finished, f.Errors())
	}
	// A stale result (value changed meanwhile) is ignored.
	user = "x"
	d.Settle()
	d.ClickLabel("提交")
	user = "y"
	d.Settle()
	pending[2]("旧结果")
	d.Settle()
	if f.Errors()["user"] == "旧结果" {
		t.Fatal("stale async result applied")
	}
}

func TestFormSubmitScrollsToFirstError(t *testing.T) {
	vals := make([]string, 12)
	var box *ScrollBox
	var f *FormW
	d := formDriver(func(c *Context) {
		box = Scroll(c, func() {
			f = Form(c).ScrollToFirstError()
			f.Body(func() {
				for i := range vals {
					it := f.Item(fmt.Sprintf("字段%d", i))
					if i == 9 {
						it.Rules(&vals[i], Required("第 9 项必填"))
					}
					it.Body(func() { Input(c, &vals[i]) })
				}
			})
			if Button(c, "提交").Clicked() {
				f.Submit()
			}
		}, layout.H(200))
	})
	box.ScrollTo(0, 1e6) // the submit button at the bottom
	d.Settle()
	d.ClickLabel("提交")
	d.Settle()
	d.Settle()
	view := d.r.c.rects[box.ID()]
	r := painted(d, "字段9")
	if r.Y < view.Y-0.5 || r.Bottom() > view.Bottom()+0.5 {
		t.Fatalf("first error not scrolled into view: %v in %v", r, view)
	}
}

func TestFormTypingValidatesLive(t *testing.T) {
	name := ""
	var f *FormW
	d := formDriver(func(c *Context) {
		f = Form(c)
		f.Body(func() {
			f.Item("名称").Rules(&name, Required("必填"), MinLen(3, "至少 3 个字符")).Body(func() { Input(c, &name) })
		})
	})
	typeAfterLabel(t, d, "名称", "ab")
	if name != "ab" || f.Errors()["名称"] != "至少 3 个字符" {
		t.Fatalf("live: %q %v", name, f.Errors())
	}
}

// ---------------------------------------------------------------------------
// ProForm.

func proTestFields() []ProField {
	return []ProField{
		{Key: "name", Label: "名称", Required: true},
		{Key: "amount", Label: "金额", ValueType: ValueMoney},
		{Key: "max", Label: "上限", ValueType: ValueDigit, Dependencies: []string{"amount"},
			Validate: func(v any, all ProValues) string {
				if _, ok := v.(float64); ok && all.Float("max") < all.Float("amount") {
					return "上限过小"
				}
				return ""
			}},
		{Key: "province", Label: "省", ValueType: ValueSelect, ValueEnum: []ProEnum{{Value: "zj", Label: "浙江"}, {Value: "js", Label: "江苏"}}},
		{Key: "city", Label: "市", ValueType: ValueSelect, Dependencies: []string{"province"},
			Options: func(v ProValues) []ProEnum {
				if v.String("province") == "zj" {
					return []ProEnum{{Value: "hz", Label: "杭州"}}
				}
				return []ProEnum{{Value: "nj", Label: "南京"}}
			},
			Disable: func(v ProValues) bool { return v.String("province") == "" }},
		{Key: "secret", Label: "备注", Hide: func(v ProValues) bool { return v.Bool("plain") }},
		{Key: "plain", Label: "简洁", ValueType: ValueSwitch},
		{Key: "status", Label: "状态", ValueType: ValueRadio, Initial: "on", ValueEnum: []ProEnum{{Value: "on", Label: "开"}, {Value: "off", Label: "关"}}},
	}
}

func TestProFormRequestSubmitReset(t *testing.T) {
	var vals ProValues
	var initDone func(ProValues, error)
	var subDone func(error)
	var got ProValues
	var pf *ProFormW
	d := formDriver(func(c *Context) {
		pf = ProForm(c, proTestFields()).Columns(2).Values(&vals).
			Request(func(done func(ProValues, error)) { initDone = done }).
			OnFinish(func(v ProValues, done func(error)) { got, subDone = v, done }).Show()
	})
	if !pf.Loading() || initDone == nil {
		t.Fatal("initial request not pending")
	}
	if vals.String("status") != "on" {
		t.Fatalf("Initial not applied: %v", vals)
	}
	initDone(ProValues{"name": "规则", "amount": 100.0, "max": 50.0, "province": "zj", "city": "hz"}, nil)
	d.Settle()
	if pf.Loading() || vals.String("name") != "规则" {
		t.Fatalf("initial values: %v", vals)
	}
	// Cross-field rule: submit fails on 上限 < 金额.
	d.ClickLabel(ZhCN.Form.Submit)
	d.Settle()
	if subDone != nil || pf.Form().Errors()["max"] != "上限过小" {
		t.Fatalf("cross-field: %v", pf.Form().Errors())
	}
	vals["max"] = 200.0
	d.Settle()
	d.ClickLabel(ZhCN.Form.Submit)
	d.Settle()
	if subDone == nil || got.Float("max") != 200 || !pf.Submitting() {
		t.Fatalf("OnFinish not called / not loading: %v %v", got, pf.Submitting())
	}
	subDone(errors.New("服务器错误"))
	d.Settle()
	if pf.Submitting() || pf.Err() == nil {
		t.Fatal("submit error not kept")
	}
	if _, ok := tblNode(d, "服务器错误"); !ok {
		t.Fatal("error alert missing")
	}
	// Reset restores the loaded values.
	vals["name"] = "改了"
	d.ClickLabel(ZhCN.Reset)
	d.Settle()
	if vals.String("name") != "规则" || pf.Err() != nil {
		t.Fatalf("reset: %v", vals)
	}
}

func TestProFormLinkage(t *testing.T) {
	vals := ProValues{"name": "n", "province": "zj", "city": "hz"}
	var pf *ProFormW
	d := formDriver(func(c *Context) {
		pf = ProForm(c, proTestFields()).Values(&vals).Show()
	})
	if _, ok := tblNode(d, "备注"); !ok {
		t.Fatal("备注 missing")
	}
	vals["plain"] = true
	d.Settle()
	if _, ok := tblNode(d, "备注"); ok {
		t.Fatal("hidden field still built")
	}
	// The province changes: the city option is gone and the value dropped.
	vals["province"] = "js"
	d.Settle()
	if _, ok := vals["city"]; ok {
		t.Fatalf("stale option kept: %v", vals)
	}
	if !pf.Form().Validate() {
		t.Fatalf("errors: %v", pf.Form().Errors())
	}
	_ = d
}

func TestStepsFormValidatesEachStep(t *testing.T) {
	var vals ProValues
	var sf *StepsFormW
	var final ProValues
	d := formDriver(func(c *Context) {
		sf = StepsForm(c, []StepsFormStep{
			{Title: "一", Fields: []ProField{{Key: "a", Label: "甲", Required: true}}},
			{Title: "二", Fields: []ProField{{Key: "b", Label: "乙", Required: true}}},
		}).Values(&vals).OnFinish(func(v ProValues, done func(error)) { final = v; done(nil) }).Show()
	})
	loc := ZhCN.Form
	d.ClickLabel(loc.NextStep)
	d.Settle()
	if sf.Current() != 0 {
		t.Fatal("moved on with an invalid step")
	}
	vals["a"] = "1"
	d.Settle()
	d.ClickLabel(loc.NextStep)
	d.Settle()
	if sf.Current() != 1 {
		t.Fatalf("not on step 2: %d", sf.Current())
	}
	d.ClickLabel(loc.PrevStep)
	if sf.Current() != 0 || vals.String("a") != "1" {
		t.Fatal("prev lost values")
	}
	d.ClickLabel(loc.NextStep)
	d.Settle()
	vals["b"] = "2"
	d.Settle()
	d.ClickLabel(loc.Submit)
	d.Settle()
	if final.String("a") != "1" || final.String("b") != "2" {
		t.Fatalf("final values %v", final)
	}
}

func TestModalFormClosesAfterSubmit(t *testing.T) {
	open := false
	var vals ProValues
	finished := 0
	d := NewDriver(func(c *Context) {
		if Button(c, "打开").Clicked() {
			open = true
		}
		if ModalForm(c, &open, "新建", []ProField{{Key: "name", Label: "名称", Required: true}}).Values(&vals).
			OnFinish(func(v ProValues, done func(error)) { done(nil) }).Show().Finished() {
			finished++
		}
	}, DriverOptions{Width: 900, Height: 700})
	d.Settle()
	d.ClickLabel("打开")
	d.Settle()
	d.ClickLabel(ZhCN.OK)
	d.Settle()
	if !open {
		t.Fatal("closed with an invalid form")
	}
	vals["name"] = "x"
	d.Settle()
	d.ClickLabel(ZhCN.OK)
	d.Settle()
	if open || finished != 1 {
		t.Fatalf("modal form: open %v finished %d", open, finished)
	}
	// Reopening starts with fresh values.
	d.ClickLabel("打开")
	d.Settle()
	if vals.String("name") != "" {
		t.Fatalf("values kept: %v", vals)
	}
}

func TestDrawerFormSubmit(t *testing.T) {
	open := true
	var vals ProValues
	var got ProValues
	d := NewDriver(func(c *Context) {
		DrawerForm(c, &open, "编辑", []ProField{{Key: "name", Label: "名称", Required: true}}).Values(&vals).
			OnFinish(func(v ProValues, done func(error)) { got = v; done(nil) }).Show()
	}, DriverOptions{Width: 900, Height: 700})
	d.Settle()
	vals["name"] = "y"
	d.Settle()
	d.ClickLabel(ZhCN.OK)
	d.Settle()
	if open || got.String("name") != "y" {
		t.Fatalf("drawer form: open %v got %v", open, got)
	}
}

func TestQueryFilterSearchResetCollapse(t *testing.T) {
	var q *QueryFilterW
	var vals ProValues
	searches := 0
	var params map[string]string
	fields := []ProField{{Key: "a", Label: "甲"}, {Key: "b", Label: "乙"}, {Key: "c", Label: "丙"}, {Key: "d", Label: "丁", ValueType: ValueSelect, ValueEnum: []ProEnum{{Value: "1", Label: "一"}}}}
	d := NewDriver(func(c *Context) {
		q = QueryFilter(c, fields).Values(&vals).Show()
		if q.Searched() {
			searches++
			params = q.Params()
		}
	}, DriverOptions{Width: 1000, Height: 600})
	d.Settle()
	d.Settle()
	// 1000px wide: 3 cells per row, so two fields and the buttons show.
	if _, ok := tblNode(d, "丙："); ok {
		t.Fatal("collapsed field shown")
	}
	d.ClickLabel(ZhCN.Expand)
	d.Settle()
	if _, ok := tblNode(d, "丁："); !ok {
		t.Fatal("expand did not show all fields")
	}
	typeAfterLabel(t, d, "甲：", "abc")
	vals["d"] = "1"
	d.ClickLabel(ZhCN.Form.Query)
	if searches != 1 || params["a"] != "abc" || params["d"] != "1" {
		t.Fatalf("search: %d %v", searches, params)
	}
	d.ClickLabel(ZhCN.Reset)
	if searches != 2 || len(params) != 0 || vals.String("a") != "" {
		t.Fatalf("reset: %d %v %v", searches, params, vals)
	}
}
