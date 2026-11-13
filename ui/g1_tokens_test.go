package ui

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/tangoui/tango/ui/geom"
	"github.com/tangoui/tango/ui/icon"
	"github.com/tangoui/tango/ui/layout"
	"github.com/tangoui/tango/ui/paint"
	"github.com/tangoui/tango/ui/theme"
)

// g1Themes are the six stock themes the LTR output must stay identical in.
func g1Themes() map[string]func() *theme.Theme {
	return map[string]func() *theme.Theme{
		"ant": func() *theme.Theme { return theme.AntDesign() }, "antd": func() *theme.Theme { return theme.AntDesign(theme.Dark()) },
		"el": func() *theme.Theme { return theme.ElementPlus() }, "eld": func() *theme.Theme { return theme.ElementPlus(theme.Dark()) },
		"sh": func() *theme.Theme { return theme.Shadcn() }, "shd": func() *theme.Theme { return theme.Shadcn(theme.Dark()) },
	}
}

// g1Page shows the group's controls in most of their static states. focus
// receives the ID to focus and hover the label to hover after settling.
func g1Page(c *Context, which int, focus *ID) {
	opts := Opts("Apple", "Banana", "Cherry")
	switch which {
	case 0: // buttons
		Column(c, func() {
			for _, sz := range []ControlSize{Small, Middle, Large} {
				Row(c, func() {
					Button(c, "Default").Size(sz)
					Button(c, "Primary").Primary().Size(sz)
					Button(c, "Dashed").Dashed().Size(sz)
					Button(c, "Text").Text().Size(sz)
					Button(c, "Link").Link().Size(sz)
					Button(c, "Second").Secondary().Size(sz)
					Button(c, "Icon").Icon(icon.Search).Size(sz)
					Button(c, "").Icon(icon.Search).Circle().Size(sz)
					Button(c, "Right").IconRight(icon.Right).Size(sz)
				}, layout.Gap(6))
			}
			Row(c, func() {
				Button(c, "Danger").Danger()
				Button(c, "DangerP").Primary().Danger()
				Button(c, "DangerT").Text().Danger()
				Button(c, "DangerS").Secondary().Danger()
				Button(c, "Ghost").Ghost()
				Button(c, "GhostP").Primary().Ghost()
				Button(c, "Round").Round()
				Button(c, "Dis").Disabled(true)
				Button(c, "DisP").Primary().Disabled(true)
				Button(c, "DisT").Text().Disabled(true)
			}, layout.Gap(6))
			Row(c, func() {
				b := Button(c, "Focus").Primary()
				*focus = b.ID()
				IconButton(c, icon.Close, "close")
			}, layout.Gap(6))
		}, layout.Gap(8), layout.Pad(10))
	case 1: // inputs
		var s1, s2, s3, s4, s5 = "hello", "", "secret", "value", "x"
		var n1 = 42.0
		Column(c, func() {
			Input(c, &s1).Prefix(icon.Search).Suffix(icon.Info).AllowClear().Width(240)
			Input(c, &s2).Placeholder("Placeholder").PrefixText("$").SuffixText("kg").Width(240)
			Input(c, &s3).Password().Width(240)
			Input(c, &s4).Status("error").Width(240)
			Input(c, &s4).Status("warning").Size(Small).Width(240)
			Input(c, &s4).Disabled(true).Size(Large).Width(240)
			Input(c, &s5).Variant("filled").Width(240)
			Input(c, &s5).Variant("underlined").Width(240)
			Input(c, &s5).ShowCount().MaxLength(10).Width(240)
			f := Input(c, &s1).Width(240)
			*focus = f.ID()
			InputNumber(c, &n1).Min(0).Max(42)
			InputNumber(c, &n1).ControlsPosition("sides").Width(140)
			InputNumber(c, &n1).PrefixText("¥").Disabled(true)
			Search(c, &s5).EnterButton("Go").Width(240)
			TextArea(c, &s1).ShowCount().AllowClear().Width(240)
		}, layout.Gap(6), layout.Pad(10))
	case 2: // selects
		var v1, v2, v3 = "Banana", "", "Apple"
		var ms = []string{"Apple", "Cherry"}
		Column(c, func() {
			Select(c, &v1, opts).Width(200)
			Select(c, &v2, opts).Placeholder("Pick").Status("error").Width(200)
			Select(c, &v1, opts).Disabled(true).Width(200)
			Select(c, &v1, opts).AllowClear().Size(Large).Width(200)
			MultiSelect(c, &ms, opts).Width(260)
			MultiSelect(c, &ms, opts).Size(Small).AllowClear().Width(260)
			VirtualSelect(c, &v3, opts).Width(200)
			AutoComplete(c, &v2, opts)
			Select(c, &v3, opts).Width(200).Open()
		}, layout.Gap(6), layout.Pad(10))
	case 3: // choices
		var t, f = true, false
		var rv = "Banana"
		var sv = "Banana"
		var rate = 2.5
		var sel = []string{"Apple"}
		Column(c, func() {
			Row(c, func() {
				Checkbox(c, "On", &t)
				Checkbox(c, "Off", &f)
				Checkbox(c, "Half", &f).Indeterminate(true)
				Checkbox(c, "Dis", &t).Disabled(true)
				Checkbox(c, "Err", &f).Status("error")
			}, layout.Gap(8))
			RadioGroup(c, &rv, opts)
			RadioGroup(c, &rv, opts).Buttons()
			RadioGroup(c, &rv, opts).Solid().Size(Small)
			Row(c, func() {
				Switch(c, &t)
				Switch(c, &f)
				Switch(c, &t).Small()
				Switch(c, &f).Texts("ON", "OFF")
				Switch(c, &t).Texts("ON", "OFF")
				Switch(c, &t).Disabled(true)
			}, layout.Gap(8))
			Segmented(c, &sv, opts)
			Segmented(c, &sv, opts).Size(Small)
			Rate(c, &rate).AllowHalf()
			Rate(c, &rate).ShowScore().Texts("a", "b", "c", "d", "e")
			CheckAll(c, &sel, opts)
			Row(c, func() {
				CheckableTag(c, "Tag", &t)
				CheckableTag(c, "Tag2", &f)
				Toggle(c, "Bold", &t)
				Toggle(c, "Ital", &f)
			}, layout.Gap(8))
			ToggleGroup(c, &sel, opts)
			s := "12"
			OTP(c, &s, 4)
			cb := Checkbox(c, "Focus", &t)
			*focus = cb.n.id
		}, layout.Gap(6), layout.Pad(10))
	}
}

