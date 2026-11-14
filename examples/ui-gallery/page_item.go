package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// Item: a general list row (media, title, description, actions).

var itemDemo struct{ last string }

func itemPage(c *ui.Context) {
	gallery.Demo(c, "变体", "default 无底色、outline 带边框、muted 浅色底；右侧为操作区。", func() {
		ui.Column(c, func() {
			ui.Item(c, "基础项").Description("没有底色与边框的简单列表项。").Icon(icon.File).
				Actions(func() { ui.Button(c, "查看").Small() })
			ui.Item(c, "安全设置").Description("已开启双重验证，最近一次登录在 2 小时前。").Icon(icon.Lock).Outline().
				Actions(func() { ui.Button(c, "管理").Small() })
			ui.Item(c, "存储空间").Description("已使用 18.4 GB / 50 GB。").Icon(icon.Folder).Muted().
				Actions(func() { ui.Button(c, "升级").Small().Primary() })
		}, layout.Gap(12), layout.W(560))
	})
	gallery.Demo(c, "尺寸与可点击", "sm 更紧凑；整行可作为链接或按钮（可聚焦、Enter 执行）。", func() {
		ui.Column(c, func() {
			if ui.Item(c, "个人资料").Description("姓名、头像与联系方式").
				Media(func() { ui.Avatar(c, "张").Size(36) }).Outline().Link().
				Actions(func() { ui.Icon(c, icon.Right) }).Clicked() {
				itemDemo.last = "个人资料"
			}
			if ui.Item(c, "通知偏好").Icon(icon.Bell).Outline().Small().Clickable().
				Actions(func() { ui.Icon(c, icon.Right) }).Clicked() {
				itemDemo.last = "通知偏好"
			}
			ui.Item(c, "已停用的功能").Description("当前套餐不可用").Icon(icon.Star).Outline().Small().Clickable().Disabled(true)
			ui.Text(c, "最近点击："+itemDemo.last).Tertiary()
		}, layout.Gap(12), layout.W(560))
	})
	gallery.Demo(c, "ItemGroup", "组成列表，项之间带分隔线，可整体加边框。", func() {
		ui.Column(c, func() {
			ui.ItemGroup(c).Separated().Bordered().A11yLabel("团队成员").Body(func() {
				for _, m := range []struct{ name, mail, role string }{
					{"王小明", "xiaoming@example.com", "管理员"},
					{"李华", "lihua@example.com", "成员"},
					{"陈静", "chenjing@example.com", "访客"},
				} {
					ui.Key(c, m.name, func() {
						ui.Item(c, m.name).Description(m.mail).Small().
							Media(func() { ui.Avatar(c, string([]rune(m.name)[:1])).Size(32) }).
							Actions(func() { ui.Text(c, m.role).Tertiary() })
					})
				}
			})
		}, layout.W(560))
	})
}

func init() {
	gallery.Register(gallery.Page{Name: "Item", Title: "Item 列表项", Group: gallery.Display, Order: 35, Render: itemPage})
}
