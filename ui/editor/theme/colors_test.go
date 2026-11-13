package theme

import (
	"image/color"
	"testing"
)

// TestDefaultColorsResolved checks the generated default table against the
// values export_colors.mjs printed from Monaco's registry, and the Go
// resolver (used for themes with colors of their own) against the table.
func TestDefaultColorsResolved(t *testing.T) {
	dark := Base("vs-dark")
	// From the script's log (vs-dark column).
	for key, want := range map[string]color.NRGBA{
		"editorIndentGuide.background1":                       {0x40, 0x40, 0x40, 0xff},
		"editorIndentGuide.activeBackground1":                 {0x70, 0x70, 0x70, 0xff},
		"editorBracketHighlight.foreground1":                  {0xff, 0xd7, 0x00, 0xff},
		"editorWhitespace.foreground":                         {0xe3, 0xe4, 0xe2, 0x29},
		"editorBracketHighlight.unexpectedBracket.foreground": {0xff, 0x12, 0x12, 0xcc},
		"editorBracketMatch.border":                           {0x88, 0x88, 0x88, 0xff},
	} {
		if got, ok := dark.Color(key); !ok || got != want {
			t.Errorf("vs-dark %s = %v %v, want %v", key, got, ok, want)
		}
	}
	vs := Base("vs")
	for key, want := range map[string]color.NRGBA{
		"editorIndentGuide.background1":      {0xd3, 0xd3, 0xd3, 0xff},
		"editorBracketHighlight.foreground1": {0x04, 0x31, 0xfa, 0xff},
		"editorWhitespace.foreground":        {0x33, 0x33, 0x33, 0x33},
		"editorGutter.background":            {0xff, 0xff, 0xfe, 0xff}, // vs's own editor.background
		"editor.foldBackground":              {0xad, 0xd6, 0xff, 0x4d}, // transparent(selection, 0.3)
	} {
		if got, ok := vs.Color(key); !ok || got != want {
			t.Errorf("vs %s = %v %v, want %v", key, got, ok, want)
		}
	}
	if c, ok := vs.Color("editor.lineHighlightBackground"); ok {
		t.Errorf("vs editor.lineHighlightBackground = %v, want none (null default)", c)
	}
	if _, ok := Base("hc-black").Color("editor.foldBackground"); ok {
		t.Error("hc-black editor.foldBackground: want none")
	}

	// The resolver reproduces the generated final colors for every id
	// under every base theme.
	for _, name := range []string{"vs", "vs-dark", "hc-black", "hc-light"} {
		b := Base(name)
		for id := range colorDefaults {
			want, wok := defaultColor(id, baseType(name))
			if own, ok := b.OwnColor(id); ok {
				want, wok = own, true
			}
			got, ok := resolveDefault(b, id)
			if own, isOwn := b.OwnColor(id); isOwn {
				got, ok = own, true
			}
			if got != want || ok != wok {
				t.Errorf("%s %s: resolver %v %v, table %v %v", name, id, got, ok, want, wok)
			}
		}
	}
}

// TestCustomThemeDefaults: defaults follow a custom theme's own colors.
func TestCustomThemeDefaults(t *testing.T) {
	th, err := Parse([]byte(`{"base": "vs-dark", "inherit": true, "rules": [],
		"colors": {"editor.background": "#102030", "editor.selectionBackground": "#808000"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := th.Color("editorGutter.background"); got != (color.NRGBA{0x10, 0x20, 0x30, 0xff}) {
		t.Errorf("editorGutter.background = %v, want the theme's editor.background", got)
	}
	if got, _ := th.Color("editor.foldBackground"); got != (color.NRGBA{0x80, 0x80, 0x00, 0x4d}) {
		t.Errorf("editor.foldBackground = %v, want transparent(selection, 0.3)", got)
	}
	// Defined by vs-dark itself: inherited, not recomputed.
	if got, _ := th.Color("editor.inactiveSelectionBackground"); got != (color.NRGBA{0x3a, 0x3d, 0x41, 0xff}) {
		t.Errorf("editor.inactiveSelectionBackground = %v", got)
	}
	if _, ok := th.Color("no.such.color"); ok {
		t.Error("unregistered id resolved")
	}
	// An inheriting theme without colors of its own is plain.
	th, _ = Parse([]byte(`{"base": "vs", "inherit": true, "rules": []}`))
	if !th.plain {
		t.Error("theme without own colors not plain")
	}
}

func TestColorMath(t *testing.T) {
	// Monaco: Color.fromHex('#264f78').lighten(0.2) ... values from color.js.
	c := rgbaColor(0x26, 0x4f, 0x78, 1)
	if got := c.transparent(0.5).nrgba(); got != (color.NRGBA{0x26, 0x4f, 0x78, 0x80}) {
		t.Errorf("transparent = %v", got)
	}
	h := c.hslaOf()
	if h.h != 210 || h.s != 0.519 || h.l != 0.31 {
		t.Errorf("hsla = %+v", h)
	}
}
