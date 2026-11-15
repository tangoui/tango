package ui

import (
	"image/color"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/input"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/theme"
)

// shad2aRect returns the last-frame rectangle of widget id.
func shad2aRect(t *testing.T, d *Driver, id ID) geom.Rect {
	t.Helper()
	r, ok := d.r.c.RectOf(id)
	if !ok {
		t.Fatalf("no rect for %v", id)
	}
	return r
}

func shad2aPx(d *Driver, x, y float32) color.RGBA {
	img := d.Image()
	return img.RGBAAt(int(x), int(y))
}

func shad2aIsBg(c color.RGBA, th *theme.Theme) bool {
	bg := th.BgContainer
	d := func(a, b uint8) int {
		if a > b {
			return int(a - b)
		}
		return int(b - a)
	}
	return d(c.R, bg.R)+d(c.G, bg.G)+d(c.B, bg.B) < 6
}

func TestShad2aButtonSizes(t *testing.T) {
	for _, dark := range []bool{false, true} {
		th := theme.Shadcn()
		if dark {
			th = theme.Shadcn(theme.Dark())
		}
		var ids [4]ID
		d := NewDriver(func(c *Context) {
			Row(c, func() {
				ids[0] = Button(c, "Mid").ID()
				ids[1] = Button(c, "Sm").Small().ID()
				ids[2] = Button(c, "Lg").Large().ID()
				ids[3] = Button(c, "").Icon(nil).Circle().A11yLabel("i").ID()
			})
		}, DriverOptions{Theme: th})
		d.Settle()
		want := []float32{36, 32, 40, 36}
		for i, id := range ids {
			if r := shad2aRect(t, d, id); r.H != want[i] {
				t.Errorf("dark=%v button %d height %v, want %v", dark, i, r.H, want[i])
			}
		}
		c := d.r.c
		if c.controlRadius(Small) != th.Radius || c.controlRadius(Large) != th.Radius {
			t.Errorf("shadcn controls are rounded-md at every size")
		}
		if c.controlFont(Large) != 14 {
			t.Errorf("shadcn large control font %v, want text-sm 14", c.controlFont(Large))
		}
	}
	// Ant keeps its per-size radius and 16px large font.
	d := NewDriver(func(c *Context) { Button(c, "x") }, DriverOptions{Theme: theme.AntDesign()})
	d.Settle()
	if c := d.r.c; c.controlRadius(Small) != 4 || c.controlFont(Large) != 16 {
		t.Errorf("Ant control metrics changed")
	}
}

func TestShad2aButtonLargePadding(t *testing.T) {
	var lg, md ID
	d := NewDriver(func(c *Context) {
		Row(c, func() {
			md = Button(c, "Go").ID()
			lg = Button(c, "Go").Large().ID()
		})
	}, DriverOptions{Theme: theme.Shadcn()})
	d.Settle()
	// px-6 vs px-4 with the same text-sm label: 16px wider.
	if dw := shad2aRect(t, d, lg).W - shad2aRect(t, d, md).W; dw != 16 {
		t.Errorf("lg - default width = %v, want 16 (px-6 vs px-4)", dw)
	}
}

func TestShad2aFocusRing(t *testing.T) {
	th := theme.Shadcn()
	var id ID
	d := NewDriver(func(c *Context) {
		Column(c, func() { id = Button(c, "Focus me").ID() }, layout.Pad(20))
	}, DriverOptions{Theme: th})
	d.Settle()
	r := shad2aRect(t, d, id)
	in0, out0 := shad2aPx(d, r.X-2, r.Y+r.H/2), shad2aPx(d, r.X-5, r.Y+r.H/2)
	d.Key(input.KeyTab, 0)
	d.Settle()
	// ring-[3px]: the halo covers the 3px right outside the border box …
	if shad2aPx(d, r.X-2, r.Y+r.H/2) == in0 {
		t.Errorf("no 3px focus halo outside the button")
	}
	// … and stops there (no offset ring further out).
	if shad2aPx(d, r.X-5, r.Y+r.H/2) != out0 {
		t.Errorf("focus ring extends beyond 3px")
	}
}

