package ui

import (
	"image/color"
	"strings"
	"testing"

	"github.com/tangoui/tango/ui/a11y"
	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

func p2dPixel(d *Driver, x, y float32) color.RGBA {
	return d.Image().RGBAAt(int(x), int(y))
}

func p2dNear(a, b color.RGBA) bool {
	diff := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return diff(a.R, b.R) <= 3 && diff(a.G, b.G) <= 3 && diff(a.B, b.B) <= 3
}

func p2dRGBA(c theme.Color) color.RGBA { return color.RGBA{c.R, c.G, c.B, 255} }

func TestP2DInputShowCount(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		v := ""
		var w *InputW
		d := p2dDriver(th, func(c *Context) { w = Input(c, &v).ShowCount().MaxLength(5).A11yLabel("名称").Width(240) })
		d.EnableAccessibility()
		d.Click(d.r.c.rects[w.box.id].Center())
		d.Type("abcdefg")
		if v != "abcde" {
			t.Fatalf("max length: %q", v)
		}
		cr, ok := d.FindText("5 / 5")
		if !ok {
			t.Fatal("count not shown")
		}
		br := d.r.c.rects[w.box.id]
		if cr.Right() > br.Right() || cr.X < br.X+br.W/2 {
			t.Fatalf("count %v not at the right of %v", cr, br)
		}
		// The text must not run under the count.
		if er := d.r.c.rects[w.ed.id]; er.Right() > cr.X {
			t.Fatalf("editor %v overlaps count %v", er, cr)
		}
		st, ok := d.A11yFind(a11y.RoleStatus, "5 / 5")
		if !ok || st.Live == a11y.LiveOff {
			t.Fatalf("count is not a live region: %+v", st)
		}
		tb, _ := d.A11yFind(a11y.RoleTextbox, "名称")
		if tb == nil || tb.Description != "5 / 5" {
			t.Fatalf("textbox description: %+v", tb)
		}
	})
}

func TestP2DInputVariantsBg(t *testing.T) {
	red := theme.Hex("#ff0000")
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		v := ""
		var filled, under, custom, plain *InputW
		d := p2dDriver(th, func(c *Context) {
			filled = Input(c, &v).Variant("filled").A11yLabel("a").Width(200)
			under = Input(c, &v).Variant("underlined").A11yLabel("b").Width(200)
			custom = Input(c, &v).Bg(red).NoBorder().A11yLabel("c").Width(200)
			plain = Input(c, &v).Variant("borderless").A11yLabel("d").Width(200)
		})
		page := p2dRGBA(th.BgLayout)
		_ = page
		fr := d.r.c.rects[filled.box.id]
		fill := p2dPixel(d, fr.X+4, fr.Y+fr.H/2)
		edge := p2dPixel(d, fr.X, fr.Y+fr.H/2)
		if p2dNear(fill, p2dRGBA(th.BgContainer)) || !p2dNear(fill, edge) {
			t.Fatalf("filled: fill %v edge %v", fill, edge)
		}
		ur := d.r.c.rects[under.box.id]
		top := p2dPixel(d, ur.X+ur.W/2, ur.Y)
		bottom := p2dPixel(d, ur.X+ur.W/2, ur.Bottom()-1)
		if p2dNear(top, bottom) {
			t.Fatalf("underlined: top %v bottom %v", top, bottom)
		}
		cr := d.r.c.rects[custom.box.id]
		if px := p2dPixel(d, cr.X+4, cr.Y+cr.H/2); !p2dNear(px, p2dRGBA(red)) {
			t.Fatalf("custom bg %v", px)
		}
		if px := p2dPixel(d, cr.X, cr.Y+cr.H/2); !p2dNear(px, p2dRGBA(red)) {
			t.Fatalf("NoBorder left edge %v", px)
		}
		// Focus does not bring the border back.
		d.Click(cr.Center())
		d.Settle()
		if px := p2dPixel(d, cr.X, cr.Y+cr.H/2); !p2dNear(px, p2dRGBA(red)) {
			t.Fatalf("NoBorder focused edge %v", px)
		}
		pr := d.r.c.rects[plain.box.id]
		if a, b := p2dPixel(d, pr.X, pr.Y+pr.H/2), p2dPixel(d, pr.X-3, pr.Y+pr.H/2); !p2dNear(a, b) {
			t.Fatalf("borderless draws a frame: %v vs %v", a, b)
		}
	})
}

