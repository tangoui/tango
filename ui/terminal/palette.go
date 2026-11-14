package terminal

import (
	"github.com/tangoui/tango/ui/terminal/vt"
	"github.com/tangoui/tango/ui/theme"
)

// Scheme is a terminal color scheme: the default colors and the 16 ANSI
// colors (0–7 normal, 8–15 bright). Indexed colors 16–255 are computed
// with the xterm formula.
type Scheme struct {
	Background, Foreground theme.Color
	Cursor                 theme.Color
	CursorText             theme.Color // the character under a block cursor
	Selection              theme.Color // selected cells (may be translucent)
	Palette                [16]theme.Color
}

// xtermANSI is the default ANSI palette of xterm.js 6
// (DEFAULT_ANSI_COLORS in src/browser/services/ThemeService.ts); the
// terminal token (theme.TerminalToken) uses the same values.
var xtermANSI = [16]theme.Color{
	theme.Hex("#2e3436"), theme.Hex("#cc0000"), theme.Hex("#4e9a06"), theme.Hex("#c4a000"),
	theme.Hex("#3465a4"), theme.Hex("#75507b"), theme.Hex("#06989a"), theme.Hex("#d3d7cf"),
	theme.Hex("#555753"), theme.Hex("#ef2929"), theme.Hex("#8ae234"), theme.Hex("#fce94f"),
	theme.Hex("#729fcf"), theme.Hex("#ad7fa8"), theme.Hex("#34e2e2"), theme.Hex("#eeeeec"),
}

// Solarized, Ethan Schoonover's official values
// (ethanschoonover.com/solarized, the README's 16-color terminal table).
var (
	solBase03  = theme.Hex("#002b36")
	solBase02  = theme.Hex("#073642")
	solBase01  = theme.Hex("#586e75")
	solBase00  = theme.Hex("#657b83")
	solBase0   = theme.Hex("#839496")
	solBase1   = theme.Hex("#93a1a1")
	solBase2   = theme.Hex("#eee8d5")
	solBase3   = theme.Hex("#fdf6e3")
	solPalette = [16]theme.Color{
		solBase02, theme.Hex("#dc322f"), theme.Hex("#859900"), theme.Hex("#b58900"),
		theme.Hex("#268bd2"), theme.Hex("#d33682"), theme.Hex("#2aa198"), solBase2,
		solBase03, theme.Hex("#cb4b16"), solBase01, solBase00,
		solBase0, theme.Hex("#6c71c4"), solBase1, solBase3,
	}
)

// Built-in schemes.
var (
	// SchemeDark is xterm.js 6's default theme: white on black, a 30%
	// white selection and the default ANSI palette.
	SchemeDark = Scheme{
		Background: theme.Hex("#000000"), Foreground: theme.Hex("#ffffff"),
		Cursor: theme.Hex("#ffffff"), CursorText: theme.Hex("#000000"),
		Selection: theme.RGBA(255, 255, 255, .3),
		Palette:   xtermANSI,
	}
	// SchemeLight is xterm.js's default palette with the default colors
	// inverted (xterm.js has no light theme of its own): black on white,
	// a 30% black selection.
	SchemeLight = Scheme{
		Background: theme.Hex("#ffffff"), Foreground: theme.Hex("#000000"),
		Cursor: theme.Hex("#000000"), CursorText: theme.Hex("#ffffff"),
		Selection: theme.RGBA(0, 0, 0, .3),
		Palette:   xtermANSI,
	}
	// SchemeSolarizedLight is Solarized light: base00 on base3, base2
	// selection.
	SchemeSolarizedLight = Scheme{
		Background: solBase3, Foreground: solBase00,
		Cursor: solBase00, CursorText: solBase3,
		Selection: solBase2,
		Palette:   solPalette,
	}
	// SchemeSolarizedDark is Solarized dark: base0 on base03, base02
	// selection.
	SchemeSolarizedDark = Scheme{
		Background: solBase03, Foreground: solBase0,
		Cursor: solBase0, CursorText: solBase03,
		Selection: solBase02,
		Palette:   solPalette,
	}
	// SchemeDracula is Dracula (official spec, draculatheme.com/spec).
	SchemeDracula = Scheme{
		Background: theme.Hex("#282a36"), Foreground: theme.Hex("#f8f8f2"),
		Cursor: theme.Hex("#f8f8f2"), CursorText: theme.Hex("#282a36"),
		Selection: theme.Hex("#44475a"),
		Palette: [16]theme.Color{
			theme.Hex("#21222c"), theme.Hex("#ff5555"), theme.Hex("#50fa7b"), theme.Hex("#f1fa8c"),
			theme.Hex("#bd93f9"), theme.Hex("#ff79c6"), theme.Hex("#8be9fd"), theme.Hex("#f8f8f2"),
			theme.Hex("#6272a4"), theme.Hex("#ff6e6e"), theme.Hex("#69ff94"), theme.Hex("#ffffa5"),
			theme.Hex("#d6acff"), theme.Hex("#ff92df"), theme.Hex("#a4ffff"), theme.Hex("#ffffff"),
		},
	}
)

// cubeLevels are the six levels of xterm's 6×6×6 color cube (16–231).
var cubeLevels = [6]uint8{0, 95, 135, 175, 215, 255}

// ResolveColor returns the color c draws with under scheme s. The default
// color is s.Foreground when fg is true, else s.Background; indexes 0–15
// are s.Palette, 16–231 the xterm 6×6×6 cube, 232–255 the 24-step gray
// ramp (8 + 10·(n−232)); RGB colors are returned as they are.
func ResolveColor(c vt.Color, s *Scheme, fg bool) theme.Color {
	switch c.Kind() {
	case vt.ColorIndexed:
		n := c.Index()
		switch {
		case n < 16:
			return s.Palette[n]
		case n < 232:
			i := n - 16
			return theme.Color{R: cubeLevels[i/36], G: cubeLevels[i/6%6], B: cubeLevels[i%6], A: 255}
		default:
			v := 8 + 10*(n-232)
			return theme.Color{R: v, G: v, B: v, A: 255}
		}
	case vt.ColorRGB:
		r, g, b := c.RGB()
		return theme.Color{R: r, G: g, B: b, A: 255}
	}
	if fg {
		return s.Foreground
	}
	return s.Background
}
