package main

// The "浮层" section of the list tab: a popup, a dialog and an action
// sheet, to exercise the system Back action (docs/specs/mobile-canvas.md
// §2.2). While one of them is open the runner reports
// ui.Context.WantBack, the Android canvas intercepts Back and delivers it
// as Escape, which closes the top-most layer; with nothing open Back
// leaves the page as usual. The last row opens the WebView page with a
// self-drawn overlay (web.go).

import (
	"fmt"
	"log"

	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/mobile"
)

var (
	popupOpen, dialogOpen, sheetOpen bool
	lastChoice                       = "—"
)

// overlayRows adds the section's rows.
func overlayRows(c *ui.Context) {
	mobile.List(c, "浮层 Overlay（返回键关闭最上层）").Body(func() {
		mobile.Cell(c, "返回键拦截 WantBack").Extra(onOff(c.WantBack()))
		mobile.Cell(c, "内存警告 EventMemoryWarning").Extra(fmt.Sprint(memWarnings))
		if mobile.Cell(c, "弹出层 Popup").Icon(icon.Appstore).Arrow().Clickable().Clicked() {
			popupOpen = true
			log.Printf("mobile-ui: popup open")
		}
		if mobile.Cell(c, "对话框 Dialog").Icon(icon.Info).Arrow().Clickable().Clicked() {
			dialogOpen = true
			log.Printf("mobile-ui: dialog open")
		}
		if mobile.Cell(c, "动作面板 ActionSheet").Icon(icon.More).Extra(lastChoice).Clickable().Clicked() {
			sheetOpen = true
			log.Printf("mobile-ui: action sheet open")
		}
		if mobile.Cell(c, "WebView + 自绘浮层").Desc("uihost.Attach").Icon(icon.Global).Arrow().Clickable().Clicked() {
			openWebPage()
		}
	})
}

// overlays builds the popups; they are drawn above the page whatever tab
// is shown.
func overlays(c *ui.Context) {
	p := mobile.Popup(c, &popupOpen).Body(func() {
		ui.Column(c, func() {
			ui.Text(c, "底部弹出层：点遮罩、下拉把手或按返回键关闭。")
			ui.Text(c, "Back 先关闭上层的对话框，再关闭这里。")
			if mobile.Button(c, "在上面再开一个对话框").Block().Clicked() {
				dialogOpen = true
			}
			if mobile.Button(c, "关闭").Primary().Block().Clicked() {
				popupOpen = false
			}
		}, layout.Pad(16), layout.Gap(12))
	})
	if p.Closed() {
		log.Printf("mobile-ui: popup closed")
	}
	d := mobile.Dialog(c, &dialogOpen, "对话框").Content("按返回键等同于取消。").Confirm("确定", "取消")
	if d.Confirmed() {
		log.Printf("mobile-ui: dialog confirmed")
	}
	sheet := mobile.ActionSheet(c, &sheetOpen, []mobile.Action{
		{Key: "copy", Text: "复制"}, {Key: "share", Text: "分享"}, {Key: "delete", Text: "删除", Danger: true},
	}).Cancel("取消")
	if i, ok := sheet.Selected(); ok {
		lastChoice = []string{"复制", "分享", "删除"}[i]
		log.Printf("mobile-ui: action sheet %q", lastChoice)
	}
}