const g1Pages = 4

// TestG1Snapshot records (G1SNAP=record) or checks (G1SNAP=check) pixel
// snapshots of the group's controls in the six stock themes, with and
// without keyboard focus; a migration step must leave LTR output identical.
func TestG1Snapshot(t *testing.T) {
	mode := os.Getenv("G1SNAP")
	if mode == "" {
		t.Skip("G1SNAP not set")
	}
	dir := "/tmp/tango-mig/g1/base"
	os.MkdirAll(dir, 0o755)
	for p := 0; p < g1Pages; p++ {
		for tname, th := range g1Themes() {
			for _, foc := range []bool{false, true} {
				name := string(rune('a'+p)) + "_" + tname
				if foc {
					name += "_focus"
				}
				var fid ID
				d := NewDriver(func(c *Context) { g1Page(c, p, &fid) }, DriverOptions{Width: 900, Height: 700, Theme: th()})
				d.Settle()
				if foc && fid != 0 {
					d.r.c.SetFocus(fid)
					d.r.c.focusVisible = true
					r := d.r.c.rects[fid]
					d.Move(geom.Pt(r.X+r.W/2, r.Y+r.H/2))
				}
				img := d.Settle()
				path := filepath.Join(dir, name+".png")
				if mode == "record" {
					d.SavePNG(path)
					continue
				}
				f, err := os.ReadFile(path)
				if err != nil {
					t.Errorf("%s: %v", name, err)
					continue
				}
				want, _ := png.Decode(bytes.NewReader(f))
				if n, box := g1Diff(img, want); n != 0 {
					d.SavePNG(filepath.Join("/tmp/tango-mig/g1", "got_"+name+".png"))
					t.Errorf("%s: %d pixels differ in %v", name, n, box)
				}
			}
		}
	}
}

func g1Diff(a, b image.Image) (int, image.Rectangle) {
	if b == nil || a.Bounds() != b.Bounds() {
		return -1, image.Rectangle{}
	}
	n := 0
	var box image.Rectangle
	r := a.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			r1, g1, b1, a1 := a.At(x, y).RGBA()
			r2, g2, b2, a2 := b.At(x, y).RGBA()
			if r1 != r2 || g1 != g2 || b1 != b2 || a1 != a2 {
				n++
				box = box.Union(image.Rect(x, y, x+1, y+1))
			}
		}
	}
	return n, box
}