func TestP2DInputConfigDefaults(t *testing.T) {
	v := "abc"
	var w *InputW
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		ConfigProvider(c).Defaults("Input", Props{"variant": "filled", "showCount": true, "maxLength": 10}).Body(func() {
			w = Input(c, &v).A11yLabel("x")
		})
	})
	if w.variant != "filled" || !w.showCount || w.maxLen != 10 {
		t.Fatalf("defaults: %q %v %d", w.variant, w.showCount, w.maxLen)
	}
	if _, ok := d.FindText("3 / 10"); !ok {
		t.Fatal("count missing")
	}
}

// Custom boxes paint their Bg and can be found by label.
func TestP2DCustomBgAndLabel(t *testing.T) {
	red := theme.Hex("#ff0000")
	clicks := 0
	var b *Box
	d := p2dDriver(theme.AntDesign(), func(c *Context) {
		b = Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 80, H: 40} }, func(l *paint.List, r geom.Rect) {}).Bg(red).Label("画布")
		if b.Clicked() {
			clicks++
		}
		Custom(c, func(_, _ float32) geom.Size { return geom.Size{W: 80, H: 40} }, nil).Bg(red).Radius(4)
	})
	r := b.Rect()
	if px := p2dPixel(d, r.X+r.W/2, r.Y+r.H/2); !p2dNear(px, p2dRGBA(red)) {
		t.Fatalf("Custom Bg not painted: %v", px)
	}
	if fr, ok := d.FindText("画布"); !ok || fr != r {
		t.Fatalf("FindText: %v %v", fr, ok)
	}
	if err := d.ClickLabel("画布"); err != nil || clicks != 1 {
		t.Fatalf("ClickLabel: %v clicks %d", err, clicks)
	}
}

func TestP2DInputNumberFormatterPrefix(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		v := 1000.0
		var w *InputNumberW
		money := func(s string) string {
			neg := strings.HasPrefix(s, "-")
			s = strings.TrimPrefix(s, "-")
			ip, fp, hasF := strings.Cut(s, ".")
			var b strings.Builder
			for i, r := range ip {
				if i > 0 && (len(ip)-i)%3 == 0 {
					b.WriteByte(',')
				}
				b.WriteRune(r)
			}
			out := b.String()
			if hasF {
				out += "." + fp
			}
			if neg {
				out = "-" + out
			}
			return "$ " + out
		}
		parse := func(s string) string { return strings.NewReplacer("$", "", ",", "", " ", "").Replace(s) }
		d := p2dDriver(th, func(c *Context) {
			w = InputNumber(c, &v).Formatter(money).Parser(parse).Prefix(nil).PrefixText("¥").Width(200).A11yLabel("金额")
		})
		if _, ok := d.FindText("$ 1,000"); ok {
			// FindText sees labels only; check the display directly.
		}
		if s := w.display(); s != "$ 1,000" {
			t.Fatalf("display %q", s)
		}
		er := d.r.c.rects[w.ed.id]
		br := d.r.c.rects[w.box.id]
		if er.X-br.X < 11+8 {
			t.Fatalf("prefix room missing: editor at %v in %v", er.X, br.X)
		}
		d.Click(er.Center())
		d.Key(input.KeyEnd, 0)
		d.Type("5")
		if v != 10005 {
			t.Fatalf("parsed %v", v)
		}
		d.Click(geom.Pt(850, 650))
		if s := w.display(); s != "$ 10,005" {
			t.Fatalf("display after blur %q", s)
		}
	})
}

func TestP2DInputNumberSidesWheel(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		v := 5.0
		var w *InputNumberW
		d := p2dDriver(th, func(c *Context) {
			w = InputNumber(c, &v).ControlsPosition("sides").Min(0).Max(6).Wheel().A11yLabel("数量")
		})
		d.EnableAccessibility()
		d.Settle()
		br, up, dn := d.r.c.rects[w.box.id], d.r.c.rects[w.up.id], d.r.c.rects[w.down.id]
		if br.W != 150 || dn.X != br.X || up.Right() != br.Right() || up.W != br.H {
			t.Fatalf("sides layout: box %v up %v down %v", br, up, dn)
		}
		d.Click(up.Center())
		if v != 6 {
			t.Fatalf("plus: %v", v)
		}
		d.Click(up.Center()) // at Max
		if v != 6 {
			t.Fatalf("clamped plus: %v", v)
		}
		d.Click(dn.Center())
		if v != 5 {
			t.Fatalf("minus: %v", v)
		}
		sb, ok := d.A11yFind(a11y.RoleSpinButton, "数量")
		if !ok || !sb.HasRange || sb.ValueNow != 5 || sb.ValueMin != 0 || sb.ValueMax != 6 {
			t.Fatalf("spinbutton: %+v", sb)
		}
		// Wheel only steps with focus.
		d.Scroll(br.Center(), -40)
		if v != 5 {
			t.Fatalf("wheel without focus changed %v", v)
		}
		d.Click(d.r.c.rects[w.ed.id].Center())
		d.Scroll(br.Center(), -40)
		d.Scroll(br.Center(), -40)
		if v != 6 {
			t.Fatalf("wheel up: %v", v)
		}
		d.Scroll(br.Center(), 40)
		if v != 5 {
			t.Fatalf("wheel down: %v", v)
		}
	})
}

