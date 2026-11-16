package main

import (
	"fmt"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
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
	ui.Text(c, "FooterToolbar 默认固定在视口底部；Fixed(false) 使用普通流，Sticky() 按阈值吸底，Target(scrollBox) 固定到容器底部；固定时为 Tango 适配的毛玻璃，HighContrast 或不支持模糊的渲染器自动使用不透明背景。").Tertiary().
		Layout(layout.Margin(geom.Insets{Bottom: 8}))
	pro2Canvas(c, func() {
		// Target-sticky is first so the state is visible even in the fixed-size
		// strict damage verifier. The bar is anchored to this ScrollBox rather
		// than to the window.
		ui.ProCard(c, "Target（滚动容器底部）").SubTitle("Affix 到指定 ScrollBox").Body(func() {
			sb := ui.Scroll(c, nil, layout.H(300), layout.MinH(0))
			sb.Body(func() {
				ui.ProCard(c, "仓库管理").Body(func() {
					ui.Paragraph(c, "底部操作栏吸底时以毛玻璃浮在内容之上：下方滚过的文字与色块被模糊后透出半透明底色。")
					for i := 0; i < 4; i++ {
						ui.Row(c, func() {
							for k, col := range []struct {
								bg, fg theme.Color
								label  string
							}{
								{th.Primary.Base, th.TextOnPrimary, "入库"},
								{th.Success.Base, th.TextOnPrimary, "在库"},
								{th.Warning.Base, th.TextOnPrimary, "预警"},
								{th.Error.Base, th.TextOnPrimary, "缺货"},
							} {
								ui.Column(c, func() {
									ui.Text(c, col.label).Color(col.fg).Strong()
									ui.Text(c, fmt.Sprintf("%d 件", 120+i*37+k*11)).Color(col.fg)
								}, layout.Grow(1), layout.PadVH(10, 12), layout.Gap(2)).Bg(col.bg).Radius(th.Radius)
							}
						}, layout.Gap(12))
						ui.Gap(c, 12)
						ui.Skeleton(c).Rows(2).Show()
						ui.Gap(c, 12)
					}
				})
				ui.Gap(c, 320)
				ui.FooterToolbar(c).Target(sb.ID()).Extra(func() {
					ui.Icon(c, icon.Info).Size(16).Color(th.Primary.Base)
					ui.Text(c, "表单有 2 处修改尚未保存")
				}).Body(func() {
					ui.Button(c, "取消")
					ui.Button(c, "提交").Primary()
				})
			})
		})

		// Ordinary flow: the bar consumes layout space and keeps the plain
		// FooterToolbar background rather than the pinned frosted surface.
		ui.ProCard(c, "普通流").SubTitle("不吸底，跟随内容自然排布").Body(func() {
			ui.Text(c, "普通流 FooterToolbar 会占据内容高度，适合页面内的局部操作。")
			ui.FooterToolbar(c).Fixed(false).Extra(func() {
				ui.Text(c, "已选择 2 项").Secondary()
			}).Body(func() {
				ui.Button(c, "取消")
				ui.Button(c, "保存").Primary()
			})
		})

		// Window-sticky: scrolling content passes under the bar and makes the
		// translucent backdrop visible in every design language.
		ui.ProCard(c, "Sticky（窗口底部）").SubTitle("Affix 到窗口底部").Body(func() {
			sb := ui.Scroll(c, nil, layout.H(220), layout.MinH(0))
			sb.Body(func() {
				ui.Column(c, nil, layout.H(440))
				ui.Text(c, "继续滚动，窗口底部的 FooterToolbar 仍保持可操作。")
			})
			ui.FooterToolbar(c).Sticky().Extra(func() {
				ui.Icon(c, icon.Info).Size(16).Color(th.Primary.Base)
				ui.Text(c, "窗口级操作").Secondary()
			}).Body(func() {
				ui.Button(c, "取消")
				ui.Button(c, "提交").Primary()
			})
		})
	}, layout.H(920), layout.Pad(0))
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
