// Command ui-hotreload shows hot reload of the self-drawn UI under tan dev.
//
//	tan dev -C examples/ui-hotreload
//
// Edit greeting below and save: the window stays open, the new text shows
// up after the rebuild, and the counter and the input keep their values
// (uihost.Persist). A compile error leaves the running version on screen
// with a red banner until it is fixed.
//
// The coloured square in the top-left corner is derived from greeting, so
// scripts can check which version is on screen from one pixel. Flags:
// -tick increments the counter every 500 ms, -anim animates continuously
// and logs the frame rate, -big opens a 1440×900 window (2880×1800 on Retina),
// -hidden opens the window hidden and shows it a second later.
//
// The window title follows the counter (Surface.SetTitle), the "计数" menu
// in the menu bar increments, resets and centers the window (sys/menu),
// size and position changes are logged (OnResized / OnMoved), and closing
// the window asks for confirmation once (OnClose) — all of which act on the
// dev host's window under tan dev.
package main

import (
	"flag"
	"fmt"
	"hash/fnv"
	"image/color"
	"log"
	"math"
	"time"

	"github.com/tangoui/tango"
	"github.com/tangoui/tango/sys/menu"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/uihost"
)

const greeting = "你好，热加载 v1"

// state survives hot reloads.
type state struct {
	Count int
	Name  string
}

var (
	app    *tango.App
	runner *ui.Runner
	surf   *tango.Surface
	st     state
	shown  = -1 // count shown in the title
	anim   = flag.Bool("anim", false, "animate continuously and log the frame rate")

	frames   int
	since    = time.Now()
	lastSize geom.Size
	lastSc   float32
)

// Swatch is the colour of the corner square for a greeting.
func Swatch(s string) color.NRGBA {
	h := fnv.New32a()
	h.Write([]byte(s))
	v := h.Sum32()
	return color.NRGBA{R: uint8(v>>16) | 0x20, G: uint8(v>>8) | 0x20, B: uint8(v) | 0x20, A: 255}
}

func page(c *ui.Context) {
	if st.Count != shown && surf != nil {
		shown = st.Count
		surf.SetTitle(fmt.Sprintf("UI 热加载 — 计数 %d", st.Count))
	}
	ui.Column(c, func() {
		ui.Custom(c, func(maxW, maxH float32) geom.Size { return geom.Size{W: 48, H: 48} },
			func(l *paint.List, r geom.Rect) { l.Fill(r, geom.Radii{}, Swatch(greeting)) })
		ui.Column(c, func() {
			ui.Title(c, greeting, 3)
			ui.Row(c, func() {
				ui.Text(c, "计数")
				ui.Title(c, itoa(st.Count), 4)
				if ui.Button(c, "加一").Primary().Clicked() {
					st.Count++
				}
				if ui.Button(c, "清零").Clicked() {
					st.Count = 0
				}
			}, layout.Middle(), layout.Gap(12))
			ui.Input(c, &st.Name).Placeholder("输入点什么，热加载后还在")
			if *anim {
				ui.Custom(c, func(maxW, maxH float32) geom.Size { return geom.Size{W: maxW, H: 24} },
					func(l *paint.List, r geom.Rect) {
						t := float64(time.Now().UnixNano()) / 1e9
						x := r.X + float32(0.5+0.5*math.Sin(t*2))*(r.W-24)
						l.Fill(geom.Rect{X: x, Y: r.Y, W: 24, H: 24}, geom.Radii{TL: 12, TR: 12, BR: 12, BL: 12}, color.NRGBA{22, 119, 255, 255})
					})
				frames++
				lastSize, lastSc = c.Size(), c.Window().Scale()
				if d := time.Since(since); d >= 2*time.Second {
					log.Printf("hotreload: fps=%.1f size=%.0fx%.0f@%.0fx", float64(frames)/d.Seconds(), lastSize.W, lastSize.H, lastSc)
					frames, since = 0, time.Now()
				}
				c.Window().Invalidate()
			}
		}, layout.Pad(24), layout.Gap(16))
	})
}

// setUp wires the window and the menu bar.
func setUp(s *tango.Surface, hidden bool) {
	s.OnResized(func(w, h int) { log.Printf("hotreload: resized %dx%d", w, h) })
	s.OnMoved(func(x, y int) { log.Printf("hotreload: moved to %d,%d", x, y) })
	s.OnStateChanged(func(st tango.WindowState) { log.Printf("hotreload: state %v", st) })
	s.OnFocused(func() { log.Printf("hotreload: focused") })
	s.OnBlurred(func() { log.Printf("hotreload: blurred") })
	asked := false
	s.OnClose(func() bool {
		if !asked {
			asked = true
			log.Printf("hotreload: close refused once; close again to quit")
			return false
		}
		return true
	})
	if hidden {
		time.AfterFunc(time.Second, func() { s.Show(); log.Printf("hotreload: shown, visible=%v", s.IsVisible()) })
	}
	w, h := s.Size()
	log.Printf("hotreload: window %dx%d at %+v title=%q", w, h, s.Bounds(), s.Title())

	bump := func(n int) func() {
		return func() {
			if n == 0 {
				st.Count = 0
			} else {
				st.Count += n
			}
			log.Printf("hotreload: menu count=%d", st.Count)
			runner.Invalidate()
		}
	}
	m := menu.New()
	m.Role(menu.AppMenu)
	m.Role(menu.EditMenu)
	cm := m.Submenu("计数")
	cm.Add("加一", bump(1)).Accelerator = "CmdOrCtrl+K"
	cm.Add("清零", bump(0))
	cm.Separator()
	cm.Add("窗口居中", func() { _ = s.Center(); log.Printf("hotreload: centered at %+v", s.Bounds()) })
	m.Role(menu.WindowMenu)
	if err := menu.SetAppMenu(app, m); err != nil {
		log.Printf("hotreload: menu: %v", err)
	}
}

func itoa(n int) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

func main() {
	tick := flag.Bool("tick", false, "increment the counter every 500 ms")
	big := flag.Bool("big", false, "open a 1440×900 window (2880×1800 pixels on Retina)")
	hidden := flag.Bool("hidden", false, "open the window hidden and show it a second later")
	flag.Parse()
	uihost.Persist(&st)
	log.Printf("hotreload: start greeting=%q count=%d name=%q", greeting, st.Count, st.Name)

	quit := true
	app = tango.New(tango.Options{Name: "UI Hot Reload", ID: "dev.tango.uihotreload", QuitOnLastClose: &quit})
	app.OnReady(func() {
		so := tango.SurfaceOptions{Title: "UI 热加载", Width: 640, Height: 420, MinWidth: 400, MinHeight: 300, Resizable: true, Centered: true}
		if *big {
			so.Width, so.Height = 1440, 900
		}
		so.Hidden = *hidden
		r, s, err := uihost.Open(app, uihost.Options{Surface: so}, page, ui.RunnerOptions{})
		if err != nil {
			log.Fatal(err)
		}
		runner, surf = r, s
		setUp(s, *hidden)
		if *tick {
			go func() {
				for range time.Tick(500 * time.Millisecond) {
					app.Post(func() {
						st.Count++
						log.Printf("hotreload: count=%d", st.Count)
						runner.Invalidate()
					})
				}
			}()
		}
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
