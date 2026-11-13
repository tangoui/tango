package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

func row(c *ui.Context, fn func()) {
	ui.Row(c, fn, layout.Gap(8), layout.Middle(), layout.WrapItems(), layout.Margin(geomBottom(12)))
}

func init() {
	gallery.Register(gallery.Page{Name: "Button", Title: "Button 按钮", Group: gallery.General, Order: 1, Render: func(c *ui.Context) {
		row(c, func() {
			ui.Button(c, "主按钮").Primary()
			ui.Button(c, "默认按钮")
			ui.Button(c, "虚线按钮").Dashed()
			ui.Button(c, "文本按钮").Text()
			ui.Button(c, "链接按钮").Link()
		})
		row(c, func() {
			ui.Button(c, "危险主按钮").Primary().Danger()
			ui.Button(c, "危险默认").Danger()
			ui.Button(c, "危险文本").Text().Danger()
			ui.Button(c, "禁用").Primary().Disabled(true)
			ui.Button(c, "禁用").Disabled(true)
		})
		row(c, func() {
			ui.Button(c, "搜索").Primary().Icon(icon.Search)
			ui.Button(c, "下载").Icon(icon.Download)
			ui.Button(c, "").Primary().Icon(icon.Search).Circle().A11yLabel("搜索")
			ui.Button(c, "").Icon(icon.Edit).A11yLabel("编辑")
			ui.Button(c, "加载中").Primary().Loading(true)
			ui.Button(c, "圆角").Round()
		})
		row(c, func() {
			ui.Button(c, "大号").Primary().Large()
			ui.Button(c, "中号").Primary()
			ui.Button(c, "小号").Primary().Small()
			ui.Button(c, "小号默认").Small()
		})
	}})
	gallery.Register(gallery.Page{Name: "Typography", Title: "Typography 排版", Group: gallery.General, Order: 2, Render: func(c *ui.Context) {
		ui.Title(c, "h1. Tango 设计语言", 1)
		ui.Title(c, "h3. 自绘组件库", 3)
		ui.Paragraph(c, "Tango UI 是一个纯 Go 实现的自绘组件库，视觉对标 Ant Design 与 Element Plus。中文排版按字断行，并遵循基本的避头尾规则：句号、逗号不会出现在行首。")
		row(c, func() {
			ui.Text(c, "默认文本")
			ui.Text(c, "次要文本").Secondary()
			ui.Text(c, "成功").Type("success")
			ui.Text(c, "警告").Type("warning")
			ui.Text(c, "危险").Type("danger")
			ui.Text(c, "禁用").Disabled()
		})
		row(c, func() {
			ui.Text(c, "加粗").Strong()
			ui.Text(c, "下划线").Underline()
			ui.Text(c, "删除线").Delete()
			ui.Text(c, "go run .").Code()
			ui.Text(c, "标记").Mark()
			ui.Link(c, "链接文本")
		})
	}})
	gallery.Register(gallery.Page{Name: "Icon", Title: "Icon 图标", Group: gallery.General, Order: 3, Render: func(c *ui.Context) {
		ui.Row(c, func() {
			for _, ic := range []*icon.Icon{icon.Search, icon.Close, icon.Check, icon.Plus, icon.Minus, icon.Down, icon.Up, icon.Left, icon.Right,
				icon.Home, icon.User, icon.Team, icon.Setting, icon.Bell, icon.Mail, icon.Lock, icon.Calendar, icon.Clock, icon.Eye, icon.Edit,
				icon.Delete, icon.Upload, icon.Download, icon.Reload, icon.Filter, icon.Star, icon.Info, icon.CheckCircle, icon.CloseCircle,
				icon.Exclamation, icon.Question, icon.Warning, icon.Dashboard, icon.Appstore, icon.Table, icon.Form, icon.BarChart, icon.PieChart,
				icon.Shop, icon.Message, icon.Global, icon.Link, icon.Copy, icon.Smile, icon.Rise, icon.Fall, icon.Inbox, icon.More} {
				ui.Tooltip(c, ic.Name, func() {
					ui.Column(c, func() { ui.Icon(c, ic).Size(20) }, layout.Size(40, 40), layout.Center())
				})
			}
		}, layout.WrapItems(), layout.Gap(4))
	}})
	gallery.Register(gallery.Page{Name: "Divider", Title: "Divider 分割线", Group: gallery.General, Order: 4, Render: func(c *ui.Context) {
		ui.Text(c, "上方内容")
		ui.Divider(c)
		ui.Text(c, "下方内容")
		ui.Divider(c, "带文字的分割线")
		ui.Row(c, func() {
			ui.Link(c, "编辑")
			ui.VDivider(c)
			ui.Link(c, "删除")
			ui.VDivider(c)
			ui.Link(c, "更多")
		}, layout.Middle())
	}})
}
