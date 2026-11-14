package ui

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/tangoui/tango/ui/theme"
)

// ---------------------------------------------------------------------------
// ANSI escape sequences (SGR colors and attributes) for LogView.

// ANSIColorKind says how an ANSIColor is given.
type ANSIColorKind uint8

// The color kinds: the default color, one of the 256 indexed colors (0–15
// are the theme's ANSI colors, 16–231 the 6×6×6 cube, 232–255 grays) or a
// 24-bit color.
const (
	ANSIDefault ANSIColorKind = iota
	ANSIIndexed
	ANSIRGB
)

// ANSIColor is a foreground or background color of an SGR sequence.
type ANSIColor struct {
	Kind    ANSIColorKind
	Index   uint8 // ANSIIndexed
	R, G, B uint8 // ANSIRGB
}

// ANSIStyle is the text style set by SGR sequences.
type ANSIStyle struct {
	Fg, Bg                                          ANSIColor
	Bold, Faint, Italic, Underline, Inverse, Strike bool
}

// ANSISpan styles bytes [Start, End) of the plain text ParseANSI returns.
type ANSISpan struct {
	Start, End int
	Style      ANSIStyle
}

// ANSIParser strips escape sequences from lines of terminal output and
// turns SGR sequences ("\x1b[1;31m") into styled spans. The style carries
// over from one line to the next, as in a terminal. Supported: reset,
// bold, faint, italic, underline, inverse, strike-through and their
// resets; the 8 + 8 colors (30–37, 90–97, 40–47, 100–107), 256 colors
// (38;5;n) and 24-bit colors (38;2;r;g;b, also with ":" separators).
// Other CSI sequences, OSC strings and control characters are removed; a
// carriage return inside a line discards what came before it (progress
// output). The zero value is ready to use.
type ANSIParser struct {
	style ANSIStyle
}

// Reset returns the parser to the default style.
func (p *ANSIParser) Reset() { p.style = ANSIStyle{} }

// Line parses one line (without "\n"): it returns the plain text and
// appends its styled spans (default-styled text has none) to dst.
func (p *ANSIParser) Line(s string, dst []ANSISpan) (string, []ANSISpan) {
	if !ansiNeedsWork(s) {
		if p.style != (ANSIStyle{}) && s != "" {
			dst = append(dst, ANSISpan{0, len(s), p.style})
		}
		return s, dst
	}
	base := len(dst)
	var b strings.Builder
	b.Grow(len(s))
	open := -1 // start of the current styled span in b
	flush := func() {
		if open >= 0 && b.Len() > open {
			if n := len(dst); n > base && dst[n-1].End == open && dst[n-1].Style == p.style {
				dst[n-1].End = b.Len()
			} else {
				dst = append(dst, ANSISpan{open, b.Len(), p.style})
			}
		}
		open = -1
	}
	setStyle := func(st ANSIStyle) {
		if st == p.style {
			return
		}
		flush()
		p.style = st
	}
	for i := 0; i < len(s); {
		ch := s[i]
		switch {
		case ch == 0x1b:
			if i+1 >= len(s) {
				i++
				continue
			}
			switch s[i+1] {
			case '[':
				j := i + 2
				for j < len(s) && s[j] >= 0x30 && s[j] <= 0x3f {
					j++
				}
				params := s[i+2 : j]
				for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
					j++
				}
				if j < len(s) {
					if s[j] == 'm' {
						setStyle(ansiSGR(p.style, params))
					}
					j++
				}
				i = j
			case ']', 'P', '_', '^': // OSC, DCS, APC, PM: up to BEL or ST
				j := i + 2
				for j < len(s) {
					if s[j] == 0x07 {
						j++
						break
					}
					if s[j] == 0x1b && j+1 < len(s) && s[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				i = j
			default: // ESC [intermediates] final
				j := i + 1
				for j < len(s) && s[j] >= 0x20 && s[j] <= 0x2f {
					j++
				}
				i = min(j+1, len(s))
			}
			continue
		case ch == '\r':
			if i+1 < len(s) {
				b.Reset()
				dst = dst[:base]
				open = -1
			}
			i++
			continue
		case ch == '\t':
		case ch < 0x20 || ch == 0x7f:
			i++
			continue
		}
		if open < 0 && p.style != (ANSIStyle{}) {
			open = b.Len()
		}
		if ch < utf8.RuneSelf {
			b.WriteByte(ch)
			i++
			continue
		}
		r, sz := utf8.DecodeRuneInString(s[i:])
		if r == 0x9b || (r >= 0x80 && r < 0xa0) { // C1 controls
			i += sz
			continue
		}
		b.WriteString(s[i : i+sz])
		i += sz
	}
	flush()
	return b.String(), dst
}

// ansiNeedsWork reports whether s holds escape sequences or control
// characters (other than tab).
func ansiNeedsWork(s string) bool {
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < 0x20 && c != '\t') || c == 0x7f || c == 0xc2 {
			return true
		}
	}
	return false
}

// ParseANSI parses s (one line, or several separated by "\n" sharing one
// style state) into plain text and styled spans of it.
func ParseANSI(s string) (string, []ANSISpan) {
	var p ANSIParser
	if !strings.Contains(s, "\n") {
		return p.Line(s, nil)
	}
	var b strings.Builder
	var spans []ANSISpan
	first := true
	for line := range strings.SplitSeq(s, "\n") {
		if !first {
			b.WriteByte('\n')
		}
		first = false
		off := b.Len()
		n := len(spans)
		plain, out := p.Line(line, spans)
		for k := n; k < len(out); k++ {
			out[k].Start += off
			out[k].End += off
		}
		spans = out
		b.WriteString(plain)
	}
	return b.String(), spans
}

