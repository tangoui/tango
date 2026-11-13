package main

import (
	"fmt"
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

var fb2 = struct {
	modal, dialog, centered, full bool
	drawer, child, top, left      bool
	loading, region               bool
	resize                        bool
	resizeSize                    float32
}{}

func resetFb2() {
	fb2 = struct {
		modal, dialog, centered, full bool
		drawer, child, top, left      bool
		loading, region               bool
		resize                        bool
		resizeSize                    float32
	}{region: true, resizeSize: 420}
}

func init() { resetFb2() }

func messagePage(c *ui.Context) {
	navDemo(c, "四种状态", "ui.Message(c, status, content) 在窗口顶部显示全局提示，3 秒后自动消失。", func() {
		ui.Row(c, func() {
			for _, s := range []struct{ label, status, text string }{
				{"成功", "success", "这是一条成功提示"}, {"错误", "error", "这是一条错误提示"},
				{"警告", "warning", "这是一条警告提示"}, {"普通", "info", "这是一条普通提示"},
			} {
				if ui.Button(c, s.label).Clicked() {
					ui.Message(c, s.status, s.text)
				}
			}
		}, layout.Gap(8))
	})
	navDemo(c, "加载态与更新同一条", "Key 相同的消息原地更新；Duration(0) 不自动关闭，MessageDestroy 关闭。", func() {
		ui.Row(c, func() {
			if ui.Button(c, "加载中").Clicked() {
				ui.Message(c, "loading", "正在保存…").Key("save").Duration(0)
			}
			if ui.Button(c, "更新为成功").Clicked() {
				ui.Message(c, "success", "保存成功").Key("save")
			}
			if ui.Button(c, "全部关闭").Clicked() {
				ui.MessageDestroy(c, "")
			}
		}, layout.Gap(8))
	})
	ui.Gap(c, 80)
}

func notifyPage(c *ui.Context) {
	navDemo(c, "四个方位 / 进度条 / 悬停暂停 / 堆叠", "超过 3 条时折叠为堆叠卡片，悬停展开。", func() {
		ui.Row(c, func() {
			for _, p := range []struct {
				label string
				pl    ui.NotifyPlacement
			}{{"右上", ui.NotifyTopRight}, {"左上", ui.NotifyTopLeft}, {"右下", ui.NotifyBottomRight}, {"左下", ui.NotifyBottomLeft}} {
				if ui.Button(c, p.label).Clicked() {
					ui.Notification(c, "info", "通知标题（"+p.label+"）").Description("这是通知的内容，用于向用户推送系统消息。").Placement(p.pl).ShowProgress()
				}
			}
			if ui.Button(c, "堆叠").Clicked() {
				for i := 0; i < 5; i++ {
					ui.Notification(c, "success", fmt.Sprintf("第 %d 条通知", i+1)).Description("堆叠显示，悬停展开。").Duration(0)
				}
			}
		}, layout.Gap(8))
	})
	navDemo(c, "操作按钮 / 更新同一条", "Actions 放置按钮；Key 相同的通知原地更新。", func() {
		ui.Row(c, func() {
			if ui.Button(c, "带按钮").Clicked() {
				ui.Notification(c, "warning", "版本更新").Description("新版本已下载完成，是否立即重启？").Key("upd").Duration(0).
					Actions(func() {
						if ui.Button(c, "稍后").Small().Clicked() {
							ui.NotificationDestroy(c, "upd")
						}
						if ui.Button(c, "立即重启").Small().Primary().Clicked() {
							ui.NotificationDestroy(c, "upd")
						}
					})
			}
			if ui.Button(c, "更新内容").Clicked() {
				ui.Notification(c, "success", "版本更新").Description("已更新到最新版本。").Key("upd")
			}
		}, layout.Gap(8))
	})
	ui.Gap(c, 260)
}

func modalPage(c *ui.Context) {
	navDemo(c, "基础 / 垂直居中 / 自定义页脚", "", func() {
		ui.Row(c, func() {
			if ui.Button(c, "打开对话框").Clicked() {
				fb2.modal = true
			}
			if ui.Button(c, "垂直居中").Clicked() {
				fb2.centered = true
			}
		}, layout.Gap(8))
		ui.Modal(c, &fb2.modal, "基础对话框").Body(func() {
			ui.Text(c, "对话框的内容……")
			ui.Text(c, "支持任意组件。")
		})
		ui.Modal(c, &fb2.centered, "垂直居中的对话框").Centered().Footer(func() {
			if ui.Button(c, "知道了").Primary().Clicked() {
				fb2.centered = false
			}
		}).Body(func() { ui.Text(c, "Centered() 使对话框在窗口中垂直居中。") })
	})
	navDemo(c, "可拖动 / 全屏切换", "拖动标题栏移动对话框；右上角按钮切换全屏。", func() {
		if ui.Button(c, "可拖动对话框").Primary().Clicked() {
			fb2.dialog = true
		}
		ui.Modal(c, &fb2.dialog, "可拖动的对话框").Draggable().FullscreenToggle().Fullscreen(&fb2.full).Body(func() {
			ui.Text(c, "按住标题栏拖动对话框。")
			ui.Text(c, "对话框内容可以是任意组件。")
		})
	})
	ui.Gap(c, 300)
}

func drawerPage(c *ui.Context) {
	navDemo(c, "四个方向与多层嵌套", "子抽屉打开时父抽屉向左推开；Extra 放置标题栏右侧操作。", func() {
		ui.Row(c, func() {
			if ui.Button(c, "右侧 + 嵌套").Primary().Clicked() {
				fb2.drawer = true
			}
			if ui.Button(c, "左侧").Clicked() {
				fb2.left = true
			}
			if ui.Button(c, "顶部").Clicked() {
				fb2.top = true
			}
		}, layout.Gap(8))
		ui.Drawer(c, &fb2.drawer, "一级抽屉").Size(460).Extra(func() { ui.Button(c, "更多").Small() }).
			Footer(func() {
				ui.Button(c, "取消")
				ui.Button(c, "确定").Primary()
			}).Body(func() {
			if ui.Button(c, "打开二级抽屉").Clicked() {
				fb2.child = true
			}
			ui.Drawer(c, &fb2.child, "二级抽屉").Size(320).Body(func() { ui.Text(c, "二级抽屉内容") })
		})
		ui.Drawer(c, &fb2.left, "左侧抽屉").Placement(ui.DrawerLeft).Body(func() { ui.Text(c, "从左侧滑出") })
		ui.Drawer(c, &fb2.top, "顶部抽屉").Placement(ui.DrawerTop).Size(200).Body(func() { ui.Text(c, "从顶部滑出") })
	})
	navDemo(c, "拖动边缘调整大小", "Resizable 让面向页面的边缘可拖动（也可聚焦后用方向键调整）；SizeLimits 限定范围，BindSize 绑定当前宽度。", func() {
		ui.Row(c, func() {
			if ui.Button(c, "可调整宽度的抽屉").Clicked() {
				fb2.resize = true
			}
			ui.Text(c, fmt.Sprintf("当前宽度 %.0fpx", fb2.resizeSize)).Secondary()
		}, layout.Gap(12), layout.Middle())
		ui.Drawer(c, &fb2.resize, "可调整宽度").Resizable(true).SizeLimits(280, 720).BindSize(&fb2.resizeSize).
			Body(func() { ui.Text(c, "拖动左侧边缘调整宽度。") })
	})
	ui.Gap(c, 360)
}

var spinPct = 45.0

func spinPage(c *ui.Context) {
	navDemo(c, "尺寸 / 提示 / 进度 / 延迟", "", func() {
		ui.Row(c, func() {
			ui.Spin(c).Size(ui.Small)
			ui.Spin(c)
			ui.Spin(c).Size(ui.Large)
			ui.Spin(c).Tip("加载中")
			ui.Spin(c).Size(ui.Large).Percent(spinPct)
			ui.Spin(c).Delay(500 * time.Millisecond)
		}, layout.Gap(48), layout.Middle())
	})
	navDemo(c, "嵌套：包裹的内容半透明", "Spin(c).Spinning(on).Wrap(fn)", func() {
		ui.Spin(c).Tip("加载中…").Wrap(func() {
			ui.Alert(c, "info", "被嵌套的内容").Description("容器加载时内容半透明，指示器居中显示。").Show()
		})
	})
}

func loadingPage(c *ui.Context) {
	navDemo(c, "区域加载", "Loading(c, on).Tip(...).Body(fn) 用遮罩覆盖区域（Element v-loading）。", func() {
		ui.Switch(c, &fb2.region).A11yLabel("区域加载")
		ui.Gap(c, 12)
		ui.Loading(c, fb2.region).Tip("拼命加载中").Body(func() {
			ui.Alert(c, "success", "区域加载").Description("加载时遮罩挡住下层内容的交互。").Show()
		})
	})
	navDemo(c, "全屏加载", "Loading(c, on).Fullscreen() 覆盖整个窗口。", func() {
		if ui.Button(c, "全屏加载").Icon(icon.Loading).Clicked() {
			fb2.loading = true
		}
		ui.Loading(c, fb2.loading).Tip("加载中").Fullscreen()
	})
}

func init() {
	navPage(gallery.Page{Name: "Message", Title: "Message 全局提示", Group: gallery.Feedback, Order: 6, Render: messagePage,
		Setup: func(d *ui.Driver) {
			d.ClickLabel("成功")
			d.ClickLabel("错误")
			d.ClickLabel("加载中")
			d.Settle()
		}}, nil)
	navPage(gallery.Page{Name: "Notification", Title: "Notification 通知提醒框", Group: gallery.Feedback, Order: 7, Height: 620, Render: notifyPage,
		Setup: func(d *ui.Driver) {
			d.ClickLabel("左下")
			d.ClickLabel("右下")
			d.ClickLabel("堆叠")
			d.ClickLabel("带按钮")
			d.Settle()
			d.Move(geom.Pt(4, 4))
			d.Settle()
		}}, nil)
	navPage(gallery.Page{Name: "Modal", Title: "Modal 对话框", Group: gallery.Feedback, Order: 3, Height: 520, Render: modalPage,
		Setup: func(d *ui.Driver) {
			d.ClickLabel("可拖动对话框")
			d.Settle()
			if r, ok := d.Find("可拖动的对话框"); ok {
				p := r.Center()
				d.Inject(pointerPress(p))
				d.Frame()
				p.X += 160
				p.Y += 40
				d.Move(p)
				d.Inject(pointerRelease(p))
				d.Frame()
			}
		}}, resetFb2)
	navPage(gallery.Page{Name: "Drawer", Title: "Drawer 抽屉", Group: gallery.Feedback, Order: 4, Height: 560, Render: drawerPage,
		Setup: func(d *ui.Driver) {
			d.ClickLabel("右侧 + 嵌套")
			d.Settle()
			d.ClickLabel("打开二级抽屉")
			d.Settle()
		}}, resetFb2)
	navPage(gallery.Page{Name: "Spin", Title: "Spin 加载中", Group: gallery.Feedback, Order: 2, Render: spinPage}, resetFb2)
	navPage(gallery.Page{Name: "Loading", Title: "Loading 加载遮罩", Group: gallery.Feedback, Order: 8, Render: loadingPage}, resetFb2)
}
