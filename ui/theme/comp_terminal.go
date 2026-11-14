package theme

// TerminalToken styles the terminal component (ui/terminal). Background,
// Foreground, Cursor, CursorText, Selection and Palette are the default
// color scheme; an app can replace them per view with a terminal.Scheme.
type TerminalToken struct {
	Alias
	Background Color // default background (SGR 49)
	Foreground Color `contrast:"text:Background"` // default foreground (SGR 39)
	Cursor     Color `contrast:"nontext:Background"`
	CursorText Color `contrast:"text:Cursor"` // the character under a block cursor
	Selection  Color // selected cells
	// Palette holds the 16 ANSI colors (0–7 normal, 8–15 bright); indexed
	// colors 16–255 follow the xterm formula.
	Palette         [16]Color
	ScrollbarThumb  Color   // the overlay scrollback scrollbar
	ContentFontSize float32 // monospace font size of the cells
}

// xtermANSI is the default ANSI palette of xterm.js 6
// (DEFAULT_ANSI_COLORS in src/browser/services/ThemeService.ts, the Tango
// desktop palette). xterm.js has a single default theme (white on black);
// light themes use the same 16 colors with dark text on the page. The same
// values are terminal.SchemeLight / SchemeDark; the other built-in schemes
// (Solarized from Ethan Schoonover's official values, Dracula from the
// official spec at draculatheme.com/spec) live in ui/terminal.
var xtermANSI = [16]Color{
	Hex("#2e3436"), Hex("#cc0000"), Hex("#4e9a06"), Hex("#c4a000"),
	Hex("#3465a4"), Hex("#75507b"), Hex("#06989a"), Hex("#d3d7cf"),
	Hex("#555753"), Hex("#ef2929"), Hex("#8ae234"), Hex("#fce94f"),
	Hex("#729fcf"), Hex("#ad7fa8"), Hex("#34e2e2"), Hex("#eeeeec"),
}

// deriveTerminal blends the terminal into the page: the container
// background and body text of the theme, the selection color of menus and
// trees, the scrollbar color of scroll areas, and the xterm.js palette for
// the ANSI colors (shared by the three design systems and both modes).
func deriveTerminal(t *Theme) TerminalToken {
	sel := t.ItemSelectedBg
	if sel.A == 0 {
		sel = t.Primary.Bg
	}
	return TerminalToken{
		Background: t.BgContainer, Foreground: t.Text,
		Cursor: t.Text, CursorText: t.BgContainer,
		Selection:       sel,
		Palette:         xtermANSI,
		ScrollbarThumb:  t.TextQuaternary, // ui.paintScrollbars idle thumb
		ContentFontSize: 13,
	}
}