// ansiSGR applies the SGR parameter string params ("1;38;5;208") to st.
func ansiSGR(st ANSIStyle, params string) ANSIStyle {
	if params == "" {
		return ANSIStyle{}
	}
	if params[0] >= '<' && params[0] <= '?' { // private sequences
		return st
	}
	ps := strings.Split(params, ";")
	num := func(s string) int {
		if i := strings.IndexByte(s, ':'); i >= 0 {
			s = s[:i]
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return 0
		}
		return n
	}
	for i := 0; i < len(ps); i++ {
		p := ps[i]
		code := num(p)
		switch {
		case code == 0:
			st = ANSIStyle{}
		case code == 1:
			st.Bold = true
		case code == 2:
			st.Faint = true
		case code == 3:
			st.Italic = true
		case code == 4:
			st.Underline = !strings.HasPrefix(p, "4:0")
		case code == 7:
			st.Inverse = true
		case code == 9:
			st.Strike = true
		case code == 21:
			st.Underline = true
		case code == 22:
			st.Bold, st.Faint = false, false
		case code == 23:
			st.Italic = false
		case code == 24:
			st.Underline = false
		case code == 27:
			st.Inverse = false
		case code == 29:
			st.Strike = false
		case code >= 30 && code <= 37:
			st.Fg = ANSIColor{Kind: ANSIIndexed, Index: uint8(code - 30)}
		case code == 39:
			st.Fg = ANSIColor{}
		case code >= 40 && code <= 47:
			st.Bg = ANSIColor{Kind: ANSIIndexed, Index: uint8(code - 40)}
		case code == 49:
			st.Bg = ANSIColor{}
		case code >= 90 && code <= 97:
			st.Fg = ANSIColor{Kind: ANSIIndexed, Index: uint8(code - 90 + 8)}
		case code >= 100 && code <= 107:
			st.Bg = ANSIColor{Kind: ANSIIndexed, Index: uint8(code - 100 + 8)}
		case code == 38 || code == 48:
			var col ANSIColor
			var ok bool
			if strings.Contains(p, ":") {
				col, ok = ansiExtColor(strings.Split(p, ":")[1:])
			} else {
				var used int
				col, used, ok = ansiExtColorSemi(ps[i+1:])
				i += used
			}
			if ok {
				if code == 38 {
					st.Fg = col
				} else {
					st.Bg = col
				}
			}
		}
	}
	return st
}

// ansiExtColor parses the colon form "5:n" / "2:[cs]:r:g:b".
func ansiExtColor(sub []string) (ANSIColor, bool) {
	if len(sub) == 0 {
		return ANSIColor{}, false
	}
	at := func(i int) int {
		if i >= len(sub) {
			return 0
		}
		n, _ := strconv.Atoi(sub[i])
		return n
	}
	switch at(0) {
	case 5:
		return ANSIColor{Kind: ANSIIndexed, Index: uint8(max(0, min(255, at(1))))}, len(sub) >= 2
	case 2:
		if len(sub) >= 5 { // 2:colorspace:r:g:b
			return ansiRGB(at(2), at(3), at(4)), true
		}
		return ansiRGB(at(1), at(2), at(3)), len(sub) >= 4
	}
	return ANSIColor{}, false
}

// ansiExtColorSemi parses the semicolon form after 38/48 and returns how
// many parameters it used.
func ansiExtColorSemi(ps []string) (ANSIColor, int, bool) {
	at := func(i int) int {
		if i >= len(ps) {
			return 0
		}
		n, _ := strconv.Atoi(ps[i])
		return n
	}
	if len(ps) == 0 {
		return ANSIColor{}, 0, false
	}
	switch at(0) {
	case 5:
		if len(ps) < 2 {
			return ANSIColor{}, len(ps), false
		}
		return ANSIColor{Kind: ANSIIndexed, Index: uint8(max(0, min(255, at(1))))}, 2, true
	case 2:
		if len(ps) < 4 {
			return ANSIColor{}, len(ps), false
		}
		return ansiRGB(at(1), at(2), at(3)), 4, true
	}
	return ANSIColor{}, 1, false
}

func ansiRGB(r, g, b int) ANSIColor {
	c := func(v int) uint8 { return uint8(max(0, min(255, v))) }
	return ANSIColor{Kind: ANSIRGB, R: c(r), G: c(g), B: c(b)}
}

// ansiColor resolves c with the LogView token's 16 colors (zero for the
// default color).
func ansiColor(tk *theme.LogViewToken, c ANSIColor) theme.Color {
	switch c.Kind {
	case ANSIRGB:
		return theme.Color{R: c.R, G: c.G, B: c.B, A: 255}
	case ANSIIndexed:
		i := int(c.Index)
		if i < 16 {
			return [16]theme.Color{tk.Black, tk.Red, tk.Green, tk.Yellow, tk.Blue, tk.Magenta, tk.Cyan, tk.White,
				tk.BrightBlack, tk.BrightRed, tk.BrightGreen, tk.BrightYellow, tk.BrightBlue, tk.BrightMagenta, tk.BrightCyan, tk.BrightWhite}[i]
		}
		if i < 232 { // 6×6×6 cube
			i -= 16
			lv := func(v int) uint8 {
				if v == 0 {
					return 0
				}
				return uint8(55 + 40*v)
			}
			return theme.Color{R: lv(i / 36), G: lv(i / 6 % 6), B: lv(i % 6), A: 255}
		}
		g := uint8(8 + 10*(i-232))
		return theme.Color{R: g, G: g, B: g, A: 255}
	}
	return theme.Color{}
}
