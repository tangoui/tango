package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// Second round of form / input features: typed rules, tooltip, extra,
// warning, validate trigger, form size and reset; Input count, variants
// and custom background; InputNumber formatter, prefix, side controls and
// string mode; Upload accept / size checks, upload progress and retry.

var p2dForm = struct {
	user, nick, email string
	age               float64
	tags              []string
	agree             bool
	validated         bool
}{nick: "tango", age: 12}

var p2dIn = struct {
	count, filled, under, bare, search, plain string
}{count: "Tango UI", filled: "填充", under: "下划线", search: ""}

var p2dNum = struct {
	money, sides, prefix float64
	exact                string
}{money: 12345.6, sides: 3, prefix: 88, exact: "0.1"}

var p2dUp = struct {
	text []ui.UploadFile
	drag []ui.UploadFile
}{
	text: []ui.UploadFile{
		{Name: "合同扫描件.pdf", Size: 820000, Status: "done"},
		{Name: "设计稿-v3.fig", Size: 2400000, Status: "uploading", Percent: 62},
		{Name: "报价单.xlsx", Size: 23000, Status: "error", Path: "/tmp/报价单.xlsx", UID: -1, Err: "HTTP 500"},
	},
}

func p2dMoney(s string) string {
	ip, fp, hasF := strings.Cut(s, ".")
	neg := strings.HasPrefix(ip, "-")
	ip = strings.TrimPrefix(ip, "-")
	var b strings.Builder
	for i, r := range ip {
		if i > 0 && (len(ip)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	out := b.String()
	if hasF {
		out += "." + fp
	}
	if neg {
		out = "-" + out
	}
	return "$ " + out
}

func p2dUnmoney(s string) string { return strings.NewReplacer("$", "", ",", "", " ", "").Replace(s) }

func init() {
	entryPage(gallery.Page{Name: "FormRules", Title: "Form 提示、额外说明、警告、校验时机与类型规则", Parent: "Form", Order: 39, Width: 900, Render: func(c *ui.Context) {
		s := &p2dForm
		f := ui.Form(c).LabelWidth(110)
		defer func() {
			if s.validated {
				s.validated = false
				f.Validate() // after Body: the items exist
			}
		}()
		f.Body(func() {
			f.Item("用户名").Tooltip("用于登录，注册后不可修改").Extra("4–16 位字母、数字或下划线").
				Rules(&s.user, ui.Required("请输入用户名")).Body(func() { ui.Input(c, &s.user).Placeholder("请输入用户名") })
			f.Item("昵称").Rules(&s.nick, ui.MinLen(6, "昵称较短，建议至少 6 个字符").Warning()).Body(func() { ui.Input(c, &s.nick) })
			f.Item("邮箱").ValidateTrigger(ui.ValidateOnBlur).Help("失焦时校验").
				Rules(&s.email, ui.Email("邮箱格式不正确")).Body(func() { ui.Input(c, &s.email).Placeholder("name@example.com") })
			ui.RulesOf(f.Item("年龄"), &s.age, ui.RangeValue(18.0, 60.0, "年龄需在 18–60 之间")).Body(func() { ui.InputNumber(c, &s.age) })
			ui.RulesOf(f.Item("兴趣"), &s.tags, ui.RequiredItems[string]("至少选择一项")).Body(func() {
				ui.CheckboxGroup(c, &s.tags, ui.Opts("阅读", "运动", "音乐"))
			})
			ui.RulesOf(f.Item("协议").NoLabel(), &s.agree, ui.MustBeTrue("请先同意用户协议")).Body(func() {
				ui.Checkbox(c, "我已阅读并同意用户协议", &s.agree)
			})
			f.Item("").NoLabel().Body(func() {
				ui.Row(c, func() {
					if ui.Button(c, "提交").Primary().Clicked() {
						f.Validate()
					}
					if ui.Button(c, "重置").Clicked() {
						f.ResetFields()
					}
				}, layout.Gap(8))
			})
		})
	}, Setup: func(d *ui.Driver) {
		p2dForm.validated = true
		d.Settle()
	}})

	entryPage(gallery.Page{Name: "FormSize", Title: "Form 尺寸（small / middle / large）", Parent: "Form", Order: 39, Width: 900, Render: func(c *ui.Context) {
		s := &p2dForm
		for _, sz := range []ui.ControlSize{ui.Small, ui.Middle, ui.Large} {
			f := ui.Form(c).Inline().Size(sz)
			f.Body(func() {
				f.Item("名称").Body(func() { ui.Input(c, &s.nick).Width(160) })
				f.Item("数量").Body(func() { ui.InputNumber(c, &s.age) })
				f.Item("").NoLabel().Body(func() { ui.Button(c, "查询").Primary() })
			})
		}
	}})

	entryPage(gallery.Page{Name: "InputVariants", Title: "Input 字数统计、外观变体与自定义底色", Parent: "Input", Order: 1, Render: func(c *ui.Context) {
		s := &p2dIn
		ui.Column(c, func() {
			ui.Input(c, &s.count).ShowCount().MaxLength(20).A11yLabel("字数统计").Width(320)
			ui.Input(c, &s.plain).Placeholder("outlined（默认）").Width(320)
			ui.Input(c, &s.filled).Variant("filled").A11yLabel("filled").Width(320)
			ui.Input(c, &s.bare).Variant("borderless").Placeholder("borderless").Width(320)
			ui.Input(c, &s.under).Variant("underlined").A11yLabel("underlined").Width(320)
			ui.Input(c, &s.search).Prefix(icon.Search).Placeholder("灰底无边框（移动端搜索栏）").
				Bg(c.Theme.FillTertiary).NoBorder().Width(320)
			ui.Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 320, H: 36} }, nil).
				Bg(c.Theme.Primary.Bg).Radius(c.Theme.Radius).Label("自绘区域（Custom.Bg + Label）")
		}, layout.Gap(12))
	}})

	entryPage(gallery.Page{Name: "InputNumberMore", Title: "InputNumber 格式化、前缀、两侧按钮与高精度", Parent: "InputNumber", Order: 2, Render: func(c *ui.Context) {
		s := &p2dNum
		ui.Column(c, func() {
			ui.Row(c, func() {
				ui.InputNumber(c, &s.money).Formatter(p2dMoney).Parser(p2dUnmoney).Width(180).A11yLabel("金额")
				ui.InputNumber(c, &s.prefix).PrefixText("¥").Width(140).A11yLabel("价格")
				ui.InputNumber(c, &s.prefix).Prefix(icon.User).Width(140).Wheel().A11yLabel("人数")
			}, layout.Gap(12), layout.Middle())
			ui.Row(c, func() {
				ui.InputNumber(c, &s.sides).ControlsPosition("sides").Min(1).Max(10).A11yLabel("数量")
				ui.InputNumber(c, &s.sides).ControlsPosition("sides").Size(ui.Small).Min(1).Max(10).A11yLabel("数量（小）")
				ui.InputNumber(c, nil).StringMode(&s.exact).Step(0.000000000000000001).Width(260).A11yLabel("高精度")
			}, layout.Gap(12), layout.Middle())
		}, layout.Gap(16))
	}})

	entryPage(gallery.Page{Name: "UploadMore", Title: "Upload 类型与大小校验、上传进度、失败重试", Parent: "Upload", Order: 72, Height: 560, Render: func(c *ui.Context) {
		s := &p2dUp
		entryRow(c, func() {
			entryCol(c, func() {
				ui.Text(c, "上传列表（失败可重试）").Tertiary()
				ui.Upload(c, &s.text).Action("http://127.0.0.1:9/upload").Width(360)
			})
			entryCol(c, func() {
				ui.Text(c, "拖拽区：仅图片、≤ 1 KB").Tertiary()
				ui.Upload(c, &s.drag).Dragger().Accept("image/*").MaxSize(1024).
					BeforeUpload(func(f ui.UploadFile) error {
						if strings.HasPrefix(f.Name, "blocked") {
							return errors.New(f.Name + " 被 BeforeUpload 拒绝")
						}
						return nil
					}).DragText("将图片拖到此处", "支持 png / jpg，单个不超过 1 KB").Width(420)
			})
		})
	}, Setup: func(d *ui.Driver) {
		// Drop a valid image, an oversized one and a text file on the
		// dragger.
		dir, err := os.MkdirTemp("", "p2d-upload")
		if err != nil {
			return
		}
		mk := func(name string, n int) string {
			p := filepath.Join(dir, name)
			_ = os.WriteFile(p, make([]byte, n), 0o644)
			return p
		}
		paths := []string{mk("logo.png", 300), mk("photo.jpg", 4096), mk("readme.txt", 10)}
		r, ok := d.Find("dragger")
		if !ok {
			return
		}
		p2dUp.drag = nil
		d.DropFiles(r.Center(), paths...)
		d.Settle()
	}})
}