func TestShad2aCheckboxInvalid(t *testing.T) {
	th := theme.Shadcn()
	v := false
	var plain, bad ID
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			plain = Checkbox(c, "a", &v).n.id
			bad = Checkbox(c, "b", &v).Status("error").n.id
		})
	}, DriverOptions{Theme: th})
	d.Settle()
	edge := func(id ID) color.RGBA {
		r := shad2aRect(t, d, id)
		bs := th.CheckboxSize
		return shad2aPx(d, r.X, r.Y+(r.H-bs)/2+bs/2)
	}
	p, b := edge(plain), edge(bad)
	if !(int(b.R) > int(b.G)+60) {
		t.Errorf("invalid checkbox border not destructive: %v", b)
	}
	if int(p.R) > int(p.G)+30 {
		t.Errorf("valid checkbox border is red: %v", p)
	}
}

func TestShad2aCheckboxInvalidAntUnchanged(t *testing.T) {
	th := theme.AntDesign()
	v := false
	var plain, bad ID
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			plain = Checkbox(c, "a", &v).n.id
			bad = Checkbox(c, "b", &v).Status("error").n.id
		})
	}, DriverOptions{Theme: th})
	d.Settle()
	pr, br := shad2aRect(t, d, plain), shad2aRect(t, d, bad)
	bs := th.CheckboxSize
	p := shad2aPx(d, pr.X, pr.Y+(pr.H-bs)/2+bs/2)
	b := shad2aPx(d, br.X, br.Y+(br.H-bs)/2+bs/2)
	if p != b {
		t.Errorf("Ant checkbox look depends on Status: %v vs %v", p, b)
	}
}

func TestShad2aSwitchGeometry(t *testing.T) {
	on := true
	var md, sm ID
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			md = Switch(c, &on).A11yLabel("m").n.id
			sm = Switch(c, &on).Small().A11yLabel("s").n.id
		})
	}, DriverOptions{Theme: theme.Shadcn()})
	d.Settle()
	if r := shad2aRect(t, d, md); r.W != 32 {
		t.Errorf("switch width %v, want w-8 (32)", r.W)
	}
	if r := shad2aRect(t, d, sm); r.W != 24 {
		t.Errorf("small switch width %v, want w-6 (24)", r.W)
	}
}

func TestShad2aSwitchDarkThumb(t *testing.T) {
	th := theme.Shadcn(theme.Dark())
	if th.SwitchHandleOff != th.Shadcn.Foreground {
		t.Errorf("dark off thumb should be foreground")
	}
	if l := theme.Shadcn(); l.SwitchHandleOff != l.SwitchHandle {
		t.Errorf("light off thumb should be background")
	}
}

func TestShad2aInputInvalidAndDisabled(t *testing.T) {
	th := theme.Shadcn()
	th.BgLayout = th.BgContainer // page = card color, as in the gallery
	a, b, cc := "x", "y", "z"
	var ok, bad, dis ID
	d := NewDriver(func(c *Context) {
		Column(c, func() {
			ok = Input(c, &a).Width(160).box.id
			bad = Input(c, &b).Width(160).Status("error").box.id
			dis = Input(c, &cc).Width(160).Disabled(true).box.id
		}, layout.Pad(20))
	}, DriverOptions{Theme: th})
	d.Settle()
	mid := func(id ID) color.RGBA {
		r := shad2aRect(t, d, id)
		return shad2aPx(d, r.X+r.W/2, r.Y)
	}
	if px := mid(bad); !(int(px.R) > int(px.G)+60) {
		t.Errorf("invalid input border not destructive: %v", px)
	}
	// Disabled fades the field (opacity-50) instead of a gray fill: the
	// inside stays the page color.
	r := shad2aRect(t, d, dis)
	if px := shad2aPx(d, r.X+r.W-6, r.Y+r.H/2); !shad2aIsBg(px, th) {
		t.Errorf("disabled shadcn input has a fill: %v", px)
	}
	_ = ok
}

