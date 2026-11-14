package main

// Spec alignment batch 3, group K: demos of the capabilities added to the
// overlay and dialog components (docs/specs/align/3-overlay.md).

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
)

var b3k struct {
	modal, drawer, tour bool
	tourCur             int
	tourIDs             [3]ui.ID
	popResult           string
}

func b3kReset() {
	b3k.modal, b3k.drawer, b3k.tour, b3k.tourCur, b3k.popResult = true, true, true, 0, ""
}

func init() {
	b3kReset()
	gallery.Register(gallery.Page{Name: "ModalAlign", Parent: "Modal", Title: "Modal 关闭按钮、键盘与加载", Group: gallery.Feedback, Order: 1, Height: 420,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "关闭按钮位置", "antd 32×32 距右上 12；Element 48×48 贴右上角；shadcn top-4 right-4。Keyboard(false) 时 Esc 不关闭。", func() {
				if ui.Button(c, "打开对话框").Clicked() {
					b3k.modal = true
				}
			})
			ui.Modal(c, &b3k.modal, "关闭按钮绝对定位").Keyboard(false).Body(func() {
				ui.Text(c, "标题右侧留出关闭按钮的位置；窄窗口时宽度不超过窗口宽 − 32。")
			})
		},
		Setup: func(d *ui.Driver) { b3kReset(); d.Settle() }})

	gallery.Register(gallery.Page{Name: "DrawerAlign", Parent: "Drawer", Title: "Drawer 关闭按钮位置与默认尺寸", Group: gallery.Feedback, Order: 1, Height: 420,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "closable.placement", "Ant 默认关闭按钮在标题前（start），ClosablePlacement(\"end\") 放到末尾；shadcn Sheet 默认宽 384（不超过 75%）。", func() {
				if ui.Button(c, "打开抽屉").Clicked() {
					b3k.drawer = true
				}
			})
			ui.Drawer(c, &b3k.drawer, "关闭按钮在末尾").ClosablePlacement("end").
				Extra(func() { ui.Button(c, "操作").Small() }).
				Footer(func() {
					ui.Button(c, "取消")
					ui.Button(c, "提交").Primary()
				}).
				Body(func() { ui.Text(c, "Keyboard / Loading / Push / AfterOpenChanged 也已补齐。") })
		},
		Setup: func(d *ui.Driver) { b3kReset(); d.Settle() }})

	gallery.Register(gallery.Page{Name: "PopconfirmAlign", Parent: "Popconfirm", Title: "Popconfirm 按钮类型、宽度与箭头", Group: gallery.Feedback, Order: 1, Height: 440,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "OkType / CancelType / Width", "箭头与 Popover 共用 drawPopupArrow；Element 默认在下方、宽 150、取消按钮为 text 类型。", func() {
				ui.Row(c, func() {
					p := ui.Popconfirm(c, "删除这条记录？", "删除后无法恢复。", func() { ui.Button(c, "删除").Danger() }).
						OkType("danger").Width(240).Placement("bottom")
					if p.Confirmed() {
						b3k.popResult = "已删除"
					}
					ui.Popconfirm(c, "无箭头", "", func() { ui.Button(c, "无箭头") }).Arrow(false).CancelType("link")
					ui.Text(c, b3k.popResult)
				}, layout.Gap(16), layout.Middle())
			})
		},
		Setup: func(d *ui.Driver) {
			b3kReset()
			d.Settle()
			d.ClickLabel("删除")
			d.Settle()
		}})

	gallery.Register(gallery.Page{Name: "TourAlign", Parent: "Tour", Title: "Tour 键盘、可点击目标与镂空圆角", Group: gallery.Display, Order: 1, Height: 460,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "keyboard / disabledInteraction / gap.radius", "←/→ 切换步骤；目标默认可点击（DisabledInteraction 关闭）；镂空圆角默认 2；面板最宽 520，按内容收缩。", func() {
				ui.Row(c, func() {
					b3k.tourIDs[0] = ui.Button(c, "上传").ID()
					b3k.tourIDs[1] = ui.Button(c, "保存").Primary().ID()
					b3k.tourIDs[2] = ui.Button(c, "更多").ID()
				}, layout.Gap(16))
			})
			ui.Tour(c, &b3k.tour, &b3k.tourCur, []ui.TourStep{
				{Target: b3k.tourIDs[0], Title: "上传文件", Description: "把文件拖到这里，或点击按钮选择。"},
				{Target: b3k.tourIDs[1], Title: "保存", Description: "保存当前的修改。"},
				{Target: b3k.tourIDs[2], Title: "更多操作", Description: "导出、分享与设置。"},
			}).Show()
		},
		Setup: func(d *ui.Driver) { b3kReset(); d.Settle(); d.Settle() }})

	gallery.Register(gallery.Page{Name: "ConfirmAlign", Parent: "Confirm", Title: "Confirm 自动聚焦与键盘", Group: gallery.Feedback, Order: 1, Height: 360,
		Render: func(c *ui.Context) {
			gallery.Demo(c, "autoFocusButton / keyboard", "AutoFocusButton: \"cancel\" 打开时聚焦取消按钮；NoKeyboard 时 Esc 不关闭；Ant 按钮与正文间距 12（marginSM）。", func() {
				if ui.Button(c, "打开确认框").Clicked() {
					ui.Confirm(c, ui.ConfirmOptions{Title: "确定离开此页面？", Content: "未保存的修改将会丢失。", AutoFocusButton: "cancel", NoKeyboard: true})
				}
			})
		},
		Setup: func(d *ui.Driver) {
			d.ClickLabel("打开确认框")
			d.Settle()
		}})
}
