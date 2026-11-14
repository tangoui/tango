// Command webview-embed puts native WebViews inside a self-drawn layout
// (docs/specs/webview-embed.md): one fills the right side of the window,
// another sits in a scrolling list on the left and is clipped by it.
// Open the menu, hover the tooltip button or open the dialog to see a
// covered WebView freeze into a snapshot under the layer; Tab moves focus
// from the toolbar into the page and back out after its last field.
//
//	go run ./examples/webview-embed
//	go run ./examples/webview-embed -smoke   # macOS: scripted checks + screenshots in build/webview-embed
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"strings"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/asset"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/uihost"
)

//go:embed frontend
var frontend embed.FS

var (
	app    *tango.App
	win    *tango.Surface
	runner *ui.Runner

	mainWeb, inlineWeb *uihost.WebViewW
	listBox            *ui.ScrollBox
	menuPanel          ui.ID

	menuOpen, modalOpen bool
	showInline          = true
	pageClicks, sent    int
	pageFocus           string // last element focused in the main page
	pageKeys            int    // trusted key events seen by the main page
	afterID, beforeID   ui.ID
	afterFocused        bool
	loaded              = map[string]bool{}
	inlineClosed        bool

	// Requests from the smoke script, applied by the page function.
	scrollReq = -1.0
	focusReq  ui.ID
	menuID    ui.ID
)

func page(c *ui.Context) {
	th := c.Theme
	ui.Column(c, func() {
		ui.Row(c, func() {
			ui.Title(c, "自绘布局 + 原生 WebView", 4)
			ui.Spacer(c)
			send := ui.Button(c, "发送到网页")
			beforeID = send.ID()
			if send.Clicked() && mainWeb.Surface() != nil {
				sent++
				mainWeb.Surface().Send(fmt.Sprintf("来自自绘按钮的第 %d 条消息", sent))
			}
			ui.Tooltip(c, "自绘 Tooltip：与网页重叠时网页冻结成截图", func() { ui.Button(c, "悬停提示") })
			if ui.Button(c, "对话框").Clicked() {
				modalOpen = true
			}
			if ui.Button(c, "列表里的 WebView").Clicked() {
				showInline = !showInline
			}
			mb := ui.Button(c, "菜单 ▾").Primary()
			menuID = mb.ID()
			if mb.Clicked() {
				menuOpen = !menuOpen
			}
			if menuOpen {
				ov := ui.Overlay(c, ui.OverlayOptions{Anchor: mb.ID(), Place: ui.BottomEnd, Offset: 4}, func() {
					for _, s := range []string{"盖住网页的下拉菜单", "第二项", "第三项", "关闭菜单"} {
						if ui.Button(c, s).Text().Clicked() {
							menuOpen = false
						}
					}
				}, layout.Pad(6), layout.W(220))
				ov.Bg(th.BgElevated).Radius(th.RadiusLG).Shadow(th.Shadow)
				menuPanel = ov.ID()
				if c.Dismissed(menuPanel, mb.ID()) {
					menuOpen = false
				}
			}
		}, layout.Gap(8), layout.PadVH(10, 16), layout.Middle())

		ui.Row(c, func() {
			listBox = ui.Scroll(c, func() {
				for i := range 3 {
					card(c, i)
				}
				if showInline {
					inlineWeb = uihost.WebView(c, "inline", uihost.WebViewOptions{
						URL: "/inline.html",
						Created: func(s *tango.Surface) {
							inlineClosed = false
							s.OnLoaded(func() { loaded["inline"] = true })
							s.OnClosed(func() { inlineClosed = true })
						},
					}, layout.H(180))
					inlineWeb.Radius(8)
				} else {
					inlineWeb = nil
				}
				for i := 3; i < 9; i++ {
					card(c, i)
				}
			}, layout.W(320), layout.Gap(8), layout.Pad(4))
			if scrollReq >= 0 {
				listBox.ScrollTo(0, scrollReq)
				scrollReq = -1
			}

			mainWeb = uihost.WebView(c, "main", uihost.WebViewOptions{
				URL: "/index.html",
				Created: func(s *tango.Surface) {
					s.OnLoaded(func() { loaded["main"] = true })
				},
			}, layout.Grow(1))
			mainWeb.A11yLabel("网页")
		}, layout.Grow(1), layout.Gap(12), layout.PadVH(0, 16))

		ui.Row(c, func() {
			after := ui.Button(c, "网页之后的按钮")
			afterID = after.ID()
			afterFocused = c.Focused() == afterID
			ui.Text(c, status()).Secondary()
		}, layout.Gap(12), layout.PadVH(10, 16), layout.Middle())
	}, layout.Fill())
	if focusReq != 0 {
		c.SetFocus(focusReq)
		focusReq = 0
	}

	ui.Modal(c, &modalOpen, "自绘对话框").Body(func() {
		ui.Text(c, "对话框和遮罩盖住两个 WebView：它们暂时换成截图，关闭后恢复原生视图。")
	})
}

func card(c *ui.Context, i int) {
	ui.Card(c, fmt.Sprintf("自绘卡片 %d", i+1)).Body(func() {
		ui.Text(c, "滚动这个列表：中间的 WebView 跟着移动，并被列表边缘裁剪。")
	})
}

func status() string {
	frozen := mainWeb != nil && mainWeb.Frozen()
	return fmt.Sprintf("网页点击 %d 次 · 网页焦点 %s · 主 WebView 冻结 %v", pageClicks, orDash(pageFocus), frozen)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func main() {
	smoke := flag.Bool("smoke", false, "run scripted checks and save screenshots, then quit")
	flag.Parse()

	site, _ := fs.Sub(frontend, "frontend")
	app = tango.New(tango.Options{Name: "WebView Embed", ID: "dev.tango.webview-embed", Assets: asset.FS(site, asset.Options{})})
	app.OnMessage(func(s *tango.Surface, msg string) {
		if mainWeb == nil || s != mainWeb.Surface() {
			return
		}
		switch {
		case strings.HasPrefix(msg, "click:"):
			fmt.Sscan(strings.TrimPrefix(msg, "click:"), &pageClicks)
		case strings.HasPrefix(msg, "focus:"):
			pageFocus = strings.TrimPrefix(msg, "focus:")
		case strings.HasPrefix(msg, "key:"):
			pageKeys++
		}
		runner.Invalidate()
	})
	app.OnReady(func() {
		var err error
		runner, win, err = uihost.Open(app, uihost.Options{Surface: tango.SurfaceOptions{
			Title: "WebView 嵌入自绘布局", Width: 1100, Height: 720, Resizable: true,
		}}, page, ui.RunnerOptions{})
		if err != nil {
			log.Fatal(err)
		}
		win.OnClosed(app.Quit)
		if *smoke {
			go runSmoke()
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
