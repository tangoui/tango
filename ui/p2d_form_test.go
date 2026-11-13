package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// p2dThemes runs fn with the Ant Design and Element Plus themes.
func p2dThemes(t *testing.T, fn func(t *testing.T, th *theme.Theme)) {
	t.Helper()
	for _, th := range []*theme.Theme{theme.AntDesign(), theme.ElementPlus()} {
		name := "ant"
		if th.Flavor == theme.Element {
			name = "element"
		}
		t.Run(name, func(t *testing.T) { fn(t, th) })
	}
}

func p2dDriver(th *theme.Theme, page func(c *Context)) *Driver {
	d := NewDriver(func(c *Context) { Column(c, func() { page(c) }, layout.Pad(10)) }, DriverOptions{Width: 900, Height: 700, Theme: th})
	d.Settle()
	return d
}

// p2dTypeAfter clicks the control right of a horizontal form label and
// types s.
func p2dTypeAfter(t *testing.T, d *Driver, label, s string) {
	t.Helper()
	r, ok := d.FindText(label)
	if !ok {
		t.Fatalf("label %q not found", label)
	}
	d.Click(geom.Pt(r.Right()+40, r.Y+r.H/2))
	d.Type(s)
}

func TestP2DFormRulesOf(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		age := 10.0
		var tags []string
		agree := false
		var f *FormW
		d := p2dDriver(th, func(c *Context) {
			f = Form(c)
			f.Body(func() {
				RulesOf(f.Item("年龄"), &age, RangeValue(18.0, 60.0, "18 到 60 岁")).Body(func() { InputNumber(c, &age) })
				RulesOf(f.Item("标签"), &tags, RequiredItems[string]("至少选一个"), MaxItems[string](2, "最多两个")).Body(func() {
					CheckboxGroup(c, &tags, Opts("a", "b", "c"))
				})
				RulesOf(f.Item("协议"), &agree, MustBeTrue("请同意协议")).Body(func() { Checkbox(c, "我已阅读并同意", &agree) })
			})
		})
		ok := f.Validate()
		d.Settle()
		if ok {
			t.Fatal("validate passed with invalid values")
		}
		for _, msg := range []string{"18 到 60 岁", "至少选一个", "请同意协议"} {
			if _, found := d.FindText(msg); !found {
				t.Errorf("error %q not shown", msg)
			}
		}
		// Ticking the checkbox re-validates live (onChange).
		if err := d.ClickLabel("我已阅读并同意"); err != nil {
			t.Fatal(err)
		}
		if !agree || f.Errors()["协议"] != "" {
			t.Fatalf("agree %v err %q", agree, f.Errors()["协议"])
		}
		tags = []string{"a", "b", "c"}
		d.Settle()
		if f.Errors()["标签"] != "最多两个" {
			t.Fatalf("max items: %q", f.Errors()["标签"])
		}
		age, tags = 30, []string{"a"}
		d.Settle()
		if !f.Validate() {
			t.Fatalf("validate failed: %v", f.Errors())
		}
		if f.Value("年龄") != "30" {
			t.Fatalf("value %q", f.Value("年龄"))
		}
	})
}

func TestP2DFormTooltipExtraWarning(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		name, nick := "", ""
		var f *FormW
		var tipItem *FormItemW
		d := p2dDriver(th, func(c *Context) {
			f = Form(c)
			f.Body(func() {
				tipItem = f.Item("用户名").Tooltip("用于登录的名称").Extra("4 到 16 个字符").
					Rules(&name, Required("必填"), MinLen(6, "建议至少 6 个字符").Warning())
				tipItem.Body(func() { Input(c, &name) })
				f.Item("昵称").ValidateStatus("warning").Help("昵称已被占用").Body(func() { Input(c, &nick) })
			})
		})
		d.EnableAccessibility()
		d.Settle()
		if _, ok := d.FindText("4 到 16 个字符"); !ok {
			t.Fatal("extra not shown")
		}
		// Hovering the question icon shows the tooltip.
		tr := d.r.c.rects[tipItem.tipID]
		lr, _ := d.FindText("用户名")
		if tr.Empty() || tr.X < lr.X || tr.Right() > lr.Right()+1 {
			t.Fatalf("tip icon %v outside label %v", tr, lr)
		}
		d.Move(tr.Center())
		d.Settle()
		if _, ok := d.A11yFind(a11y.RoleTooltip, "用于登录的名称"); !ok {
			t.Fatal("tooltip not shown on hover")
		}
		// A warning rule shows its message but does not fail validation.
		p2dTypeAfter(t, d, "用户名", "abc")
		if f.Errors()["用户名"] != "" {
			t.Fatalf("warning counted as error: %q", f.Errors()["用户名"])
		}
		if _, ok := d.FindText("建议至少 6 个字符"); !ok {
			t.Fatal("warning not shown")
		}
		if !f.Validate() {
			t.Fatal("warning failed validation")
		}
		d.Settle()
		in, ok := d.A11yFind(a11y.RoleTextbox, "昵称")
		if !ok {
			t.Fatal("nick textbox missing")
		}
		_ = in
		if _, ok := d.A11yFind(a11y.RoleStatus, "建议至少 6 个字符"); !ok {
			t.Fatal("warning not a status")
		}
		if _, ok := d.FindText("昵称已被占用"); !ok {
			t.Fatal("help of warning status missing")
		}
	})
}

