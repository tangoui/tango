package theme

import (
	"reflect"
	"strings"
	"testing"
)

func TestTerminalTokenDefaults(t *testing.T) {
	for _, c := range ctors {
		for _, dark := range []bool{false, true} {
			var opts []Option
			if dark {
				opts = append(opts, Dark())
			}
			th := c.mk(opts...)
			k := th.Components.Terminal
			if k.Background != th.BgContainer || k.Foreground != th.Text {
				t.Errorf("%s dark=%v: Background/Foreground not from BgContainer/Text", c.name, dark)
			}
			want := xtermANSI
			if k.Palette != want {
				t.Errorf("%s dark=%v: palette %v", c.name, dark, k.Palette)
			}
			for i, p := range k.Palette {
				if p == (Color{}) {
					t.Errorf("%s dark=%v: Palette[%d] zero", c.name, dark, i)
				}
			}
			if k.Cursor == (Color{}) || k.CursorText == (Color{}) || k.Selection == (Color{}) || k.ScrollbarThumb == (Color{}) {
				t.Errorf("%s dark=%v: zero color in %+v", c.name, dark, k)
			}
			if k.ScrollbarThumb != th.TextQuaternary {
				t.Errorf("%s dark=%v: ScrollbarThumb %v, want TextQuaternary", c.name, dark, k.ScrollbarThumb)
			}
			if k.ContentFontSize != 13 {
				t.Errorf("%s dark=%v: ContentFontSize %v", c.name, dark, k.ContentFontSize)
			}
			if r := ContrastRatio(k.Foreground, k.Background); r < 4.5 {
				t.Errorf("%s dark=%v: Foreground on Background %.2f", c.name, dark, r)
			}
			for _, chk := range CheckContrast(HighContrast(th)) {
				if chk.Status == ContrastStatusFail || chk.Status == ContrastStatusDesignDefault {
					t.Errorf("%s dark=%v high contrast: %s", c.name, dark, chk)
				}
			}
		}
	}
}

// TestTerminalPaletteOverride: overriding one palette entry keeps the
// others and changes the override hash.
func TestTerminalPaletteOverride(t *testing.T) {
	var o Components
	o.Terminal.Palette[1] = Hex("#ff0000")
	th := AntDesign(WithComponents(o))
	p := th.Components.Terminal.Palette
	if p[1] != Hex("#ff0000") || p[0] != xtermANSI[0] || p[15] != xtermANSI[15] {
		t.Errorf("palette override = %v", p)
	}
	if o.Hash() == (Components{}).Hash() {
		t.Error("palette override does not change Hash")
	}
	var o2 Components
	o2.Terminal.Palette[2] = Hex("#00ff00")
	m := o.Merge(o2).Terminal.Palette
	if m[1] != Hex("#ff0000") || m[2] != Hex("#00ff00") || !reflect.DeepEqual(m[3], Color{}) {
		t.Errorf("merged palette = %v", m)
	}
}

// TestTerminalPaletteHighContrast: HighContrast lifts every ANSI color to
// 4.5:1 on the terminal background; the normal theme keeps xterm.js's.
func TestTerminalPaletteHighContrast(t *testing.T) {
	for _, th := range BuiltinThemes() {
		if th.Components.Terminal.Palette != xtermANSI {
			t.Errorf("%s: stock palette changed", themeLabel(th))
		}
		k := HighContrast(th).Components.Terminal
		for i, p := range k.Palette {
			if r := ContrastRatio(p, k.Background); r < 4.5 {
				t.Errorf("%s high contrast: Palette[%d] %s on %s %.2f", themeLabel(th), i, HexString(p), HexString(k.Background), r)
			}
		}
	}
}

// TestTerminalPaletteAudit: the contrast audit measures all 16 palette
// entries of a stock theme; the xterm.js values below AA are design
// defaults, an app's override below AA fails.
func TestTerminalPaletteAudit(t *testing.T) {
	for _, th := range BuiltinThemes() {
		n := 0
		for _, c := range CheckContrast(th) {
			if !strings.HasPrefix(c.Token, "Terminal.Palette[") {
				continue
			}
			n++
			if c.On != "Terminal.Background" || c.Kind != ContrastText {
				t.Errorf("%s: %s", themeLabel(th), c)
			}
			if !c.Pass() && (c.Status != ContrastStatusDesignDefault || c.Spec == "") {
				t.Errorf("%s: want design default: %s", themeLabel(th), c)
			}
		}
		if n != 16 {
			t.Errorf("%s: %d palette pairs, want 16", themeLabel(th), n)
		}
	}
	var o Components
	o.Terminal.Palette[3] = Hex("#ffff00")
	var got []string
	for _, c := range ContrastFailures(AntDesign(WithComponents(o))) {
		got = append(got, c.Token)
	}
	if s := strings.Join(got, ","); s != "Terminal.Palette[3]" {
		t.Errorf("override failures = %s, want Terminal.Palette[3]", s)
	}
}