// g1Painted reports whether a fill (rect or path) or stroke of color col
// is drawn inside r.
func g1Painted(d *Driver, col theme.Color, r geom.Rect) bool {
	in := func(b geom.Rect) bool { return r.Expand(2).Contains(geom.Pt(b.X+b.W/2, b.Y+b.H/2)) }
	for _, op := range d.r.List().Ops {
		switch o := op.(type) {
		case paint.FillRect:
			if o.Color == col && in(o.Rect) {
				return true
			}
		case paint.StrokeRect:
			if o.Color == col && in(o.Rect) {
				return true
			}
		case paint.FillPath:
			if o.Color == col && o.Path != nil && in(o.Path.Bounds()) {
				return true
			}
		}
	}
	return false
}

// TestG1TokenOverridesAreScoped overrides one token of each migrated
// component inside a nested ConfigProvider and checks that only the
// widget in the scope paints it.
func TestG1TokenOverridesAreScoped(t *testing.T) {
	mark := theme.Hex("#12ab34")
	cases := []struct {
		name  string
		comps theme.Components
		build func(c *Context) ID
	}{
		{"Button", theme.Components{Button: theme.ButtonToken{DefaultBg: mark}}, func(c *Context) ID { return Button(c, "B").ID() }},
		{"Input", theme.Components{Input: theme.InputToken{Bg: mark}}, func(c *Context) ID {
			s := "x"
			return Input(c, &s).Width(120).box.id
		}},
		{"Select", theme.Components{Select: theme.SelectToken{SelectorBg: mark}}, func(c *Context) ID {
			v := "a"
			return Select(c, &v, Opts("a", "b")).Width(120).n.id
		}},
		{"MultiSelect", theme.Components{Select: theme.SelectToken{MultipleItemBg: mark}}, func(c *Context) ID {
			v := []string{"a"}
			return MultiSelect(c, &v, Opts("a", "b")).Width(160).box.n.id
		}},
		{"Checkbox", theme.Components{Checkbox: theme.CheckboxToken{CheckedBg: mark}}, func(c *Context) ID {
			on := true
			return Checkbox(c, "C", &on).n.id
		}},
		{"Radio", theme.Components{Radio: theme.RadioToken{DotColor: mark}}, func(c *Context) ID { return Radio(c, "R", true).n.id }},
		{"Switch", theme.Components{Switch: theme.SwitchToken{OnBg: mark}}, func(c *Context) ID {
			on := true
			return Switch(c, &on).n.id
		}},
		{"Segmented", theme.Components{Segmented: theme.SegmentedToken{TrackBg: mark}}, func(c *Context) ID {
			v := "a"
			return Segmented(c, &v, Opts("a", "b")).n.id
		}},
	}
	for _, tc := range cases {
		for tname, th := range g1Themes() {
			var inner, outer ID
			d := NewDriver(func(c *Context) {
				Column(c, func() {
					outer = tc.build(c)
					ConfigProvider(c).Components(tc.comps).Body(func() { inner = tc.build(c) })
				}, layout.Gap(10), layout.Pad(10))
			}, DriverOptions{Width: 400, Height: 300, Theme: th()})
			d.Settle()
			if !g1Painted(d, mark, d.r.c.rects[inner]) {
				t.Errorf("%s/%s: override not painted inside the scope", tc.name, tname)
			}
			if g1Painted(d, mark, d.r.c.rects[outer]) {
				t.Errorf("%s/%s: override leaked outside the scope", tc.name, tname)
			}
		}
	}
}

// TestG1ButtonAliasRadius: a Button Alias (BorderRadius) reaches the
// button's corners through ThemeFor, without touching inputs.
func TestG1ButtonAliasRadius(t *testing.T) {
	var b, in ID
	d := NewDriver(func(c *Context) {
		ConfigProvider(c).Components(theme.Components{Button: theme.ButtonToken{Alias: theme.Alias{BorderRadius: 13}}}).Body(func() {
			Column(c, func() {
				b = Button(c, "B").ID()
				s := ""
				in = Input(c, &s).Width(100).box.id
			})
		})
	}, DriverOptions{Width: 400, Height: 300})
	d.Settle()
	got := map[ID]float32{}
	for _, op := range d.r.List().Ops {
		if f, ok := op.(paint.StrokeRect); ok {
			for _, id := range []ID{b, in} {
				if f.Rect == d.r.c.rects[id] {
					got[id] = f.Radii.TL
				}
			}
		}
	}
	if got[b] != 13 {
		t.Errorf("button radius %v, want 13", got[b])
	}
	if got[in] == 13 {
		t.Error("button alias reached the input")
	}
}
