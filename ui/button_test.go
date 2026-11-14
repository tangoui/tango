package ui

import (
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

func buttonThemes() []*theme.Theme {
	return []*theme.Theme{theme.AntDesign(), theme.AntDesign(theme.Dark()), theme.ElementPlus(), theme.Shadcn(), theme.Shadcn(theme.Dark())}
}

// The variant modifiers and their color × appearance spelling draw the
// same button.
func TestButtonComboMatchesVariants(t *testing.T) {
	pairs := []struct {
		name     string
		old, new func(b *ButtonW)
	}{
		{"primary", func(b *ButtonW) { b.Primary() }, func(b *ButtonW) { b.Color(ButtonColorPrimary).Appearance(ButtonSolid) }},
		{"default", func(b *ButtonW) {}, func(b *ButtonW) { b.Color(ButtonColorDefault).Appearance(ButtonOutlined) }},
		{"dashed", func(b *ButtonW) { b.Dashed() }, func(b *ButtonW) { b.Appearance(ButtonDashed) }},
		{"text", func(b *ButtonW) { b.Text() }, func(b *ButtonW) { b.Color(ButtonColorDefault).Appearance(ButtonText) }},
		{"link", func(b *ButtonW) { b.Link() }, func(b *ButtonW) { b.Appearance(ButtonLink) }},
		{"secondary", func(b *ButtonW) { b.Secondary() }, func(b *ButtonW) { b.Appearance(ButtonFilled) }},
		{"danger primary", func(b *ButtonW) { b.Primary().Danger() }, func(b *ButtonW) { b.Color(ButtonColorDanger).Appearance(ButtonSolid) }},
		{"danger", func(b *ButtonW) { b.Danger() }, func(b *ButtonW) { b.Color(ButtonColorDanger) }},
		{"danger text", func(b *ButtonW) { b.Text().Danger() }, func(b *ButtonW) { b.Color(ButtonColorDanger).Appearance(ButtonText) }},
		{"primary + color", func(b *ButtonW) { b.Primary() }, func(b *ButtonW) { b.Primary().Color(ButtonColorPrimary) }},
	}
	for _, th := range buttonThemes() {
		for _, p := range pairs {
			var a, b *ButtonW
			d := NewDriver(func(c *Context) {
				Row(c, func() {
					a = Button(c, "A")
					p.old(a)
					b = Button(c, "B")
					p.new(b)
				})
			}, DriverOptions{Theme: th})
			d.Frame()
			if la, lb := a.look(), b.look(); !lookEqual(la, lb) {
				t.Errorf("%v %s: %+v != %+v", th.Flavor, p.name, la, lb)
			}
		}
	}
}

func lookEqual(a, b btnLook) bool {
	return a.bg == b.bg && a.border == b.border && a.fg == b.fg && a.dashed == b.dashed &&
		a.opacity == b.opacity && a.underline == b.underline && len(a.shadow) == len(b.shadow)
}

func TestButtonPresetColors(t *testing.T) {
	for _, th := range buttonThemes() {
		looks := map[ButtonAppearance]btnLook{}
		var bs []*ButtonW
		aps := []ButtonAppearance{ButtonSolid, ButtonOutlined, ButtonDashed, ButtonFilled, ButtonText, ButtonLink, ButtonPlain}
		d := NewDriver(func(c *Context) {
			bs = bs[:0]
			Row(c, func() {
				for _, ap := range aps {
					bs = append(bs, Button(c, string(ap)).Color(ButtonColorPurple).Appearance(ap))
				}
			})
		}, DriverOptions{Theme: th})
		d.Frame()
		for i, ap := range aps {
			looks[ap] = bs[i].look()
		}
		pal := th.ButtonPalette(theme.Hex(antPreset["purple"]))
		if l := looks[ButtonSolid]; l.bg != pal.Base || l.fg != th.TextOn(pal.Base, th.TextOnSolid) {
			t.Errorf("%v solid: %+v", th.Flavor, l)
		}
		if l := looks[ButtonOutlined]; l.border != pal.Base || l.dashed {
			t.Errorf("%v outlined: %+v", th.Flavor, l)
		}
		if l := looks[ButtonDashed]; !l.dashed || l.border != pal.Base {
			t.Errorf("%v dashed: %+v", th.Flavor, l)
		}
		if l := looks[ButtonFilled]; l.bg != pal.Bg || l.border.A != 0 {
			t.Errorf("%v filled: %+v", th.Flavor, l)
		}
		if l := looks[ButtonText]; l.bg.A != 0 || l.border.A != 0 {
			t.Errorf("%v text: %+v", th.Flavor, l)
		}
		if l := looks[ButtonPlain]; l.bg != pal.Bg || l.border != pal.Border {
			t.Errorf("%v plain: %+v", th.Flavor, l)
		}
	}
}

// Labels in pale presets are deepened to stay readable; the solid fill
// picks a dark label.
func TestButtonPresetReadable(t *testing.T) {
	th := theme.AntDesign()
	var txt, solid *ButtonW
	d := NewDriver(func(c *Context) {
		Row(c, func() {
			txt = Button(c, "Y").Color(ButtonColorYellow).Appearance(ButtonText)
			solid = Button(c, "S").Color(ButtonColorYellow).Appearance(ButtonSolid)
		})
	}, DriverOptions{Theme: th})
	d.Frame()
	if cr := theme.ContrastRatio(txt.look().fg, th.BgContainer); cr < 3 {
		t.Errorf("yellow text contrast %.2f", cr)
	}
	if fg := solid.look().fg; theme.Luma(fg) > 0.5 {
		t.Errorf("label on yellow is not dark: %v", fg)
	}
	// Hex colors work too.
	var hx *ButtonW
	d = NewDriver(func(c *Context) { hx = Button(c, "H").Color("#13c2c2").Appearance(ButtonSolid) }, DriverOptions{Theme: th})
	d.Frame()
	if hx.look().bg != theme.Hex("#13c2c2") {
		t.Errorf("hex solid bg %v", hx.look().bg)
	}
}

func TestButtonElementTypesAndPlain(t *testing.T) {
	th := theme.ElementPlus()
	var s, w, i, sp, pp, dp, plain *ButtonW
	d := NewDriver(func(c *Context) {
		Row(c, func() {
			s = Button(c, "success").Success()
			w = Button(c, "warning").Warning()
			i = Button(c, "info").Info()
			sp = Button(c, "success plain").Success().Plain()
			pp = Button(c, "primary plain").Primary().Plain()
			dp = Button(c, "danger plain").Danger().Plain()
			plain = Button(c, "plain").Plain()
		}, layout.Gap(8))
	}, DriverOptions{Theme: th})
	d.Frame()
	if s.look().bg != th.Success.Base || w.look().bg != th.Warning.Base || i.look().bg != th.Info.Base {
		t.Errorf("type fills: %v %v %v", s.look().bg, w.look().bg, i.look().bg)
	}
	for _, c := range []struct {
		b   *ButtonW
		pal theme.Palette
	}{{sp, th.Success}, {pp, th.Primary}, {dp, th.Error}} {
		if l := c.b.look(); l.bg != c.pal.Bg || l.border != c.pal.Border || l.fg != c.pal.Base {
			t.Errorf("%s: %+v", c.b.label, l)
		}
	}
	if l := plain.look(); l.border != th.Components.Button.DefaultBorderColor {
		t.Errorf("default plain: %+v", l)
	}
	// Hover fills a plain button with its color.
	d.Move(mustFind(t, d, "success plain").Center())
	d.Frame()
	if l := sp.look(); l.bg != th.Success.Base || l.fg != th.TextOn(th.Success.Base, th.TextOnSolid) {
		t.Errorf("plain hover: %+v", l)
	}
	d.Move(mustFind(t, d, "plain").Center())
	d.Frame()
	if l := plain.look(); l.border != th.Primary.Base || l.fg != th.Primary.Base {
		t.Errorf("default plain hover: %+v", l)
	}
}

func TestButtonComboDisabled(t *testing.T) {
	for _, th := range buttonThemes() {
		var b *ButtonW
		clicked := false
		d := NewDriver(func(c *Context) {
			b = Button(c, "x").Color(ButtonColorGreen).Appearance(ButtonFilled).Disabled(true)
			clicked = clicked || b.Clicked()
		}, DriverOptions{Theme: th})
		d.Frame()
		d.Click(mustFind(t, d, "x").Center())
		if clicked {
			t.Fatalf("%v: disabled combo button clicked", th.Flavor)
		}
		l := b.look()
		if th.Flavor == theme.StyleShadcn {
			if l.opacity != 0.5 {
				t.Errorf("shadcn disabled opacity %v", l.opacity)
			}
		} else if th.Flavor != theme.Element && l.fg != th.TextDisabled {
			t.Errorf("%v disabled fg %v", th.Flavor, l.fg)
		}
	}
}

func TestButtonGroupJoins(t *testing.T) {
	var bs []*ButtonW
	var g *ButtonGroupW
	rtl, vertical := false, false
	d := driver(func(c *Context) {
		bs = bs[:0]
		dir := LTR
		if rtl {
			dir = RTL
		}
		ConfigProvider(c).Direction(dir).Body(func() {
			Column(c, func() {
				g = ButtonGroup(c).Size(Large).A11yLabel("pager")
				if vertical {
					g.Vertical()
				}
				g.Body(func() {
					bs = append(bs, Button(c, "one"), Button(c, "two").Small(), Button(c, "three"))
				})
			}, layout.Pad(20))
		})
	})
	d.Frame()
	d.Frame()
	one, two, three := mustFind(t, d, "one"), mustFind(t, d, "two"), mustFind(t, d, "three")
	if two.X != one.Right()-1 || three.X != two.Right()-1 {
		t.Fatalf("borders not shared: %v %v %v", one, two, three)
	}
	if bs[0].size != Large || bs[1].size != Small {
		t.Fatalf("sizes: group %v, explicit %v", bs[0].size, bs[1].size)
	}
	r0, r1, r2 := bs[0].radius(one), bs[1].radius(two), bs[2].radius(three)
	if r0.TL == 0 || r0.BL == 0 || r0.TR != 0 || r0.BR != 0 {
		t.Errorf("first radii %+v", r0)
	}
	if !r1.IsZero() {
		t.Errorf("middle radii %+v", r1)
	}
	if r2.TR == 0 || r2.BR == 0 || r2.TL != 0 || r2.BL != 0 {
		t.Errorf("last radii %+v", r2)
	}
	// RTL: the first button is at the right with its right corners round.
	rtl = true
	d.Frame()
	d.Frame()
	one, three = mustFind(t, d, "one"), mustFind(t, d, "three")
	if one.X <= three.X {
		t.Fatalf("rtl order: %v %v", one, three)
	}
	if r := bs[0].radius(one); r.TR == 0 || r.TL != 0 {
		t.Errorf("rtl first radii %+v", r)
	}
	// Vertical: stacked with shared horizontal borders.
	rtl, vertical = false, true
	d.Frame()
	d.Frame()
	one, two = mustFind(t, d, "one"), mustFind(t, d, "two")
	if two.Y != one.Bottom()-1 {
		t.Fatalf("vertical: %v %v", one, two)
	}
	if r := bs[0].radius(one); r.TL == 0 || r.TR == 0 || r.BL != 0 || r.BR != 0 {
		t.Errorf("vertical first radii %+v", r)
	}
	if one.W != mustFind(t, d, "three").W {
		t.Errorf("vertical buttons not equal width")
	}
}

func TestButtonGroupColorFocus(t *testing.T) {
	var a, b *ButtonW
	clicks := 0
	d := driver(func(c *Context) {
		ButtonGroup(c).Color(ButtonColorCyan).Appearance(ButtonSolid).Body(func() {
			a = Button(c, "a")
			b = Button(c, "b").Appearance(ButtonOutlined)
			if b.Clicked() {
				clicks++
			}
		})
	})
	d.Frame()
	if a.color != ButtonColorCyan || a.appearance() != ButtonSolid || b.appearance() != ButtonOutlined {
		t.Fatalf("group settings: %v %v %v", a.color, a.appearance(), b.appearance())
	}
	// Every button is a Tab stop and activates with the keyboard.
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyTab, 0)
	d.Key(input.KeyEnter, 0)
	if clicks != 1 {
		t.Fatalf("keyboard clicks %d", clicks)
	}
	// The focused button's ring is drawn by the group over its neighbours.
	if !b.ia.FocusVisible {
		t.Fatal("b not focus-visible")
	}
	l := d.Runner().List()
	if len(l.Ops) == 0 {
		t.Fatal("no ops")
	}
	_ = geom.Rect{}
}

// A button outside a group, or in a plain Row, is unaffected.
func TestButtonNotInGroup(t *testing.T) {
	var b *ButtonW
	d := driver(func(c *Context) {
		Row(c, func() { Button(c, "x"); b = Button(c, "y") })
	})
	d.Frame()
	if b.grp != nil {
		t.Fatal("button joined a Row")
	}
	if r := b.radius(mustFind(t, d, "y")); r.TL == 0 {
		t.Fatalf("radii %+v", r)
	}
}
