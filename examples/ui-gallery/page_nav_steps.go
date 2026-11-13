package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var stepsState = struct{ cur, vcur int }{cur: 1, vcur: 1}

func stepsPage(c *ui.Context) {
	three := []ui.StepItem{{Title: "已完成", Description: "这是一段描述"}, {Title: "进行中", SubTitle: "剩余 00:00:08", Description: "这是一段描述"}, {Title: "待处理", Description: "这是一段描述"}}
	navDemo(c, "基础 / 小尺寸", "", func() {
		ui.Steps(c, three).Current(1).Show()
		ui.Gap(c, 24)
		ui.Steps(c, []ui.StepItem{{Title: "已完成"}, {Title: "进行中"}, {Title: "待处理"}}).Current(1).Small().Show()
	})
	navDemo(c, "带图标 / 错误状态", "", func() {
		ui.Steps(c, []ui.StepItem{{Title: "登录", Icon: icon.User}, {Title: "验证", Icon: icon.Lock}, {Title: "支付", Icon: icon.Shop}, {Title: "完成", Icon: icon.Smile}}).Current(2).Show()
		ui.Gap(c, 24)
		ui.Steps(c, []ui.StepItem{{Title: "提交"}, {Title: "审核", Description: "审核未通过"}, {Title: "完成"}}).Current(1).Status("error").Show()
	})
	navDemo(c, "可点击 / 点状 / 标签竖排", "点击任意步骤切换当前步骤。", func() {
		ui.Steps(c, three).Clickable(&stepsState.cur).Show()
		ui.Gap(c, 24)
		ui.Steps(c, three).Current(1).Dot().Show()
		ui.Gap(c, 24)
		ui.Steps(c, []ui.StepItem{{Title: "提交"}, {Title: "审核"}, {Title: "完成"}}).Current(2).LabelVertical().FinishStatus("success").Show()
	})
	navDemo(c, "竖直方向", "", func() {
		navCols(c, func() {
			ui.Steps(c, three).Vertical().Clickable(&stepsState.vcur).Show()
		}, func() {
			ui.Steps(c, three).Vertical().Small().Current(1).Show()
		}, func() {
			ui.Steps(c, three).Vertical().Dot().Current(1).Show()
		})
	})
}

var phState = struct{ tab string }{tab: "1"}

func pageHeaderPage(c *ui.Context) {
	navDemo(c, "基础页头", "", func() {
		ui.PageHeader(c, "标题").SubTitle("这是一段副标题").Back().Show()
	})
	navDemo(c, "完整页头", "面包屑、标签、操作区、内容区与底部标签页。", func() {
		ui.PageHeader(c, "订单详情").Back().SubTitle("单号 2026-1112-0001").Breadcrumb("首页", "订单管理", "订单详情").
			Tags(func() { ui.Tag(c, "进行中").Color("processing") }).
			Extra(func() {
				ui.Button(c, "操作")
				ui.Button(c, "主操作").Primary()
			}).
			Content(func() {
				ui.Row(c, func() {
					ui.Text(c, "创建人：曲丽丽").Secondary()
					ui.Text(c, "关联单据：12421").Secondary()
					ui.Text(c, "生效日期：2026-11-12").Secondary()
				}, layout.Gap(48))
			}).
			Footer(func() {
				ui.Tabs(c, &phState.tab, []ui.TabItem{{Key: "1", Label: "详情"}, {Key: "2", Label: "规则"}}).Show()
			}).Bordered().Show()
	})
}

func init() {
	navPage(gallery.Page{Name: "Steps", Title: "Steps 步骤条", Group: gallery.Navigation, Order: 21, Render: stepsPage,
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find("待处理"); ok {
				d.Move(r.Center())
			}
		}}, func() { stepsState.cur, stepsState.vcur = 1, 1 })
	navPage(gallery.Page{Name: "PageHeader", Title: "PageHeader 页头", Group: gallery.Navigation, Order: 22, Render: pageHeaderPage}, nil)
}
