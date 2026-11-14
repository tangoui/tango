package terminal

import (
	"testing"

	"github.com/tangoui/tango/ui/terminal/vt"
	"github.com/tangoui/tango/ui/theme"
)

func TestResolveColor256(t *testing.T) {
	s := &SchemeDark
	for _, tc := range []struct {
		n    uint8
		want string
	}{
		{16, "#000000"}, {17, "#00005f"}, {21, "#0000ff"}, {196, "#ff0000"}, {231, "#ffffff"},
		{59, "#5f5f5f"}, {145, "#afafaf"},
		{232, "#080808"}, {244, "#808080"}, {255, "#eeeeee"},
	} {
		if got := ResolveColor(vt.IndexedColor(tc.n), s, true); got != theme.Hex(tc.want) {
			t.Errorf("index %d = %s, want %s", tc.n, theme.HexString(got), tc.want)
		}
	}
	for i := range 16 {
		if got := ResolveColor(vt.IndexedColor(uint8(i)), s, false); got != s.Palette[i] {
			t.Errorf("index %d = %s, want palette %s", i, theme.HexString(got), theme.HexString(s.Palette[i]))
		}
	}
	if got := ResolveColor(vt.RGBColor(0x12, 0x34, 0x56), s, true); got != theme.Hex("#123456") {
		t.Errorf("RGB = %s", theme.HexString(got))
	}
}

func TestResolveColorDefault(t *testing.T) {
	for _, s := range []*Scheme{&SchemeLight, &SchemeDark, &SchemeSolarizedLight, &SchemeSolarizedDark, &SchemeDracula} {
		if got := ResolveColor(vt.ColorDefault, s, true); got != s.Foreground {
			t.Errorf("default fg = %s, want %s", theme.HexString(got), theme.HexString(s.Foreground))
		}
		if got := ResolveColor(vt.ColorDefault, s, false); got != s.Background {
			t.Errorf("default bg = %s, want %s", theme.HexString(got), theme.HexString(s.Background))
		}
	}
}

func TestSchemes(t *testing.T) {
	for name, s := range map[string]Scheme{"light": SchemeLight, "dark": SchemeDark, "solarized-light": SchemeSolarizedLight,
		"solarized-dark": SchemeSolarizedDark, "dracula": SchemeDracula} {
		for _, c := range append([]theme.Color{s.Background, s.Foreground, s.Cursor, s.CursorText, s.Selection}, s.Palette[:]...) {
			if c.A == 0 {
				t.Errorf("%s: zero color", name)
			}
		}
		if r := theme.ContrastRatio(s.Foreground, s.Background); r < 4 {
			t.Errorf("%s: foreground on background %.2f", name, r)
		}
	}
	if SchemeDark.Palette[1] != theme.Hex("#cc0000") || SchemeDark.Background != theme.Hex("#000000") || SchemeLight.Background != theme.Hex("#ffffff") {
		t.Error("xterm.js defaults")
	}
	if SchemeSolarizedDark.Background != theme.Hex("#002b36") || SchemeSolarizedLight.Background != theme.Hex("#fdf6e3") ||
		SchemeSolarizedDark.Palette[9] != theme.Hex("#cb4b16") {
		t.Error("Solarized")
	}
	if SchemeDracula.Background != theme.Hex("#282a36") || SchemeDracula.Palette[4] != theme.Hex("#bd93f9") {
		t.Error("Dracula")
	}
}

// TestTokenMatchesSchemes: the terminal token's palette is the built-in
// scheme's for the theme's mode.
func TestTokenMatchesSchemes(t *testing.T) {
	if p := theme.AntDesign().Components.Terminal.Palette; p != SchemeLight.Palette {
		t.Errorf("light token palette %v", p)
	}
	if p := theme.AntDesign(theme.Dark()).Components.Terminal.Palette; p != SchemeDark.Palette {
		t.Errorf("dark token palette %v", p)
	}
}