func TestShad2aSelectPopupRadius(t *testing.T) {
	if r := theme.Shadcn().Components.Select.PopupRadius; r != 8 { // rounded-md = --radius (10) - 2
		t.Errorf("shadcn select panel radius %v, want rounded-md 8", r)
	}
	if r := theme.AntDesign().Components.Select.PopupRadius; r != 8 {
		t.Errorf("Ant select panel radius changed: %v", r)
	}
}

func TestShad2aDefaults(t *testing.T) {
	var btn, explicit, sel, otp ID
	var sw *SwitchW
	var rate *RateW
	var rg *RadioGroupW
	var num *InputNumberW
	var ta *TextAreaW
	var cp *ColorPickerW
	s, v, n := "", 0.0, 1.0
	on := false
	col := theme.Hex("#ff0000")
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).
			Defaults("Button", Props{"size": "small", "shape": "round", "type": "primary"}).
			Defaults("Select", Props{"size": "large"}).
			Defaults("Switch", Props{"size": "small"}).
			Defaults("Rate", Props{"count": 10, "allowHalf": true}).
			Defaults("Radio", Props{"optionType": "button", "buttonStyle": "solid"}).
			Defaults("InputNumber", Props{"size": "large", "placeholder": "n"}).
			Defaults("TextArea", Props{"rows": 5, "placeholder": "t"}).
			Defaults("ColorPicker", Props{"showText": true}).
			Defaults("OTP", Props{"size": "small"}).
			Body(func() {
				Column(c, func() {
					b := Button(c, "a")
					btn = b.ID()
					if !b.round || b.variant != VariantPrimary {
						t.Errorf("Button shape/type defaults not applied")
					}
					explicit = Button(c, "b").Large().ID()
					sel = Select(c, &s, Opts("x")).n.id
					sw = Switch(c, &on).A11yLabel("s")
					rate = Rate(c, &v)
					rg = RadioGroup(c, &s, Opts("x", "y"))
					num = InputNumber(c, &n)
					ta = TextArea(c, &s)
					cp = ColorPicker(c, &col)
					otp = OTP(c, &s, 4).n.id
				})
			})
	}, DriverOptions{Theme: theme.AntDesign()})
	d.Settle()
	if h := shad2aRect(t, d, btn).H; h != 24 {
		t.Errorf("Button size default: height %v, want 24", h)
	}
	if h := shad2aRect(t, d, explicit).H; h != 40 {
		t.Errorf("explicit Large must win over the default: %v", h)
	}
	if h := shad2aRect(t, d, sel).H; h != 40 {
		t.Errorf("Select size default: height %v, want 40", h)
	}
	if h := shad2aRect(t, d, otp).H; h != 24 {
		t.Errorf("OTP size default: height %v, want 24", h)
	}
	if !sw.small || rate.count != 10 || !rate.half || !rg.buttons || !rg.solid {
		t.Errorf("Switch/Rate/Radio defaults not applied")
	}
	if num.size != Large || num.placeholder != "n" || ta.rows != 5 || ta.placeholder != "t" || !cp.showText {
		t.Errorf("InputNumber/TextArea/ColorPicker defaults not applied")
	}
}

func TestShad2aParseSize(t *testing.T) {
	cases := map[any]ControlSize{"small": Small, "large": Large, "middle": Middle, Large: Large}
	for in, want := range cases {
		if got, ok := shad2aParseSize(in); !ok || got != want {
			t.Errorf("%v -> %v %v", in, got, ok)
		}
	}
	if _, ok := shad2aParseSize(3); ok {
		t.Errorf("an int is not a size")
	}
}
