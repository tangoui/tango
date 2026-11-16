package main

// Spec alignment batch 3, group L: demos of the capabilities added to the
// feedback and status components (docs/specs/align/3-feedback.md).

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var (
	b3lSkelLoading = true
	b3lLoading     = true
)

func init() {
	gallery.Register(gallery.Page{Name: "AlertAlign", Parent: "Alert", Title: "Alert 图标、变体与关闭", Group: gallery.Feedback, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "图标默认关闭", "antd showIcon / EP show-icon 默认 false；ShowIcon(true) 显示；Banner 默认带图标。", func() {
				ui.Column(c, func() {
					ui.Alert(c, "info", "默认不带图标").Show()
					ui.Alert(c, "success", "ShowIcon(true)").ShowIcon(true).Show()
					ui.Alert(c, "warning", "Banner 默认带图标、无边框无圆角").Banner().Show()
					ui.Alert(c, "info", "自定义图标 Icon(icon.Star)").Icon(icon.Star).Show()
				}, layout.Gap(8))
			})
			gallery.Demo(c, "变体 variant", "antd 6.4：outlined（默认，状态色描边）/ filled（无描边）。", func() {
				ui.Column(c, func() {
					for _, v := range []string{"outlined", "filled"} {
						ui.Alert(c, "error", v).Variant(v).ShowIcon(true).Description("描述文字在标题下方。").Show()
					}
				}, layout.Gap(8))
			})
			gallery.Demo(c, "关闭", "CloseIcon 换关闭图标；CloseText（EP close-text）用文字代替；CloseDisabled 禁用关闭按钮。", func() {
				ui.Column(c, func() {
					ui.Alert(c, "info", "自定义关闭图标").CloseIcon(icon.CloseCircle).Show()
					ui.Alert(c, "success", "文字关闭按钮").CloseText("知道了").Show()
					ui.Alert(c, "warning", "关闭按钮已禁用").Closable().CloseDisabled(true).Show()
				}, layout.Gap(8))
			})
		}})

	gallery.Register(gallery.Page{Name: "MessageAlign", Parent: "Message", Title: "Message 悬停暂停、图标与合并", Group: gallery.Feedback, Order: 1, Height: 420,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "新选项", "悬停时暂停计时（antd pauseOnHover 默认开）；Icon 自定义图标；Grouping 合并相同内容并计数（EP grouping）。", func() {
				ui.Row(c, func() {
					if ui.Button(c, "自定义图标").Clicked() {
						ui.Message(c, "info", "收到一颗星").Icon(icon.Star).Duration(0)
					}
					if ui.Button(c, "合并计数").Clicked() {
						ui.Message(c, "error", "网络错误").Grouping().Duration(0)
					}
					if ui.Button(c, "不暂停").Clicked() {
						ui.Message(c, "success", "悬停也会关闭").PauseOnHover(false)
					}
				}, layout.Gap(8))
			})
		},
		Setup: func(d *ui.Driver) {
			d.ClickLabel("自定义图标")
			for range 3 {
				d.ClickLabel("合并计数")
			}
			d.Settle()
		}})

	gallery.Register(gallery.Page{Name: "NotificationAlign", Parent: "Notification", Title: "Notification 关闭、图标与进度条", Group: gallery.Feedback, Order: 1, Height: 560,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "新选项", "Closable(false) 隐藏关闭按钮；Icon 自定义图标；ShowProgress 进度条（antd 渐变、EP 按类型着色）。", func() {
				ui.Row(c, func() {
					if ui.Button(c, "无关闭按钮").Clicked() {
						ui.Notification(c, "info", "无关闭按钮").Description("Closable(false)").Closable(false).Duration(0)
					}
					if ui.Button(c, "进度条").Clicked() {
						ui.Notification(c, "success", "带进度条").Description("ShowProgress，悬停暂停").Icon(icon.Star).ShowProgress().Duration(time.Hour)
					}
				}, layout.Gap(8))
			})
		},
		Setup: func(d *ui.Driver) {
			d.ClickLabel("无关闭按钮")
			d.ClickLabel("进度条")
			d.Settle()
		}})

	gallery.Register(gallery.Page{Name: "ResultAlign", Parent: "Result", Title: "Result 间距", Group: gallery.Display, Order: 1, Width: 900,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "标题与副标题", "antd 标题 marginBlock 8（与图标 24、操作区 24 折叠）；内容区无圆角。EP 行高 1.3。", func() {
				ui.Result(c, "success", "提交成功").SubTitle("订单号 2026111500001，云服务器配置需要 1-5 分钟。").Extra(func() {
					ui.Button(c, "返回列表").Primary()
					ui.Button(c, "再买一台")
				}).Body(func() { ui.Text(c, "附加内容区") })
			})
		}})

	gallery.Register(gallery.Page{Name: "SpinAlign", Parent: "Spin", Title: "Spin 令牌尺寸、描述、自动进度与骨架屏节流", Group: gallery.Feedback, Order: 1,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "尺寸与描述", "尺寸来自 SpinToken（antd 14/20/32，EP 28/42/50，shadcn 默认 16）；Description 取代 Tip。", func() {
				ui.Row(c, func() {
					ui.Spin(c).Size(ui.Small).Description("小")
					ui.Spin(c).Description("默认")
					ui.Spin(c).Size(ui.Large).Description("大")
					ui.Spin(c).Indicator(icon.Loading).Description("自定义指示器")
					ui.Spin(c).Size(ui.Large).Percent(60)
					ui.Spin(c).Size(ui.Large).PercentAuto()
				}, layout.Gap(40), layout.Middle())
			})
			gallery.Demo(c, "Loading 文字与指示器", "Loading(c, on).Text(...).Spinner(ic)（EP element-loading-text / spinner）。", func() {
				ui.Loading(c, b3lLoading).Text("加载中").Body(func() {
					ui.Alert(c, "info", "被遮罩的内容").Description("加载时内容按令牌变淡并被遮罩覆盖。").ShowIcon(true).Show()
				})
			})
			gallery.Demo(c, "Skeleton 节流", "Throttle(leading, trailing)（EP throttle）：加载超过 leading 才显示骨架屏，结束后再保留 trailing。", func() {
				if ui.Button(c, "切换加载").Clicked() {
					b3lSkelLoading = !b3lSkelLoading
				}
				ui.Skeleton(c).Loading(b3lSkelLoading).Throttle(300*time.Millisecond, 200*time.Millisecond).Body(func() {
					ui.Text(c, "真实内容")
				})
			})
		}})
}
