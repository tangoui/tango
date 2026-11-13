package main

import (
	"strings"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

func init() {
	fpPage("FormDependency", "Form 字段联动与校验增强", 32, 960, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		var kind = "company"
		var company, province, city, user, pwd, confirm, email string
		notify := false
		province, city = "zj", "hz"
		user, pwd, confirm = "admin", "abc12345", "abc1234"
		return func(c *ui.Context) {
				tbDemo(c, "依赖字段：显示/隐藏、禁用、选项刷新、跨字段与异步校验", "企业名称仅在「企业」时显示；城市选项随省份刷新（省份变化时清空城市）；关闭通知时邮箱禁用；确认密码依赖密码重新校验；用户名异步校验（admin 已被占用）。", func() {
					f := ui.Form(c).LabelWidth(100).ScrollToFirstError()
					f.Body(func() {
						f.Item("主体类型").Name("kind").RulesFunc(func() string { return kind }).Body(func() {
							ui.Row(c, func() {
								ui.RadioGroup(c, &kind, []ui.Option{{Label: "个人", Value: "person"}, {Label: "企业", Value: "company"}}).Buttons()
							})
						})
						f.Item("企业名称").Name("company").Hidden(f.Value("kind") != "company").
							Rules(&company, ui.Required("请输入企业名称")).Body(func() {
							ui.Input(c, &company).Placeholder("请输入营业执照上的名称")
						})
						f.Item("省份").Name("province").RulesFunc(func() string { return province }).Body(func() {
							ui.Select(c, &province, opts(fpProvinces)).Placeholder("请选择省份")
						})
						f.Item("城市").Name("city").Dependencies("province").OnDepsChange(func() { city = "" }).
							Disabled(province == "").Rules(&city, ui.Required("请选择城市")).Body(func() {
							ui.Select(c, &city, opts(fpCities[province])).Placeholder("请选择城市")
						})
						f.Item("用户名").Name("user").Rules(&user, ui.Required("请输入用户名"), ui.AsyncCheck(func(v string, done func(string)) {
							if strings.EqualFold(v, "admin") {
								done("用户名已被占用")
								return
							}
							done("")
						})).Body(func() { ui.Input(c, &user) })
						f.Item("密码").Name("pwd").Rules(&pwd, ui.Required("请输入密码"), ui.MinLen(8, "至少 8 位")).Body(func() {
							ui.Input(c, &pwd).Password()
						})
						f.Item("确认密码").Name("confirm").Dependencies("pwd").Rules(&confirm, ui.Required("请再次输入密码"), ui.Match(&pwd, "两次输入的密码不一致")).Body(func() {
							ui.Input(c, &confirm).Password()
						})
						f.Item("邮件通知").Body(func() { ui.Row(c, func() { ui.Switch(c, &notify) }, layout.H(32), layout.Middle()) })
						f.Item("通知邮箱").Name("email").Disabled(!notify).Rules(&email, ui.Email("邮箱格式不正确")).Body(func() {
							ui.Input(c, &email).Placeholder("开启邮件通知后填写")
						})
						f.Item("").NoLabel().Body(func() {
							ui.Row(c, func() {
								if ui.Button(c, "提交").Primary().Loading(f.Submitting()).Clicked() {
									f.Submit()
								}
							}, layout.Gap(8))
						})
					})
				})
			}, func(d *ui.Driver) {
				d.ClickLabel("提交")
			}
	})
}

func opts(es []ui.ProEnum) []ui.Option {
	out := make([]ui.Option, len(es))
	for i, e := range es {
		out[i] = ui.Option{Label: e.Label, Value: e.Value}
	}
	return out
}
