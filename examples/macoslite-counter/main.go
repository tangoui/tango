// macoslite-counter is the reference window for the macoslite package: a
// counter drawn with Canvas calls only, used to measure the idle memory of a
// native lite window against other frameworks (see README.md).
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/tangoui/tango/macoslite"
)

// host is the part of *macoslite.App the view uses; tests fake it.
type host interface {
	Invalidate()
	Stop()
}

// Layout, in points; colors follow the Ant Design 6 default tokens
// (colorPrimary #1677FF, colorText rgba(0,0,0,0.88), colorBgLayout #F5F5F5,
// default button border #D9D9D9, borderRadius 6).
var (
	minusRect = macoslite.Rect{X: 24, Y: 96, W: 88, H: 32}
	resetRect = macoslite.Rect{X: 136, Y: 96, W: 88, H: 32}
	plusRect  = macoslite.Rect{X: 248, Y: 96, W: 88, H: 32}

	bg      = macoslite.Color{R: 0xF5, G: 0xF5, B: 0xF5, A: 255}
	text    = macoslite.Color{A: 224}
	primary = macoslite.Color{R: 0x16, G: 0x77, B: 0xFF, A: 255}
	border  = macoslite.Color{R: 0xD9, G: 0xD9, B: 0xD9, A: 255}
	white   = macoslite.Color{R: 255, G: 255, B: 255, A: 255}
)

type counter struct {
	host  host
	n     int
	press *macoslite.Rect // button pressed and not yet released
}

func (c *counter) Draw(f *macoslite.Frame, cv *macoslite.Canvas) {
	cv.Fill(macoslite.Rect{W: f.Width, H: f.Height}, bg)
	cv.Text(strconv.Itoa(c.n), macoslite.Point{X: 24, Y: 64}, 32, text)
	button(cv, minusRect, "−", false)
	button(cv, resetRect, "重置", false)
	button(cv, plusRect, "+", true)
	if os.Getenv("MACOSLITE_FIRST_DRAW") != "" {
		fmt.Println("first-draw")
		os.Unsetenv("MACOSLITE_FIRST_DRAW")
	}
}

func button(cv *macoslite.Canvas, r macoslite.Rect, label string, primaryBtn bool) {
	fill, ink := white, text
	if primaryBtn {
		fill, ink = primary, white
	} else {
		cv.RoundRect(r, 6, border)
		r = macoslite.Rect{X: r.X + 1, Y: r.Y + 1, W: r.W - 2, H: r.H - 2}
	}
	cv.RoundRect(r, 6, fill)
	cv.Text(label, macoslite.Point{X: r.X + r.W/2 - 7*float64(len([]rune(label))), Y: r.Y + r.H/2 + 5}, 14, ink)
}

func (c *counter) Mouse(e macoslite.MouseEvent) {
	hit := func() *macoslite.Rect {
		for _, r := range []*macoslite.Rect{&minusRect, &resetRect, &plusRect} {
			if r.Contains(e.Pos) {
				return r
			}
		}
		return nil
	}
	switch e.Kind {
	case macoslite.MouseDown:
		c.press = hit()
	case macoslite.MouseUp:
		if r := hit(); r != nil && r == c.press {
			switch r {
			case &minusRect:
				c.set(c.n - 1)
			case &resetRect:
				c.set(0)
			case &plusRect:
				c.set(c.n + 1)
			}
		}
		c.press = nil
	}
}

func (c *counter) Key(e macoslite.KeyEvent) {
	if !e.Down {
		return
	}
	switch e.Key {
	case "Escape":
		c.host.Stop()
	case "+", "=":
		c.set(c.n + 1)
	case "-":
		c.set(c.n - 1)
	case "0":
		c.set(0)
	}
}

func (c *counter) set(n int) {
	c.n = n
	c.host.Invalidate()
}

func (c *counter) Close() {}

func main() {
	c := &counter{}
	app, err := macoslite.New(macoslite.Options{Name: "Lite Counter", Title: "Lite Counter", Width: 360, Height: 260}, c)
	if err != nil {
		log.Fatal(err)
	}
	c.host = app
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
