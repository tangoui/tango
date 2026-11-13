package main

import (
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

type fpContact struct{ name, phone, role string }

func init() {
	fpPage("FormList", "Form.List 动态增减表单项", 31, 960, 0, func() (func(*ui.Context), func(*ui.Driver)) {
		contacts := []fpContact{{name: "曲丽丽", phone: "13800000000", role: "owner"}, {name: "", phone: "1380"}}
		var project string
		roles := []ui.Option{{Label: "负责人", Value: "owner"}, {Label: "开发", Value: "dev"}, {Label: "测试", Value: "qa"}}
		return func(c *ui.Context) {
				tbDemo(c, "动态增减联系人", "FormList 绑定切片：添加、删除、上移下移；每一行的字段各自校验（row.Name 生成唯一字段名），Rule 校验列表本身（至少 1 行，最多 5 行）。", func() {
					f := ui.Form(c).LabelWidth(90).ScrollToFirstError()
					f.Body(func() {
						f.Item("项目名称").Rules(&project, ui.Required("请输入项目名称")).Body(func() {
							ui.Input(c, &project).Placeholder("请输入")
						})
						ui.FormList(f, "联系人", &contacts).Min(1).Max(5).
							Creator(func() fpContact { return fpContact{role: "dev"} }).
							Rule(func(n int) string {
								if n < 2 {
									return "至少需要 2 位联系人"
								}
								return ""
							}).
							Body(func(row *ui.FormListRow[fpContact]) {
								v := row.Value
								f.Item("姓名").Name(row.Name("name")).Rules(&v.name, ui.Required("请输入姓名")).Body(func() {
									ui.Input(c, &v.name).Placeholder("姓名").Width(140)
								})
								f.Item("手机").Name(row.Name("phone")).Rules(&v.phone, ui.Required("请输入手机号"), ui.Pattern(`^1\d{10}$`, "手机号格式不正确")).Body(func() {
									ui.Input(c, &v.phone).Placeholder("11 位手机号").Width(160)
								})
								f.Item("角色").Name(row.Name("role")).Body(func() {
									ui.Select(c, &v.role, roles).Width(110)
								})
							})
						f.Item("").NoLabel().Body(func() {
							ui.Row(c, func() {
								if ui.Button(c, "提交").Primary().Clicked() {
									f.Submit()
								}
								if ui.Button(c, "重置").Clicked() {
									f.ResetValidation()
								}
							}, layout.Gap(8))
						})
						if f.Finished() {
							ui.Message(c, "success", "提交成功")
						}
					})
				})
				_ = geom.Pt
			}, func(d *ui.Driver) {
				d.ClickLabel("添加一行数据")
				d.ClickLabel("提交")
			}
	})
}
