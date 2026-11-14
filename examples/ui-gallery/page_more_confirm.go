package main

import (
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
)

var moreConfirmDemo struct {
	result string
	email  string
}

func moreConfirmButtons(c *ui.Context) {
	s := &moreConfirmDemo
	gallery.Demo(c, "确认框", "ui.Confirm 命令式调用，不需要维护 open 状态；OnOk / OnCancel 在 UI 线程回调。", func() {
		moreRow(c, func() {
			if ui.Button(c, "确认对话框").Clicked() {
				ui.Confirm(c, ui.ConfirmOptions{Title: "确定要删除这些条目吗？", Content: "删除后无法恢复，请谨慎操作。",
					OnOk: func(*ui.ConfirmRef) { s.result = "已删除" }, OnCancel: func() { s.result = "已取消" }})
			}
			if ui.Button(c, "危险操作").Danger().Clicked() {
				ui.Confirm(c, ui.ConfirmOptions{Title: "清空回收站", Content: "此操作将永久删除全部文件。", Status: "error", Danger: true, OkText: "清空", MaskClosable: true})
			}
			if ui.Button(c, "异步关闭").Clicked() {
				ui.Confirm(c, ui.ConfirmOptions{Title: "提交审核", Content: "OnOk 中调用 d.Loading(true) 后对话框保持打开，完成后 d.Close()。",
					Status: "info", OnOk: func(d *ui.ConfirmRef) { d.Loading(true) }})
			}
		})
	})
	gallery.Demo(c, "信息提示", "ui.MessageBox：只有一个确认按钮，Status 决定图标。", func() {
		moreRow(c, func() {
			for _, b := range []struct{ label, status, title string }{
				{"信息", "info", "这是一条通知消息"},
				{"成功", "success", "订单已提交成功"},
				{"错误", "error", "网络请求失败"},
				{"警告", "warning", "存储空间即将用尽"},
			} {
				if ui.Button(c, b.label).Clicked() {
					ui.MessageBox(c, ui.ConfirmOptions{Title: b.title, Content: "一些补充说明文字……", Status: b.status})
				}
			}
		})
	})
	gallery.Demo(c, "区分取消与关闭、居中、拖动、自定义图标", "DistinguishCancelAndClose 让关闭按钮 / Esc / 遮罩走 OnClose；Center 居中；Draggable 可拖动标题；Icon 换掉状态图标。", func() {
		moreRow(c, func() {
			if ui.Button(c, "区分取消与关闭").Clicked() {
				ui.Confirm(c, ui.ConfirmOptions{Title: "未保存的修改", Content: "离开前是否保存？", DistinguishCancelAndClose: true,
					OkText: "保存", CancelText: "放弃修改",
					OnOk:     func(*ui.ConfirmRef) { s.result = "已保存" },
					OnCancel: func() { s.result = "放弃修改" },
					OnClose:  func() { s.result = "留在此页" }})
			}
			if ui.Button(c, "居中可拖动").Clicked() {
				ui.MessageBox(c, ui.ConfirmOptions{Title: "收藏成功", Content: "可以拖动标题移动对话框。", Status: "success",
					Center: true, Draggable: true, Icon: icon.StarFilled})
			}
		})
	})
	gallery.Demo(c, "输入框", "ui.Prompt：带输入框与校验，回车提交。", func() {
		moreRow(c, func() {
			if ui.Button(c, "修改邮箱").Clicked() {
				ui.Prompt(c, ui.ConfirmOptions{Title: "提示", Content: "请输入新的邮箱地址", InputPlaceholder: "name@example.com",
					Validate: func(v string) string {
						if !strings.Contains(v, "@") {
							return "邮箱格式不正确"
						}
						return ""
					},
					OnOk: func(d *ui.ConfirmRef) { s.email = d.Value() }})
			}
		})
	})
	if s.result != "" || s.email != "" {
		ui.Text(c, "结果："+s.result+" "+s.email).Secondary()
	}
}

func moreConfirmReset() { moreConfirmDemo.result, moreConfirmDemo.email = "", "" }

func init() {
	morePage(gallery.Page{Name: "Confirm", Title: "Confirm 确认框", Group: gallery.Feedback, Order: 6, Height: 640,
		Render: moreConfirmButtons, Setup: func(d *ui.Driver) { d.ClickLabel("确认对话框") }}, moreConfirmReset)
	morePage(gallery.Page{Name: "MessageBox", Title: "MessageBox 信息提示", Group: gallery.Feedback, Order: 7, Height: 640,
		Render: moreConfirmButtons, Setup: func(d *ui.Driver) { d.ClickLabel("居中可拖动") }}, moreConfirmReset)
	morePage(gallery.Page{Name: "Prompt", Title: "Prompt 输入框", Group: gallery.Feedback, Order: 8, Height: 640,
		Render: moreConfirmButtons, Setup: func(d *ui.Driver) {
			d.ClickLabel("修改邮箱")
			d.Type("abc")
			d.ClickLabel("确定")
		}}, moreConfirmReset)
}
