package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/tangoui/tango/examples/ui-gallery/gallery"
	"github.com/tangoui/tango/ui"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/mobile"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/text"
	"github.com/tangoui/tango/ui/theme"
)

type paintColor = theme.Color

func themeHex(s string) theme.Color { return theme.Hex(s) }

// Phone canvas of the mobile screenshots (iPhone 14 class, logical pt).
const (
	phoneW, phoneH = 390, 844
	phoneScale     = 3
)

// phoneInsets are the simulated safe-area insets (status bar, home bar).
var phoneInsets = geom.Insets{Top: 47, Bottom: 34}

// phonePage wraps a screen body in a full phone page: status bar, the
// screen, and the page background of the mobile style; rtl lays the
// screen out right to left (the status bar is system chrome and keeps its
// physical layout).
func phonePage(style mobile.Style, rtl bool, body func(c *ui.Context)) func(c *ui.Context) {
	return func(c *ui.Context) {
		in := phoneInsets
		page := func() {
			mobile.Provider(c, mobile.Options{Style: style, SafeArea: &in}, func() {
				th := mobile.ThemeOf(c)
				ui.Column(c, func() {
					statusBar(c, th, in.Top)
					body(c)
				}, layout.Size(c.Size().W, c.Size().H)).Bg(th.PageBg)
			})
		}
		if rtl {
			ui.ConfigProvider(c).Direction(ui.RTL).Body(page)
			return
		}
		page()
	}
}

// statusBar draws a simulated system status bar in the top inset.
func statusBar(c *ui.Context, th *mobile.Theme, h float32) {
	ui.Custom(c, func(maxW, _ float32) geom.Size { return geom.Size{W: maxW, H: h} }, func(l *paint.List, r geom.Rect) {
		l.Fill(r, geom.Radii{}, th.Box)
		fs := text.Style{Size: 16, Weight: text.Semibold, LineHeight: 20}
		lay := text.Shape("9:41", fs, text.Options{NoWrap: true})
		l.Glyphs(geom.Pt(r.X+52-lay.Width/2, r.Y+17), lay.Glyphs, th.Text)
		// Signal, battery.
		x := r.Right() - 34
		l.Stroke(geom.Rect{X: x, Y: r.Y + 19, W: 25, H: 12}, geom.Rad(3.5), 1, theme.Fade(th.Text, .4))
		l.Fill(geom.Rect{X: x + 2, Y: r.Y + 21, W: 19, H: 8}, geom.Rad(2), th.Text)
		for i := 0; i < 4; i++ {
			bh := float32(4 + 2*i)
			l.Fill(geom.Rect{X: r.Right() - 70 + float32(i)*5, Y: r.Y + 31 - bh, W: 3, H: bh}, geom.Rad(1), th.Text)
		}
	})
}

type phoneVariant struct {
	suffix string
	style  mobile.Style
	theme  func() *theme.Theme
}

var phoneVariants = []phoneVariant{
	{"antd", mobile.AntMobile, func() *theme.Theme { return theme.AntDesign() }},
	{"vant", mobile.Vant, func() *theme.Theme { return theme.AntDesign() }},
	{"antd-dark", mobile.AntMobile, func() *theme.Theme { return theme.AntDesign(theme.Dark()) }},
	{"vant-dark", mobile.Vant, func() *theme.Theme { return theme.AntDesign(theme.Dark()) }},
	{"shadcn", mobile.Shadcn, func() *theme.Theme { return theme.Shadcn() }},
	{"shadcn-dark", mobile.Shadcn, func() *theme.Theme { return theme.Shadcn(theme.Dark()) }},
}

// renderPhoneShots writes build/ui-gallery/mobile/<screen>-<variant>.png
// (<screen>-<variant>-rtl.png when rtl) at 390×844 logical points, 3×
// device pixels.
func renderPhoneShots(only string, rtl bool) error {
	suffix := ""
	if rtl {
		suffix = "-rtl"
	}
	for _, s := range mobileScreens {
		if only != "" && only != s.name {
			continue
		}
		for _, v := range phoneVariants {
			d := ui.NewDriver(phonePage(v.style, rtl, s.make(true)), ui.DriverOptions{Width: phoneW, Height: phoneH, Scale: phoneScale, Theme: v.theme()})
			d.Settle()
			d.Settle()
			p := filepath.Join(*outDir, "mobile", s.name+"-"+v.suffix+suffix+".png")
			if err := d.SavePNG(p); err != nil {
				return err
			}
			fmt.Println("wrote", p)
		}
	}
	return nil
}

// The Mobile page's Setup writes the phone shots. It runs once per theme
// pass and direction pass, but the shots already cover every style and
// theme (phoneVariants), so they are written once per direction: the page
// records the direction it was rendered in (-dir rtl wraps it in an RTL
// ConfigProvider) and Setup skips a direction already written.
var (
	phoneShotsRTL  bool
	phoneShotsDone = map[bool]bool{}
)

func init() {
	// One gallery page per mobile screen: the screen inside a phone-sized
	// frame (popups stay closed so they do not cover the gallery).
	for i, s := range mobileScreens {
		s := s
		// The screen factory owns its values; keep the closure between frames
		// so the live gallery does not reset switches, inputs and selection.
		body := s.make(false)
		gallery.Register(gallery.Page{
			Name: "Mobile" + strings.ToUpper(s.name[:1]) + s.name[1:], Title: "Mobile · " + s.title, Group: gallery.Display, Order: 900 + i, Width: phoneW + 48,
			Render: func(c *ui.Context) {
				ui.Div(c, layout.Size(phoneW, phoneH)).Clip().Radius(24).Border(1, c.Theme.BorderSecondary).Body(func() {
					mobile.Provider(c, mobile.Options{SafeArea: &phoneInsets}, func() {
						th := mobile.ThemeOf(c)
						ui.Column(c, func() {
							statusBar(c, th, phoneInsets.Top)
							body(c)
						}, layout.Grow(1)).Bg(th.PageBg)
					})
				})
			},
		})
	}
	// The phone screenshots (all screens × Ant Mobile / Vant / shadcn × light /
	// dark at 3×) are produced by this page's Setup, once per direction:
	//   go run ./examples/ui-gallery -only Mobile           # <screen>-<variant>.png
	//   go run ./examples/ui-gallery -only Mobile -dir rtl  # <screen>-<variant>-rtl.png
	gallery.Register(gallery.Page{
		Name: "Mobile", Title: "Mobile 移动端组件", Group: gallery.Display, Order: 899, Width: phoneW + 48,
		Render: func(c *ui.Context) {
			phoneShotsRTL = c.IsRTL()
			ui.Text(c, fmt.Sprintf("手机画布 %d×%d、%d 倍输出：build/ui-gallery/mobile/<screen>-<antd|vant|shadcn|antd-dark|vant-dark|shadcn-dark>.png", phoneW, phoneH, phoneScale)).Tertiary()
		},
		Setup: func(*ui.Driver) {
			rtl := phoneShotsRTL
			if phoneShotsDone[rtl] {
				return
			}
			phoneShotsDone[rtl] = true
			if err := renderPhoneShots("", rtl); err != nil {
				fmt.Println("mobile shots:", err)
			}
		},
	})
}
