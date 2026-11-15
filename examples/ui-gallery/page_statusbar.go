package main

import (
	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
)

// StatusBar: the strip at the bottom of a workbench window.

var statusBarDemo struct{ last string }

func statusBarItems(c *ui.Context, branch string, problems int) (left, right func()) {
	left = func() {
		if ui.StatusBarItem(c, branch).Icon(icon.Branch).Tooltip("切换分支").Clicked() {
			statusBarDemo.last = "branch"
		}
		ui.StatusBarItem(c, "").Icon(icon.Sync).Loading(true).Tooltip("正在同步")
		if ui.StatusBarItem(c, "错误").Icon(icon.CloseCircle).Badge(problems).Tooltip("问题").Clicked() {
			statusBarDemo.last = "problems"
		}
		ui.StatusBarItem(c, "索引中").Progress(0.42).Static()
	}
	right = func() {
		ui.StatusBarItem(c, "行 12，列 5")
		ui.StatusBarItem(c, "UTF-8").Static()
		ui.StatusBarItem(c, "Go").Icon(icon.Code)
		ui.StatusBarItem(c, "").Icon(icon.Bell).Badge(3).Tooltip("通知")
	}
	return
}

func statusBarPage(c *ui.Context) {
	gallery.Demo(c, "工作台状态栏", "左右两区的条目：图标、文字、计数徽标、进度；可点击的条目悬停高亮并带提示，Tab 可达；Static 为纯展示。", func() {
		l, r := statusBarItems(c, "main", 2)
		ui.StatusBar(c).Left(l).Right(r)
		ui.Text(c, "最近点击："+statusBarDemo.last).Tertiary().Layout(layout.Margin(geom.Insets{Top: 12}))
	})
	gallery.Demo(c, "状态色", "整条状态栏着色（调试中、远程连接），或单个条目着色（警告、错误）。", func() {
		ui.Column(c, func() {
			l, r := statusBarItems(c, "debug", 0)
			ui.StatusBar(c).Status("primary").Left(l).Right(r)
			ui.StatusBar(c).Left(func() {
				ui.StatusBarItem(c, "远程：dev-box").Icon(icon.Monitor).Status("success")
				ui.StatusBarItem(c, "2 个警告").Icon(icon.Warning).Status("warning")
			}).Right(func() {
				ui.StatusBarItem(c, "构建失败").Icon(icon.CloseCircle).Status("error")
			})
		}, layout.Gap(12))
	})
}

func init() {
	gallery.Register(gallery.Page{Name: "StatusBar", Title: "StatusBar 状态栏", Group: gallery.Layout, Order: 60, Render: statusBarPage,
		Setup: func(d *ui.Driver) {
			statusBarDemo.last = "—"
			if r, ok := d.Find("main"); ok {
				d.Move(r.Center())
				d.Settle()
			}
		}})
}
