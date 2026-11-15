package main

import (
	"time"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
)

// NotificationCenter: a bell with an unread badge and the panel of
// notifications.

var notifyCenterDemo struct {
	items []ui.NotificationItem
	empty []ui.NotificationItem
	last  string
}

func notifyCenterReset(now time.Time) {
	notifyCenterDemo.items = []ui.NotificationItem{
		{Key: "build", Group: "今天", Level: "error", Title: "构建失败", Body: "go build ./...：ui/titlebar.go:42 未定义的标识符 frame。", Time: now.Add(-2 * time.Minute),
			Actions: []ui.NotificationAction{{Key: "open", Label: "打开文件", Primary: true}, {Key: "log", Label: "查看日志"}}},
		{Key: "update", Group: "今天", Level: "info", Title: "有可用更新", Body: "Tango 0.4.0 已发布，重启后安装。", Time: now.Add(-40 * time.Minute),
			Actions: []ui.NotificationAction{{Key: "restart", Label: "立即重启"}}},
		{Key: "push", Group: "今天", Level: "success", Title: "推送完成", Body: "已推送 3 个提交到 origin/main。", Time: now.Add(-3 * time.Hour), Read: true},
		{Key: "ssh", Group: "更早", Level: "warning", Title: "SSH 连接断开", Body: "dev-box 在 5 分钟内无响应，已自动重连。", Time: now.Add(-50 * time.Hour), Read: true},
	}
	notifyCenterDemo.empty = nil
	notifyCenterDemo.last = "—"
}

func notifyCenterPage(c *ui.Context) {
	if notifyCenterDemo.items == nil && notifyCenterDemo.last == "" {
		notifyCenterReset(c.Now())
	}
	gallery.Demo(c, "通知中心", "铃铛上的徽标是未读数；面板按分组列出通知，显示相对时间、操作按钮；点击通知或操作会标为已读并关闭面板，标题栏可全部标为已读或清空，单条可移除。", func() {
		ui.Row(c, func() {
			nc := ui.NotificationCenter(c, &notifyCenterDemo.items).Placement("bottomLeft")
			if k, ok := nc.Clicked(); ok {
				notifyCenterDemo.last = "打开 " + k
			}
			if k, a, ok := nc.Action(); ok {
				notifyCenterDemo.last = k + " → " + a
			}
			ui.Text(c, "最近操作："+notifyCenterDemo.last).Tertiary()
		}, layout.Gap(16), layout.Middle())
		ui.Column(c, func() {}, layout.H(470))
	})
	gallery.Demo(c, "空状态", "", func() {
		ui.NotificationCenter(c, &notifyCenterDemo.empty).Placement("bottomLeft")
	})
}

func init() {
	gallery.Register(gallery.Page{Name: "NotificationCenter", Title: "NotificationCenter 通知中心", Group: gallery.Feedback, Order: 60, Render: notifyCenterPage,
		Setup: func(d *ui.Driver) {
			notifyCenterReset(d.Now())
			d.Frame()
			// The bell sits left of the "最近操作" text.
			if r, ok := d.FindText("最近操作：—"); ok {
				bell := geom.Pt(r.X-30, r.Y+r.H/2)
				d.Click(bell)
				d.Settle()
				d.Move(geom.Pt(bell.X+200, bell.Y+150))
				d.Settle()
			}
		}})
}
