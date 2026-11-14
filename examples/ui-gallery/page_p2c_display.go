package main

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// Extra sections of the display / feedback components: Tooltip options,
// Popconfirm options, Tag sizes and effects, icon Avatars, Divider
// options, Alert action, card Carousel and Grid push / pull / order.

var p2cDisp = struct{ card int }{}

func init() {
	navPage(gallery.Page{Name: "TooltipMore", Title: "Tooltip 位置、触发、颜色与富内容", Group: gallery.Display, Order: 30, Parent: "Tooltip", Render: p2cTooltipPage,
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find("点击触发"); ok {
				d.Click(r.Center())
			}
			if r, ok := d.Find("底部 · 自定义颜色"); ok {
				d.Move(r.Center())
				d.Settle()
			}
		}}, nil)
	navPage(gallery.Page{Name: "PopconfirmMore", Title: "Popconfirm 文案、图标与位置", Group: gallery.Feedback, Order: 30, Parent: "Popconfirm", Height: 360, Render: p2cPopconfirmPage,
		Setup: func(d *ui.Driver) {
			if r, ok := d.Find("删除任务"); ok {
				d.Click(r.Center())
			}
		}}, nil)
	navPage(gallery.Page{Name: "TagMore", Title: "Tag 尺寸与效果、图标头像", Group: gallery.Display, Order: 30, Parent: "Tag", Render: p2cTagPage}, nil)
	navPage(gallery.Page{Name: "DividerMore", Title: "Divider 虚线与标题位置", Group: gallery.General, Order: 30, Parent: "Divider", Render: p2cDividerPage}, nil)
	navPage(gallery.Page{Name: "AlertMore", Title: "Alert 操作区", Group: gallery.Feedback, Order: 30, Parent: "Alert", Render: p2cAlertPage}, nil)
	navPage(gallery.Page{Name: "CarouselCard", Title: "Carousel 卡片模式", Group: gallery.Display, Order: 30, Parent: "Carousel", Render: p2cCarouselPage}, func() { p2cDisp.card = 0 })
	navPage(gallery.Page{Name: "GridMore", Title: "Grid Push / Pull / Order", Group: gallery.Layout, Order: 30, Parent: "Grid", Render: p2cGridPage}, nil)
}

func p2cTooltipPage(c *ui.Context) {
	navDemo(c, "触发方式、位置与颜色", "Placement 支持 12 个方向；Trigger(\"hover\"|\"click\"|\"focus\")；Color 支持状态色、预设色与 #hex；Arrow(false) 去箭头。", func() {
		ui.Row(c, func() {
			ui.Tooltip(c, "自定义颜色出现在下方", func() { ui.Button(c, "底部 · 自定义颜色") }).Placement("bottom").Color("blue")
			ui.Tooltip(c, "点击后出现，点外部关闭", func() { ui.Button(c, "点击触发") }).Trigger("click").Placement("right").Color("#722ed1")
			ui.Tooltip(c, "聚焦时出现", func() { ui.Button(c, "聚焦触发") }).Trigger("focus").Arrow(false)
			ui.Tooltip(c, "延迟 300ms", func() { ui.Button(c, "延迟显示") }).MouseEnterDelay(300 * time.Millisecond).MouseLeaveDelay(100 * time.Millisecond)
		}, layout.Gap(16), layout.Middle(), layout.Margin(navPad(80)))
	})
	navDemo(c, "富内容 Content / 亮色 Effect(\"light\")", "", func() {
		ui.Row(c, func() {
			ui.Tooltip(c, "订单详情", func() { ui.Link(c, "悬停查看订单") }).Content(func() {
				ui.Text(c, "订单 #20261113").Strong().Color(c.Theme.TextOnSpotlight)
				ui.Text(c, "金额 ¥ 1,280.00").Color(c.Theme.TextOnSpotlight)
			})
			ui.Tooltip(c, "亮色提示", func() { ui.Link(c, "亮色 Effect") }).Effect("light")
		}, layout.Gap(24))
	})
}

func p2cPopconfirmPage(c *ui.Context) {
	navDemo(c, "OkText / CancelText / Icon / Description / Placement", "Disabled(true) 时不弹出；OkDanger 危险确认；ShowCancel(false) 隐藏取消。", func() {
		ui.Row(c, func() {
			ui.Popconfirm(c, "确定删除这个任务吗？", "", func() { ui.Button(c, "删除任务").Danger() }).
				Description("删除后不可恢复，相关记录会一并清除。").OkText("删除").CancelText("再想想").
				Icon(icon.Question).Placement("bottomLeft").OkDanger()
			ui.Popconfirm(c, "已禁用", "", func() { ui.Button(c, "禁用的确认") }).Disabled(true)
			ui.Popconfirm(c, "只有确认", "", func() { ui.Button(c, "无取消") }).ShowCancel(false).Placement("right")
		}, layout.Gap(16))
	})
	ui.Gap(c, 140)
}

