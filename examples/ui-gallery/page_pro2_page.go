package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var pro2PageDemo struct {
	tab    string
	clicks int
}

func pro2PageReset() {
	pro2PageDemo.tab = "base"
	pro2PageDemo.clicks = 0
}

func pro2PageContainerDemo(c *ui.Context) {
	s := &pro2PageDemo
	pro2Canvas(c, func() {
		ui.PageContainer(c, "高级表单").
			Breadcrumb("首页", "表单页", "高级表单").
			Back().
			SubTitle("这是一个副标题").
			Tags(func() { ui.Tag(c, "进行中").Color("processing") }).
			Extra(func() {
				ui.Button(c, "次要按钮")
				ui.Button(c, "主操作").Primary()
			}).
			Content(func() {
				ui.Descriptions(c, []ui.DescItem{
					{Label: "创建人", Value: "曲丽丽"},
					{Label: "关联表单", Value: "421421"},
					{Label: "创建时间", Value: "2017-01-10"},
					{Label: "生效日期", Value: "2017-07-07 ~ 2017-08-08"},
				}).Column(2).Size(ui.Small)
			}).
			ExtraContent(func() {
				ui.Row(c, func() {
					ui.Statistic(c, "状态", 0, 0).Value("待审批")
					ui.Statistic(c, "订单金额", 568.08, 2).Prefix("¥")
				}, layout.Gap(32))
			}).
			Tabs(&s.tab, []ui.TabItem{{Key: "base", Label: "基本信息"}, {Key: "info", Label: "详细信息"}, {Key: "log", Label: "操作日志", Disabled: true}}).
			Footer(func() {
				ui.Button(c, "重置")
				if ui.Button(c, "提交").Primary().Clicked() {
					s.clicks++
				}
			}).
			FooterExtra(func() { ui.Text(c, "已选择 2 项").Secondary() }).
			Fill().
			Body(func() {
				ui.ProCard(c, "页面内容").SubTitle("当前标签：" + s.tab).Body(func() {
					ui.Text(c, "PageContainer 由页头（面包屑、返回、标题、副标题、标签、操作区、内容与右侧内容、标签页）、内容区与底部 FooterToolbar 组成。")
				})
				ui.Gap(c, 16)
				ui.ProCard(c, "更多内容").Body(func() {
					ui.Skeleton(c).Rows(3).Show()
				})
			})
	}, layout.H(700), layout.Pad(0))
}

func pro2FooterToolbarDemo(c *ui.Context) {
	th := c.Theme
	pro2Canvas(c, func() {
		ui.Scroll(c, func() {
			ui.ProCard(c, "仓库管理").Body(func() {
				for i := 0; i < 3; i++ {
					ui.Skeleton(c).Rows(2).Show()
					ui.Gap(c, 16)
				}
			})
		}, layout.Grow(1), layout.MinH(0), layout.Pad(16))
		ui.FooterToolbar(c).
			Extra(func() {
				ui.Icon(c, icon.Info).Size(16).Color(th.Primary.Base)
				ui.Text(c, "表单有 2 处修改尚未保存")
			}).
			Body(func() {
				ui.Button(c, "取消")
				ui.Button(c, "提交").Primary()
			})
	}, layout.H(360), layout.Pad(0), layout.Gap(0))
	ui.Gap(c, 16)
	ui.Text(c, "宽度跟随所在内容区；Sticky() 时以 Affix 固定在视口（或 Target 滚动容器）底部。").Tertiary().
		Layout(layout.Margin(geom.Insets{Top: 4}))

}

func init() {
	pro2PageReset()
	pro2Page(gallery.Page{Name: "PageContainer", Title: "PageContainer 页容器", Group: gallery.Layout, Order: 40, Width: 1100,
		Render: pro2PageContainerDemo,
		Setup: func(d *ui.Driver) {
			// Hover the primary footer action so the bar reads as live.
			if r, ok := d.Find("提交"); ok {
				d.Move(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
				d.Settle()
			}
		},
	}, pro2PageReset)
	pro2Page(gallery.Page{Name: "FooterToolbar", Title: "FooterToolbar 底部操作栏", Group: gallery.Layout, Order: 41, Width: 900,
		Render: pro2FooterToolbarDemo}, nil)
}
