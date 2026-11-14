package main

import (
	"image"
	"image/color"
	"math"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// p3Logo is a small brand mark.
func p3Logo(c *ui.Context) {
	th := c.Theme
	ui.Row(c, func() {
		ui.Text(c, "T").Size(24).Strong().Color(th.TextOnPrimary)
	}, layout.W(44), layout.H(44), layout.Center(), layout.Middle()).Bg(th.Primary.Base).Radius(10)
}

// p3OtherLogins is the Actions slot: icon-only buttons, so each one gets
// an accessible name.
func p3OtherLogins(c *ui.Context) {
	for _, m := range []struct {
		ic   *icon.Icon
		name string
	}{{icon.Message, "短信登录"}, {icon.Global, "第三方账号登录"}, {icon.Mail, "邮箱登录"}} {
		ui.Button(c, "").Icon(m.ic).Circle().A11yLabel(m.name)
	}
}

var p3BgImg image.Image

// p3Background is a generated landscape-like gradient image.
func p3Background() image.Image {
	if p3BgImg != nil {
		return p3BgImg
	}
	w, h := 320, 200
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			fx, fy := float64(x)/float64(w), float64(y)/float64(h)
			wave := 0.5 + 0.5*math.Sin(fx*6+fy*3)
			r := uint8(22 + 40*fy + 30*wave)
			g := uint8(80 + 60*fx + 20*wave)
			b := uint8(160 + 80*(1-fy))
			img.Set(x, y, color.RGBA{r, g, b, 255})
		}
	}
	p3BgImg = img
	return img
}

func init() {
	pro3Page("LoginForm", "LoginForm 登录表单", gallery.Form, 37, 960, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		return func(c *ui.Context) {
				tbDemo(c, "账号 / 手机号登录", "Logo、标题、副标题；两个 Tab；验证码按钮倒计时；自动登录、忘记密码；其他登录方式插槽；提交时按钮转圈。", func() {
					ui.Row(c, func() {
						ui.LoginForm(c).Logo(func() { p3Logo(c) }).Title("Tango").SubTitle("即时模式自绘 GUI 的管理后台模板").
							OnForgot(func() {}).Actions(func() { p3OtherLogins(c) }).
							OnFinish(func(v ui.ProValues, done func(error)) { done(nil) }).Show()
						ui.LoginForm(c).Logo(func() { p3Logo(c) }).Title("Tango").SubTitle("手机号登录").MobileOnly().CodeInterval(60).
							OnFinish(func(v ui.ProValues, done func(error)) { done(nil) }).Show()
					}, layout.Gap(64), layout.Center(), layout.AlignItems(layout.AlignStart))
				})
			}, func(d *ui.Driver) {
				d.ClickLabel("登录") // empty: shows the required messages
				// The second form: type a number and request a code.
				_ = d.ClickLabel("获取验证码")
			}
	})
	pro3Page("LoginFormPage", "LoginFormPage 登录页", gallery.Form, 38, 1200, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		return func(c *ui.Context) {
			ui.LoginFormPage(c).Logo(func() { p3Logo(c) }).Title("Tango").SubTitle("一套代码，桌面 / Web / 移动端").
				BackgroundImage(p3Background()).
				Side(func() {
					th := c.Theme
					ui.Column(c, func() {
						ui.Text(c, "Tango 1.0 发布").Size(36).Strong().Color(th.TextOnSolid)
						ui.Text(c, "对标 Ant Design 5/6 与 Element Plus 的自绘组件库").Size(18).Color(th.TextOnSolid)
					}, layout.Gap(12))
				}).
				OnForgot(func() {}).Actions(func() { p3OtherLogins(c) }).Show()
		}, nil
	})
}