func p2cTagPage(c *ui.Context) {
	navDemo(c, "尺寸 Size", "Element 尺寸：large 32px、default 24px、small 20px。", func() {
		ui.Row(c, func() {
			ui.Tag(c, "大号").Size(ui.Large)
			ui.Tag(c, "默认")
			ui.Tag(c, "小号").Size(ui.Small)
			ui.Tag(c, "可关闭").Size(ui.Small).Closable()
		}, layout.Gap(8), layout.Middle())
	})
	navDemo(c, "效果 Effect(dark / light / plain)", "", func() {
		ui.Column(c, func() {
			for _, e := range []string{"dark", "light", "plain"} {
				ui.Row(c, func() {
					for _, col := range []string{"", "success", "info", "warning", "error"} {
						label := col
						if label == "" {
							label = "default"
						}
						ui.Tag(c, e+" "+label).Effect(e).Color(col)
					}
				}, layout.Gap(8))
			}
		}, layout.Gap(8))
	})
	navDemo(c, "图标头像 Avatar.Icon", "", func() {
		ui.Row(c, func() {
			ui.Avatar(c, "").Icon(icon.User).Size(40)
			ui.Avatar(c, "").Icon(icon.Team).Bg(c.Theme.Primary.Base)
			ui.Avatar(c, "").Icon(icon.Shop).Square().Bg(theme.Hex("#87d068"))
			ui.Avatar(c, "").Icon(icon.Bell).Size(24)
		}, layout.Gap(16), layout.Middle())
	})
}

func p2cDividerPage(c *ui.Context) {
	navDemo(c, "虚线与标题位置", "Dashed()；Orientation(\"left\"|\"right\"|\"center\")；Plain() 普通字重。", func() {
		ui.Text(c, "Lorem ipsum dolor sit amet, consectetur adipiscing elit.")
		ui.Divider(c).Dashed()
		ui.Text(c, "Sed nonne merninisti licere mihi ista probare.")
		ui.Divider(c, "左侧标题").Orientation("left")
		ui.Divider(c, "右侧标题").Orientation("right").Plain()
		ui.Divider(c, "居中虚线").Dashed()
	})
}

func p2cAlertPage(c *ui.Context) {
	navDemo(c, "操作区 Action", "", func() {
		ui.Column(c, func() {
			ui.Alert(c, "success", "已保存草稿").Closable().Action(func() { ui.Button(c, "撤销").Small() }).Show()
			ui.Alert(c, "info", "有新版本可用").Description("新版本修复了若干问题，建议更新。").Action(func() {
				ui.Column(c, func() {
					ui.Button(c, "立即更新").Small().Primary()
					ui.Button(c, "稍后").Small()
				}, layout.Gap(8))
			}).Show()
		}, layout.Gap(16))
	})
}

func p2cCarouselPage(c *ui.Context) {
	navDemo(c, "卡片模式 Card()", "中间为当前项，两侧缩小并变暗；点击两侧卡片切换。", func() {
		th := c.Theme
		cols := []theme.Color{th.Primary.Base, th.Success.Base, th.Warning.Base, th.Error.Base, th.Info.Base}
		ui.Carousel(c, len(cols), func(i int) {
			fg := th.TextOnSolid
			if i == 0 {
				fg = th.TextOnPrimary // shadcn's primary is near white in dark mode
			}
			ui.Column(c, func() { ui.Title(c, fmt.Sprint(i+1), 3).Color(fg) }, layout.Grow(1), layout.Center()).Bg(cols[i])
		}).Card().Height(200).Current(&p2cDisp.card).Show()
	})
}

func p2cGridPage(c *ui.Context) {
	th := c.Theme
	cell := func(s string, col theme.Color) func() {
		return func() {
			ui.Column(c, func() { ui.Text(c, s).Color(th.TextOnPrimary) }, layout.H(40), layout.Center()).Bg(col)
		}
	}
	navDemo(c, "栅格排序 Push / Pull", "col-18 push-6 与 col-6 pull-18 交换显示位置。", func() {
		ui.GridRow(c).Body(func() {
			ui.Col(c, 18).Push(6).Body(cell("col-18 col-push-6", th.Primary.Base))
			ui.Col(c, 6).Pull(18).Body(cell("col-6 col-pull-18", th.Primary.Hover))
		})
	})
	navDemo(c, "Order", "按 Order 值排列（相同值保持书写顺序）。", func() {
		ui.GridRow(c).Gutter(16, 0).Body(func() {
			for i, o := range []int{4, 3, 2, 1} {
				ui.Col(c, 6).Order(o).Body(cell(fmt.Sprintf("%d col-order-%d", i+1, o), th.Primary.Base))
			}
		})
	})
}