func TestP2DInputNumberStringMode(t *testing.T) {
	p2dThemes(t, func(t *testing.T, th *theme.Theme) {
		s := "0.1"
		big := "12345678901234567890.1"
		var w, wb *InputNumberW
		d := p2dDriver(th, func(c *Context) {
			w = InputNumber(c, nil).StringMode(&s).Step(0.2).A11yLabel("小数")
			wb = InputNumber(c, nil).StringMode(&big).Width(260).A11yLabel("大数")
		})
		d.EnableAccessibility()
		d.Click(d.r.c.rects[w.ed.id].Center())
		d.Key(input.KeyUp, 0)
		if s != "0.3" {
			t.Fatalf("0.1 + 0.2 = %q", s)
		}
		d.Click(d.r.c.rects[wb.ed.id].Center())
		d.Key(input.KeyUp, 0)
		if big != "12345678901234567891.1" {
			t.Fatalf("big + 1 = %q", big)
		}
		// Typing keeps every digit.
		d.Key(input.KeyA, input.ModCtrl|input.ModMeta)
		d.Type("0.30000000000000000001")
		d.Key(input.KeyEnter, 0)
		if big != "0.30000000000000000001" {
			t.Fatalf("typed %q", big)
		}
		if sb, ok := d.A11yFind(a11y.RoleSpinButton, "大数"); !ok || sb.Value != big {
			t.Fatalf("a11y value %+v", sb)
		}
		// Precision rounds the decimal (half away from zero).
		d2 := p2dDriver(th, func(c *Context) {
			w = InputNumber(c, nil).StringMode(&s).Precision(2).Step(0.005).A11yLabel("p")
		})
		s = "1.00"
		d2.Settle()
		d2.Click(d2.r.c.rects[w.ed.id].Center())
		d2.Key(input.KeyUp, 0)
		if s != "1.01" {
			t.Fatalf("precision: %q", s)
		}
		// Clearing the text empties the value.
		d2.Key(input.KeyA, input.ModCtrl|input.ModMeta)
		d2.Key(input.KeyBackspace, 0)
		if s != "" {
			t.Fatalf("cleared: %q", s)
		}
	})
}

func TestP2DDecimal(t *testing.T) {
	cases := []struct{ a, b, sum string }{
		{"0.1", "0.2", "0.3"},
		{"-1.5", "1.5", "0"},
		{"99999999999999999999", "1", "100000000000000000000"},
		{"1e-3", "0.001", "0.002"},
		{"-0.25", "0.05", "-0.2"},
	}
	for _, c := range cases {
		a, ok1 := parseDec(c.a)
		b, ok2 := parseDec(c.b)
		if !ok1 || !ok2 {
			t.Fatalf("parse %q %q", c.a, c.b)
		}
		if got := a.add(b).String(); got != c.sum {
			t.Errorf("%s + %s = %s, want %s", c.a, c.b, got, c.sum)
		}
	}
	for _, c := range []struct {
		s    string
		p    int
		want string
	}{{"1.005", 2, "1.01"}, {"-1.005", 2, "-1.01"}, {"2.5", 0, "3"}, {"1.2", 3, "1.200"}, {"0.004", 2, "0.00"}} {
		d, _ := parseDec(c.s)
		if got := d.fixed(c.p); got != c.want {
			t.Errorf("fixed(%s, %d) = %s, want %s", c.s, c.p, got, c.want)
		}
	}
	for _, bad := range []string{"", "-", ".", "1.2.3", "abc", "1e"} {
		if _, ok := parseDec(bad); ok {
			t.Errorf("parsed %q", bad)
		}
	}
}

func TestP2DInputNumberConfigDefaults(t *testing.T) {
	v := 1.0
	var w *InputNumberW
	p2dDriver(theme.ElementPlus(), func(c *Context) {
		ConfigProvider(c).Defaults("InputNumber", Props{"controlsPosition": "sides", "wheel": true}).Body(func() {
			w = InputNumber(c, &v).A11yLabel("n")
		})
	})
	if w.ctrlPos != "sides" || !w.wheel {
		t.Fatalf("defaults: %q %v", w.ctrlPos, w.wheel)
	}
}
