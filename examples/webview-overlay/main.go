// Command webview-overlay mixes a WebView page with Tango's self-drawn UI
// in one window (docs/specs/canvas-surface.md, step D).
//
//	go run ./examples/webview-overlay          # canvas above the page
//	go run ./examples/webview-overlay -below   # canvas below a transparent page
//
// Above (default): a full-window canvas with PassThrough sits on the
// WebView. Only the tool card in the bottom-right corner and the round
// floating button (drag it around; click it to hide or show the card) are
// opaque; everywhere else the canvas is transparent, so clicks, wheel
// scrolling and text selection reach the page underneath.
//
// Below (-below, or the switch buttons): the canvas draws the window
// background and the page, whose CSS background is transparent, floats
// over it. The canvas gets no pointer input there; the page's toggle
// button (a JS message) brings it back on top.
package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"math"
	"strings"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/asset"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
	"github.com/tangoui/tango/uihost"
)

//go:embed frontend
var frontend embed.FS

var (
	app      *tango.App
	surface  *tango.Surface
	runner   *ui.Runner
	attached *uihost.Attached

	below      bool
	pageClicks int
	sent       int
	showCard   = true
	showFab    = true

	fab               = geom.Pt(40, 0) // top-left of the floating button; Y set from the window height
	fabGrab           geom.Point
	fabMoved, fabInit bool
)

func layer() uihost.Layer {
	// Zero Bounds: fill the window and follow its size.
	return uihost.Layer{Above: !below, PassThrough: !below}
}

func toggle() {
	below = !below
	if err := attached.SetLayer(layer()); err != nil {
		log.Print(err)
	}
	runner.Invalidate()
}

func page(c *ui.Context) {
	if below {
		background(c)
		return
	}
	if showCard {
		toolCard(c)
	}
	if showFab {
		floatingButton(c)
	}
}

// toolCard is an ordinary ui.Card pinned to the bottom-right corner.
func toolCard(c *ui.Context) {
	ui.Card(c, "自绘工具面板").Shadow("always").
		Layout(layout.Abs(layout.Unset, layout.Unset, 20, 20), layout.W(300)).
		Body(func() {
			ui.Column(c, func() {
				ui.Text(c, fmt.Sprintf("网页按钮已点击 %d 次（来自 JS 消息）", pageClicks))
				if ui.Button(c, "给网页发消息").Primary().Block().Clicked() {
					sent++
					surface.Send(fmt.Sprintf("来自自绘面板的第 %d 条消息", sent))
				}
				ui.Row(c, func() {
					ui.Text(c, "显示悬浮按钮")
					ui.Spacer(c)
					ui.Switch(c, &showFab)
				}, layout.Middle())
				if ui.Button(c, "把自绘层移到网页下方").Block().Clicked() {
					app.Post(toggle) // after this frame
				}
			}, layout.Gap(12))
		})
}

// floatingButton is a round button that can be dragged; a click without
// dragging hides or shows the card.
func floatingButton(c *ui.Context) {
	const d = 56
	sz := c.Size()
	if !fabInit && sz.H > 0 {
		fab.Y, fabInit = sz.H-d-40, true
	}
	th := c.Theme
	b := ui.Div(c, layout.Abs(fab.X, fab.Y, layout.Unset, layout.Unset), layout.Size(d, d), layout.Center()).
		Radius(d / 2).Bg(th.Primary.Base)
	b.Body(func() { ui.Text(c, "拖我").Color(th.TextOnPrimary) })
	cur := input.CursorGrab
	if c.Dragging(b.ID()) {
		cur = input.CursorGrabbing
	}
	ia := b.Interact(cur)
	p, _ := c.Pointer()
	if ia.PressedNow {
		fabGrab, fabMoved = p.Sub(fab), false
	}
	if c.Dragging(b.ID()) {
		next := p.Sub(fabGrab)
		next.X = float32(math.Max(0, math.Min(float64(sz.W-d), float64(next.X))))
		next.Y = float32(math.Max(0, math.Min(float64(sz.H-d), float64(next.Y))))
		if next != fab {
			fabMoved = true
			fab = next
			runner.Invalidate() // lay out at the new place
		}
	}
	if ia.Clicked && !fabMoved {
		showCard = !showCard
	}
}

// background is drawn under the transparent page in -below mode.
func background(c *ui.Context) {
	sz := c.Size()
	ui.Div(c, layout.Abs(0, 0, layout.Unset, layout.Unset), layout.Size(sz.W, sz.H)).
		Bg(theme.Hex("#e8f0ff")).Body(func() {
		for i, col := range []string{"#adc6ff", "#ffd6e7", "#b7eb8f"} {
			r := float32(160 + 60*i)
			x := sz.W*float32(i+1)/4 - r/2
			y := sz.H - r*0.8 - float32(i*40)
			ui.Div(c, layout.Abs(x, y, layout.Unset, layout.Unset), layout.Size(r, r)).Radius(r / 2).Bg(theme.Hex(col))
		}
		ui.Div(c, layout.Abs(layout.Unset, layout.Unset, 24, 20)).Body(func() {
			ui.Text(c, "这一层是自绘背景，网页（背景透明）叠在上面")
		})
	})
}

func main() {
	flag.BoolVar(&below, "below", false, "start with the canvas below the page")
	flag.Parse()

	site, _ := fs.Sub(frontend, "frontend")
	app = tango.New(tango.Options{Name: "WebView Overlay", ID: "dev.tango.webview-overlay", Assets: asset.FS(site, asset.Options{})})
	app.OnMessage(func(s *tango.Surface, msg string) {
		switch {
		case msg == "toggle":
			toggle()
		case strings.HasPrefix(msg, "click:"):
			fmt.Sscan(strings.TrimPrefix(msg, "click:"), &pageClicks)
			runner.Invalidate()
		}
	})
	app.OnReady(func() {
		var err error
		surface, err = app.NewSurface(tango.SurfaceOptions{
			Title: "WebView + 自绘浮层", URL: "/index.html", Width: 960, Height: 640, Resizable: true,
		})
		if err != nil {
			log.Fatal(err)
		}
		// A transparent layout background: only widgets are opaque, so
		// PassThrough leaves the rest of the window to the page.
		th := theme.AntDesign()
		th.BgLayout = theme.Color{}
		runner, attached, err = uihost.Attach(surface, layer(), page, ui.RunnerOptions{Theme: th})
		if err != nil {
			log.Fatal(err)
		}
		surface.OnClosed(app.Quit)
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
