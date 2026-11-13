package main

// The mixed page (docs/specs/mobile-canvas.md §1.4, §6.3 item 3): a
// WebView surface (app.NewSurface on the inline page below) with a
// self-drawn canvas attached above it (uihost.Attach, PassThrough). The
// canvas fills the page and is transparent except for a card at the
// bottom, so taps elsewhere reach the HTML (its button counts them and
// reports the count to Go). The card's button opens a mobile.Popup on the
// canvas: Back closes the popup first, then the page.

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"testing/fstest"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/asset"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/mobile"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

const webHTML = `<!doctype html>
<html><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>WebView + 自绘浮层</title>
<style>
body { font: 17px -apple-system, system-ui, sans-serif; margin: 0;
       padding: calc(env(safe-area-inset-top) + 24px) 20px 240px; background: #f4f6fb; color: #1f2329; }
h1 { font-size: 22px; }
button { font-size: 17px; padding: 12px 20px; border-radius: 10px; border: 0; background: #1677ff; color: #fff; }
p { line-height: 1.6; }
</style></head>
<body>
<h1>这是 WebView 页面</h1>
<p>下方的卡片是 Tango 自绘的 canvas，叠在 WebView 上面（uihost.Attach，Above + PassThrough）。
卡片以外的透明区域把触摸交给网页：点下面的按钮试试。</p>
<p><button id="b">网页按钮</button> <span id="n">0</span> 次</p>
<p>按返回键：浮层里的弹出层打开时先关闭它，否则关闭这个页面。</p>
<script>
let n = 0;
document.getElementById('b').onclick = () => {
  n++; document.getElementById('n').textContent = n;
  if (window.__tango) window.__tango.post('web-click:' + n);
};
</script>
</body></html>
`

var (
	webSurface *tango.Surface
	webRunner  *ui.Runner
	webClicks  int
	webPopup   bool
)

// webAssets serves the inline page at /web.html.
func webAssets() http.Handler {
	var site fs.FS = fstest.MapFS{"web.html": {Data: []byte(webHTML)}}
	return asset.FS(site, asset.Options{})
}

// webMessage handles the page's messages (App.OnMessage).
func webMessage(s *tango.Surface, msg string) {
	if v, ok := strings.CutPrefix(msg, "web-click:"); ok {
		fmt.Sscan(v, &webClicks)
		log.Printf("mobile-ui: web page click #%d", webClicks)
		if webRunner != nil {
			webRunner.Invalidate()
		}
	}
}

// openWebPage opens the WebView surface and attaches the overlay.
func openWebPage() {
	if webSurface != nil {
		return
	}
	s, err := app.NewSurface(tango.SurfaceOptions{Title: "WebView + 自绘浮层", URL: "/web.html", Width: 390, Height: 844})
	if err != nil {
		log.Printf("mobile-ui: web page: %v", err)
		return
	}
	webSurface = s
	s.OnClosed(func() {
		log.Printf("mobile-ui: web page closed")
		webSurface, webRunner, webPopup = nil, nil, false
	})
	// A transparent layout background: only the card and the popup are
	// opaque, so PassThrough leaves the rest of the page to the WebView.
	th := theme.AntDesign()
	th.BgLayout = theme.Color{}
	r, _, err := uihost.Attach(s, uihost.Layer{Above: true, PassThrough: true}, webOverlay, ui.RunnerOptions{Theme: th})
	if err != nil {
		log.Printf("mobile-ui: uihost.Attach: %v (the page shows without the overlay)", err)
		return
	}
	webRunner = r
	log.Printf("mobile-ui: web page %d open, overlay attached", s.ID())
}

// webOverlay is the attached canvas's page.
func webOverlay(c *ui.Context) {
	mobile.Provider(c, mobile.Options{}, func() {
		ui.Column(c, func() {
			ui.Div(c, layout.Grow(1)) // transparent: taps go to the WebView
			ui.Column(c, func() {
				ui.Column(c, func() {
					ui.Text(c, "自绘浮层 · uihost.Attach")
					ui.Text(c, fmt.Sprintf("网页按钮被点了 %d 次；返回键拦截：%s", webClicks, onOff(c.WantBack())))
					ui.Row(c, func() {
						if mobile.Button(c, "打开弹出层").Primary().Clicked() {
							webPopup = true
						}
						if mobile.Button(c, "关闭页面").Clicked() && webSurface != nil {
							webSurface.Close()
						}
					}, layout.Gap(12))
				}, layout.Pad(16), layout.Gap(8)).Bg(c.Theme.BgContainer).Radius(12)
				mobile.SafeArea(c, mobile.Bottom)
			}, layout.PadVH(0, 12))
		}, layout.Grow(1))
		mobile.Popup(c, &webPopup).Body(func() {
			ui.Column(c, func() {
				ui.Text(c, "画在 WebView 上方的弹出层。按返回键先关闭它。")
				if mobile.Button(c, "关闭").Primary().Block().Clicked() {
					webPopup = false
				}
			}, layout.Pad(16), layout.Gap(12))
		})
	})
}
