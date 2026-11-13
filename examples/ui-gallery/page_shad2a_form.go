package main

import (
	"context"
	"errors"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Form: Element / dark variants of the main Form page, form-level
// disabled, per-field validation and clearing, and Upload picture-card
// retry.

type shadFormDemo struct {
	name, email, phone string
	agree              bool
	dis                bool
	files              []ui.UploadFile
}

var sfd = &shadFormDemo{name: "tango", dis: true, files: []ui.UploadFile{
	{Name: "photo.png", Status: "error", Err: "HTTP 500", Path: "photo.png", UID: 1 << 40},
}}

func init() {
	// Form (page_form.go) has no Element / dark copies: register them from
	// its Render (this file's init runs after page_form.go's).
	for _, p := range gallery.Pages() {
		if p.Name != "Form" {
			continue
		}
		el := p
		el.Variant = true
		el.Name, el.Title = "Form-Element", p.Title+"（Element Plus）"
		el.Setup = func(d *ui.Driver) {
			t := theme.ElementPlus()
			t.BgLayout = t.BgContainer
			d.SetTheme(t)
			d.Settle()
		}
		gallery.Register(el)
		dk := p
		dk.Variant = true
		dk.Name, dk.Title = "Form-Dark", p.Title+"（暗色）"
		dk.Setup = func(d *ui.Driver) {
			d.SetTheme(theme.AntDesign(theme.Dark()))
			d.Settle()
		}
		gallery.Register(dk)
	}

	entryPage(gallery.Page{Name: "FormFields", Parent: "Form", Title: "Form 按字段校验 / 清除、表单级禁用、隐藏必填星号", Order: 8, Width: 960, Height: 420,
		Render: func(c *ui.Context) {
			ui.Row(c, func() {
				ui.Column(c, func() {
					ui.Text(c, "按字段校验（只校验邮箱）").Tertiary()
					f := ui.Form(c).Vertical()
					f.Body(func() {
						f.Item("用户名").Rules(&sfd.name, ui.Required("请输入用户名")).Body(func() {
							ui.Input(c, &sfd.name).Placeholder("请输入用户名")
						})
						f.Item("邮箱").Rules(&sfd.email, ui.Required("请输入邮箱"), ui.Email("邮箱格式不正确")).Body(func() {
							ui.Input(c, &sfd.email).Placeholder("name@example.com")
						})
						f.Item("手机").Help("用于接收验证码").Body(func() { ui.Input(c, &sfd.phone).Placeholder("请输入手机号") })
						f.Item("").NoLabel().Body(func() {
							ui.Row(c, func() {
								if ui.Button(c, "校验邮箱").Primary().Clicked() {
									f.ValidateFields("邮箱")
								}
								if ui.Button(c, "清除校验").Clicked() {
									f.ClearValidate()
								}
							}, layout.Gap(8))
						})
					})
				}, layout.W(420))
				ui.Column(c, func() {
					ui.Text(c, "表单级禁用 + RequiredMark(false)").Tertiary()
					f := ui.Form(c).Vertical().Disabled(sfd.dis).RequiredMark(false)
					f.Body(func() {
						f.Item("用户名").Rules(&sfd.name, ui.Required("请输入用户名")).Body(func() { ui.Input(c, &sfd.name) })
						f.Item("").NoLabel().Body(func() { ui.Checkbox(c, "同意用户协议", &sfd.agree) })
						f.Item("").NoLabel().Body(func() { ui.Button(c, "提交").Primary() })
					})
					ui.Text(c, "照片墙：失败的文件悬停可重试").Tertiary()
					ui.Upload(c, &sfd.files).PictureCard().
						CustomRequest(func(ctx context.Context, f ui.UploadFile, progress func(float64)) ([]byte, error) {
							return nil, errors.New("demo")
						})
				}, layout.W(420))
			}, layout.Gap(48), layout.AlignItems(layout.AlignStart))
		},
		Setup: func(d *ui.Driver) {
			d.ClickLabel("清除校验")
			d.ClickLabel("校验邮箱")
			d.Settle()
			if r, ok := d.Find("photo.png"); ok {
				d.Move(r.Center())
				d.Settle()
			}
		},
	})
}