func TestP2DFormValidateTrigger(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		a, b := "", ""
		var f *FormW
		d := p2dDriver(th, func(c *Context) {
			f = Form(c)
			f.Body(func() {
				f.Item("失焦").ValidateTrigger(ValidateOnBlur).Rules(&a, MinLen(3, "失焦太短")).Body(func() { Input(c, &a) })
				f.Item("提交").ValidateTrigger(ValidateOnSubmit).Rules(&b, MinLen(3, "提交太短")).Body(func() { Input(c, &b) })
			})
		})
		p2dTypeAfter(t, d, "失焦", "x")
		if f.Errors()["失焦"] != "" {
			t.Fatal("onBlur validated while typing")
		}
		// Moving focus to the next field blurs the first one.
		p2dTypeAfter(t, d, "提交", "y")
		if f.Errors()["失焦"] != "失焦太短" {
			t.Fatalf("onBlur: %v", f.Errors())
		}
		d.Click(geom.Pt(800, 600)) // blur the second field
		if f.Errors()["提交"] != "" {
			t.Fatal("onSubmit validated on blur")
		}
		if f.Validate() {
			t.Fatal("validate passed")
		}
		d.Settle()
		if f.Errors()["提交"] != "提交太短" {
			t.Fatalf("onSubmit: %v", f.Errors())
		}
	})
}

func TestP2DFormSizeAndReset(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		name := "初始"
		n := 3.0
		tags := []string{"a"}
		var f *FormW
		var in *InputW
		reset := false
		d := p2dDriver(th, func(c *Context) {
			if reset {
				reset = false
				f.ResetFields()
			}
			f = Form(c).Size(Large)
			f.Body(func() {
				f.Item("名称").Rules(&name, Required("必填")).Body(func() { in = Input(c, &name) })
				RulesOf(f.Item("数量"), &n).Body(func() { InputNumber(c, &n) })
				RulesOf(f.Item("标签"), &tags).Body(func() { CheckboxGroup(c, &tags, Opts("a", "b")) })
			})
		})
		if h := d.r.c.rects[in.box.id].H; h != th.ControlHeightLG {
			t.Fatalf("large form: input height %v, want %v", h, th.ControlHeightLG)
		}
		name, n = "", 9
		tags = append(tags, "b")
		d.Settle()
		if f.Errors()["名称"] != "必填" {
			t.Fatalf("not validated: %v", f.Errors())
		}
		reset = true
		d.Settle()
		if name != "初始" || n != 3 || len(tags) != 1 || tags[0] != "a" {
			t.Fatalf("reset: %q %v %v", name, n, tags)
		}
		if f.Errors()["名称"] != "" {
			t.Fatalf("reset kept errors: %v", f.Errors())
		}
	})
}

func TestP2DFormConfigDefaults(t *testing.T) {
	v := ""
	var f *FormW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		ConfigProvider(c).Defaults("Form", Props{"validateTrigger": "onSubmit", "layout": "vertical"}).Body(func() {
			f = Form(c)
			f.Body(func() {
				f.Item("字段").Rules(&v, MinLen(3, "太短")).Body(func() { Input(c, &v) })
			})
		})
	})
	if f.layout != FormVertical || f.trigger != ValidateOnSubmit {
		t.Fatalf("defaults not applied: %v %v", f.layout, f.trigger)
	}
	v = "x"
	d.Settle()
	v = "xy"
	d.Settle()
	if f.Errors()["字段"] != "" {
		t.Fatal("validated before submit")
	}
}
